<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { BarChart3, Database, Eye, Pencil, Play, Plus, Search, ShieldCheck, Trash2 } from 'lucide-vue-next'
import * as echarts from 'echarts'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, getErrorMessage } from '../../api/client'
import { dateRange, formatDate, formatValue } from '../../utils/format'
import NumText from '../../components/NumText.vue'
import EmptyBlock from '../../components/EmptyBlock.vue'

const router = useRouter()

const datasets = ref([])
const datasources = ref([])
const loading = ref(true)
const saving = ref(false)
const keyword = ref('')

const dialogVisible = ref(false)
const editingId = ref(null)
const form = reactive({
  name: '',
  code: '',
  description: '',
  sql_text: '',
  scope_column: 'org_id',
  require_scope: true,
  datasource_id: null,
  status: 'active',
})

const previewVisible = ref(false)
const previewLoading = ref(false)
const previewDataset = ref(null)
const previewDates = ref(dateRange(7))
const previewData = ref(null)
const previewError = ref('')

// 新建 / 编辑数据模型时的 SQL 草稿即时预览
const sqlPreview = reactive({
  dates: dateRange(7),
  loading: false,
  data: null,
  error: '',
  view: 'table',
  dimensionIndex: -1,
  metricIndex: -1,
})
const sqlPreviewEl = ref(null)
let sqlPreviewChart = null

const dashboardVisible = ref(false)
const dashboardSaving = ref(false)
const dashboardDataset = ref(null)
const dashboardFields = ref([])
const dashboardForm = reactive({
  name: '',
  title: '',
  chart_type: 'bar',
  x_field: '',
  y_field: '',
  unit: '元',
  publish: true,
})

const filtered = computed(() => {
  const text = keyword.value.trim().toLowerCase()
  if (!text) return datasets.value
  return datasets.value.filter(
    (item) =>
      (item.name || '').toLowerCase().includes(text) ||
      (item.code || '').toLowerCase().includes(text) ||
      (item.description || '').toLowerCase().includes(text),
  )
})

const scopedCount = computed(() => datasets.value.filter((item) => item.require_scope).length)
const disabledCount = computed(() => datasets.value.filter((item) => item.status !== 'active').length)
const datasourceMap = computed(() => Object.fromEntries(datasources.value.map((item) => [item.id, item])))

const defaultSQL = `SELECT org_id,
       SUM(revenue) AS revenue,
       SUM(orders) AS orders,
       SUM(gross_profit) AS gross_profit
FROM sales_facts
WHERE stat_date BETWEEN {{start_date}} AND {{end_date}}
  AND org_id IN ({{org_scope}})
GROUP BY org_id
ORDER BY revenue DESC`

function resetForm() {
  editingId.value = null
  Object.assign(form, {
    name: '',
    code: '',
    description: '',
    sql_text: defaultSQL,
    scope_column: 'org_id',
    require_scope: true,
    datasource_id: null,
    status: 'active',
  })
}

function resetSqlPreview() {
  sqlPreview.dates = dateRange(7)
  sqlPreview.loading = false
  sqlPreview.data = null
  sqlPreview.error = ''
  sqlPreview.view = 'table'
  sqlPreview.dimensionIndex = -1
  sqlPreview.metricIndex = -1
  disposeSqlPreviewChart()
}

function disposeSqlPreviewChart() {
  if (sqlPreviewChart) {
    sqlPreviewChart.dispose()
    sqlPreviewChart = null
  }
}

async function runSqlPreview() {
  if (!form.sql_text.trim()) {
    ElMessage.warning('请先填写 SQL 查询')
    return
  }
  if (form.require_scope && !form.sql_text.includes('{{org_scope}}')) {
    ElMessage.warning('启用数据权限时，SQL 必须包含 {{org_scope}} 占位符')
    return
  }
  sqlPreview.loading = true
  sqlPreview.error = ''
  sqlPreview.data = null
  try {
    sqlPreview.data = await api.post('/datasets/preview', {
      sql_text: form.sql_text,
      require_scope: form.require_scope,
      scope_column: form.scope_column || 'org_id',
      datasource_id: form.datasource_id || null,
      start_date: sqlPreview.dates?.[0] || dateRange(7)[0],
      end_date: sqlPreview.dates?.[1] || dateRange(7)[1],
    })
    const generated = syncSqlPreviewFields()
    ElMessage.success(
      generated
        ? `查询成功，共 ${sqlPreview.data?.count || 0} 行，已自动生成图表`
        : `查询成功，共 ${sqlPreview.data?.count || 0} 行`,
    )
  } catch (error) {
    sqlPreview.error = getErrorMessage(error)
  } finally {
    sqlPreview.loading = false
  }
  await nextTick()
  renderSqlPreviewChart()
}

const sqlPreviewColumns = computed(() => sqlPreview.data?.columns || [])
const sqlPreviewRows = computed(() => sqlPreview.data?.rows || [])
const sqlPreviewFieldInfo = computed(() => inferSqlPreviewFields())

function isNumericCell(value) {
  return typeof value === 'number' && Number.isFinite(value)
}

// 自动识别字段：文本列优先作为维度，数值列作为指标。
// 如果 SQL 第一列是 org_id 这类数字维度，则不会把它误当成销售额。
function inferSqlPreviewFields() {
  const columns = sqlPreviewColumns.value
  const rows = sqlPreviewRows.value
  if (!columns.length || !rows.length) {
    return { dimensions: [], metrics: [], dimensionIndex: -1, metricIndex: -1 }
  }
  const dimensions = []
  const numeric = []
  columns.forEach((label, index) => {
    if (rows.some((row) => isNumericCell(row[index]))) {
      numeric.push({ index, label })
    } else {
      dimensions.push({ index, label })
    }
  })
  if (!dimensions.length) dimensions.push({ index: 0, label: columns[0] })

  const dimensionIndex = dimensions[0].index
  let metrics = numeric.filter((item) => item.index !== dimensionIndex)
  // 单指标查询没有独立维度时，仍允许展示 KPI / 单柱效果。
  if (!metrics.length) metrics = numeric

  const preferred = /(revenue|sales|amount|profit|gmv|order|count|qty|total|销售|金额|毛利|订单|数量|目标|完成)/i
  const preferredMetric = metrics.find((item) => preferred.test(item.label))
  return {
    dimensions,
    metrics,
    dimensionIndex,
    metricIndex: preferredMetric?.index ?? metrics[0]?.index ?? -1,
  }
}

function syncSqlPreviewFields() {
  const fields = inferSqlPreviewFields()
  sqlPreview.dimensionIndex = fields.dimensionIndex
  sqlPreview.metricIndex = fields.metricIndex
  if (fields.metricIndex >= 0) {
    sqlPreview.view = 'bar'
    return true
  }
  sqlPreview.view = 'table'
  return false
}

function renderSqlPreviewChart() {
  if (sqlPreview.view === 'table' || !sqlPreviewEl.value || !sqlPreviewRows.value.length) {
    disposeSqlPreviewChart()
    return
  }
  const metricIndex = sqlPreview.metricIndex
  const dimensionIndex = sqlPreview.dimensionIndex
  if (metricIndex < 0 || dimensionIndex < 0) {
    disposeSqlPreviewChart()
    return
  }
  if (sqlPreviewChart && sqlPreviewChart.getDom() !== sqlPreviewEl.value) {
    disposeSqlPreviewChart()
  }
  if (!sqlPreviewChart) sqlPreviewChart = echarts.init(sqlPreviewEl.value)
  const rows = sqlPreviewRows.value.slice(0, 30)
  const labels = rows.map((row) => String(row[dimensionIndex] ?? '—'))
  const values = rows.map((row) => Number(row[metricIndex]) || 0)
  const metricLabel = sqlPreviewColumns.value[metricIndex] || '指标'
  const tooltip = {
    backgroundColor: 'rgba(16,42,35,.94)',
    borderWidth: 0,
    textStyle: { color: '#eafff5', fontSize: 11 },
  }
  const common = {
    animationDuration: 350,
    tooltip,
  }
  let option
  if (sqlPreview.view === 'donut') {
    option = {
      ...common,
      tooltip: {
        ...tooltip,
        trigger: 'item',
        formatter: (item) => `${item.name}<br/>${metricLabel}：${formatValue(item.value, 'number')}`,
      },
      legend: {
        bottom: 0,
        type: 'scroll',
        textStyle: { color: '#718078', fontSize: 9 },
      },
      series: [
        {
          name: metricLabel,
          type: 'pie',
          radius: ['42%', '68%'],
          center: ['50%', '44%'],
          itemStyle: { borderRadius: 4, borderColor: '#fff', borderWidth: 2 },
          label: { color: '#53635b', fontSize: 9 },
          data: labels.map((name, index) => ({ name, value: values[index] })),
        },
      ],
    }
  } else {
    option = {
      ...common,
      grid: { left: 8, right: 18, top: 24, bottom: 6, containLabel: true },
      tooltip: {
        ...tooltip,
        trigger: 'axis',
        formatter: (items) => {
          const item = Array.isArray(items) ? items[0] : items
          if (!item) return ''
          return `${item.name}<br/>${metricLabel}：${formatValue(item.value, 'number')}`
        },
      },
      xAxis: {
        type: 'category',
        data: labels,
        axisTick: { show: false },
        axisLine: { lineStyle: { color: '#dfe7e0' } },
        axisLabel: { color: '#94a39b', fontSize: 9, hideOverlap: true },
      },
      yAxis: {
        type: 'value',
        splitLine: { lineStyle: { color: '#edf1ed', type: 'dashed' } },
        axisLabel: {
          color: '#94a39b',
          fontSize: 9,
          formatter: (value) => (Math.abs(value) >= 10000 ? `${(value / 10000).toFixed(1)}万` : value),
        },
      },
      series: [
        {
          name: metricLabel,
          type: sqlPreview.view,
          smooth: 0.3,
          barMaxWidth: 22,
          itemStyle:
            sqlPreview.view === 'bar'
              ? { color: '#14B8A6', borderRadius: [4, 4, 0, 0] }
              : { color: '#0F766E' },
          areaStyle: sqlPreview.view === 'line' ? { opacity: 0.14, color: '#14B8A6' } : undefined,
          data: values,
        },
      ],
    }
  }
  sqlPreviewChart.setOption(option, true)
  sqlPreviewChart.resize()
}

function switchSqlPreviewView(view) {
  sqlPreview.view = view
  nextTick(renderSqlPreviewChart)
}

function changeSqlPreviewField() {
  nextTick(renderSqlPreviewChart)
}

function openCreate() {
  resetForm()
  resetSqlPreview()
  dialogVisible.value = true
}

function openEdit(row) {
  resetForm()
  resetSqlPreview()
  editingId.value = row.id
  Object.assign(form, {
    name: row.name || '',
    code: row.code || '',
    description: row.description || '',
    sql_text: row.sql_text || '',
    scope_column: row.scope_column || 'org_id',
    require_scope: row.require_scope !== false,
    datasource_id: row.datasource_id || null,
    status: row.status || 'active',
  })
  dialogVisible.value = true
}

async function load() {
  loading.value = true
  try {
    const [list, sources] = await Promise.all([
      api.get('/datasets'),
      api.get('/admin/datasources').catch(() => []),
    ])
    datasets.value = list || []
    datasources.value = sources || []
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!form.name.trim() || !form.code.trim()) {
    ElMessage.warning('请填写数据集名称和编码')
    return
  }
  if (!form.sql_text.trim()) {
    ElMessage.warning('请填写 SQL 查询')
    return
  }
  if (form.require_scope && !form.sql_text.includes('{{org_scope}}')) {
    ElMessage.warning('启用数据权限时，SQL 必须包含 {{org_scope}} 占位符')
    return
  }
  saving.value = true
  try {
    const payload = {
      name: form.name.trim(),
      code: form.code.trim(),
      description: form.description.trim(),
      sql_text: form.sql_text,
      scope_column: form.scope_column.trim() || 'org_id',
      require_scope: form.require_scope,
      datasource_id: form.datasource_id || null,
      status: form.status,
    }
    if (editingId.value) {
      await api.put(`/datasets/${editingId.value}`, payload)
      ElMessage.success('数据集已更新')
    } else {
      await api.post('/datasets', payload)
      ElMessage.success('数据集已创建')
    }
    dialogVisible.value = false
    await load()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    saving.value = false
  }
}

async function remove(row) {
  try {
    await ElMessageBox.confirm(
      `确认删除数据集「${row.name}」？引用该数据集的看板组件将无法查询数据。`,
      '删除数据集',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
    await api.delete(`/datasets/${row.id}`)
    ElMessage.success('数据集已删除')
    await load()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(getErrorMessage(error))
  }
}

function openPreview(row) {
  previewDataset.value = row
  previewDates.value = dateRange(7)
  previewData.value = null
  previewError.value = ''
  previewVisible.value = true
  loadPreview()
}

async function loadPreview() {
  if (!previewDataset.value) return
  if (!previewDates.value?.[0]) {
    ElMessage.warning('请选择预览日期范围')
    return
  }
  previewLoading.value = true
  previewError.value = ''
  try {
    const query = new URLSearchParams({
      start_date: previewDates.value[0],
      end_date: previewDates.value[1],
    })
    previewData.value = await api.get(
      `/datasets/${previewDataset.value.id}/preview?${query.toString()}`,
    )
  } catch (error) {
    previewError.value = getErrorMessage(error)
    previewData.value = null
  } finally {
    previewLoading.value = false
  }
}

function cellText(value) {
  if (value === null || value === undefined || value === '') return '—'
  if (typeof value === 'number') return formatValue(value, 'number')
  return String(value)
}

function datasourceLabel(id) {
  if (!id) return '平台主库'
  return datasourceMap.value[id]?.name || `数据源 #${id}`
}

async function openDashboard(row) {
  dashboardDataset.value = row
  dashboardFields.value = []
  Object.assign(dashboardForm, {
    name: `${row.name}看板`,
    title: row.name,
    chart_type: 'bar',
    x_field: '',
    y_field: '',
    unit: '元',
    publish: true,
  })
  dashboardVisible.value = true
  try {
    const [start, end] = dateRange(30)
    const query = new URLSearchParams({ start_date: start, end_date: end })
    const result = await api.get(`/datasets/${row.id}/preview?${query.toString()}`)
    dashboardFields.value = result?.columns || []
    if (dashboardFields.value.length) {
      dashboardForm.x_field = dashboardFields.value[0]
      const numericIndex = dashboardFields.value.findIndex((_, index) => index > 0)
      dashboardForm.y_field = dashboardFields.value[numericIndex >= 0 ? numericIndex : 0]
    }
  } catch (error) {
    ElMessage.warning(`字段加载失败：${getErrorMessage(error)}`)
  }
}

async function createDashboard() {
  if (!dashboardDataset.value) return
  if (!dashboardForm.name.trim()) {
    ElMessage.warning('请填写看板名称')
    return
  }
  if (dashboardForm.chart_type !== 'table' && (!dashboardForm.x_field || !dashboardForm.y_field)) {
    ElMessage.warning('请选择分类字段和数值字段')
    return
  }
  dashboardSaving.value = true
  try {
    const payload = {
      name: dashboardForm.name.trim(),
      title: dashboardForm.title.trim(),
      chart_type: dashboardForm.chart_type,
      x_field: dashboardForm.x_field,
      y_field: dashboardForm.y_field,
      unit: dashboardForm.unit,
      publish: dashboardForm.publish,
      metrics: dashboardFields.value.slice(0, 4).map((field, index) => ({
        field,
        label: field,
        format: index === 0 ? 'text' : /revenue|profit|amount|sales/i.test(field) ? 'currency' : 'number',
        unit: index === 0 ? '' : dashboardForm.unit,
      })),
    }
    const created = await api.post(`/datasets/${dashboardDataset.value.id}/to-dashboard`, payload)
    ElMessage.success('看板已生成')
    dashboardVisible.value = false
    if (created?.id) router.push(`/dashboards/${created.id}/edit`)
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    dashboardSaving.value = false
  }
}

onMounted(load)
onBeforeUnmount(disposeSqlPreviewChart)
</script>

<template>
  <div>
    <div class="metric-mini-grid">
      <div class="metric-mini">
        <span>数据集</span>
        <strong>{{ datasets.length }}</strong>
      </div>
      <div class="metric-mini">
        <span>启用数据权限</span>
        <strong>{{ scopedCount }}</strong>
      </div>
      <div class="metric-mini">
        <span>启用中</span>
        <strong>{{ datasets.length - disabledCount }}</strong>
      </div>
      <div class="metric-mini">
        <span>已停用</span>
        <strong>{{ disabledCount }}</strong>
      </div>
    </div>

    <div class="admin-layout">
      <div class="admin-main">
        <div class="panel">
          <div class="table-toolbar">
            <div class="table-toolbar-left">
              <div class="search-box">
                <Search :size="14" />
                <input v-model="keyword" type="search" placeholder="搜索数据集名称或编码" />
              </div>
              <span class="tag">{{ filtered.length }} 个数据集</span>
            </div>
            <button class="btn btn-primary btn-sm" type="button" @click="openCreate">
              <Plus :size="14" /> 新建数据集
            </button>
          </div>

          <div v-if="loading" style="padding: 20px">
            <div class="skeleton" style="height: 320px" />
          </div>

          <div v-else-if="filtered.length" class="dataset-grid" style="padding: 18px">
            <article v-for="item in filtered" :key="item.id" class="dataset-card">
              <div class="dataset-card-head">
                <div class="dataset-icon"><Database :size="17" /></div>
                <span class="tag" :class="item.status === 'active' ? 'tag-green' : 'tag-red'">
                  {{ item.status === 'active' ? '启用' : '停用' }}
                </span>
              </div>
              <h3>{{ item.name }}</h3>
              <p>{{ item.description || '未填写描述' }}</p>
              <div>
                <span class="dataset-code mono">{{ item.code }}</span>
                <span class="tag" style="margin-left: 6px">{{ datasourceLabel(item.datasource_id) }}</span>
                <span v-if="item.require_scope" class="tag tag-green" style="margin-left: 6px">
                  <ShieldCheck :size="10" /> 组织权限
                </span>
                <span v-else class="tag tag-amber" style="margin-left: 6px">无数据权限</span>
              </div>
              <div class="dataset-foot">
                <span>范围字段 {{ item.scope_column || 'org_id' }} · 更新 {{ formatDate(item.updated_at) }}</span>
                <div class="inline-actions">
                  <button class="btn btn-ghost btn-sm" type="button" @click="openDashboard(item)">
                    <BarChart3 :size="13" /> 生成看板
                  </button>
                  <button class="btn btn-ghost btn-sm" type="button" @click="openPreview(item)">
                    <Eye :size="13" /> 预览
                  </button>
                  <button class="btn btn-ghost btn-sm" type="button" @click="openEdit(item)">
                    <Pencil :size="13" />
                  </button>
                  <button class="btn btn-ghost btn-sm" type="button" style="color: var(--rose)" @click="remove(item)">
                    <Trash2 :size="13" />
                  </button>
                </div>
              </div>
            </article>
          </div>

          <div v-else class="panel" style="border: 0">
            <EmptyBlock
              title="还没有数据模型"
              :description="datasets.length ? '换一个关键词试试。' : '创建受控 SQL 数据集后，看板组件即可引用它。'"
            >
              <button class="btn btn-primary btn-sm" type="button" @click="openCreate">
                <Plus :size="14" /> 新建数据集
              </button>
            </EmptyBlock>
          </div>
        </div>
      </div>

      <aside class="admin-aside">
        <div class="scope-card">
          <h3><ShieldCheck :size="13" style="vertical-align: -2px" /> SQL 安全规则</h3>
          <p>
            仅允许单条 SELECT / WITH 查询，禁止注释、多语句与写操作关键字；
            单次最多返回 2000 行，查询超时 10 秒，时间跨度不超过 366 天。
          </p>
          <div class="sql-hints">
            <span class="sql-hint" v-text="'{{org_scope}}'"></span>
            <span class="sql-hint" v-text="'{{start_date}}'"></span>
            <span class="sql-hint" v-text="'{{end_date}}'"></span>
            <span class="sql-hint" v-text="'{{filter:channel}}'"></span>
            <span class="sql-hint" v-text="'{{filter_in:channel}}'"></span>
            <span class="sql-hint" v-text="'{{filter_has:channel}}'"></span>
          </div>
          <p style="margin-top: 8px">
            看板全局筛选器通过 <span class="mono" v-text="'{{filter:字段名}}'"></span> 注入，
            服务端会转义为字符串字面量。空值表示“全部”，例如
            <span class="mono" v-text="'AND ({{filter:channel}} = \'\' OR channel = {{filter:channel}})'"></span>。
            多选筛选器使用 <span class="mono" v-text="'{{filter_in:字段名}}'"></span> 展开为 ('a','b') ，配合 <span class="mono" v-text="'{{filter_has:字段名}}'"></span> （已选择=1，未选择=0）判断“全部”，例如
            <span class="mono" v-text="'AND ({{filter_has:channel}} = 0 OR channel IN ({{filter_in:channel}}))'"></span>。
          </p>
        </div>
        <div class="scope-card">
          <h3><Database :size="13" style="vertical-align: -2px" /> 生产建议</h3>
          <p>
            为 BI 配置只读数据库账号，只授权业务明细视图。不要在 SQL 中拼接外部参数，
            统一使用平台占位符，由服务端渲染组织范围。
          </p>
        </div>
        <div class="scope-card">
          <h3><Database :size="13" style="vertical-align: -2px" /> 多数据库方言</h3>
          <p>
            数据集绑定数据源后会自动按方言限制行数：MySQL / PostgreSQL / SQLite 使用 LIMIT，
            SQL Server 使用 TOP，Oracle 使用 ROWNUM，无需手写分页。
          </p>
          <p style="margin-top: 6px">
            Oracle 中日期字段比较请显式转换，例如
            <span class="mono" v-text="'TO_DATE(\'{{start_date}}\', \'YYYY-MM-DD\')'"></span>，
            避免依赖会话 NLS 设置。
          </p>
        </div>
      </aside>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="editingId ? '编辑数据模型' : '新建数据模型'"
      width="860px"
      destroy-on-close
      @closed="resetSqlPreview"
    >
      <div class="form-grid">
        <label class="field-label">
          数据集名称
          <el-input v-model="form.name" maxlength="100" placeholder="例如：销售核心指标" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          编码
          <el-input v-model="form.code" maxlength="64" placeholder="例如：sales_summary" style="margin-top: 8px" />
        </label>
        <label class="field-label form-span-2">
          描述
          <el-input v-model="form.description" maxlength="500" placeholder="说明指标口径与使用场景" style="margin-top: 8px" />
        </label>
        <label class="field-label form-span-2">
          SQL 查询
          <textarea
            v-model="form.sql_text"
            class="sql-editor"
            spellcheck="false"
            placeholder="SELECT org_id, SUM(revenue) AS revenue ..."
          />
        </label>
        <div class="sql-hints form-span-2" style="margin-top: -4px">
          <span class="sql-hint" v-text="'{{org_scope}}'"></span>
          <span class="sql-hint" v-text="'{{start_date}}'"></span>
          <span class="sql-hint" v-text="'{{end_date}}'"></span>
          <span class="sql-hint" v-text="'{{filter:channel}}'"></span>
          <span class="sql-hint" v-text="'{{filter_in:channel}}'"></span>
            <span class="sql-hint" v-text="'{{filter_has:channel}}'"></span>
          <span style="color: var(--muted-light); font-size: 8.5px; align-self: center">
            启用数据权限时必须包含 <span v-text="'{{org_scope}}'"></span>
          </span>
        </div>
        <div class="form-span-2 sql-preview-block">
          <div class="table-toolbar" style="padding: 0; margin: 0">
            <div class="table-toolbar-left">
              <el-date-picker
                v-model="sqlPreview.dates"
                type="daterange"
                value-format="YYYY-MM-DD"
                range-separator="至"
                start-placeholder="开始日期"
                end-placeholder="结束日期"
                size="small"
                style="width: 232px"
              />
              <button class="btn btn-secondary btn-sm" type="button" :disabled="sqlPreview.loading" @click="runSqlPreview">
                <Play :size="13" /> {{ sqlPreview.loading ? '查询中…' : '查询并生成' }}
              </button>
              <span v-if="sqlPreview.data" class="tag">
                {{ sqlPreview.data.count }} 行 · {{ sqlPreviewColumns.length }} 列
                <template v-if="sqlPreview.data.elapsed_ms != null"> · {{ sqlPreview.data.elapsed_ms }}ms</template>
              </span>
            </div>
            <div v-if="sqlPreview.data?.count" class="date-range">
              <button :class="{ active: sqlPreview.view === 'table' }" type="button" @click="switchSqlPreviewView('table')">表格</button>
              <button :class="{ active: sqlPreview.view === 'bar' }" type="button" @click="switchSqlPreviewView('bar')">柱状</button>
              <button :class="{ active: sqlPreview.view === 'line' }" type="button" @click="switchSqlPreviewView('line')">折线</button>
              <button :class="{ active: sqlPreview.view === 'donut' }" type="button" @click="switchSqlPreviewView('donut')">环形</button>
            </div>
          </div>
          <div
            v-if="sqlPreview.data?.count && sqlPreviewFieldInfo.metrics.length"
            class="sql-preview-config"
          >
            <span class="sql-preview-config-label">维度</span>
            <el-select
              v-model="sqlPreview.dimensionIndex"
              size="small"
              class="sql-preview-config-select"
              @change="changeSqlPreviewField"
            >
              <el-option
                v-for="field in sqlPreviewFieldInfo.dimensions"
                :key="field.index"
                :value="field.index"
                :label="field.label"
              />
            </el-select>
            <span class="sql-preview-config-label">指标</span>
            <el-select
              v-model="sqlPreview.metricIndex"
              size="small"
              class="sql-preview-config-select"
              @change="changeSqlPreviewField"
            >
              <el-option
                v-for="field in sqlPreviewFieldInfo.metrics"
                :key="field.index"
                :value="field.index"
                :label="field.label"
              />
            </el-select>
            <span class="sql-preview-auto-tip">
              已自动识别维度和数值指标，也可手动切换
            </span>
          </div>
          <div v-if="sqlPreview.error" class="chart-error" style="margin-top: 10px">{{ sqlPreview.error }}</div>
          <div v-else-if="sqlPreview.loading" class="skeleton" style="height: 220px; margin-top: 10px" />
          <template v-else-if="sqlPreview.data">
            <div v-if="sqlPreview.view === 'table'" class="preview-table sql-preview-table">
              <table class="data-table">
                <thead>
                  <tr>
                    <th style="width: 42px">#</th>
                    <th v-for="column in sqlPreviewColumns" :key="column">{{ column }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(row, rowIndex) in sqlPreviewRows" :key="rowIndex">
                    <td><span class="rank-index">{{ rowIndex + 1 }}</span></td>
                    <td v-for="(value, columnIndex) in row" :key="columnIndex">
                      <NumText v-if="typeof value === 'number'" :value="value" format="number" />
                      <template v-else>{{ cellText(value) }}</template>
                    </td>
                  </tr>
                  <tr v-if="!sqlPreviewRows.length">
                    <td :colspan="sqlPreviewColumns.length + 1" style="text-align: center; color: var(--muted)">
                      当前条件下没有数据
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-show="sqlPreview.view !== 'table'" ref="sqlPreviewEl" class="sql-preview-chart" />
            <p v-if="sqlPreview.view !== 'table' && sqlPreview.metricIndex < 0" class="sql-preview-tip">
              结果中没有可识别的数值列，暂时无法生成图表，可切换回表格查看。
            </p>
          </template>
          <div v-else class="empty-state" style="padding: 22px">
            选择日期范围后点击「查询并生成」，将直接执行 SQL 并自动生成表格与图表效果。
          </div>
        </div>
        <label class="field-label">
          组织范围字段
          <el-input v-model="form.scope_column" maxlength="64" placeholder="org_id" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          数据源
          <el-select v-model="form.datasource_id" clearable placeholder="平台主库" style="width: 100%; margin-top: 8px">
            <el-option :value="null" label="平台主库（默认）" />
            <el-option
              v-for="source in datasources"
              :key="source.id"
              :value="source.id"
              :label="`${source.name} · ${source.driver}`"
            />
          </el-select>
        </label>
        <div class="field-label">
          数据权限
          <div class="scope-note" style="margin-top: 8px; width: 100%">
            <el-switch v-model="form.require_scope" />
            <span>{{ form.require_scope ? '查询时自动注入组织范围' : '不限制组织范围（谨慎使用）' }}</span>
          </div>
        </div>
        <label class="field-label">
          状态
          <el-select v-model="form.status" style="width: 100%; margin-top: 8px">
            <el-option value="active" label="启用" />
            <el-option value="disabled" label="停用" />
          </el-select>
        </label>
      </div>
      <template #footer>
        <div class="inline-actions" style="justify-content: flex-end">
          <button class="btn btn-secondary" type="button" @click="dialogVisible = false">取消</button>
          <button class="btn btn-primary" type="button" :disabled="saving" @click="save">
            {{ saving ? '保存中…' : '保存' }}
          </button>
        </div>
      </template>
    </el-dialog>

    <el-dialog v-model="previewVisible" width="900px" destroy-on-close>
      <template #header>
        <div>
          <div style="font-family: var(--font-display); font-size: 20px; font-weight: 600">
            数据预览 · {{ previewDataset?.name }}
          </div>
          <div style="margin-top: 4px; color: var(--muted); font-size: 9px">
            预览结果受当前管理员账号的组织范围限制，最多 2000 行。
          </div>
        </div>
      </template>
      <div class="inline-actions" style="margin-bottom: 13px">
        <el-date-picker
          v-model="previewDates"
          type="daterange"
          value-format="YYYY-MM-DD"
          range-separator="至"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          size="small"
          style="width: 232px"
        />
        <button class="btn btn-secondary btn-sm" type="button" :disabled="previewLoading" @click="loadPreview">
          {{ previewLoading ? '查询中…' : '执行预览' }}
        </button>
        <span v-if="previewData" class="tag">
          {{ previewData.count }} 行<template v-if="previewData.elapsed_ms != null"> · {{ previewData.elapsed_ms }}ms</template>
        </span>
      </div>

      <div v-if="previewError" class="chart-error" style="margin-top: 0">{{ previewError }}</div>
      <div v-else-if="previewLoading" class="skeleton" style="height: 260px" />
      <div v-else-if="previewData" class="preview-table">
        <table class="data-table">
          <thead>
            <tr>
              <th style="width: 42px">#</th>
              <th v-for="column in previewData.columns" :key="column">{{ column }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, rowIndex) in previewData.rows" :key="rowIndex">
              <td><span class="rank-index">{{ rowIndex + 1 }}</span></td>
          <td v-for="(value, columnIndex) in row" :key="columnIndex">
            <NumText v-if="typeof value === 'number'" :value="value" format="number" />
            <template v-else>{{ cellText(value) }}</template>
          </td>
            </tr>
            <tr v-if="!previewData.rows?.length">
              <td :colspan="(previewData.columns?.length || 0) + 1" style="text-align: center; color: var(--muted)">
                当前条件下没有数据
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="empty-state" style="padding: 30px">选择日期范围后执行预览。</div>
    </el-dialog>

    <el-dialog v-model="dashboardVisible" width="620px" destroy-on-close>
      <template #header>
        <div>
          <div style="font-family: var(--font-display); font-size: 20px; font-weight: 600">
            一键生成看板
          </div>
          <div style="margin-top: 4px; color: var(--muted); font-size: 9px">
            数据集：{{ dashboardDataset?.name }} · 生成后可在编辑器中继续调整
          </div>
        </div>
      </template>
      <div class="form-grid">
        <label class="field-label">
          看板名称
          <el-input v-model="dashboardForm.name" maxlength="100" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          组件标题
          <el-input v-model="dashboardForm.title" maxlength="100" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          图表类型
          <el-select v-model="dashboardForm.chart_type" style="width: 100%; margin-top: 8px">
            <el-option value="kpi" label="指标卡" />
            <el-option value="line" label="折线图" />
            <el-option value="area" label="面积图" />
            <el-option value="bar" label="柱状图" />
            <el-option value="bar_h" label="条形图" />
            <el-option value="bar_stacked" label="堆叠柱" />
            <el-option value="combo" label="组合图" />
            <el-option value="scatter" label="散点图" />
            <el-option value="donut" label="环形图" />
            <el-option value="table" label="明细表" />
            <el-option value="gauge" label="目标达成" />
          </el-select>
        </label>
        <label class="field-label">
          单位
          <el-input v-model="dashboardForm.unit" maxlength="16" style="margin-top: 8px" placeholder="元" />
        </label>
        <label class="field-label">
          分类字段
          <el-select v-model="dashboardForm.x_field" filterable allow-create style="width: 100%; margin-top: 8px">
            <el-option v-for="field in dashboardFields" :key="field" :value="field" :label="field" />
          </el-select>
        </label>
        <label class="field-label">
          数值字段
          <el-select v-model="dashboardForm.y_field" filterable allow-create style="width: 100%; margin-top: 8px">
            <el-option v-for="field in dashboardFields" :key="field" :value="field" :label="field" />
          </el-select>
        </label>
        <div class="field-label form-span-2">
          发布状态
          <div class="scope-note" style="margin-top: 8px; width: 100%">
            <el-switch v-model="dashboardForm.publish" />
            <span>{{ dashboardForm.publish ? '生成后立即发布给有权限的用户' : '先生成草稿，稍后发布' }}</span>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="inline-actions" style="justify-content: flex-end">
          <button class="btn btn-secondary" type="button" @click="dashboardVisible = false">取消</button>
          <button class="btn btn-primary" type="button" :disabled="dashboardSaving" @click="createDashboard">
            {{ dashboardSaving ? '生成中…' : '生成看板' }}
          </button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.sql-preview-block {
  margin-top: 4px;
  padding: 12px 14px;
  border: 1px solid var(--line-soft);
  border-radius: 10px;
  background: #fbfdfb;
}
.sql-preview-table { max-height: 300px; overflow: auto; border-radius: 8px; }
.sql-preview-table .data-table { min-width: 100%; }
.sql-preview-chart { height: 260px; width: 100%; margin-top: 8px; }
.sql-preview-tip { margin: 8px 0 0; color: var(--muted-light); font-size: 9px; }
.sql-preview-config {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 7px;
  margin-top: 10px;
  padding: 8px 10px;
  border: 1px dashed var(--line-soft);
  border-radius: 8px;
  background: #fff;
}
.sql-preview-config-label {
  color: var(--muted);
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.04em;
}
.sql-preview-config-select { width: 132px; }
.sql-preview-auto-tip {
  margin-left: auto;
  color: var(--muted-light);
  font-size: 8.5px;
}
@media (max-width: 720px) {
  .sql-preview-block .table-toolbar { flex-direction: column; align-items: stretch; gap: 10px; }
  .sql-preview-chart { height: 200px; }
  .sql-preview-config-select { flex: 1; min-width: 100px; width: auto; }
  .sql-preview-auto-tip { width: 100%; margin-left: 0; }
}
</style>
