<!-- 指标趋势弹窗：
     在看板 / 分享页点击指标数字或趋势按钮时打开，展示该组件指标的按日趋势。
     默认最近 7 天，支持近 7 / 30 / 90 天与自定义日期区间；
     趋势数据由后端按组织数据权限计算，和看板本身的数据边界完全一致。 -->
<script setup>
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { TrendingUp } from 'lucide-vue-next'
import echarts from '../utils/echarts'
import { api, getErrorMessage } from '../api/client'
import { compactNumber, dateRange, formatValue, toNumber } from '../utils/format'
import NumText from './NumText.vue'

const props = defineProps({
  // 弹窗是否可见（配合 v-model 使用）
  modelValue: { type: Boolean, default: false },
  // 关联看板 ID，登录态使用
  dashboardId: { type: [Number, String], default: null },
  // 分享令牌，存在时改用公开趋势接口
  shareToken: { type: String, default: '' },
  // 组件信息：{ id, title }
  widget: { type: Object, default: null },
  // 指标字段与展示元信息
  field: { type: String, default: '' },
  label: { type: String, default: '' },
  unit: { type: String, default: '' },
  format: { type: String, default: '' },
  // 看板当前生效的全局筛选条件
  filters: { type: Object, default: () => ({}) },
})

const emit = defineEmits(['update:modelValue'])

const loading = ref(false)
const trend = ref(null)
const error = ref('')
const days = ref(7)
const range = ref(dateRange(7))
const chartEl = ref(null)
let chart = null

const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

const title = computed(
  () => `${props.widget?.title || '指标'} · ${trend.value?.field_label || props.label || props.field || '趋势'}`,
)

// 趋势值格式化：优先使用后端返回的字段格式
const valueFormat = computed(() => {
  const format = trend.value?.format || props.format
  if (format === 'percent' || format === 'integer' || format === 'currency') return format
  return 'number'
})

// 比率型指标（毛利率、达成率）不做合计，改看区间均值与最低值
const isRate = computed(() => valueFormat.value === 'percent')

// 日期参数：分享模式与登录模式使用不同的接口前缀
function endpoint() {
  const widgetId = encodeURIComponent(String(props.widget?.id || ''))
  if (props.shareToken) {
    return `/public/dashboards/${encodeURIComponent(props.shareToken)}/widgets/${widgetId}/trend`
  }
  return `/dashboards/${props.dashboardId}/widgets/${widgetId}/trend`
}

// 加载趋势数据
async function load() {
  if (!props.widget?.id || !range.value?.[0]) return
  const widgetId = String(props.widget.id)
  if (!props.shareToken && !props.dashboardId) return
  loading.value = true
  error.value = ''
  try {
    const query = new URLSearchParams({
      start_date: range.value[0],
      end_date: range.value[1],
    })
    if (props.field) query.set('field', props.field)
    if (props.filters && Object.keys(props.filters).length) {
      query.set('filters', JSON.stringify(props.filters))
    }
    trend.value = await api.get(`${endpoint()}?${query.toString()}`)
    await nextTick()
    renderChart()
  } catch (err) {
    trend.value = null
    error.value = getErrorMessage(err)
    chart?.dispose()
    chart = null
  } finally {
    loading.value = false
  }
}

// 快捷时间范围
function chooseDays(value) {
  days.value = value
  range.value = dateRange(value)
  load()
}

// 自定义时间范围
function chooseRange(value) {
  // 清空日期时回到默认的近 7 天，避免停留在上一段数据上
  if (!value || !value[0]) {
    days.value = 7
    range.value = dateRange(7)
    load()
    return
  }
  days.value = 0
  load()
}

// 数值格式化（tooltip / 轴标签）
function formatTrendValue(value) {
  const format = valueFormat.value
  if (format === 'percent') return `${toNumber(value).toFixed(1)}%`
  const unit = trend.value?.unit || props.unit || ''
  return formatValue(value, format, unit)
}

// 渲染趋势折线图
function renderChart() {
  const points = trend.value?.points || []
  if (!chartEl.value || !points.length) {
    chart?.dispose()
    chart = null
    return
  }
  if (chart && chart.getDom() !== chartEl.value) {
    chart.dispose()
    chart = null
  }
  if (!chart) chart = echarts.init(chartEl.value)
  const isPercent = valueFormat.value === 'percent'
  chart.setOption(
    {
      grid: { left: 8, right: 18, top: 28, bottom: 6, containLabel: true },
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(16, 42, 35, 0.94)',
        borderWidth: 0,
        textStyle: { color: '#eafff5', fontSize: 11 },
        valueFormatter: (value) => formatTrendValue(value),
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
          name: trend.value?.field_label || '趋势',
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
  chart.resize()
}

// 弹窗动画结束后再初始化图表，保证容器已有尺寸
function onOpened() {
  renderChart()
}

// 窗口尺寸变化时重置图表，保持自适应
function handleResize() {
  chart?.resize()
}

// 每次打开时按传入的字段与默认区间重新加载
watch(
  () => props.modelValue,
  (value) => {
    if (!value) {
      // 关闭时移除窗口缩放监听，避免重复叠加
      window.removeEventListener('resize', handleResize)
      chart?.dispose()
      chart = null
      return
    }
    days.value = 7
    range.value = dateRange(7)
    trend.value = null
    error.value = ''
    // 打开前先移除旧监听，保证每次打开只注册一次
    window.removeEventListener('resize', handleResize)
    window.addEventListener('resize', handleResize)
    load()
  },
)

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  chart?.dispose()
  chart = null
})
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="title"
    width="min(820px, 94vw)"
    append-to-body
    destroy-on-close
    @opened="onOpened"
  >
    <div class="trend-toolbar">
      <div class="date-range">
        <button :class="{ active: days === 7 }" type="button" @click="chooseDays(7)">近 7 天</button>
        <button :class="{ active: days === 30 }" type="button" @click="chooseDays(30)">近 30 天</button>
        <button :class="{ active: days === 90 }" type="button" @click="chooseDays(90)">近 90 天</button>
      </div>
      <el-date-picker
        v-model="range"
        type="daterange"
        value-format="YYYY-MM-DD"
        range-separator="至"
        start-placeholder="开始日期"
        end-placeholder="结束日期"
        size="small"
        style="width: 240px"
        @change="chooseRange"
      />
    </div>

    <div class="metric-mini-grid trend-summary">
      <div class="metric-mini">
        <span>{{ isRate ? '区间均值' : '区间合计' }}</span>
        <strong>
          <NumText :value="isRate ? trend?.average : trend?.total" :format="valueFormat" start-compact />
        </strong>
        <small>{{ trend?.start_date || '—' }} 至 {{ trend?.end_date || '—' }}</small>
      </div>
      <div class="metric-mini">
        <span>{{ isRate ? '区间最低' : '日均' }}</span>
        <strong>
          <NumText :value="isRate ? trend?.min : trend?.average" :format="valueFormat" start-compact />
        </strong>
        <small>{{ isRate ? trend?.min_date || '—' : `${trend?.points?.length || 0} 天` }}</small>
      </div>
      <div class="metric-mini">
        <span>峰值</span>
        <strong><NumText :value="trend?.peak" :format="valueFormat" start-compact /></strong>
        <small>{{ trend?.peak_date || '—' }}</small>
      </div>
      <div class="metric-mini">
        <span>数据来源</span>
        <strong class="source-text">
          <TrendingUp :size="14" />
          {{ trend?.source === 'scope' ? '组织范围汇总' : '数据集按日聚合' }}
        </strong>
        <small>{{ trend?.date_column ? `日期字段 ${trend.date_column}` : '按指标口径计算' }}</small>
      </div>
    </div>

    <div v-if="error" class="trend-error">{{ error }}</div>
    <div v-else v-loading="loading" class="trend-chart" ref="chartEl" />
    <div v-if="!loading && !error && !trend?.points?.length" class="empty-state" style="padding: 20px">
      所选区间内暂无趋势数据，可切换时间范围后重试
    </div>
  </el-dialog>
</template>

<style scoped>
.trend-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 14px;
}
.trend-summary {
  grid-template-columns: repeat(4, minmax(0, 1fr));
  margin-bottom: 12px;
}
.trend-summary .metric-mini small {
  display: block;
  margin-top: 3px;
  color: var(--muted-light);
  font-size: 8px;
}
.source-text {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 10.5px;
}
.trend-chart {
  height: 300px;
  width: 100%;
}
.trend-error {
  padding: 22px;
  border-radius: 10px;
  background: var(--amber-soft, #fff8ec);
  color: #b45309;
  font-size: 10px;
  line-height: 1.8;
}
@media (max-width: 720px) {
  .trend-toolbar {
    flex-direction: column;
    align-items: stretch;
  }
  .trend-summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .trend-chart {
    height: 220px;
  }
}
</style>
