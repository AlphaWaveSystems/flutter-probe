package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DoctorInput is what `probe ai doctor` checks. It is read-only and exists
// purely for the user: nothing in a test run calls it.
type DoctorInput struct {
	Provider string
	APIKey   string
	Model    string
	Endpoint string
	Timeout  time.Duration
}

// DoctorCheck is one line of the report.
type DoctorCheck struct {
	Name   string
	OK     bool
	Detail string
}

// DoctorReport is the outcome of Doctor.
type DoctorReport struct {
	Checks []DoctorCheck
	// Vision is true when the model accepted an image. Only meaningful when
	// VisionChecked is true.
	Vision        bool
	VisionChecked bool
	// TextOK is true when a plain text round trip worked.
	TextOK bool
}

// OK reports whether the provider is usable for text features (triage,
// generate). Vision is reported separately: a text-only model is a valid,
// supported setup.
func (r DoctorReport) OK() bool { return r.TextOK }

func (r *DoctorReport) add(name string, ok bool, format string, args ...any) {
	r.Checks = append(r.Checks, DoctorCheck{Name: name, OK: ok, Detail: fmt.Sprintf(format, args...)})
}

// onePixelPNG is a valid 1x1 PNG used to probe whether a model accepts images.
var onePixelPNG, _ = base64.StdEncoding.DecodeString(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")

// Doctor checks the configured provider end to end and never returns an
// error: problems are findings in the report.
func Doctor(ctx context.Context, in DoctorInput) DoctorReport {
	var r DoctorReport
	if in.Provider == "" {
		r.add("config", false, "no ai.provider set in probe.yaml — AI features are off (this is fine; tests do not need them)")
		return r
	}

	c, err := NewTextCompleter(in.Provider, in.APIKey, in.Model, in.Endpoint, in.Timeout)
	if err != nil {
		r.add("config", false, "%v", err)
		return r
	}
	r.add("config", true, "provider %s, model %s", in.Provider, orDefault(in.Model, "(provider default)"))

	if in.Provider == "local" || in.Provider == "openai" {
		base := strings.TrimSuffix(in.Endpoint, "/")
		if in.Provider == "openai" {
			base = "https://api.openai.com/v1"
		}
		ids, lerr := listModels(ctx, base, in.APIKey, in.Timeout)
		switch {
		case lerr != nil:
			r.add("endpoint", false, "could not list models at %s/models: %v", base, lerr)
		case in.Model == "":
			r.add("endpoint", true, "reachable, %d model(s)", len(ids))
		case contains(ids, in.Model):
			r.add("endpoint", true, "reachable, model %q is available", in.Model)
		default:
			r.add("endpoint", false, "reachable, but model %q is not listed (available: %s)", in.Model, strings.Join(firstN(ids, 8), ", "))
		}
	}

	start := time.Now()
	tctx, cancel := context.WithTimeout(ctx, timeoutOrDefault(in.Timeout))
	out, terr := c.Complete(tctx, "Reply with exactly one word.", "Reply with the single word: pong")
	cancel()
	if terr != nil {
		r.add("text", false, "text request failed: %v", terr)
		return r
	}
	r.TextOK = true
	r.add("text", true, "answered in %s: %q", time.Since(start).Round(time.Millisecond), truncate(ReasoningStripped(out), 40))

	vp, verr := NewVisionProvider(in.Provider, in.APIKey, in.Model, in.Endpoint, in.Timeout)
	if verr != nil {
		return r
	}
	vctx, vcancel := context.WithTimeout(ctx, timeoutOrDefault(in.Timeout))
	defer vcancel()
	_, aerr := vp.ExtractText(vctx, onePixelPNG, "any text")
	r.VisionChecked = true
	// A model that accepts the image but finds no text answers with the
	// "not found" error — that is still a working vision path. Only a
	// transport/protocol error means images are not accepted.
	if aerr == nil || strings.Contains(aerr.Error(), "could not find") {
		r.Vision = true
		r.add("vision", true, "the model accepts images — `with ai` screenshot assertions will work")
	} else {
		r.add("vision", false, "the model did not accept an image (%v) — set ai.vision: false to evaluate `with ai` against the screen's visible texts instead", truncate(aerr.Error(), 120))
	}
	return r
}

func listModels(ctx context.Context, base, apiKey string, timeout time.Duration) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeoutOrDefault(timeout))
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("unexpected response: %w", err)
	}
	ids := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func firstN(list []string, n int) []string {
	if len(list) > n {
		return list[:n]
	}
	return list
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
