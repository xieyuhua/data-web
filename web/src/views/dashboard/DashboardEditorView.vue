<!-- 看板编辑器：栅格布局组件，配置指标、图表、筛选器与看板设置（下钻、组织范围、自动刷新）。 -->
<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ArrowLeft,
  BarChart3,
  ChartArea,
  ChartBar,
  ChartColumnStacked,
  ChartNoAxesCombined,
  ChartScatter,
  ChevronDown,
  ChevronUp,
  Copy,
  Eye,
  Gauge,
  Hash,
  LineChart,
  PieChart,
  Plus,
  Save,
  ShieldCheck,
  Sparkles,
  Table2,
  Trash2,
  Type,
} from 'lucide-vue-next'
import { ElMessage } from 'element-plus'
import { api, getErrorMessage } from '../../api/client'
import EmptyBlock from '../../components/EmptyBlock.vue'
import OrgTreeSelect from '../../components/OrgTreeSelect.vue'

const route = useRoute()
const router = useRouter()

const dashboardId = ref(route.params.id ? Number(route.params.id) : 0)
const loading = ref(true)
const saving = ref(false)
const datasets = ref([])
const selectedId = ref('')
const previewColumns = ref([])
const previewLoading = ref(false)
const fieldError = ref('')
const roles = ref([])
const users = ref([])
const orgTree = ref([])

// 看板权限与下钻设置
const access = reactive({
  visibility_mode: 'all',
  visible_roles: [],
  visible_user_ids: [],
  visible_org_ids: [],
  scope_mode: 'role_org',
  drill_enabled: true,
  drill_mode: 'level',
})

const meta = reactive({
  name: '未命名看板',
  code: '',
  description: '',
  category: '经营分析',
  status: 'draft',
})

const layout = reactive({
  settings: { refresh_seconds: 0, show_title: true },
  widgets: [],
})

const COMPONENTS = [
  { type: 'kpi', label: '指标卡', icon: Hash },
  { type: 'line', label: '折线图', icon: LineChart },
  { type: 'area', label: '面积图', icon: ChartArea },
  { type: 'bar', label: '柱状图', icon: ChartBar },
  { type: 'bar_h', label: '条形图', icon: BarChart3 },
  { type: 'bar_stacked', label: '堆叠柱', icon: ChartColumnStacked },
  { type: 'combo', label: '组合图', icon: ChartNoAxesCombined },
  { type: 'scatter', label: '散点图', icon: ChartScatter },
  { type: 'donut', label: '环图', icon: PieChart },
  { type: 'gauge', label: '目标达成', icon: Gauge },
  { type: 'table', label: '明细表', icon: Table2 },
  { type: 'text', label: '文本', icon: Type },
]

const CHART_LABELS = {
  kpi: '指标卡',
  line: '折线图',
  area: '面积图',
  bar: '柱状图',
  bar_h: '条形图',
  bar_stacked: '堆叠柱',
  combo: '组合图',
  scatter: '散点图',
  donut: '环图',
  gauge: '目标达成',
  table: '明细表',
  text: '文本',
}

const FIELD_LABELS = {
  revenue: '销售额',
  gross_profit: '毛利额',
  orders: '订单量',
  customers: '客流',
  gross_margin: '毛利率',
  date: '日期',
  profit: '毛利',
  region: '区域',
  store: '门店',
  manager: '店长',
  channel: '渠道',
}

const selected = computed(() => layout.widgets.find((item) => item.id === selectedId.value) || null)
const isEdit = computed(() => dashboardId.value > 0)
const totalWidgets = computed(() => layout.widgets.length)
const datasetOptions = computed(() => datasets.value.map((item) => ({ label: item.name, value: item.id })))
const editorFilters = computed(() => (Array.isArray(layout.settings.filters) ? layout.settings.filters : []))

// 全局筛选器在编辑器内以 optionsText 编辑，保存时再拆分为数组。
function normalizeFilter(item = {}) {
  return {
    key: item.key || '',
    label: item.label || '',
    type: ['select', 'multiselect', 'date', 'input'].includes(item.type) ? item.type : 'select',
    optionsText: Array.isArray(item.options) ? item.options.join('\n') : '',
    default: item.default || '',
    placeholder: item.placeholder || '',
    hideOnShare: item.hide_on_share === true,
  }
}

// 解析筛选选项文本（按行或逗号分隔）
function parseFilterOptions(text) {
  return String(text || '')
    .split(/[\n,，;；]+/)
    .map((item) => item.trim())
    .filter(Boolean)
    .slice(0, 50)
}

// 新增筛选器
function addFilter() {
  if (!Array.isArray(layout.settings.filters)) layout.settings.filters = []
  if (layout.settings.filters.length >= 8) {
    ElMessage.warning('单个看板最多配置 8 个全局筛选器')
    return
  }
  layout.settings.filters.push(normalizeFilter())
}

// 删除指定筛选器
function removeFilter(index) {
  if (!Array.isArray(layout.settings.filters)) return
  layout.settings.filters.splice(index, 1)
}

// 组装看板设置（下钻、自动刷新、筛选器）请求体
function buildSettingsPayload() {
  const settings = { ...layout.settings }
  const filters = (layout.settings.filters || [])
    .map((item) => ({
      key: String(item.key || '').trim(),
      label: String(item.label || '').trim(),
      type: ['select', 'multiselect', 'date', 'input'].includes(item.type) ? item.type : 'select',
      options: parseFilterOptions(item.optionsText),
      default: String(item.default || '').trim(),
      placeholder: String(item.placeholder || '').trim(),
      hide_on_share: item.hideOnShare === true,
    }))
    .filter((item) => item.key)
  if (filters.length) {
    settings.filters = filters
  } else {
    delete settings.filters
  }
  return settings
}

// 图表类型转中文名称
function chartLabel(type) {
  return CHART_LABELS[type] || type
}

// 字段名转展示名称
function fieldLabel(field) {
  return FIELD_LABELS[field] || field
}

// 图表类型对应的图标
function chartIcon(type) {
  return COMPONENTS.find((item) => item.type === type)?.icon || Type
}

// 解析看板布局配置
function parseLayout(value) {
  if (!value) return { settings: {}, widgets: [] }
  if (typeof value === 'string') {
    try {
      return JSON.parse(value)
    } catch {
      return { settings: {}, widgets: [] }
    }
  }
  return value
}

// 规范化组件结构，兼容旧版配置
function normalizeWidget(widget) {
  widget.config = widget.config || {}
  widget.w = Number(widget.w) || 12
  widget.h = Number(widget.h) || 4
  widget.dataset_id = Number(widget.dataset_id) || 0
  const config = widget.config
  if (widget.chart_type === 'kpi' && !Array.isArray(widget.config.columns)) {
    widget.config.columns = []
  }
  if (widget.chart_type === 'table' && !Array.isArray(widget.config.columns)) {
    widget.config.columns = []
  }
  if (['line', 'area', 'bar', 'bar_h', 'bar_stacked', 'combo'].includes(widget.chart_type) && !Array.isArray(config.yFields)) {
    if (config.yField) {
      config.yFields = [{
        field: config.yField,
        name: config.yField,
        color: config.color || '#14B8A6',
        type: widget.chart_type === 'line' || widget.chart_type === 'area' ? 'line' : 'bar',
        axis: 0,
      }]
    } else {
      config.yFields = []
    }
  }
  if (widget.chart_type === 'table') {
    config.showSummary = config.showSummary === true
    config.conditional = {
      field: '',
      operator: 'lt',
      threshold: 0,
      ...(config.conditional || {}),
    }
  }
  return widget
}

// 生成组件唯一 ID
function newId(type) {
  return `${type}_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
}

// 按组件类型生成默认配置
function defaultConfig(type) {
  if (type === 'kpi') {
    return {
      columns: [
        { field: 'revenue', label: '销售额', unit: '元', format: 'currency', target: '', compare: '' },
        { field: 'orders', label: '订单量', unit: '笔', format: 'integer', target: '', compare: '' },
        { field: 'customers', label: '客流', unit: '人', format: 'integer', target: '', compare: '' },
        { field: 'gross_margin', label: '毛利率', unit: '%', format: 'number', target: '', compare: '' },
      ],
      footnote: '当前权限范围',
    }
  }
  if (type === 'line') {
    return {
      xField: 'date',
      yFields: [
        { field: 'revenue', name: '销售额', color: '#0F766E' },
        { field: 'profit', name: '毛利', color: '#E8A13A' },
      ],
      unit: '元',
    }
  }
  if (type === 'area') {
    return {
      xField: 'date',
      yFields: [
        { field: 'revenue', name: '销售额', color: '#0F766E', type: 'line' },
        { field: 'profit', name: '毛利', color: '#E8A13A', type: 'line' },
      ],
      unit: '元',
    }
  }
  if (type === 'bar') {
    return {
      xField: 'region',
      yFields: [{ field: 'revenue', name: '销售额', color: '#14B8A6', type: 'bar' }],
      unit: '元',
    }
  }
  if (type === 'bar_h') {
    return {
      xField: 'store',
      yFields: [{ field: 'revenue', name: '销售额', color: '#14B8A6', type: 'bar' }],
      unit: '元',
    }
  }
  if (type === 'bar_stacked') {
    return {
      xField: 'region',
      yFields: [
        { field: 'revenue', name: '销售额', color: '#0F766E', type: 'bar' },
        { field: 'profit', name: '毛利', color: '#E8A13A', type: 'bar' },
      ],
      unit: '元',
    }
  }
  if (type === 'combo') {
    return {
      xField: 'date',
      yFields: [
        { field: 'revenue', name: '销售额', color: '#14B8A6', type: 'bar', axis: 0 },
        { field: 'gross_margin', name: '毛利率', color: '#E8A13A', type: 'line', axis: 1 },
      ],
      unit: '元',
    }
  }
  if (type === 'scatter') {
    return { xField: 'orders', yField: 'revenue', sizeField: '', unit: '元', color: '#0F766E' }
  }
  if (type === 'donut') {
    return { xField: 'channel', yField: 'revenue', unit: '元', rose: false, showLabel: false }
  }
  if (type === 'gauge') {
    return { rateField: '', actualField: '', targetField: '', unit: '%' }
  }
  if (type === 'table') {
    return {
      columns: [
        { field: 'store', label: '门店' },
        { field: 'manager', label: '店长' },
        { field: 'revenue', label: '销售额', format: 'currency' },
        { field: 'orders', label: '订单', format: 'integer' },
      ],
      showSummary: false,
      conditional: { field: '', operator: 'lt', threshold: 0, color: '#C53030', background: '#FDF2F1' },
    }
  }
  return { content: '在这里填写业务说明、指标口径或分析结论。' }
}

// 向看板新增组件
function addWidget(type) {
  const dataset = datasets.value[0]
  const widget = normalizeWidget({
    id: newId(type),
    title: chartLabel(type),
    subtitle: '当前权限范围',
    chart_type: type,
    dataset_id: type === 'text' ? 0 : dataset?.id || 0,
    x: 0,
    y: 0,
    w: type === 'kpi' || type === 'text' ? 12 : 6,
    h: type === 'kpi' || type === 'text' ? 2 : 6,
    config: defaultConfig(type),
  })
  layout.widgets.push(widget)
  selectedId.value = widget.id
  if (widget.dataset_id) loadFields(widget.dataset_id)
}

// 选中组件以编辑属性
function selectWidget(widget) {
  selectedId.value = widget.id
  if (widget.dataset_id) loadFields(widget.dataset_id)
}

// 复制组件
function duplicateWidget(widget) {
  const copy = normalizeWidget(JSON.parse(JSON.stringify(widget)))
  copy.id = newId(copy.chart_type)
  copy.title = `${copy.title} 副本`
  const index = layout.widgets.findIndex((item) => item.id === widget.id)
  layout.widgets.splice(index + 1, 0, copy)
  selectedId.value = copy.id
}

// 删除组件
function removeWidget(widget) {
  const index = layout.widgets.findIndex((item) => item.id === widget.id)
  if (index < 0) return
  layout.widgets.splice(index, 1)
  if (selectedId.value === widget.id) {
    selectedId.value = layout.widgets[Math.max(0, index - 1)]?.id || ''
  }
}

// 调整组件顺序
function moveWidget(widget, offset) {
  const index = layout.widgets.findIndex((item) => item.id === widget.id)
  const target = index + offset
  if (index < 0 || target < 0 || target >= layout.widgets.length) return
  const [item] = layout.widgets.splice(index, 1)
  layout.widgets.splice(target, 0, item)
}

// 加载数据集字段列表
async function loadFields(datasetId) {
  if (!datasetId) {
    previewColumns.value = []
    return
  }
  previewLoading.value = true
  fieldError.value = ''
  try {
    const data = await api.get(`/datasets/${datasetId}/preview`)
    previewColumns.value = data?.columns || []
  } catch (err) {
    previewColumns.value = []
    fieldError.value = getErrorMessage(err)
  } finally {
    previewLoading.value = false
  }
}

// 切换数据集后重置字段配置
function onDatasetChange(widget, datasetId) {
  widget.dataset_id = Number(datasetId) || 0
  loadFields(widget.dataset_id)
}

// 切换组件图表类型
function switchChartType(widget, type) {
  widget.chart_type = type
  const config = widget.config || {}
  if ((type === 'kpi' || type === 'table') && !Array.isArray(config.columns)) {
    config.columns = []
  }
  if (['line', 'area', 'bar', 'bar_h', 'bar_stacked', 'combo'].includes(type) && !Array.isArray(config.yFields)) {
    config.yFields = []
  }
  if (['bar', 'bar_h', 'bar_stacked', 'combo', 'scatter', 'donut', 'line', 'area'].includes(type) && !config.xField) {
    config.xField = previewColumns.value[0] || ''
    config.yField = previewColumns.value[1] || ''
  }
  if (type === 'combo' && Array.isArray(config.yFields)) {
    config.yFields = config.yFields.map((item, index) => ({
      ...item,
      type: item.type || (index === 0 ? 'bar' : 'line'),
      axis: item.axis === 1 ? 1 : 0,
    }))
  }
  if (type === 'table') {
    config.showSummary = config.showSummary === true
    config.conditional = { field: '', operator: 'lt', threshold: 0, ...(config.conditional || {}) }
  }
  if (type === 'gauge') {
    config.rateField = config.rateField || ''
    config.actualField = config.actualField || ''
    config.targetField = config.targetField || ''
    config.unit = config.unit || '%'
  }
  if (type === 'text' && typeof config.content !== 'string') {
    config.content = ''
  }
  widget.config = config
}

// 按字段自动生成默认图表配置
function autoConfig(widget) {
  const columns = previewColumns.value
  if (!columns.length) {
    ElMessage.warning('请先选择并载入数据集字段')
    return
  }
  if (widget.chart_type === 'kpi') {
    const numeric = columns.filter((field) => !/date|time|month|day|period/i.test(field))
    widget.config.columns = numeric.slice(0, 4).map((field) => ({
      field,
      label: fieldLabel(field),
      unit: '',
      format: /revenue|profit|amount|sales/i.test(field) ? 'currency' : 'number',
      target: '',
      compare: '',
    }))
  } else if (widget.chart_type === 'line' || widget.chart_type === 'area') {
    widget.config.xField = columns[0]
    widget.config.yFields = columns.slice(1, 3).map((field, index) => ({
      field,
      name: fieldLabel(field),
      color: index === 0 ? '#0F766E' : '#E8A13A',
      type: 'line',
    }))
  } else if (['bar', 'bar_h', 'bar_stacked', 'combo'].includes(widget.chart_type)) {
    widget.config.xField = columns[0]
    const numeric = columns.slice(1, widget.chart_type === 'bar_stacked' ? 4 : 3)
    widget.config.yFields = (numeric.length ? numeric : [columns[0]]).map((field, index) => ({
      field,
      name: fieldLabel(field),
      color: ['#0F766E', '#E8A13A', '#4A78B8'][index % 3],
      type: widget.chart_type === 'combo' && index > 0 ? 'line' : 'bar',
      axis: widget.chart_type === 'combo' && index > 0 && ['gross_margin', 'rate', 'percent'].some((key) => field.includes(key)) ? 1 : 0,
    }))
  } else if (widget.chart_type === 'scatter') {
    const numeric = columns.filter((field) => !/date|time|region|store|channel|name/i.test(field))
    widget.config.xField = numeric[0] || columns[0]
    widget.config.yField = numeric[1] || columns[1] || columns[0]
    widget.config.sizeField = numeric[2] || ''
  } else if (widget.chart_type === 'donut') {
    widget.config.xField = columns[0]
    widget.config.yField = columns[1] || columns[0]
  } else if (widget.chart_type === 'gauge') {
    widget.config.rateField = columns.find((field) => /rate|achiev|达成/i.test(field)) || ''
    widget.config.actualField =
      columns.find((field) => /actual|实际|revenue|amount|sales/i.test(field)) || columns[0]
    widget.config.targetField = columns.find((field) => /target|目标/i.test(field)) || columns[1] || ''
  } else if (widget.chart_type === 'table') {
    widget.config.columns = columns.map((field) => ({
      field,
      label: fieldLabel(field),
      format: /revenue|profit|amount|sales/i.test(field) ? 'currency' : 'text',
      progress: false,
    }))
    widget.config.showSummary = true
    widget.config.conditional = {
      field: '',
      operator: 'lt',
      threshold: 0,
      ...(widget.config.conditional || {}),
    }
  }
  ElMessage.success('已按数据集字段自动填充')
}

// 新增 KPI 指标项
function addMetric(widget) {
  widget.config.columns.push({ field: '', label: '', unit: '', format: 'number', target: '', compare: '' })
}

// 新增图表指标系列
function addYField(widget) {
  widget.config.yFields.push({
    field: '',
    name: '',
    color: '#0F766E',
    type: widget.chart_type === 'combo' ? 'bar' : widget.chart_type === 'line' || widget.chart_type === 'area' ? 'line' : 'bar',
    axis: 0,
  })
}

// 新增表格列配置
function addTableColumn(widget) {
  widget.config.columns.push({ field: '', label: '', format: 'text', progress: false })
}

// 从数组指定位置移除一项
function removeAt(list, index) {
  list.splice(index, 1)
}

// 计算组件高度
function widgetHeight(widget) {
  return `${Math.max(120, Number(widget.h || 4) * 34)}px`
}

// 计算组件栅格样式
function widgetStyle(widget) {
  return {
    gridColumn: `span ${Math.min(12, Math.max(1, Number(widget.w) || 12))}`,
    '--widget-height': widgetHeight(widget),
  }
}

// 返回上一页（有未保存提示）
function goBack() {
  router.push(isEdit.value ? `/dashboards/${dashboardId.value}` : '/dashboards')
}

// 打开看板预览
function openPreview() {
  if (!isEdit.value) {
    ElMessage.info('请先保存看板后再预览')
    return
  }
  window.open(`/dashboards/${dashboardId.value}`, '_blank')
}

// 加载看板详情用于编辑
async function load() {
  loading.value = true
  try {
    const [datasetList, detail, roleList, userList, tree] = await Promise.all([
      api.get('/datasets'),
      isEdit.value ? api.get(`/dashboards/${dashboardId.value}`) : Promise.resolve(null),
      api.get('/roles').catch(() => []),
      api.get('/users').catch(() => []),
      api.get('/orgs/tree').catch(() => []),
    ])
    datasets.value = datasetList || []
    roles.value = roleList || []
    users.value = userList || []
    orgTree.value = tree || []
    if (detail) {
      meta.name = detail.name
      meta.code = detail.code
      meta.description = detail.description || ''
      meta.category = detail.category || '经营分析'
      meta.status = detail.status || 'draft'
      const parsed = parseLayout(detail.layout)
      layout.settings = { refresh_seconds: 0, show_title: true, ...(parsed.settings || {}) }
      layout.settings.filters = Array.isArray(layout.settings.filters)
        ? layout.settings.filters.map(normalizeFilter)
        : []
      layout.widgets = (parsed.widgets || []).map(normalizeWidget)
      access.visibility_mode = detail.visibility_mode || 'all'
      access.visible_roles = Array.isArray(detail.visible_roles) ? [...detail.visible_roles] : []
      access.visible_user_ids = Array.isArray(detail.visible_user_ids) ? [...detail.visible_user_ids] : []
      access.visible_org_ids = Array.isArray(detail.visible_org_ids) ? [...detail.visible_org_ids] : []
      access.scope_mode = detail.scope_mode || 'role_org'
      access.drill_enabled = detail.drill_enabled !== false
      access.drill_mode = detail.drill_mode || 'level'
      if (layout.widgets.length) {
        selectedId.value = layout.widgets[0].id
        if (layout.widgets[0].dataset_id) loadFields(layout.widgets[0].dataset_id)
      }
    }
  } catch (err) {
    ElMessage.error(getErrorMessage(err))
  } finally {
    loading.value = false
  }
}

// 保存看板（草稿 / 发布）
async function save(status) {
  if (!meta.name.trim()) {
    ElMessage.warning('请填写看板名称')
    return
  }
  saving.value = true
  try {
    if (!meta.code) {
      meta.code = `dash_${Date.now()}`
    }
    const payload = {
      name: meta.name.trim(),
      code: meta.code.trim(),
      description: meta.description,
      category: meta.category || '经营分析',
      theme: 'light',
      status,
      visibility_mode: access.visibility_mode,
      visible_roles: access.visibility_mode === 'roles' ? access.visible_roles : [],
      visible_user_ids: access.visibility_mode === 'users' ? access.visible_user_ids : [],
      visible_org_ids: access.visibility_mode === 'orgs' ? access.visible_org_ids : [],
      scope_mode: access.scope_mode,
      drill_enabled: access.drill_enabled,
      drill_mode: access.drill_mode,
      layout: {
        settings: buildSettingsPayload(),
        widgets: layout.widgets.map((item) => JSON.parse(JSON.stringify(item))),
      },
    }
    let item
    if (isEdit.value) {
      item = await api.put(`/dashboards/${dashboardId.value}`, payload)
    } else {
      item = await api.post('/dashboards', payload)
      dashboardId.value = item.id
    }
    meta.status = item.status
    meta.code = item.code
    ElMessage.success(status === 'published' ? '看板已发布' : '草稿已保存')
    if (status === 'published') {
      router.push(`/dashboards/${item.id}`)
    } else if (!route.params.id) {
      router.replace(`/dashboards/${item.id}/edit`)
    }
  } catch (err) {
    ElMessage.error(getErrorMessage(err))
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="editor-page">
    <aside class="editor-rail">
      <div class="editor-section-title">组件库</div>
      <div class="component-list">
        <button
          v-for="item in COMPONENTS"
          :key="item.type"
          class="component-button"
          type="button"
          @click="addWidget(item.type)"
        >
          <component :is="item.icon" :size="18" />
          {{ item.label }}
        </button>
      </div>

      <div class="widget-list">
        <div class="editor-section-title">画布组件 · {{ totalWidgets }}</div>
        <div
          v-for="widget in layout.widgets"
          :key="widget.id"
          class="widget-list-item"
          :class="{ active: widget.id === selectedId }"
          @click="selectWidget(widget)"
        >
          <component :is="chartIcon(widget.chart_type)" :size="13" />
          <span>{{ widget.title || chartLabel(widget.chart_type) }}</span>
        </div>
        <p v-if="!layout.widgets.length" style="color: var(--muted-light); font-size: 9px; line-height: 1.7">
          点击上方组件添加到画布。
        </p>
      </div>

      <div class="editor-section-title" style="margin-top: 24px">看板设置</div>
      <div class="property-group">
        <label class="property-label">自动刷新（秒）</label>
        <el-input-number v-model="layout.settings.refresh_seconds" :min="0" :max="3600" :step="15" style="width: 100%" />
        <p style="margin-top: 5px; color: var(--muted-light); font-size: 8px">0 表示关闭，最低 15 秒。</p>
      </div>
      <div class="property-group" style="display: flex; align-items: center; justify-content: space-between">
        <span class="property-label" style="margin: 0">显示看板标题</span>
        <el-switch v-model="layout.settings.show_title" />
      </div>

      <div class="editor-section-title" style="margin-top: 20px">
        全局筛选器 · {{ editorFilters.length }}
      </div>
      <div
        v-for="(filter, index) in editorFilters"
        :key="index"
        class="filter-config-card"
      >
        <div class="inline-actions" style="justify-content: space-between; margin-bottom: 6px">
          <span class="property-label" style="margin: 0">筛选器 {{ index + 1 }}</span>
          <button class="mini-icon" type="button" title="移除筛选器" @click="removeFilter(index)">
            <Trash2 :size="12" />
          </button>
        </div>
        <div class="form-grid" style="gap: 6px">
          <el-input v-model="filter.key" size="small" placeholder="字段名，如 channel" />
          <el-input v-model="filter.label" size="small" placeholder="显示名称，如 渠道" />
          <el-select v-model="filter.type" size="small">
            <el-option value="select" label="下拉选择" />
            <el-option value="multiselect" label="下拉多选" />
            <el-option value="date" label="日期选择" />
            <el-option value="input" label="文本输入" />
          </el-select>
          <el-input
            v-model="filter.default"
            size="small"
            :placeholder="filter.type === 'multiselect' ? '默认值，逗号分隔（可选）' : '默认值（可选）'"
          />
        </div>
        <el-input
          v-if="['select', 'multiselect'].includes(filter.type)"
          v-model="filter.optionsText"
          type="textarea"
          :rows="2"
          size="small"
          placeholder="选项，支持逗号或换行分隔"
          style="margin-top: 6px"
        />
        <el-input
          v-model="filter.placeholder"
          size="small"
          placeholder="未选择时的提示，默认“全部”"
          style="margin-top: 6px"
        />
        <el-checkbox v-model="filter.hideOnShare" size="small" style="margin-top: 6px">
          不在分享页显示（服务端同时忽略该筛选值）
        </el-checkbox>
      </div>
      <button class="btn btn-ghost btn-sm" type="button" style="width: 100%" @click="addFilter">
        <Plus :size="12" /> 添加筛选器
      </button>
      <p style="margin-top: 7px; color: var(--muted-light); font-size: 8px; line-height: 1.7">
        数据集 SQL 用 <code v-text="'{{filter:channel}}'"></code> 接收筛选值，空值写法：
        <code v-text="'AND ({{filter:channel}} = \'\' OR channel = {{filter:channel}})'"></code>。
            多选筛选器用 <code v-text="'{{filter_in:channel}}'"></code> 展开为 <code>'a','b'</code>，配合 <code v-text="'{{filter_has:channel}}'"></code> 判断是否已选择：
        <code v-text="'AND ({{filter_has:channel}} = 0 OR channel IN ({{filter_in:channel}}))'"></code>。
      </p>
    </aside>

    <section class="editor-canvas-wrap">
      <div class="editor-toolbar">
        <div class="inline-actions">
          <button class="icon-btn" type="button" title="返回" @click="goBack"><ArrowLeft :size="16" /></button>
          <div>
            <el-input v-model="meta.name" placeholder="看板名称" style="width: min(340px, 52vw)" />
            <div style="margin-top: 5px; color: var(--muted-light); font-size: 8.5px">
              {{ isEdit ? '编辑已发布看板' : '新看板 · 保存后生成编码' }} · {{ totalWidgets }} 个组件
            </div>
          </div>
        </div>
        <div class="inline-actions">
          <button class="btn btn-secondary btn-sm" type="button" @click="openPreview"><Eye :size="14" /> 预览</button>
          <button class="btn btn-secondary btn-sm" type="button" :disabled="saving" @click="save('draft')">
            <Save :size="14" /> 保存草稿
          </button>
          <button class="btn btn-primary btn-sm" type="button" :disabled="saving" @click="save('published')">
            <Sparkles :size="14" /> {{ saving ? '处理中…' : '发布看板' }}
          </button>
        </div>
      </div>

      <div v-if="loading" class="canvas-grid">
        <div class="skeleton" style="grid-column: span 12; height: 280px" />
      </div>

      <div v-else-if="!layout.widgets.length" class="editor-empty">
        <EmptyBlock title="开始搭建你的经营视图" description="从左侧组件库添加指标卡、折线图、柱状图、环图、明细表或文本说明。">
          <button class="btn btn-primary btn-sm" type="button" @click="addWidget('kpi')">
            <Plus :size="14" /> 添加第一个指标卡
          </button>
        </EmptyBlock>
      </div>

      <div v-else class="canvas-grid">
        <div
          v-for="widget in layout.widgets"
          :key="widget.id"
          class="canvas-widget"
          :class="{ active: widget.id === selectedId }"
          :style="widgetStyle(widget)"
          @click="selectWidget(widget)"
        >
          <div class="canvas-widget-head">
            <div>
              <h3>{{ widget.title || chartLabel(widget.chart_type) }}</h3>
              <p>{{ widget.subtitle || '未填写副标题' }}</p>
            </div>
            <div class="canvas-widget-actions">
              <button class="mini-icon" type="button" title="上移" @click.stop="moveWidget(widget, -1)">
                <ChevronUp :size="12" />
              </button>
              <button class="mini-icon" type="button" title="下移" @click.stop="moveWidget(widget, 1)">
                <ChevronDown :size="12" />
              </button>
              <button class="mini-icon" type="button" title="复制" @click.stop="duplicateWidget(widget)">
                <Copy :size="12" />
              </button>
              <button class="mini-icon" type="button" title="删除" @click.stop="removeWidget(widget)">
                <Trash2 :size="12" />
              </button>
            </div>
          </div>

          <div class="canvas-visual">
            <div v-if="widget.chart_type === 'kpi'" class="mock-kpis">
              <div v-for="index in 4" :key="index" class="mock-kpi"><i /><b /></div>
            </div>
            <p v-else-if="widget.chart_type === 'text'" style="color: var(--muted); font-size: 9.5px; line-height: 1.8; text-align: left">
              {{ widget.config.content || '业务说明' }}
            </p>
            <div
              v-else-if="widget.chart_type === 'donut'"
              style="width: 78px; height: 78px; border-radius: 50%; background: conic-gradient(#0f766e 0 42%, #e8a13a 42% 68%, #4a78b8 68% 84%, #d1dfd8 84% 100%)"
            >
              <div style="width: 44px; height: 44px; margin: 17px; border-radius: 50%; background: white" />
            </div>
            <div v-else-if="widget.chart_type === 'line'" class="mock-line">
              <svg viewBox="0 0 220 78" width="100%" height="78" fill="none">
                <path d="M0 60C20 56 32 34 52 40C72 46 80 20 100 26C120 32 130 52 150 38C170 24 184 12 220 8" stroke="#0F766E" stroke-width="2" stroke-linecap="round" />
                <path d="M0 68C20 66 36 54 56 58C76 62 88 44 108 48C128 52 142 64 162 54C182 44 198 36 220 32" stroke="#E8A13A" stroke-width="1.5" stroke-linecap="round" stroke-dasharray="3 4" />
              </svg>
            </div>
            <div v-else-if="widget.chart_type === 'gauge'" style="width: 112px; height: 58px; overflow: hidden; position: relative">
              <div style="width: 112px; height: 112px; border-radius: 50%; background: conic-gradient(from 270deg, #0f766e 0 55%, #e6ede7 55% 100%)" />
              <div style="position: absolute; left: 14px; top: 14px; width: 84px; height: 84px; border-radius: 50%; background: #fff" />
            </div>
            <div v-else-if="widget.chart_type === 'table'" style="width: 100%; font-size: 8px; color: var(--muted)">
              <div v-for="index in 4" :key="index" style="display: flex; gap: 6px; padding: 5px 0; border-bottom: 1px solid var(--line-soft)">
                <span style="width: 30%">门店 {{ index }}</span>
                <span style="flex: 1; height: 5px; margin-top: 4px; border-radius: 3px; background: #e3ece6" />
                <span style="width: 24%; height: 5px; margin-top: 4px; border-radius: 3px; background: #bfe5db" />
              </div>
            </div>
            <div v-else class="mock-bars">
              <i v-for="(height, index) in [42, 66, 54, 82, 62, 92]" :key="index" :style="{ '--bar': `${height}%` }" />
            </div>
          </div>
        </div>
      </div>
    </section>

    <aside class="editor-properties" style="max-height: calc(100vh - 74px); overflow-y: auto">
      <div class="editor-section-title">看板信息</div>
      <div class="property-group">
        <label class="property-label">看板编码</label>
        <el-input v-model="meta.code" placeholder="保存时自动生成" />
      </div>
      <div class="property-group">
        <label class="property-label">业务分类</label>
        <el-select v-model="meta.category" style="width: 100%" filterable allow-create>
          <el-option label="经营分析" value="经营分析" />
          <el-option label="销售分析" value="销售分析" />
          <el-option label="区域管理" value="区域管理" />
          <el-option label="门店管理" value="门店管理" />
        </el-select>
      </div>
      <div class="property-group">
        <label class="property-label">看板说明</label>
        <el-input v-model="meta.description" type="textarea" :rows="2" placeholder="说明看板用途与指标口径" />
      </div>

      <div class="editor-section-title" style="margin-top: 22px">
        <ShieldCheck :size="12" style="vertical-align: -2px" /> 权限与下钻
      </div>
      <div class="property-group">
        <label class="property-label">可见范围</label>
        <el-select v-model="access.visibility_mode" style="width: 100%">
          <el-option value="all" label="全部登录用户" />
          <el-option value="roles" label="指定角色" />
          <el-option value="users" label="指定人员" />
          <el-option value="orgs" label="指定组织" />
        </el-select>
        <p style="margin-top: 5px; color: var(--muted-light); font-size: 8px">
          仅在可见范围内的用户能在首页看到该看板，分享与推送同样受限制。
        </p>
      </div>
      <div v-if="access.visibility_mode === 'roles'" class="property-group">
        <label class="property-label">可见角色</label>
        <el-select v-model="access.visible_roles" multiple collapse-tags style="width: 100%" placeholder="选择角色">
          <el-option v-for="role in roles" :key="role.code" :value="role.code" :label="role.name" />
        </el-select>
      </div>
      <div v-if="access.visibility_mode === 'users'" class="property-group">
        <label class="property-label">可见人员</label>
        <el-select v-model="access.visible_user_ids" multiple filterable collapse-tags style="width: 100%" placeholder="选择人员">
          <el-option v-for="user in users" :key="user.id" :value="user.id" :label="`${user.name} · ${user.username}`" />
        </el-select>
      </div>
      <div v-if="access.visibility_mode === 'orgs'" class="property-group">
        <label class="property-label">可见组织</label>
        <OrgTreeSelect v-model="access.visible_org_ids" multiple :data="orgTree" placeholder="选择组织" />
        <p style="margin-top: 5px; color: var(--muted-light); font-size: 8px">
          组织内用户及其下级组织成员可见。
        </p>
      </div>
      <div class="property-group">
        <label class="property-label">数据范围</label>
        <el-select v-model="access.scope_mode" style="width: 100%">
          <el-option value="role_org" label="按组织层级（默认）" />
          <el-option value="all" label="全量数据" />
        </el-select>
        <p style="margin-top: 5px; color: var(--muted-light); font-size: 8px">
          全量数据模式下，有权限的查看者看到所有组织汇总。
        </p>
      </div>
      <div class="property-group" style="display: flex; align-items: center; justify-content: space-between">
        <span class="property-label" style="margin: 0">组织下钻</span>
        <el-switch v-model="access.drill_enabled" />
      </div>
      <div v-if="access.drill_enabled" class="property-group">
        <label class="property-label">下钻默认模式</label>
        <el-select v-model="access.drill_mode" style="width: 100%">
          <el-option value="level" label="逐级下钻" />
          <el-option value="all" label="全部门店平铺" />
          <el-option value="employees" label="全部员工" />
        </el-select>
      </div>

      <div class="editor-section-title" style="margin-top: 22px">
        {{ selected ? '组件属性' : '未选择组件' }}
      </div>

      <template v-if="selected">
        <div class="property-group">
          <label class="property-label">组件标题</label>
          <el-input v-model="selected.title" placeholder="组件标题" />
        </div>
        <div class="property-group">
          <label class="property-label">副标题</label>
          <el-input v-model="selected.subtitle" placeholder="例如：当前权限范围" />
        </div>
        <div class="property-group">
          <label class="property-label">图表类型</label>
          <div class="segmented" style="grid-template-columns: repeat(3, 1fr)">
            <button
              v-for="item in COMPONENTS"
              :key="item.type"
              type="button"
              :class="{ active: selected.chart_type === item.type }"
              @click="switchChartType(selected, item.type)"
            >
              {{ item.label }}
            </button>
          </div>
        </div>
        <div v-if="selected.chart_type !== 'kpi' && selected.chart_type !== 'text'" class="property-group">
          <label class="property-label">默认视图</label>
          <div class="segmented" style="grid-template-columns: repeat(3, 1fr)">
            <button
              type="button"
              :class="{ active: !selected.config.viewMode }"
              @click="selected.config.viewMode = ''"
            >
              自动
            </button>
            <button
              type="button"
              :class="{ active: selected.config.viewMode === 'table' }"
              @click="selected.config.viewMode = 'table'"
            >
              表格
            </button>
            <button
              type="button"
              :class="{ active: selected.config.viewMode === 'chart' }"
              @click="selected.config.viewMode = 'chart'"
            >
              图表
            </button>
          </div>
          <p style="margin-top: 6px; color: var(--muted-light); font-size: 8.5px; line-height: 1.6">
            自动：1 行数据默认表格，多行数据默认图表；阅读时仍可随时切换。
          </p>
        </div>
        <div
          v-if="editorFilters.length && !['kpi', 'text', 'table'].includes(selected.chart_type)"
          class="property-group"
        >
          <label class="property-label">点击联动筛选器</label>
          <el-select
            v-model="selected.config.linkFilter"
            style="width: 100%"
            clearable
            placeholder="不联动（点击查看明细）"
          >
            <el-option
              v-for="filter in editorFilters"
              :key="filter.key || filter.label"
              :label="`${filter.label || filter.key}（${filter.key || '未命名'}）`"
              :value="filter.key"
              :disabled="!filter.key"
            />
          </el-select>
          <p style="margin-top: 6px; color: var(--muted-light); font-size: 8.5px; line-height: 1.6">
            点击图表分类时自动写入该全局筛选器，整份看板一起刷新，实现图表联动。
          </p>
        </div>
        <div class="property-group">
          <label class="property-label">数据源</label>
          <el-select
            :model-value="selected.dataset_id"
            style="width: 100%"
            placeholder="选择 SQL 数据集"
            :disabled="selected.chart_type === 'text'"
            @update:model-value="onDatasetChange(selected, $event)"
          >
            <el-option v-for="item in datasetOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <div v-if="selected.chart_type !== 'text'" class="inline-actions" style="margin-top: 7px">
            <button class="btn btn-ghost btn-sm" type="button" @click="loadFields(selected.dataset_id)">
              {{ previewLoading ? '载入中…' : '载入字段' }}
            </button>
            <button class="btn btn-ghost btn-sm" type="button" @click="autoConfig(selected)">
              <Sparkles :size="12" /> 自动填充
            </button>
          </div>
          <div v-if="previewColumns.length" style="display: flex; flex-wrap: wrap; gap: 4px; margin-top: 7px">
            <span v-for="column in previewColumns" :key="column" class="tag">{{ column }}</span>
          </div>
          <p v-if="fieldError" style="margin-top: 6px; color: #b4544b; font-size: 8.5px">{{ fieldError }}</p>
        </div>

        <div
          v-if="['line', 'area', 'bar', 'bar_h', 'bar_stacked', 'combo', 'donut'].includes(selected.chart_type)"
          class="property-group"
        >
          <label class="property-label">排序与 Top N</label>
          <div class="inline-actions">
            <el-select v-model="selected.config.sortOrder" size="small" clearable placeholder="保持查询顺序" style="flex: 1">
              <el-option label="按指标升序" value="asc" />
              <el-option label="按指标降序" value="desc" />
            </el-select>
            <el-input-number
              v-model="selected.config.topN"
              :min="0"
              :max="100"
              size="small"
              controls-position="right"
              style="width: 108px"
            />
          </div>
          <p style="margin-top: 6px; color: var(--muted-light); font-size: 8.5px; line-height: 1.6">
            排序字段默认取第一个指标系列；Top N 填 0 表示不限制，适合排行榜和重点分类展示。
          </p>
        </div>

        <template v-if="selected.chart_type === 'kpi'">
          <div class="property-group">
            <div class="inline-actions" style="justify-content: space-between; margin-bottom: 7px">
              <span class="property-label" style="margin: 0">指标字段</span>
              <button class="mini-icon" type="button" title="添加指标" @click="addMetric(selected)"><Plus :size="12" /></button>
            </div>
            <div v-for="(metric, index) in selected.config.columns" :key="index" style="padding: 8px; margin-bottom: 7px; border: 1px solid var(--line-soft); border-radius: 8px">
              <div class="form-grid" style="gap: 6px">
                <el-select v-model="metric.field" size="small" filterable allow-create placeholder="字段">
                  <el-option v-for="column in previewColumns" :key="column" :label="column" :value="column" />
                </el-select>
                <el-input v-model="metric.label" size="small" placeholder="显示名称" />
                <el-select v-model="metric.format" size="small">
                  <el-option label="金额" value="currency" />
                  <el-option label="整数" value="integer" />
                  <el-option label="小数" value="number" />
                  <el-option label="百分比" value="percent" />
                  <el-option label="文本" value="text" />
                </el-select>
                <el-input v-model="metric.unit" size="small" placeholder="单位" />
              </div>
              <div class="inline-actions" style="margin-top: 6px">
                <el-select v-model="metric.target" size="small" clearable filterable placeholder="目标字段（可选）" style="flex: 1">
                  <el-option v-for="column in previewColumns" :key="column" :label="column" :value="column" />
                </el-select>
                <el-select v-model="metric.compare" size="small" clearable filterable placeholder="对比字段（可选）" style="flex: 1">
                  <el-option v-for="column in previewColumns" :key="column" :label="column" :value="column" />
                </el-select>
              </div>
              <button class="btn btn-ghost btn-sm" type="button" style="margin-top: 4px" @click="removeAt(selected.config.columns, index)">
                移除
              </button>
            </div>
          </div>
          <div class="property-group">
            <label class="property-label">卡片脚注</label>
            <el-input v-model="selected.config.footnote" placeholder="例如：数据口径 / 更新时间" />
          </div>
        </template>

        <template v-else-if="['line', 'area', 'bar', 'bar_h', 'bar_stacked', 'combo'].includes(selected.chart_type)">
          <div class="property-group">
            <label class="property-label">X 轴字段</label>
            <el-select v-model="selected.config.xField" style="width: 100%" filterable allow-create placeholder="日期或分类字段">
              <el-option v-for="column in previewColumns" :key="column" :label="column" :value="column" />
            </el-select>
          </div>
          <div class="property-group">
            <div class="inline-actions" style="justify-content: space-between; margin-bottom: 7px">
              <span class="property-label" style="margin: 0">指标系列</span>
              <button class="mini-icon" type="button" title="添加指标" @click="addYField(selected)"><Plus :size="12" /></button>
            </div>
            <div v-for="(item, index) in selected.config.yFields" :key="index" style="padding: 8px; margin-bottom: 7px; border: 1px solid var(--line-soft); border-radius: 8px">
              <el-select v-model="item.field" size="small" filterable allow-create placeholder="指标字段" style="width: 100%">
                <el-option v-for="column in previewColumns" :key="column" :label="column" :value="column" />
              </el-select>
              <div class="inline-actions" style="margin-top: 6px">
                <el-input v-model="item.name" size="small" placeholder="图例名称" />
                <el-color-picker v-model="item.color" size="small" />
                <button class="mini-icon" type="button" @click="removeAt(selected.config.yFields, index)"><Trash2 :size="11" /></button>
              </div>
              <div v-if="selected.chart_type === 'combo'" class="inline-actions" style="margin-top: 6px">
                <el-select v-model="item.type" size="small" style="width: 96px">
                  <el-option label="柱形" value="bar" />
                  <el-option label="折线" value="line" />
                </el-select>
                <el-select v-model="item.axis" size="small" style="width: 96px">
                  <el-option label="左轴" :value="0" />
                  <el-option label="右轴" :value="1" />
                </el-select>
              </div>
            </div>
            <p v-if="!selected.config.yFields?.length" style="color: var(--muted-light); font-size: 8.5px">
              添加指标后即可展示趋势；也可以点击“自动填充”。
            </p>
          </div>
          <div class="property-group" style="display: flex; align-items: center; justify-content: space-between">
            <span class="property-label" style="margin: 0">显示数值标签</span>
            <el-switch v-model="selected.config.showLabel" />
          </div>
          <div
            v-if="['line', 'area', 'combo'].includes(selected.chart_type)"
            class="property-group"
            style="display: flex; align-items: center; justify-content: space-between"
          >
            <span class="property-label" style="margin: 0">平滑曲线</span>
            <el-switch :model-value="selected.config.smooth !== false" @update:model-value="selected.config.smooth = $event" />
          </div>
          <div class="property-group">
            <label class="property-label">单位</label>
            <el-input v-model="selected.config.unit" placeholder="元" />
          </div>
        </template>

        <template v-else-if="selected.chart_type === 'donut'">
          <div class="property-group">
            <label class="property-label">分类字段</label>
            <el-select v-model="selected.config.xField" style="width: 100%" filterable allow-create>
              <el-option v-for="column in previewColumns" :key="column" :label="column" :value="column" />
            </el-select>
          </div>
          <div class="property-group">
            <label class="property-label">数值字段</label>
            <el-select v-model="selected.config.yField" style="width: 100%" filterable allow-create>
              <el-option v-for="column in previewColumns" :key="column" :label="column" :value="column" />
            </el-select>
          </div>
          <div class="property-group">
            <label class="property-label">单位</label>
            <el-input v-model="selected.config.unit" placeholder="元" />
          </div>
          <div class="property-group" style="display: flex; align-items: center; justify-content: space-between">
            <span class="property-label" style="margin: 0">玫瑰图</span>
            <el-switch v-model="selected.config.rose" />
          </div>
          <div class="property-group" style="display: flex; align-items: center; justify-content: space-between">
            <span class="property-label" style="margin: 0">显示占比标签</span>
            <el-switch v-model="selected.config.showLabel" />
          </div>
        </template>

        <template v-else-if="selected.chart_type === 'scatter'">
          <div class="property-group">
            <label class="property-label">X 轴数值字段</label>
            <el-select v-model="selected.config.xField" style="width: 100%" filterable allow-create>
              <el-option v-for="column in previewColumns" :key="column" :label="column" :value="column" />
            </el-select>
          </div>
          <div class="property-group">
            <label class="property-label">Y 轴数值字段</label>
            <el-select v-model="selected.config.yField" style="width: 100%" filterable allow-create>
              <el-option v-for="column in previewColumns" :key="column" :label="column" :value="column" />
            </el-select>
          </div>
          <div class="property-group">
            <label class="property-label">气泡大小字段（可选）</label>
            <el-select v-model="selected.config.sizeField" clearable style="width: 100%" filterable allow-create>
              <el-option v-for="column in previewColumns" :key="column" :label="column" :value="column" />
            </el-select>
          </div>
          <div class="property-group">
            <label class="property-label">单位</label>
            <el-input v-model="selected.config.unit" placeholder="元" />
          </div>
        </template>

        <template v-else-if="selected.chart_type === 'gauge'">
          <div class="property-group">
            <label class="property-label">达成率字段（可选）</label>
            <el-select v-model="selected.config.rateField" clearable filterable allow-create style="width: 100%" placeholder="直接使用达成率列">
              <el-option v-for="column in previewColumns" :key="column" :label="column" :value="column" />
            </el-select>
          </div>
          <div class="property-group">
            <label class="property-label">实际值字段</label>
            <el-select v-model="selected.config.actualField" clearable filterable allow-create style="width: 100%">
              <el-option v-for="column in previewColumns" :key="column" :label="column" :value="column" />
            </el-select>
          </div>
          <div class="property-group">
            <label class="property-label">目标值字段</label>
            <el-select v-model="selected.config.targetField" clearable filterable allow-create style="width: 100%">
              <el-option v-for="column in previewColumns" :key="column" :label="column" :value="column" />
            </el-select>
          </div>
          <div class="property-group">
            <label class="property-label">单位</label>
            <el-input v-model="selected.config.unit" placeholder="元" />
          </div>
          <p style="color: var(--muted); font-size: 8.5px; line-height: 1.7">
            优先使用达成率字段；未配置时按「实际值 / 目标值 × 100%」计算，取结果第一行。
          </p>
        </template>

        <template v-else-if="selected.chart_type === 'table'">
          <div class="property-group">
            <div class="inline-actions" style="justify-content: space-between; margin-bottom: 7px">
              <span class="property-label" style="margin: 0">表格列</span>
              <button class="mini-icon" type="button" title="添加列" @click="addTableColumn(selected)"><Plus :size="12" /></button>
            </div>
            <div v-for="(column, index) in selected.config.columns" :key="index" style="padding: 8px; margin-bottom: 7px; border: 1px solid var(--line-soft); border-radius: 8px">
              <el-select v-model="column.field" size="small" filterable allow-create placeholder="字段" style="width: 100%">
                <el-option v-for="item in previewColumns" :key="item" :label="item" :value="item" />
              </el-select>
              <div class="inline-actions" style="margin-top: 6px">
                <el-input v-model="column.label" size="small" placeholder="列名称" />
                <el-select v-model="column.format" size="small" style="width: 96px">
                  <el-option label="文本" value="text" />
                  <el-option label="金额" value="currency" />
                  <el-option label="整数" value="integer" />
                  <el-option label="小数" value="number" />
                  <el-option label="百分比" value="percent" />
                </el-select>
                <button class="mini-icon" type="button" @click="removeAt(selected.config.columns, index)"><Trash2 :size="11" /></button>
              </div>
              <div class="inline-actions" style="margin-top: 6px">
                <el-checkbox v-model="column.progress" size="small">数值条形</el-checkbox>
                <el-select v-model="column.aggregate" size="small" clearable placeholder="合计方式" style="width: 112px">
                  <el-option label="求和" value="sum" />
                  <el-option label="平均" value="avg" />
                  <el-option label="不统计" value="none" />
                </el-select>
              </div>
            </div>
          </div>
          <div class="property-group" style="display: flex; align-items: center; justify-content: space-between">
            <span class="property-label" style="margin: 0">显示合计行</span>
            <el-switch v-model="selected.config.showSummary" />
          </div>
          <div class="property-group">
            <label class="property-label">条件格式（阈值高亮）</label>
            <div class="inline-actions" style="margin-bottom: 6px">
              <el-select v-model="selected.config.conditional.field" clearable size="small" placeholder="选择数值列" style="flex: 1">
                <el-option v-for="column in selected.config.columns" :key="column.field" :label="column.label || column.field" :value="column.field" />
              </el-select>
              <el-select v-model="selected.config.conditional.operator" size="small" style="width: 92px">
                <el-option label="低于" value="lt" />
                <el-option label="高于" value="gt" />
              </el-select>
              <el-input-number v-model="selected.config.conditional.threshold" size="small" :controls="false" style="width: 100px" />
            </div>
            <p style="color: var(--muted-light); font-size: 8.5px; line-height: 1.6">
              满足条件的单元格会高亮，可用于毛利预警、低达成率提醒等场景。
            </p>
          </div>
        </template>

        <template v-else-if="selected.chart_type === 'text'">
          <div class="property-group">
            <label class="property-label">文本内容</label>
            <el-input v-model="selected.config.content" type="textarea" :rows="6" placeholder="填写业务说明或分析结论" />
          </div>
        </template>

        <div class="property-group">
          <label class="property-label">组件宽度</label>
          <div class="segmented">
            <button v-for="width in [4, 6, 8, 12]" :key="width" type="button" :class="{ active: selected.w === width }" @click="selected.w = width">
              {{ width === 12 ? '整行' : `${width}/12` }}
            </button>
          </div>
        </div>
        <div class="property-group">
          <label class="property-label">组件高度</label>
          <div class="segmented">
            <button v-for="height in [2, 4, 6, 8]" :key="height" type="button" :class="{ active: selected.h === height }" @click="selected.h = height">
              {{ height <= 2 ? '紧凑' : height <= 4 ? '标准' : height <= 6 ? '较高' : '高' }}
            </button>
          </div>
        </div>
      </template>

      <EmptyBlock v-else title="选择一个组件" description="点击画布中的组件后，可在这里调整标题、数据源、字段与尺寸。">
        <button class="btn btn-secondary btn-sm" type="button" @click="addWidget('line')">
          <Plus :size="13" /> 添加折线图
        </button>
      </EmptyBlock>
    </aside>
  </div>
</template>
