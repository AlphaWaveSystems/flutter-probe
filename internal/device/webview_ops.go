package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// The lookups inside a WebView page are fixed pieces of JavaScript that live here and nowhere else. A selector or a text from a script is NEVER part of the JavaScript: it travels as a JSON value in the
// `arguments` of Runtime.callFunctionOn, so the page's JavaScript engine never parses it as code, whatever it contains. The functions only read the page (and scroll the found element into view, focus it when
// asked); they return small objects, never HTML.

// findFunction finds the elements that match query and describes the first visible one. A query that starts with "#", "." or "[" is a CSS selector; any other query matches the visible text of an element
// (an exact match first, then a text that contains the query), innermost element first. mode "focus" also focuses the element. The text it returns is capped at maxText characters.
const findFunction = `function (query, maxText, mode) {
  function label(e) { return String(e.innerText || e.value || '').replace(/\s+/g, ' ').trim(); }
  function innermost(e) {
    var mine = label(e);
    return !Array.prototype.some.call(e.children, function (c) { return label(c) === mine; });
  }
  function visible(e) {
    var r = e.getBoundingClientRect();
    if (r.width <= 0 || r.height <= 0) { return false; }
    var s = window.getComputedStyle(e);
    return s.visibility !== 'hidden' && s.display !== 'none';
  }
  var first = query.charAt(0);
  var list;
  if (first === '#' || first === '.' || first === '[') {
    try { list = Array.prototype.slice.call(document.querySelectorAll(query)); }
    catch (err) { return { invalid: true }; }
  } else {
    var all = Array.prototype.slice.call(document.querySelectorAll('body *'));
    list = all.filter(function (e) { return label(e) === query && innermost(e); });
    if (list.length === 0) {
      list = all.filter(function (e) { var t = label(e); return t !== '' && t.indexOf(query) >= 0 && innermost(e); });
    }
  }
  var found = null;
  for (var i = 0; i < list.length; i++) { if (visible(list[i])) { found = list[i]; break; } }
  if (found === null) { return { count: list.length, visible: false }; }
  found.scrollIntoView({ block: 'center', inline: 'center' });
  if (mode === 'focus' && typeof found.focus === 'function') { found.focus(); }
  var r = found.getBoundingClientRect();
  return { count: list.length, visible: true, x: r.left + r.width / 2, y: r.top + r.height / 2, w: r.width, h: r.height, text: label(found).slice(0, maxText) };
}`

const (
	// maxQueryChars is the longest selector or text accepted; a longer one is refused before it is sent.
	maxQueryChars = 2048
	// maxElementText is the longest element text read back.
	maxElementText = 80
	// maxTypedChars is the longest text typed in one step.
	maxTypedChars = 8192
	// maxShownChars is the longest piece of page or script text repeated in an error message.
	maxShownChars = 60
)

// ElementInfo describes the first visible element a query matched. Count is how many elements matched in all.
type ElementInfo struct {
	Count   int
	Visible bool
	X, Y    float64 // the centre, in CSS pixels of the page's viewport
	W, H    float64
	Text    string // at most maxElementText printable characters
}

// WebViewError is a failure with a stable kind, for the runner to turn into a step result.
type WebViewError struct {
	Kind    string // "not found", "not visible", "invalid selector", "too long", "connection lost", "page error"
	Message string
}

func (e *WebViewError) Error() string { return e.Message }

// shown returns s made safe to repeat in a message: printable characters only, at most maxShownChars of them.
func shown(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if !unicode.IsPrint(r) {
			r = ' '
		}
		if n == maxShownChars {
			b.WriteString("…")
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

func checkQuery(q string) error {
	if q == "" {
		return &WebViewError{Kind: "invalid selector", Message: "the selector is empty"}
	}
	if len([]rune(q)) > maxQueryChars {
		return &WebViewError{Kind: "too long", Message: fmt.Sprintf("the selector or text is longer than %d characters", maxQueryChars)}
	}
	return nil
}

// callFindFunction runs findFunction on the page with the query as a JSON argument.
func (s *WebViewSession) callFindFunction(ctx context.Context, query, mode string) (ElementInfo, error) {
	if err := checkQuery(query); err != nil {
		return ElementInfo{}, err
	}
	win, err := s.conn.Call(ctx, "Runtime.evaluate", map[string]any{"expression": "window", "returnByValue": false})
	if err != nil {
		return ElementInfo{}, s.callError(ctx, err)
	}
	var w struct {
		Result struct {
			ObjectID string `json:"objectId"`
		} `json:"result"`
	}
	if json.Unmarshal(win, &w) != nil || w.Result.ObjectID == "" {
		return ElementInfo{}, &WebViewError{Kind: "page error", Message: "the WebView page did not give a window object"}
	}
	res, err := s.conn.Call(ctx, "Runtime.callFunctionOn", map[string]any{
		"objectId":            w.Result.ObjectID,
		"functionDeclaration": findFunction,
		"arguments":           []map[string]any{{"value": query}, {"value": maxElementText}, {"value": mode}},
		"returnByValue":       true,
		"awaitPromise":        false,
	})
	if err != nil {
		return ElementInfo{}, s.callError(ctx, err)
	}
	var out struct {
		Result struct {
			Value struct {
				Invalid bool    `json:"invalid"`
				Count   int     `json:"count"`
				Visible bool    `json:"visible"`
				X       float64 `json:"x"`
				Y       float64 `json:"y"`
				W       float64 `json:"w"`
				H       float64 `json:"h"`
				Text    string  `json:"text"`
			} `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if json.Unmarshal(res, &out) != nil || len(out.ExceptionDetails) > 0 {
		// the exception text can quote the page; it is not repeated
		return ElementInfo{}, &WebViewError{Kind: "page error", Message: "the WebView page could not run the lookup"}
	}
	v := out.Result.Value
	if v.Invalid {
		return ElementInfo{}, &WebViewError{Kind: "invalid selector", Message: fmt.Sprintf("%q is not a valid selector for the WebView page", shown(s.redact(query)))}
	}
	return ElementInfo{Count: v.Count, Visible: v.Visible, X: v.X, Y: v.Y, W: v.W, H: v.H, Text: shown(s.redact(v.Text))}, nil
}

// callError turns a failed Call into the error a step reports: the context's own error, or a lost connection.
func (s *WebViewSession) callError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var ce *CDPError
	if errors.As(err, &ce) {
		return &WebViewError{Kind: "page error", Message: "the WebView page refused the command: " + shown(s.redact(ce.Message))}
	}
	return &WebViewError{Kind: "connection lost", Message: "the WebView page changed or closed while the step ran"}
}

// Find reports the elements that match query. A query with no match is not an error: Count is 0.
func (s *WebViewSession) Find(ctx context.Context, query string) (ElementInfo, error) {
	return s.callFindFunction(ctx, query, "find")
}

// locate finds the first visible element or says why there is none, in the words of the proposal's failure table.
func (s *WebViewSession) locate(ctx context.Context, query, mode string) (ElementInfo, error) {
	info, err := s.callFindFunction(ctx, query, mode)
	if err != nil {
		return ElementInfo{}, err
	}
	switch {
	case info.Count == 0:
		return ElementInfo{}, &WebViewError{Kind: "not found", Message: fmt.Sprintf("No element in the WebView page %q matches %q", s.title, shown(s.redact(query)))}
	case !info.Visible:
		return ElementInfo{}, &WebViewError{Kind: "not visible", Message: fmt.Sprintf("The element %q exists in the WebView page %q but is not visible", shown(s.redact(query)), s.title)}
	}
	return info, nil
}

// Tap taps the first visible element that matches query, at its centre, with a mouse press and release.
func (s *WebViewSession) Tap(ctx context.Context, query string) error {
	info, err := s.locate(ctx, query, "find")
	if err != nil {
		return err
	}
	for _, typ := range []string{"mousePressed", "mouseReleased"} {
		if _, err := s.conn.Call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": typ, "x": info.X, "y": info.Y, "button": "left", "clickCount": 1}); err != nil {
			return s.callError(ctx, err)
		}
	}
	return nil
}

// Type focuses the first visible element that matches query and inserts text. The text is never written to a log or an error.
func (s *WebViewSession) Type(ctx context.Context, query, text string) error {
	if len([]rune(text)) > maxTypedChars {
		return &WebViewError{Kind: "too long", Message: fmt.Sprintf("the text to type is longer than %d characters", maxTypedChars)}
	}
	if _, err := s.locate(ctx, query, "focus"); err != nil {
		return err
	}
	if _, err := s.conn.Call(ctx, "Input.insertText", map[string]any{"text": text}); err != nil {
		return s.callError(ctx, redactText(err, text))
	}
	return nil
}

// redactText returns err with the typed text removed from the page's message. The kind of the error is kept: a CDPError stays a CDPError, so the caller still reports a page error and not a lost connection.
func redactText(err error, text string) error {
	if text == "" || len(text) < 4 { // a shorter text would mask every occurrence of its characters in the message
		return err
	}
	var ce *CDPError
	if errors.As(err, &ce) {
		return &CDPError{Code: ce.Code, Message: strings.ReplaceAll(ce.Message, text, "<typed text>")}
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), text, "<typed text>"))
}

// WaitFor waits until a visible element matches query, polling every 250 ms, and fails when timeout passes.
func (s *WebViewSession) WaitFor(ctx context.Context, query string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		info, err := s.Find(ctx, query)
		if err != nil {
			return err
		}
		if info.Count > 0 && info.Visible {
			return nil
		}
		if time.Now().After(deadline) {
			return &WebViewError{Kind: "not found", Message: fmt.Sprintf("Timed out waiting for %q in the WebView page %q", shown(s.redact(query)), s.title)}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}
