<!-- 看板组件统一渲染入口：
     根据组件类型分发到 KPI 卡 / 表格 / 图表 / 文本说明，
     并负责「表格 ⇄ 图表」切换、点击图表元素查看明细、联动全局筛选与图片下载 -->
<script setup>
import { computed, ref, watch } from 'vue'
import { BarChart3, Download, Table2, TrendingUp } from 'lucide-vue-next'
import ChartRenderer from './ChartRenderer.vue'
import KpiWidget from './KpiWidget.vue'
import DataTableWidget from './DataTableWidget.vue'

const props = defineProps({
  widget: { type: Object, required: true },
  result: { type: Object, default: null },
})

const emit = defineEmits(['link-filter', 'metric-trend'])

const chartData = computed(() => props.result?.data || null)
const error = computed(() => props.result?.error || '')
const config = computed(() => props.widget.config || props.result?.config || {})
const isText = computed(() => props.widget.chart_type === 'text')
const chartRef = ref(null)
const detailVisible = ref(false)
const detailTitle = ref('')
const detailData = ref(null)

// 手动切换的视图；空字符串表示跟随默认规则（单行表格 / 多行图表）
const viewMode = ref('')

const rowCount = computed(() => chartData.value?.rows?.length || 0)

const canToggle = computed(
  () => !isText.value && props.widget.chart_type !== 'kpi' && Boolean(chartData.value) && !error.value,
)

const configuredMode = computed(() => {
  const value = String(config.value?.viewMode || config.value?.view_mode || '').toLowerCase()
  return value === 'table' || value === 'chart' ? value : ''
})

const defaultMode = computed(() => {
  // 后台显式指定了默认视图时优先
  if (configuredMode.value) return configuredMode.value
  // 一行数据默认表格，多行数据默认图表
  if (rowCount.value <= 1) return 'table'
  return 'chart'
})

const currentMode = computed(() => viewMode.value || defaultMode.value)
const showTable = computed(() => currentMode.value === 'table')

// 表格型组件切到图表时，使用配置的图形类型，默认柱状图
const renderChartType = computed(() => {
  if (props.widget.chart_type !== 'table') return props.widget.chart_type
  return config.value?.fallbackChartType || config.value?.chartType || 'bar'
})

const chartHeight = computed(() => (renderChartType.value === 'donut' ? '235px' : '230px'))
const elapsedText = computed(() => {
  const elapsed = Number(chartData.value?.elapsed_ms || 0)
  return elapsed > 0 ? `${elapsed}ms` : ''
})
const linkFilter = computed(() => String(config.value?.linkFilter || '').trim())

// 推断组件可做趋势的指标字段：优先组件配置，其次结果里第一个数值列。
// 该字段会交给后端趋势接口，由后端按数据权限聚合。
const trendTarget = computed(() => {
  const cfg = config.value || {}
  const columns = chartData.value?.columns || []
  const row = chartData.value?.rows?.[0] || []
  let field = ''
  for (const key of ['yField', 'rateField', 'actualField', 'valueField']) {
    if (typeof cfg[key] === 'string' && cfg[key].trim()) {
      field = cfg[key].trim()
      break
    }
  }
  if (!field && Array.isArray(cfg.columns)) {
    const entry = cfg.columns.find((item) => item && item.field)
    if (entry) field = entry.field
  }
  if (!field) {
    field = columns.find((name, index) => {
      const value = row[index]
      if (value === null || value === undefined || value === '') return false
      return Number.isFinite(Number(value))
    }) || ''
  }
  if (!field) return null
  let label = field
  let unit = typeof cfg.unit === 'string' ? cfg.unit : ''
  let format = ''
  if (Array.isArray(cfg.columns)) {
    const entry = cfg.columns.find((item) => item && item.field === field)
    if (entry) {
      label = entry.label || label
      unit = entry.unit || unit
      format = entry.format || format
    }
  }
  return { field, label, unit, format }
})

// 有数据且能识别出指标字段时才展示趋势入口
const canTrend = computed(() => Boolean(trendTarget.value) && Boolean(chartData.value) && !error.value)

// 打开趋势弹窗：把组件与指标信息交给页面，由页面统一渲染弹窗
function openTrend(target = null) {
  const item = target || trendTarget.value
  if (!item?.field) return
  emit('metric-trend', {
    widget: props.widget,
    field: item.field,
    label: item.label,
    unit: item.unit,
    format: item.format,
  })
}

// KPI 卡片内点击某个指标：按该指标字段打开趋势
function handleMetricClick(metric) {
  if (!metric?.field) return
  openTrend({
    field: metric.field,
    label: metric.label,
    unit: metric.unit,
    format: metric.format,
  })
}

function downloadImage() {
  chartRef.value?.exportImage(props.widget.title || props.widget.chart_type || 'chart')
}

// 点击图表元素后展示对应分类的明细，和常见 BI 的“查看数据”交互保持一致。
function handleItemClick(payload = {}) {
  const data = chartData.value
  if (!data?.rows?.length) return
  // 配置了全局筛选联动时，点击图表元素直接切换筛选器，而不是弹出明细。
  if (linkFilter.value && payload.name !== undefined && payload.name !== null && payload.name !== '') {
    emit('link-filter', { filterKey: linkFilter.value, value: String(payload.name) })
    return
  }
  const cfg = config.value || {}
  const fields = data.columns || []
  const xField = cfg.xField || fields[0]
  const xIndex = fields.indexOf(xField)
  let matched = []
  const rawIndex = Number(payload.dataIndex)
  const indexUsable = Number.isInteger(rawIndex) && rawIndex >= 0 && rawIndex < data.rows.length
  if (renderChartType.value === 'scatter' && indexUsable) {
    matched = [data.rows[rawIndex]]
  } else if (indexUsable && !payload.name) {
    matched = [data.rows[payload.dataIndex]]
  } else if (xIndex >= 0 && payload.name !== undefined && payload.name !== null) {
    matched = data.rows.filter((row) => String(row[xIndex]) === String(payload.name))
  }
  if (!matched.length) {
    matched = data.rows
  }
  detailTitle.value = `${props.widget.title || '分析组件'}${payload.name !== undefined && payload.name !== null ? ` · ${payload.name}` : ''}`
  detailData.value = { columns: fields, rows: matched, count: matched.length }
  detailVisible.value = true
}

// 仅当组件本身发生变化时重置手动选择，定时刷新数据不打断用户当前视图
watch(
  () => [props.widget.id, props.widget.chart_type],
  () => {
    viewMode.value = ''
  },
)
</script>

<template>
  <div v-if="isText" class="chart-card">
    <div class="chart-card-head">
      <div>
        <h3>{{ widget.title || '文本说明' }}</h3>
        <div v-if="widget.subtitle" class="chart-card-subtitle">{{ widget.subtitle }}</div>
      </div>
    </div>
    <div class="chart-card-body">
      <p style="white-space: pre-wrap; font-size: 11px; line-height: 1.9; color: var(--muted)">
        {{ config.content || '点击编辑补充业务说明。' }}
      </p>
    </div>
  </div>

  <KpiWidget
    v-else-if="widget.chart_type === 'kpi'"
    :data="chartData"
    :config="config"
    @metric-click="handleMetricClick"
  />

  <div v-else class="chart-card">
    <div class="chart-card-head">
      <div>
        <h3>{{ widget.title || '分析组件' }}</h3>
        <div v-if="widget.subtitle" class="chart-card-subtitle">{{ widget.subtitle }}</div>
      </div>
      <div class="widget-head-actions">
        <span v-if="linkFilter" class="tag tag-green" title="点击图表元素可联动全局筛选器">联动</span>
        <span v-if="chartData?.cached" class="tag tag-green" title="20 秒内复用的查询结果">缓存</span>
        <span v-else-if="elapsedText" class="tag">{{ elapsedText }}</span>
        <span v-if="chartData?.count" class="tag">{{ chartData.count }} 行</span>
        <!-- 指标趋势：点击数字或此按钮可查看最近 7 天走势，时间区间可切换 -->
        <button
          v-if="canTrend"
          type="button"
          class="widget-icon-btn"
          title="查看指标趋势"
          aria-label="查看指标趋势"
          @click.stop="openTrend()"
        >
          <TrendingUp :size="13" />
        </button>
        <button
          v-if="chartData && !showTable && !error"
          type="button"
          class="widget-icon-btn"
          title="下载图表图片"
          aria-label="下载图表图片"
          @click.stop="downloadImage"
        >
          <Download :size="12" />
        </button>
        <div v-if="canToggle" class="widget-view-toggle">
          <button
            type="button"
            :class="{ active: currentMode === 'table' }"
            title="表格视图"
            aria-label="表格视图"
            @click="viewMode = 'table'"
          >
            <Table2 :size="12" />
          </button>
          <button
            type="button"
            :class="{ active: currentMode === 'chart' }"
            title="图表视图"
            aria-label="图表视图"
            @click="viewMode = 'chart'"
          >
            <BarChart3 :size="12" />
          </button>
        </div>
      </div>
    </div>
    <div class="chart-card-body">
      <div v-if="error" class="chart-error">{{ error }}</div>
      <template v-else-if="chartData">
        <DataTableWidget
          v-if="showTable"
          :data="chartData"
          :config="config"
          :title="widget.title || '数据明细'"
        />
        <ChartRenderer
          v-else
          ref="chartRef"
          :data="chartData"
          :config="config"
          :chart-type="renderChartType"
          :height="chartHeight"
          @item-click="handleItemClick"
        />
      </template>
      <div v-else class="chart-error">正在加载数据…</div>
    </div>
  </div>

  <el-dialog
    v-model="detailVisible"
    :title="detailTitle"
    width="min(860px, 92vw)"
    append-to-body
    destroy-on-close
  >
    <p class="widget-detail-tip">点击图表元素查看对应明细；表格支持排序、合计与 CSV 导出。</p>
    <DataTableWidget
      v-if="detailData"
      :data="detailData"
      :config="{}"
      :title="detailTitle"
    />
  </el-dialog>
</template>

<style scoped>
.widget-head-actions {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  flex-shrink: 0;
}

.widget-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 1px solid var(--line);
  border-radius: 6px;
  background: var(--surface-soft);
  color: var(--muted);
  cursor: pointer;
}

.widget-icon-btn:hover {
  color: var(--brand-deep);
  border-color: var(--brand);
}

.widget-detail-tip {
  margin: 0 0 10px;
  color: var(--muted-light);
  font-size: 9px;
}

.widget-view-toggle {
  display: inline-flex;
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--line);
  border-radius: 7px;
  background: var(--surface-soft);
}

.widget-view-toggle button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  padding: 0;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--muted-light);
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.widget-view-toggle button:hover {
  color: var(--brand-deep);
}

.widget-view-toggle button.active {
  background: #fff;
  color: var(--brand-deep);
  box-shadow: 0 1px 2px rgba(16, 42, 35, 0.08);
}
</style>
