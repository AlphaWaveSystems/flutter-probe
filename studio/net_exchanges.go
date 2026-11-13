package main

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// Studio's view of the app's recorded backend traffic: the exchanges the agent saw during a run, read with the same probe.http_log
// call the runner uses, and shown through the redactor (net_redact.go). It makes no request of its own, writes nothing to disk or to
// a log, and nothing it reads goes to an AI provider or anywhere else.

const (
	netMaxRows    = 500 // the newest exchanges that a listing returns
	netLogTimeout = 10 * time.Second
	netMaxFilter  = 200
)

// Fixed texts: the agent's own error can carry an address or a header, so it is never shown.
var (
	errNetNotConnected = errors.New("connect to a device first: the request log is read from the app")
	errNetLogFailed    = errors.New("the app did not answer the request log")
	errNetNoExchange   = errors.New("that request is no longer in the log")
	errNetEpoch        = errors.New("the app restarted since the list was read: refresh the list")
)

// NetFilter selects rows of the listing. Method, Status and Text are matched against what is shown, never against a raw value, so a
// filter cannot confirm a secret by whether it finds it.
type NetFilter struct {
	Since  int    `json:"since"`  // only exchanges after this sequence number
	Method string `json:"method"` // "GET", "POST"... or empty
	Status string `json:"status"` // "2xx", "3xx", "4xx", "5xx", "error" or empty
	Text   string `json:"text"`   // part of the method, the URL, the status or the error, any case
}

// NetRow is one exchange in the listing: no headers and no bodies.
type NetRow struct {
	Seq           int    `json:"seq"`
	Method        string `json:"method"`
	URL           string `json:"url"`
	Status        int    `json:"status"`
	DurationMs    int    `json:"durationMs"`
	Mocked        bool   `json:"mocked"`
	Error         string `json:"error,omitempty"`
	RequestBytes  int64  `json:"requestBytes"`
	ResponseBytes int64  `json:"responseBytes"`
}

// NetList is the listing. Epoch changes when the app restarts, which starts the sequence over: a page that sees it change starts its
// list again.
type NetList struct {
	Epoch  string   `json:"epoch"`
	Latest int      `json:"latest"`
	Total  int      `json:"total"` // exchanges that matched, before the cap
	Shown  int      `json:"shown"` // rows returned
	Rows   []NetRow `json:"rows"`
}

// NetDetail is one exchange with its headers and bodies, all redacted.
type NetDetail struct {
	Epoch string `json:"epoch"`
	NetRow
	RequestHeaders      []NetHeader `json:"requestHeaders"`
	ResponseHeaders     []NetHeader `json:"responseHeaders"`
	MoreRequestHeaders  int         `json:"moreRequestHeaders,omitempty"`
	MoreResponseHeaders int         `json:"moreResponseHeaders,omitempty"`
	RequestBody         NetBody     `json:"requestBody"`
	ResponseBody        NetBody     `json:"responseBody"`
}

func netRow(e probelink.HTTPEntry) NetRow {
	reqBytes, respBytes := e.RequestBytes, e.ResponseBytes
	if reqBytes == 0 {
		reqBytes = int64(len(e.RequestBody)) // an agent that does not report sizes
	}
	if respBytes == 0 {
		respBytes = int64(len(e.ResponseBody))
	}
	return NetRow{
		Seq:           e.Seq,
		Method:        strings.ToUpper(shortText(e.Method, netMaxMethod)),
		URL:           redactURL(e.URL),
		Status:        e.Status,
		DurationMs:    e.DurationMs,
		Mocked:        e.Mocked,
		Error:         redactErrorText(e.Error, netMaxError),
		RequestBytes:  reqBytes,
		ResponseBytes: respBytes,
	}
}

// filterNeedle is the filter text as it is searched for: trimmed, lower-cased and cut to netMaxFilter characters. The cut is by
// characters, because a cut by bytes can end in half of one.
func filterNeedle(text string) string {
	t := strings.ToLower(strings.TrimSpace(text))
	if r := []rune(t); len(r) > netMaxFilter {
		t = string(r[:netMaxFilter])
	}
	return t
}

func (f NetFilter) matches(r NetRow) bool {
	if m := strings.TrimSpace(f.Method); m != "" && !strings.EqualFold(m, r.Method) {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(f.Status))
	switch status {
	case "":
	case "2xx", "3xx", "4xx", "5xx":
		if r.Status/100 != int(status[0]-'0') {
			return false
		}
	case "error":
		if r.Error == "" && r.Status != 0 {
			return false
		}
	default:
		return false
	}
	if t := filterNeedle(f.Text); t != "" {
		hay := strings.ToLower(r.Method + " " + r.URL + " " + strconv.Itoa(r.Status) + " " + r.Error)
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

// netLog reads the recorded exchanges from the connected app. It is the only place this file asks the app anything.
func (a *App) netLog(p probelink.HTTPLogParams) (probelink.HTTPLogResult, error) {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil || conn.client == nil {
		return probelink.HTTPLogResult{}, errNetNotConnected
	}
	ctx, cancel := context.WithTimeout(context.Background(), netLogTimeout)
	defer cancel()
	res, err := conn.client.HTTPLog(ctx, p)
	if err != nil {
		return probelink.HTTPLogResult{}, errNetLogFailed
	}
	return res, nil
}

// HTTPExchanges lists the app's recorded exchanges after f.Since that match f, the newest 500 at most, oldest first, with every
// string redacted. It asks the app for the list without bodies, and the filter runs on the redacted text.
func (a *App) HTTPExchanges(f NetFilter) (NetList, error) {
	res, err := a.netLog(probelink.HTTPLogParams{Since: f.Since, NoBodies: true})
	if err != nil {
		return NetList{}, err
	}
	rows := make([]NetRow, 0, len(res.Entries))
	for _, e := range res.Entries {
		if e.Seq <= f.Since {
			continue
		}
		if r := netRow(e); f.matches(r) {
			rows = append(rows, r)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Seq < rows[j].Seq })
	total := len(rows)
	if total > netMaxRows {
		rows = rows[total-netMaxRows:]
	}
	return NetList{Epoch: shortText(res.Epoch, 64), Latest: res.Latest, Total: total, Shown: len(rows), Rows: rows}, nil
}

// HTTPExchange returns one recorded exchange with its headers and bodies, redacted and cut. epoch is the one the listing returned: a
// sequence number means the same exchange only within one run of the app, so when the app has restarted the answer is an error and
// not another exchange that has the same number.
func (a *App) HTTPExchange(epoch string, seq int) (NetDetail, error) {
	if seq < 1 {
		return NetDetail{}, errNetNoExchange
	}
	res, err := a.netLog(probelink.HTTPLogParams{Since: seq - 1})
	if err != nil {
		return NetDetail{}, err
	}
	if shortText(res.Epoch, 64) != epoch {
		return NetDetail{}, errNetEpoch
	}
	for _, e := range res.Entries {
		if e.Seq != seq {
			continue
		}
		d := NetDetail{Epoch: epoch, NetRow: netRow(e)}
		d.RequestHeaders, d.MoreRequestHeaders = redactHeaders(e.RequestHeaders)
		d.ResponseHeaders, d.MoreResponseHeaders = redactHeaders(e.ResponseHeaders)
		d.RequestBody = redactBody(e.RequestBody, e.RequestBytes, headerValue(e.RequestHeaders, "content-type"))
		d.ResponseBody = redactBody(e.ResponseBody, e.ResponseBytes, headerValue(e.ResponseHeaders, "content-type"))
		return d, nil
	}
	return NetDetail{}, errNetNoExchange
}
