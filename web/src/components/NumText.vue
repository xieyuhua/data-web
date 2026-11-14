<!-- 数字展示组件：
     默认展示原始数字，点击可在“原始值”与“万/亿缩写”之间切换（10000 ⇄ 1万），
     所有展示指标数字的地方统一使用该组件，避免各处写法不一致 -->
<script setup>
import { computed, ref } from 'vue'
import { compactValue, formatValue, toNumber } from '../utils/format'

const props = defineProps({
  // 原始数值，也允许传入非数字字符串（此时原样展示）
  value: { type: [Number, String], default: null },
  // number | currency | integer | percent
  format: { type: String, default: 'number' },
  // 单位后缀，如 元、单、人
  unit: { type: String, default: '' },
  // 首次展示是否使用万/亿缩写
  startCompact: { type: Boolean, default: false },
  // 是否允许点击切换（嵌入其他可点击区域时可关闭）
  interactive: { type: Boolean, default: true },
  // 空值展示文案
  placeholder: { type: String, default: '—' },
})

// 当前是否使用缩写展示，点击后取反
const compact = ref(props.startCompact)

// 空值判断：null / undefined / '' 都视为空
const isBlank = computed(
  () => props.value === null || props.value === undefined || props.value === '',
)

// 是否为可格式化数字（排除空值与非数字字符串）
const isNumeric = computed(() => {
  if (isBlank.value) return false
  if (typeof props.value === 'string' && Number.isNaN(Number(props.value))) return false
  return Number.isFinite(Number(props.value))
})

// 完整数字文案（千分位，小数按格式处理）
const fullText = computed(() => {
  if (props.format === 'percent') {
    return `${toNumber(props.value).toLocaleString('zh-CN', { maximumFractionDigits: 1, minimumFractionDigits: 1 })}%`
  }
  return formatValue(props.value, props.format, props.unit)
})

// 缩写文案（万 / 亿），百分比格式保持原样
const compactText = computed(() => {
  if (props.format === 'percent') {
    return `${toNumber(props.value).toLocaleString('zh-CN', { maximumFractionDigits: 1, minimumFractionDigits: 1 })}%`
  }
  return compactValue(props.value, props.unit)
})

// 仅在可交互、且缩写与完整文案不同时才允许点击切换
const canToggle = computed(
  () => props.interactive && isNumeric.value && compactText.value !== fullText.value,
)

// 最终展示文案
const text = computed(() => {
  if (isBlank.value) return props.placeholder
  if (!isNumeric.value) return String(props.value)
  return compact.value ? compactText.value : fullText.value
})

// 点击切换原始值 / 缩写的核心逻辑
function toggle() {
  if (!canToggle.value) return
  compact.value = !compact.value
}
</script>

<template>
  <span
    class="num-toggle"
    :class="{ clickable: canToggle }"
    :tabindex="canToggle ? 0 : undefined"
    :title="canToggle ? '点击切换 原始数字 / 万·亿缩写' : undefined"
    @click.stop="toggle"
    @keydown.enter.stop.prevent="toggle"
    @keydown.space.stop.prevent="toggle"
  >{{ text }}</span>
</template>
