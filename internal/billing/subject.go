package billing

import "context"

// SessionProfile 是一次会话的资源快照：provider / model / voice 三条链，用于
// 价格匹配。会话的 pipeline 是连接时按当时加载的配置建的，之后管理员改了
// voicebot 的模型配置，并不影响这个会话实际在调谁——所以这份快照以 authorize
// 时的那一份为准。
type SessionProfile struct {
	VoicebotID string      `json:"voicebot_id,omitempty"`
	OwnerID    string      `json:"owner_id,omitempty"`
	LLM        ResourceRef `json:"llm,omitempty"`
	TTS        ResourceRef `json:"tts,omitempty"`
	ASR        ResourceRef `json:"asr,omitempty"`
}

// RefFor 按计量点返回对应的资源链。
func (p SessionProfile) RefFor(meterSource string) ResourceRef {
	switch meterSource {
	case MeterSourceLLM:
		return p.LLM
	case MeterSourceTTS:
		return p.TTS
	case MeterSourceASR:
		return p.ASR
	default:
		return ResourceRef{}
	}
}

// SubjectResolver 是计费向业务追问“这个 device 属于谁、用的是哪个模型”的窄接
// 口。由计费的宿主（manager）实现，计费的领域层不 import store。
type SubjectResolver interface {
	// ResolveAccount 返回 device 对应的计费主体。
	ResolveAccount(ctx context.Context, deviceID string) (subjectType, subjectID string, err error)
	// ResolveSessionProfile 返回用于价格匹配的资源快照。
	ResolveSessionProfile(ctx context.Context, deviceID string) (SessionProfile, error)
}
