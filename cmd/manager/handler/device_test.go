package handler

import (
	"strings"
	"testing"

	"github.com/liuscraft/orion-x/internal/store"
)

func TestNewDeviceResponseMasksTelegramToken(t *testing.T) {
	token := "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	response := newDeviceResponse(&store.Device{ID: "device-1", TgBotToken: token})

	if !response.Telegram.Enabled {
		t.Fatal("Telegram channel should be enabled")
	}
	if response.Telegram.TokenHint == token || strings.Contains(response.Telegram.TokenHint, "ABCDEFGHI") {
		t.Fatalf("token hint exposes the token: %q", response.Telegram.TokenHint)
	}
}

func TestNewDeviceResponseHidesDisabledTelegramChannel(t *testing.T) {
	response := newDeviceResponse(&store.Device{ID: "device-1"})
	if response.Telegram.Enabled || response.Telegram.TokenHint != "" {
		t.Fatalf("unexpected disabled channel status: %+v", response.Telegram)
	}
}
