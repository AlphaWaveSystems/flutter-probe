package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// fakeAIClient is a minimal probelink.ProbeClient for "with ai" executor
// tests — every method beyond Screenshot/SelectorBounds is unused by
// runAssertWithAI and just returns zero values.
type fakeAIClient struct {
	screenshotPath string
	screenshotErr  error
	bounds         probelink.BoundsResult
	boundsErr      error
	boundsCalls    []probelink.SelectorParam
}

func (f *fakeAIClient) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return nil, nil
}
func (f *fakeAIClient) Ping(ctx context.Context) error { return nil }
func (f *fakeAIClient) Handshake(ctx context.Context, clientVersion string) (*probelink.HandshakeResult, error) {
	return &probelink.HandshakeResult{}, nil
}
func (f *fakeAIClient) Close() error                                                    { return nil }
func (f *fakeAIClient) Connected() bool                                                 { return true }
func (f *fakeAIClient) WaitSettled(ctx context.Context, timeout time.Duration) error     { return nil }
func (f *fakeAIClient) Open(ctx context.Context, screen string) error                   { return nil }
func (f *fakeAIClient) Tap(ctx context.Context, sel probelink.SelectorParam) error       { return nil }
func (f *fakeAIClient) TypeText(ctx context.Context, sel probelink.SelectorParam, text string) error {
	return nil
}
func (f *fakeAIClient) See(ctx context.Context, params probelink.SeeParams) error { return nil }
func (f *fakeAIClient) Wait(ctx context.Context, params probelink.WaitParams) error { return nil }
func (f *fakeAIClient) Swipe(ctx context.Context, direction string, sel *probelink.SelectorParam) error {
	return nil
}
func (f *fakeAIClient) Scroll(ctx context.Context, direction string, sel *probelink.SelectorParam) error {
	return nil
}
func (f *fakeAIClient) LongPress(ctx context.Context, sel probelink.SelectorParam) error { return nil }
func (f *fakeAIClient) DoubleTap(ctx context.Context, sel probelink.SelectorParam) error { return nil }
func (f *fakeAIClient) Clear(ctx context.Context, sel probelink.SelectorParam) error     { return nil }

func (f *fakeAIClient) Screenshot(ctx context.Context, name string) (string, error) {
	if f.screenshotErr != nil {
		return "", f.screenshotErr
	}
	return f.screenshotPath, nil
}
func (f *fakeAIClient) DumpWidgetTree(ctx context.Context) (string, error) { return "", nil }

func (f *fakeAIClient) SelectorBounds(ctx context.Context, sel probelink.SelectorParam) (probelink.BoundsResult, error) {
	f.boundsCalls = append(f.boundsCalls, sel)
	if f.boundsErr != nil {
		return probelink.BoundsResult{}, f.boundsErr
	}
	return f.bounds, nil
}

func (f *fakeAIClient) RunDart(ctx context.Context, code string) error                { return nil }
func (f *fakeAIClient) RegisterMock(ctx context.Context, m probelink.MockParam) error  { return nil }
func (f *fakeAIClient) DeviceAction(ctx context.Context, action, value string) error   { return nil }
func (f *fakeAIClient) SaveLogs(ctx context.Context) error                            { return nil }
func (f *fakeAIClient) CopyToClipboard(ctx context.Context, text string) error        { return nil }
func (f *fakeAIClient) PasteFromClipboard(ctx context.Context) (string, error)        { return "", nil }
func (f *fakeAIClient) VerifyBrowser(ctx context.Context) error                       { return nil }
func (f *fakeAIClient) SetNextToken(ctx context.Context, token string) error          { return nil }
func (f *fakeAIClient) OpenLink(ctx context.Context, url string) error                { return nil }
func (f *fakeAIClient) SetTimeDilation(ctx context.Context, factor float64) error     { return nil }
func (f *fakeAIClient) DrainOutput(ctx context.Context) (map[string]string, error)    { return nil, nil }

// fakeVisionProvider is a scripted ai.VisionProvider — no network calls.
type fakeVisionProvider struct {
	verdict      ai.VisionVerdict
	err          error
	gotImage     []byte
	gotAssertion string

	extractedText string
	extractErr    error
	gotQuery      string
}

func (f *fakeVisionProvider) AssertScreen(ctx context.Context, image []byte, assertion string) (ai.VisionVerdict, error) {
	f.gotImage = image
	f.gotAssertion = assertion
	if f.err != nil {
		return ai.VisionVerdict{}, f.err
	}
	return f.verdict, nil
}

func (f *fakeVisionProvider) ExtractText(ctx context.Context, image []byte, query string) (string, error) {
	f.gotImage = image
	f.gotQuery = query
	if f.extractErr != nil {
		return "", f.extractErr
	}
	return f.extractedText, nil
}

// writeSolidPNGFixture writes a solid-red PNG to a temp file and returns its path.
func writeSolidPNGFixture(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for y := range 20 {
		for x := range 20 {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fixture png: %v", err)
	}
	path := filepath.Join(t.TempDir(), "screenshot.png")
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write fixture png: %v", err)
	}
	return path
}

func aiAssertStep(text string) parser.AssertStep {
	return parser.AssertStep{
		Sel:    parser.Selector{Kind: parser.SelectorText, Text: text},
		WithAI: true,
		Line:   1,
	}
}

func TestRunAssertWithAI_NoProviderConfigured(t *testing.T) {
	e := newTestExecutor()
	err := e.runAssert(context.Background(), aiAssertStep("checkout total looks correct"))
	if err == nil {
		t.Fatal("expected an error when no ai provider is configured")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error should say the provider isn't configured, got: %v", err)
	}
}

func TestRunAssertWithAI_UnknownProviderConfigErrSurfaced(t *testing.T) {
	e := newTestExecutor()
	e.SetAI(config.AIConfig{Provider: "not-a-real-provider", APIKey: "sk-x"})
	err := e.runAssert(context.Background(), aiAssertStep("x"))
	if err == nil {
		t.Fatal("expected an error for an unrecognized ai.provider value")
	}
	if !strings.Contains(err.Error(), "not-a-real-provider") {
		t.Errorf("error should name the bad provider value, got: %v", err)
	}
}

func TestRunAssertWithAI_Pass(t *testing.T) {
	e := newTestExecutor()
	path := writeSolidPNGFixture(t)
	e.client = &fakeAIClient{screenshotPath: path}
	fp := &fakeVisionProvider{verdict: ai.VisionVerdict{True: true, Reasoning: "looks right"}}
	e.aiProvider = fp

	err := e.runAssert(context.Background(), aiAssertStep("checkout total looks correct"))
	if err != nil {
		t.Fatalf("expected a passing AI assertion to return nil, got: %v", err)
	}
	if fp.gotAssertion != "checkout total looks correct" {
		t.Errorf("assertion passed to provider = %q, want %q", fp.gotAssertion, "checkout total looks correct")
	}
	if len(e.artifacts) != 1 || e.artifacts[0] != path {
		t.Errorf("expected the screenshot path to be recorded as an artifact, got: %v", e.artifacts)
	}
}

func TestRunAssertWithAI_Fail(t *testing.T) {
	e := newTestExecutor()
	e.client = &fakeAIClient{screenshotPath: writeSolidPNGFixture(t)}
	e.aiProvider = &fakeVisionProvider{verdict: ai.VisionVerdict{True: false, Reasoning: "total is missing"}}

	err := e.runAssert(context.Background(), aiAssertStep("checkout total looks correct"))
	if err == nil {
		t.Fatal("expected an error when the AI verdict is false")
	}
	if !strings.Contains(err.Error(), "checkout total looks correct") || !strings.Contains(err.Error(), "total is missing") {
		t.Errorf("error should include the assertion and the reasoning, got: %v", err)
	}
}

func TestRunAssertWithAI_ScreenshotFailurePropagates(t *testing.T) {
	e := newTestExecutor()
	e.client = &fakeAIClient{screenshotErr: errors.New("device disconnected")}
	e.aiProvider = &fakeVisionProvider{verdict: ai.VisionVerdict{True: true}}

	err := e.runAssert(context.Background(), aiAssertStep("x"))
	if err == nil || !strings.Contains(err.Error(), "device disconnected") {
		t.Errorf("expected the screenshot error to propagate, got: %v", err)
	}
}

func TestRunAssertWithAI_RedactionBlacksOutConfiguredRegion(t *testing.T) {
	e := newTestExecutor()
	client := &fakeAIClient{
		screenshotPath: writeSolidPNGFixture(t),
		bounds:         probelink.BoundsResult{X: 5, Y: 5, Width: 10, Height: 10},
	}
	e.client = client
	e.aiCfg.Redact = []config.RedactRule{{Selector: "#credit_card_field"}}
	fp := &fakeVisionProvider{verdict: ai.VisionVerdict{True: true}}
	e.aiProvider = fp

	if err := e.runAssert(context.Background(), aiAssertStep("x")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.boundsCalls) != 1 {
		t.Fatalf("expected exactly one SelectorBounds call, got %d", len(client.boundsCalls))
	}
	if client.boundsCalls[0].Kind != "id" || client.boundsCalls[0].Text != "#credit_card_field" {
		t.Errorf("selector bounds resolved for %+v, want id selector \"#credit_card_field\"", client.boundsCalls[0])
	}

	// Verify the image handed to the AI provider is actually redacted:
	// pixel inside the configured bounds is black, outside stays red.
	got, err := png.Decode(bytes.NewReader(fp.gotImage))
	if err != nil {
		t.Fatalf("decode image passed to provider: %v", err)
	}
	r, g, b, a := got.At(10, 10).RGBA()
	if r != 0 || g != 0 || b != 0 || a>>8 != 255 {
		t.Errorf("pixel inside redacted region = (%d,%d,%d,%d), want black", r>>8, g>>8, b>>8, a>>8)
	}
	r, g, b, a = got.At(1, 1).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 || a>>8 != 255 {
		t.Errorf("pixel outside redacted region = (%d,%d,%d,%d), want original red", r>>8, g>>8, b>>8, a>>8)
	}
}

// ---- assert no visual defects with ai ----

func TestRunAssertNoDefects_NoProviderConfigured(t *testing.T) {
	e := newTestExecutor()
	err := e.runAssertNoDefects(context.Background(), parser.AssertNoDefectsStep{Line: 1})
	if err == nil {
		t.Fatal("expected an error when no ai provider is configured")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error should say the provider isn't configured, got: %v", err)
	}
}

func TestRunAssertNoDefects_Pass(t *testing.T) {
	e := newTestExecutor()
	path := writeSolidPNGFixture(t)
	e.client = &fakeAIClient{screenshotPath: path}
	fp := &fakeVisionProvider{verdict: ai.VisionVerdict{True: true}}
	e.aiProvider = fp

	err := e.runAssertNoDefects(context.Background(), parser.AssertNoDefectsStep{Line: 1})
	if err != nil {
		t.Fatalf("expected a passing no-defects check to return nil, got: %v", err)
	}
	if fp.gotAssertion != ai.NoDefectsAssertion {
		t.Errorf("assertion passed to provider = %q, want the fixed NoDefectsAssertion prompt", fp.gotAssertion)
	}
	if len(e.artifacts) != 1 || e.artifacts[0] != path {
		t.Errorf("expected the screenshot path to be recorded as an artifact, got: %v", e.artifacts)
	}
}

func TestRunAssertNoDefects_Fail(t *testing.T) {
	e := newTestExecutor()
	e.client = &fakeAIClient{screenshotPath: writeSolidPNGFixture(t)}
	e.aiProvider = &fakeVisionProvider{verdict: ai.VisionVerdict{True: false, Reasoning: "the submit button is cut off at the bottom edge"}}

	err := e.runAssertNoDefects(context.Background(), parser.AssertNoDefectsStep{Line: 1})
	if err == nil {
		t.Fatal("expected an error when the AI finds defects")
	}
	if !strings.Contains(err.Error(), "cut off at the bottom edge") {
		t.Errorf("error should include the AI's reasoning, got: %v", err)
	}
}

func TestRunAssertNoDefects_RedactionApplied(t *testing.T) {
	e := newTestExecutor()
	client := &fakeAIClient{
		screenshotPath: writeSolidPNGFixture(t),
		bounds:         probelink.BoundsResult{X: 5, Y: 5, Width: 10, Height: 10},
	}
	e.client = client
	e.aiCfg.Redact = []config.RedactRule{{Selector: "#credit_card_field"}}
	fp := &fakeVisionProvider{verdict: ai.VisionVerdict{True: true}}
	e.aiProvider = fp

	if err := e.runAssertNoDefects(context.Background(), parser.AssertNoDefectsStep{Line: 1}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.boundsCalls) != 1 {
		t.Fatalf("expected exactly one SelectorBounds call, got %d", len(client.boundsCalls))
	}

	got, err := png.Decode(bytes.NewReader(fp.gotImage))
	if err != nil {
		t.Fatalf("decode image passed to provider: %v", err)
	}
	r, g, b, a := got.At(10, 10).RGBA()
	if r != 0 || g != 0 || b != 0 || a>>8 != 255 {
		t.Errorf("pixel inside redacted region = (%d,%d,%d,%d), want black", r>>8, g>>8, b>>8, a>>8)
	}
}

// ---- read "..." with ai into <var> ----

func readActionStep(query, varName string) parser.ActionStep {
	return parser.ActionStep{
		Verb: parser.VerbReadWithAI,
		Text: query,
		Name: varName,
		Line: 1,
	}
}

func TestRunReadWithAI_NoProviderConfigured(t *testing.T) {
	e := newTestExecutor()
	err := e.runReadWithAI(context.Background(), readActionStep("the OTP code", "otp"))
	if err == nil {
		t.Fatal("expected an error when no ai provider is configured")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error should say the provider isn't configured, got: %v", err)
	}
}

func TestRunReadWithAI_Pass(t *testing.T) {
	e := newTestExecutor()
	path := writeSolidPNGFixture(t)
	e.client = &fakeAIClient{screenshotPath: path}
	fp := &fakeVisionProvider{extractedText: "123456"}
	e.aiProvider = fp

	err := e.runReadWithAI(context.Background(), readActionStep("the 6-digit OTP code", "otp"))
	if err != nil {
		t.Fatalf("expected a successful extraction to return nil, got: %v", err)
	}
	if fp.gotQuery != "the 6-digit OTP code" {
		t.Errorf("query passed to provider = %q, want %q", fp.gotQuery, "the 6-digit OTP code")
	}
	if got := e.vars["otp"]; got != "123456" {
		t.Errorf("e.vars[\"otp\"] = %q, want %q", got, "123456")
	}
	if len(e.artifacts) != 1 || e.artifacts[0] != path {
		t.Errorf("expected the screenshot path to be recorded as an artifact, got: %v", e.artifacts)
	}
}

func TestRunReadWithAI_NotFoundPropagatesError(t *testing.T) {
	e := newTestExecutor()
	e.client = &fakeAIClient{screenshotPath: writeSolidPNGFixture(t)}
	e.aiProvider = &fakeVisionProvider{extractErr: errors.New(`ai: could not find "the OTP code" on screen`)}

	err := e.runReadWithAI(context.Background(), readActionStep("the OTP code", "otp"))
	if err == nil {
		t.Fatal("expected an error when the provider can't find the requested text")
	}
	if _, ok := e.vars["otp"]; ok {
		t.Error("otp should not be set in e.vars when extraction fails")
	}
}

func TestRunReadWithAI_RedactionApplied(t *testing.T) {
	e := newTestExecutor()
	client := &fakeAIClient{
		screenshotPath: writeSolidPNGFixture(t),
		bounds:         probelink.BoundsResult{X: 5, Y: 5, Width: 10, Height: 10},
	}
	e.client = client
	e.aiCfg.Redact = []config.RedactRule{{Selector: "#credit_card_field"}}
	fp := &fakeVisionProvider{extractedText: "123456"}
	e.aiProvider = fp

	if err := e.runReadWithAI(context.Background(), readActionStep("the OTP code", "otp")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.boundsCalls) != 1 {
		t.Fatalf("expected exactly one SelectorBounds call, got %d", len(client.boundsCalls))
	}

	got, err := png.Decode(bytes.NewReader(fp.gotImage))
	if err != nil {
		t.Fatalf("decode image passed to provider: %v", err)
	}
	r, g, b, a := got.At(10, 10).RGBA()
	if r != 0 || g != 0 || b != 0 || a>>8 != 255 {
		t.Errorf("pixel inside redacted region = (%d,%d,%d,%d), want black", r>>8, g>>8, b>>8, a>>8)
	}
}
