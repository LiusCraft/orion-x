package agent

import "testing"

func TestDeltaFromBufferedContent(t *testing.T) {
	cases := []struct {
		name       string
		content    string
		lastLength int
		wantDelta  string
		wantNext   int
	}{
		{
			name:       "initial chunk",
			content:    "你好",
			lastLength: 0,
			wantDelta:  "你好",
			wantNext:   len("你好"),
		},
		{
			name:       "prefix growth",
			content:    "你好，",
			lastLength: len("你好"),
			wantDelta:  "，",
			wantNext:   len("你好，"),
		},
		{
			name:       "another prefix growth",
			content:    "你好，请问",
			lastLength: len("你好，"),
			wantDelta:  "请问",
			wantNext:   len("你好，请问"),
		},
		{
			name:       "clamp when length shrinks",
			content:    "abc",
			lastLength: 5,
			wantDelta:  "",
			wantNext:   len("abc"),
		},
		{
			name:       "negative length",
			content:    "xyz",
			lastLength: -3,
			wantDelta:  "xyz",
			wantNext:   len("xyz"),
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			delta, next := deltaFromBufferedContent(tt.content, tt.lastLength)
			if delta != tt.wantDelta {
				t.Fatalf("delta = %q, want %q", delta, tt.wantDelta)
			}
			if next != tt.wantNext {
				t.Fatalf("next = %d, want %d", next, tt.wantNext)
			}
		})
	}
}
