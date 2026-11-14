<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ArrowLeft,
  CalendarRange,
  Copy,
  Download,
  FileDown,
  FileSpreadsheet,
  LayoutDashboard,
  Maximize2,
  Minimize2,
  Pencil,
  Printer,
  RefreshCw,
  Share2,
  ShieldCheck,
  GitBranch,
  Trash2,
} from 'lucide-vue-next'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, getErrorMessage } from '../../api/client'
import { useAuthStore } from '../../stores/auth'
import { dateRange, formatDate, monthRange } from '../../utils/format'
import { dataScopeLabel } from '../../utils/org'
import { useFullscreen } from '../../utils/fullscreen'
import DashboardFilterBar from '../../components/DashboardFilterBar.vue'
import EmptyBlock from '../../components/EmptyBlock.vue'
import OrgDrillExplorer from '../../components/OrgDrillExplorer.vue'
import ShareDialog from '../../components/ShareDialog.vue'
import WidgetRenderer from '../../components/WidgetRenderer.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const dashboard = ref(null)
const layout = ref({ settings: {}, widgets: [] })
const result = ref(null)
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const rangeDays = ref(30)
const rangeKey = ref('30d')
const dates = ref(dateRange(30))
const shareVisible = ref(false)
const updatedAt = ref('')
const activeTab = ref('dashboard')
const pageRef = ref(null)
const exporting = ref(false)
const filterValues = ref({})
let refreshTimer = null
let filterTimer = null

const { isFullscreen, toggleFullscreen } = useFullscreen(pageRef)

const dashboardId = computed(() => route.params.id)
const widgets = computed(() => layout.value?.widgets || [])
const settings = computed(() => layout.value?.settings || {})
const filterDefs = computed(() => (Array.isArray(settings.value.filters) ? settings.value.filters : []))
const canEdit = computed(
  () => auth.isAdmin || (dashboard.value && dashboard.value.created_by === auth.user?.id),
)
const canShare = computed(() => auth.can('share:create'))
const scopeLabel = computed(() => dataScopeLabel(auth.dataScope))
const drillEnabled = computed(() => dashboard.value?.drill_enabled === true)
const drillMode = computed(() => dashboard.value?.drill_mode || 'level')
const exportableWidgets = computed(() =>
  widgets.value.filter((widget) => widget.chart_type !== 'text' && widget.dataset_id),
)

const resultMap = computed(() => {
  const map = {}
  for (const item of result.value?.widgets || []) {
    map[item.widget_id] = item
  }
  return map
})

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

function widgetStyle(widget) {
  return {
    gridColumn: `span ${Math.min(12, Math.max(1, Number(widget.w) || 12))}`,
  }
}

function clearRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
}

function setupRefresh() {
  clearRefresh()
  const seconds = Number(settings.value.refresh_seconds || 0)
  if (seconds >= 15) {
    refreshTimer = setInterval(() => loadData(true), seconds * 1000)
  }
}

// 全局筛选器：仅提交已配置且有值的筛选条件，避免参数噪声。
function isMultiFilter(item) {
  return item?.type === 'multiselect'
}

function normalizeFilterValue(item, value) {
  if (isMultiFilter(item)) {
    if (Array.isArray(value)) return value
    return String(value ?? '')
      .split(/[,，;；]+/)
      .map((part) => part.trim())
      .filter(Boolean)
  }
  return value ?? ''
}

// 生成当前生效的筛选参数；多选值保持数组，由服务端编码并转义。
function activeFilters() {
  const filters = {}
  for (const item of filterDefs.value) {
    const value = normalizeFilterValue(item, filterValues.value[item.key])
    if (Array.isArray(value)) {
      if (value.length) filters[item.key] = value
      continue
    }
    const text = String(value).trim()
    if (text) filters[item.key] = text
  }
  return filters
}

function appendFilters(query) {
  const filters = activeFilters()
  if (Object.keys(filters).length) query.set('filters', JSON.stringify(filters))
}

function readRouteFilters() {
  const raw = route.query.filters
  if (!raw) return null
  try {
    const parsed = JSON.parse(String(raw))
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null
    return parsed
  } catch {
    return null
  }
}

function syncFilterDefaults() {
  const next = {}
  for (const item of filterDefs.value) {
    next[item.key] = normalizeFilterValue(item, item.default)
  }
  const fromURL = readRouteFilters()
  if (fromURL) {
    for (const item of filterDefs.value) {
      if (Object.prototype.hasOwnProperty.call(fromURL, item.key)) {
        next[item.key] = normalizeFilterValue(item, fromURL[item.key])
      }
    }
  }
  filterValues.value = next
}

function handleFilterChange(next) {
  filterValues.value = { ...next }
  syncFilterQuery()
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(() => {
    filterTimer = null
    loadData()
  }, 320)
}

// 把筛选条件写入地址栏，刷新或转发链接后仍能保持当前筛选。
function syncFilterQuery() {
  const active = activeFilters()
  const query = { ...route.query }
  if (Object.keys(active).length) {
    query.filters = JSON.stringify(active)
  } else {
    delete query.filters
  }
  const current = route.query.filters ? String(route.query.filters) : ''
  const next = query.filters || ''
  if (current === next) return
  router.replace({ query }).catch(() => {})
}

// 图表点击联动：把分类值写入对应的全局筛选器并刷新整个看板。
function handleLinkFilter(payload = {}) {
  const key = String(payload.filterKey || '')
  if (!key || !filterDefs.value.some((item) => item.key === key)) return
  const value = String(payload.value ?? '')
  if (!value) return
  if (filterTimer) {
    clearTimeout(filterTimer)
    filterTimer = null
  }
  const target = filterDefs.value.find((item) => item.key === key)
  filterValues.value = {
    ...filterValues.value,
    [key]: isMultiFilter(target) ? [value] : value,
  }
  syncFilterQuery()
  loadData()
  const label = target?.label || key
  ElMessage.success(`已联动筛选：${label} = ${value}`)
}

async function load() {
  loading.value = true
  error.value = ''
  result.value = null
  try {
    const detail = await api.get(`/dashboards/${dashboardId.value}`)
    dashboard.value = detail
    layout.value = parseLayout(detail.layout)
    syncFilterDefaults()
    await loadData()
    setupRefresh()
  } catch (err) {
    error.value = getErrorMessage(err)
  } finally {
    loading.value = false
  }
}

async function loadData(silent = false) {
  if (!dashboardId.value) return
  if (silent) refreshing.value = true
  try {
    const query = new URLSearchParams({
      start_date: dates.value[0],
      end_date: dates.value[1],
    })
    appendFilters(query)
    if (!silent) query.set('nocache', '1')
    result.value = await api.get(`/dashboards/${dashboardId.value}/data?${query.toString()}`)
    updatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  } catch (err) {
    if (!silent) ElMessage.error(getErrorMessage(err))
  } finally {
    refreshing.value = false
  }
}

function chooseRange(days) {
  rangeDays.value = days
  rangeKey.value = `${days}d`
  dates.value = dateRange(days)
  loadData()
}

function chooseMonth(offset) {
  rangeDays.value = 0
  rangeKey.value = offset === 0 ? 'month' : 'last-month'
  dates.value = monthRange(offset)
  loadData()
}

function chooseCustom(value) {
  if (!value || !value[0]) return
  rangeDays.value = 0
  rangeKey.value = 'custom'
  loadData()
}

function printPage() {
  window.print()
}

function safeFilename(value) {
  return String(value || '')
    .replace(/[\\/:*?"<>|]/g, '_')
    .slice(0, 60)
}

async function handleExport(command) {
  if (exporting.value || !dashboardId.value) return
  exporting.value = true
  try {
    const query = new URLSearchParams({
      start_date: dates.value[0],
      end_date: dates.value[1],
    })
    appendFilters(query)
    if (command === 'xlsx') {
      query.set('format', 'xlsx')
      const filename = `${safeFilename(dashboard.value?.name || '看板')}_${dates.value[0]}_${dates.value[1]}.xlsx`
      await api.download(`/dashboards/${dashboardId.value}/export?${query.toString()}`, filename)
      ElMessage.success('看板 Excel 已导出')
      return
    }
    if (command.startsWith('csv:')) {
      const widgetId = command.slice(4)
      const widget = widgets.value.find((item) => String(item.id) === widgetId)
      query.set('format', 'csv')
      query.set('widget_id', widgetId)
      const filename = `${safeFilename(dashboard.value?.name || '看板')}_${safeFilename(widget?.title || '明细')}.csv`
      await api.download(`/dashboards/${dashboardId.value}/export?${query.toString()}`, filename)
      ElMessage.success('组件 CSV 已导出')
    }
  } catch (err) {
    ElMessage.error(getErrorMessage(err))
  } finally {
    exporting.value = false
  }
}

async function duplicate() {
  try {
    const item = await api.post(`/dashboards/${dashboardId.value}/duplicate`)
    ElMessage.success('已复制为草稿')
    router.push(`/dashboards/${item.id}/edit`)
  } catch (err) {
    ElMessage.error(getErrorMessage(err))
  }
}

async function remove() {
  try {
    await ElMessageBox.confirm('删除后看板与布局将无法恢复，确认删除？', '删除看板', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
    await api.delete(`/dashboards/${dashboardId.value}`)
    ElMessage.success('看板已删除')
    router.push('/dashboards')
  } catch (err) {
    if (err !== 'cancel') ElMessage.error(getErrorMessage(err))
  }
}

watch(
  () => route.params.id,
  () => load(),
)

// 支持浏览器后退/前进时恢复地址栏中的筛选条件。
watch(
  () => route.query.filters,
  (raw) => {
    if (!filterDefs.value.length) return
    let parsed = null
    try {
      parsed = raw ? JSON.parse(String(raw)) : null
    } catch {
      parsed = null
    }
    const next = {}
    for (const item of filterDefs.value) {
      next[item.key] = parsed && Object.prototype.hasOwnProperty.call(parsed, item.key)
        ? normalizeFilterValue(item, parsed[item.key])
        : normalizeFilterValue(item, item.default)
    }
    if (JSON.stringify(next) === JSON.stringify(filterValues.value)) return
    filterValues.value = next
    loadData()
  },
)

onMounted(load)
onBeforeUnmount(() => {
  clearRefresh()
  if (filterTimer) clearTimeout(filterTimer)
})
</script>

<template>
  <div ref="pageRef" class="dashboard-page" :class="{ 'bi-fullscreen': isFullscreen }">
    <div v-if="loading" class="panel" style="padding: 22px">
      <div class="skeleton" style="height: 64px; margin-bottom: 18px" />
      <div class="dash-grid">
        <div v-for="index in 5" :key="index" class="skeleton" style="grid-column: span 12; height: 190px" />
      </div>
    </div>

    <div v-else-if="error" class="panel">
      <EmptyBlock title="看板暂时不可访问" :description="error">
        <button class="btn btn-secondary btn-sm" type="button" @click="router.push('/dashboards')">
          <ArrowLeft :size="14" /> 返回看板列表
        </button>
      </EmptyBlock>
    </div>

    <template v-else>
      <div class="dashboard-view-head">
        <div class="head-info">
          <div class="inline-actions" style="margin-bottom: 12px">
            <button class="icon-btn" type="button" title="返回" @click="router.push('/dashboards')">
              <ArrowLeft :size="16" />
            </button>
            <span class="tag" :class="dashboard?.status === 'published' ? 'tag-green' : 'tag-amber'">
              {{ dashboard?.status === 'published' ? '已发布' : '草稿' }}
            </span>
            <span class="tag">{{ dashboard?.category || '经营分析' }}</span>
          </div>
          <div class="dashboard-title-row">
            <h1>{{ dashboard?.name }}</h1>
            <span class="status-dot">LIVE</span>
          </div>
          <p class="dashboard-desc">
            {{ dashboard?.description || '所有指标均在当前账号授权范围内实时计算' }}
          </p>
        </div>

      <div class="view-toolbar">
          <div v-if="activeTab === 'dashboard'" class="date-range">
            <button :class="{ active: rangeKey === '7d' }" type="button" @click="chooseRange(7)">近 7 天</button>
            <button :class="{ active: rangeKey === '30d' }" type="button" @click="chooseRange(30)">近 30 天</button>
            <button :class="{ active: rangeKey === '90d' }" type="button" @click="chooseRange(90)">近 90 天</button>
            <button :class="{ active: rangeKey === 'month' }" type="button" @click="chooseMonth(0)">本月</button>
            <button :class="{ active: rangeKey === 'last-month' }" type="button" @click="chooseMonth(-1)">上月</button>
          </div>
          <el-date-picker
            v-if="activeTab === 'dashboard'"
            v-model="dates"
            type="daterange"
            value-format="YYYY-MM-DD"
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            size="small"
            style="width: 232px"
            @change="chooseCustom"
          />
          <button v-if="activeTab === 'dashboard'" class="icon-btn" type="button" title="刷新（跳过缓存）" @click="loadData()">
            <RefreshCw :size="15" :class="{ spin: refreshing }" />
          </button>
          <el-dropdown trigger="click" :disabled="exporting" @command="handleExport">
            <button class="btn btn-secondary btn-sm" type="button">
              <Download :size="14" /> 导出
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="xlsx">
                  <FileSpreadsheet :size="13" /> 整份看板 Excel
                </el-dropdown-item>
                <el-dropdown-item
                  v-for="widget in exportableWidgets"
                  :key="widget.id"
                  :command="`csv:${widget.id}`"
                >
                  <FileDown :size="13" /> {{ widget.title || '明细表' }}（CSV）
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <button class="icon-btn" type="button" title="打印" @click="printPage">
            <Printer :size="15" />
          </button>
          <button
            class="icon-btn"
            type="button"
            :title="isFullscreen ? '退出全屏' : '全屏查看'"
            @click="toggleFullscreen"
          >
            <Minimize2 v-if="isFullscreen" :size="15" />
            <Maximize2 v-else :size="15" />
          </button>
          <button v-if="canShare" class="btn btn-secondary btn-sm" type="button" @click="shareVisible = true">
            <Share2 :size="14" /> 分享
          </button>
          <button v-if="canEdit" class="btn btn-primary btn-sm" type="button" @click="router.push(`/dashboards/${dashboardId}/edit`)">
            <Pencil :size="14" /> 编辑
          </button>
          <el-dropdown v-if="canEdit" trigger="click">
            <button class="icon-btn" type="button" title="更多操作">···</button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="duplicate"><Copy :size="13" /> 复制看板</el-dropdown-item>
                <el-dropdown-item divided @click="remove"><Trash2 :size="13" /> 删除看板</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>

      <DashboardFilterBar
        v-if="activeTab === 'dashboard'"
        :filters="filterDefs"
        :model-value="filterValues"
        :loading="refreshing"
        @change="handleFilterChange"
      />

      <div v-if="activeTab === 'dashboard'" class="scope-note">
        <ShieldCheck :size="14" />
        <span>
          数据范围：<strong>{{ scopeLabel }}</strong> · {{ auth.orgName || '全部组织' }} ·
          统计周期 {{ dates[0] }} 至 {{ dates[1] }} · 最近刷新 {{ updatedAt || '—' }}
        </span>
      </div>

      <div v-if="drillEnabled" class="view-tabs">
        <button :class="{ active: activeTab === 'dashboard' }" type="button" @click="activeTab = 'dashboard'">
          <LayoutDashboard :size="14" /> 标准看板
        </button>
        <button :class="{ active: activeTab === 'drill' }" type="button" @click="activeTab = 'drill'">
          <GitBranch :size="14" /> 组织下钻
        </button>
      </div>

      <div v-show="activeTab === 'dashboard'">
        <div class="dash-grid">
          <div v-for="widget in widgets" :key="widget.id" class="widget-shell" :style="widgetStyle(widget)">
            <WidgetRenderer
              :widget="widget"
              :result="resultMap[widget.id]"
              @link-filter="handleLinkFilter"
            />
          </div>
        </div>

        <div v-if="!widgets.length" class="panel">
          <EmptyBlock title="看板还没有组件" description="进入编辑器添加指标卡、图表或明细表。">
            <button v-if="canEdit" class="btn btn-primary btn-sm" type="button" @click="router.push(`/dashboards/${dashboardId}/edit`)">
              <Pencil :size="14" /> 开始编辑
            </button>
          </EmptyBlock>
        </div>

        <div class="scope-note" style="margin-top: 18px; margin-bottom: 0">
          <CalendarRange :size="14" />
          <span>
            数据由受控 SQL 数据集实时查询，单次最多 2000 行，查询超时 10 秒。
            分享链接会继承创建者的组织数据边界，并在服务端校验有效期与访问次数。
          </span>
        </div>
      </div>

      <OrgDrillExplorer
        v-if="drillEnabled && activeTab === 'drill'"
        api-base="/drill"
        :initial-mode="drillMode"
      />

      <ShareDialog
        v-model="shareVisible"
        :dashboard-id="Number(dashboardId)"
        :dashboard-name="dashboard?.name || '经营看板'"
      />
    </template>
  </div>
</template>

<style scoped>
.dashboard-page {
  min-width: 0;
}
.dashboard-page.bi-fullscreen {
  min-height: 100vh;
  padding: 22px 26px 28px;
  overflow: auto;
  background: #f6f9f7;
}
.dashboard-page.bi-fullscreen .dashboard-desc,
.dashboard-page.bi-fullscreen .scope-note {
  display: none;
}
.dashboard-page.bi-fullscreen .dashboard-view-head {
  margin-bottom: 14px;
}

.view-tabs { display: inline-flex; gap: 4px; padding: 4px; margin-bottom: 16px; border-radius: 10px; background: #f2f6f3; border: 1px solid var(--line-soft); }
.view-tabs button { border: 0; background: transparent; color: var(--muted); font-size: 11px; display: inline-flex; align-items: center; gap: 6px; padding: 7px 14px; border-radius: 8px; cursor: pointer; transition: all .16s ease; }
.view-tabs button:hover { color: var(--emerald-deep); }
.view-tabs button.active { background: #fff; color: var(--emerald-deep); font-weight: 600; box-shadow: 0 1px 2px rgba(16, 42, 35, .08); }
@media (max-width: 640px) {
  .view-tabs { width: 100%; }
  .view-tabs button { flex: 1; justify-content: center; }
}
</style>
