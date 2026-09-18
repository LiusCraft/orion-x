package handler

import (
	"context"
	"time"

	"github.com/liuscraft/orion-x/internal/store"
)

// voicePreviewTTL 是复刻音色试听地址的有效期。试听地址在列表响应时现算，
// 页面可能停留较久，故比资源库默认的 presign_ttl 留更长窗口。
const voicePreviewTTL = 2 * time.Hour

// voiceAssetStore 是资源库的只读访问（Get + 预签名），nil 表示未配置对象存储。
type voiceAssetStore interface {
	Get(ctx context.Context, ownerID, assetID string) (*store.Asset, error)
	PresignURLWithTTL(ctx context.Context, a *store.Asset, ttl time.Duration, download bool) (string, error)
}

// voicePreviewURLs 给复刻音色补上试听地址：直接复用上传的参考音频
// （source_asset_id 指向的 voice_sample 资源）现算预签名 URL。
//
// 预签名是本地计算且 URL 会过期，因此只在响应时计算、不写回数据库；
// 单个音色取件失败只跳过该条，不影响整个列表。
func voicePreviewURLs(ctx context.Context, svc voiceAssetStore, ownerID string, list []store.ModelVoice) map[string]string {
	if svc == nil {
		return nil
	}

	out := make(map[string]string)
	for _, v := range list {
		if v.PreviewURL != "" || v.SourceAssetID == "" {
			continue
		}
		asset, err := svc.Get(ctx, ownerID, v.SourceAssetID)
		if err != nil {
			continue
		}
		url, err := svc.PresignURLWithTTL(ctx, asset, voicePreviewTTL, false)
		if err != nil {
			continue
		}
		out[v.ID] = url
	}
	return out
}

// voiceWithPreview 是音色列表响应项。复刻音色的试听地址不在库里，只在响应时
// 现算，因此用外层同名字段覆盖 ModelVoice 的 preview_url。
type voiceWithPreview struct {
	store.ModelVoice
	PreviewURL string `json:"preview_url,omitempty"`
}

// withPreviews 把现算的试听地址合并进音色列表。
func withPreviews(list []store.ModelVoice, previews map[string]string) []voiceWithPreview {
	out := make([]voiceWithPreview, 0, len(list))
	for _, v := range list {
		preview := v.PreviewURL
		if url, ok := previews[v.ID]; ok {
			preview = url
		}
		out = append(out, voiceWithPreview{ModelVoice: v, PreviewURL: preview})
	}
	return out
}
