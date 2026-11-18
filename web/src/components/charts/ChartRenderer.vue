<template>
  <div ref="root" class="chart-root">
    <div v-if="error" class="chart-placeholder error">{{ error }}</div>
    <template v-else-if="type === 'kpi'">
      <div class="kpi" v-if="kpiValue !== null">
        <div class="kpi-value">{{ kpiValue }}</div>
        <div class="kpi-label">{{ kpiLabel }}</div>
      </div>
      <div v-else class="chart-placeholder">暂无数据</div>
    </template>
    <el-table v-else-if="type === 'table'" :data="tableRows" size="small" border height="100%" class="table">
      <el-table-column v-for="(c, i) in result?.columns || []" :key="c.name" :label="c.name" min-width="110">
        <template #default="{ row }">{{ formatCell(row[i], c) }}</template>
      </el-table-column>
    </el-table>
    <div v-else ref="chartEl" class="echarts"></div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts'
import type { ChartConfig, ColumnMeta, ResultSet } from '@/types'

const props = defineProps<{
  type: string
  result?: ResultSet | null
  config?: ChartConfig | null
}>()

const root = ref<HTMLDivElement>()
const chartEl = ref<HTMLDivElement>()
const error = ref('')
let chart: echarts.ECharts | null = null

const tableRows = computed(() => props.result?.rows || [])

// KPI displays the last metric: for calculated KPIs that is the ratio, while
// earlier columns are only inputs for the expression.
const kpiMetricIndex = computed(() => {
  const rs = props.result
  if (!rs) return 0
  const dims = props.config?.dimensions?.length || 0
  const metricCount = props.config?.metrics?.length || 1
  const idx = dims + metricCount - 1
  return Math.max(0, Math.min(idx, rs.columns.length - 1))
})
const kpiValue = computed(() => {
  const rs = props.result
  if (!rs || !rs.rows.length) return null
  const idx = kpiMetricIndex.value
  return formatCell(rs.rows[0][idx], rs.columns[idx])
})
const kpiLabel = computed(() => {
  const rs = props.result
  if (!rs || !rs.rows.length) return ''
  return rs.columns[kpiMetricIndex.value]?.name || ''
})

function formatCell(v: any, col: ColumnMeta | undefined) {
  if (v === null || v === undefined) return ''
  if (typeof v === 'number') {
    if (col?.logicalType === 'number') {
      const decimals = props.config?.format?.decimals
      if (col.format === 'percent') {
        return `${v.toFixed(decimals !== undefined && decimals >= 0 ? Math.max(decimals, 1) : 2)}%`
      }
      if (col.format === 'currency') {
        const n = new Intl.NumberFormat('zh-CN', {
          minimumFractionDigits: decimals !== undefined && decimals >= 0 ? decimals : 2,
          maximumFractionDigits: decimals !== undefined && decimals >= 0 ? decimals : 2,
        }).format(v)
        return `¥${n}`
      }
      if (decimals !== undefined && decimals !== null && decimals >= 0) return new Intl.NumberFormat('zh-CN', {
        minimumFractionDigits: decimals,
        maximumFractionDigits: decimals,
      }).format(v)
      return new Intl.NumberFormat('zh-CN').format(v)
    }
    return String(v)
  }
  return String(v)
}

function numericRows() {
  const rs = props.result
  const cfg = props.config
  if (!rs || !cfg) return []
  const dimCount = cfg.dimensions?.length || 0
  const metricStart = dimCount
  return rs.rows.map((row) => ({
    dims: row.slice(0, dimCount),
    values: row.slice(metricStart, metricStart + (cfg.metrics?.length || 0)),
  }))
}

function buildOption(): echarts.EChartsOption {
  const rs = props.result!
  const cfg = props.config!
  const dims = cfg.dimensions || []
  const metrics = cfg.metrics || []
  const rows = numericRows()
  const floor = cfg.format?.decimals
  const decimals = floor !== undefined && floor !== null && floor >= 0 ? floor : undefined

  const metricNames = metrics.map((m, i) => m.alias || rs.columns[dims.length + i]?.name || `指标${i + 1}`)
  const dimNames = dims.map((d, i) => d.alias || rs.columns[i]?.name || `维度${i + 1}`)

  const tooltip: any = { trigger: 'axis' }
  const legend: any = { show: cfg.echarts?.legend !== false, top: 0 }
  const grid = { top: 40, left: 48, right: 24, bottom: 30, containLabel: true }

  if (props.type === 'pie') {
    const data = rows.map((r) => ({
      name: String(dims.length > 0 ? r.dims[0] : '合计'),
      value: r.values[0],
    }))
    return {
      tooltip: { trigger: 'item' },
      legend: { show: true, bottom: 0 },
      series: [
        {
          type: 'pie',
          radius: ['35%', '65%'],
          data,
          label: { formatter: '{b}: {d}%' },
          emphasis: { itemStyle: { shadowBlur: 10, shadowColor: 'rgba(0,0,0,.3)' } },
        },
      ],
    }
  }

  // bar / line
  const categories = rows.map((r) => r.dims.map(String).join(' / '))
  const series: any[] = metricNames.map((name, mi) => ({
    name,
    type: props.type === 'line' ? 'line' : 'bar',
    smooth: true,
    barMaxWidth: 48,
    stack: cfg.echarts?.stack ? 'total' : undefined,
    areaStyle: cfg.echarts?.area && props.type === 'line' ? {} : undefined,
    label: {
      show: cfg.echarts?.showLabel === true,
      position: 'top',
      formatter: decimals !== undefined ? (p: any) => Number(p.value).toFixed(decimals) : undefined,
    },
    data: rows.map((r) => r.values[mi]),
  }))
  return {
    tooltip,
    legend,
    grid,
    xAxis: { type: 'category', data: categories, axisLabel: { interval: 'auto', rotate: categories.length > 8 ? 30 : 0 } },
    yAxis: { type: 'value' },
    series,
  }
}

function render() {
  error.value = ''
  if (props.type === 'table' || props.type === 'kpi') return
  if (!props.result || !props.config) return
  nextTick(() => {
    if (!chartEl.value) return
    if (!chart) chart = echarts.init(chartEl.value)
    try {
      chart.setOption(buildOption(), true)
    } catch (e: any) {
      error.value = '图表渲染失败：' + (e?.message || e)
    }
  })
}

let observer: ResizeObserver | null = null
let rafId = 0

function resize() {
  cancelAnimationFrame(rafId)
  rafId = requestAnimationFrame(() => chart?.resize())
}

onMounted(() => {
  render()
  window.addEventListener('resize', resize)
  // Grid items resize without a window resize event; observe the host element.
  if (root.value && typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(() => resize())
    observer.observe(root.value)
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', resize)
  cancelAnimationFrame(rafId)
  observer?.disconnect()
  observer = null
  chart?.dispose()
  chart = null
})

watch(() => [props.result, props.config, props.type], render, { deep: true })

defineExpose({
  resize,
  getDataURL: () => chart?.getDataURL({ type: 'png', pixelRatio: 2, backgroundColor: '#fff' }),
})
</script>

<style scoped>
.chart-root {
  width: 100%;
  height: 100%;
  min-height: 80px;
  position: relative;
}
.echarts {
  width: 100%;
  height: 100%;
  min-height: 80px;
}
.table {
  width: 100%;
  height: 100%;
}
@media (max-width: 640px) {
  .table :deep(.el-table__cell) {
    padding: 4px 0;
  }
  .table :deep(.cell) {
    font-size: 12px;
  }
}
.chart-placeholder {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #909399;
  font-size: 13px;
}
.chart-placeholder.error {
  color: #f56c6c;
  padding: 12px;
  text-align: center;
}
.kpi {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
}
.kpi-value {
  font-size: clamp(22px, 4.2vw, 34px);
  font-weight: 700;
  color: #1f2d3d;
  line-height: 1.15;
  padding: 0 8px;
  text-align: center;
  word-break: break-all;
}
.kpi-label {
  color: #909399;
  font-size: clamp(11px, 2.6vw, 13px);
  text-align: center;
}
</style>
