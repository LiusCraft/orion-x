import axios from "axios";

const http = axios.create({ baseURL: "/api" });

http.interceptors.request.use((config) => {
	const token = localStorage.getItem("token");
	if (token) config.headers.Authorization = `Bearer ${token}`;
	return config;
});

http.interceptors.response.use(
	(r) => r,
	(err) => {
		if (err.response?.status === 401) {
			localStorage.removeItem("token");
			window.location.href = "/login";
		}
		return Promise.reject(err);
	},
);

export interface Voicebot {
	id: string;
	name: string;
	owner_id: string;
	config_json: string;
	created_at: string;
	updated_at: string;
	creator: string;
}

export interface Device {
	id: string;
	voicebot_id: string;
	name: string;
	created_at: string;
	creator: string;
	telegram: {
		enabled: boolean;
		token_hint?: string;
	};
}

export interface OAuthBinding {
	provider: string;
	provider_uid: string;
}

export interface OAuthProvider {
	provider: string;
	name: string;
}

export const authApi = {
	login: (email: string, password: string) =>
		http.post<{ token: string; user_id: string; email: string; username: string; is_admin: boolean }>(
			"/auth/login",
			{ email, password },
		),
	register: (email: string, password: string, username?: string) =>
		http.post<{ token: string; user_id: string; email: string; username: string; is_admin: boolean }>(
			"/auth/register",
			{ email, password, username },
		),
	oauthProviders: () =>
		http.get<{ providers: OAuthProvider[] }>("/auth/oauth/providers"),
	oauthLoginUrl: (provider: string) => {
		const from = encodeURIComponent(window.location.origin);
		return `/api/auth/oauth/${provider}/login?from=${from}`;
	},
	unbindOAuth: (provider: string) =>
		http.post<{ message: string }>(`/auth/oauth/${provider}/unbind`),
	changePassword: (oldPassword: string, newPassword: string) =>
		http.post<{ message: string }>("/auth/change-password", {
			old_password: oldPassword,
			new_password: newPassword,
		}),
	bindEmail: (email: string) =>
		http.post<{ message: string; email: string }>("/auth/bind-email", {
			email,
		}),
	profile: () =>
		http.get<{
			user_id: string;
			email: string;
			username: string;
			is_admin: boolean;
			has_password: boolean;
			bindings: OAuthBinding[];
		}>("/auth/profile"),
};

export const voicebotApi = {
	list: () => http.get<Voicebot[]>("/voicebots"),
	get: (id: string) => http.get<Voicebot>(`/voicebots/${id}`),
	create: (name: string, config_json?: string) =>
		http.post<Voicebot>("/voicebots", {
			name,
			config_json: config_json ? JSON.parse(config_json) : undefined,
		}),
	update: (id: string, name: string, config_json: string) =>
		http.put<Voicebot>(`/voicebots/${id}`, {
			name,
			config_json: JSON.parse(config_json),
		}),
	remove: (id: string) => http.delete(`/voicebots/${id}`),
};

export const deviceApi = {
	list: (voicebotId: string) =>
		http.get<Device[]>(`/voicebots/${voicebotId}/devices`),
	create: (voicebotId: string, id: string, name: string) =>
		http.post<Device>(`/voicebots/${voicebotId}/devices`, { id, name }),
	remove: (voicebotId: string, deviceId: string) =>
		http.delete(`/voicebots/${voicebotId}/devices/${deviceId}`),
	setTelegram: (voicebotId: string, deviceId: string, botToken: string) =>
		http.put<Device>(
			`/voicebots/${voicebotId}/devices/${deviceId}/channels/telegram`,
			{ bot_token: botToken },
		),
	clearTelegram: (voicebotId: string, deviceId: string) =>
		http.delete<Device>(
			`/voicebots/${voicebotId}/devices/${deviceId}/channels/telegram`,
		),
};

export interface Provider {
	id: string;
	name: string;
	slug: string;
	base_url: string;
	is_system: boolean;
	extra?: Record<string, unknown>;
	created_at: string;
	creator: string;
}

export type ModelType =
	| "text"
	| "vision"
	| "speech"
	| "multimodal"
	| "embedding";

export interface AIModel {
	id: string;
	provider_id: string;
	provider?: Provider;
	name: string;
	type: ModelType;
	base_url: string;
	model_id: string;
	is_system: boolean;
	langs?: string[];
	extra?: Record<string, unknown>;
	created_at: string;
	creator: string;
}

export interface ProviderSlug {
	slug: string;
	category: string;
	name: string;
	base_url: string;
}

export const providerApi = {
	list: () => http.get<Provider[]>("/providers"),
	slugs: () => http.get<ProviderSlug[]>("/providers/slugs"),
	get: (id: string) => http.get<Provider>(`/providers/${id}`),
	create: (data: {
		name: string;
		slug: string;
		base_url: string;
		api_key?: string;
		extra?: Record<string, unknown>;
	}) => http.post<Provider>("/providers", data),
	update: (
		id: string,
		data: {
			name?: string;
			base_url?: string;
			api_key?: string;
			extra?: Record<string, unknown>;
		},
	) => http.put<Provider>(`/providers/${id}`, data),
	remove: (id: string) => http.delete(`/providers/${id}`),
};

export const modelApi = {
	list: (type?: ModelType) =>
		http.get<AIModel[]>("/models", { params: type ? { type } : undefined }),
	types: () => http.get<ModelType[]>("/models/types"),
	get: (id: string) => http.get<AIModel>(`/models/${id}`),
	create: (data: {
		provider_id: string;
		name: string;
		type: ModelType;
		base_url?: string;
		model_id: string;
		extra?: Record<string, unknown>;
	}) => http.post<AIModel>("/models", data),
	update: (
		id: string,
		data: {
			name?: string;
			base_url?: string;
			model_id?: string;
			extra?: Record<string, unknown>;
		},
	) => http.put<AIModel>(`/models/${id}`, data),
	remove: (id: string) => http.delete(`/models/${id}`),
	voices: (modelId: string, lang?: string) =>
		http.get<ModelVoice[]>(`/models/${modelId}/voices`, {
			params: lang ? { lang } : undefined,
		}),
};

export interface ModelVoice {
	id: string;
	model_id: string;
	voice_id: string;
	name: string;
	description?: string;
	gender?: "male" | "female" | "neutral";
	avatar_url?: string;
	preview_url?: string;
	tags?: string[];
	langs?: string[];
	emotions?: Record<string, unknown>;
	is_system: boolean;
	is_cloned: boolean;
	source_asset_id?: string;
	source_audio_url?: string;
	extra?: Record<string, unknown>;
	created_at: string;
	updated_at: string;
	creator: string;
}

export interface Language {
	code: string;
	name: string;
	parent_code?: string;
	children?: Language[];
}

export const languageApi = {
	list: (parentCode?: string) =>
		http.get<Language[]>("/languages", {
			params: parentCode ? { parent_code: parentCode } : undefined,
		}),
};

export const voiceApi = {
	listSystem: (lang?: string) =>
		http.get<ModelVoice[]>("/voices/system", {
			params: lang ? { lang } : undefined,
		}),
	// 当前用户自建（含复刻）的音色
	listMine: (lang?: string) =>
		http.get<ModelVoice[]>("/voices/mine", {
			params: lang ? { lang } : undefined,
		}),
	remove: (modelId: string, voiceId: string) =>
		http.delete(`/models/${modelId}/voices/${voiceId}`),
};

export type AssetPurpose = "image" | "voice:sample" | "kb:document";

export interface Asset {
	id: string;
	owner_id: string;
	purpose: AssetPurpose;
	name: string;
	mime_type: string;
	size: number;
	url?: string;
	created_at: string;
	updated_at: string;
	creator: string;
}

export const assetsApi = {
	upload: (
		file: File,
		purpose: AssetPurpose,
		onProgress?: (percent: number) => void,
	) => {
		const form = new FormData();
		form.append("file", file);
		form.append("purpose", purpose);
		return http.post<Asset>("/assets", form, {
			onUploadProgress: (e) => {
				if (onProgress && e.total) {
					onProgress(Math.round((e.loaded / e.total) * 100));
				}
			},
		});
	},
	get: (id: string) => http.get<Asset>(`/assets/${id}`),
};

export interface VoiceCloneModel {
	id: string;
	name: string;
	model_id: string;
	provider_id: string;
	provider_name: string;
	provider_slug: string;
	langs?: string[];
	configured: boolean;
}

export interface VoiceCloneRequest {
	name: string;
	description?: string;
	prefix?: string;
	source_asset_id?: string;
	source_audio_url?: string;
	format?: string;
	langs?: string[];
}

export const voiceCloneApi = {
	models: () => http.get<VoiceCloneModel[]>("/models/voice-cloning"),
	clone: (modelId: string, payload: VoiceCloneRequest) =>
		http.post<ModelVoice>(`/models/${modelId}/voices/clone`, payload),
};

export interface AgentTemplate {
	id: string;
	name: string;
	description?: string;
	icon?: string;
	color?: string;
	category: string;
	tags?: string[];
	config_json: string;
	is_system: boolean;
	use_count: number;
	created_at: string;
	updated_at: string;
	creator: string;
}

export const agentTemplateApi = {
	listSystem: (category?: string, q?: string) =>
		http.get<AgentTemplate[]>("/agent-templates/system", {
			params: { category, q },
		}),
	get: (id: string) =>
		http.get<AgentTemplate>(`/agent-templates/${id}`),
	use: (id: string) =>
		http.post<{ template_id: string; name: string; config: Record<string, unknown> }>(
			`/agent-templates/${id}/use`,
		),
};

export interface ResourceOption {
	id: string;
	name: string;
}

export interface VoiceResource {
	id: string;
	name: string;
	description?: string;
	gender?: string;
	avatar_url?: string;
	preview_url?: string;
	tags?: string[];
	langs?: string[];
	emotions?: Record<string, unknown>;
	is_system: boolean;
	is_cloned: boolean;
	source_audio_url?: string;
}

export interface AvailableResources {
	asr: ResourceOption[];
	voices: VoiceResource[];
}

export interface HeaderMetaItem {
	kind: "required" | "optional" | "auto";
	label?: string;
	description?: string;
	placeholder?: string;
	default?: string;
	value?: string;
}

export interface MCPMarketEntry {
	id: string;
	name: string;
	description?: string;
	icon?: string;
	tags?: string[];
	provider?: string;
	billing?: string;
	price?: string;
	config: Record<string, unknown>;
	header_meta?: Record<string, HeaderMetaItem>;
	created_at: string;
}

export interface MCPServer {
	id: string;
	owner_id?: string;
	market_id?: string;
	name: string;
	description?: string;
	icon?: string;
	tags?: string[];
	transport: "stdio" | "sse" | "streamable";
	command?: string;
	args?: string[];
	env?: Record<string, string>;
	cwd?: string;
	endpoint?: string;
	headers?: Record<string, string>;
	tool_name_list?: string[];
	timeout_ms: number;
	created_at: string;
	creator: string;
}

export interface VoicebotMCPServer extends MCPServer {
	bound: boolean;
	enabled: boolean;
}

export interface PaginatedResponse<T> {
	data: T[];
	total: number;
	page: number;
}

export interface PaginationParams {
	page?: number;
	page_size?: number;
}

export const mcpApi = {
	market: {
		list: (params?: PaginationParams) =>
			http.get<PaginatedResponse<MCPMarketEntry>>("/mcp/market", {
				params,
			}),
	},
	// User-level MCP server CRUD
	servers: {
		list: (params?: PaginationParams) =>
			http.get<PaginatedResponse<MCPServer>>("/mcp/servers", {
				params,
			}),
		get: (id: string) => http.get<MCPServer>(`/mcp/servers/${id}`),
		create: (data: {
			market_id?: string;
			name?: string;
			description?: string;
			transport?: string;
			command?: string;
			args?: string[];
			env?: Record<string, string>;
			cwd?: string;
			endpoint?: string;
			headers?: Record<string, string>;
			tool_name_list?: string[];
			timeout_ms?: number;
		}) => http.post<MCPServer>("/mcp/servers", data),
		update: (id: string, data: Record<string, unknown>) =>
			http.put<MCPServer>(`/mcp/servers/${id}`, data),
		remove: (id: string) => http.delete(`/mcp/servers/${id}`),
	},
	// Voicebot MCP binding
	bindings: {
		list: (voicebotId: string) =>
			http.get<VoicebotMCPServer[]>(`/voicebots/${voicebotId}/mcps`),
		bind: (voicebotId: string, mcpServerId: string) =>
			http.post(`/voicebots/${voicebotId}/mcps`, {
				mcp_server_id: mcpServerId,
			}),
		unbind: (voicebotId: string, mcpServerId: string) =>
			http.delete(`/voicebots/${voicebotId}/mcps/${mcpServerId}`),
		toggle: (voicebotId: string, mcpServerId: string) =>
			http.patch(`/voicebots/${voicebotId}/mcps/${mcpServerId}/toggle`),
	},
	testConnection: (data: {
		transport: string;
		command?: string;
		args?: string[];
		env?: Record<string, string>;
		cwd?: string;
		endpoint?: string;
		headers?: Record<string, string>;
		timeout_ms?: number;
		market_id?: string;
	}) =>
		http.post<{ success: boolean; message: string }>(
			"/mcp/test-connection",
			data,
		),
	listTools: (data: {
		transport: string;
		command?: string;
		args?: string[];
		env?: Record<string, string>;
		cwd?: string;
		endpoint?: string;
		headers?: Record<string, string>;
		timeout_ms?: number;
		market_id?: string;
	}) =>
		http.post<{
			success: boolean;
			message?: string;
			tools?: {
				name: string;
				description: string;
				input_schema?: Record<string, unknown>;
			}[];
		}>("/mcp/list-tools", data),
	callTool: (data: {
		transport: string;
		command?: string;
		args?: string[];
		env?: Record<string, string>;
		cwd?: string;
		endpoint?: string;
		headers?: Record<string, string>;
		timeout_ms?: number;
		market_id?: string;
		tool_name: string;
		arguments?: Record<string, unknown>;
	}) =>
		http.post<{
			success: boolean;
			message?: string;
			is_error?: boolean;
			output?: string;
		}>("/mcp/call-tool", data),
};

export interface MemoryEntry {
	id: string;
	target: "memory" | "user";
	content: string;
	device_id: string;
	agent_name: string;
	created_at: string;
	updated_at: string;
}

export interface DeviceUsage {
	memory: { used: number; limit: number };
	user: { used: number; limit: number };
}

export interface DeviceGroup {
	id: string;
	name: string;
	entries: MemoryEntry[];
	total: number;
	usage: DeviceUsage;
}

export interface AgentGroup {
	id: string;
	name: string;
	devices: DeviceGroup[];
	total: number;
}

export interface AgentItem {
	id: string;
	name: string;
	device_count: number;
	total: number;
}

export interface DeviceItem {
	id: string;
	name: string;
	total: number;
	usage: {
		memory: { used: number; limit: number };
		user: { used: number; limit: number };
	};
}

export interface EntryItem {
	id: string;
	target: "memory" | "user";
	content: string;
	created_at: string;
	updated_at: string;
}

export const memoryApi = {
	listAgents: (params?: { page?: number; page_size?: number; q?: string }) =>
		http.get<PaginatedResponse<AgentItem> & { agents: AgentItem[] }>(
			"/data/memory/agents",
			{ params },
		),
	listDevices: (
		agentId: string,
		params?: { page?: number; page_size?: number; q?: string; target?: string },
	) =>
		http.get<PaginatedResponse<DeviceItem> & { devices: DeviceItem[] }>(
			`/data/memory/agents/${agentId}/devices`,
			{ params },
		),
	listEntries: (
		deviceId: string,
		params?: { page?: number; page_size?: number; q?: string; target?: string },
	) =>
		http.get<PaginatedResponse<EntryItem> & { entries: EntryItem[] }>(
			`/data/memory/devices/${deviceId}/entries`,
			{ params },
		),
	remove: (id: string) => http.delete(`/data/memory/${id}`),
};

export interface KnowledgeBase {
	id: string;
	voicebot_id: string;
	name: string;
	description?: string;
	embedding_model_id: string;
	embedding_dim: number;
	created_at: string;
	updated_at: string;
}

export interface KnowledgeDocument {
	id: string;
	knowledge_base_id: string;
	name: string;
	source: "file" | "url";
	source_url?: string;
	status:
		| "pending"
		| "parsing"
		| "chunking"
		| "embedding"
		| "storing"
		| "ready"
		| "error";
	chunk_count: number;
	char_count: number;
	error_message?: string;
	created_at: string;
	updated_at: string;
}

export interface SearchResultItem {
	chunk_id: string;
	content: string;
	score: number;
	document_name: string;
}

export const knowledgeApi = {
	listAllKBs: (params?: { page?: number; page_size?: number; q?: string }) =>
		http.get<{ knowledge_bases: KnowledgeBase[] }>(
			`/data/knowledge/knowledge_bases`,
			{ params },
		),
	listKBs: (botId: string, params?: { page?: number; page_size?: number }) =>
		http.get<{ knowledge_bases: KnowledgeBase[] }>(
			`/data/knowledge/bots/${botId}/knowledge_bases`,
			{ params },
		),
	listBoundKBs: (botId: string) =>
		http.get<{ knowledge_bases: KnowledgeBase[] }>(
			`/data/knowledge/bots/${botId}/knowledge_bases/bound`,
		),
	createKB: (
		botId: string,
		data: { name: string; description?: string; embedding_model_id: string },
	) =>
		http.post<KnowledgeBase>(
			`/data/knowledge/bots/${botId}/knowledge_bases`,
			data,
		),
	getKB: (kbId: string) =>
		http.get<KnowledgeBase>(`/data/knowledge/knowledge_bases/${kbId}`),
	searchKB: (kbId: string, q: string, top_k?: number) =>
		http.get<SearchResultItem[]>(
			`/data/knowledge/knowledge_bases/${kbId}/search`,
			{ params: { q, top_k } },
		),
	deleteKB: (kbId: string) =>
		http.delete(`/data/knowledge/knowledge_bases/${kbId}`),
	bindKB: (botId: string, kbId: string) =>
		http.post(`/data/knowledge/bots/${botId}/knowledge_bases/bind`, {
			kb_id: kbId,
		}),
	unbindKB: (botId: string, kbId: string) =>
		http.delete(`/data/knowledge/bots/${botId}/knowledge_bases/${kbId}/bind`),
	listDocs: (kbId: string) =>
		http.get<{ documents: KnowledgeDocument[] }>(
			`/data/knowledge/knowledge_bases/${kbId}/documents`,
		),
	uploadDoc: (kbId: string, file: File) => {
		const form = new FormData();
		form.append("file", file);
		return http.post<KnowledgeDocument>(
			`/data/knowledge/knowledge_bases/${kbId}/documents`,
			form,
			{ headers: { "Content-Type": "multipart/form-data" } },
		);
	},
	ingestURL: (kbId: string, url: string) =>
		http.post<KnowledgeDocument>(
			`/data/knowledge/knowledge_bases/${kbId}/documents/url`,
			{ url },
		),
	deleteDoc: (docId: string) =>
		http.delete(`/data/knowledge/documents/${docId}`),
	getDocStatus: (docId: string) =>
		http.get<KnowledgeDocument>(`/data/knowledge/documents/${docId}/status`),
	retryDoc: (docId: string) =>
		http.post(`/data/knowledge/documents/${docId}/retry`),
};

export const availableResourcesApi = {
	list: (lang?: string) =>
		http.get<AvailableResources>("/available-resources", {
			params: lang ? { lang } : undefined,
		}),
};

// ------------------------------------------------------------------ 计费
//
// 行形状对应 internal/store/billing_models.go，字段名与 JSON tag 一一对应；金额一律是
// int64 微单位（1e-6 元），展示统一走 @/lib/billing。

export type BillingChargeMode = "count" | "duration" | "usage" | "recurring";
export type BillingUnit =
	| "call"
	| "second"
	| "token"
	| "char"
	| "byte"
	| "period";
export type BillingMeterSource =
	| "llm"
	| "tts"
	| "asr"
	| "voice:clone"
	| "session"
	| "mcp"
	| "kb";
export type BillingResourceType = "item" | "provider" | "model" | "voice";
export type BillingRounding = "none" | "ceil" | "half:up";
export type BillingAccountStatus = "active" | "suspended" | "closed";
export type BillingSubjectType = "user" | "org";
export type BillingDirection = "debit" | "credit";
export type BillingUsageStatus =
	| "pending"
	| "charged"
	| "skipped"
	| "unpaid"
	| "rejected";

export interface BillingItem {
	code: string;
	name: string;
	charge_mode: BillingChargeMode;
	unit: BillingUnit;
	meter_source: BillingMeterSource;
	enabled: boolean;
	is_system: boolean;
	created_at: string;
	updated_at: string;
}

export interface BillingTier {
	up_to: number;
	unit_price_micro: number;
}

export interface BillingPrice {
	id: string;
	item_code: string;
	/** '' = 平台标准价 */
	account_id: string;
	resource_type: BillingResourceType;
	resource_id: string;
	currency: string;
	unit_price_micro: number;
	unit_size: number;
	min_charge_micro: number;
	rounding: BillingRounding;
	tiers?: BillingTier[];
	effective_from: string;
	effective_to?: string | null;
	created_at: string;
	updated_at: string;
}

/** 新建价格版本时提交的字段：scope 与生效时间必填，其余由服务端补默认值。 */
export interface BillingPriceCreate {
	item_code: string;
	account_id?: string;
	resource_type?: BillingResourceType;
	resource_id?: string;
	currency?: string;
	unit_price_micro: number;
	unit_size: number;
	min_charge_micro?: number;
	rounding?: BillingRounding;
	tiers?: BillingTier[];
	effective_from?: string;
}

/** 局部更新一版价格；服务端只认识这 8 个字段，其余一律 400。 */
export interface BillingPriceUpdate {
	currency?: string;
	unit_price_micro?: number;
	unit_size?: number;
	min_charge_micro?: number;
	rounding?: BillingRounding;
	tiers?: BillingTier[];
	effective_from?: string;
	effective_to?: string;
}

export interface BillingAccount {
	id: string;
	subject_type: BillingSubjectType;
	subject_id: string;
	currency: string;
	balance_micro: number;
	frozen_micro: number;
	credit_limit_micro: number;
	status: BillingAccountStatus;
	created_at: string;
	updated_at: string;
}

export interface BillingLedger {
	id: number;
	account_id: string;
	direction: BillingDirection;
	amount_micro: number;
	balance_after_micro: number;
	kind: string;
	item_code: string;
	ref_type: string;
	ref_id: string;
	idempotency_key: string;
	occurred_at: string;
	note?: string;
	creator?: string;
}

export interface BillingUsageEvent {
	id: string;
	account_id: string;
	voicebot_id: string;
	device_id: string;
	session_id: string;
	item_code: string;
	quantity: number;
	aimodel_id: string;
	provider_id: string;
	voice_id: string;
	byok: boolean;
	occurred_at: string;
	received_at: string;
	turn_index: number;
	dimensions?: Record<string, unknown>;
	status: BillingUsageStatus;
	price_id: string;
	amount_micro: number;
	last_error?: string;
}

/** 按计费项聚合的一行（summary.top_items / stats.usage_by_item）。 */
export interface BillingUsageAggregate {
	item_code: string;
	quantity: number;
	amount_micro: number;
}

export interface BillingSummary {
	/** 还没有任何用量的人没有账户，这里是 null */
	account: BillingAccount | null;
	currency: string;
	balance_micro: number;
	frozen_micro: number;
	credit_limit_micro: number;
	period_charged_micro: number;
	top_items?: BillingUsageAggregate[];
	from: string;
	to: string;
}

export interface BillingStats {
	account: BillingAccount;
	summary: BillingSummary;
	usage_by_item: BillingUsageAggregate[];
	from: string;
	to: string;
}

/**
 * 模型监控的数据源（GET /api/billing/usage-by-model）：一行 = 模型 × 计费项。
 *
 * `quantity` 含还在 pending 等结算的量，`amount_micro` 只含已结算（charged）的事件
 * ——两者不是同一批事件的产物。三个状态计数器把差额说清楚：pending 只是慢一拍，
 * unpaid（没配价格 / 透支被拒）是真正需要处理的那部分，skipped 是本来就不计费的
 * （BYOK / 计费项未启用 / 量为 0）；剩下的就是 charged。
 */
export interface BillingModelUsageRow {
	aimodel_id: string;
	/** 模型行被删掉时为空，按 aimodel_id 显示 */
	model_name: string;
	model_type: string;
	provider_id: string;
	provider_name: string;
	item_code: string;
	quantity: number;
	amount_micro: number;
	event_count: number;
	/** 还在结算队列里（秒级） */
	pending_events: number;
	/** 没算成钱：没命中价格 / 透支被拒，需要人工处理 */
	unpaid_events: number;
	/** 本来就不计费：BYOK / 计费项未启用 / 量为 0 */
	skipped_events: number;
}

export interface BillingModelUsage {
	currency: string;
	from: string;
	to: string;
	usage_by_model: BillingModelUsageRow[];
}

export interface BillingPageList<T> {
	items: T[];
	total: number;
	page: number;
	page_size: number;
}

/** 价格列表不带 total：价格表按 scope 唯一，量级是“几十条版本”。 */
export interface BillingPriceList {
	items: BillingPrice[];
	page: number;
	page_size: number;
}

export interface BillingItemList {
	items: BillingItem[];
	total: number;
}

export interface BillingAdjustRequest {
	amount_micro: number;
	item_code?: string;
	note: string;
	/** true = 限定计费项的赠款，进 billing_grants，不动余额 */
	grant?: boolean;
	ref_id?: string;
}

export interface BillingSummaryParams {
	/** RFC3339；都省略 = 本账期到现在 */
	from?: string;
	to?: string;
}

export interface BillingUsageParams extends BillingSummaryParams {
	item_code?: string;
	page?: number;
	page_size?: number;
}

export interface BillingPriceQueryParams {
	item_code?: string;
	account_id?: string;
	resource_type?: BillingResourceType;
	resource_id?: string;
	active_only?: string;
	page?: number;
	page_size?: number;
}

export interface BillingAccountQueryParams {
	subject_type?: BillingSubjectType;
	status?: BillingAccountStatus;
	keyword?: string;
	page?: number;
	page_size?: number;
}

export interface BillingLedgerQueryParams extends BillingSummaryParams {
	account_id?: string;
	item_code?: string;
	kind?: string;
	page?: number;
	page_size?: number;
}

// 用户端：只读自己的账户与用量。返回 503 表示平台没开计费，不是错误。
export const billingApi = {
	summary: (params?: BillingSummaryParams) =>
		http.get<BillingSummary>("/billing/summary", { params }),
	usage: (params?: BillingUsageParams) =>
		http.get<BillingPageList<BillingUsageEvent>>("/billing/usage", { params }),
	/** 按模型 × 计费项聚合的用量（模型监控页） */
	modelUsage: (params?: BillingSummaryParams) =>
		http.get<BillingModelUsage>("/billing/usage-by-model", { params }),
	prices: () => http.get<BillingPriceList>("/billing/prices"),
};

// 管理端：定价、账户、流水、报表（middleware.RequireAdmin）。
export const billingAdminApi = {
	items: () => http.get<BillingItemList>("/billing/items"),
	setItem: (code: string, enabled: boolean) =>
		http.put<{ code: string; enabled: boolean }>(
			`/billing/items/${encodeURIComponent(code)}`,
			{ enabled },
		),
	prices: (params?: BillingPriceQueryParams) =>
		http.get<BillingPriceList>("/billing/prices", { params }),
	createPrice: (data: BillingPriceCreate) =>
		http.post<BillingPrice>("/billing/prices", data),
	updatePrice: (id: string, data: BillingPriceUpdate) =>
		http.put<BillingPrice>(`/billing/prices/${id}`, data),
	deletePrice: (id: string) => http.delete(`/billing/prices/${id}`),
	accounts: (params?: BillingAccountQueryParams) =>
		http.get<BillingPageList<BillingAccount>>("/billing/accounts", { params }),
	adjust: (id: string, data: BillingAdjustRequest) =>
		http.post<BillingAccount>(
			`/billing/accounts/${encodeURIComponent(id)}/adjust`,
			data,
		),
	ledger: (params?: BillingLedgerQueryParams) =>
		http.get<BillingPageList<BillingLedger>>("/billing/ledger", { params }),
	stats: (params: { account_id: string } & BillingSummaryParams) =>
		http.get<BillingStats>("/billing/stats", { params }),
};
