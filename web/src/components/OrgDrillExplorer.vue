<!-- 组织下钻分析组件（看板核心交互）：
     - 逐级下钻：以当前登录人权限范围内的一级组织为起点，点击行下钻到下一级（大区 → 区域 → 门店 → 员工）
     - 全部门店：跨层级平铺当前范围内的所有门店
     - 全部员工：合并展示当前范围内所有门店的员工销售
     - 点击组织名进入该组织，点击指标名查看最近一段时间的趋势图 -->
<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  ArrowRight,
  Building2,
  ChevronRight,
  Download,
  Grid3X3,
  Layers,
  RefreshCw,
  ShieldCheck,
  Store,
  Target,
  TrendingUp,
  Users,
} from 'lucide-vue-next'
import echarts from '../utils/echarts'
import { ElMessage } from 'element-plus'
import { api, getErrorMessage } from '../api/client'
import { compactNumber, dateRange, formatValue, toNumber } from '../utils/format'
import NumText from './NumText.vue'
import { orgTypeLabel } from '../utils/org'
import EmptyBlock from '../components/EmptyBlock.vue'

const props = defineProps({
  // 下钻接口前缀：登录态 /drill，分享链接 /public/dashboards/:token/drill
  apiBase: { type: String, default: '/drill' },
  initialMode: { type: String, default: 'level' },
})

const loading = ref(true)
// 当前层级下钻结果：{ current, nodes: [...], next_level_type }
const result = ref(null)
// 可展示的指标列表，默认五项，可被后台目标指标配置覆盖
const metrics = ref([
  { key: 'revenue', label: '销售额', unit: '元' },
  { key: 'orders', label: '订单量', unit: '笔' },
  { key: 'gross_profit', label: '毛利额', unit: '元' },
  { key: 'customers', label: '客流', unit: '人' },
  { key: 'gross_margin', label: '毛利率', unit: '%' },
])
// 当前周期类型：月 / 季 / 年
const periodType = ref('month')
const period = ref(currentPeriod('month'))
const metric = ref('revenue')
// 展示模式：level 逐级下钻 / all 全部门店 / employees 全部员工
const mode = ref(['level', 'all', 'employees'].includes(props.initialMode) ? props.initialMode : 'level')
// 当前所在组织 ID，0 表示当前权限范围内的默认起点
const currentOrgID = ref(0)
const chartEl = ref(null)
let chart = null

// 趋势图弹窗
const trendVisible = ref(false)
const trendLoading = ref(false)
const trend = ref(null)
const trendNode = ref(null)
const trendMetric = ref('revenue')
const trendRange = ref(dateRange(7))
const trendDays = ref(7)
const trendChartEl = ref(null)
let trendChart = null

const modeOptions = [
  { value: 'level', label: '逐级下钻', icon: Layers, hint: '按组织层级展示下一级，点击行继续下钻' },
  { value: 'all', label: '全部门店', icon: Store, hint: '跨层级平铺当前范围内的所有门店' },
  { value: 'employees', label: '全部员工', icon: Users, hint: '合并展示当前范围内所有门店的员工销售' },
]

const monthOptions = computed(() => {
  const now = new Date()
  return Array.from({ length: 18 }, (_, index) => {
    const date = new Date(now.getFullYear(), now.getMonth() - index, 1)
    const value = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`
    return { value, label: value }
  })
})

const quarterOptions = computed(() => {
  const now = new Date()
  const current = Math.floor(now.getMonth() / 3)
  return Array.from({ length: 12 }, (_, index) => {
    const quarter = current - index
    const year = now.getFullYear() + Math.floor(quarter / 4)
    const normalized = ((quarter % 4) + 4) % 4
    const value = `${year}-Q${normalized + 1}`
    return { value, label: value }
  })
})

const yearOptions = computed(() => {
  const now = new Date().getFullYear()
  return Array.from({ length: 6 }, (_, index) => {
    const value = String(now - index)
    return { value, label: value }
  })
})

const periodOptions = computed(() => {
  if (periodType.value === 'quarter') return quarterOptions.value
  if (periodType.value === 'year') return yearOptions.value
  return monthOptions.value
})

const metricInfo = computed(() => metrics.value.find((item) => item.key === metric.value) || metrics.value[0])
// 指标格式化：毛利率按百分比，订单/客流按整数，其余按金额
const metricFormat = computed(() => {
  const key = metricInfo.value?.key
  if (key === 'gross_margin') return 'percent'
  if (key === 'orders' || key === 'customers') return 'integer'
  return 'currency'
})
const maxActual = computed(() => Math.max(1, ...(result.value?.nodes || []).map((node) => Math.abs(toNumber(node.actual_value)))))
// 当前组织类型中文名（如“大区”）
const currentTypeLabel = computed(() => orgTypeLabel(result.value?.current?.type))
// 下级节点类型中文名（节点类型不一致时显示“下级”）
const nodeTypeLabel = computed(() => {
  const nodes = result.value?.nodes || []
  const types = Array.from(new Set(nodes.map((node) => node.type).filter(Boolean)))
  if (types.length === 1) return orgTypeLabel(types[0])
  if (types.length > 1) return '下级'
  if (mode.value === 'employees') return '员工'
  return orgTypeLabel(result.value?.next_level_type)
})
// 当前视图内是否存在目标值（决定是否展示达成率列）
const hasTargetInView = computed(() => (result.value?.nodes || []).some((node) => node.has_target))

// 计算默认周期值，如 2026-11 / 2026-Q4 / 2026
function currentPeriod(type) {
  const now = new Date()
  if (type === 'quarter') return `${now.getFullYear()}-Q${Math.floor(now.getMonth() / 3) + 1}`
  if (type === 'year') return String(now.getFullYear())
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

function formatMetric(value, format = 'number') {
  const info = metricInfo.value
  if (info?.key === 'gross_margin') return `${toNumber(value).toFixed(1)}%`
  if (info?.key === 'orders' || info?.key === 'customers') return formatValue(value, 'integer', info?.unit || '')
  return formatValue(value, format === 'percent' ? 'number' : 'currency', info?.unit || '')
}

// 达成率文案，无目标时显示占位符
function rateText(node) {
  if (!node?.has_target || node.rate === null || node.rate === undefined) return '—'
  return `${toNumber(node.rate).toFixed(1)}%`
}

// 达成率样式：>=100% 领先、>=80% 正常、其余落后
function rateTone(node) {
  if (!node?.has_target) return 'muted'
  if (node.rate >= 100) return 'ahead'
  if (node.rate >= 80) return 'normal'
  return 'behind'
}

// 渲染当前层级对比条形图（实际值 vs 目标值）
function renderChart() {
  const nodes = (result.value?.nodes || []).slice(0, 8)
  if (!chartEl.value || !nodes.length) {
    if (chart) {
      chart.dispose()
      chart = null
    }
    return
  }
  if (chart && chart.getDom() !== chartEl.value) {
    chart.dispose()
    chart = null
  }
  if (!chart) {
    chart = echarts.init(chartEl.value)
  }
  const items = [...nodes].reverse()
  const hasTarget = items.some((node) => node.has_target && toNumber(node.target_value) > 0)
  const categoryLabel = nodeLabel
  const series = [
    {
      name: metricInfo.value?.label || '实际',
      type: 'bar',
      barMaxWidth: 12,
      itemStyle: { color: '#14B8A6', borderRadius: [0, 4, 4, 0] },
      data: items.map((node) => toNumber(node.actual_value)),
    },
  ]
  if (hasTarget) {
    series.push({
      name: '目标',
      type: 'bar',
      barMaxWidth: 12,
      itemStyle: { color: '#E8A13A', borderRadius: [0, 4, 4, 0] },
      data: items.map((node) => (node.has_target ? toNumber(node.target_value) : null)),
    })
  }
  chart.setOption(
    {
      grid: { left: 6, right: 20, top: hasTarget ? 28 : 10, bottom: 6, containLabel: true },
      legend: hasTarget
        ? { top: 0, right: 4, icon: 'circle', itemWidth: 7, itemHeight: 7, textStyle: { color: '#6b7d74', fontSize: 9 } }
        : undefined,
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        backgroundColor: 'rgba(16, 42, 35, 0.94)',
        borderWidth: 0,
        textStyle: { color: '#eafff5', fontSize: 11 },
        valueFormatter: (value) => formatMetric(value),
      },
      xAxis: {
        type: 'value',
        splitLine: { lineStyle: { color: '#edf1ed', type: 'dashed' } },
        axisLabel: { color: '#94a39b', fontSize: 9, formatter: (value) => compactNumber(value) },
      },
      yAxis: {
        type: 'category',
        data: items.map(categoryLabel),
        axisLine: { lineStyle: { color: '#dfe7e0' } },
        axisTick: { show: false },
        axisLabel: {
          color: '#74867d',
          fontSize: 9,
          width: items.some((node) => node.type === 'employee') ? 152 : 118,
          overflow: 'truncate',
        },
      },
      series,
    },
    true,
  )
  chart.resize()
}

function nodeLabel(node) {
  if (node?.type === 'employee' && node.parent_name) return `${node.name}·${node.parent_name}`
  return node?.name || ''
}

function csvCell(value) {
  const text = String(value ?? '')
  return /[",\r\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text
}

function exportCsv() {
  const rows = result.value?.nodes || []
  if (!rows.length) {
    ElMessage.warning('当前视图没有可导出的数据')
    return
  }
  const header = ['名称', '编码', '类型', '上级/所属门店', '负责人', metricInfo.value?.label || '实际', '目标', '达成率(%)', '订单量']
  const lines = rows.map((row) => [
    row.name,
    row.code || '',
    orgTypeLabel(row.type),
    row.parent_name || '',
    row.manager_name || '',
    toNumber(row.actual_value),
    row.has_target ? toNumber(row.target_value) : '',
    row.has_target && row.rate !== null && row.rate !== undefined ? toNumber(row.rate) : '',
    toNumber(row.orders),
  ])
  const csv = `\uFEFF${[header, ...lines].map((cols) => cols.map(csvCell).join(',')).join('\r\n')}`
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  const name = (result.value?.current?.name || '组织下钻').replace(/[\\/:*?"<>|]/g, '_')
  link.href = url
  link.download = `${name}_${period.value}_${metricInfo.value?.label || '经营数据'}.csv`
  link.click()
  URL.revokeObjectURL(url)
}

async function loadMetrics() {
  try {
    const list = await api.get('/targets/metrics')
    if (Array.isArray(list) && list.length) metrics.value = list
  } catch {
    // 使用内置指标
  }
}

const trendFormat = computed(() => {
  const key = trend.value?.metric || trendMetric.value
  if (key === 'gross_margin') return 'percent'
  if (key === 'orders' || key === 'customers') return 'integer'
  return 'currency'
})

// 打开趋势图弹窗，默认展示最近 7 天
function openTrend(row, metricKey) {
  if (!row) return
  trendNode.value = row
  trendMetric.value = metricKey || metric.value
  trendDays.value = 7
  trendRange.value = dateRange(7)
  trendVisible.value = true
  trend.value = null
  loadTrend()
}

// 按当前组织节点 + 指标 + 时间区间加载趋势数据
async function loadTrend() {
  const node = trendNode.value
  if (!node || !trendRange.value?.[0]) return
  trendLoading.value = true
  try {
    const params = new URLSearchParams({
      node_id: String(node.id),
      node_type: node.type || 'org',
      metric: trendMetric.value,
      start_date: trendRange.value[0],
      end_date: trendRange.value[1],
    })
    trend.value = await api.get(`${props.apiBase}/trend?${params.toString()}`)
    await nextTick()
    renderTrendChart()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    trendLoading.value = false
  }
}

// 快捷时间范围：最近 N 天
function chooseTrendDays(days) {
  trendDays.value = days
  trendRange.value = dateRange(days)
  loadTrend()
}

// 自定义时间范围（日期选择器）
function chooseTrendRange(value) {
  if (!value || !value[0]) return
  trendDays.value = 0
  loadTrend()
}

// 趋势图坐标轴 / tooltip 的数值格式化
function trendLabel(value) {
  const isPercent = trend.value?.metric === 'gross_margin'
  const key = trend.value?.metric
  if (isPercent) return `${toNumber(value).toFixed(1)}%`
  return formatValue(value, key === 'orders' || key === 'customers' ? 'integer' : 'currency', trend.value?.unit || '')
}

function renderTrendChart() {
  if (!trendChartEl.value) return
  const points = trend.value?.points || []
  if (!points.length) {
    trendChart?.dispose()
    trendChart = null
    return
  }
  if (trendChart && trendChart.getDom() !== trendChartEl.value) {
    trendChart.dispose()
    trendChart = null
  }
  if (!trendChart) trendChart = echarts.init(trendChartEl.value)
  const isPercent = trend.value?.metric === 'gross_margin'
  trendChart.setOption(
    {
      grid: { left: 8, right: 18, top: 26, bottom: 6, containLabel: true },
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(16, 42, 35, 0.94)',
        borderWidth: 0,
        textStyle: { color: '#eafff5', fontSize: 11 },
        valueFormatter: (value) => trendLabel(value),
      },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: points.map((item) => item.date.slice(5)),
        axisLine: { lineStyle: { color: '#dfe7e0' } },
        axisTick: { show: false },
        axisLabel: { color: '#94a39b', fontSize: 9, hideOverlap: true },
      },
      yAxis: {
        type: 'value',
        splitLine: { lineStyle: { color: '#edf1ed', type: 'dashed' } },
        axisLabel: {
          color: '#94a39b',
          fontSize: 9,
          formatter: (value) => (isPercent ? `${value}%` : compactNumber(value)),
        },
      },
      series: [
        {
          name: trend.value?.metric_label || '趋势',
          type: 'line',
          smooth: 0.3,
          symbol: 'circle',
          symbolSize: 5,
          showSymbol: points.length <= 31,
          lineStyle: { width: 2, color: '#0F766E' },
          itemStyle: { color: '#0F766E' },
          areaStyle: {
            opacity: 0.14,
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: '#14B8A6' },
              { offset: 1, color: 'rgba(255,255,255,0)' },
            ]),
          },
          data: points.map((item) => toNumber(item.value)),
        },
      ],
    },
    true,
  )
  trendChart.resize()
}

function onTrendOpened() {
  renderTrendChart()
}

// 加载当前层级的下钻数据
async function load() {
  loading.value = true
  try {
    const params = new URLSearchParams({
      period_type: periodType.value,
      period: period.value,
      metric: metric.value,
      mode: mode.value,
    })
    if (currentOrgID.value) params.set('org_id', String(currentOrgID.value))
    result.value = await api.get(`${props.apiBase}?${params.toString()}`)
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
  await nextTick()
  renderChart()
}

// 切换周期类型（月/季/年）并重置到当前周期
function changePeriodType(value) {
  period.value = currentPeriod(value)
}

// 切换展示模式（逐级下钻 / 全部门店 / 全部员工）
function switchMode(value) {
  mode.value = value
  load()
}

// 跳转到指定组织（面包屑点击）
function gotoOrg(id) {
  currentOrgID.value = Number(id) || 0
  load()
}

// 点击行下钻到该组织；员工节点不可再下钻
function drill(node) {
  if (!node?.drillable) return
  if (node.type === 'employee') return
  currentOrgID.value = node.id
  mode.value = 'level'
  load()
}

// 回到权限范围内的默认起点
function resetToScope() {
  currentOrgID.value = 0
  mode.value = 'level'
  load()
}

watch([metric, period], () => load())
watch(() => props.apiBase, () => { currentOrgID.value = 0; load() })

// 组件尺寸变化时同步重置两张图表的尺寸
function handleResize() {
  chart?.resize()
  trendChart?.resize()
}

onMounted(async () => {
  window.addEventListener('resize', handleResize)
  await loadMetrics()
  await load()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  if (chart) {
    chart.dispose()
    chart = null
  }
  if (trendChart) {
    trendChart.dispose()
    trendChart = null
  }
})
</script>

<template>
  <div class="drill-page">
    <div class="panel drill-toolbar">
      <div class="drill-toolbar-main">
        <div class="scope-badge">
          <ShieldCheck :size="15" />
          <div>
            <strong>当前数据范围</strong>
            <span>{{ result?.current?.name || '按角色自动定位' }} · {{ currentTypeLabel }}</span>
          </div>
        </div>
        <div class="inline-actions drill-filters">
          <el-select v-model="periodType" size="small" style="width: 108px" @change="changePeriodType">
            <el-option value="month" label="按月" />
            <el-option value="quarter" label="按季" />
            <el-option value="year" label="按年" />
          </el-select>
          <el-select v-model="period" size="small" filterable style="width: 126px">
            <el-option v-for="item in periodOptions" :key="item.value" :value="item.value" :label="item.label" />
          </el-select>
          <el-select v-model="metric" size="small" style="width: 130px">
            <el-option v-for="item in metrics" :key="item.key" :value="item.key" :label="item.label" />
          </el-select>
          <button class="btn btn-secondary btn-sm" type="button" :disabled="loading" @click="load">
            <RefreshCw :size="13" /> 刷新
          </button>
        </div>
      </div>

      <div class="drill-toolbar-bottom">
        <div class="breadcrumb-strip">
          <button class="crumb crumb-root" type="button" @click="resetToScope">
            <Building2 :size="12" /> 我的范围
          </button>
          <template v-for="item in result?.breadcrumbs || []" :key="item.id">
            <ChevronRight :size="12" class="crumb-sep" />
            <button
              class="crumb"
              :class="{ active: item.id === result?.current?.id }"
              type="button"
              @click="gotoOrg(item.id)"
            >
              {{ item.name }}
            </button>
          </template>
        </div>
        <div class="segmented drill-modes">
          <button
            v-for="item in modeOptions"
            :key="item.value"
            type="button"
            :class="{ active: mode === item.value }"
            :title="item.hint"
            @click="switchMode(item.value)"
          >
            <component :is="item.icon" :size="13" />
            {{ item.label }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="loading" class="metric-mini-grid">
      <div v-for="index in 4" :key="index" class="metric-mini"><div class="skeleton" style="height: 52px" /></div>
    </div>
    <div v-else class="metric-mini-grid drill-summary">
      <div class="metric-mini">
        <span>{{ metricInfo?.label || '实际' }}合计</span>
        <strong>
          <NumText :value="result?.summary?.actual_value" :format="metricFormat" start-compact />
          <!-- 单位紧跟在数值后面同行展示，避免“元”单独掉到下一行显得割裂 -->
          <small v-if="metricFormat !== 'percent'" class="metric-unit">{{ metricInfo?.unit || '' }}</small>
        </strong>
        <small>当前层级汇总口径</small>
      </div>
      <div class="metric-mini">
        <span>目标值</span>
        <strong>
          <NumText
            v-if="result?.summary?.target_value"
            :value="result.summary.target_value"
            :format="metricFormat"
            start-compact
          />
          <template v-else>—</template>
        </strong>
        <small>{{ result?.summary?.target_count ? `${result.summary.target_count} 个目标` : '未设置' }}</small>
      </div>
      <div class="metric-mini">
        <span>整体达成率</span>
        <strong :class="['rate-text', result?.summary?.rate >= 100 ? 'ahead' : result?.summary?.rate >= 80 ? 'normal' : 'behind']">
          {{ result?.summary?.target_value ? `${toNumber(result.summary.rate).toFixed(1)}%` : '—' }}
        </strong>
        <small>目标加权口径</small>
      </div>
      <div class="metric-mini">
        <span>{{ nodeTypeLabel }}数量</span>
        <strong>{{ result?.summary?.node_count || 0 }}</strong>
        <small>{{ mode === 'level' ? '可继续下钻' : '平铺视图' }}</small>
      </div>
    </div>

    <div v-if="!loading && result?.nodes?.length" class="panel drill-chart-panel">
      <div class="panel-head">
        <div>
          <div class="panel-title">{{ nodeTypeLabel }}{{ metricInfo?.label }}{{ hasTargetInView ? '与目标对比' : '分布' }}</div>
          <div class="panel-subtitle">
            {{ result?.current?.name }} · {{ result?.period }} · 当前层 Top {{ Math.min(8, result.nodes.length) }}
          </div>
        </div>
        <span class="tag">{{ hasTargetInView ? '含目标口径' : '实际值视图' }}</span>
      </div>
      <div ref="chartEl" class="drill-chart" />
    </div>

    <div class="drill-grid">
      <div class="panel drill-main-panel">
        <div class="panel-head">
          <div>
            <div class="panel-title">{{ result?.current?.name || '组织经营' }} · {{ nodeTypeLabel }}经营明细</div>
            <div class="panel-subtitle">
              {{ result?.start_date }} 至 {{ result?.end_date }} · 点击行可继续下钻
            </div>
          </div>
          <div class="inline-actions" style="gap: 8px">
            <span class="tag tag-green">{{ result?.nodes?.length || 0 }} 条</span>
            <button class="btn btn-secondary btn-sm" type="button" :disabled="!result?.nodes?.length" @click="exportCsv">
              <Download :size="13" /> 导出
            </button>
          </div>
        </div>

        <div v-if="result?.nodes?.length" class="drill-table-wrap">
          <el-table :data="result.nodes" style="width: 100%" @row-click="drill">
            <el-table-column :label="nodeTypeLabel" min-width="170">
              <template #default="{ row }">
                <div class="drill-name">
                  <span class="drill-icon">
                    <Store v-if="row.type === 'store'" :size="14" />
                    <Users v-else-if="row.type === 'employee'" :size="14" />
                    <Building2 v-else :size="14" />
                  </span>
                  <div>
                    <strong>{{ row.name }}</strong>
                    <span v-if="row.type === 'employee'">{{ row.code || '—' }}{{ row.manager_name ? ` · ${row.manager_name}` : '' }}</span>
                    <span v-else>{{ row.code || row.manager_name || '—' }}</span>
                  </div>
                </div>
              </template>
            </el-table-column>
            <el-table-column :label="mode === 'employees' ? '所属门店' : '上级'" min-width="115">
              <template #default="{ row }">{{ row.parent_name || '—' }}</template>
            </el-table-column>
            <el-table-column :label="metricInfo?.label || '实际'" min-width="130" align="right">
              <template #default="{ row }">
                <button class="metric-trend" type="button" title="点击查看最近趋势" @click.stop="openTrend(row, metric)">
                  <TrendingUp :size="11" />
                  <strong class="drill-amount">
                    <NumText :value="row.actual_value" :format="metricFormat" :interactive="false" />
                  </strong>
                </button>
              </template>
            </el-table-column>
            <el-table-column label="目标" min-width="125" align="right">
              <template #default="{ row }">
                <NumText v-if="row.has_target" :value="row.target_value" :format="metricFormat" />
                <template v-else>—</template>
              </template>
            </el-table-column>
            <el-table-column label="达成率" min-width="150">
              <template #default="{ row }">
                <div v-if="row.has_target" class="rate-cell">
                  <div class="rate-track">
                    <span
                      :class="['rate-fill', rateTone(row)]"
                      :style="{ width: `${Math.min(100, Math.max(2, toNumber(row.rate)))}%` }"
                    />
                  </div>
                  <b :class="['rate-text', rateTone(row)]">{{ rateText(row) }}</b>
                </div>
                <span v-else class="rate-text muted">—</span>
              </template>
            </el-table-column>
            <el-table-column label="订单" width="88" align="right">
              <template #default="{ row }">
                <button class="metric-trend" type="button" title="点击查看订单趋势" @click.stop="openTrend(row, 'orders')">
                  <NumText :value="row.orders" format="integer" :interactive="false" />
                </button>
              </template>
            </el-table-column>
            <el-table-column v-if="mode === 'level' && result?.current?.type !== 'store'" label="下级" width="76" align="center">
              <template #default="{ row }">
                <span class="tag" :class="row.drillable ? 'tag-green' : ''">
                  {{ row.type === 'store' ? '员工' : row.child_count || 0 }}
                </span>
              </template>
            </el-table-column>
            <el-table-column label="" width="48" align="center">
              <template #default="{ row }">
                <ArrowRight v-if="row.drillable" :size="14" class="drill-arrow" />
              </template>
            </el-table-column>
          </el-table>
        </div>
        <EmptyBlock
          v-else
          :title="`${nodeTypeLabel}暂无数据`"
          description="当前周期或权限范围内没有可展示的数据，可切换指标、周期或视图模式。"
        />
      </div>

      <aside class="drill-side">
        <div class="panel drill-rank-panel">
          <div class="panel-head">
            <div>
              <div class="panel-title">{{ nodeTypeLabel }}贡献</div>
              <div class="panel-subtitle">按{{ metricInfo?.label }}排序</div>
            </div>
          </div>
          <div v-if="result?.nodes?.length" class="rank-list">
            <button
              v-for="(node, index) in result.nodes.slice(0, 8)"
              :key="node.id"
              class="rank-row"
              type="button"
              @click="drill(node)"
            >
              <span class="rank-index">{{ String(index + 1).padStart(2, '0') }}</span>
              <div class="rank-body">
                <div class="rank-top">
                  <strong>{{ nodeLabel(node) }}</strong>
                  <span><NumText :value="node.actual_value" :format="metricFormat" start-compact /></span>
                </div>
                <div class="rank-track">
                  <i :style="{ width: `${Math.max(3, (Math.abs(toNumber(node.actual_value)) / maxActual) * 100)}%` }" />
                </div>
              </div>
            </button>
          </div>
          <div v-else class="empty-state" style="padding: 24px">暂无排行数据</div>
        </div>

        <div class="panel drill-tip">
          <h3><Target :size="14" /> 下钻说明</h3>
          <p>点击组织名称继续下钻；点击销售额、毛利、订单等指标，查看该节点最近 7 天趋势，时间范围可切换。</p>
          <div class="drill-mini-modes">
            <span><Grid3X3 :size="12" /> 全部门店：跨层级平铺</span>
            <span><Users :size="12" /> 全部员工：合并所有门店员工</span>
          </div>
        </div>
      </aside>
    </div>

    <el-dialog
      v-model="trendVisible"
      :title="`${trend?.name || trendNode?.name || ''} · ${trend?.metric_label || '指标'}趋势`"
      width="780px"
      destroy-on-close
      @opened="onTrendOpened"
    >
      <div class="trend-toolbar">
        <div class="date-range">
          <button :class="{ active: trendDays === 7 }" type="button" @click="chooseTrendDays(7)">近 7 天</button>
          <button :class="{ active: trendDays === 30 }" type="button" @click="chooseTrendDays(30)">近 30 天</button>
          <button :class="{ active: trendDays === 90 }" type="button" @click="chooseTrendDays(90)">近 90 天</button>
        </div>
        <el-date-picker
          v-model="trendRange"
          type="daterange"
          value-format="YYYY-MM-DD"
          range-separator="至"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          size="small"
          style="width: 240px"
          @change="chooseTrendRange"
        />
      </div>

      <div class="metric-mini-grid trend-summary">
        <div class="metric-mini">
          <span>区间合计</span>
          <strong><NumText :value="trend?.total" :format="trendFormat" start-compact /></strong>
          <small>{{ trend?.start_date || '—' }} 至 {{ trend?.end_date || '—' }}</small>
        </div>
        <div class="metric-mini">
          <span>日均</span>
          <strong><NumText :value="trend?.average" :format="trendFormat" start-compact /></strong>
          <small>{{ trend?.points?.length || 0 }} 天</small>
        </div>
        <div class="metric-mini">
          <span>峰值</span>
          <strong><NumText :value="trend?.peak" :format="trendFormat" start-compact /></strong>
          <small>{{ trend?.peak_date || '—' }}</small>
        </div>
      </div>

      <div v-loading="trendLoading" class="trend-chart" ref="trendChartEl" />
      <div v-if="!trendLoading && !trend?.points?.length" class="empty-state" style="padding: 20px">
        所选区间内暂无趋势数据
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.drill-toolbar { padding: 16px 18px; margin-bottom: 14px; }
.drill-toolbar-main { display: flex; align-items: center; justify-content: space-between; gap: 14px; flex-wrap: wrap; }
.drill-toolbar-bottom { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--line-soft); }
.scope-badge { display: flex; align-items: center; gap: 10px; }
.scope-badge > svg { color: var(--emerald); }
.scope-badge strong { display: block; font-size: 11px; font-weight: 600; }
.scope-badge span { display: block; margin-top: 2px; color: var(--muted); font-size: 11px; }
.drill-filters { flex-wrap: wrap; }
.breadcrumb-strip { display: flex; align-items: center; gap: 4px; flex-wrap: wrap; min-width: 0; }
.crumb { border: 0; background: transparent; color: var(--muted); font-size: 11.5px; padding: 4px 7px; border-radius: 6px; cursor: pointer; display: inline-flex; align-items: center; gap: 4px; }
.crumb:hover { background: var(--emerald-soft); color: var(--emerald-deep); }
.crumb.active { background: var(--emerald-soft); color: var(--emerald-deep); font-weight: 600; }
.crumb-sep { color: var(--muted-light); flex: none; }
.drill-modes { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.drill-modes button { display: inline-flex; align-items: center; justify-content: center; gap: 4px; white-space: nowrap; }
/* 只让卡片底部的说明文字独占一行，数值内的单位仍与数字同行 */
.drill-summary .metric-mini > small { color: var(--muted-light); font-size: 11px; display: block; margin-top: 6px; }
.rate-text.ahead { color: #0f766e; }
.rate-text.normal { color: #b7791f; }
.rate-text.behind { color: #c53030; }
.rate-text.muted { color: var(--muted-light); }
.drill-chart-panel { margin-top: 14px; overflow: hidden; }
.drill-chart { height: 244px; padding: 4px 10px 10px; }
.drill-grid { display: grid; grid-template-columns: minmax(0, 1fr) 300px; gap: 14px; margin-top: 14px; align-items: start; }
.drill-main-panel { overflow: hidden; }
.drill-table-wrap { overflow-x: auto; }
.drill-name { display: flex; align-items: center; gap: 9px; min-width: 0; }
.drill-icon { width: 28px; height: 28px; border-radius: 8px; background: #eef7f2; color: var(--emerald); display: inline-flex; align-items: center; justify-content: center; flex: none; }
.drill-name strong { display: block; font-size: 12px; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.drill-name span { display: block; margin-top: 2px; color: var(--muted-light); font-size: 10.5px; }
.drill-amount { font-size: 12px; color: var(--ink); }
.rate-cell { display: flex; align-items: center; gap: 8px; }
.rate-track { flex: 1; min-width: 52px; height: 5px; border-radius: 999px; background: #e9efe9; overflow: hidden; }
.rate-fill { display: block; height: 100%; border-radius: inherit; background: #0f766e; }
.rate-fill.normal { background: #d69e2e; }
.rate-fill.behind { background: #c53030; }
.rate-cell b { font-size: 11px; white-space: nowrap; }
.drill-arrow { color: var(--emerald); }
.drill-side { display: flex; flex-direction: column; gap: 14px; }
.rank-list { padding: 8px 14px 14px; display: flex; flex-direction: column; gap: 11px; }
.rank-row { border: 0; background: transparent; display: flex; align-items: center; gap: 9px; text-align: left; cursor: pointer; padding: 0; width: 100%; }
.rank-row .rank-index { flex: none; }
.rank-body { flex: 1; min-width: 0; }
.rank-top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.rank-top strong { font-size: 11.5px; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.rank-top span { font-size: 11px; color: var(--muted); flex: none; }
.rank-track { height: 4px; border-radius: 999px; background: #edf1ed; margin-top: 5px; overflow: hidden; }
.rank-track i { display: block; height: 100%; border-radius: inherit; background: linear-gradient(90deg, #14b8a6, #0f766e); }
.drill-tip { padding: 16px 18px; }
.drill-tip h3 { display: flex; align-items: center; gap: 6px; font-size: 11px; margin: 0 0 8px; }
.drill-tip p { color: var(--muted); font-size: 11px; line-height: 1.8; margin: 0; }
.drill-mini-modes { display: flex; flex-direction: column; gap: 6px; margin-top: 10px; color: var(--muted); font-size: 11px; }
.drill-mini-modes span { display: flex; align-items: center; gap: 5px; }
.metric-trend { margin: 0; padding: 0; border: 0; background: transparent; display: inline-flex; align-items: center; gap: 4px; color: var(--ink); font: inherit; font-size: 12px; cursor: pointer; border-radius: 5px; transition: color .15s ease, background .15s ease; }
.metric-trend svg { color: var(--emerald); opacity: .55; flex: none; transition: opacity .15s ease; }
.metric-trend:hover { color: var(--emerald-deep); background: var(--emerald-soft, rgba(20, 184, 166, .1)); }
.metric-trend:hover svg { opacity: 1; }
.metric-trend:focus-visible { outline: 2px solid var(--emerald); outline-offset: 2px; }
.trend-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; margin-bottom: 14px; }
.trend-summary { grid-template-columns: repeat(3, minmax(0, 1fr)); margin-bottom: 12px; }
.trend-summary .metric-mini > small { display: block; margin-top: 6px; color: var(--muted-light); font-size: 11px; }
.trend-chart { height: 300px; width: 100%; }
@media (max-width: 1060px) {
  .drill-grid { grid-template-columns: 1fr; }
  .drill-side { display: grid; grid-template-columns: 1fr 1fr; }
}
@media (max-width: 720px) {
  .drill-toolbar-main, .drill-toolbar-bottom { flex-direction: column; align-items: stretch; }
  .drill-modes { width: 100%; }
  .drill-side { grid-template-columns: 1fr; }
  .breadcrumb-strip { flex-wrap: nowrap; overflow-x: auto; padding-bottom: 2px; }
  .drill-chart { height: 210px; }
  .trend-toolbar { flex-direction: column; align-items: stretch; }
  .trend-summary { grid-template-columns: 1fr; }
  .trend-chart { height: 220px; }
}
</style>
