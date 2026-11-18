<template>
  <div class="page-card">
    <div class="page-header">
      <span class="page-title">图表</span>
      <el-button type="primary" :icon="Plus" @click="openCreate">新建图表</el-button>
    </div>

    <div class="table-scroll">
      <el-table :data="list" v-loading="loading" border stripe>
      <el-table-column prop="name" label="名称" min-width="180" />
      <el-table-column label="类型" width="110">
        <template #default="{ row }">
          <el-tag effect="plain">{{ typeLabel(row.type) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="数据集" min-width="160">
        <template #default="{ row }">{{ datasetName(row.datasetId) }}</template>
      </el-table-column>
      <el-table-column label="更新时间" width="170">
        <template #default="{ row }">{{ fmt(row.updatedAt) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="260" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="primary" @click="duplicate(row)">复制</el-button>
          <el-dropdown trigger="click" @command="(fmt: string) => exportChart(row, fmt)">
            <el-button link type="primary">导出</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="xlsx">Excel (.xlsx)</el-dropdown-item>
                <el-dropdown-item command="csv">CSV (.csv)</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-button link type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
      </el-table>
    </div>

    <ChartEditorDialog v-model="dialogVisible" :chart="editing" @saved="load" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { chartApi, datasetApi } from '@/api'
import type { Chart, Dataset } from '@/types'
import ChartEditorDialog from '@/components/charts/ChartEditorDialog.vue'

const list = ref<Chart[]>([])
const datasets = ref<Dataset[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const editing = ref<Chart | null>(null)

function typeLabel(t: string) {
  return { bar: '柱状图', line: '折线图', pie: '饼图', table: '表格', kpi: 'KPI' }[t] || t
}
function datasetName(id: number) {
  return datasets.value.find((d) => d.id === id)?.name || `#${id}`
}
function fmt(t: string) {
  return t ? dayjs(t).format('YYYY-MM-DD HH:mm') : ''
}

async function load() {
  loading.value = true
  try {
    const [charts, ds] = await Promise.all([chartApi.list(), datasetApi.list()])
    list.value = charts
    datasets.value = ds
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  dialogVisible.value = true
}

function openEdit(c: Chart) {
  editing.value = c
  dialogVisible.value = true
}

async function exportChart(c: Chart, fmt: string) {
  if (fmt === 'csv') {
    await chartApi.exportCSV(c.id)
  } else {
    await chartApi.export(c.id)
  }
}

async function duplicate(c: Chart) {
  await chartApi.duplicate(c.id)
  ElMessage.success('已复制')
  await load()
}

async function remove(c: Chart) {
  await ElMessageBox.confirm(`确定删除图表「${c.name}」吗？`, '删除确认', { type: 'warning' })
  await chartApi.remove(c.id)
  ElMessage.success('已删除')
  await load()
}

onMounted(load)
</script>
