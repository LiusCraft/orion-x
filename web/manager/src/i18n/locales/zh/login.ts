// 登录 / 注册页文案：品牌标语、表单、第三方登录与验证邮件提示。
export default {
	hero: {
		title: "为你的设备注入语音智能",
		desc: "开源 AI 语音机器人框架，支持多设备管理、可插拔 LLM 和低延迟流式语音对话。",
	},
	features: {
		multiSession: {
			title: "多路语音会话",
			desc: "每个设备独立会话，互不干扰",
		},
		llm: {
			title: "灵活 LLM 接入",
			desc: "支持 OpenAI 兼容接口，自由切换模型",
		},
		streaming: {
			title: "实时流式响应",
			desc: "VAD 检测 + 流式 ASR/TTS，延迟极低",
		},
		mcp: {
			title: "MCP 工具扩展",
			desc: "通过 MCP 协议接入外部工具与服务",
		},
	},
	title: {
		login: "欢迎回来",
		register: "创建账号",
	},
	subtitle: {
		login: "登录到管理控制台",
		register: "注册一个管理控制台账号",
	},
	email: "邮箱",
	username: "昵称（选填）",
	usernamePlaceholder: "你的昵称",
	password: "密码",
	showPassword: "显示密码",
	hidePassword: "隐藏密码",
	action: {
		login: "登录",
		register: "注册",
		loggingIn: "登录中...",
		registering: "注册中...",
		resendVerification: "重发验证邮件",
		resending: "发送中...",
	},
	or: "或",
	oauthLogin: "使用 {{name}} 登录",
	noAccount: "还没有账号？",
	haveAccount: "已有账号？",
	notice: {
		emailVerified: "邮箱验证成功，请登录",
		verifyLinkInvalid: "验证链接无效或已过期",
		verificationSent: "验证邮件已发送，请查收",
		verificationResent: "验证邮件已发送，请查收（1 分钟内不会重复发送）",
		emailUnverified: "邮箱未验证，请先完成邮箱验证",
	},
	error: {
		loginFailed: "邮箱或密码错误",
		registerFailed: "注册失败",
		resendFailed: "发送失败，请稍后重试",
	},
};
