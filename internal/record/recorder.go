package record

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// Event describes a step boundary without depending on the runner.
type Event struct {
	File, TestName string
	Line           int
	Description    string
	Status         string
	Duration       time.Duration
	Error          string
	Attempt, Depth int
	At             time.Time
	// Target is the element the step acts on; nil for a step that acts on no element.
	Target *Target
}

// Target is the element a step acts on. Text is the selector as the step resolved it, and it is redacted before it is
// written. Lookup is what the recorder sends to find the element's bounds.
type Target struct {
	Verb   string
	Text   string
	Lookup probelink.SelectorParam
}

// Client is the subset of ProbeClient needed for recording.
type Client interface {
	HTTPLog(context.Context, probelink.HTTPLogParams) (probelink.HTTPLogResult, error)
	DumpWidgetTree(context.Context) (string, error)
	Screenshot(context.Context, string) (string, error)
	SelectorBounds(context.Context, probelink.SelectorParam) (probelink.BoundsResult, error)
}

var _ Client = (probelink.ProbeClient)(nil)

type Options struct{ Dir, RunID, Device, Screenshots, CLIVersion, Platform, OSVersion string }

type recordingKey struct {
	file, test string
	attempt    int
}

// Recorder serializes recording operations for one device.
type Recorder struct {
	mu       sync.Mutex
	opts     Options
	cursor   *Cursor
	bundle   *Bundle
	key      recordingKey
	manifest Manifest
	timeline Timeline
	open     map[int]bool
	warnings []string
	// exchanges numbers this bundle's exchange files. The agent's seq starts again after an app restart, so
	// seq cannot name the files: a reused name would overwrite an earlier exchange.
	exchanges int
}

func NewRecorder(opts Options) *Recorder { return &Recorder{opts: opts, cursor: NewCursor()} }

func eventKey(ev Event) recordingKey { return recordingKey{ev.File, ev.TestName, ev.Attempt} }

func recordingSlug(s string) string {
	var b strings.Builder
	dash := false
	for _, c := range strings.ToLower(s) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(c)
			dash = false
		} else {
			dash = true
		}
	}
	slug := b.String()
	if len(slug) > 60 {
		slug = slug[:60]
	}
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "test"
	}
	return slug
}

func (r *Recorder) warn(operation string, err error) error {
	if err != nil {
		r.warnings = append(r.warnings, operation+": "+scrubText(err.Error()))
	}
	return err
}

// scrubText is what a recording writes for text the agent or the app produced: credential shapes are redacted, then
// resolved environment values are masked with the same sysdialog.ScrubKnown the run uses for printed lines.
func scrubText(s string) string { return sysdialog.ScrubKnown(RedactBody(s)) }

// scrubHeaders is RedactHeaders with resolved environment values masked in the header values.
func scrubHeaders(h map[string]string) map[string]string {
	out := RedactHeaders(h)
	for k, v := range out {
		out[k] = sysdialog.ScrubKnown(v)
	}
	return out
}

// deviceCall contains failures (including faulty client panics) within recording.
func deviceCall[T any](f func(context.Context) (T, error)) (value T, err error) {
	return deviceCallWithin(15*time.Second, f)
}

// deviceCallWithin is deviceCall with its own time limit.
func deviceCallWithin[T any](limit time.Duration, f func(context.Context) (T, error)) (value T, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("client panic: %v", p)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	return f(ctx)
}

// interaction records the element a step acts on: its selector, and its bounds and tap point when the element is on
// screen as the step starts. The lookup has a two-second limit. A failed lookup is an advisory warning, and the step
// runs as it would without recording.
func (r *Recorder) interaction(client Client, t *Target) *Interaction {
	in := &Interaction{Kind: t.Verb, Selector: scrubText(t.Text)}
	if client == nil {
		return in
	}
	bounds, err := deviceCallWithin(2*time.Second, func(ctx context.Context) (probelink.BoundsResult, error) {
		return client.SelectorBounds(ctx, t.Lookup)
	})
	if err != nil {
		r.warn("element lookup", err)
		return in
	}
	in.Target = &Rect{Left: bounds.X, Top: bounds.Y, Width: bounds.Width, Height: bounds.Height}
	in.TapX = bounds.X + bounds.Width/2
	in.TapY = bounds.Y + bounds.Height/2
	return in
}

func (r *Recorder) BeginStep(client Client, ev Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result error
	if r.bundle == nil || r.key != eventKey(ev) {
		result = r.finish()
		if err := r.start(ev); err != nil {
			result = errors.Join(result, err)
			if r.bundle == nil {
				return result
			}
		}
	}
	step := Step{Index: len(r.timeline.Steps) + 1, Depth: ev.Depth, File: ev.File, Line: ev.Line, Description: ev.Description, Status: "started", StartedAt: ev.At}
	if r.opts.Screenshots == "every_step" {
		name, err := r.screenshot(client, step.Index, "before")
		step.ScreenshotBefore = name
		result = errors.Join(result, err)
	}
	if ev.Target != nil {
		step.Interaction = r.interaction(client, ev.Target)
	}
	r.timeline.Steps = append(r.timeline.Steps, step)
	r.open[step.Index-1] = true
	return errors.Join(result, r.warn("write timeline", r.bundle.WriteTimeline(r.timeline)))
}

func (r *Recorder) start(ev Event) error {
	if err := os.MkdirAll(r.opts.Dir, 0700); err != nil {
		return r.warn("create recording root", err)
	}
	base := fmt.Sprintf("%s-attempt-%d", recordingSlug(ev.TestName), ev.Attempt)
	dir := filepath.Join(r.opts.Dir, base)
	for suffix := 2; ; suffix++ {
		err := os.Mkdir(dir, 0700)
		if err == nil {
			break
		}
		if !os.IsExist(err) {
			return r.warn("create recording", err)
		}
		dir = filepath.Join(r.opts.Dir, fmt.Sprintf("%s-%d", base, suffix))
	}
	b, err := Create(dir)
	if err != nil {
		return r.warn("create bundle", err)
	}
	r.bundle, r.key = b, eventKey(ev)
	r.timeline = Timeline{Steps: []Step{}}
	r.open = make(map[int]bool)
	r.exchanges = 0
	r.manifest = Manifest{SchemaVersion: SchemaVersion, RunID: r.opts.RunID, TestName: ev.TestName, File: ev.File, Attempt: ev.Attempt, DeviceName: r.opts.Device, Platform: r.opts.Platform, OSVersion: r.opts.OSVersion, CLIVersion: r.opts.CLIVersion, StartedAt: ev.At, Status: "running", Capture: Capture{Screenshots: r.opts.Screenshots, Native: "off", Redacted: redactedKinds(), NotRedacted: notRedactedKinds()}}
	data, err := os.ReadFile(ev.File)
	if err != nil {
		r.warn("read test file", err)
	} else {
		r.manifest.FileSHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	return r.warn("write manifest", b.WriteManifest(r.manifest))
}

func (r *Recorder) screenshot(client Client, index int, phase string) (string, error) {
	name := fmt.Sprintf("%04d-%s.png", index, phase)
	data, err := deviceCall(func(ctx context.Context) ([]byte, error) {
		if provider, ok := client.(probelink.ScreenshotBytesProvider); ok {
			return provider.ScreenshotBytes(ctx, name)
		}
		path, err := client.Screenshot(ctx, name)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(path)
	})
	if err != nil {
		r.warn("capture screenshot", err)
		return "", nil
	}
	if err := r.warn("write screenshot", r.bundle.WriteScreenshot(name, data)); err != nil {
		return "", err
	}
	return name, nil
}

func (r *Recorder) EndStep(client Client, ev Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	index := -1
	if r.bundle != nil && r.key == eventKey(ev) {
		for i := len(r.timeline.Steps) - 1; i >= 0; i-- {
			s := r.timeline.Steps[i]
			if r.open[i] && s.Depth == ev.Depth && s.Line == ev.Line && s.Description == ev.Description {
				index = i
				break
			}
		}
	}
	if index < 0 {
		r.warn("end step", errors.New("no matching open step"))
		return nil
	}
	step := &r.timeline.Steps[index]
	step.Status, step.Error, step.DurationMs = ev.Status, scrubText(ev.Error), ev.Duration.Milliseconds()
	delete(r.open, index)
	var result error
	read, err := deviceCall(func(ctx context.Context) (Read, error) { return r.cursor.Next(ctx, client) })
	if err != nil {
		r.warn("read HTTP log", err)
	} else {
		r.manifest.Truncated.Exchanges += read.Lost
		step.AppRestarted = read.Restarted
		for _, e := range read.Exchanges {
			r.exchanges++
			id := fmt.Sprintf("%04d", r.exchanges)
			exchange := Exchange{ID: id, Seq: e.Seq, Method: e.Method, URL: sysdialog.ScrubKnown(RedactURL(e.URL)), Status: e.Status, DurationMs: e.DurationMs, Mocked: e.Mocked, Error: scrubText(e.Error), RequestHeaders: scrubHeaders(e.RequestHeaders), ResponseHeaders: scrubHeaders(e.ResponseHeaders), RequestBody: scrubText(e.RequestBody), ResponseBody: scrubText(e.ResponseBody), Source: "dart_io"}
			err := r.warn("write exchange", r.bundle.WriteExchange(exchange))
			result = errors.Join(result, err)
			if err == nil {
				step.Exchanges = append(step.Exchanges, id)
			}
		}
	}
	if r.opts.Screenshots == "every_step" || r.opts.Screenshots == "on_failure" && ev.Status == "failed" {
		step.ScreenshotAfter, err = r.screenshot(client, step.Index, "after")
		result = errors.Join(result, err)
		tree, err := deviceCall(func(ctx context.Context) (string, error) { return client.DumpWidgetTree(ctx) })
		if err != nil {
			r.warn("capture tree", err)
		} else {
			id := fmt.Sprintf("%04d", step.Index)
			err := r.warn("write tree", r.bundle.WriteTree(id, sysdialog.ScrubKnown(tree)))
			result = errors.Join(result, err)
			if err == nil {
				step.Tree = id
			}
		}
	}
	return errors.Join(result, r.warn("write timeline", r.bundle.WriteTimeline(r.timeline)))
}

func (r *Recorder) finish() error {
	if r.bundle == nil {
		return nil
	}
	r.manifest.EndedAt = time.Now()
	r.manifest.Status = "skipped"
	for _, step := range r.timeline.Steps {
		if step.Status == "failed" {
			r.manifest.Status = "failed"
			break
		}
		if step.Status == "passed" {
			r.manifest.Status = "passed"
		}
	}
	err := errors.Join(r.warn("write manifest", r.bundle.WriteManifest(r.manifest)), r.warn("write timeline", r.bundle.WriteTimeline(r.timeline)), r.warn("write viewer", r.bundle.WriteViewer(r.manifest, r.timeline)))
	r.bundle = nil
	return err
}

func (r *Recorder) Close() error { r.mu.Lock(); defer r.mu.Unlock(); return r.finish() }
func (r *Recorder) Warnings() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.warnings...)
}
