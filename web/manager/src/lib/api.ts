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
}

export const authApi = {
	login: (username: string, password: string) =>
		http.post<{ token: string; user_id: string; username: string }>(
			"/auth/login",
			{ username, password },
		),
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
		http.get<{ user_id: string; username: string; email: string }>(
			"/auth/profile",
		),
};

export const voicebotApi = {
	list: () => http.get<Voicebot[]>("/voicebots"),
	get: (id: string) => http.get<Voicebot>(`/voicebots/${id}`),
	create: (name: string, config_json?: string) =>
		http.post<Voicebot>("/voicebots", { name, config_json }),
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

export const availableResourcesApi = {
	list: (lang?: string) =>
		http.get<AvailableResources>("/available-resources", {
			params: lang ? { lang } : undefined,
		}),
};
