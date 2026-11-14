<!-- 公开授权看板页：通过分享 token 访问，无需登录；继承分享者数据权限，支持筛选、时间区间与图表联动。 -->
<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  CalendarRange,
  GitBranch,
  LayoutDashboard,
  Maximize2,
  Minimize2,
  RefreshCw,
  ShieldCheck,
} from 'lucide-vue-next'
import { api, getErrorMessage } from '../api/client'
import { dateRange, formatDate, monthRange } from '../utils/format'
import { useFullscreen } from '../utils/fullscreen'
import AppBrand from '../components/AppBrand.vue'
import DashboardFilterBar from '../components/DashboardFilterBar.vue'
import EmptyBlock from '../components/EmptyBlock.vue'
import OrgDrillExplorer from '../components/OrgDrillExplorer.vue'
import WidgetRenderer from '../components/WidgetRenderer.vue'
import WidgetTrendDialog from '../components/WidgetTrendDialog.vue'

const route = useRoute()
const router = useRouter()

const dashboard = ref(null)
const layout = ref({ settings: {}, widgets: [] })
const result = ref(null)
const share = ref(null)
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const errorStatus = ref(0)
const rangeDays = ref(30)
const rangeKey = ref('30d')
const dates = ref(dateRange(30))
const activeTab = ref('dashboard')
const pageRef = ref(null)
const filterValues = ref({})
// 指标趋势弹窗：分享页通过令牌接口读取，权限继承分享链接绑定的组织范围
const trendVisible = ref(false)
const trendPayload = ref(null)
let filterTimer = null

const { isFullscreen, toggleFullscreen } = useFullscreen(pageRef)

const token = computed(() => route.params.token)
const widgets = computed(() => layout.value?.widgets || [])
const settings = computed(() => layout.value?.settings || {})
const filterDefs = computed(() => (Array.isArray(settings.value.filters) ? settings.value.filters : []))
// 分享页可以按看板配置隐藏部分筛选器（服务端同样会忽略这些筛选值）。
const visibleFilterDefs = computed(() => filterDefs.value.filter((item) => item.hide_on_share !== true))
const drillEnabled = computed(() => dashboard.value?.drill_enabled === true)
const drillMode = computed(() => dashboard.value?.drill_mode || 'level')
const drillBase = computed(() => `/public/dashboards/${token.value}/drill`)

const resultMap = computed(() => {
  const map = {}
  for (const item of result.value?.widgets || []) {
    map[item.widget_id] = item
  }
  return map
})

// 解析看板布局配置，兼容字符串与对象两种存储格式
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

// 计算组件栅格布局样式
function widgetStyle(widget) {
  return {
    gridColumn: `span ${Math.min(12, Math.max(1, Number(widget.w) || 12))}`,
  }
}

// 判断筛选器是否为多选类型
function isMultiFilter(item) {
  return item?.type === 'multiselect'
}

// 规范化筛选值，多选统一转成数组
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

// 取当前生效的筛选条件
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

// 把筛选条件拼接为查询参数
function appendFilters(query) {
  const filters = activeFilters()
  if (Object.keys(filters).length) query.set('filters', JSON.stringify(filters))
}

// 从地址栏读取筛选条件，分享链接后可保持视角
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

// 初始化筛选器默认值
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

// 筛选变化后重新加载看板数据
function handleFilterChange(next) {
  filterValues.value = { ...next }
  syncFilterQuery()
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(() => {
    filterTimer = null
    refresh()
  }, 320)
}

// 把筛选条件同步到地址栏
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

// 图表点击联动：把点击的维度值写入对应筛选器
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
  refresh()
}

// 点击指标数字 / 趋势按钮时打开趋势弹窗
function handleWidgetTrend(payload = {}) {
  if (!payload?.field) return
  trendPayload.value = payload
  trendVisible.value = true
}

// 加载公开看板的配置与数据
async function load() {
  if (!token.value) return
  loading.value = true
  error.value = ''
  errorStatus.value = 0
  try {
    const query = new URLSearchParams({
      start_date: dates.value[0],
      end_date: dates.value[1],
    })
    // 首次请求还没有看板配置，直接透传地址栏中的筛选条件；服务端会按看板配置校验。
    const fromURL = readRouteFilters()
    if (fromURL && Object.keys(fromURL).length) {
      query.set('filters', JSON.stringify(fromURL))
    }
    const payload = await api.get(`/public/dashboards/${token.value}?${query.toString()}`)
    dashboard.value = payload?.dashboard || null
    layout.value = parseLayout(payload?.dashboard?.layout)
    syncFilterDefaults()
    result.value = payload?.data || null
    share.value = payload?.share || null
  } catch (err) {
    error.value = getErrorMessage(err)
    errorStatus.value = err?.status || 0
    dashboard.value = null
    result.value = null
  } finally {
    loading.value = false
  }
}

// 手动刷新看板数据
async function refresh() {
  if (!token.value) return
  refreshing.value = true
  try {
    const query = new URLSearchParams({
      start_date: dates.value[0],
      end_date: dates.value[1],
    })
    appendFilters(query)
    const payload = await api.get(`/public/dashboards/${token.value}?${query.toString()}`)
    dashboard.value = payload?.dashboard || dashboard.value
    layout.value = parseLayout(payload?.dashboard?.layout)
    result.value = payload?.data || null
    share.value = payload?.share || share.value
  } catch (err) {
    error.value = getErrorMessage(err)
    errorStatus.value = err?.status || 0
  } finally {
    refreshing.value = false
  }
}

// 快捷时间范围：最近 N 天
function chooseRange(days) {
  rangeDays.value = days
  rangeKey.value = `${days}d`
  dates.value = dateRange(days)
  refresh()
}

// 快捷时间范围：本月 / 上月
function chooseMonth(offset) {
  rangeDays.value = 0
  rangeKey.value = offset === 0 ? 'month' : 'last-month'
  dates.value = monthRange(offset)
  refresh()
}

// 自定义时间范围
function chooseCustom(value) {
  if (!value || !value[0]) return
  rangeDays.value = 0
  rangeKey.value = 'custom'
  refresh()
}

const errorTitle = computed(() => {
  if (errorStatus.value === 403) return '授权链接不可用'
  if (errorStatus.value === 404) return '看板不存在'
  return '看板暂时无法加载'
})

watch(
  () => route.params.token,
  () => load(),
)

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
    refresh()
  },
)

onMounted(load)
onBeforeUnmount(() => {
  if (filterTimer) clearTimeout(filterTimer)
})
</script>

<template>
  <div ref="pageRef" class="public-page" :class="{ 'bi-fullscreen': isFullscreen }">
    <header class="public-topbar">
      <div class="public-brand">
        <AppBrand />
        <div>
          <strong>青衡 BI</strong>
          <span style="display: block; margin-top: 3px; color: var(--muted-light); font-size: 11.5px; letter-spacing: 0.08em">
            AUTHORIZED VIEW
          </span>
        </div>
      </div>
      <div class="public-topbar-actions">
        <div class="public-secure">
          <ShieldCheck :size="14" />
          <span>服务端授权 · 只读访问</span>
        </div>
        <button
          class="icon-btn"
          type="button"
          :title="isFullscreen ? '退出全屏' : '全屏查看'"
          @click="toggleFullscreen"
        >
          <Minimize2 v-if="isFullscreen" :size="14" />
          <Maximize2 v-else :size="14" />
        </button>
      </div>
    </header>

    <main class="public-content">
      <div v-if="loading" class="panel" style="padding: 24px">
        <div class="skeleton" style="height: 58px; margin-bottom: 20px" />
        <div class="dash-grid">
          <div v-for="index in 4" :key="index" class="skeleton" style="grid-column: span 12; height: 170px" />
        </div>
      </div>

      <div v-else-if="error && !dashboard" class="panel">
        <EmptyBlock :title="errorTitle" :description="error">
          <p style="color: var(--muted-light); font-size: 11.5px; line-height: 1.7; text-align: center">
            请联系看板创建者重新生成授权链接。链接可能已过期、被撤销或访问次数已用尽。
          </p>
          <button class="btn btn-secondary btn-sm" type="button" @click="load">
            <RefreshCw :size="14" /> 重新验证
          </button>
        </EmptyBlock>
      </div>

      <template v-else>
        <div class="public-head">
          <div>
            <div class="eyebrow" style="color: var(--brand)">授权经营看板 · 只读</div>
            <h1>{{ dashboard?.name || '经营看板' }}</h1>
            <p>{{ dashboard?.description || '数据按分享时绑定的组织权限范围实时计算' }}</p>
          </div>
          <div class="public-meta">
            <div>授权到期：{{ formatDate(share?.expires_at, true) }}</div>
            <div>
              访问次数：{{ share?.views || 0 }}
              <template v-if="share?.max_views"> / {{ share.max_views }}</template>
              <template v-else>（不限）</template>
            </div>
            <div>统计周期：{{ dates[0] }} 至 {{ dates[1] }}</div>
          </div>
        </div>

        <div class="scope-note" style="margin-bottom: 12px">
          <ShieldCheck :size="14" />
          <span>
            本页通过服务端分享令牌校验，只展示创建者授权范围内的组织数据。
            链接不可编辑看板，也无法访问其他组织或原始 SQL。
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

        <div v-show="activeTab === 'dashboard'" class="view-toolbar" style="margin-bottom: 16px">
          <div class="date-range">
            <button :class="{ active: rangeKey === '7d' }" type="button" @click="chooseRange(7)">近 7 天</button>
            <button :class="{ active: rangeKey === '30d' }" type="button" @click="chooseRange(30)">近 30 天</button>
            <button :class="{ active: rangeKey === '90d' }" type="button" @click="chooseRange(90)">近 90 天</button>
            <button :class="{ active: rangeKey === 'month' }" type="button" @click="chooseMonth(0)">本月</button>
            <button :class="{ active: rangeKey === 'last-month' }" type="button" @click="chooseMonth(-1)">上月</button>
          </div>
          <el-date-picker
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
          <button class="icon-btn" type="button" title="刷新" @click="refresh">
            <RefreshCw :size="15" :class="{ spin: refreshing }" />
          </button>
        </div>

        <div v-if="error" class="scope-note" style="margin-bottom: 12px; color: var(--rose)">
          <ShieldCheck :size="14" />
          <span>{{ error }}</span>
        </div>

        <DashboardFilterBar
          v-if="activeTab === 'dashboard'"
          :filters="visibleFilterDefs"
          :model-value="filterValues"
          :loading="refreshing"
          @change="handleFilterChange"
        />

        <div v-show="activeTab === 'dashboard'">
          <div class="dash-grid">
            <div v-for="widget in widgets" :key="widget.id" class="widget-shell" :style="widgetStyle(widget)">
              <WidgetRenderer
                :widget="widget"
                :result="resultMap[widget.id]"
                @link-filter="handleLinkFilter"
                @metric-trend="handleWidgetTrend"
                @retry="refresh()"
              />
            </div>
          </div>

          <div v-if="!widgets.length" class="panel">
            <EmptyBlock title="看板暂无组件" description="创建者还没有为这个看板添加图表或指标卡。" />
          </div>
        </div>

        <OrgDrillExplorer
          v-if="drillEnabled && activeTab === 'drill'"
          :api-base="drillBase"
          :initial-mode="drillMode"
        />

        <div class="public-meta" style="margin-top: 24px; text-align: center">
          <CalendarRange :size="12" style="vertical-align: -2px" />
          数据由受控 SQL 实时查询，单次最多 2000 行 · 青衡 BI
        </div>

        <!-- 指标趋势弹窗：分享令牌接口，只读且受分享范围限制 -->
        <WidgetTrendDialog
          v-model="trendVisible"
          :share-token="String(token || '')"
          :widget="trendPayload?.widget"
          :field="trendPayload?.field"
          :label="trendPayload?.label"
          :unit="trendPayload?.unit"
          :format="trendPayload?.format"
          :filters="activeFilters()"
        />
      </template>
    </main>
  </div>
</template>

<style scoped>
.public-topbar-actions { display: flex; align-items: center; gap: 8px; }
.public-page.bi-fullscreen { min-height: 100vh; background: #f6f9f7; }
.public-page.bi-fullscreen .public-content { max-width: none; }

.view-tabs { display: inline-flex; gap: 4px; padding: 4px; margin-bottom: 12px; border-radius: 10px; background: #f2f6f3; border: 1px solid var(--line-soft); }
.view-tabs button { border: 0; background: transparent; color: var(--muted); font-size: 12.5px; display: inline-flex; align-items: center; gap: 6px; padding: 8px 15px; border-radius: 8px; cursor: pointer; transition: all .16s ease; }
.view-tabs button:hover { color: var(--emerald-deep); }
.view-tabs button.active { background: #fff; color: var(--emerald-deep); font-weight: 600; box-shadow: 0 1px 2px rgba(16, 42, 35, .08); }
@media (max-width: 640px) {
  .view-tabs { width: 100%; }
  .view-tabs button { flex: 1; justify-content: center; }
}
</style>
