export interface ApiResponse<T = any> {
  code: number
  message: string
  data: T
}

export interface PageData<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}

export interface User {
  id: number
  username: string
  displayName: string
  email: string
  role: 'admin' | 'user'
  enabled: boolean
  lastLoginAt?: string
  createdAt: string
}

export interface DataSource {
  id: number
  name: string
  type: 'mysql' | 'postgres' | 'sqlite'
  host: string
  port: number
  database: string
  username: string
  dsnExtra: string
  optionsJson: string
  hasPassword: boolean
  lastTestedAt?: string
  lastTestStatus?: string
  lastTestMessage?: string
  createdAt: string
}

export interface DatasetParam {
  id?: number
  name: string
  label: string
  type: 'string' | 'number' | 'bool' | 'date'
  required: boolean
  defaultValue: string
}

export interface Dataset {
  id: number
  name: string
  description: string
  dataSourceId: number
  sqlText: string
  version: number
  defaultLimit: number
  columnMetaJson: string
  params: DatasetParam[]
  createdAt: string
  updatedAt: string
}

export interface ColumnMeta {
  name: string
  dbType: string
  logicalType: 'string' | 'number' | 'bool' | 'date' | 'datetime'
  nullable: boolean
  format?: 'plain' | 'percent' | 'currency' | 'number'
}

export interface ResultSet {
  columns: ColumnMeta[]
  rows: any[][]
  rowCount: number
  truncated: boolean
  elapsedMs: number
}

export interface FieldRef {
  field: string
  alias: string
  grain?: 'day' | 'week' | 'month' | 'quarter' | 'year' | ''
}

export interface MetricRef {
  field: string
  agg: 'sum' | 'avg' | 'count' | 'count_distinct' | 'min' | 'max'
  alias: string
  /** Calculated metric: expression over other metric aliases, e.g. "sales_sum / NULLIF(target_sum, 0)". */
  expr?: string
  format?: 'plain' | 'percent' | 'currency' | 'number'
}

export interface SortSpec {
  alias: string
  desc: boolean
}

export interface ChartConfig {
  chartType: string
  dataMode: 'aggregate' | 'raw'
  dimensions: FieldRef[]
  metrics: MetricRef[]
  sort: SortSpec[]
  limit: number
  paramDefaults: Record<string, string>
  format: { decimals: number; numberFormat: string }
  echarts: Record<string, any>
}

export interface Chart {
  id: number
  name: string
  description: string
  datasetId: number
  type: 'bar' | 'line' | 'pie' | 'table' | 'kpi'
  configJson: string
  version: number
  createdAt: string
}

export interface DashboardWidget {
  id: number
  dashboardId: number
  widgetType: 'chart' | 'text'
  chartId?: number
  title: string
  x: number
  y: number
  w: number
  h: number
  optionsJson: string
}

export interface DashboardFilter {
  id: number
  dashboardId: number
  name: string
  label: string
  filterType: 'select' | 'date' | 'daterange' | 'number' | 'text'
  datasetId?: number
  valueColumn: string
  labelColumn: string
  defaultValueJson: string
  multiple?: boolean
  x: number
  y: number
  w: number
  h: number
}

export interface FilterBinding {
  id?: number
  filterId: number
  widgetId: number
  paramName: string
  valueIndex: number
}

export interface Dashboard {
  id: number
  name: string
  description: string
  version: number
  ownerId: number
  createdAt: string
  updatedAt: string
  widgets?: DashboardWidget[]
  filters?: DashboardFilter[]
}

export interface WidgetResult {
  result?: ResultSet
  error?: string
}

export interface DashboardData {
  widgets: Record<string, WidgetResult>
  elapsedMs: number
}

export interface ShareLink {
  id: number
  token?: string
  expiresAt?: string
  enabled: boolean
  createdAt: string
}

export interface Grant {
  id: number
  dashboardId: number
  userId: number
  permission: 'view' | 'edit'
  createdAt: string
}

export interface ScheduleTask {
  id: number
  name: string
  dashboardId: number
  cronExpr: string
  timezone: string
  enabled: boolean
  deliveryChannel: string
  messageFormat?: 'card' | 'text'
  linkExpireHours?: number
  format: string
  recipientsJson: string
  nextRunAt?: string
  lastRunStatus?: string
  lastRunAt?: string
  createdAt: string
}

export interface ScheduleRun {
  id: number
  taskId: number
  triggeredBy: string
  startedAt: string
  finishedAt?: string
  status: 'running' | 'success' | 'failed'
  errorMessage: string
  filePath: string
  fileSize: number
}

export interface PublicDashboard {
  name: string
  widgets: Array<{
    id: number
    widgetType: string
    chartId?: number
    title: string
    x: number
    y: number
    w: number
    h: number
    chartType?: string
    chartConfig?: string
  }>
  filters: Array<{
    id: number
    name: string
    label: string
    filterType: string
    hasOptions: boolean
    multiple?: boolean
    defaultValueJson: string
  }>
  bindings: FilterBinding[]
}

export interface FilterOption {
  value: string
  label: string
}
