package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// SubjectResolver 是 billing.SubjectResolver 在 manager 进程内的实现：
// device → voicebot → owner，以及 voicebot 配置 → provider/model/voice 快照。
//
// 计费不 import store，脏活都在这儿（§19）。哪天计费拆成独立服务，换一次内部
// API 调用即可，领域代码一行不动。
type SubjectResolver struct {
	devices   *store.DeviceStore
	voicebots *store.VoicebotStore
	models    *store.AIModelStore
	voices    *store.ModelVoiceStore
}

// NewSubjectResolver 组装计费的主体解析器。
func NewSubjectResolver(
	devices *store.DeviceStore,
	voicebots *store.VoicebotStore,
	models *store.AIModelStore,
	voices *store.ModelVoiceStore,
) *SubjectResolver {
	return &SubjectResolver{devices: devices, voicebots: voicebots, models: models, voices: voices}
}

// ResolveAccount 按 device 反查计费主体。设备或 voicebot 不存在时返回空主体
// （调用方据此拒绝会话，原因 no_account）。
func (r *SubjectResolver) ResolveAccount(_ context.Context, deviceID string) (string, string, error) {
	voicebot, err := r.voicebotOf(deviceID)
	if err != nil {
		return "", "", err
	}
	if voicebot == nil || voicebot.OwnerID == "" {
		return "", "", nil
	}
	return billing.SubjectTypeUser, voicebot.OwnerID, nil
}

// ResolveSessionProfile 返回这次会话的资源快照，用于价格匹配。
//
// 只认新式 voicebot 配置（asr/tts/llm 段里的 model_id / voice_id）；旧式整份
// AppConfig 里只有厂商 model 名、没有内部 ID，此时快照为空，价格匹配退到
// item 级兜底价。
func (r *SubjectResolver) ResolveSessionProfile(_ context.Context, deviceID string) (billing.SessionProfile, error) {
	voicebot, err := r.voicebotOf(deviceID)
	if err != nil {
		return billing.SessionProfile{}, err
	}
	if voicebot == nil {
		return billing.SessionProfile{}, nil
	}

	cfg := parseVoicebotConfig(voicebot.ConfigJSON)
	profile := billing.SessionProfile{
		VoicebotID: voicebot.ID,
		OwnerID:    voicebot.OwnerID,
		LLM:        r.modelRef(cfg.LLM.ModelID),
		ASR:        r.modelRef(cfg.ASR.ModelID),
		TTS:        r.ttsRef(cfg.TTS.VoiceID, cfg.TTS.ModelID),
	}
	if profile.LLM.IsZero() && profile.ASR.IsZero() && profile.TTS.IsZero() {
		logging.Warnf("billing: voicebot %s has no model/voice IDs in config; prices will fall back to item level", voicebot.ID)
	}
	return profile, nil
}

func (r *SubjectResolver) voicebotOf(deviceID string) (*store.Voicebot, error) {
	if deviceID == "" {
		return nil, nil
	}
	device, err := r.devices.GetByID(deviceID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	voicebot, err := r.voicebots.GetByID(device.VoicebotID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return voicebot, nil
}

// modelRef 把内部 model ID 解析成资源链。System 取 provider 的 IsSystem：真正
// 付给厂商的是 provider 那把 key，所以“平台代付还是 BYOK”由它决定（§7）。
func (r *SubjectResolver) modelRef(modelID string) billing.ResourceRef {
	if modelID == "" || r.models == nil {
		return billing.ResourceRef{}
	}
	m, err := r.models.GetByID(modelID)
	if err != nil {
		logging.Warnf("billing: resolve model %s: %v", modelID, err)
		return billing.ResourceRef{}
	}
	ref := billing.ResourceRef{ModelID: m.ID, System: m.IsSystem}
	if m.Provider != nil {
		ref.ProviderID = m.Provider.ID
		ref.System = m.Provider.IsSystem
	}
	return ref
}

// ttsRef 解析音色链：voice 已隐含 model，model 已隐含 provider。
func (r *SubjectResolver) ttsRef(voiceID, modelID string) billing.ResourceRef {
	if voiceID == "" {
		return r.modelRef(modelID)
	}
	if r.voices == nil {
		return billing.ResourceRef{}
	}
	voice, err := r.voices.GetByID(voiceID)
	if err != nil {
		logging.Warnf("billing: resolve voice %s: %v", voiceID, err)
		return r.modelRef(modelID)
	}
	ref := r.modelRef(voice.ModelID)
	ref.VoiceID = voice.ID
	return ref
}

// voicebotConfig 是 store 里 AgentConfig 形态的最小投影。
type voicebotConfig struct {
	ASR struct {
		ModelID string `json:"model_id"`
	} `json:"asr"`
	TTS struct {
		ModelID string `json:"model_id"`
		VoiceID string `json:"voice_id"`
	} `json:"tts"`
	LLM struct {
		ModelID string `json:"model_id"`
	} `json:"llm"`
}

func parseVoicebotConfig(configJSON string) voicebotConfig {
	var cfg voicebotConfig
	if configJSON == "" {
		return cfg
	}
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return voicebotConfig{}
	}
	return cfg
}
