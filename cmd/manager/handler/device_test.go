package handler

import (
	"strings"
	"testing"

	"github.com/liuscraft/orion-x/internal/store"
)

func TestNewDeviceResponseMasksChannelSecrets(t *testing.T) {
	token := "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	response := newDeviceResponse(&store.Device{ID: "device-1"}, []store.DeviceChannel{
		{DeviceID: "device-1", Platform: "telegram", Config: map[string]any{"bot_token": token}},
	})

	if len(response.Channels) != 1 {
		t.Fatalf("channels = %+v, want one entry", response.Channels)
	}
	got := response.Channels[0]
	if !got.Enabled || got.Platform != "telegram" {
		t.Fatalf("channel status = %+v, want enabled telegram", got)
	}
	hint := got.Config["bot_token"]
	if hint == token || strings.Contains(hint, "ABCDEFGHI") {
		t.Fatalf("config hint exposes the token: %q", hint)
	}
}

func TestNewDeviceResponseHidesNoChannel(t *testing.T) {
	response := newDeviceResponse(&store.Device{ID: "device-1"}, nil)
	if len(response.Channels) != 0 {
		t.Fatalf("channels = %+v, want empty", response.Channels)
	}
}

// 平台已下线时，历史遗留的通道行不能把配置回显出去。
func TestNewDeviceChannelStatusesSkipsUnknownPlatform(t *testing.T) {
	statuses := newDeviceChannelStatuses([]store.DeviceChannel{
		{DeviceID: "device-1", Platform: "retired-platform", Config: map[string]any{"token": "secret"}},
	})
	if len(statuses) != 0 {
		t.Fatalf("statuses = %+v, want unknown platform skipped", statuses)
	}
}
