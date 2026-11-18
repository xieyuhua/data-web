import http, { request, download, uploadFile, downloadUpload } from './http'
import type {
  DataSource, Dataset, Chart, Dashboard, ResultSet, User, PageData,
  ShareLink, Grant, ScheduleTask, ScheduleRun, PublicDashboard, FilterOption,
  DashboardData, FilterBinding, ColumnMeta,
} from '@/types'

export const authApi = {
  login: (username: string, password: string) =>
    request<{ token: string; user: User }>({ url: '/auth/login', method: 'POST', data: { username, password } }),
  me: () => request<User>({ url: '/auth/me' }),
  changePassword: (oldPassword: string, newPassword: string) =>
    request({ url: '/auth/password', method: 'PUT', data: { oldPassword, newPassword } }),
  wecomStatus: () =>
    request<{ enabled: boolean; corpId?: string; agentId?: number; lastSyncAt?: string; lastStatus?: string; lastMessage?: string }>({
      url: '/wecom/status',
    }),
  wecomAuthorizeURL: (state = '') =>
    request<{ url: string }>({ url: '/wecom/authorize-url', params: { state } }),
  wecomLogin: (code: string) =>
    request<{ token: string; user: User }>({ url: '/wecom/login', method: 'POST', data: { code } }),
}

export const userApi = {
  list: (params: { page?: number; pageSize?: number; keyword?: string }) =>
    request<PageData<User>>({ url: '/users', params }),
  create: (data: Partial<User> & { password: string }) =>
    request<User>({ url: '/users', method: 'POST', data }),
  update: (id: number, data: Partial<User>) =>
    request<User>({ url: `/users/${id}`, method: 'PUT', data }),
  resetPassword: (id: number, password: string) =>
    request({ url: `/users/${id}/password`, method: 'PUT', data: { password } }),
  remove: (id: number) => request({ url: `/users/${id}`, method: 'DELETE' }),
}

export const dataSourceApi = {
  list: () => request<DataSource[]>({ url: '/datasources' }),
  get: (id: number) => request<DataSource>({ url: `/datasources/${id}` }),
  create: (data: any) => request<DataSource>({ url: '/datasources', method: 'POST', data }),
  update: (id: number, data: any) => request<DataSource>({ url: `/datasources/${id}`, method: 'PUT', data }),
  remove: (id: number) => request({ url: `/datasources/${id}`, method: 'DELETE' }),
  test: (data: any) => request<{ message: string }>({ url: '/datasources/test', method: 'POST', data }),
  testById: (id: number) => request<{ message: string }>({ url: `/datasources/${id}/test`, method: 'POST' }),
}

export const datasetApi = {
  list: (keyword?: string) => request<Dataset[]>({ url: '/datasets', params: { keyword } }),
  get: (id: number) => request<Dataset>({ url: `/datasets/${id}` }),
  create: (data: any) => request<Dataset>({ url: '/datasets', method: 'POST', data }),
  update: (id: number, data: any) => request<Dataset>({ url: `/datasets/${id}`, method: 'PUT', data }),
  remove: (id: number) => request({ url: `/datasets/${id}`, method: 'DELETE' }),
  preview: (id: number, data: { params?: Record<string, string>; limit?: number }) =>
    request<ResultSet>({ url: `/datasets/${id}/preview`, method: 'POST', data }),
  columns: (id: number) => request<ColumnMeta[]>({ url: `/datasets/${id}/columns` }),
}

export const chartApi = {
  list: (keyword?: string) => request<Chart[]>({ url: '/charts', params: { keyword } }),
  get: (id: number) => request<Chart>({ url: `/charts/${id}` }),
  create: (data: any) => request<Chart>({ url: '/charts', method: 'POST', data }),
  update: (id: number, data: any) => request<Chart>({ url: `/charts/${id}`, method: 'PUT', data }),
  remove: (id: number) => request({ url: `/charts/${id}`, method: 'DELETE' }),
  duplicate: (id: number) => request<Chart>({ url: `/charts/${id}/duplicate`, method: 'POST' }),
  data: (id: number, params?: Record<string, string>) =>
    request<ResultSet>({ url: `/charts/${id}/data`, method: 'POST', data: { params } }),
  previewData: (data: { datasetId: number; type: string; config: any; params?: Record<string, string> }) =>
    request<ResultSet>({ url: '/charts/preview-data', method: 'POST', data }),
  export: (id: number, params?: Record<string, string>) =>
    download({ url: `/charts/${id}/export`, method: 'POST', data: { params } }, 'chart.xlsx'),
  exportCSV: (id: number, params?: Record<string, string>) =>
    download({ url: `/charts/${id}/export-csv`, method: 'POST', data: { params } }, 'chart.csv'),
}

export const dashboardApi = {
  list: (params: { page?: number; pageSize?: number; keyword?: string }) =>
    request<PageData<Dashboard>>({ url: '/dashboards', params }),
  get: (id: number) =>
    request<{ dashboard: Dashboard; permission: 'edit' | 'view'; bindings: FilterBinding[] }>({ url: `/dashboards/${id}` }),
  create: (data: { name: string; description?: string }) =>
    request<Dashboard>({ url: '/dashboards', method: 'POST', data }),
  update: (id: number, data: { name: string; description?: string }) =>
    request<Dashboard>({ url: `/dashboards/${id}`, method: 'PUT', data }),
  remove: (id: number) => request({ url: `/dashboards/${id}`, method: 'DELETE' }),
  duplicate: (id: number) => request<Dashboard>({ url: `/dashboards/${id}/duplicate`, method: 'POST' }),
  saveLayout: (id: number, data: { widgets: any[]; filters: any[] }) =>
    request<Dashboard>({ url: `/dashboards/${id}/layout`, method: 'PUT', data }),
  saveBindings: (id: number, bindings: FilterBinding[]) =>
    request({ url: `/dashboards/${id}/bindings`, method: 'PUT', data: { bindings } }),
  data: (id: number, filterValues: Record<string, any>) =>
    request<DashboardData>({ url: `/dashboards/${id}/data`, method: 'POST', data: { filterValues } }),
  filterOptions: (id: number, filterId: number) =>
    request<FilterOption[]>({ url: `/dashboards/${id}/filters/${filterId}/options`, method: 'POST' }),
  export: (id: number, filterValues: Record<string, any>) =>
    download({ url: `/dashboards/${id}/export`, method: 'POST', data: { filterValues } }, 'dashboard.xlsx'),
  grants: (id: number) => request<Grant[]>({ url: `/dashboards/${id}/grants` }),
  addGrant: (id: number, userId: number, permission: string) =>
    request({ url: `/dashboards/${id}/grants`, method: 'POST', data: { userId, permission } }),
  removeGrant: (id: number, grantId: number) =>
    request({ url: `/dashboards/${id}/grants/${grantId}`, method: 'DELETE' }),
  shares: (id: number) => request<ShareLink[]>({ url: `/dashboards/${id}/shares` }),
  createShare: (id: number, expiresInDays = 0) =>
    request<ShareLink>({ url: `/dashboards/${id}/shares`, method: 'POST', data: { expiresInDays } }),
  revokeShare: (id: number, shareId: number) =>
    request({ url: `/dashboards/${id}/shares/${shareId}`, method: 'DELETE' }),
}

export const publicApi = {
  view: (token: string) => request<PublicDashboard>({ url: `/public/dashboards/${token}` }),
  data: (token: string, filterValues: Record<string, any>) =>
    request<DashboardData>({ url: `/public/dashboards/${token}/data`, method: 'POST', data: { filterValues } }),
  filterOptions: (token: string, filterId: number) =>
    request<FilterOption[]>({ url: `/public/dashboards/${token}/filters/${filterId}/options`, method: 'POST' }),
}

export const scheduleApi = {
  list: () => request<ScheduleTask[]>({ url: '/schedules' }),
  create: (data: any) => request<ScheduleTask>({ url: '/schedules', method: 'POST', data }),
  update: (id: number, data: any) => request<ScheduleTask>({ url: `/schedules/${id}`, method: 'PUT', data }),
  remove: (id: number) => request({ url: `/schedules/${id}`, method: 'DELETE' }),
  run: (id: number) => request<ScheduleRun>({ url: `/schedules/${id}/run`, method: 'POST' }),
  runs: (id: number) => request<ScheduleRun[]>({ url: `/schedules/${id}/runs` }),
  download: (runId: number) => download({ url: `/schedule-runs/${runId}/download` }, 'report.xlsx'),
}

export const settingsApi = {
  get: () => request<{ version: string; smtpEnabled: boolean; schedulerEnabled: boolean }>({ url: '/admin/settings' }),
}

export const wecomApi = {
  sync: () => request<{ departments: number; users: number; status: string; message: string }>({
    url: '/admin/wecom/sync', method: 'POST',
  }),
  departments: () => request<Array<{ wecomId: number; name: string; parentWecomId: number; depth: number }>>({
    url: '/departments',
  }),
  deptGrants: (dashboardId: number) =>
    request<Array<{ id: number; dashboardId: number; wecomDeptId: number; deptName: string; permission: string; createdAt: string }>>({
      url: `/dashboards/${dashboardId}/dept-grants`,
    }),
  addDeptGrant: (dashboardId: number, wecomDeptId: number, permission: string) =>
    request({ url: `/dashboards/${dashboardId}/dept-grants`, method: 'POST', data: { wecomDeptId, permission } }),
  removeDeptGrant: (dashboardId: number, grantId: number) =>
    request({ url: `/dashboards/${dashboardId}/dept-grants/${grantId}`, method: 'DELETE' }),
}

export interface BigRegion {
  id: number
  code: string
  name: string
  pinyin: string
  initials: string
  sortOrder: number
  remarks?: string
}
export interface TerritoryRegion {
  id: number
  code: string
  name: string
  bigRegionId: number
  pinyin: string
  initials: string
  sortOrder: number
  remarks?: string
}
export interface Store {
  id: number
  code: string
  name: string
  bigRegionCode: string
  regionCode: string
  bigRegionId: number
  regionId: number
  bigRegionName?: string
  regionName?: string
  address: string
  contact: string
  phone: string
  status: 'active' | 'closed'
  openedAt?: string
  remarks?: string
}

export interface ImportResult {
  total: number
  created: number
  updated: number
  failed: number
  issues?: Array<{ row: number; key: string; level: 'error' | 'warning'; reason: string }>
}

export const territoryApi = {
  bigRegions: (keyword?: string) => request<BigRegion[]>({ url: '/big-regions', params: { keyword } }),
  saveBigRegion: (data: Partial<BigRegion>, id?: number) =>
    request<BigRegion>({ url: id ? `/big-regions/${id}` : '/big-regions', method: id ? 'PUT' : 'POST', data }),
  removeBigRegion: (id: number) => request({ url: `/big-regions/${id}`, method: 'DELETE' }),

  regions: (params: { keyword?: string; bigRegionId?: number }) =>
    request<TerritoryRegion[]>({ url: '/regions', params }),
  saveRegion: (data: Partial<TerritoryRegion>, id?: number) =>
    request<TerritoryRegion>({ url: id ? `/regions/${id}` : '/regions', method: id ? 'PUT' : 'POST', data }),
  removeRegion: (id: number) => request({ url: `/regions/${id}`, method: 'DELETE' }),

  stores: (params: {
    keyword?: string; bigRegionCode?: string; regionCode?: string; status?: string;
    page?: number; pageSize?: number
  }) => request<PageData<Store>>({ url: '/stores', params }),
  saveStore: (data: Partial<Store>, id?: number) =>
    request<Store>({ url: id ? `/stores/${id}` : '/stores', method: id ? 'PUT' : 'POST', data }),
  removeStore: (id: number) => request({ url: `/stores/${id}`, method: 'DELETE' }),
  exportStores: () => download({ url: '/stores/export' }, '门店列表.xlsx'),
  storeTemplate: () => download({ url: '/stores/template' }, '门店导入模板.xlsx'),
  importStores: (file: File) => uploadFile<ImportResult>('/stores/import', file),
  storeImportReport: (file: File) => downloadUpload('/stores/import-report', file, '门店导入核对报告.xlsx'),

  exportOrg: () => download({ url: '/admin/org/export' }, '组织架构.xlsx'),
  importOrg: (file: File) => uploadFile<ImportResult>('/admin/org/import', file),
  orgImportReport: (file: File) => downloadUpload('/admin/org/import-report', file, '组织架构导入核对报告.xlsx'),
}

export interface ParamSuggestion {
  name: string
  label: string
  type: 'string' | 'number' | 'bool' | 'date' | 'list'
  required: boolean
  defaultValue: string
  reason?: string
}

export interface TestSQLResult {
  columns: ColumnMeta[]
  rows: any[][]
  rowCount: number
  truncated: boolean
  elapsedMs: number
  paramSuggestions: ParamSuggestion[]
}

export const aiApi = {
  status: () => request<{ enabled: boolean; model?: string; baseUrl?: string }>({ url: '/ai/status' }),
  ping: () => request<{ message: string }>({ url: '/admin/ai/ping', method: 'POST' }),
  testSQL: (data: {
    dataSourceId: number; sqlText: string; params?: any[];
    paramValues?: Record<string, string>; limit?: number
  }) => request<TestSQLResult>({ url: '/datasets/test-sql', method: 'POST', data }),
  inferParams: (sqlText: string, params: any[]) =>
    request<ParamSuggestion[]>({ url: '/datasets/infer-params', method: 'POST', data: { sqlText, params } }),
  generateSQL: (dataSourceId: number, requirement: string, sqlText = '') =>
    request<{ sqlText: string }>({ url: '/ai/sql', method: 'POST', data: { dataSourceId, requirement, sqlText } }),
  completeSQL: (dataSourceId: number, sqlText: string, cursor: number) =>
    request<{ completion: string }>({ url: '/ai/complete', method: 'POST', data: { dataSourceId, sqlText, cursor } }),
  suggestParams: (sqlText: string, params: any[]) =>
    request<ParamSuggestion[]>({ url: '/ai/params', method: 'POST', data: { sqlText, params } }),
  suggestMetadata: (sqlText: string, columns: ColumnMeta[]) =>
    request<{ name: string; description: string }>({ url: '/ai/metadata', method: 'POST', data: { sqlText, columns } }),
}

export interface ConfigField {
  key: string
  label: string
  section: string
  kind: 'string' | 'int' | 'bool'
  value: string
  secret: boolean
  set: boolean
  hint?: string
  options?: string[]
}
export interface ConfigSection {
  name: string
  label: string
  fields: ConfigField[]
}

export const configApi = {
  list: () => request<{ sections: ConfigSection[] }>({ url: '/admin/config' }),
  update: (values: Record<string, string>) =>
    request({ url: '/admin/config', method: 'PUT', data: { values } }),
  reset: (keys: string[]) =>
    request({ url: '/admin/config/reset', method: 'POST', data: { keys } }),
}

export const auditApi = {
  list: (params: { page?: number; pageSize?: number; action?: string }) =>
    request<PageData<{ id: number; userId: number; action: string; resourceType: string; resourceId: number; ip: string; createdAt: string }>>({
      url: '/admin/audit-logs',
      params,
    }),
}

export { http }
