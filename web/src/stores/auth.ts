import { defineStore } from 'pinia'
import { authApi } from '@/api'
import { TOKEN_KEY } from '@/api/http'
import type { User } from '@/types'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem(TOKEN_KEY) || '',
    user: null as User | null,
    loaded: false,
  }),
  getters: {
    isLoggedIn: (s) => !!s.token,
    isAdmin: (s) => s.user?.role === 'admin',
    displayName: (s) => s.user?.displayName || s.user?.username || '',
  },
  actions: {
    async login(username: string, password: string) {
      const res = await authApi.login(username, password)
      this.token = res.token
      this.user = res.user
      this.loaded = true
      localStorage.setItem(TOKEN_KEY, res.token)
    },
    async fetchMe() {
      if (!this.token) return null
      this.user = await authApi.me()
      this.loaded = true
      return this.user
    },
    logout() {
      this.token = ''
      this.user = null
      this.loaded = false
      localStorage.removeItem(TOKEN_KEY)
    },
  },
})
