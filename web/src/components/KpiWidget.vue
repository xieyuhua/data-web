<script setup>
import { computed } from 'vue'
import NumText from './NumText.vue'
import { toNumber } from '../utils/format'

const props = defineProps({
  data: { type: Object, default: null },
  config: { type: Object, default: () => ({}) },
})

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
    const targetIndex = item.target ? columns.indexOf(item.target) : -1
    const target = targetIndex >= 0 ? toNumber(row[targetIndex]) : 0
    const compareIndex = item.compare ? columns.indexOf(item.compare) : -1
    const compare = compareIndex >= 0 ? toNumber(row[compareIndex]) : 0
    const rate = target > 0 && isNumeric ? (toNumber(value) / target) * 100 : null
    const delta = compare !== 0 && isNumeric ? ((toNumber(value) - compare) / Math.abs(compare)) * 100 : null
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
</script>

<template>
  <div class="kpi-widget">
    <div v-for="(metric, index) in metrics" :key="`${metric.field}-${index}`" class="kpi-cell">
      <div class="kpi-label">{{ metric.label }}</div>
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
