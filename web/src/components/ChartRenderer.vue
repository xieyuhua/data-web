<!-- 通用图表渲染器：基于 ECharts 封装折线图、柱状图、堆叠图、组合图、散点图、环形图、仪表盘等，
     统一处理图例中文名、坐标轴数字缩写、点击事件回调与容器尺寸自适应 -->
<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import echarts from '../utils/echarts'
import { formatValue, toNumber } from '../utils/format'

const props = defineProps({
  // 查询结果：{ columns: [...], rows: [[...]] }
  data: { type: Object, default: null },
  // 图表配置：xField 维度、yFields 指标系列、排序、TopN、单位等
  config: { type: Object, default: () => ({}) },
  // 图表类型：line / bar / bar_h / bar_stacked / combo / area / scatter / donut / gauge
  chartType: { type: String, default: 'line' },
  height: { type: String, default: '230px' },
})

const emit = defineEmits(['item-click'])

const container = ref(null)
let chart = null
// 容器尺寸监听器：页面宽度变化时自动 resize，保证自适应
let observer = null

// 统一色板，保证各图表风格一致
const palette = ['#0F766E', '#E8A13A', '#4A78B8', '#D15F5F', '#14B8A6', '#7C6BB0', '#6B8E4E']

function rawRows() {
  return props.data?.rows || []
}

function columns() {
  return props.data?.columns || []
}

function columnIndex(field, fallback) {
  const index = columns().indexOf(field)
  return index >= 0 ? index : fallback
}

// 从数据中自动识别数值列（抽样前 8 行全部为数字即视为指标列）
function numericFields() {
  return columns().filter((column, index) => {
    const sample = rawRows().slice(0, 8).map((row) => row[index]).filter((value) => value !== null && value !== '')
    return sample.length > 0 && sample.every((value) => Number.isFinite(Number(value)))
  })
}

// 排序 / Top N：默认按第一个指标系列处理，保持与常见 BI 排行榜交互一致。
function sortFieldName() {
  const config = props.config || {}
  if (config.sortField) return config.sortField
  const configured = Array.isArray(config.yFields) ? config.yFields.find((item) => item && item.field) : null
  if (configured) return configured.field
  if (config.yField) return config.yField
  return numericFields().find((field) => field !== (config.xField || columns()[0])) || ''
}

function rows() {
  const config = props.config || {}
  const order = String(config.sortOrder || '').toLowerCase()
  const topN = Math.max(0, Math.min(2000, Number(config.topN) || 0))
  const field = sortFieldName()
  const index = field ? columnIndex(field, -1) : -1
  let list = rawRows()
  if (index < 0 || (order !== 'asc' && order !== 'desc' && topN <= 0)) {
    return list
  }
  list = [...list].sort((left, right) => {
    const diff = toNumber(left[index]) - toNumber(right[index])
    return order === 'asc' ? diff : -diff
  })
  if (topN > 0) list = list.slice(0, topN)
  return list
}

function seriesColor(field, index) {
  return field.color || palette[index % palette.length]
}

// 图表图例名优先取组件配置里的中文列名，避免直接回退到数据库字段名。
function fieldLabel(field) {
  const config = props.config || {}
  const columnList = Array.isArray(config.columns) ? config.columns : []
  const columnHit = columnList.find((item) => item && item.field === field)
  if (columnHit && columnHit.label) return columnHit.label
  const seriesList = Array.isArray(config.yFields) ? config.yFields : []
  const seriesHit = seriesList.find((item) => item && item.field === field)
  if (seriesHit && seriesHit.name) return seriesHit.name
  return field
}

// 兼容旧看板的 xField/yField 配置，以及新的多系列 yFields 配置。
function resolveSeries(config, fallbackType = 'line') {
  const configured = Array.isArray(config.yFields) ? config.yFields.filter((item) => item && item.field) : []
  if (configured.length) {
    return configured.map((item, index) => ({
      field: item.field,
      name: item.name || item.field,
      color: item.color || palette[index % palette.length],
      type: item.type || fallbackType,
      axis: Number(item.axis) === 1 ? 1 : 0,
    }))
  }
  if (config.yField) {
    return [{
      field: config.yField,
      name: config.name || config.yField,
      color: config.color || palette[0],
      type: fallbackType,
      axis: 0,
    }]
  }
  return numericFields()
    .filter((field) => field !== config.xField)
    .slice(0, 3)
    .map((field, index) => ({
      field,
      name: fieldLabel(field),
      color: palette[index % palette.length],
      type: fallbackType,
      axis: 0,
    }))
}

// 坐标轴数字格式化：超过 1 亿显示“亿”，超过 1 万显示“万”
function axisNumber(value) {
  const number = Number(value)
  if (!Number.isFinite(number)) return value
  if (Math.abs(number) >= 100000000) return `${(number / 100000000).toFixed(1)}亿`
  if (Math.abs(number) >= 10000) return `${(number / 10000).toFixed(0)}万`
  return number.toLocaleString('zh-CN', { maximumFractionDigits: 1 })
}

// 类目轴配置（横向柱图时压缩标签宽度）
function categoryAxis(data, horizontal = false) {
  return {
    type: 'category',
    data,
    axisLine: { lineStyle: { color: '#dfe7e0' } },
    axisTick: { show: false },
    axisLabel: {
      color: '#74867d',
      fontSize: 11,
      interval: 0,
      hideOverlap: true,
      width: horizontal ? 86 : 72,
      overflow: 'truncate',
    },
  }
}

// 数值轴配置：虚线网格 + 缩写刻度
function valueAxis(name = '') {
  return {
    type: 'value',
    name,
    nameTextStyle: { color: '#94a39b', fontSize: 11 },
    splitLine: { lineStyle: { color: '#edf1ed', type: 'dashed' } },
    axisLabel: { color: '#94a39b', fontSize: 11, formatter: (value) => axisNumber(value) },
  }
}

// tooltip 公共配置，数值统一按看板单位格式化
function tooltipBase(config, trigger = 'axis') {
  return {
    trigger,
    confine: true,
    axisPointer: trigger === 'axis' ? { type: 'shadow' } : undefined,
    backgroundColor: 'rgba(16, 42, 35, 0.94)',
    borderWidth: 0,
    textStyle: { color: '#eafff5', fontSize: 12 },
    valueFormatter: (value) => formatValue(value, 'number', config.unit || ''),
  }
}

// 折线 / 面积图
function buildLineOption(areaMode = false) {
  const config = props.config || {}
  const xField = config.xField || columns()[0]
  const xIndex = columnIndex(xField, 0)
  const seriesFields = resolveSeries(config, 'line')
  const useArea = areaMode || config.area === true
  const hasRightAxis = seriesFields.some((field) => field.axis === 1)

  return {
    color: palette,
    animationDuration: 420,
    grid: { left: 8, right: hasRightAxis ? 22 : 16, top: 40, bottom: 8, containLabel: true },
    tooltip: tooltipBase(config, 'axis'),
    legend: seriesFields.length > 1
      ? { top: 0, right: 4, icon: 'circle', itemWidth: 8, itemHeight: 8, itemGap: 14, textStyle: { color: '#6b7d74', fontSize: 11 } }
      : undefined,
    xAxis: {
      ...categoryAxis(rows().map((row) => row[xIndex])),
      boundaryGap: false,
    },
    yAxis: hasRightAxis ? [valueAxis(), valueAxis()] : [valueAxis()],
    series: seriesFields.map((field, index) => {
      const indexInRow = columnIndex(field.field, index + 1)
      const color = seriesColor(field, index)
      return {
        name: field.name,
        type: 'line',
        smooth: config.smooth === false ? 0 : 0.35,
        symbol: 'circle',
        symbolSize: 5,
        showSymbol: rows().length <= 31,
        yAxisIndex: field.axis,
        label: { show: config.showLabel === true, position: 'top', color: '#74867d', fontSize: 10 },
        lineStyle: { width: 2, color },
        itemStyle: { color },
        areaStyle: useArea
          ? {
              opacity: 0.16,
              color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                { offset: 0, color },
                { offset: 1, color: 'rgba(255,255,255,0)' },
              ]),
            }
          : undefined,
        data: rows().map((row) => toNumber(row[indexInRow])),
      }
    }),
  }
}

// 柱状图（支持横向、堆叠）
function buildBarOption({ horizontal = false, stacked = false } = {}) {
  const config = props.config || {}
  const xField = config.xField || columns()[0]
  const xIndex = columnIndex(xField, 0)
  const seriesFields = resolveSeries(config, 'bar')
  const categories = rows().map((row) => row[xIndex])
  const category = categoryAxis(categories, horizontal)
  const value = valueAxis()

  if (!horizontal) {
    category.boundaryGap = true
  }

  return {
    color: palette,
    animationDuration: 420,
    grid: {
      left: 8,
      right: horizontal ? 26 : 12,
      top: seriesFields.length > 1 ? 40 : 20,
      bottom: 8,
      containLabel: true,
    },
    tooltip: tooltipBase(config, 'axis'),
    legend: seriesFields.length > 1
      ? { top: 0, right: 4, icon: 'circle', itemWidth: 8, itemHeight: 8, itemGap: 14, textStyle: { color: '#6b7d74', fontSize: 11 } }
      : undefined,
    xAxis: horizontal ? value : category,
    yAxis: horizontal ? category : value,
    series: seriesFields.map((field, index) => {
      const indexInRow = columnIndex(field.field, index + 1)
      const color = seriesColor(field, index)
      return {
        name: field.name,
        type: 'bar',
        stack: stacked ? 'total' : undefined,
        barMaxWidth: stacked ? 20 : 24,
        label: {
          show: config.showLabel === true,
          position: horizontal ? 'right' : 'top',
          color: '#74867d',
          fontSize: 10,
        },
        itemStyle: {
          borderRadius: horizontal ? [0, 5, 5, 0] : [5, 5, 0, 0],
          color: new echarts.graphic.LinearGradient(
            horizontal ? 0 : 0,
            horizontal ? 0 : 1,
            horizontal ? 1 : 0,
            horizontal ? 0 : 0,
            [
              { offset: 0, color },
              { offset: 1, color: `${color}66` },
            ],
          ),
        },
        data: rows().map((row) => toNumber(row[indexInRow])),
      }
    }),
  }
}

// 组合图：同一维度下柱状 + 折线（可配置双 Y 轴）
function buildComboOption() {
  const config = props.config || {}
  const xField = config.xField || columns()[0]
  const xIndex = columnIndex(xField, 0)
  const seriesFields = resolveSeries(config, 'bar')
  const hasRightAxis = seriesFields.some((field) => field.axis === 1)

  return {
    color: palette,
    animationDuration: 420,
    grid: { left: 8, right: hasRightAxis ? 22 : 16, top: 40, bottom: 8, containLabel: true },
    tooltip: tooltipBase(config, 'axis'),
    legend: { top: 0, right: 4, icon: 'circle', itemWidth: 8, itemHeight: 8, itemGap: 14, textStyle: { color: '#6b7d74', fontSize: 11 } },
    xAxis: categoryAxis(rows().map((row) => row[xIndex])),
    yAxis: hasRightAxis ? [valueAxis(), valueAxis()] : [valueAxis()],
    series: seriesFields.map((field, index) => {
      const indexInRow = columnIndex(field.field, index + 1)
      const color = seriesColor(field, index)
      const common = {
        name: field.name,
        yAxisIndex: field.axis,
        itemStyle: { color },
        data: rows().map((row) => toNumber(row[indexInRow])),
        label: { show: config.showLabel === true, position: 'top', color: '#74867d', fontSize: 10 },
      }
      if (field.type === 'line') {
        return {
          ...common,
          type: 'line',
          smooth: config.smooth === false ? 0 : 0.3,
          symbol: 'circle',
          symbolSize: 5,
          lineStyle: { width: 2, color },
        }
      }
      return {
        ...common,
        type: 'bar',
        barMaxWidth: 22,
        itemStyle: { color, borderRadius: [5, 5, 0, 0] },
      }
    }),
  }
}

// 散点图：可配置气泡大小字段
function buildScatterOption() {
  const config = props.config || {}
  const xField = config.xField || numericFields()[0] || columns()[0]
  const yField = config.yField || numericFields().find((field) => field !== xField) || columns()[1]
  const xIndex = columnIndex(xField, 0)
  const yIndex = columnIndex(yField, Math.min(1, columns().length - 1))
  const sizeIndex = config.sizeField ? columnIndex(config.sizeField, -1) : -1
  const xSample = rows().slice(0, 12).map((row) => row[xIndex]).filter((value) => value !== null && value !== '')
  const numericX = xSample.length > 0 && xSample.every((value) => Number.isFinite(Number(value)))
  const sizes = sizeIndex >= 0 ? rows().map((row) => Math.abs(toNumber(row[sizeIndex]))) : []
  const maxSize = Math.max(...sizes, 0)

  const points = rows().map((row, index) => {
    const x = numericX ? toNumber(row[xIndex]) : index
    const y = toNumber(row[yIndex])
    const size = sizeIndex >= 0 && maxSize > 0 ? 8 + (Math.abs(toNumber(row[sizeIndex])) / maxSize) * 20 : 10
    return {
      name: numericX ? `#${index + 1}` : String(row[xIndex] ?? `#${index + 1}`),
      value: xField === columns()[0] && !numericX ? [index, y] : [x, y],
      symbolSize: size,
      rawIndex: index,
    }
  })

  return {
    color: palette,
    animationDuration: 420,
    grid: { left: 8, right: 18, top: 26, bottom: 8, containLabel: true },
    tooltip: {
      trigger: 'item',
      confine: true,
      backgroundColor: 'rgba(16, 42, 35, 0.94)',
      borderWidth: 0,
      textStyle: { color: '#eafff5', fontSize: 11 },
      formatter: (item) => {
        const [x, y] = item.value || []
        const xText = numericX ? formatValue(x, 'number') : item.name
        return `${item.name}<br/>${xField}：${xText}<br/>${yField}：${formatValue(y, 'number', config.unit || '')}`
      },
    },
    xAxis: numericX
      ? { ...valueAxis(xField), splitLine: { lineStyle: { color: '#edf1ed', type: 'dashed' } } }
      : categoryAxis(rows().map((row) => row[xIndex])),
    yAxis: { ...valueAxis(yField), name: '' },
    series: [
      {
        name: yField,
        type: 'scatter',
        data: points,
        itemStyle: { color: config.color || palette[0], opacity: 0.78 },
        emphasis: { focus: 'series', scale: 1.2 },
      },
    ],
  }
}

// 环形图 / 饼图（支持玫瑰图）
function buildDonutOption() {
  const config = props.config || {}
  const xField = config.xField || columns()[0]
  const yField = config.yField || numericFields().find((field) => field !== xField) || columns()[1]
  const xIndex = columnIndex(xField, 0)
  const yIndex = columnIndex(yField, Math.min(1, columns().length - 1))
  return {
    color: palette,
    animationDuration: 420,
    tooltip: tooltipBase(config, 'item'),
    legend: {
      type: 'scroll',
      orient: 'vertical',
      right: 0,
      top: 'center',
      icon: 'circle',
      itemWidth: 8,
      itemHeight: 8,
      itemGap: 12,
      textStyle: { color: '#74867d', fontSize: 11 },
    },
    series: [
      {
        type: 'pie',
        radius: config.rose ? ['28%', '72%'] : ['48%', '72%'],
        center: ['36%', '50%'],
        roseType: config.rose ? 'radius' : undefined,
        avoidLabelOverlap: true,
        itemStyle: { borderColor: '#fff', borderWidth: 3, borderRadius: 4 },
        label: {
          show: config.showLabel === true,
          color: '#74867d',
          fontSize: 11,
          formatter: '{b}\n{d}%',
        },
        labelLine: { show: config.showLabel === true, length: 6, length2: 6 },
        emphasis: { label: { show: true, fontSize: 12, fontWeight: 700, formatter: '{b}\n{d}%' } },
        data: rows().map((row) => ({ name: row[xIndex], value: toNumber(row[yIndex]) })),
      },
    ],
  }
}

// 仪表盘：用于目标达成率展示
function buildGaugeOption() {
  const config = props.config || {}
  const row = rows()[0] || []
  const actualIndex = columnIndex(config.actualField, 0)
  const targetIndex = columnIndex(config.targetField, 1)
  const actual = toNumber(row[actualIndex])
  const target = toNumber(row[targetIndex])
  const rateIndex = config.rateField ? columnIndex(config.rateField, -1) : -1
  let rate = rateIndex >= 0 ? toNumber(row[rateIndex]) : target > 0 ? (actual / target) * 100 : 0
  if (!Number.isFinite(rate)) rate = 0
  const capped = Math.max(0, Math.min(rate, 150))
  const color = rate >= 100 ? '#0F766E' : rate >= 80 ? '#D69E2E' : '#C53030'
  const unit = config.unit || ''
  const footText = target > 0
    ? `实际 ${formatValue(actual, 'number', unit)} · 目标 ${formatValue(target, 'number', unit)}`
    : ''

  return {
    animationDuration: 420,
    series: [
      {
        type: 'gauge',
        startAngle: 210,
        endAngle: -30,
        min: 0,
        max: 150,
        splitNumber: 5,
        center: ['50%', '58%'],
        radius: '90%',
        axisLine: {
          lineStyle: {
            width: 14,
            color: [
              [Math.max(0.01, capped / 150), color],
              [1, '#E9EFE9'],
            ],
          },
        },
        pointer: { length: '56%', width: 5, itemStyle: { color } },
        anchor: { show: true, size: 9, itemStyle: { color } },
        axisTick: { distance: -14, length: 5, lineStyle: { color: '#fff', width: 1 } },
        splitLine: { distance: -14, length: 12, lineStyle: { color: '#fff', width: 2 } },
        axisLabel: { distance: 18, color: '#94A39B', fontSize: 11 },
        detail: {
          offsetCenter: [0, '36%'],
          fontSize: 22,
          fontWeight: 700,
          color,
          formatter: () => `${rate.toFixed(1)}%`,
        },
        title: { offsetCenter: [0, '66%'], color: '#74867d', fontSize: 12 },
        data: [{ value: capped, name: '目标达成率' }],
      },
    ],
    graphic: footText
      ? [
          {
            type: 'text',
            left: 'center',
            bottom: 2,
            style: { text: footText, fill: '#94A39B', fontSize: 11 },
          },
        ]
      : [],
  }
}

// 根据图表类型分发到对应的配置构建函数
function buildOption() {
  switch (props.chartType) {
    case 'donut':
    case 'pie':
      return buildDonutOption()
    case 'bar':
      return buildBarOption()
    case 'bar_h':
      return buildBarOption({ horizontal: true })
    case 'bar_stacked':
      return buildBarOption({ stacked: true })
    case 'combo':
      return buildComboOption()
    case 'area':
      return buildLineOption(true)
    case 'scatter':
      return buildScatterOption()
    case 'gauge':
      return buildGaugeOption()
    default:
      return buildLineOption()
  }
}

// 初始化或更新图表实例，并转发点击事件给父组件（用于下钻/联动/明细）
function render() {
  if (!container.value) return
  if (!chart) {
    chart = echarts.init(container.value)
    chart.on('click', (params) => {
      emit('item-click', {
        name: params.name,
        seriesName: params.seriesName,
        dataIndex: params.dataIndex,
        value: params.value,
      })
    })
  }
  chart.setOption(buildOption(), true)
  chart.resize()
}

// 导出当前图表为 PNG 图片（2 倍像素，白底）
function exportImage(filename = 'chart') {
  if (!chart) return
  const url = chart.getDataURL({ type: 'png', pixelRatio: 2, backgroundColor: '#ffffff' })
  const link = document.createElement('a')
  link.href = url
  link.download = `${String(filename || 'chart').replace(/[\\/:*?"<>|]/g, '_')}.png`
  document.body.appendChild(link)
  link.click()
  link.remove()
}

defineExpose({ exportImage })

// 组件挂载后渲染，并监听容器尺寸变化实现自适应
onMounted(() => {
  render()
  observer = new ResizeObserver(() => chart?.resize())
  observer.observe(container.value)
})

onBeforeUnmount(() => {
  observer?.disconnect()
  chart?.dispose()
  chart = null
})

watch(
  () => [props.data, props.config, props.chartType],
  () => render(),
  { deep: true },
)
</script>

<template>
  <div ref="container" class="chart-render" :style="{ height }" />
</template>
