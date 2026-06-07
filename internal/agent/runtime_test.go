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

func TestBuildToolSummaryMessages(t *testing.T) {
	messages := buildToolSummaryMessages("search", map[string]interface{}{"query": "test"}, map[string]interface{}{"results": []string{"a"}})
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[1].Content == "" {
		t.Fatalf("expected user message content")
	}
}

func TestParseToolArgs(t *testing.T) {
	args, err := parseToolArgs("")
	if err != nil {
		t.Fatalf("empty args error = %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("empty args len = %d, want 0", len(args))
	}

	args, err = parseToolArgs(`{"city":"北京","days":3}`)
	if err != nil {
		t.Fatalf("valid args error = %v", err)
	}
	if args["city"] != "北京" {
		t.Fatalf("city = %v, want 北京", args["city"])
	}
	if args["days"] != float64(3) {
		t.Fatalf("days = %v, want 3", args["days"])
	}

	if _, err := parseToolArgs(`{"city":`); err == nil {
		t.Fatalf("expected invalid json error")
	}
}
