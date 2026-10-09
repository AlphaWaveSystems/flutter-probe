package locale

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in, bcp, posix string
		rtl, system    bool
	}{
		{"de", "de", "de", false, false},
		{"de-de", "de-DE", "de_DE", false, false},
		{"pt_BR", "pt-BR", "pt_BR", false, false},
		{"zh-hans-cn", "zh-Hans-CN", "zh_Hans_CN", false, false},
		{"es-419", "es-419", "es_419", false, false},
		{"ar", "ar", "ar", true, false},
		{"he-IL", "he-IL", "he_IL", true, false},
		{"fa", "fa", "fa", true, false},
		{" System ", "", "", false, true},
		{"default", "", "", false, true},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.in, err)
		}
		if got.BCP47 != c.bcp || got.POSIX != c.posix || got.RTL != c.rtl || got.System != c.system {
			t.Errorf("Parse(%q) = %+v", c.in, got)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{"", "d", "german", "de-D", "de-DE-x1", "12"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) should fail", in)
		}
	}
}

func TestParseList(t *testing.T) {
	tags, err := ParseList("de, ja,,ar")
	if err != nil || len(tags) != 3 || tags[2].BCP47 != "ar" {
		t.Fatalf("ParseList = %+v, %v", tags, err)
	}
	if _, err := ParseList(" , "); err == nil {
		t.Fatal("empty list must fail")
	}
}
