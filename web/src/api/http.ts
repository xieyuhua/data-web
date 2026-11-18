import axios, { AxiosError } from 'axios'
import { ElMessage } from 'element-plus'
import type { ApiResponse } from '@/types'

const http = axios.create({
  baseURL: '/api',
  timeout: 120000,
})

export const TOKEN_KEY = 'bi_token'

http.interceptors.request.use((config) => {
  const token = localStorage.getItem(TOKEN_KEY)
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

let redirecting = false

http.interceptors.response.use(
  (resp) => resp,
  (error: AxiosError<ApiResponse>) => {
    const status = error.response?.status
    if (status === 401) {
      localStorage.removeItem(TOKEN_KEY)
      if (!redirecting && !location.pathname.startsWith('/login') && !location.pathname.startsWith('/share/')) {
        redirecting = true
        location.href = '/login'
      }
    }
    return Promise.reject(error)
  },
)

export async function request<T = any>(config: Parameters<typeof http.request>[0]): Promise<T> {
  try {
    const resp = await http.request<ApiResponse<T>>(config)
    return resp.data.data
  } catch (e) {
    const err = e as AxiosError<ApiResponse>
    const msg = err.response?.data?.message || err.message || '请求失败'
    ElMessage.error(msg)
    throw e
  }
}

export async function download(config: Parameters<typeof http.request>[0], fallbackName = 'export.xlsx') {
  try {
    const resp = await http.request<Blob>({ ...config, responseType: 'blob' })
    const disposition = String(resp.headers['content-disposition'] || '')
    let filename = fallbackName
    const m = disposition.match(/filename="?([^";]+)"?/)
    if (m) filename = decodeURIComponent(m[1])
    const url = URL.createObjectURL(resp.data)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    const err = e as AxiosError<ApiResponse>
    ElMessage.error(err.response?.data?.message || '导出失败')
    throw e
  }
}

export async function uploadFile<T = any>(url: string, file: File): Promise<T> {
  const fd = new FormData()
  fd.append('file', file)
  try {
    const resp = await http.request<ApiResponse<T>>({ url, method: 'POST', data: fd })
    return resp.data.data
  } catch (e) {
    const err = e as AxiosError<ApiResponse>
    ElMessage.error(err.response?.data?.message || '导入失败')
    throw e
  }
}

export async function downloadUpload(url: string, file: File, fallbackName: string) {
  const fd = new FormData()
  fd.append('file', file)
  try {
    const resp = await http.request<Blob>({ url, method: 'POST', data: fd, responseType: 'blob' })
    const disposition = String(resp.headers['content-disposition'] || '')
    let filename = fallbackName
    const m = disposition.match(/filename\*=UTF-8''([^;]+)/)
    if (m) filename = decodeURIComponent(m[1])
    const obj = URL.createObjectURL(resp.data)
    const a = document.createElement('a')
    a.href = obj
    a.download = filename
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(obj)
  } catch (e) {
    const err = e as AxiosError<ApiResponse>
    ElMessage.error(err.response?.data?.message || '生成报告失败')
    throw e
  }
}

export default http
