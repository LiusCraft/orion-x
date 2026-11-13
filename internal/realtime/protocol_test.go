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

func TestParseClientEventPreservesTurnDetectionNull(t *testing.T) {
	event, err := ParseClientEvent([]byte(`{"type":"session.update","session":{"audio":{"input":{"turn_detection":null}}}}`))
	if err != nil {
		t.Fatalf("ParseClientEvent() error = %v", err)
	}
	input := event.Session.Audio.Input
	if !input.TurnDetectionSpecified() {
		t.Fatal("TurnDetectionSpecified() = false, want explicit null to be preserved")
	}
	if input.TurnDetection != nil {
		t.Fatalf("TurnDetection = %#v, want nil", input.TurnDetection)
	}
}

func TestParseClientEventDistinguishesOmittedTurnDetection(t *testing.T) {
	event, err := ParseClientEvent([]byte(`{"type":"session.update","session":{"audio":{"input":{}}}}`))
	if err != nil {
		t.Fatalf("ParseClientEvent() error = %v", err)
	}
	if event.Session.Audio.Input.TurnDetectionSpecified() {
		t.Fatal("TurnDetectionSpecified() = true, want omitted field to remain unspecified")
	}
}
