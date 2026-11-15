package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
)

// The lookups inside a WebView page are fixed pieces of JavaScript that live here and nowhere else. A selector or a text from a script is NEVER part of the JavaScript: it travels as a JSON value in the
// `arguments` of Runtime.callFunctionOn, so the page's JavaScript engine never parses it as code, whatever it contains. The functions only read the page (and scroll the found element into view, focus it when
// asked); they return small objects, never HTML.

// findFunction finds the elements that match query and describes the first visible one. kind says what query is: "css" (a CSS selector) or anything else (the visible text of an element: an exact match first,
// then a text that contains the query, innermost element first). The caller chooses the kind; the function never guesses it from the characters of the query. mode "focus" also focuses the element and reports
// whether it holds the focus afterwards. The text it returns is capped at maxText characters; vw and vh are the viewport, so the caller can check the position it gets back.
const findFunction = `function (kind, query, maxText, mode) {
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
  var list;
  if (kind === 'css') {
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
  var focused = false;
  if (mode === 'focus') {
    if (typeof found.focus === 'function') { found.focus(); }
    focused = document.activeElement === found;
  }
  var tag = String(found.tagName || '').toLowerCase();
  var disabled = false;
  try { disabled = found.matches(':disabled'); } catch (err) { disabled = false; }
  var r = found.getBoundingClientRect();
  return { count: list.length, visible: true, focused: focused,
    tag: tag.slice(0, 20), inputType: tag === 'input' ? String(found.type || '').toLowerCase().slice(0, 20) : '',
    readOnly: found.readOnly === true, disabled: disabled, editable: found.isContentEditable === true, x: r.left + r.width / 2, y: r.top + r.height / 2, w: r.width, h: r.height, vw: window.innerWidth, vh: window.innerHeight, text: label(found).slice(0, maxText) };
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
	// maxMatchCount and maxPixels bound the numbers a page may report: a count of matches, and a size or position in CSS pixels.
	maxMatchCount = 100_000
	maxPixels     = 1_000_000
)

// SelectorKind says how a Selector is read.
type SelectorKind string

const (
	// SelectText matches the visible text of an element: an exact match first, then a text that contains the value, innermost element first.
	SelectText SelectorKind = "text"
	// SelectCSS is a CSS selector for document.querySelectorAll.
	SelectCSS SelectorKind = "css"
)

// Selector says what to find in the page. The caller chooses the kind; it is never guessed from the characters of the value, so a text such as "#1 Item" or ".5 off" is text and a selector is a selector.
// The runner decides the kind from the syntax of the step (for example a quoted text against css:"...").
type Selector struct {
	Kind  SelectorKind
	Value string
}

// Text is a Selector for the visible text of an element.
func Text(v string) Selector { return Selector{Kind: SelectText, Value: v} }

// CSS is a Selector for a CSS selector.
func CSS(v string) Selector { return Selector{Kind: SelectCSS, Value: v} }

// ElementInfo describes the first visible element a query matched. Count is how many elements matched in all.
type ElementInfo struct {
	Count   int
	Visible bool
	Focused bool // only set by a focusing lookup: the element holds the focus afterwards
	// What kind of element it is, as the page reports it (checked before use): the lower-case tag name, the type of an input, and whether it is read-only,
	// disabled (itself or through a disabled fieldset) or content-editable.
	Tag, InputType               string
	ReadOnly, Disabled, Editable bool
	X, Y                         float64 // the centre, in CSS pixels of the page's viewport
	W, H                         float64
	Text                         string // at most maxElementText printable characters
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

func checkQuery(sel Selector) error {
	q := sel.Value
	if sel.Kind != SelectText && sel.Kind != SelectCSS {
		return &WebViewError{Kind: "invalid selector", Message: "the selector kind must be text or css"}
	}
	if q == "" {
		return &WebViewError{Kind: "invalid selector", Message: "the selector is empty"}
	}
	if len([]rune(q)) > maxQueryChars {
		return &WebViewError{Kind: "too long", Message: fmt.Sprintf("the selector or text is longer than %d characters", maxQueryChars)}
	}
	return nil
}

// callFindFunction runs findFunction on the page with the query as a JSON argument.
func (s *WebViewSession) callFindFunction(ctx context.Context, sel Selector, mode string) (ElementInfo, error) {
	query := sel.Value
	if err := checkQuery(sel); err != nil {
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
		"arguments":           []map[string]any{{"value": string(sel.Kind)}, {"value": query}, {"value": maxElementText}, {"value": mode}},
		"returnByValue":       true,
		"awaitPromise":        false,
	})
	if err != nil {
		return ElementInfo{}, s.callError(ctx, err)
	}
	var out struct {
		Result struct {
			Value struct {
				Invalid  bool    `json:"invalid"`
				Count    int     `json:"count"`
				Visible  bool    `json:"visible"`
				Focused  bool    `json:"focused"`
				X        float64 `json:"x"`
				Y        float64 `json:"y"`
				W        float64 `json:"w"`
				H        float64 `json:"h"`
				VW       float64 `json:"vw"`
				VH       float64 `json:"vh"`
				Text     string  `json:"text"`
				Tag      string  `json:"tag"`
				Type     string  `json:"inputType"`
				ReadOnly bool    `json:"readOnly"`
				Disabled bool    `json:"disabled"`
				Editable bool    `json:"editable"`
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
	// the numbers come from the page: they are checked before anything is done with them
	if v.Count < 0 || v.Count > maxMatchCount {
		return ElementInfo{}, &WebViewError{Kind: "page error", Message: "the WebView page reported an impossible number of matches"}
	}
	if v.Visible && !sane(v.X, v.Y, v.W, v.H, v.VW, v.VH) {
		return ElementInfo{}, &WebViewError{Kind: "page error", Message: "the WebView page reported a position outside its viewport"}
	}
	return ElementInfo{Count: v.Count, Visible: v.Visible, Focused: v.Focused, X: v.X, Y: v.Y, W: v.W, H: v.H, Text: shown(s.redact(v.Text)),
		Tag: kindWord(v.Tag), InputType: kindWord(v.Type), ReadOnly: v.ReadOnly, Disabled: v.Disabled, Editable: v.Editable}, nil
}

// sane reports whether a position and size reported by the page are finite, within bounds, and inside the viewport the page reported.
func sane(x, y, w, h, vw, vh float64) bool {
	for _, n := range []float64{x, y, w, h, vw, vh} {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return false
		}
	}
	return vw > 0 && vh > 0 && vw <= maxPixels && vh <= maxPixels && w >= 0 && h >= 0 && w <= maxPixels && h <= maxPixels && x >= 0 && y >= 0 && x <= vw && y <= vh
}

// callError turns a failed Call into the error a step reports: the context's own error, or a lost connection.
func (s *WebViewSession) callError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, ErrMalformedReply) {
		return &WebViewError{Kind: "page error", Message: ErrMalformedReply.Error()}
	}
	var ce *CDPError
	if errors.As(err, &ce) {
		return &WebViewError{Kind: "page error", Message: "the WebView page refused the command: " + shown(s.redact(ce.Message))}
	}
	return &WebViewError{Kind: "connection lost", Message: "the WebView page changed or closed while the step ran"}
}

// Find reports the elements that match query. A query with no match is not an error: Count is 0.
func (s *WebViewSession) Find(ctx context.Context, sel Selector) (ElementInfo, error) {
	return s.callFindFunction(ctx, sel, "find")
}

// locate finds the first visible element or says why there is none, in the words of the proposal's failure table.
func (s *WebViewSession) locate(ctx context.Context, sel Selector, mode string) (ElementInfo, error) {
	query := sel.Value
	info, err := s.callFindFunction(ctx, sel, mode)
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

// See succeeds when a visible element matches sel. When none does it says why, in the words of the proposal's failure table ("not found" or "not visible" kinds).
func (s *WebViewSession) See(ctx context.Context, sel Selector) error {
	_, err := s.locate(ctx, sel, "find")
	return err
}

// kindWord accepts a tag name or an input type from the page only in the shape those have (lower case letters, digits and hyphens, at most 20); anything else is "?" so it can never carry page text into a message.
func kindWord(s string) string {
	if len(s) > 20 {
		return "?"
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return "?"
		}
	}
	return s
}

// textInputTypes are the input types Input.insertText can fill with text.
var textInputTypes = map[string]bool{"": true, "text": true, "search": true, "email": true, "url": true, "tel": true, "password": true, "number": true}

// notEditable is the refusal of a step that would type into an element that takes no text. Input.insertText into such an element does nothing and reports no
// error, so the step refuses instead of passing with nothing typed.
func (s *WebViewSession) notEditable(sel Selector, why string) error {
	return &WebViewError{Kind: "not editable", Message: fmt.Sprintf("The element %q in the WebView page %q cannot be typed into (%s), so nothing was typed", shown(s.redact(sel.Value)), s.title, why)}
}

// formControls are elements that take no typed text whatever their surroundings: inside a content-editable host they report isContentEditable too, but typing
// into them types nothing.
var formControls = map[string]bool{"select": true, "button": true, "option": true, "optgroup": true, "meter": true, "progress": true, "output": true, "fieldset": true}

// typeRefusal says why text cannot be typed into the element, or "" when it can: a disabled or read-only field, an input of a type that takes no typed text (a checkbox, a date, a file picker...),
// or an element that is not a text field at all. A content-editable element and a textarea take text.
func typeRefusal(info ElementInfo) string {
	switch {
	case info.Disabled:
		return "disabled"
	case info.ReadOnly:
		return "read-only"
	case info.Tag == "input":
		// before the content-editable test: a checkbox inside an editing host is still a checkbox
		if textInputTypes[info.InputType] {
			return ""
		}
		return fmt.Sprintf("an input of type %q", info.InputType)
	case info.Tag == "textarea":
		return ""
	case formControls[info.Tag]:
		// a select or a button inside an editing host is still not a text field
	case info.Editable:
		return ""
	}
	return fmt.Sprintf("not a text field: %s", info.Tag)
}

// Tap taps the first visible element that matches query, at its centre, with a mouse press and release.
func (s *WebViewSession) Tap(ctx context.Context, sel Selector) error {
	info, err := s.locate(ctx, sel, "find")
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

// Type focuses the first visible element that matches sel, checks that it holds the focus, and inserts text. If the element did not take the focus nothing is typed. The text is never written to a log or an error.
func (s *WebViewSession) Type(ctx context.Context, sel Selector, text string) error {
	if len([]rune(text)) > maxTypedChars {
		return &WebViewError{Kind: "too long", Message: fmt.Sprintf("the text to type is longer than %d characters", maxTypedChars)}
	}
	// A first lookup, which does not focus anything, decides whether the element can take text at all: a refused element is never focused.
	// (It is still scrolled into view, like any found element.) The second lookup focuses it and checks that it holds the focus.
	info, err := s.locate(ctx, sel, "find")
	if err != nil {
		return err
	}
	if why := typeRefusal(info); why != "" {
		return s.notEditable(sel, why)
	}
	info, err = s.locate(ctx, sel, "focus")
	if err != nil {
		return err
	}
	if why := typeRefusal(info); why != "" { // the page changed between the two lookups
		return s.notEditable(sel, why)
	}
	if !info.Focused {
		// typing into whatever else has the focus would put the text in the wrong place
		return &WebViewError{Kind: "not focusable", Message: fmt.Sprintf("The element %q in the WebView page %q cannot take the focus, so nothing was typed", shown(s.redact(sel.Value)), s.title)}
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

// WaitFor waits until a visible element matches sel, polling every 250 ms. The total time is limited to timeout (and to the caller's own deadline, whichever is shorter) over the whole loop, including a poll
// that hangs: a page that stops answering ends the wait at the limit.
func (s *WebViewSession) WaitFor(ctx context.Context, sel Selector, timeout time.Duration) error {
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	timedOut := func() error {
		return &WebViewError{Kind: "not found", Message: fmt.Sprintf("Timed out waiting for %q in the WebView page %q", shown(s.redact(sel.Value)), s.title)}
	}
	for {
		info, err := s.Find(wctx, sel)
		if err != nil {
			if ctx.Err() == nil && wctx.Err() != nil {
				return timedOut() // our own limit ended the poll
			}
			return err
		}
		if info.Count > 0 && info.Visible {
			return nil
		}
		select {
		case <-wctx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return timedOut()
		case <-time.After(250 * time.Millisecond):
		}
	}
}
