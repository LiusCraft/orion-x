import axios from 'axios'

const http = axios.create({ baseURL: '/api' })

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

http.interceptors.response.use(
  (r) => r,
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('token')
      window.location.href = '/login'
    }
    return Promise.reject(err)
  }
)

export interface Voicebot {
  id: string
  name: string
  owner_id: string
  config_json: string
  created_at: string
  updated_at: string
  creator: string
}

export interface Device {
  id: string
  voicebot_id: string
  name: string
  created_at: string
  creator: string
}

export const authApi = {
  login: (username: string, password: string) =>
    http.post<{ token: string; user_id: string; username: string }>('/auth/login', { username, password }),
}

export const voicebotApi = {
  list: () => http.get<Voicebot[]>('/voicebots'),
  get: (id: string) => http.get<Voicebot>(`/voicebots/${id}`),
  create: (name: string, config_json?: string) =>
    http.post<Voicebot>('/voicebots', { name, config_json }),
  update: (id: string, name: string, config_json: string) =>
    http.put<Voicebot>(`/voicebots/${id}`, { name, config_json: JSON.parse(config_json) }),
  remove: (id: string) => http.delete(`/voicebots/${id}`),
}

export const deviceApi = {
  list: (voicebotId: string) =>
    http.get<Device[]>(`/voicebots/${voicebotId}/devices`),
  create: (voicebotId: string, id: string, name: string) =>
    http.post<Device>(`/voicebots/${voicebotId}/devices`, { id, name }),
  remove: (voicebotId: string, deviceId: string) =>
    http.delete(`/voicebots/${voicebotId}/devices/${deviceId}`),
}

export interface Provider {
  id: string
  name: string
  slug: string
  base_url: string
  is_system: boolean
  extra?: Record<string, unknown>
  created_at: string
  creator: string
}

export type ModelType = 'text' | 'vision' | 'speech' | 'multimodal' | 'embedding'

export interface AIModel {
  id: string
  provider_id: string
  provider?: Provider
  name: string
  type: ModelType
  base_url: string
  model_id: string
  is_system: boolean
  extra?: Record<string, unknown>
  created_at: string
  creator: string
}

export interface ProviderSlug {
  slug: string
  category: string
  name: string
  base_url: string
}

export const providerApi = {
  list: () => http.get<Provider[]>('/providers'),
  slugs: () => http.get<ProviderSlug[]>('/providers/slugs'),
  get: (id: string) => http.get<Provider>(`/providers/${id}`),
  create: (data: { name: string; slug: string; base_url: string; api_key?: string; extra?: Record<string, unknown> }) =>
    http.post<Provider>('/providers', data),
  update: (id: string, data: { name?: string; base_url?: string; api_key?: string; extra?: Record<string, unknown> }) =>
    http.put<Provider>(`/providers/${id}`, data),
  remove: (id: string) => http.delete(`/providers/${id}`),
}

export const modelApi = {
  list: (type?: ModelType) =>
    http.get<AIModel[]>('/models', { params: type ? { type } : undefined }),
  types: () => http.get<ModelType[]>('/models/types'),
  get: (id: string) => http.get<AIModel>(`/models/${id}`),
  create: (data: { provider_id: string; name: string; type: ModelType; base_url?: string; model_id: string; extra?: Record<string, unknown> }) =>
    http.post<AIModel>('/models', data),
  update: (id: string, data: { name?: string; base_url?: string; model_id?: string; extra?: Record<string, unknown> }) =>
    http.put<AIModel>(`/models/${id}`, data),
  remove: (id: string) => http.delete(`/models/${id}`),
}

export interface ModelVoice {
  id: string
  model_id: string
  voice_id: string
  name: string
  description?: string
  gender?: 'male' | 'female' | 'neutral'
  avatar_url?: string
  preview_url?: string
  tags?: string[]
  langs?: string[]
  emotions?: Record<string, unknown>
  is_system: boolean
  is_cloned: boolean
  source_audio_url?: string
  extra?: Record<string, unknown>
  created_at: string
  updated_at: string
  creator: string
}

export interface Language {
  code: string
  name: string
  parent_code?: string
  children?: Language[]
}

export const languageApi = {
  list: (parentCode?: string) =>
    http.get<Language[]>('/languages', { params: parentCode ? { parent_code: parentCode } : undefined }),
}

export const voiceApi = {
  listSystem: (lang?: string) =>
    http.get<ModelVoice[]>('/voices/system', { params: lang ? { lang } : undefined }),
}
