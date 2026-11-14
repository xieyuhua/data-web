<!-- 看板筛选栏：按看板配置渲染下拉 / 多选 / 日期 / 文本筛选器，
     筛选值以 { 占位符 key: 值 } 的形式传回父组件，最终替换 SQL 中的 {filter:xxx} 占位符 -->
<script setup>
import { computed } from 'vue'
import { Filter, RotateCcw } from 'lucide-vue-next'

const props = defineProps({
  // 看板保存的筛选器配置
  filters: { type: Array, default: () => [] },
  // 当前筛选值集合
  modelValue: { type: Object, default: () => ({}) },
  loading: { type: Boolean, default: false },
})

const emit = defineEmits(['update:modelValue', 'change', 'reset'])

const values = computed(() => props.modelValue || {})

// 判断是否为多选类型
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

// 单选返回原值，多选统一转成数组
function valueOf(item) {
  return normalizeValue(item, values.value[item.key])
}

// 判断某个筛选器是否已生效
function isEmpty(item) {
  const value = valueOf(item)
  if (Array.isArray(value)) return value.length === 0
  return String(value).trim() === ''
}

// 是否有任意筛选条件生效（用于启用“重置”按钮）
const hasValue = computed(() =>
  props.filters.some((item) => !isEmpty(item)),
)

// 更新单个筛选值：同步 v-model 并触发查询
function setValue(key, value) {
  const next = { ...values.value, [key]: value ?? '' }
  emit('update:modelValue', next)
  emit('change', next)
}

// 重置为配置中的默认值
function reset() {
  const next = {}
  for (const item of props.filters) next[item.key] = normalizeValue(item, item.default)
  emit('update:modelValue', next)
  emit('change', next)
  emit('reset', next)
}

// 根据类型计算输入控件宽度
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
