package realtime

import "testing"

func TestParseClientEvent(t *testing.T) {
	event, err := ParseClientEvent([]byte(`{"type":"conversation.item.create","item":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`))
	if err != nil {
		t.Fatalf("ParseClientEvent() error = %v", err)
	}
	if event.Type != ClientConversationItemCreate {
		t.Errorf("event.Type = %q, want %q", event.Type, ClientConversationItemCreate)
	}
	if event.Item == nil || len(event.Item.Content) != 1 || event.Item.Content[0].Text != "hello" {
		t.Fatalf("event.Item = %#v, want one text content part", event.Item)
	}
}

func TestParseClientEventRequiresType(t *testing.T) {
	if _, err := ParseClientEvent([]byte(`{"event_id":"evt_1"}`)); err == nil {
		t.Fatal("ParseClientEvent() error = nil, want required type error")
	}
}
