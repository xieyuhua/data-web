<template>
  <div class="editor-page">
    <div class="toolbar">
      <div class="flex-gap">
        <el-button :icon="ArrowLeft" circle @click="router.push('/dashboards')" />
        <el-input v-model="name" class="name-input" placeholder="仪表盘名称" />
        <el-tag v-if="dirty" size="small" type="warning">未保存</el-tag>
      </div>
      <div class="flex-gap">
        <el-button :icon="View" @click="previewVisible = true">预览</el-button>
        <el-button :icon="Share" @click="shareVisible = true">分享与授权</el-button>
        <el-button :icon="Plus" @click="openAddWidget">添加组件</el-button>
        <el-button :icon="Filter" @click="openAddFilter">添加筛选器</el-button>
        <el-button :icon="Connection" @click="openBindings">联动设置</el-button>
        <el-button type="primary" :loading="saving" :icon="Check" @click="saveAll">保存</el-button>
      </div>
    </div>

    <div class="body">
      <div class="canvas-wrap">
        <div class="filter-strip" v-if="draftFilters.length">
          <div v-for="f in draftFilters" :key="f._key" class="draft-filter">
            <el-tag closable @close="removeFilter(f)">
              <el-icon><Filter /></el-icon>
              {{ f.label || f.name }} · {{ filterTypeLabel(f.filterType) }}
            </el-tag>
          </div>
        </div>

        <el-empty v-if="!draftWidgets.length" description="暂无组件，点击右上角「添加组件」开始搭建" class="canvas-empty" />

        <GridCanvas v-else :layout="layout" :editable="true" @layout-updated="onLayoutUpdated">
          <template #default="{ item }">
            <WidgetFrame :widget="item.widget">
              <template #actions>
                <el-button link size="small" :icon="Setting" @click="editWidget(item.widget)" />
                <el-button link size="small" type="danger" :icon="Delete" @click="removeWidget(item.widget)" />
              </template>
              <ChartRenderer
                v-if="item.widget.widgetType === 'chart'"
                :type="item.widget.chartType || 'table'"
                :result="previewResults[item.widget._key]?.result"
                :config="item.widget.chartConfig"
              />
              <div v-else class="text-widget">{{ item.widget.text }}</div>
              <div v-if="previewResults[item.widget._key]?.error" class="widget-error">
                {{ previewResults[item.widget._key]?.error }}
              </div>
            </WidgetFrame>
          </template>
        </GridCanvas>
      </div>

      <div class="side">
        <el-tabs v-model="sideTab">
          <el-tab-pane label="组件" name="widgets">
            <div v-for="w in draftWidgets" :key="w._key" class="side-item" @click="editWidget(w)">
              <el-icon><Histogram v-if="w.widgetType === 'chart'" /><Document v-else /></el-icon>
              <span class="side-item-title">{{ w.title || '未命名组件' }}</span>
              <el-tag v-if="w.widgetType === 'text'" size="small" type="info">文本</el-tag>
            </div>
            <el-empty v-if="!draftWidgets.length" description="暂无组件" :image-size="60" />
          </el-tab-pane>
          <el-tab-pane label="筛选器" name="filters">
            <div v-for="f in draftFilters" :key="f._key" class="side-item" @click="editFilter(f)">
              <el-icon><Filter /></el-icon>
              <span class="side-item-title">{{ f.label || f.name }}</span>
              <el-button link type="danger" :icon="Delete" @click.stop="removeFilter(f)" />
            </div>
            <el-empty v-if="!draftFilters.length" description="暂无筛选器" :image-size="60" />
          </el-tab-pane>
          <el-tab-pane label="联动" name="bindings">
            <div v-if="!draftBindings.length" class="muted" style="padding: 8px">尚未配置筛选器联动</div>
            <div v-for="(b, i) in draftBindings" :key="i" class="binding-row">
              <span class="binding-text">
                {{ filterLabel(b.filterId) }} → {{ widgetLabel(b.widgetId) }} · {{ b.paramName }}
                <el-tag v-if="b.valueIndex === 1" size="small" type="warning">结束值</el-tag>
              </span>
              <el-button link type="danger" :icon="Delete" @click="draftBindings.splice(i, 1)" />
            </div>
            <el-button size="small" style="margin-top: 8px" @click="openBindings">编辑联动</el-button>
          </el-tab-pane>
        </el-tabs>
      </div>
    </div>

    <!-- Add/Edit widget -->
    <el-dialog v-model="widgetDialog" :title="editingWidget ? '编辑组件' : '添加组件'" width="760px" destroy-on-close>
      <el-form label-width="90px">
        <el-form-item label="组件类型">
          <el-radio-group v-model="widgetForm.widgetType">
            <el-radio-button value="chart">图表</el-radio-button>
            <el-radio-button value="text">文本</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="标题">
          <el-input v-model="widgetForm.title" placeholder="显示在组件顶部" />
        </el-form-item>
        <template v-if="widgetForm.widgetType === 'chart'">
          <el-form-item label="选择图表">
            <div style="width: 100%">
              <el-select v-model="widgetForm.chartId" style="width: 100%" filterable>
                <el-option v-for="c in charts" :key="c.id" :label="`${c.name} (${chartTypeLabel(c.type)})`" :value="c.id" />
              </el-select>
              <el-button style="margin-top: 8px" @click="openNewChart">新建图表</el-button>
            </div>
          </el-form-item>
        </template>
        <el-form-item v-else label="文本内容">
          <el-input v-model="widgetForm.text" type="textarea" :rows="6" placeholder="支持多行文本说明" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="widgetDialog = false">取消</el-button>
        <el-button type="primary" @click="applyWidget">确定</el-button>
      </template>
    </el-dialog>

    <ChartEditorDialog v-model="chartDialog" :chart="null" @saved="onChartCreated" />

    <!-- Add/Edit filter -->
    <el-dialog v-model="filterDialog" :title="editingFilter ? '编辑筛选器' : '添加筛选器'" width="680px" destroy-on-close>
      <el-form label-width="100px">
        <el-form-item label="名称">
          <el-input v-model="filterForm.name" placeholder="英文/下划线，例如 from_date" />
        </el-form-item>
        <el-form-item label="显示标签">
          <el-input v-model="filterForm.label" placeholder="例如 开始日期" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="filterForm.filterType" style="width: 200px" filterable>
            <el-option label="下拉选择" value="select" />
            <el-option label="单日期" value="date" />
            <el-option label="日期区间" value="daterange" />
            <el-option label="数字" value="number" />
            <el-option label="文本" value="text" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="filterForm.filterType === 'select'" label="允许多选">
          <el-switch v-model="filterForm.multiple" />
          <span class="muted" style="margin-left: 8px">多选值会绑定到数据集 list 参数（IN 查询）</span>
        </el-form-item>
        <template v-if="filterForm.filterType === 'select'">
          <el-form-item label="选项数据集">
            <el-select v-model="filterForm.datasetId" style="width: 100%" clearable filterable>
              <el-option v-for="d in datasets" :key="d.id" :label="d.name" :value="d.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="值列 / 显示列">
            <el-input v-model="filterForm.valueColumn" placeholder="值列，例如 region" style="width: 46%" />
            <el-input v-model="filterForm.labelColumn" placeholder="显示列（默认同值列）" style="width: 46%; margin-left: 8px" />
          </el-form-item>
        </template>
        <el-form-item label="默认值">
          <el-date-picker
            v-if="filterForm.filterType === 'date'"
            v-model="defaultScalar"
            type="date"
            value-format="YYYY-MM-DD"
            placeholder="默认日期"
          />
          <el-input v-else v-model="defaultScalar" placeholder="可选，例如 2026-01-01" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="filterDialog = false">取消</el-button>
        <el-button type="primary" @click="applyFilter">确定</el-button>
      </template>
    </el-dialog>

    <!-- Bindings -->
    <el-dialog v-model="bindingDialog" title="筛选器联动设置" width="820px" top="5vh" destroy-on-close>
      <el-alert type="info" :closable="false" style="margin-bottom: 12px">
        选择筛选器，勾选它要驱动的图表，并填写该图表数据集中的参数名。日期区间会同时提供开始/结束两个值。
      </el-alert>
      <el-table :data="bindingMatrix" size="small" border max-height="480">
        <el-table-column label="图表 \\ 筛选器" width="190" fixed>
          <template #default="{ row }">
            <span class="binding-widget">{{ row.widgetTitle }}</span>
          </template>
        </el-table-column>
        <el-table-column v-for="f in draftFilters" :key="f._key" :label="f.label || f.name" min-width="220">
          <template #default="{ row }">
            <div class="binding-cell">
              <el-checkbox v-model="row.cells[f._key].enabled">{{ row.cells[f._key].enabled ? '绑定' : '未绑定' }}</el-checkbox>
              <el-input
                v-if="row.cells[f._key].enabled"
                v-model="row.cells[f._key].paramName"
                size="small"
                placeholder="参数名，例如 from_date"
              />
              <el-checkbox
                v-if="row.cells[f._key].enabled && f.filterType === 'daterange'"
                v-model="row.cells[f._key].isEnd"
                size="small"
              >
                使用结束日期
              </el-checkbox>
            </div>
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="bindingDialog = false">取消</el-button>
        <el-button type="primary" @click="applyBindings">应用</el-button>
      </template>
    </el-dialog>

    <ShareDialog v-model="shareVisible" :dashboard="dashboardMeta" />

    <el-dialog v-model="previewVisible" title="预览" width="100%" fullscreen destroy-on-close>
      <div class="preview-modal-body">
        <iframe :src="previewSrc" style="width: 100%; height: 100%; border: none" />
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  ArrowLeft, Check, Connection, Delete, Document, Filter, Histogram, Plus, Setting, Share, View,
} from '@element-plus/icons-vue'
import {
  chartApi, dashboardApi, datasetApi,
} from '@/api'
import type { Chart, ChartConfig, Dashboard, Dataset, FilterBinding, ResultSet } from '@/types'
import GridCanvas from '@/components/dashboard/GridCanvas.vue'
import WidgetFrame from '@/components/dashboard/WidgetFrame.vue'
import ChartRenderer from '@/components/charts/ChartRenderer.vue'
import ChartEditorDialog from '@/components/charts/ChartEditorDialog.vue'
import ShareDialog from '@/components/dashboard/ShareDialog.vue'

const route = useRoute()
const router = useRouter()
const dashId = Number(route.params.id)

interface DraftWidget {
  _key: string
  id: number
  widgetType: 'chart' | 'text'
  chartId?: number
  title: string
  x: number
  y: number
  w: number
  h: number
  text?: string
  chartType?: string
  chartConfig?: ChartConfig | null
}

interface DraftFilter {
  _key: string
  id: number
  name: string
  label: string
  filterType: string
  datasetId?: number
  valueColumn: string
  labelColumn: string
  defaultValueJson: string
  multiple: boolean
  x: number
  y: number
  w: number
  h: number
}

interface DraftBinding {
  filterId: number
  widgetId: number
  paramName: string
  valueIndex: number
  _filterKey: string
  _widgetKey: string
}

const name = ref('')
const dashboardMeta = ref<Dashboard | null>(null)
const draftWidgets = ref<DraftWidget[]>([])
const draftFilters = ref<DraftFilter[]>([])
const draftBindings = ref<DraftBinding[]>([])
const charts = ref<Chart[]>([])
const datasets = ref<Dataset[]>([])
const previewResults = reactive<Record<string, { result?: ResultSet; error?: string }>>({})
const dirty = ref(false)
const saving = ref(false)
const sideTab = ref('widgets')

const widgetDialog = ref(false)
const editingWidget = ref<DraftWidget | null>(null)
const widgetForm = reactive({ widgetType: 'chart' as 'chart' | 'text', chartId: undefined as number | undefined, title: '', text: '' })
const chartDialog = ref(false)

const filterDialog = ref(false)
const editingFilter = ref<DraftFilter | null>(null)
const filterForm = reactive({
  name: '', label: '', filterType: 'text', datasetId: undefined as number | undefined,
  valueColumn: '', labelColumn: '', defaultValueJson: '', multiple: false,
})
const defaultScalar = ref('')

const bindingDialog = ref(false)
const bindingMatrix = ref<any[]>([])
const shareVisible = ref(false)
const previewVisible = ref(false)
const previewSrc = computed(() => `/dashboards/${dashId}/view`)

let keySeq = 1
function newKey() {
  return `n${Date.now()}_${keySeq++}`
}

const layout = computed(() =>
  draftWidgets.value.map((w) => ({
    i: w._key,
    x: w.x,
    y: w.y,
    w: w.w,
    h: w.h,
    widget: w,
  })),
)

function chartTypeLabel(t: string) {
  return { bar: '柱状图', line: '折线图', pie: '饼图', table: '表格', kpi: 'KPI' }[t] || t
}
function filterTypeLabel(t: string) {
  return { select: '下拉', date: '日期', daterange: '日期区间', number: '数字', text: '文本' }[t] || t
}
function filterLabel(id: number) {
  return draftFilters.value.find((f) => f.id === id)?.label || `筛选器#${id}`
}
function widgetLabel(id: number) {
  return draftWidgets.value.find((w) => w.id === id)?.title || `组件#${id}`
}

function markDirty() {
  dirty.value = true
}

function onLayoutUpdated(l: any[]) {
  for (const item of l) {
    const w = draftWidgets.value.find((x) => x._key === item.i)
    if (w) {
      w.x = item.x
      w.y = item.y
      w.w = item.w
      w.h = item.h
    }
  }
  markDirty()
}

async function loadCharts() {
  charts.value = await chartApi.list()
}

async function loadDatasets() {
  datasets.value = await datasetApi.list()
}

async function loadDashboard() {
  const res = await dashboardApi.get(dashId)
  dashboardMeta.value = res.dashboard
  name.value = res.dashboard.name
  await loadCharts()
  await loadDatasets()
  draftWidgets.value = (res.dashboard.widgets || []).map((w) => {
    let text = ''
    try {
      text = JSON.parse(w.optionsJson || '{}').text || ''
    } catch {}
    return {
      _key: `w${w.id}`,
      id: w.id,
      widgetType: (w.widgetType as any) || 'chart',
      chartId: w.chartId,
      title: w.title,
      x: w.x,
      y: w.y,
      w: w.w,
      h: w.h,
      text,
      chartType: 'table',
      chartConfig: null,
    }
  })
  draftFilters.value = (res.dashboard.filters || []).map((f) => ({
    _key: `f${f.id}`,
    id: f.id,
    name: f.name,
    label: f.label,
    filterType: f.filterType,
    datasetId: f.datasetId,
    valueColumn: f.valueColumn,
    labelColumn: f.labelColumn,
    defaultValueJson: f.defaultValueJson,
    multiple: !!f.multiple,
    x: f.x,
    y: f.y,
    w: f.w,
    h: f.h,
  }))
  const chartById = new Map(charts.value.map((c) => [c.id, c]))
  for (const w of draftWidgets.value) {
    if (w.chartId && chartById.has(w.chartId)) {
      const c = chartById.get(w.chartId)!
      w.chartType = c.type
      w.chartConfig = JSON.parse(c.configJson || '{}')
    }
  }
  draftBindings.value = (res.bindings || []).map((b) => ({
    filterId: b.filterId,
    widgetId: b.widgetId,
    paramName: b.paramName,
    valueIndex: b.valueIndex,
    _filterKey: `f${b.filterId}`,
    _widgetKey: `w${b.widgetId}`,
  }))
  dirty.value = false
  refreshPreviews()
}

function openAddWidget() {
  editingWidget.value = null
  widgetForm.widgetType = 'chart'
  widgetForm.chartId = undefined
  widgetForm.title = ''
  widgetForm.text = ''
  widgetDialog.value = true
  loadCharts()
}

function editWidget(w: DraftWidget) {
  editingWidget.value = w
  widgetForm.widgetType = w.widgetType
  widgetForm.chartId = w.chartId
  widgetForm.title = w.title
  widgetForm.text = w.text || ''
  widgetDialog.value = true
}

function applyWidget() {
  const w = editingWidget.value
  if (widgetForm.widgetType === 'chart' && !widgetForm.chartId) {
    ElMessage.warning('请选择图表')
    return
  }
  const chart = charts.value.find((c) => c.id === widgetForm.chartId)
  if (w) {
    w.widgetType = widgetForm.widgetType
    w.chartId = widgetForm.widgetType === 'chart' ? widgetForm.chartId : undefined
    w.title = widgetForm.title
    w.text = widgetForm.text
    if (chart) {
      w.chartType = chart.type
      w.chartConfig = JSON.parse(chart.configJson || '{}')
    }
  } else {
    const maxY = draftWidgets.value.reduce((m, x) => Math.max(m, x.y + x.h), 0)
    draftWidgets.value.push({
      _key: newKey(),
      id: 0,
      widgetType: widgetForm.widgetType,
      chartId: widgetForm.widgetType === 'chart' ? widgetForm.chartId : undefined,
      title: widgetForm.title || (chart?.name ?? ''),
      x: 0,
      y: maxY,
      w: 6,
      h: 8,
      text: widgetForm.text,
      chartType: chart?.type,
      chartConfig: chart ? JSON.parse(chart.configJson || '{}') : null,
    })
  }
  widgetDialog.value = false
  markDirty()
  refreshPreviews()
}

function removeWidget(w: DraftWidget) {
  draftWidgets.value = draftWidgets.value.filter((x) => x !== w)
  draftBindings.value = draftBindings.value.filter((b) => b._widgetKey !== w._key)
  delete previewResults[w._key]
  markDirty()
}

function openNewChart() {
  chartDialog.value = true
}

async function onChartCreated(chart: Chart) {
  await loadCharts()
  widgetForm.chartId = chart.id
  widgetForm.title = widgetForm.title || chart.name
}

function openAddFilter() {
  editingFilter.value = null
  Object.assign(filterForm, {
    name: '', label: '', filterType: 'text', datasetId: undefined,
    valueColumn: '', labelColumn: '', defaultValueJson: '', multiple: false,
  })
  defaultScalar.value = ''
  filterDialog.value = true
  loadDatasets()
}

function editFilter(f: DraftFilter) {
  editingFilter.value = f
  Object.assign(filterForm, {
    name: f.name,
    label: f.label,
    filterType: f.filterType,
    datasetId: f.datasetId,
    valueColumn: f.valueColumn,
    labelColumn: f.labelColumn,
    defaultValueJson: f.defaultValueJson,
    multiple: !!f.multiple,
  })
  try {
    const v = f.defaultValueJson ? JSON.parse(f.defaultValueJson) : ''
    defaultScalar.value = typeof v === 'string' ? v : ''
  } catch {
    defaultScalar.value = ''
  }
  filterDialog.value = true
}

function applyFilter() {
  if (!filterForm.name.trim()) {
    ElMessage.warning('请填写筛选器名称')
    return
  }
  if (!/^[a-zA-Z_][a-zA-Z0-9_]*$/.test(filterForm.name)) {
    ElMessage.warning('名称只能包含字母、数字、下划线')
    return
  }
  const dv = defaultScalar.value ? JSON.stringify(defaultScalar.value) : ''
  const f = editingFilter.value
  if (f) {
    Object.assign(f, { ...filterForm, defaultValueJson: dv })
  } else {
    draftFilters.value.push({
      _key: newKey(),
      id: 0,
      ...filterForm,
      defaultValueJson: dv,
      x: 0, y: 0, w: 3, h: 1,
    } as DraftFilter)
  }
  filterDialog.value = false
  markDirty()
}

function removeFilter(f: DraftFilter) {
  draftFilters.value = draftFilters.value.filter((x) => x !== f)
  draftBindings.value = draftBindings.value.filter((b) => b._filterKey !== f._key)
  markDirty()
}

function openBindings() {
  const rows = draftWidgets.value
    .filter((w) => w.widgetType === 'chart' && w.chartId)
    .map((w) => {
      const cells: Record<string, { enabled: boolean; paramName: string; isEnd: boolean }> = {}
      for (const f of draftFilters.value) {
        const existing = draftBindings.value.find((b) => b._widgetKey === w._key && b._filterKey === f._key)
        cells[f._key] = {
          enabled: !!existing,
          paramName: existing?.paramName || f.name,
          isEnd: existing?.valueIndex === 1,
        }
      }
      return { widgetKey: w._key, widgetTitle: w.title || '未命名组件', cells }
    })
  bindingMatrix.value = rows
  bindingDialog.value = true
}

function applyBindings() {
  const out: DraftBinding[] = []
  for (const row of bindingMatrix.value) {
    for (const f of draftFilters.value) {
      const cell = row.cells[f._key]
      if (!cell?.enabled || !cell.paramName.trim()) continue
      out.push({
        filterId: f.id,
        widgetId: draftWidgets.value.find((w) => w._key === row.widgetKey)?.id || 0,
        paramName: cell.paramName.trim(),
        valueIndex: f.filterType === 'daterange' && cell.isEnd ? 1 : 0,
        _filterKey: f._key,
        _widgetKey: row.widgetKey,
      })
    }
  }
  draftBindings.value = out
  bindingDialog.value = false
  markDirty()
}

async function saveAll() {
  saving.value = true
  try {
    if (name.value.trim() && name.value !== dashboardMeta.value?.name) {
      await dashboardApi.update(dashId, { name: name.value.trim(), description: dashboardMeta.value?.description || '' })
    }
    const saved = await dashboardApi.saveLayout(dashId, {
      widgets: draftWidgets.value.map((w) => ({
        id: w.id || undefined,
        widgetType: w.widgetType,
        chartId: w.chartId,
        title: w.title,
        x: w.x,
        y: w.y,
        w: w.w,
        h: w.h,
        optionsJson: JSON.stringify({ text: w.text || '' }),
      })),
      filters: draftFilters.value.map((f) => ({
        id: f.id || undefined,
        name: f.name,
        label: f.label,
        filterType: f.filterType,
        datasetId: f.datasetId,
        valueColumn: f.valueColumn,
        labelColumn: f.labelColumn,
        defaultValueJson: f.defaultValueJson,
        multiple: f.multiple,
        x: f.x,
        y: f.y,
        w: f.w,
        h: f.h,
      })),
    })
    dashboardMeta.value = saved
    // Re-map draft keys to persisted IDs by position (layout endpoint recreates rows).
    const widgets = saved.widgets || []
    const filters = saved.filters || []
    const keyToWidget = new Map<string, number>()
    const keyToFilter = new Map<string, number>()
    draftWidgets.value.forEach((w, i) => {
      w.id = widgets[i]?.id || 0
      if (w.id) keyToWidget.set(w._key, w.id)
    })
    draftFilters.value.forEach((f, i) => {
      f.id = filters[i]?.id || 0
      if (f.id) keyToFilter.set(f._key, f.id)
    })
    draftBindings.value = draftBindings.value.map((b) => ({
      ...b,
      filterId: keyToFilter.get(b._filterKey) || b.filterId,
      widgetId: keyToWidget.get(b._widgetKey) || b.widgetId,
    }))
    if (draftBindings.value.length) {
      await dashboardApi.saveBindings(
        dashId,
        draftBindings.value.map((b) => ({
          filterId: b.filterId,
          widgetId: b.widgetId,
          paramName: b.paramName,
          valueIndex: b.valueIndex,
        })),
      )
    }
    dirty.value = false
    ElMessage.success('已保存')
    await refreshPreviews()
  } finally {
    saving.value = false
  }
}

/** Preview widget data with default filter values for in-editor rendering. */
async function refreshPreviews() {
  const values: Record<string, any> = {}
  for (const f of draftFilters.value) {
    if (f.defaultValueJson) {
      try {
        values[f.name] = JSON.parse(f.defaultValueJson)
      } catch {}
    }
  }
  for (const w of draftWidgets.value) {
    if (w.widgetType !== 'chart' || !w.chartId) continue
    try {
      previewResults[w._key] = { result: await chartApi.data(w.chartId) }
    } catch (e: any) {
      previewResults[w._key] = { error: '图表取数失败，请检查数据集配置' }
    }
  }
}

onMounted(async () => {
  try {
    await loadDashboard()
  } catch {
    ElMessage.error('无法加载仪表盘')
    router.push('/dashboards')
  }
})
</script>

<style scoped>
.editor-page {
  height: calc(100dvh - 92px);
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #fff;
  border-radius: 8px;
  padding: 10px 14px;
  flex-wrap: wrap;
  gap: 8px;
}
.name-input {
  width: 260px;
}
@media (max-width: 640px) {
  .name-input {
    width: 100%;
  }
  .toolbar {
    padding: 8px 10px;
  }
}
.body {
  flex: 1;
  display: flex;
  gap: 12px;
  min-height: 0;
}
@media (max-width: 900px) {
  .editor-page {
    height: auto;
    min-height: calc(100dvh - 92px);
  }
  .body {
    flex-direction: column;
  }
  .side {
    width: 100%;
    max-height: 320px;
  }
  .canvas-wrap {
    min-height: 420px;
  }
}
.canvas-wrap {
  flex: 1;
  background: #f0f2f5;
  border-radius: 8px;
  padding: 12px;
  overflow-y: auto;
  min-width: 0;
}
.filter-strip {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.canvas-empty {
  margin-top: 80px;
}
.side {
  width: 300px;
  background: #fff;
  border-radius: 8px;
  padding: 10px 12px;
  overflow-y: auto;
}
.side-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 6px;
  border-radius: 6px;
  cursor: pointer;
}
.side-item:hover {
  background: #f5f7fa;
}
.side-item-title {
  flex: 1;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.binding-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 4px;
  border-bottom: 1px dashed #ebeef5;
  font-size: 12px;
}
.binding-text {
  flex: 1;
}
.binding-widget {
  font-size: 12px;
}
.binding-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.text-widget {
  white-space: pre-wrap;
  color: #303133;
  font-size: 14px;
  line-height: 1.7;
  height: 100%;
  overflow: auto;
}
.widget-error {
  color: #f56c6c;
  font-size: 12px;
  padding: 4px 6px;
}
.preview-modal-body {
  height: calc(100vh - 120px);
}
</style>
