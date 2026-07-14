package language

import (
	"testing"
)

func TestAllNotEmpty(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("All() returned empty")
	}
	for _, info := range all {
		if info.Code == "" || info.Name == "" {
			t.Fatalf("All() entry has empty code or name: %+v", info)
		}
	}
}

func TestGetHitAndMiss(t *testing.T) {
	if got := Get(ZH); got == nil || got.Code != ZH {
		t.Fatalf("Get(ZH) = %v, want non-nil with code ZH", got)
	}
	if got := Get(Code("xx")); got != nil {
		t.Fatalf("Get(xx) = %v, want nil", got)
	}
}

func TestExists(t *testing.T) {
	if !Exists(ZH) {
		t.Fatal("Exists(ZH) = false, want true")
	}
	if Exists(Code("xx")) {
		t.Fatal("Exists(xx) = true, want false")
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		raw  string
		want Code
	}{
		{"zh", ZH},
		{"zh-CN", ZH},
		{"zh-TW", ZH},
		{"ZH", ZH},
		{"Chinese", ZH},
		{"中文", ZH},
		{"en", EN},
		{"en-US", EN},
		{"English", EN},
		{"ja", JA},
		{"Japanese", JA},
		{"ko", KO},
		{"Korean", KO},
		{"ru", RU},
		{"Russian", RU},
		{"", ""},
		{"xx", Code("xx")}, // unknown passes through
	}
	for _, tc := range tests {
		got := Normalize(tc.raw)
		if got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		want      string
		supported []Code
		expected  bool
	}{
		{"zh", []Code{ZH, EN, RU}, true},
		{"zh-CN", []Code{ZH, EN}, true},
		{"en-US", []Code{ZH, EN}, true},
		{"ja", []Code{ZH, EN}, false},
		{"zh", nil, false},
		{"zh", []Code{}, false},
	}
	for _, tc := range tests {
		got := Match(tc.want, tc.supported)
		if got != tc.expected {
			t.Errorf("Match(%q, %v) = %v, want %v", tc.want, tc.supported, got, tc.expected)
		}
	}
}

func TestCodes(t *testing.T) {
	got := Codes([]string{"zh", "en", "xx", "ja"})
	if len(got) != 3 {
		t.Fatalf("Codes() len=%d, want 3 (xx filtered)", len(got))
	}
	if got[0] != ZH || got[1] != EN || got[2] != JA {
		t.Fatalf("Codes() = %v, want [ZH EN JA]", got)
	}
}
