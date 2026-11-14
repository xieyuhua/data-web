<!-- KPI 指标卡组件：把查询结果第一行渲染成若干指标块，支持目标达成率与同比/环比 -->
<script setup>
import { computed } from 'vue'
import { TrendingUp } from 'lucide-vue-next'
import NumText from './NumText.vue'
import { toNumber } from '../utils/format'

const props = defineProps({
  // 查询结果：{ columns: [...], rows: [[...]] }
  data: { type: Object, default: null },
  // 组件配置：columns 指定展示哪些字段、target 达成目标、compare 对比字段
  config: { type: Object, default: () => ({}) },
})

// 点击指标块时把字段信息抛给上层，用于打开「指标趋势」弹窗
const emit = defineEmits(['metric-click'])

// 根据配置或查询列自动生成指标列表，并计算达成率、变化率与状态颜色
const metrics = computed(() => {
  const columns = props.data?.columns || []
  const row = props.data?.rows?.[0] || []
  const configured = Array.isArray(props.config?.columns) ? props.config.columns : []
  const list = configured.length
    ? configured
    : columns.map((field) => ({
        field,
        label: field,
        format: Number.isFinite(Number(row[columns.indexOf(field)])) ? 'number' : 'text',
      }))
  return list.map((item) => {
    const index = columns.indexOf(item.field)
    const value = index >= 0 ? row[index] : null
    const isNumeric = item.format !== 'text' && Number.isFinite(Number(value))
    // 目标值：配置了 target 字段时读取同一行的目标列
    const targetIndex = item.target ? columns.indexOf(item.target) : -1
    const target = targetIndex >= 0 ? toNumber(row[targetIndex]) : 0
    // 对比值：用于计算环比/同比增长率
    const compareIndex = item.compare ? columns.indexOf(item.compare) : -1
    const compare = compareIndex >= 0 ? toNumber(row[compareIndex]) : 0
    // 达成率 = 实际值 / 目标值；对比增长率 = (实际 - 对比) / |对比|
    const rate = target > 0 && isNumeric ? (toNumber(value) / target) * 100 : null
    const delta = compare !== 0 && isNumeric ? ((toNumber(value) - compare) / Math.abs(compare)) * 100 : null
    // 达成率配色：<80% 红色、80%~100% 黄色、>=100% 绿色
    let rateColor = '#0F766E'
    if (rate !== null && rate < 80) rateColor = '#C53030'
    else if (rate !== null && rate < 100) rateColor = '#D69E2E'
    return {
      ...item,
      rawValue: value,
      isNumeric,
      unit: item.unit || '',
      target,
      rate,
      rateColor,
      delta,
    }
  })
})

// 点击指标块：仅数值型指标可查看趋势
function handleClick(metric) {
  if (!metric?.isNumeric) return
  emit('metric-click', {
    field: metric.field,
    label: metric.label,
    unit: metric.unit,
    format: metric.format,
  })
}
</script>

<template>
  <div class="kpi-widget">
    <div
      v-for="(metric, index) in metrics"
      :key="`${metric.field}-${index}`"
      class="kpi-cell"
      :class="{ 'kpi-clickable': metric.isNumeric }"
      :tabindex="metric.isNumeric ? 0 : undefined"
      :title="metric.isNumeric ? '点击查看该指标趋势（点击数字可切换 原始值 / 万·亿）' : undefined"
      @click="handleClick(metric)"
      @keydown.enter.prevent="handleClick(metric)"
    >
      <div class="kpi-label">
        {{ metric.label }}
        <TrendingUp v-if="metric.isNumeric" :size="11" class="kpi-trend-icon" />
      </div>
      <div class="kpi-value">
        <NumText
          v-if="metric.isNumeric"
          :value="metric.rawValue"
          :format="metric.format || 'number'"
        />
        <template v-else>{{ metric.rawValue ?? '—' }}</template>
        <small v-if="metric.unit">{{ metric.unit }}</small>
      </div>
      <div v-if="metric.rate !== null" class="kpi-progress">
        <!-- 达成率进度条：宽度限制在 0~100% 之间 -->
        <div class="kpi-progress-track">
          <div
            class="kpi-progress-fill"
            :style="{
              width: `${Math.max(0, Math.min(100, metric.rate))}%`,
              background: metric.rateColor,
            }"
          />
        </div>
        <span :style="{ color: metric.rateColor }">达成 {{ metric.rate.toFixed(1) }}%</span>
      </div>
      <div class="kpi-foot">
        <span v-if="metric.delta !== null" :class="metric.delta >= 0 ? 'up' : 'down'">
          {{ metric.delta >= 0 ? '↑' : '↓' }} {{ Math.abs(metric.delta).toFixed(1) }}%
        </span>
        <span class="up">● 实时</span>
        <span>{{ config.footnote || '当前权限范围' }}</span>
      </div>
    </div>
    <div v-if="!metrics.length" class="kpi-cell">
      <div class="kpi-label">暂无指标</div>
      <div class="kpi-value">—</div>
      <div class="kpi-foot"><span>请为组件配置 KPI 字段</span></div>
    </div>
  </div>
</template>

<style scoped>
/* 数值型指标块可点击查看趋势，悬停时给出轻量反馈 */
.kpi-clickable {
  cursor: pointer;
  transition: background 0.15s ease;
}
.kpi-clickable:hover {
  background: rgba(20, 184, 166, 0.06);
}
.kpi-clickable:focus-visible {
  outline: 2px solid var(--emerald);
  outline-offset: -2px;
}
.kpi-trend-icon {
  color: var(--emerald);
  opacity: 0.45;
  vertical-align: -1px;
  margin-left: 3px;
  transition: opacity 0.15s ease;
}
.kpi-clickable:hover .kpi-trend-icon {
  opacity: 1;
}
</style>
