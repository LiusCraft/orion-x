package aliyun

import (
	"github.com/liuscraft/orion-x/internal/language"
	tts "github.com/liuscraft/orion-x/internal/provider/tts"
)

func init() {
	tts.Register(tts.TypeAliyun, func(cfg tts.Config) (tts.Synthesizer, error) {
		return NewDashScopeProvider(cfg)
	}, tts.ProviderMeta{
		Name:           "阿里云 Dashscope",
		Description:    "阿里云 CosyVoice 语音合成服务，支持流式合成与句边界标记。",
		DefaultBaseURL: defaultDashScopeEndpoint,
		Models: map[string]tts.ModelInfo{
			"cosyvoice-v3-flash": {
				SupportedLanguages: []language.Code{language.ZH, language.EN},
				SystemVoices:       voicesV3Flash,
			},
		},
		Features: []tts.Feature{
			tts.FeatureStreaming,
			tts.FeatureSSML,
			tts.FeatureEmotion,
			tts.FeatureWarmup,
		},
	})
}

// voicesV3Flash 是 cosyvoice-v3-flash 模型的所有系统音色，来源：
// https://help.aliyun.com/zh/model-studio/cosyvoice-voice-list
var voicesV3Flash = func() []tts.VoiceInfo {
	zh, en := language.ZH, language.EN
	emotions := []string{"neutral", "fearful", "angry", "sad", "surprised", "happy", "disgusted"}

	v := func(id, name, gender string, langs []language.Code, tags []string, emotions []string) tts.VoiceInfo {
		return tts.VoiceInfo{VoiceID: id, Name: name, Gender: gender, Languages: langs, Tags: tags, Emotions: emotions}
	}

	return []tts.VoiceInfo{
		// ── 社交陪伴（标杆音色）──
		v("longanyang", "龙安洋", "male", []language.Code{zh, en}, []string{"播报", "社交"}, emotions),
		v("longanhuan_v3", "龙安欢(V3)", "female", []language.Code{zh, en}, []string{"播报", "社交", "方言"}, nil),
		v("longanhuan", "龙安欢", "female", []language.Code{zh, en}, []string{"播报", "社交"}, emotions),

		// ── 童声（标杆音色）──
		v("longhuhu_v3", "龙呼呼", "female", []language.Code{zh, en}, []string{"童声", "故事"}, emotions),

		// ── 智能玩具 / 儿童故事机 ──
		v("longpaopao_v3", "龙泡泡", "female", []language.Code{zh, en}, []string{"童声", "故事"}, nil),
		v("longjielidou_v3", "龙杰力豆", "male", []language.Code{zh, en}, []string{"童声", "故事"}, nil),
		v("longxian_v3", "龙仙", "female", []language.Code{zh, en}, []string{"童声"}, nil),
		v("longling_v3", "龙铃", "female", []language.Code{zh, en}, []string{"童声"}, nil),

		// ── 消费电子 - 儿童有声书 ──
		v("longshanshan_v3", "龙闪闪", "female", []language.Code{zh, en}, []string{"童声", "故事"}, nil),
		v("longniuniu_v3", "龙牛牛", "male", []language.Code{zh, en}, []string{"童声", "故事"}, nil),

		// ── 方言 ──
		v("longjiaxin_v3", "龙嘉欣", "female", []language.Code{zh, en}, []string{"粤语"}, nil),
		v("longjiayi_v3", "龙嘉怡", "female", []language.Code{zh, en}, []string{"粤语"}, nil),
		v("longanyue_v3", "龙安粤", "male", []language.Code{zh, en}, []string{"粤语"}, nil),
		v("longlaotie_v3", "龙老铁", "male", []language.Code{zh, en}, []string{"东北话"}, nil),
		v("longshange_v3", "龙陕哥", "male", []language.Code{zh, en}, []string{"陕西话"}, nil),
		v("longanmin_v3", "龙安闽", "female", []language.Code{zh, en}, []string{"闽南话"}, nil),

		// ── 诗词朗诵 ──
		v("longfei_v3", "龙飞", "male", []language.Code{zh, en}, []string{"朗诵", "播报"}, nil),

		// ── 电话销售 ──
		v("longyingxiao_v3", "龙应笑", "female", []language.Code{zh, en}, []string{"销售"}, nil),

		// ── 客服 ──
		v("longyingxun_v3", "龙应询", "male", []language.Code{zh, en}, []string{"客服"}, nil),
		v("longyingjing_v3", "龙应静", "female", []language.Code{zh, en}, []string{"客服"}, nil),
		v("longyingling_v3", "龙应聆", "female", []language.Code{zh, en}, []string{"客服"}, nil),
		v("longyingtao_v3", "龙应桃", "female", []language.Code{zh, en}, []string{"客服"}, nil),

		// ── 语音助手 ──
		v("longxiaochun_v3", "龙小淳", "female", []language.Code{zh, en}, []string{"助手"}, nil),
		v("longxiaoxia_v3", "龙小夏", "female", []language.Code{zh, en}, []string{"助手"}, nil),
		v("longyumi_v3", "YUMI", "female", []language.Code{zh, en}, []string{"助手"}, nil),
		v("longanyun_v3", "龙安昀", "male", []language.Code{zh, en}, []string{"助手"}, nil),
		v("longanwen_v3", "龙安温", "female", []language.Code{zh, en}, []string{"助手"}, nil),
		v("longanli_v3", "龙安莉", "female", []language.Code{zh, en}, []string{"助手"}, nil),
		v("longanlang_v3", "龙安朗", "male", []language.Code{zh, en}, []string{"助手"}, nil),

		// ── 出海营销：韩语 ──
		v("loongkyong_v3", "Loongkyong", "female", []language.Code{language.KO}, []string{"海外"}, nil),
		v("loongjihun_v3", "Jihun", "male", []language.Code{language.KO}, []string{"海外"}, nil),

		// ── 出海营销：日语 ──
		v("loongriko_v3", "Riko", "female", []language.Code{language.JA}, []string{"海外"}, nil),
		v("loongtomoka_v3", "Loongtomoka", "female", []language.Code{language.JA}, []string{"海外"}, nil),
		v("loongtomoya_v3", "Loongtomoya", "male", []language.Code{language.JA}, []string{"海外"}, nil),
		v("loongyuuna_v3", "Yuuna", "female", []language.Code{language.JA}, []string{"海外"}, nil),
		v("loongyuuma_v3", "Yuuma", "male", []language.Code{language.JA}, []string{"海外"}, nil),

		// ── 出海营销：英语 ──
		v("loongabby_v3", "Loongabby", "female", []language.Code{en}, []string{"海外", "美式"}, nil),
		v("loongandy_v3", "Loongandy", "male", []language.Code{en}, []string{"海外", "美式"}, nil),
		v("loongannie_v3", "Loongannie", "female", []language.Code{en}, []string{"海外", "美式"}, nil),
		v("loongava_v3", "Loongava", "female", []language.Code{en}, []string{"海外", "美式"}, nil),
		v("loongbeth_v3", "Loongbeth", "female", []language.Code{en}, []string{"海外", "美式"}, nil),
		v("loongbetty_v3", "Loongbetty", "female", []language.Code{en}, []string{"海外", "美式"}, nil),
		v("loongcally_v3", "Loongcally", "female", []language.Code{en}, []string{"海外", "美式"}, nil),
		v("loongcindy_v3", "Loongcindy", "female", []language.Code{en}, []string{"海外", "美式"}, nil),
		v("loongdavid_v3", "Loongdavid", "male", []language.Code{en}, []string{"海外", "美式"}, nil),
		v("loongdonna_v3", "Loongdonna", "female", []language.Code{en}, []string{"海外", "美式"}, nil),
		v("loongemily_v3", "Loongemily", "female", []language.Code{en}, []string{"海外", "英式"}, nil),
		v("loongeric_v3", "Loongeric", "male", []language.Code{en}, []string{"海外", "英式"}, nil),
		v("loongluna_v3", "Loongluna", "female", []language.Code{en}, []string{"海外", "英式"}, nil),
		v("loongluca_v3", "Loongluca", "male", []language.Code{en}, []string{"海外", "英式"}, nil),

		// ── 出海营销：印尼语 ──
		v("loongindah_v3", "Loongindah", "female", []language.Code{language.ID}, []string{"海外"}, nil),
	}
}()
