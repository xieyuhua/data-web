<!-- 组织下钻页面：复用 OrgDrillExplorer 组件，按当前登录人权限展示可逐级下钻的组织经营数据。 -->
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
  Users,
} from 'lucide-vue-next'
import echarts from '../utils/echarts'
import { ElMessage } from 'element-plus'
import { api, getErrorMessage } from '../api/client'
import { compactNumber, formatValue, toNumber } from '../utils/format'
import NumText from '../components/NumText.vue'
import { orgTypeLabel } from '../utils/org'
import EmptyBlock from '../components/EmptyBlock.vue'

const loading = ref(true)
const result = ref(null)
const metrics = ref([
  { key: 'revenue', label: '销售额', unit: '元' },
  { key: 'orders', label: '订单量', unit: '笔' },
  { key: 'gross_profit', label: '毛利额', unit: '元' },
  { key: 'customers', label: '客流', unit: '人' },
  { key: 'gross_margin', label: '毛利率', unit: '%' },
])
const periodType = ref('month')
const period = ref(currentPeriod('month'))
const metric = ref('revenue')
const mode = ref('level')
const currentOrgID = ref(0)
const chartEl = ref(null)
let chart = null

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
const metricFormat = computed(() => {
  const key = metricInfo.value?.key
  if (key === 'gross_margin') return 'percent'
  if (key === 'orders' || key === 'customers') return 'integer'
  return 'currency'
})
const maxActual = computed(() => Math.max(1, ...(result.value?.nodes || []).map((node) => Math.abs(toNumber(node.actual_value)))))
const currentTypeLabel = computed(() => orgTypeLabel(result.value?.current?.type))
const nodeTypeLabel = computed(() => {
  const nodes = result.value?.nodes || []
  const types = Array.from(new Set(nodes.map((node) => node.type).filter(Boolean)))
  if (types.length === 1) return orgTypeLabel(types[0])
  if (types.length > 1) return '下级'
  if (mode.value === 'employees') return '员工'
  return orgTypeLabel(result.value?.next_level_type)
})
const hasTargetInView = computed(() => (result.value?.nodes || []).some((node) => node.has_target))

// 计算默认周期值，如 2026-11 / 2026-Q4 / 2026
function currentPeriod(type) {
  const now = new Date()
  if (type === 'quarter') return `${now.getFullYear()}-Q${Math.floor(now.getMonth() / 3) + 1}`
  if (type === 'year') return String(now.getFullYear())
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

// 按指标类型格式化数值（金额 / 整数 / 百分比）
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

// 渲染当前层级的实际值 / 目标值对比条形图
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

// 节点展示名称：组织名或员工名
function nodeLabel(node) {
  if (node?.type === 'employee' && node.parent_name) return `${node.name}·${node.parent_name}`
  return node?.name || ''
}

// CSV 单元格转义，防止 Excel 公式注入
function csvCell(value) {
  const text = String(value ?? '')
  return /[",\r\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text
}

// 导出当前层级数据为 CSV
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

// 加载后台配置的可用指标列表
async function loadMetrics() {
  try {
    const list = await api.get('/targets/metrics')
    if (Array.isArray(list) && list.length) metrics.value = list
  } catch {
    // 使用内置指标
  }
}

// 加载当前组织层级的经营数据
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
    result.value = await api.get(`/drill?${params.toString()}`)
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
  await nextTick()
  renderChart()
}

// 切换周期类型（月 / 季 / 年）并重置到当前周期
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

// 下钻到下一级组织
function drill(node) {
  if (!node?.drillable) return
  if (node.type === 'employee') return
  currentOrgID.value = node.id
  mode.value = 'level'
  load()
}

// 回到当前权限范围内的默认起点
function resetToScope() {
  currentOrgID.value = 0
  mode.value = 'level'
  load()
}

watch([metric, period], () => load())

// 窗口尺寸变化时重置图表尺寸，保证自适应
function handleResize() {
  chart?.resize()
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
        <strong><NumText :value="result?.summary?.actual_value" :format="metricFormat" start-compact /></strong>
        <small>{{ metricFormat === 'percent' ? '' : metricInfo?.unit || '' }}</small>
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
                <strong class="drill-amount">
                  <NumText :value="row.actual_value" :format="metricFormat" />
                </strong>
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
              <template #default="{ row }"><NumText :value="row.orders" format="integer" /></template>
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
          <p>默认展示当前组织下一级汇总。点击“区域 / 门店 / 员工”继续下钻，数据权限在每一层自动收窄。</p>
          <div class="drill-mini-modes">
            <span><Grid3X3 :size="12" /> 全部门店：跨层级平铺</span>
            <span><Users :size="12" /> 全部员工：合并所有门店员工</span>
          </div>
        </div>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.drill-toolbar { padding: 16px 18px; margin-bottom: 14px; }
.drill-toolbar-main { display: flex; align-items: center; justify-content: space-between; gap: 14px; flex-wrap: wrap; }
.drill-toolbar-bottom { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--line-soft); }
.scope-badge { display: flex; align-items: center; gap: 10px; }
.scope-badge > svg { color: var(--emerald); }
.scope-badge strong { display: block; font-size: 11px; font-weight: 600; }
.scope-badge span { display: block; margin-top: 2px; color: var(--muted); font-size: 9px; }
.drill-filters { flex-wrap: wrap; }
.breadcrumb-strip { display: flex; align-items: center; gap: 4px; flex-wrap: wrap; min-width: 0; }
.crumb { border: 0; background: transparent; color: var(--muted); font-size: 10px; padding: 4px 7px; border-radius: 6px; cursor: pointer; display: inline-flex; align-items: center; gap: 4px; }
.crumb:hover { background: var(--emerald-soft); color: var(--emerald-deep); }
.crumb.active { background: var(--emerald-soft); color: var(--emerald-deep); font-weight: 600; }
.crumb-sep { color: var(--muted-light); flex: none; }
.drill-modes { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.drill-modes button { display: inline-flex; align-items: center; justify-content: center; gap: 4px; white-space: nowrap; }
.drill-summary .metric-mini small { color: var(--muted-light); font-size: 8px; display: block; margin-top: 3px; }
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
.drill-name strong { display: block; font-size: 10.5px; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.drill-name span { display: block; margin-top: 2px; color: var(--muted-light); font-size: 8px; }
.drill-amount { font-size: 10.5px; color: var(--ink); }
.rate-cell { display: flex; align-items: center; gap: 8px; }
.rate-track { flex: 1; min-width: 52px; height: 5px; border-radius: 999px; background: #e9efe9; overflow: hidden; }
.rate-fill { display: block; height: 100%; border-radius: inherit; background: #0f766e; }
.rate-fill.normal { background: #d69e2e; }
.rate-fill.behind { background: #c53030; }
.rate-cell b { font-size: 9px; white-space: nowrap; }
.drill-arrow { color: var(--emerald); }
.drill-side { display: flex; flex-direction: column; gap: 14px; }
.rank-list { padding: 8px 14px 14px; display: flex; flex-direction: column; gap: 11px; }
.rank-row { border: 0; background: transparent; display: flex; align-items: center; gap: 9px; text-align: left; cursor: pointer; padding: 0; width: 100%; }
.rank-row .rank-index { flex: none; }
.rank-body { flex: 1; min-width: 0; }
.rank-top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.rank-top strong { font-size: 9.5px; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.rank-top span { font-size: 8.5px; color: var(--muted); flex: none; }
.rank-track { height: 4px; border-radius: 999px; background: #edf1ed; margin-top: 5px; overflow: hidden; }
.rank-track i { display: block; height: 100%; border-radius: inherit; background: linear-gradient(90deg, #14b8a6, #0f766e); }
.drill-tip { padding: 16px 18px; }
.drill-tip h3 { display: flex; align-items: center; gap: 6px; font-size: 11px; margin: 0 0 8px; }
.drill-tip p { color: var(--muted); font-size: 9px; line-height: 1.8; margin: 0; }
.drill-mini-modes { display: flex; flex-direction: column; gap: 6px; margin-top: 10px; color: var(--muted); font-size: 8.5px; }
.drill-mini-modes span { display: flex; align-items: center; gap: 5px; }
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
}
</style>
