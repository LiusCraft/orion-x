import { create } from 'zustand'

interface AuthState {
  token: string | null
  userId: string | null
  username: string | null
  isAdmin: boolean
  setAuth: (token: string, userId: string, username: string, isAdmin: boolean) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>((set) => ({
  token: localStorage.getItem('token'),
  userId: localStorage.getItem('userId'),
  username: localStorage.getItem('username'),
  isAdmin: localStorage.getItem('isAdmin') === 'true',
  setAuth: (token, userId, username, isAdmin) => {
    localStorage.setItem('token', token)
    localStorage.setItem('userId', userId)
    localStorage.setItem('username', username)
    localStorage.setItem('isAdmin', String(isAdmin))
    set({ token, userId, username, isAdmin })
  },
  logout: () => {
    localStorage.removeItem('token')
    localStorage.removeItem('userId')
    localStorage.removeItem('username')
    localStorage.removeItem('isAdmin')
    set({ token: null, userId: null, username: null, isAdmin: false })
  },
}))
