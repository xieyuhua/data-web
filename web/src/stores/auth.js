// 登录态与权限 Store：
// - token 持久化在 localStorage，刷新页面后保持登录
// - permissions 为后端下发的权限码集合，包含 '*' 表示超级管理员
// - dataScope 描述数据可见范围（全部组织 / 本组织及下级 / 仅本组织）
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { api } from '../api/client'

export const useAuthStore = defineStore('auth', () => {
  // 登录凭证：初始化时从本地缓存恢复
  const token = ref(localStorage.getItem('bi_token') || '')
  const user = ref(null)
  const role = ref(null)
  const permissions = ref([])
  const dataScope = ref('')
  // 是否超级管理员：拥有全部权限，前端权限判断直接放行
  const isAdmin = ref(false)
  const loading = ref(false)

  const displayName = computed(() => user.value?.name || '访客')
  const orgName = computed(() => user.value?.organization?.name || '')
  // 头像占位文字：取姓名前两个字
  const initials = computed(() => displayName.value.slice(0, 2))
  const isLoggedIn = computed(() => Boolean(token.value))

  // 判断当前用户是否拥有某个权限码
  // 规则：超级管理员 / 拥有通配符 '*' / 权限列表命中
  function can(permission) {
    return isAdmin.value || permissions.value.includes('*') || permissions.value.includes(permission)
  }

  // 登录成功后统一写入登录态（账号密码登录与企业微信登录共用）
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

  // 账号密码登录
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

  // 用企业微信回调签发的一次性 ticket 换取正式登录态
  async function exchangeWecom(ticket) {
    loading.value = true
    try {
      const data = await api.post('/auth/wecom/exchange', { ticket })
      applyLogin(data)
      return data
    } finally {
      loading.value = false
    }
  }

  // 进入应用或刷新时拉取最新的用户、角色与权限信息
  // 失败（如 token 过期）会清空本地登录态并抛出，由路由守卫跳转登录页
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

  // 退出登录：清空内存状态与本地缓存（JWT 无状态，无需调用后端）
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
    exchangeWecom,
    fetchMe,
    logout,
  }
})
