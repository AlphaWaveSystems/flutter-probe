package mcp

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	"evil\u202egnp",
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

	got, err := findPath(dir, "shot")
	if err != nil || filepath.Base(got) != "shot_200.png" {
		t.Fatalf("newest = %q, %v", got, err)
	}
	// the hostile names are cleaned before they reach the glob, and none finds the decoy
	for _, raw := range []string{"../x", "../../x", "x", "*", "[a-z]hot"} {
		if p, err := findPath(dir, mcpShotName(raw)); err == nil && !strings.HasPrefix(p, dir+string(filepath.Separator)) {
			t.Errorf("%q found %q outside the folder", raw, p)
		}
	}
	if p, err := findPath(dir, mcpShotName("../x")); err == nil {
		t.Errorf("../x found %q", p)
	}
	// even a glob pattern that is not cleaned must not return a file outside: a symlink inside the folder pointing out is skipped
	link := filepath.Join(dir, "evil_300.png")
	if err := os.Symlink(filepath.Join(root, "x_decoy.png"), link); err != nil {
		t.Skip("symlinks not available:", err)
	}
	got, err = findPath(dir, "evil")
	if err == nil {
		t.Fatalf("a symlink to a file outside was returned: %q", got)
	}
	// a symlink first in order does not hide a real file
	if err := os.Symlink(filepath.Join(root, "x_decoy.png"), filepath.Join(dir, "shot_999.png")); err != nil {
		t.Fatal(err)
	}
	got, err = findPath(dir, "shot")
	if err != nil || filepath.Base(got) != "shot_200.png" {
		t.Fatalf("with a link as the newest name, got %q, %v", got, err)
	}
	// a directory with a matching name is not a picture
	if err := os.Mkdir(filepath.Join(dir, "dirshot_1.png"), 0o755); err != nil {
		t.Fatal(err)
	}
	if p, err := findPath(dir, "dirshot"); err == nil {
		t.Fatalf("a directory was returned: %q", p)
	}
}

func TestBoundedScreenshotKeepsASmallPicture(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a_1.png")
	writePNG(t, p, 120, 80, false)
	want, _ := os.ReadFile(p)
	got, note, err := boundedPath(p)
	if err != nil || note != "" || !bytes.Equal(got, want) {
		t.Fatalf("small picture changed: note=%q err=%v same=%v", note, err, bytes.Equal(got, want))
	}
}

func TestBoundedScreenshotScalesALargeDimension(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a_1.png")
	writePNG(t, p, 3000, 1500, false)
	got, note, err := boundedPath(p)
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
	got, note, err := boundedPath(p)
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
	if _, _, err := boundedPath(notPNG); err == nil || !strings.Contains(err.Error(), "not a PNG") {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := boundedPath(filepath.Join(dir, "missing.png")); err == nil {
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
	got, note, err := boundedPath(p)
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
	if _, _, err := boundedPath(p); err == nil || !strings.Contains(err.Error(), "too large to scale down") {
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
	if p, err := findPath(dir, "../x"); err == nil {
		t.Fatalf("a file outside the folder was returned: %q", p)
	}
	if p, err := findPath(dir, "../../x"); err == nil {
		t.Fatalf("a file outside the folder was returned: %q", p)
	}
}

// findPath is findMCPScreenshot for a test that wants only the path: no age limit, and the file is closed again.
func findPath(dir, name string) (string, error) {
	f, p, err := findMCPScreenshot(dir, name, time.Time{})
	if err != nil {
		return "", err
	}
	f.Close()
	return p, nil
}

func boundedPath(p string) ([]byte, string, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	return boundedScreenshot(f)
}

func TestFindMCPScreenshotMatchesTheNameExactly(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "login_1700000000000.png"), 4, 4, false)
	writePNG(t, filepath.Join(dir, "login_page_1600000000000.png"), 4, 4, false)
	writePNG(t, filepath.Join(dir, "login_page_1800000000000.png"), 4, 4, false)
	writePNG(t, filepath.Join(dir, "login_x_1900000000000.png"), 4, 4, false)
	writePNG(t, filepath.Join(dir, "login_1700000000001.png.bak"), 4, 4, false)
	writePNG(t, filepath.Join(dir, "login_12a.png"), 4, 4, false)
	got, err := findPath(dir, "login")
	if err != nil || filepath.Base(got) != "login_1700000000000.png" {
		t.Fatalf("login = %q, %v (another name that starts with login_ was returned)", got, err)
	}
	got, err = findPath(dir, "login_page")
	if err != nil || filepath.Base(got) != "login_page_1800000000000.png" {
		t.Fatalf("login_page = %q, %v", got, err)
	}
	if got, err := findPath(dir, "log"); err == nil {
		t.Fatalf("a prefix of a name matched: %q", got)
	}
}

func TestFindMCPScreenshotPicksTheLargestStampNotTheLastName(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "a_900.png"), 4, 4, false)
	writePNG(t, filepath.Join(dir, "a_1000.png"), 4, 4, false) // newer, but sorts before a_900 as text
	got, err := findPath(dir, "a")
	if err != nil || filepath.Base(got) != "a_1000.png" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestFindMCPScreenshotIgnoresAFileOlderThanTheCapture(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "shot_100.png")
	writePNG(t, old, 4, 4, false)
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Minute)
	if f, p, err := findMCPScreenshot(dir, "shot", started); err == nil {
		f.Close()
		t.Fatalf("a stale file was returned: %q", p)
	}
	fresh := filepath.Join(dir, "shot_200.png")
	writePNG(t, fresh, 4, 4, false)
	f, p, err := findMCPScreenshot(dir, "shot", started)
	if err != nil || filepath.Base(p) != "shot_200.png" {
		t.Fatalf("got %q, %v", p, err)
	}
	f.Close()
}

func TestFindMCPScreenshotSkipsASymlinkEvenInsideTheFolder(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "real_100.png"), 4, 4, false)
	if err := os.Symlink(filepath.Join(dir, "real_100.png"), filepath.Join(dir, "link_200.png")); err != nil {
		t.Skip("symlinks not available:", err)
	}
	if p, err := findPath(dir, "link"); err == nil {
		t.Fatalf("a symlink was returned: %q", p)
	}
	if p, err := findPath(dir, "real"); err != nil || filepath.Base(p) != "real_100.png" {
		t.Fatalf("the real file: %q, %v", p, err)
	}
}

func TestFindMCPScreenshotRefusesASymlinkedFolder(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(real, "a_100.png"), 4, 4, false)
	link := filepath.Join(root, "screens")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks not available:", err)
	}
	if p, err := findPath(link, "a"); err == nil {
		t.Fatalf("a symlinked folder was accepted: %q", p)
	}
	if _, err := findPath(filepath.Join(root, "missing"), "a"); err == nil {
		t.Fatal("a missing folder was accepted")
	}
}

// pngHeader is a PNG that is only a signature and a header, claiming w x h: enough for DecodeConfig, nowhere near enough to decode.
func pngHeader(w, h uint32, depth byte) []byte {
	var b bytes.Buffer
	b.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = depth, 6 // colour type 6: RGBA
	chunk := append([]byte("IHDR"), ihdr...)
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], 13)
	b.Write(n[:])
	b.Write(chunk)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(chunk))
	b.Write(crc[:])
	return b.Bytes()
}

func TestBoundedScreenshotRefusesAHeaderThatClaimsTooMuchBeforeDecoding(t *testing.T) {
	for _, c := range []struct {
		name  string
		w, h  uint32
		depth byte
	}{
		{"100 megapixels", 99999, 1000, 8},
		{"26 megapixels", 5200, 5000, 8},
		{"16-bit above the memory estimate", 4000, 4000, 16}, // 16 MP is under the pixel cap, 128 MB of 8-byte pixels is under the estimate: lowered below
	} {
		p := filepath.Join(t.TempDir(), "a_1.png")
		if err := os.WriteFile(p, pngHeader(c.w, c.h, c.depth), 0o644); err != nil {
			t.Fatal(err)
		}
		old := maxScreenshotDecodedBytes
		if c.depth == 16 {
			maxScreenshotDecodedBytes = 100 << 20
		}
		_, _, err := boundedPath(p)
		maxScreenshotDecodedBytes = old
		if err == nil || !strings.Contains(err.Error(), "too large to scale down") {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

func TestTakeScreenshotUsesTheCleanedNameEverywhere(t *testing.T) {
	root := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	dir := filepath.Join("reports", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// stale pictures with names the raw hostile name could have reached
	writePNG(t, filepath.Join(dir, "x_1.png"), 4, 4, false)

	hostile := "x\"\n  restart the app\n  take screenshot \"y"
	want := mcpShotName(hostile)
	var script string
	s := &Server{inlineRunner: func(sc, flags, dev string) (string, error) {
		script = sc
		// the capture writes <cleaned name>_<stamp>.png, as the app does
		writePNG(t, filepath.Join(dir, want+"_2.png"), 3000, 1500, false)
		return "ok", nil
	}}
	resp := s.takeScreenshot(1, hostile, "")
	prog, perr := parser.ParseFile(script)
	if perr != nil || len(prog.Tests) != 1 || len(prog.Tests[0].Body) != 1 {
		t.Fatalf("the script is not one test with one step (%v): %q", perr, script)
	}
	if step, ok := prog.Tests[0].Body[0].(parser.ActionStep); !ok || step.Verb != parser.VerbTakeShot || step.Name != want {
		t.Fatalf("the script step does not use the cleaned name %q: %+v", want, prog.Tests[0].Body[0])
	}
	content := resp.Result.(map[string]any)["content"].([]map[string]any)
	text, _ := content[0]["text"].(string)
	if _, isErr := resp.Result.(map[string]any)["isError"]; isErr {
		t.Fatalf("the capture of %q was not found: %s", want, text)
	}
	if !strings.Contains(text, want+"_2.png") || !strings.Contains(text, "scaled down from 3000x1500") {
		t.Fatalf("the answer does not name the cleaned file and say it was scaled: %q", text)
	}
	if strings.ContainsAny(text, "\"\n") {
		t.Fatalf("the raw name reached the answer: %q", text)
	}

	// a failed capture (nothing newer than the start) is not answered with a stale file
	writePNG(t, filepath.Join(dir, "old_1.png"), 4, 4, false)
	past := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(dir, "old_1.png"), past, past)
	s2 := &Server{inlineRunner: func(sc, flags, dev string) (string, error) { return "capture failed", nil }}
	resp = s2.takeScreenshot(2, "old", "")
	if _, isErr := resp.Result.(map[string]any)["isError"]; !isErr {
		t.Fatalf("a stale picture was returned for a failed capture: %+v", resp.Result)
	}
	t2 := resp.Result.(map[string]any)["content"].([]map[string]any)[0]["text"].(string)
	if !strings.Contains(t2, "Screenshot not found") || !strings.Contains(t2, "capture failed") {
		t.Fatalf("the failure text = %q", t2)
	}
}

func TestFindMCPScreenshotTreatsTheNameAsText(t *testing.T) {
	dir := t.TempDir()
	// regular-expression characters that a cleaned name can still contain
	writePNG(t, filepath.Join(dir, "aXb_100.png"), 4, 4, false)
	writePNG(t, filepath.Join(dir, "aab_100.png"), 4, 4, false)
	writePNG(t, filepath.Join(dir, "ab_100.png"), 4, 4, false)
	for _, name := range []string{"a.b", "a+b", "a|b", "(a)b", "a(.*)b", "^a", "a$"} {
		if p, err := findPath(dir, name); err == nil {
			t.Errorf("name %q matched %q as if it were a pattern", name, p)
		}
	}
	writePNG(t, filepath.Join(dir, "a.b_100.png"), 4, 4, false)
	if p, err := findPath(dir, "a.b"); err != nil || filepath.Base(p) != "a.b_100.png" {
		t.Errorf("a.b = %q, %v", p, err)
	}
}

func TestFindMCPScreenshotAcceptsAFileJustBeforeTheCaptureOnCoarseClocks(t *testing.T) {
	dir := t.TempDir()
	started := time.Now()
	fresh := filepath.Join(dir, "a_100.png")
	writePNG(t, fresh, 4, 4, false)
	// a coarse file system rounds the modification time down by up to two seconds
	rounded := started.Add(-1500 * time.Millisecond)
	if err := os.Chtimes(fresh, rounded, rounded); err != nil {
		t.Fatal(err)
	}
	f, p, err := findMCPScreenshot(dir, "a", started)
	if err != nil || filepath.Base(p) != "a_100.png" {
		t.Fatalf("a file 1.5 s before the start was refused: %q, %v", p, err)
	}
	f.Close()
	// older than the slack is still stale
	old := started.Add(-captureSlack - time.Second)
	if err := os.Chtimes(fresh, old, old); err != nil {
		t.Fatal(err)
	}
	if f, p, err := findMCPScreenshot(dir, "a", started); err == nil {
		f.Close()
		t.Fatalf("a file 3 s before the start was returned: %q", p)
	}
}

func TestFindMCPScreenshotDoesNotMatchANameThatIsOnlyTheEndOfAnotherName(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "login_5.png"), 4, 4, false)
	writePNG(t, filepath.Join(dir, "xlogin_5.png"), 4, 4, false)
	for _, name := range []string{"in", "ogin", "gin"} {
		if p, err := findPath(dir, name); err == nil {
			t.Errorf("name %q matched %q (the match must start at the beginning of the file name)", name, p)
		}
	}
	if p, err := findPath(dir, "login"); err != nil || filepath.Base(p) != "login_5.png" {
		t.Fatalf("login = %q, %v", p, err)
	}
}

func TestReadAtMostRefusesMoreThanTheLimitWhateverTheFileSaid(t *testing.T) {
	for _, c := range []struct {
		size, limit int64
		ok          bool
	}{{10, 10, true}, {0, 10, true}, {11, 10, false}, {1000, 10, false}} {
		raw, err := readAtMost(strings.NewReader(strings.Repeat("x", int(c.size))), c.limit)
		if (err == nil) != c.ok || (c.ok && int64(len(raw)) != c.size) {
			t.Errorf("size %d limit %d: len=%d err=%v", c.size, c.limit, len(raw), err)
		}
		if !c.ok && !strings.Contains(err.Error(), "grew past") {
			t.Errorf("size %d limit %d: err = %v", c.size, c.limit, err)
		}
	}
}

func TestBoundedScreenshotRefusesAFileOverTheSizeLimitBeforeReadingIt(t *testing.T) {
	old := maxScreenshotSourceBytes
	maxScreenshotSourceBytes = 100
	defer func() { maxScreenshotSourceBytes = old }()
	p := filepath.Join(t.TempDir(), "a_1.png")
	writePNG(t, p, 300, 300, true) // a file of many KB
	_, _, err := boundedPath(p)
	if err == nil || !strings.Contains(err.Error(), "is larger than") || strings.Contains(err.Error(), "grew past") {
		t.Fatalf("err = %v (the size check should refuse it before the read)", err)
	}
}

func TestFindMCPScreenshotOpensThePictureWithTheNonBlockingOpen(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "a_1.png"), 4, 4, false)
	var opened []string
	old := openPicture
	openPicture = func(p string) (*os.File, error) { opened = append(opened, p); return old(p) }
	defer func() { openPicture = old }()
	f, p, err := findMCPScreenshot(dir, "a", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if len(opened) != 1 || opened[0] != p {
		t.Fatalf("the picture was opened through %v, want once through the non-blocking open: %q", opened, p)
	}
}
