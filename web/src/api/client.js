const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export class ApiError extends Error {
  constructor(message, status = 0, data = null) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.data = data
  }
}

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
    if (response.status === 401 && !path.includes('/auth/login')) {
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
  async upload(path, file) {
    const form = new FormData()
    form.append('file', file)
    return request(path, { method: 'POST', body: form })
  },
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

export function getErrorMessage(error) {
  return error?.message || '操作失败，请稍后重试'
}
