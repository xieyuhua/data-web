<template>
  <div class="filter-bar">
    <div v-for="f in filters" :key="f.id" class="filter-item">
      <span class="filter-label">{{ f.label || f.name }}</span>

      <el-select
        v-if="f.filterType === 'select'"
        v-model="model[f.name]"
        :multiple="!!f.multiple"
        :collapse-tags="!!f.multiple"
        collapse-tags-tooltip
        clearable
        filterable
        :placeholder="f.multiple ? '可多选' : '全部'"
        :style="{ width: f.multiple ? '240px' : '170px' }"
        @change="emitChange"
      >
        <el-option v-for="o in options[f.id] || []" :key="o.value" :label="o.label" :value="o.value" />
      </el-select>

      <el-date-picker
        v-else-if="f.filterType === 'date'"
        v-model="model[f.name]"
        type="date"
        value-format="YYYY-MM-DD"
        placeholder="选择日期"
        style="width: 160px"
        @change="emitChange"
      />

      <el-date-picker
        v-else-if="f.filterType === 'daterange'"
        v-model="model[f.name]"
        type="daterange"
        value-format="YYYY-MM-DD"
        start-placeholder="开始"
        end-placeholder="结束"
        unlink-panels
        style="width: 260px"
        @change="emitChange"
      />

      <el-input-number
        v-else-if="f.filterType === 'number'"
        v-model="model[f.name]"
        controls-position="right"
        placeholder="数值"
        style="width: 140px"
        @change="emitChange"
      />

      <el-input
        v-else
        v-model="model[f.name]"
        placeholder="输入筛选值"
        clearable
        style="width: 170px"
        @change="emitChange"
      />
    </div>
    <el-button v-if="filters.length" link type="primary" @click="reset">重置</el-button>
  </div>
</template>

<script setup lang="ts">
import { reactive, watch } from 'vue'
import type { FilterOption } from '@/types'

interface FilterDef {
  id: number
  name: string
  label: string
  filterType: string
  defaultValueJson?: string
  hasOptions?: boolean
  multiple?: boolean
}

const props = defineProps<{
  filters: FilterDef[]
  options: Record<number, FilterOption[]>
  initial?: Record<string, any>
}>()

const emit = defineEmits<{
  (e: 'change', values: Record<string, any>): void
}>()

const model = reactive<Record<string, any>>({})

function defaults(): Record<string, any> {
  const out: Record<string, any> = {}
  for (const f of props.filters) {
    if (props.initial && props.initial[f.name] !== undefined) {
      out[f.name] = props.initial[f.name]
      continue
    }
    if (f.defaultValueJson) {
      try {
        const v = JSON.parse(f.defaultValueJson)
        out[f.name] = v
        continue
      } catch {
        // ignore invalid default
      }
    }
    if (f.filterType === 'daterange') out[f.name] = null
    else if (f.filterType === 'select' && f.multiple) out[f.name] = []
    else out[f.name] = f.filterType === 'number' ? null : ''
  }
  return out
}

function syncValues() {
  const d = defaults()
  for (const k of Object.keys(model)) delete model[k]
  Object.assign(model, d)
}

function emitChange() {
  emit('change', { ...model })
}

function reset() {
  syncValues()
  emitChange()
}

watch(() => props.filters, () => {
  syncValues()
  emitChange()
}, { immediate: true })
</script>

<style scoped>
.filter-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 14px;
  padding: 10px 12px;
  background: #fff;
  border-radius: 8px;
  margin-bottom: 12px;
}
@media (max-width: 640px) {
  .filter-bar {
    gap: 10px;
    padding: 10px;
  }
  .filter-item {
    width: 100%;
  }
  .filter-item :deep(.el-select),
  .filter-item :deep(.el-date-editor),
  .filter-item :deep(.el-input-number),
  .filter-item :deep(.el-input) {
    width: 100% !important;
    flex: 1;
  }
}
.filter-item {
  display: flex;
  align-items: center;
  gap: 8px;
}
.filter-label {
  font-size: 13px;
  color: #606266;
  white-space: nowrap;
}
</style>
