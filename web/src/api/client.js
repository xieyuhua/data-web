// 统一的 API 请求封装：
// - 自动携带 JWT（Authorization: Bearer xxx）
// - 自动序列化 JSON，FormData 原样提交
// - 统一解析后端 { code, message, data } 结构，直接返回 data
// - 401 时清理本地登录态并跳转登录页（登录与企业微信接口除外）
const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

// 业务/网络错误统一包装，页面可通过 getErrorMessage 获取提示文案
export class ApiError extends Error {
  constructor(message, status = 0, data = null) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.data = data
  }
}

// 核心请求方法：拼接 baseURL、注入 token、解析响应、统一错误处理
export async function request(path, options = {}) {
  const token = localStorage.getItem('bi_token')
  const headers = new Headers(options.headers || {})
  if (token) headers.set('Authorization', `Bearer ${token}`)
  let body = options.body
  if (body && !(body instanceof FormData) && typeof body !== 'string') {
    headers.set('Content-Type', 'application/json')
    body = JSON.stringify(body)
  }
  const response = await fetch(`${API_BASE}${path}`, { ...options, headers, body })
  const contentType = response.headers.get('content-type') || ''
  let payload = null
  if (contentType.includes('application/json')) {
    payload = await response.json().catch(() => null)
  }
  if (!response.ok) {
    // 登录与企业微信授权接口的 401 交给页面展示，不做全局跳转，避免回调页丢失错误信息
    if (response.status === 401 && !path.includes('/auth/login') && !path.includes('/auth/wecom/')) {
      localStorage.removeItem('bi_token')
      localStorage.removeItem('bi_user')
      if (!location.pathname.startsWith('/login') && !location.pathname.startsWith('/share/')) {
        location.assign(`/login?redirect=${encodeURIComponent(location.pathname + location.search)}`)
      }
    }
    throw new ApiError(payload?.message || `请求失败（${response.status}）`, response.status, payload)
  }
  if (payload && payload.code !== 0) {
    throw new ApiError(payload.message || '请求失败', response.status, payload)
  }
  return payload?.data
}

// 常用请求方法的语法糖
export const api = {
  get(path) {
    return request(path)
  },
  post(path, body) {
    return request(path, { method: 'POST', body })
  },
  put(path, body) {
    return request(path, { method: 'PUT', body })
  },
  delete(path) {
    return request(path, { method: 'DELETE' })
  },
  // 上传：以 multipart/form-data 提交单个文件（如 Excel 导入）
  async upload(path, file) {
    const form = new FormData()
    form.append('file', file)
    return request(path, { method: 'POST', body: form })
  },
  // 下载：用原生 fetch 获取二进制流，再通过临时 <a> 标签触发浏览器下载
  async download(path, filename) {
    const token = localStorage.getItem('bi_token')
    const response = await fetch(`${API_BASE}${path}`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!response.ok) {
      let message = '下载失败'
      try {
        const payload = await response.json()
        message = payload.message || message
      } catch {
        /* ignore */
      }
      throw new ApiError(message, response.status)
    }
    const blob = await response.blob()
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  },
}

// 从异常对象中提取用户可读的提示文案
export function getErrorMessage(error) {
  return error?.message || '操作失败，请稍后重试'
}
