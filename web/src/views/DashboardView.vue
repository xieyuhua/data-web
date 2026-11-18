<template>
  <div v-loading="loading" class="view-page">
    <div class="page-header">
      <div class="flex-gap">
        <el-button :icon="ArrowLeft" circle @click="router.push('/dashboards')" />
        <span class="page-title">{{ dashboard?.name || '仪表盘' }}</span>
        <el-tag v-if="permission === 'view'" size="small" type="info">只读</el-tag>
      </div>
      <div class="flex-gap">
        <span v-if="elapsedMs" class="muted">加载耗时 {{ elapsedMs }}ms</span>
        <el-dropdown @command="setAutoRefresh">
          <el-button :icon="Clock">
            {{ autoRefresh ? `每 ${autoRefreshLabel} 刷新` : '自动刷新' }}
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item :command="0">关闭</el-dropdown-item>
              <el-dropdown-item :command="15">每 15 秒</el-dropdown-item>
              <el-dropdown-item :command="60">每 1 分钟</el-dropdown-item>
              <el-dropdown-item :command="300">每 5 分钟</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button :icon="Refresh" @click="reload">刷新</el-button>
        <el-button :icon="Download" @click="exportXlsx">导出 Excel</el-button>
        <el-button v-if="permission === 'edit'" type="primary" :icon="Edit" @click="router.push(`/dashboards/${id}/edit`)">
          编辑
        </el-button>
      </div>
    </div>

    <FilterBar v-if="filters.length" :filters="filters" :options="options" :initial="filterValues" @change="onFilterChange" />

    <div v-if="!widgets.length" class="empty">
      <el-empty description="仪表盘还没有组件">
        <el-button v-if="permission === 'edit'" type="primary" @click="router.push(`/dashboards/${id}/edit`)">去编辑</el-button>
      </el-empty>
    </div>

    <GridCanvas v-else :layout="layout" :editable="false">
      <template #default="{ item }">
        <WidgetFrame :widget="item.widget">
          <ChartRenderer
            v-if="item.widget.widgetType === 'chart'"
            ref="renderers"
            :type="item.widget.chartType || 'table'"
            :result="results[item.widget.id]?.result"
            :config="item.widget.chartConfig"
          />
          <div v-else class="text-widget">{{ textOf(item.widget) }}</div>
          <div v-if="results[item.widget.id]?.error" class="widget-error">
            {{ results[item.widget.id]?.error }}
          </div>
        </WidgetFrame>
      </template>
    </GridCanvas>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, Clock, Download, Edit, Refresh } from '@element-plus/icons-vue'
import { chartApi, dashboardApi } from '@/api'
import { useDashboardRuntime } from '@/composables/useDashboardData'
import type { Dashboard, DashboardFilter, DashboardWidget } from '@/types'
import FilterBar from '@/components/dashboard/FilterBar.vue'
import GridCanvas from '@/components/dashboard/GridCanvas.vue'
import WidgetFrame from '@/components/dashboard/WidgetFrame.vue'
import ChartRenderer from '@/components/charts/ChartRenderer.vue'

const route = useRoute()
const router = useRouter()
const id = Number(route.params.id)

const dashboard = ref<Dashboard | null>(null)
const permission = ref<'edit' | 'view'>('view')
const widgets = ref<DashboardWidget[]>([])
const filters = ref<DashboardFilter[]>([])
const filterValues = reactive<Record<string, any>>({})
const renderers = ref<any[]>([])

const { results, options, loading, elapsedMs, refresh } = useDashboardRuntime({
  loadData: (v) => dashboardApi.data(id, v) as any,
  loadFilterOptions: (fid) => dashboardApi.filterOptions(id, fid),
})

const chartIndex = reactive<Record<number, { type: string; config: any }>>({})

const layout = computed(() =>
  widgets.value.map((w) => ({
    i: String(w.id),
    x: w.x,
    y: w.y,
    w: w.w,
    h: w.h,
    // chartType/chartConfig come from the chart definition, not the widget row;
    // omitting chartType would silently render every chart as a table.
    widget: {
      ...w,
      chartType: chartIndex[w.chartId || 0]?.type || 'table',
      chartConfig: chartConfigOf(w),
    },
  })),
)

function chartConfigOf(w: DashboardWidget) {
  const chart = chartIndex[w.chartId || 0]
  return chart?.config || null
}

function textOf(w: DashboardWidget) {
  try {
    const opt = JSON.parse(w.optionsJson || '{}')
    return opt.text || ''
  } catch {
    return ''
  }
}

async function load() {
  const res = await dashboardApi.get(id)
  dashboard.value = res.dashboard
  permission.value = res.permission
  widgets.value = res.dashboard.widgets || []
  filters.value = res.dashboard.filters || []
  // Load chart display definitions once.
  for (const w of widgets.value) {
    if (w.widgetType === 'chart' && w.chartId && !chartIndex[w.chartId]) {
      try {
        const chart = await chartApi.get(w.chartId)
        chartIndex[w.chartId] = { type: chart.type, config: JSON.parse(chart.configJson || '{}') }
      } catch {
        chartIndex[w.chartId] = { type: 'table', config: null }
      }
    }
  }
  // Seed from filter defaults; FilterBar emits the same values on mount, so the
  // first data request carries the correct parameters.
  for (const f of filters.value) {
    if (!f.defaultValueJson) continue
    try {
      filterValues[f.name] = JSON.parse(f.defaultValueJson)
    } catch {}
  }
  await refresh(filters.value, { ...filterValues })
}

async function reload() {
  await refresh(filters.value, { ...filterValues })
}

async function onFilterChange(values: Record<string, any>) {
  for (const k of Object.keys(filterValues)) delete filterValues[k]
  Object.assign(filterValues, values)
  await refresh(filters.value, { ...filterValues })
}

async function exportXlsx() {
  await dashboardApi.export(id, { ...filterValues })
  ElMessage.success('导出已开始')
}

// ---------- Auto refresh ----------

const autoRefresh = ref(0)
let timer: number | null = null

const autoRefreshLabel = computed(() =>
  autoRefresh.value >= 60 ? `${autoRefresh.value / 60} 分钟` : `${autoRefresh.value} 秒`,
)

function setAutoRefresh(seconds: number) {
  autoRefresh.value = seconds
  if (timer !== null) {
    window.clearInterval(timer)
    timer = null
  }
  if (seconds > 0) {
    timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') reload()
    }, seconds * 1000)
  }
}

function stopAutoRefresh() {
  if (timer !== null) {
    window.clearInterval(timer)
    timer = null
  }
}

onMounted(async () => {
  try {
    await load()
  } catch {
    ElMessage.error('无法加载仪表盘')
  }
})

onBeforeUnmount(stopAutoRefresh)
</script>

<style scoped>
.view-page {
  min-height: 100%;
}
@media (max-width: 640px) {
  .view-page :deep(.page-header) {
    gap: 8px;
  }
  .view-page :deep(.page-header .flex-gap) {
    width: 100%;
  }
}
.empty {
  background: #fff;
  border-radius: 8px;
  padding: 60px 0;
}
.text-widget {
  white-space: pre-wrap;
  color: #303133;
  font-size: 14px;
  line-height: 1.7;
  overflow: auto;
  height: 100%;
}
.widget-error {
  color: #f56c6c;
  font-size: 12px;
  padding: 4px 6px;
}
</style>
