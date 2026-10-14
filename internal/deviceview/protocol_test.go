package deviceview

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func f64(v float64) *float64 { return &v }

func TestParseIncomingTable(t *testing.T) {
	big := `{"t":"text","s":"` + strings.Repeat("a", MaxMessageBytes) + `"}`
	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"tap", `{"t":"tap","x":1,"y":2}`, false},
		{"tap with numeric id", `{"t":"tap","id":12,"x":1,"y":2}`, false},
		{"string id", `{"t":"tap","id":"a-1","x":1,"y":2}`, false},
		{"null id", `{"t":"tap","id":null,"x":1,"y":2}`, false},
		{"object id", `{"t":"tap","id":{"a":1},"x":1,"y":2}`, true},
		{"array id", `{"t":"tap","id":[1],"x":1,"y":2}`, true},
		{"bool id", `{"t":"tap","id":true}`, true},
		{"long id", `{"t":"tap","id":"` + strings.Repeat("x", 40) + `"}`, true},
		{"empty", ``, true},
		{"truncated", `{"t":"tap","x":`, true},
		{"not an object", `[1,2]`, true},
		{"string", `"tap"`, true},
		{"null", `null`, true},
		{"type is a number", `{"t":1}`, true},
		{"missing type", `{"x":1}`, true},
		{"empty type", `{"t":""}`, true},
		{"coordinate overflow", `{"t":"tap","x":1e999,"y":1}`, true},
		{"unknown fields are ignored", `{"t":"tap","x":1,"y":2,"zzz":[1,2,3]}`, false},
		{"nested garbage", `{"t":"tap","x":{"a":{"b":{}}}}`, true},
		{"oversized", big, true},
		{"invalid utf8 in string", "{\"t\":\"text\",\"s\":\"\xff\xfe\"}", false}, // decoded with replacement characters
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseIncoming([]byte(tc.in))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestCoordinateValidation(t *testing.T) {
	ok := []float64{0, 1, 19999.5, MaxCoord}
	bad := []float64{-0.001, MaxCoord + 0.001, math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, v := range ok {
		if _, good := validCoord(f64(v)); !good {
			t.Errorf("%v rejected", v)
		}
	}
	for _, v := range bad {
		if _, good := validCoord(f64(v)); good {
			t.Errorf("%v accepted", v)
		}
	}
	if _, good := validCoord(nil); good {
		t.Error("nil accepted")
	}
}

func TestGestureMsClamp(t *testing.T) {
	if gestureMs(nil, 600) != 600 || gestureMs(f64(0), 600) != 1 || gestureMs(f64(-5), 600) != 1 ||
		gestureMs(f64(1e9), 600) != maxGestureMs || gestureMs(f64(250.4), 600) != 250 {
		t.Fatal("gestureMs clamp wrong")
	}
}

func TestConfigApply(t *testing.T) {
	cfg := defaultConfig()
	m, _ := parseIncoming([]byte(`{"t":"config","fps":30,"quality":1,"scale":0.1,"tree":true}`))
	got, bad := m.applyTo(cfg)
	if len(bad) != 0 || got != (Config{FPS: 30, Quality: 1, Scale: 0.1, Tree: true}) {
		t.Fatalf("got %+v bad=%v", got, bad)
	}
	m, _ = parseIncoming([]byte(`{"t":"config","fps":31,"quality":101,"scale":0.09}`))
	got, bad = m.applyTo(cfg)
	if len(bad) != 3 || got != cfg {
		t.Fatalf("got %+v bad=%v", got, bad)
	}
}

func TestFrameRoundTrip(t *testing.T) {
	img := []byte{9, 8, 7}
	b := encodeFrame(frameHeader{Seq: 5, TS: 123, W: 10, H: 20, Fmt: "jpeg"}, img)
	if b[0] != 0x01 {
		t.Fatalf("marker = %#x", b[0])
	}
	n := int(b[1])<<24 | int(b[2])<<16 | int(b[3])<<8 | int(b[4])
	var hdr map[string]any
	if err := json.Unmarshal(b[5:5+n], &hdr); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"seq", "ts", "w", "h", "fmt"} {
		if _, ok := hdr[k]; !ok {
			t.Fatalf("header lacks %q: %s", k, b[5:5+n])
		}
	}
	h, got, err := decodeFrame(b)
	if err != nil || h.Seq != 5 || h.Fmt != "jpeg" || string(got) != string(img) {
		t.Fatalf("round trip: %+v %v %v", h, got, err)
	}
	for _, bad := range [][]byte{nil, {1}, {2, 0, 0, 0, 0}, {1, 0, 0, 0, 9, '{'}} {
		if _, _, err := decodeFrame(bad); err == nil {
			t.Fatalf("decodeFrame(%v) succeeded", bad)
		}
	}
}

func TestLimiter(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLimiter(50, now)
	allowed := 0
	for i := 0; i < 200; i++ {
		if l.allow(now) {
			allowed++
		}
	}
	if allowed != 50 {
		t.Fatalf("burst allowed %d, want 50", allowed)
	}
	now = now.Add(500 * time.Millisecond)
	allowed = 0
	for i := 0; i < 200; i++ {
		if l.allow(now) {
			allowed++
		}
	}
	if allowed != 25 {
		t.Fatalf("after 0.5 s allowed %d, want 25", allowed)
	}
}

func FuzzParseIncoming(f *testing.F) {
	for _, s := range []string{
		`{"t":"tap","id":1,"x":1,"y":2}`, `{"t":"config","fps":10}`, `{"t":`, `null`, `[]`,
		`{"t":"swipe","x1":1,"y1":1,"x2":2,"y2":2,"ms":300}`, `{"t":"text","s":"\u0000"}`,
		`{"id":"` + strings.Repeat("9", 40) + `"}`, "\xff\xfe\x00",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := parseIncoming(data)
		if len(data) > MaxMessageBytes && err == nil {
			t.Fatal("oversized message accepted")
		}
		if err != nil {
			return
		}
		if m.T == "" {
			t.Fatal("accepted message without type")
		}
		if len(m.ID) > 0 && !json.Valid(m.ID) {
			t.Fatalf("accepted invalid id %q", m.ID)
		}
		// replies must always be encodable
		if _, err := json.Marshal(ackMsg{T: "ack", ID: m.ID, OK: true}); err != nil {
			t.Fatalf("cannot echo id: %v", err)
		}
		_, _, _ = m.point()
		_, _, _, _, _ = m.swipePoints()
		_, _ = m.applyTo(defaultConfig())
	})
}

func TestEncodeFrameRefusesHugeImage(t *testing.T) {
	if encodeFrame(frameHeader{Seq: 1, Fmt: "png"}, make([]byte, maxImageBytes+1)) != nil {
		t.Fatal("an image over the bound must not be encoded")
	}
	if encodeFrame(frameHeader{Seq: 1, Fmt: "png"}, make([]byte, 1024)) == nil {
		t.Fatal("a normal image must be encoded")
	}
}
