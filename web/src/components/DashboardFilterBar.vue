<script setup>
import { computed } from 'vue'
import { Filter, RotateCcw } from 'lucide-vue-next'

const props = defineProps({
  filters: { type: Array, default: () => [] },
  modelValue: { type: Object, default: () => ({}) },
  loading: { type: Boolean, default: false },
})

const emit = defineEmits(['update:modelValue', 'change', 'reset'])

const values = computed(() => props.modelValue || {})

function isMulti(item) {
  return item.type === 'multiselect'
}

// 多选筛选器在阅读页统一使用数组，兼容后端默认值中用逗号分隔的写法。
function normalizeValue(item, value) {
  if (isMulti(item)) {
    if (Array.isArray(value)) return value
    return String(value ?? '')
      .split(/[,，;；]+/)
      .map((part) => part.trim())
      .filter(Boolean)
  }
  return value ?? ''
}

function valueOf(item) {
  return normalizeValue(item, values.value[item.key])
}

function isEmpty(item) {
  const value = valueOf(item)
  if (Array.isArray(value)) return value.length === 0
  return String(value).trim() === ''
}

const hasValue = computed(() =>
  props.filters.some((item) => !isEmpty(item)),
)

function setValue(key, value) {
  const next = { ...values.value, [key]: value ?? '' }
  emit('update:modelValue', next)
  emit('change', next)
}

function reset() {
  const next = {}
  for (const item of props.filters) next[item.key] = normalizeValue(item, item.default)
  emit('update:modelValue', next)
  emit('change', next)
  emit('reset', next)
}

const inputWidth = (item) => (isMulti(item) ? 200 : item.type === 'date' ? 150 : 150)
</script>

<template>
  <div v-if="filters.length" class="dashboard-filter-bar">
    <span class="dashboard-filter-title">
      <Filter :size="13" />
      筛选
    </span>
    <div v-for="item in filters" :key="item.key" class="dashboard-filter-item">
      <span class="dashboard-filter-label" :title="`SQL 占位符：{{filter:${item.key}}}`">
        {{ item.label || item.key }}
      </span>
      <el-select
        v-if="item.type === 'select'"
        :model-value="valueOf(item)"
        size="small"
        clearable
        filterable
        :placeholder="item.placeholder || '全部'"
        style="width: 150px"
        :disabled="loading"
        @update:model-value="setValue(item.key, $event)"
      >
        <el-option v-for="option in item.options || []" :key="option" :label="option" :value="option" />
      </el-select>
      <el-select
        v-else-if="item.type === 'multiselect'"
        :model-value="valueOf(item)"
        multiple
        collapse-tags
        collapse-tags-tooltip
        clearable
        filterable
        size="small"
        :placeholder="item.placeholder || '全部'"
        :style="{ width: `${inputWidth(item)}px` }"
        :disabled="loading"
        @update:model-value="setValue(item.key, $event)"
      >
        <el-option v-for="option in item.options || []" :key="option" :label="option" :value="option" />
      </el-select>
      <el-date-picker
        v-else-if="item.type === 'date'"
        :model-value="valueOf(item)"
        type="date"
        value-format="YYYY-MM-DD"
        size="small"
        :placeholder="item.placeholder || '选择日期'"
        :style="{ width: `${inputWidth(item)}px` }"
        :disabled="loading"
        @update:model-value="setValue(item.key, $event)"
      />
      <el-input
        v-else
        :model-value="valueOf(item)"
        size="small"
        clearable
        :placeholder="item.placeholder || '输入后自动筛选'"
        style="width: 150px"
        :disabled="loading"
        @update:model-value="setValue(item.key, $event)"
      />
    </div>
    <button
      class="btn btn-ghost btn-sm"
      type="button"
      :disabled="loading || !hasValue"
      @click="reset"
    >
      <RotateCcw :size="12" /> 重置
    </button>
  </div>
</template>
