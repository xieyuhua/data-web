import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { api } from '../api/client'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('bi_token') || '')
  const user = ref(null)
  const role = ref(null)
  const permissions = ref([])
  const dataScope = ref('')
  const isAdmin = ref(false)
  const loading = ref(false)

  const displayName = computed(() => user.value?.name || '访客')
  const orgName = computed(() => user.value?.organization?.name || '')
  const initials = computed(() => displayName.value.slice(0, 2))
  const isLoggedIn = computed(() => Boolean(token.value))

  function can(permission) {
    return isAdmin.value || permissions.value.includes('*') || permissions.value.includes(permission)
  }

  function applyLogin(data) {
    token.value = data.token
    user.value = data.user
    role.value = data.role
    permissions.value = data.permissions || []
    dataScope.value = data.data_scope
    isAdmin.value = data.permissions?.includes('*') || data.role?.code === 'super_admin'
    localStorage.setItem('bi_token', data.token)
    localStorage.setItem('bi_user', JSON.stringify(data.user))
  }

  async function login(username, password) {
    loading.value = true
    try {
      const data = await api.post('/auth/login', { username, password })
      applyLogin(data)
      return data
    } finally {
      loading.value = false
    }
  }

  async function fetchMe() {
    if (!token.value) return null
    loading.value = true
    try {
      const data = await api.get('/auth/me')
      user.value = data.user
      role.value = data.role
      permissions.value = data.permissions || []
      dataScope.value = data.data_scope
      isAdmin.value = Boolean(data.is_admin)
      localStorage.setItem('bi_user', JSON.stringify(data.user))
      return data
    } catch (error) {
      logout()
      throw error
    } finally {
      loading.value = false
    }
  }

  function logout() {
    token.value = ''
    user.value = null
    role.value = null
    permissions.value = []
    dataScope.value = ''
    isAdmin.value = false
    localStorage.removeItem('bi_token')
    localStorage.removeItem('bi_user')
  }

  return {
    token,
    user,
    role,
    permissions,
    dataScope,
    isAdmin,
    loading,
    displayName,
    orgName,
    initials,
    isLoggedIn,
    can,
    login,
    fetchMe,
    logout,
  }
})
