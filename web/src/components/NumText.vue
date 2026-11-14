<script setup>
import { computed, ref } from 'vue'
import { compactValue, formatValue, toNumber } from '../utils/format'

const props = defineProps({
  value: { type: [Number, String], default: null },
  // number | currency | integer | percent
  format: { type: String, default: 'number' },
  unit: { type: String, default: '' },
  // 首次展示是否使用万/亿缩写
  startCompact: { type: Boolean, default: false },
  // 是否允许点击切换（嵌入其他可点击区域时可关闭）
  interactive: { type: Boolean, default: true },
  placeholder: { type: String, default: '—' },
})

const compact = ref(props.startCompact)

const isBlank = computed(
  () => props.value === null || props.value === undefined || props.value === '',
)

const isNumeric = computed(() => {
  if (isBlank.value) return false
  if (typeof props.value === 'string' && Number.isNaN(Number(props.value))) return false
  return Number.isFinite(Number(props.value))
})

const fullText = computed(() => {
  if (props.format === 'percent') {
    return `${toNumber(props.value).toLocaleString('zh-CN', { maximumFractionDigits: 1, minimumFractionDigits: 1 })}%`
  }
  return formatValue(props.value, props.format, props.unit)
})

const compactText = computed(() => {
  if (props.format === 'percent') {
    return `${toNumber(props.value).toLocaleString('zh-CN', { maximumFractionDigits: 1, minimumFractionDigits: 1 })}%`
  }
  return compactValue(props.value, props.unit)
})

const canToggle = computed(
  () => props.interactive && isNumeric.value && compactText.value !== fullText.value,
)

const text = computed(() => {
  if (isBlank.value) return props.placeholder
  if (!isNumeric.value) return String(props.value)
  return compact.value ? compactText.value : fullText.value
})

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
