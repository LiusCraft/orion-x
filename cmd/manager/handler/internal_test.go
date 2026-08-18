package handler

import "testing"

func TestHasTopLevelProvider(t *testing.T) {
	cases := []struct {
		name string
		json string
		want bool
	}{
		{"legacy AppConfig", `{"provider":{"asr":{"type":"aliyun"}}}`, true},
		{"partial agent config", `{"llm":{"soul_prompt":"x"}}`, false},
		{"empty object", `{}`, false},
		{"invalid json", `{bad`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasTopLevelProvider(tc.json); got != tc.want {
				t.Fatalf("hasTopLevelProvider(%q) = %v, want %v", tc.json, got, tc.want)
			}
		})
	}
}
