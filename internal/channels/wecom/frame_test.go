package wecom

import (
	"testing"
	"unicode/utf8"
)

func TestCallbackBodyText(t *testing.T) {
	tests := []struct {
		name      string
		body      callbackBody
		want      string
		supported bool
	}{
		{
			name:      "text message",
			body:      callbackBody{MsgType: msgTypeText, Text: &textContent{Content: "你好"}},
			want:      "你好",
			supported: true,
		},
		{
			name:      "voice uses transcript",
			body:      callbackBody{MsgType: msgTypeVoice, Voice: &textContent{Content: "这是语音转成文本的内容"}},
			want:      "这是语音转成文本的内容",
			supported: true,
		},
		{
			name: "mixed joins only text items",
			body: callbackBody{MsgType: msgTypeMixed, Mixed: &mixedContent{MsgItem: []mixedItem{
				{MsgType: msgTypeText, Text: &textContent{Content: "@机器人 看一下"}},
				{MsgType: msgTypeImage},
				{MsgType: msgTypeText, Text: &textContent{Content: "   "}},
				{MsgType: msgTypeText, Text: &textContent{Content: "这两张图"}},
			}}},
			want:      "@机器人 看一下\n这两张图",
			supported: true,
		},
		{
			name:      "image unsupported",
			body:      callbackBody{MsgType: msgTypeImage},
			supported: false,
		},
		{
			name:      "unknown type unsupported",
			body:      callbackBody{MsgType: "location"},
			supported: false,
		},
		{
			name:      "text without payload is empty but supported",
			body:      callbackBody{MsgType: msgTypeText},
			supported: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, supported := tt.body.text()
			if supported != tt.supported {
				t.Fatalf("supported = %v, want %v", supported, tt.supported)
			}
			if got != tt.want {
				t.Errorf("text() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripMention(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text untouched", "你好", "你好"},
		{"group mention removed", "@RobotA hello robot", "hello robot"},
		{"multiple spaces collapsed", "@RobotA   hi", "hi"},
		{"mention only kept as-is", "@RobotA", "@RobotA"},
		{"email-like text kept", "@a@b.com 看下", "看下"},
		{"at inside text kept", "hello @someone", "hello @someone"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripMention(tt.in); got != tt.want {
				t.Errorf("stripMention(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestConversationKey(t *testing.T) {
	tests := []struct {
		name string
		body callbackBody
		want string
	}{
		{
			name: "group uses chat id",
			body: callbackBody{ChatType: chatTypeGroup, ChatID: "CHATID", From: callbackFrom{UserID: "USERID"}},
			want: "wecom:dev-1:CHATID",
		},
		{
			name: "single chat uses user id",
			body: callbackBody{ChatType: "single", From: callbackFrom{UserID: "USERID"}},
			want: "wecom:dev-1:USERID",
		},
		{
			name: "group without chat id falls back to user id",
			body: callbackBody{ChatType: chatTypeGroup, From: callbackFrom{UserID: "USERID"}},
			want: "wecom:dev-1:USERID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := conversationKey("dev-1", &tt.body); got != tt.want {
				t.Errorf("conversationKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		maxBytes int
		want     string
	}{
		{"short strings untouched", "你好", 10, "你好"},
		{"cuts on rune boundary", "你好世界", 5, "你"},
		{"exact boundary kept", "你好世界", 6, "你好"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateUTF8(tt.in, tt.maxBytes)
			if got != tt.want {
				t.Errorf("truncateUTF8(%q, %d) = %q, want %q", tt.in, tt.maxBytes, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("truncateUTF8 produced invalid UTF-8: %q", got)
			}
		})
	}
}
