<script setup>
import { computed, ref, watch } from 'vue'
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  ChevronLeft,
  ChevronRight,
  Download,
} from 'lucide-vue-next'
import { toNumber } from '../utils/format'
import NumText from './NumText.vue'

const props = defineProps({
  data: { type: Object, default: null },
  config: { type: Object, default: () => ({}) },
  title: { type: String, default: '数据明细' },
})

const sortField = ref('')
const sortOrder = ref('')
const page = ref(1)
const pageSize = ref(10)

const columns = computed(() => {
  const source = props.data?.columns || []
  const configured = Array.isArray(props.config?.columns) ? props.config.columns : []
  if (configured.length) {
    return configured.map((item) => ({
      field: item.field,
      label: item.label || item.field,
      format: item.format || 'text',
      progress: item.progress === true ? true : item.progress === false ? false : undefined,
      progressColor: item.progressColor || '',
      aggregate: item.aggregate || '',
      index: source.indexOf(item.field),
    }))
  }
  return source.map((field, index) => ({
    field,
    label: field,
    format: 'text',
    progress: undefined,
    progressColor: '',
    aggregate: '',
    index,
  }))
})

const rows = computed(() => props.data?.rows || [])

const sortedRows = computed(() => {
  const list = rows.value.map((row, index) => ({ row, index }))
  const column = columns.value.find((item) => item.field === sortField.value)
  if (!column || !sortOrder.value) {
    return list.map((item, position) => ({ ...item, rank: position + 1 }))
  }
  const direction = sortOrder.value === 'asc' ? 1 : -1
  list.sort((left, right) => {
    const a = column.index >= 0 ? left.row[column.index] : ''
    const b = column.index >= 0 ? right.row[column.index] : ''
    const emptyA = a === null || a === undefined || a === ''
    const emptyB = b === null || b === undefined || b === ''
    if (emptyA && emptyB) return 0
    if (emptyA) return 1
    if (emptyB) return -1
    if (column.format !== 'text') {
      return (toNumber(a) - toNumber(b)) * direction
    }
    return String(a).localeCompare(String(b), 'zh-CN', { numeric: true }) * direction
  })
  return list.map((item, position) => ({ ...item, rank: position + 1 }))
})

const totalPages = computed(() => Math.max(1, Math.ceil(sortedRows.value.length / pageSize.value)))
const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return sortedRows.value.slice(start, start + pageSize.value)
})

const columnStats = computed(() => {
  const stats = {}
  for (const column of columns.value) {
    if (column.index < 0) continue
    const numbers = rows.value
      .map((row) => toNumber(row[column.index]))
      .filter((value) => Number.isFinite(value))
    stats[column.field] = {
      max: numbers.length ? Math.max(...numbers) : 0,
      min: numbers.length ? Math.min(...numbers) : 0,
      sum: numbers.reduce((total, value) => total + value, 0),
      count: numbers.length,
    }
  }
  return stats
})

const conditional = computed(() => {
  const cfg = props.config?.conditional || {}
  if (!cfg.field) return null
  return {
    field: cfg.field,
    operator: cfg.operator === 'gt' ? 'gt' : 'lt',
    threshold: toNumber(cfg.threshold),
    color: cfg.color || '#C53030',
    background: cfg.background || '#FDF2F1',
  }
})

const summaryRow = computed(() => {
  if (!props.config?.showSummary || !rows.value.length) return null
  const values = columns.value.map((column, index) => {
    if (index === 0) return '合计'
    if (column.index < 0 || column.format === 'text') return ''
    const stats = columnStats.value[column.field]
    if (!stats) return ''
    if (column.aggregate === 'avg' || (column.aggregate === '' && column.format === 'percent')) {
      return stats.count ? stats.sum / stats.count : 0
    }
    if (column.aggregate === 'none') return ''
    return stats.sum
  })
  return { columns: columns.value, values }
})

watch(
  () => props.data,
  () => {
    page.value = 1
    sortField.value = ''
    sortOrder.value = ''
  },
)

watch(pageSize, () => {
  page.value = 1
})

function cellValue(row, column) {
  return column.index >= 0 ? row[column.index] : ''
}

function showProgress(column) {
  if (column.progress) return true
  // 兼容历史看板：销售额列默认展示数值条形，可在编辑器关闭。
  return column.field === 'revenue' && column.progress !== false && column.format !== 'text'
}

function progressWidth(row, column) {
  const stats = columnStats.value[column.field]
  const max = Math.abs(stats?.max || 0)
  if (!max) return '0%'
  const value = Math.abs(toNumber(cellValue(row, column)))
  return `${Math.max(2, Math.min(100, (value / max) * 100))}%`
}

function cellAlertStyle(row, column) {
  const rule = conditional.value
  if (!rule || rule.field !== column.field) return undefined
  const value = toNumber(cellValue(row, column))
  const hit = rule.operator === 'gt' ? value > rule.threshold : value < rule.threshold
  if (!hit) return undefined
  return { color: rule.color, background: rule.background, fontWeight: 650 }
}

function toggleSort(column) {
  if (sortField.value !== column.field) {
    sortField.value = column.field
    sortOrder.value = 'asc'
    return
  }
  if (sortOrder.value === 'asc') {
    sortOrder.value = 'desc'
    return
  }
  if (sortOrder.value === 'desc') {
    sortField.value = ''
    sortOrder.value = ''
    return
  }
  sortOrder.value = 'asc'
}

function csvCell(value) {
  if (value === null || value === undefined) return ''
  const text = String(value)
  if (text && /^[=+@\t\r-]/.test(text) && Number.isNaN(Number(text))) {
    return `'${text}`
  }
  return text
}

function exportCsv() {
  const header = columns.value.map((column) => column.label)
  const lines = [header]
  for (const item of sortedRows.value) {
    lines.push(columns.value.map((column) => csvCell(cellValue(item.row, column))))
  }
  const escape = (value) => {
    const text = value === null || value === undefined ? '' : String(value)
    return /[",\r\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text
  }
  const content = `\ufeff${lines.map((line) => line.map(escape).join(',')).join('\r\n')}`
  const blob = new Blob([content], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `${String(props.title || '数据明细').replace(/[\\/:*?"<>|]/g, '_')}.csv`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}
</script>

<template>
  <div class="table-widget">
    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th style="width: 36px">#</th>
            <th
              v-for="column in columns"
              :key="column.field"
              class="sortable-th"
              :class="{ active: sortField === column.field }"
              @click="toggleSort(column)"
            >
              <span class="th-sort">
                {{ column.label }}
                <ArrowUp v-if="sortField === column.field && sortOrder === 'asc'" :size="10" />
                <ArrowDown v-else-if="sortField === column.field && sortOrder === 'desc'" :size="10" />
                <ArrowUpDown v-else :size="10" class="sort-idle" />
              </span>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in pagedRows" :key="item.index">
            <td><span class="rank-index" :class="{ top: item.rank <= 3 }">{{ item.rank }}</span></td>
            <td v-for="column in columns" :key="column.field">
              <template v-if="showProgress(column) && column.index >= 0">
                <div class="progress-cell">
                  <span>
                    <NumText v-if="column.format !== 'text'" :value="cellValue(item.row, column)" :format="column.format" />
                    <template v-else>{{ cellValue(item.row, column) }}</template>
                  </span>
                  <div class="progress-track">
                    <div
                      class="progress-fill"
                      :style="{ width: progressWidth(item.row, column), background: column.progressColor || undefined }"
                    />
                  </div>
                </div>
              </template>
              <template v-else>
                <span :style="cellAlertStyle(item.row, column)">
                  <NumText v-if="column.format !== 'text'" :value="cellValue(item.row, column)" :format="column.format" />
                  <template v-else>{{ cellValue(item.row, column) }}</template>
                </span>
              </template>
            </td>
          </tr>
          <tr v-if="!rows.length">
            <td :colspan="columns.length + 1" style="text-align: center; color: var(--muted)">暂无数据</td>
          </tr>
        </tbody>
        <tfoot v-if="summaryRow">
          <tr class="summary-row">
            <td />
            <td v-for="(value, index) in summaryRow.values" :key="index">
              <NumText
                v-if="index > 0 && summaryRow.columns[index]?.format !== 'text' && value !== ''"
                :value="value"
                :format="summaryRow.columns[index]?.format || 'number'"
              />
              <template v-else>{{ value }}</template>
            </td>
          </tr>
        </tfoot>
      </table>
    </div>

    <div v-if="rows.length" class="table-pager">
      <span class="table-pager-info">共 {{ rows.length }} 行 · 第 {{ page }} / {{ totalPages }} 页</span>
      <div class="table-pager-actions">
        <select v-model.number="pageSize" class="table-page-size" aria-label="每页条数">
          <option :value="10">10 条/页</option>
          <option :value="20">20 条/页</option>
          <option :value="50">50 条/页</option>
        </select>
        <button class="table-page-btn" type="button" :disabled="page <= 1" @click="page -= 1">
          <ChevronLeft :size="12" />
        </button>
        <button class="table-page-btn" type="button" :disabled="page >= totalPages" @click="page += 1">
          <ChevronRight :size="12" />
        </button>
        <button class="table-page-btn" type="button" title="导出当前表 CSV" @click="exportCsv">
          <Download :size="12" />
        </button>
      </div>
    </div>
  </div>
</template>
