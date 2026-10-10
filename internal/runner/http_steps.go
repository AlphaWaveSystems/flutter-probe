package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// httpState is what an executor knows about the app's recorded backend traffic.
type httpState struct {
	epoch    string               // the agent process whose sequence numbers `consumed` refers to
	consumed map[int]bool         // exchanges already taken by a `wait for response`
	mocks    []probelink.MockParam // active mocks, re-sent after the app restarts
}

// beginHTTP resets the agent's recording and mocks for a new test. Older agents
// without the method, and runs without a device, are fine: it is best effort.
func (e *Executor) beginHTTP(ctx context.Context) {
	e.http = httpState{}
	if e.client == nil {
		return
	}
	_ = e.client.HTTPClear(ctx, true)
}

// reapplyMocks re-registers the active mocks on a fresh agent process (after a restart).
func (e *Executor) reapplyMocks(ctx context.Context) {
	e.http.consumed = nil
	e.http.epoch = ""
	for _, m := range e.http.mocks {
		_ = e.client.RegisterMock(ctx, m)
	}
}

func (e *Executor) resolveRef(r parser.ResponseRef) parser.ResponseRef {
	r.Pattern = e.resolve(r.Pattern)
	return r
}

func refString(r parser.ResponseRef) string {
	if r.Method != "" {
		return r.Method + " " + strconv.Quote(r.Pattern)
	}
	return strconv.Quote(r.Pattern)
}

func (e *Executor) runHTTPStep(ctx context.Context, s parser.HTTPStep) error {
	if e.client == nil {
		return nil
	}
	ref := e.resolveRef(s.Ref)
	switch s.Kind {
	case parser.HTTPClearRequests:
		e.http.consumed = nil
		return e.client.HTTPClear(ctx, false)

	case parser.HTTPWaitResponse:
		return e.waitResponse(ctx, ref, s.Check)

	case parser.HTTPSeeResponse:
		entry, err := e.lastResponse(ctx, ref)
		if err != nil {
			return err
		}
		if entry == nil {
			return fmt.Errorf("no such response recorded%s", e.recentRequests(ctx))
		}
		check := e.resolveCheck(s.Check)
		if ok, actual := checkResponse(*entry, check); !ok {
			return fmt.Errorf("got %s (%s %s -> %d)%s", actual,
				entry.Method, entry.URL, entry.Status, bodyHint(*entry))
		}
		return nil

	case parser.HTTPStoreResponse:
		entry, err := e.lastResponse(ctx, ref)
		if err != nil {
			return err
		}
		if entry == nil {
			return fmt.Errorf("no such response recorded%s", e.recentRequests(ctx))
		}
		v, ok := jsonLookup(entry.ResponseBody, e.resolve(s.Path))
		if !ok {
			return fmt.Errorf("json %q not found in the body%s", s.Path, bodyHint(*entry))
		}
		e.vars[s.Var] = v
		return nil

	case parser.HTTPSeeRequests:
		res, err := e.client.HTTPLog(ctx, probelink.HTTPLogParams{Method: ref.Method, Pattern: ref.Pattern, CountOnly: true})
		if err != nil {
			return fmt.Errorf("see requests: %w", err)
		}
		if res.Count != s.Count {
			return fmt.Errorf("the app made %d%s", res.Count, e.recentRequests(ctx))
		}
		return nil
	}
	return nil
}

func countWord(n int) string {
	if n == 0 {
		return "no"
	}
	return "exactly " + strconv.Itoa(n)
}

func (e *Executor) resolveCheck(c parser.ResponseCheck) parser.ResponseCheck {
	c.Text = e.resolve(c.Text)
	c.Equals = e.resolve(c.Equals)
	return c
}

// lastResponse returns the newest finished exchange matching ref, or nil. With an
// implicit wait configured, a missing response is retried for that long.
func (e *Executor) lastResponse(ctx context.Context, ref parser.ResponseRef) (*probelink.HTTPEntry, error) {
	deadline := time.Now().Add(e.implicitWait)
	for {
		res, err := e.client.HTTPLog(ctx, probelink.HTTPLogParams{Method: ref.Method, Pattern: ref.Pattern, Last: true})
		if err != nil {
			return nil, fmt.Errorf("reading recorded responses: %w (does the app run flutter_probe_agent >= 0.22 started before its HTTP clients are created?)", err)
		}
		if len(res.Entries) > 0 {
			return &res.Entries[0], nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return nil, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// waitResponse waits for a matching exchange that no earlier `wait for response`
// has taken, optionally with the given status.
func (e *Executor) waitResponse(ctx context.Context, ref parser.ResponseRef, check parser.ResponseCheck) error {
	for {
		res, err := e.client.HTTPLog(ctx, probelink.HTTPLogParams{Method: ref.Method, Pattern: ref.Pattern})
		if err != nil {
			return fmt.Errorf("reading recorded responses: %w (does the app run flutter_probe_agent >= 0.22 started before its HTTP clients are created?)", err)
		}
		if res.Epoch != e.http.epoch {
			e.http.epoch, e.http.consumed = res.Epoch, nil
		}
		for _, entry := range res.Entries {
			if e.http.consumed[entry.Seq] {
				continue
			}
			if check.Kind == parser.CheckStatus && entry.Status != check.Status {
				continue
			}
			if e.http.consumed == nil {
				e.http.consumed = map[int]bool{}
			}
			e.http.consumed[entry.Seq] = true
			return nil
		}
		select {
		case <-ctx.Done():
			want := ""
			if check.Kind == parser.CheckStatus {
				want = fmt.Sprintf(" waiting for status %d", check.Status)
			}
			return fmt.Errorf("timed out%s%s", want, e.recentRequests(context.Background()))
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// evalResponseCond answers `if response ...` once, without waiting.
func (e *Executor) evalResponseCond(ctx context.Context, c parser.ResponseCond) (bool, error) {
	entry, err := e.lastResponse(ctx, e.resolveRef(c.Ref))
	if err != nil || entry == nil {
		return false, err
	}
	ok, _ := checkResponse(*entry, e.resolveCheck(c.Check))
	return ok, nil
}

// recentRequests lists the last requests the app made, to make a miss debuggable.
func (e *Executor) recentRequests(ctx context.Context) string {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	res, err := e.client.HTTPLog(cctx, probelink.HTTPLogParams{NoBodies: true})
	if err != nil {
		return ""
	}
	if len(res.Entries) == 0 {
		return "\n    no requests recorded (traffic through dart:io HttpClient only: not WebViews or native SDKs; the agent must start before the app creates its HTTP clients)"
	}
	entries := res.Entries
	if len(entries) > 8 {
		entries = entries[len(entries)-8:]
	}
	var b strings.Builder
	b.WriteString("\n    recent requests:")
	for _, en := range entries {
		status := strconv.Itoa(en.Status)
		if en.Error != "" {
			status = "error"
		}
		fmt.Fprintf(&b, "\n      %s %s -> %s", en.Method, en.URL, status)
	}
	return b.String()
}

func bodyHint(en probelink.HTTPEntry) string {
	body := strings.TrimSpace(en.ResponseBody)
	if body == "" {
		return ""
	}
	if len(body) > 200 {
		body = body[:200] + "..."
	}
	return "\n    body: " + body
}

func describeCheck(c parser.ResponseCheck) string {
	switch c.Kind {
	case parser.CheckStatus:
		return fmt.Sprintf("status %d", c.Status)
	case parser.CheckContains:
		return fmt.Sprintf("contains %q", c.Text)
	case parser.CheckJSONEquals:
		return fmt.Sprintf("json %q equals %q", c.Text, c.Equals)
	case parser.CheckJSONExists:
		return fmt.Sprintf("json %q exists", c.Text)
	case parser.CheckTime:
		return fmt.Sprintf("below %d ms", c.Status)
	}
	return ""
}

// checkResponse evaluates c against a recorded exchange and describes what was found.
func checkResponse(en probelink.HTTPEntry, c parser.ResponseCheck) (bool, string) {
	switch c.Kind {
	case parser.CheckStatus:
		return en.Status == c.Status, strconv.Itoa(en.Status)
	case parser.CheckTime:
		return en.DurationMs < c.Status, fmt.Sprintf("%d ms", en.DurationMs)
	case parser.CheckContains:
		return strings.Contains(en.ResponseBody, c.Text), "a body without it"
	case parser.CheckJSONExists:
		_, ok := jsonLookup(en.ResponseBody, c.Text)
		return ok, "no such path"
	case parser.CheckJSONEquals:
		v, ok := jsonLookup(en.ResponseBody, c.Text)
		if !ok {
			return false, "no such path"
		}
		return v == c.Equals, strconv.Quote(v)
	}
	return false, ""
}

// jsonLookup resolves a path such as `data.plan`, `items[0].id` or `items.0.id`
// in a JSON document and returns the value as text (numbers and booleans as
// written, null as "null", objects and arrays as compact JSON).
func jsonLookup(doc, path string) (string, bool) {
	dec := json.NewDecoder(strings.NewReader(doc))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", false
	}
	for _, seg := range splitJSONPath(path) {
		switch cur := v.(type) {
		case map[string]any:
			next, ok := cur[seg]
			if !ok {
				return "", false
			}
			v = next
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(cur) {
				return "", false
			}
			v = cur[i]
		default:
			return "", false
		}
	}
	switch t := v.(type) {
	case string:
		return t, true
	case json.Number:
		return t.String(), true
	case bool:
		return strconv.FormatBool(t), true
	case nil:
		return "null", true
	default:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(t); err != nil {
			return "", false
		}
		return strings.TrimSpace(buf.String()), true
	}
}

func splitJSONPath(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" || path == "$" {
		return nil
	}
	path = strings.TrimPrefix(path, "$.")
	var segs []string
	for _, part := range strings.Split(path, ".") {
		for part != "" {
			i := strings.Index(part, "[")
			if i < 0 {
				segs = append(segs, part)
				break
			}
			if i > 0 {
				segs = append(segs, part[:i])
			}
			j := strings.Index(part[i:], "]")
			if j < 0 {
				segs = append(segs, part[i+1:])
				break
			}
			segs = append(segs, part[i+1:i+j])
			part = part[i+j+1:]
		}
	}
	return segs
}
