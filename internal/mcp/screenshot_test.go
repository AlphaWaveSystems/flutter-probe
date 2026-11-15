package mcp

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

var hostileShotNames = []string{
	"x\"\n  restart the app\n  take screenshot \"y",
	"x\"\n  clear app data\n  take screenshot \"y",
	"x\"\n  call POST \"http://example.invalid/\" with body \"{}\"\n  take screenshot \"y",
	"x\" # comment",
	"../../etc/x",
	"/etc/cron.d/x",
	`..\..\x`,
	"a*b",
	"a?b",
	"[a-z]",
	"a\x00b",
	"evil‮gnp",
	"${HOME}",
	"<timestamp>",
	"`id`",
	"$(id)",
	"it's",
	strings.Repeat("a", 10000),
}

func TestMCPShotNameIsPlain(t *testing.T) {
	for _, raw := range hostileShotNames {
		got := mcpShotName(raw)
		if strings.ContainsAny(got, "\"'`$<>{}*?[]/\\\x00\n\r") || strings.Contains(got, "..") || len(got) > 120 || got == "" {
			t.Errorf("mcpShotName(%.30q) = %q is not a plain fragment", raw, got)
		}
	}
}

func TestMCPShotNameCases(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "mcp_capture"},
		{"   ", "mcp_capture"},
		{"login screen 2", "login screen 2"},
		{"checkout_v1.2-ok", "checkout_v1.2-ok"},
		{"Prüfung 支付", "Prüfung 支付"},
		{"a*b?c[d]", "a_b_c_d_"},
		{`say "hi"`, "say _hi_"},
		{"${HOME}", "__HOME_"},
		{"<test>", "_test_"},
		{"../../x", "__x"},
	}
	for _, c := range cases {
		if got := mcpShotName(c.in); got != c.want {
			t.Errorf("mcpShotName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMCPShotScriptIsExactlyOneStep(t *testing.T) {
	for _, raw := range append(hostileShotNames, "plain name", "") {
		name := mcpShotName(raw)
		prog, err := parser.ParseFile(mcpShotScript(name))
		if err != nil {
			t.Errorf("%.30q: script does not parse: %v", raw, err)
			continue
		}
		if len(prog.Tests) != 1 || len(prog.Tests[0].Body) != 1 {
			t.Errorf("%.30q: want one test with one step, got %d tests, body %+v", raw, len(prog.Tests), prog.Tests)
			continue
		}
		step, ok := prog.Tests[0].Body[0].(parser.ActionStep)
		if !ok || step.Verb != parser.VerbTakeShot || step.Name != name {
			t.Errorf("%.30q: step = %+v, want take screenshot %q", raw, prog.Tests[0].Body[0], name)
		}
	}
}

func writePNG(t *testing.T, path string, w, h int, noisy bool) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewSource(1))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if noisy {
				img.Set(x, y, color.RGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255})
			} else {
				img.Set(x, y, color.RGBA{10, 20, 30, 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindMCPScreenshotStaysInsideTheFolder(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "reports", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// a decoy outside the folder that a path-escaping name would reach
	writePNG(t, filepath.Join(root, "x_decoy.png"), 4, 4, false)
	writePNG(t, filepath.Join(dir, "shot_100.png"), 4, 4, false)
	writePNG(t, filepath.Join(dir, "shot_200.png"), 4, 4, false)

	got, err := findMCPScreenshot(dir, "shot")
	if err != nil || filepath.Base(got) != "shot_200.png" {
		t.Fatalf("newest = %q, %v", got, err)
	}
	// the hostile names are cleaned before they reach the glob, and none finds the decoy
	for _, raw := range []string{"../x", "../../x", "x", "*", "[a-z]hot"} {
		if p, err := findMCPScreenshot(dir, mcpShotName(raw)); err == nil && !strings.HasPrefix(p, dir+string(filepath.Separator)) {
			t.Errorf("%q found %q outside the folder", raw, p)
		}
	}
	if p, err := findMCPScreenshot(dir, mcpShotName("../x")); err == nil {
		t.Errorf("../x found %q", p)
	}
	// even a glob pattern that is not cleaned must not return a file outside: a symlink inside the folder pointing out is skipped
	link := filepath.Join(dir, "evil_300.png")
	if err := os.Symlink(filepath.Join(root, "x_decoy.png"), link); err != nil {
		t.Skip("symlinks not available:", err)
	}
	got, err = findMCPScreenshot(dir, "evil")
	if err == nil {
		t.Fatalf("a symlink to a file outside was returned: %q", got)
	}
	// a symlink first in order does not hide a real file
	if err := os.Symlink(filepath.Join(root, "x_decoy.png"), filepath.Join(dir, "shot_999.png")); err != nil {
		t.Fatal(err)
	}
	got, err = findMCPScreenshot(dir, "shot")
	if err != nil || filepath.Base(got) != "shot_200.png" {
		t.Fatalf("with a link as the newest name, got %q, %v", got, err)
	}
	// a directory with a matching name is not a picture
	if err := os.Mkdir(filepath.Join(dir, "dirshot_1.png"), 0o755); err != nil {
		t.Fatal(err)
	}
	if p, err := findMCPScreenshot(dir, "dirshot"); err == nil {
		t.Fatalf("a directory was returned: %q", p)
	}
}

func TestBoundedScreenshotKeepsASmallPicture(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a_1.png")
	writePNG(t, p, 120, 80, false)
	want, _ := os.ReadFile(p)
	got, note, err := boundedScreenshot(p)
	if err != nil || note != "" || !bytes.Equal(got, want) {
		t.Fatalf("small picture changed: note=%q err=%v same=%v", note, err, bytes.Equal(got, want))
	}
}

func TestBoundedScreenshotScalesALargeDimension(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a_1.png")
	writePNG(t, p, 3000, 1500, false)
	got, note, err := boundedScreenshot(p)
	if err != nil || !strings.Contains(note, "scaled down from 3000x1500") {
		t.Fatalf("note=%q err=%v", note, err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(got))
	if err != nil || cfg.Width != 2000 || cfg.Height != 1000 {
		t.Fatalf("scaled size = %dx%d, %v", cfg.Width, cfg.Height, err)
	}
	if len(got) > maxScreenshotBytes {
		t.Fatalf("result is %d bytes", len(got))
	}
}

func TestBoundedScreenshotScalesAHeavyFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a_1.png")
	writePNG(t, p, 1500, 1500, true) // noisy: a PNG far above the byte cap although within the dimension cap
	fi, _ := os.Stat(p)
	if fi.Size() <= maxScreenshotBytes {
		t.Fatalf("test picture is only %d bytes, it must be above the cap", fi.Size())
	}
	got, note, err := boundedScreenshot(p)
	if err != nil || note == "" || len(got) > maxScreenshotBytes {
		t.Fatalf("note=%q err=%v len=%d", note, err, len(got))
	}
}

func TestBoundedScreenshotRefusals(t *testing.T) {
	dir := t.TempDir()
	notPNG := filepath.Join(dir, "a_1.png")
	if err := os.WriteFile(notPNG, []byte("not a picture"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := boundedScreenshot(notPNG); err == nil || !strings.Contains(err.Error(), "not a PNG") {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := boundedScreenshot(filepath.Join(dir, "missing.png")); err == nil {
		t.Fatal("a missing file was accepted")
	}
}

func TestScaleToFitKeepsRatioAndColour(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 400; x++ {
			src.Set(x, y, color.RGBA{200, 100, 50, 255})
		}
	}
	out := scaleToFit(src, 100)
	if b := out.Bounds(); b.Dx() != 100 || b.Dy() != 50 {
		t.Fatalf("size = %v", b)
	}
	if r, g, b, a := out.At(10, 10).RGBA(); r>>8 != 200 || g>>8 != 100 || b>>8 != 50 || a>>8 != 255 {
		t.Fatalf("colour changed: %d %d %d %d", r>>8, g>>8, b>>8, a>>8)
	}
	same := scaleToFit(src, 1000)
	if b := same.Bounds(); b.Dx() != 400 || b.Dy() != 200 {
		t.Fatalf("a picture within the limit was resized: %v", b)
	}
}

func TestBoundedScreenshotScalesATallPicture(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a_1.png")
	writePNG(t, p, 1000, 3000, false) // within the width limit, above the height limit
	got, note, err := boundedScreenshot(p)
	if err != nil || note == "" {
		t.Fatalf("note=%q err=%v", note, err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(got))
	if err != nil || cfg.Width != 666 || cfg.Height != 2000 {
		t.Fatalf("scaled size = %dx%d, %v", cfg.Width, cfg.Height, err)
	}
}

func TestBoundedScreenshotRefusesTooManyPixels(t *testing.T) {
	old := maxScreenshotSourcePixels
	maxScreenshotSourcePixels = 100
	defer func() { maxScreenshotSourcePixels = old }()
	p := filepath.Join(t.TempDir(), "a_1.png")
	writePNG(t, p, 3000, 100, false) // must be scaled, and has more pixels than the lowered limit
	if _, _, err := boundedScreenshot(p); err == nil || !strings.Contains(err.Error(), "too large to scale down") {
		t.Fatalf("err = %v", err)
	}
}

func TestFindMCPScreenshotRefusesAFileOutsideEvenForAnUncleanedName(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "reports", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(root, "reports", "x_decoy.png"), 4, 4, false)
	// the cleaning prevents this name from ever arriving; the check must hold on its own
	if p, err := findMCPScreenshot(dir, "../x"); err == nil {
		t.Fatalf("a file outside the folder was returned: %q", p)
	}
	if p, err := findMCPScreenshot(dir, "../../x"); err == nil {
		t.Fatalf("a file outside the folder was returned: %q", p)
	}
}
