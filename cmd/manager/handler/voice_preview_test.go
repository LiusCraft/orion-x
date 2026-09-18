package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/liuscraft/orion-x/internal/assets"
	"github.com/liuscraft/orion-x/internal/store"
)

func TestVoicePreviewURLs(t *testing.T) {
	assetSvc := &fakeVoiceCloneAssets{
		asset:      voiceSampleAsset("asset-1", "sample.wav"),
		presignURL: "https://storage.test/sample.wav?signature=stub",
	}
	cloned := store.ModelVoice{ID: "voice-1", SourceAssetID: "asset-1", IsCloned: true}
	stored := store.ModelVoice{ID: "voice-2", PreviewURL: "https://cdn.test/preview.mp3"}
	manual := store.ModelVoice{ID: "voice-3"}

	got := voicePreviewURLs(context.Background(), assetSvc, "user-1", []store.ModelVoice{cloned, stored, manual})

	if len(got) != 1 {
		t.Fatalf("previews = %#v, want only the cloned voice", got)
	}
	if got[cloned.ID] != assetSvc.presignURL {
		t.Fatalf("preview for cloned voice = %q, want %q", got[cloned.ID], assetSvc.presignURL)
	}
	if assetSvc.gotOwner != "user-1" || assetSvc.gotAssetID != "asset-1" {
		t.Fatalf("asset lookup = (%q, %q), want (user-1, asset-1)", assetSvc.gotOwner, assetSvc.gotAssetID)
	}
	if assetSvc.gotTTL != voicePreviewTTL {
		t.Fatalf("presign ttl = %v, want %v", assetSvc.gotTTL, voicePreviewTTL)
	}
}

func TestVoicePreviewURLsSkipsFailures(t *testing.T) {
	cases := []struct {
		name   string
		svc    voiceAssetStore
		voices []store.ModelVoice
	}{
		{
			name:   "no object storage",
			svc:    nil,
			voices: []store.ModelVoice{{ID: "voice-1", SourceAssetID: "asset-1"}},
		},
		{
			name:   "asset missing",
			svc:    &fakeVoiceCloneAssets{getErr: assets.ErrNotFound},
			voices: []store.ModelVoice{{ID: "voice-1", SourceAssetID: "asset-1"}},
		},
		{
			name:   "presign failure",
			svc:    &fakeVoiceCloneAssets{asset: voiceSampleAsset("asset-1", "sample.wav"), presignErr: assets.ErrStorage},
			voices: []store.ModelVoice{{ID: "voice-1", SourceAssetID: "asset-1"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := voicePreviewURLs(context.Background(), tc.svc, "user-1", tc.voices); len(got) != 0 {
				t.Fatalf("previews = %#v, want empty (listing must not fail)", got)
			}
		})
	}
}

// TestWithPreviewsOverridesStoredURL 复刻音色的试听地址只在响应时现算，
// 序列化时必须覆盖库里可能为空的 preview_url。
func TestWithPreviewsOverridesStoredURL(t *testing.T) {
	list := []store.ModelVoice{
		{ID: "voice-1", Name: "cloned", SourceAssetID: "asset-1"},
		{ID: "voice-2", Name: "system", PreviewURL: "https://cdn.test/preview.mp3"},
	}

	raw, err := json.Marshal(withPreviews(list, map[string]string{"voice-1": "https://storage.test/signed"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got []struct {
		ID         string `json:"id"`
		PreviewURL string `json:"preview_url"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("decoded %d voices, want 2", len(got))
	}
	if got[0].PreviewURL != "https://storage.test/signed" {
		t.Errorf("cloned preview = %q, want resolved presigned URL", got[0].PreviewURL)
	}
	if got[1].PreviewURL != "https://cdn.test/preview.mp3" {
		t.Errorf("system preview = %q, want stored URL", got[1].PreviewURL)
	}
}
