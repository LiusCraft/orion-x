// Login / register page copy: brand pitch, form, OAuth, and verification notices.
export default {
	hero: {
		title: "Give your devices voice intelligence",
		desc: "Open-source AI voice bot framework with multi-device management, pluggable LLMs, and low-latency streaming voice.",
	},
	features: {
		multiSession: {
			title: "Multi-device sessions",
			desc: "Independent sessions per device, no interference",
		},
		llm: {
			title: "Flexible LLM integration",
			desc: "OpenAI-compatible APIs, switch models freely",
		},
		streaming: {
			title: "Real-time streaming",
			desc: "VAD + streaming ASR/TTS for ultra-low latency",
		},
		mcp: {
			title: "MCP tooling",
			desc: "Connect external tools and services via MCP",
		},
	},
	title: {
		login: "Welcome back",
		register: "Create account",
	},
	subtitle: {
		login: "Sign in to the management console",
		register: "Register a management console account",
	},
	email: "Email",
	username: "Nickname (optional)",
	usernamePlaceholder: "Your nickname",
	password: "Password",
	showPassword: "Show password",
	hidePassword: "Hide password",
	action: {
		login: "Sign in",
		register: "Sign up",
		loggingIn: "Signing in...",
		registering: "Signing up...",
		resendVerification: "Resend verification email",
		resending: "Sending...",
	},
	or: "or",
	oauthLogin: "Sign in with {{name}}",
	noAccount: "Don't have an account?",
	haveAccount: "Already have an account?",
	notice: {
		emailVerified: "Email verified. Please sign in.",
		verifyLinkInvalid: "The verification link is invalid or has expired",
		verificationSent: "Verification email sent. Please check your inbox.",
		verificationResent:
			"Verification email sent. Please check your inbox (no resend within 1 minute).",
		emailUnverified: "Email not verified. Please verify your email first.",
	},
	error: {
		loginFailed: "Incorrect email or password",
		registerFailed: "Registration failed",
		resendFailed: "Failed to send, please try again later",
	},
};
