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
