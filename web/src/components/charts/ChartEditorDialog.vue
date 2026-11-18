<template>
  <el-dialog
    :model-value="modelValue"
    :title="chart ? `编辑图表：${chart.name}` : '新建图表'"
    :width="dialogWidth(1180)"
    top="3vh"
    destroy-on-close
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
  >
    <div class="editor">
      <div class="panel config-panel">
        <el-form label-width="82px" size="default">
          <el-form-item label="名称">
            <el-input v-model="name" placeholder="图表名称" />
          </el-form-item>
          <el-form-item label="数据集">
            <el-select v-model="datasetId" style="width: 100%" @change="onDatasetChange" filterable>
              <el-option v-for="ds in datasets" :key="ds.id" :label="ds.name" :value="ds.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="类型">
            <el-radio-group v-model="type" @change="onTypeChange">
              <el-radio-button value="bar">柱状图</el-radio-button>
              <el-radio-button value="line">折线图</el-radio-button>
              <el-radio-button value="pie">饼图</el-radio-button>
              <el-radio-button value="table">表格</el-radio-button>
              <el-radio-button value="kpi">KPI</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="数据模式">
            <el-radio-group v-model="dataMode" :disabled="type !== 'table'">
              <el-radio-button value="aggregate">聚合</el-radio-button>
              <el-radio-button value="raw">明细</el-radio-button>
            </el-radio-group>
          </el-form-item>

          <el-divider content-position="left">维度</el-divider>
          <div v-for="(d, i) in dimensions" :key="'d' + i" class="field-row">
            <el-select v-model="d.field" placeholder="选择字段" style="flex: 1" filterable>
              <el-option v-for="c in columns" :key="c.name" :label="`${c.name} (${c.logicalType})`" :value="c.name" />
            </el-select>
            <el-select
              v-model="d.grain"
              clearable
              placeholder="原值"
              style="width: 96px"
              :disabled="!isDateField(d.field)" filterable>
              <el-option label="按日" value="day" />
              <el-option label="按周" value="week" />
              <el-option label="按月" value="month" />
              <el-option label="按季" value="quarter" />
              <el-option label="按年" value="year" />
            </el-select>
            <el-input v-model="d.alias" placeholder="别名" style="width: 100px" />
            <el-button link type="danger" @click="dimensions.splice(i, 1)">删除</el-button>
          </div>
          <el-button size="small" @click="dimensions.push({ field: '', alias: '', grain: '' })">+ 维度</el-button>

          <template v-if="dataMode === 'aggregate'">
            <el-divider content-position="left">基础指标</el-divider>
            <div v-if="!baseMetrics.length" class="empty-hint">还没有基础指标，点击下方按钮添加</div>
            <div v-for="m in baseMetrics" :key="'m' + m._key" class="metric-card">
              <div class="metric-line">
                <el-select v-model="m.field" placeholder="选择字段" class="metric-field" filterable>
                  <el-option label="* (计数)" value="*" />
                  <el-option v-for="c in numericColumns" :key="c.name" :label="c.name" :value="c.name" />
                </el-select>
                <el-select v-model="m.agg" class="metric-agg" filterable>
                  <el-option label="求和" value="sum" />
                  <el-option label="平均" value="avg" />
                  <el-option label="计数" value="count" />
                  <el-option label="去重计数" value="count_distinct" />
                  <el-option label="最小" value="min" />
                  <el-option label="最大" value="max" />
                </el-select>
                <el-button link type="danger" class="metric-del" @click="removeMetric(m)">删除</el-button>
              </div>
              <div class="metric-line">
                <el-input v-model="m.alias" placeholder="指标别名（如 实际销售）" class="metric-alias" />
                <el-select v-model="m.format" class="metric-format" filterable>
                  <el-option label="数字" value="number" />
                  <el-option label="百分比" value="percent" />
                  <el-option label="金额" value="currency" />
                </el-select>
              </div>
            </div>
            <el-button size="small" class="add-btn" @click="addBaseMetric">+ 基础指标</el-button>

            <el-divider content-position="left">计算指标</el-divider>
            <div v-if="!calcMetrics.length" class="empty-hint">
              基于上方基础指标计算比率，例如 <code>实际销售 / NULLIF(基础目标, 0) * 100</code>
            </div>
            <div v-for="m in calcMetrics" :key="'c' + m._key" class="calc-row">
              <el-tag size="small" type="warning" effect="plain">fx</el-tag>
              <span class="calc-alias" :title="m.alias">{{ m.alias || '未命名' }}</span>
              <code class="calc-expr" :title="m.expr">{{ m.expr }}</code>
              <el-tag size="small" effect="plain">{{ formatLabel(m.format) }}</el-tag>
              <el-button link type="primary" @click="editCalc(m)">编辑</el-button>
              <el-button link type="danger" @click="removeMetric(m)">删除</el-button>
            </div>
            <el-button size="small" type="primary" plain class="add-btn" @click="openCalcDialog()">
              + 计算指标
            </el-button>
          </template>

          <el-divider content-position="left">展示</el-divider>
          <el-form-item label="排序">
            <el-select v-model="sortAlias" clearable placeholder="不排序" style="width: 150px" @change="onSortAlias" filterable>
              <el-option v-for="a in aliasOptions" :key="a" :label="a" :value="a" />
            </el-select>
            <el-select v-model="sortDesc" style="width: 110px; margin-left: 8px" :disabled="!sortAlias" filterable>
              <el-option label="降序" :value="true" />
              <el-option label="升序" :value="false" />
            </el-select>
          </el-form-item>
          <el-form-item label="行数上限">
            <el-input-number v-model="limit" :min="1" :max="5000" controls-position="right" />
          </el-form-item>
          <el-form-item label="小数位">
            <el-input-number v-model="decimals" :min="-1" :max="6" controls-position="right" />
          </el-form-item>
          <el-form-item label="图例">
            <el-switch v-model="legend" />
          </el-form-item>
          <el-form-item v-if="type === 'bar'" label="堆叠">
            <el-switch v-model="stack" />
          </el-form-item>
          <el-form-item v-if="type === 'line'" label="面积">
            <el-switch v-model="area" />
          </el-form-item>
          <el-form-item label="数值标签">
            <el-switch v-model="showLabel" />
          </el-form-item>
        </el-form>
      </div>

      <div class="panel preview-panel">
        <div class="preview-toolbar">
          <el-button type="primary" :loading="previewing" @click="runPreview">刷新预览</el-button>
          <span v-if="previewResult" class="muted">
            {{ previewResult.rowCount }} 行 / {{ previewResult.elapsedMs }}ms
            <el-tag v-if="previewResult.truncated" size="small" type="warning">已截断</el-tag>
          </span>
          <span v-if="previewError" class="error-text">{{ previewError }}</span>
        </div>
        <div class="preview-body">
          <ChartRenderer :type="type" :result="previewResult" :config="currentConfig" />
        </div>
      </div>
    </div>

    <el-dialog
      v-model="calcVisible"
      :title="editingCalcKey ? '编辑计算指标' : '新增计算指标'"
      :width="dialogWidth(600)"
      append-to-body
    >
      <el-alert type="info" :closable="false" style="margin-bottom: 12px">
        计算指标在聚合之后计算，可引用已添加的基础指标别名。除法会自动防止除零。
      </el-alert>
      <el-form label-width="80px">
        <el-form-item label="别名">
          <el-input v-model="calcForm.alias" placeholder="例如 目标达成率" />
        </el-form-item>
        <el-form-item label="表达式">
          <el-input
            v-model="calcForm.expr"
            type="textarea"
            :rows="3"
            class="calc-input"
            placeholder="sales_sum / NULLIF(target_sum, 0) * 100"
          />
          <div class="calc-help">
            <div class="muted">可用基础指标别名：</div>
            <el-tag
              v-for="a in baseMetricAliases"
              :key="a"
              size="small"
              class="alias-chip"
              @click="insertAlias(a)"
            >
              {{ a }}
            </el-tag>
            <div class="muted" style="margin-top: 6px">
              支持 + - * / 与括号，函数 NULLIF / COALESCE / ABS / ROUND。
            </div>
          </div>
        </el-form-item>
        <el-alert v-if="calcError" :title="calcError" type="error" show-icon :closable="false" />
        <el-form-item label="格式">
          <el-radio-group v-model="calcForm.format">
            <el-radio-button value="number">数字</el-radio-button>
            <el-radio-button value="percent">百分比</el-radio-button>
            <el-radio-button value="currency">金额</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="常用模板">
          <div class="template-list">
            <el-button v-for="t in calcTemplates" :key="t.label" size="small" @click="applyTemplate(t)">
              {{ t.label }}
            </el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="calcVisible = false">取消</el-button>
        <el-button type="primary" @click="applyCalc">{{ editingCalcKey ? '保存' : '添加' }}</el-button>
      </template>
    </el-dialog>

    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存图表</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { chartApi, datasetApi } from '@/api'
import type { Chart, ChartConfig, ColumnMeta, Dataset, FieldRef, MetricRef, ResultSet } from '@/types'
import ChartRenderer from './ChartRenderer.vue'
import { dialogWidth } from '@/utils/responsive'

const props = defineProps<{
  modelValue: boolean
  chart?: Chart | null
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'saved', chart: Chart): void
}>()

const datasets = ref<Dataset[]>([])
const columns = ref<ColumnMeta[]>([])
const name = ref('')
const datasetId = ref<number>(0)
const type = ref<'bar' | 'line' | 'pie' | 'table' | 'kpi'>('bar')
const dataMode = ref<'aggregate' | 'raw'>('aggregate')
const dimensions = ref<FieldRef[]>([])
interface MetricRow {
  _key: string
  field: string
  agg: MetricRef['agg']
  alias: string
  expr?: string
  format?: MetricRef['format']
}
const metrics = ref<MetricRow[]>([])

function newKey() {
  return `m${Date.now()}_${Math.floor(Math.random() * 1000)}`
}
const sortAlias = ref('')
const sortDesc = ref(true)
const limit = ref(500)
const decimals = ref(2)
const legend = ref(true)
const stack = ref(false)
const area = ref(false)
const showLabel = ref(false)

const calcVisible = ref(false)
const editingCalcKey = ref<string | null>(null)
const calcForm = reactive({ alias: '', expr: '', format: 'percent' as MetricRef['format'] })
const calcError = ref('')

const baseMetrics = computed(() => metrics.value.filter((m) => !m.expr?.trim()))
const calcMetrics = computed(() => metrics.value.filter((m) => !!m.expr?.trim()))
const baseMetricAliases = computed(() =>
  baseMetrics.value.filter((m) => m.field).map((m) => m.alias || `${m.agg}_${m.field}`),
)

function formatLabel(f?: string) {
  return { percent: '百分比', currency: '金额', number: '数字' }[f || 'number'] || '数字'
}

function addBaseMetric() {
  metrics.value.push({ _key: newKey(), field: '', agg: 'sum', alias: '', format: 'number' })
}

function removeMetric(m: { _key: string }) {
  const i = metrics.value.findIndex((x) => x._key === m._key)
  if (i >= 0) metrics.value.splice(i, 1)
}

const calcTemplates = [
  { label: '达成率', alias: '达成率', expr: '{a} / NULLIF({b}, 0) * 100', format: 'percent' },
  { label: '毛利率', alias: '毛利率', expr: '{a} / NULLIF({b}, 0) * 100', format: 'percent' },
  { label: '差值', alias: '差额', expr: '{a} - {b}', format: 'number' },
  { label: '同比增幅', alias: '增幅', expr: '({a} - {b}) / NULLIF({b}, 0) * 100', format: 'percent' },
]

function openCalcDialog(edit?: any) {
  calcError.value = ''
  if (edit) {
    editingCalcKey.value = edit._key
    calcForm.alias = edit.alias
    calcForm.expr = edit.expr || ''
    calcForm.format = (edit.format || 'percent') as MetricRef['format']
  } else {
    editingCalcKey.value = null
    calcForm.alias = ''
    calcForm.expr = ''
    calcForm.format = 'percent'
  }
  calcVisible.value = true
}

function editCalc(m: any) {
  openCalcDialog(m)
}

function insertAlias(alias: string) {
  calcForm.expr = calcForm.expr ? `${calcForm.expr} ${alias}` : alias
}

function applyTemplate(t: { alias: string; expr: string; format: string }) {
  const [a, b] = baseMetricAliases.value
  if (!a || !b) {
    ElMessage.warning('请先添加至少两个基础指标')
    return
  }
  calcForm.alias = t.alias
  calcForm.expr = t.expr.replace('{a}', a).replace('{b}', b)
  calcForm.format = t.format as MetricRef['format']
}

function applyCalc() {
  calcError.value = ''
  if (!calcForm.alias.trim() || !calcForm.expr.trim()) {
    calcError.value = '请填写别名和表达式'
    return
  }
  const alias = calcForm.alias.trim()
  if (baseMetricAliases.value.includes(alias)) {
    calcError.value = '别名与基础指标重复'
    return
  }
  const dup = calcMetrics.value.find((m) => m.alias === alias && m._key !== editingCalcKey.value)
  if (dup) {
    calcError.value = '已存在同名计算指标'
    return
  }
  if (editingCalcKey.value) {
    const target = metrics.value.find((m) => m._key === editingCalcKey.value)
    if (target) {
      target.alias = alias
      target.expr = calcForm.expr.trim()
      target.format = calcForm.format
    }
  } else {
    metrics.value.push({
      _key: newKey(),
      field: '',
      agg: 'sum',
      alias,
      expr: calcForm.expr.trim(),
      format: calcForm.format,
    })
  }
  calcVisible.value = false
}



const previewing = ref(false)
const previewResult = ref<ResultSet | null>(null)
const previewError = ref('')
const saving = ref(false)

const numericColumns = computed(() =>
  columns.value.filter((c) => c.logicalType === 'number'),
)

function isDateField(field: string) {
  const c = columns.value.find((x) => x.name === field)
  return c?.logicalType === 'date' || c?.logicalType === 'datetime'
}
const aliasOptions = computed(() => {
  const out = dimensions.value.map((d) => d.alias || d.field).filter(Boolean)
  for (const m of metrics.value) out.push(m.alias || `${m.agg}_${m.field}`)
  return out
})

const currentConfig = computed<ChartConfig>(() => ({
  chartType: type.value,
  dataMode: dataMode.value,
  dimensions: dimensions.value.filter((d) => d.field).map((d) => ({ field: d.field, alias: d.alias || d.field, grain: d.grain || undefined })),
  metrics: dataMode.value === 'aggregate'
    ? metrics.value
        .filter((m) => m.field || m.expr?.trim())
        .map((m) => ({
          field: m.field,
          agg: m.agg,
          alias: m.alias || `${m.agg}_${m.field}`,
          ...(m.expr?.trim() ? { expr: m.expr.trim() } : {}),
          ...(m.format ? { format: m.format } : {}),
        }))
    : [],
  sort: sortAlias.value ? [{ alias: sortAlias.value, desc: sortDesc.value }] : [],
  limit: limit.value,
  paramDefaults: {},
  format: { decimals: decimals.value, numberFormat: '' },
  echarts: { legend: legend.value, stack: stack.value, area: area.value, showLabel: showLabel.value },
}))

async function loadDatasets() {
  datasets.value = await datasetApi.list()
}

async function loadColumns(id: number) {
  if (!id) {
    columns.value = []
    return
  }
  try {
    columns.value = await datasetApi.columns(id)
  } catch {
    columns.value = []
  }
}

async function onDatasetChange() {
  await loadColumns(datasetId.value)
  dimensions.value = []
  metrics.value = []
  previewResult.value = null
}

function onTypeChange() {
  if (type.value === 'table' && dataMode.value === 'aggregate') {
    // keep as-is; user may switch to raw
  }
  if (type.value === 'kpi') {
    dimensions.value = []
    if (!metrics.value.length) {
      metrics.value = [{ _key: newKey(), field: '', agg: 'sum', alias: '', format: 'number' }]
    }
  }
  if (type.value === 'pie') {
    dataMode.value = 'aggregate'
    metrics.value = metrics.value.slice(0, 1)
  }
  if (type.value !== 'table') dataMode.value = 'aggregate'
}

function onSortAlias(val: string) {
  if (!val) sortDesc.value = true
}

async function hydrate(chart: Chart) {
  name.value = chart.name
  datasetId.value = chart.datasetId
  type.value = chart.type
  await loadDatasets()
  await loadColumns(chart.datasetId)
  const cfg: ChartConfig = JSON.parse(chart.configJson || '{}')
  dataMode.value = cfg.dataMode || 'aggregate'
  dimensions.value = (cfg.dimensions || []).map((d) => ({ ...d }))
  metrics.value = (cfg.metrics || []).map((m) => ({
    _key: newKey(),
    field: m.field || '',
    agg: m.agg || 'sum',
    alias: m.alias || '',
    expr: m.expr || '',
    format: (m.format || 'number') as MetricRef['format'],
  }))
  sortAlias.value = cfg.sort?.[0]?.alias || ''
  sortDesc.value = cfg.sort?.[0]?.desc ?? true
  limit.value = cfg.limit || 500
  decimals.value = cfg.format?.decimals ?? 2
  legend.value = cfg.echarts?.legend !== false
  stack.value = !!cfg.echarts?.stack
  area.value = !!cfg.echarts?.area
  showLabel.value = !!cfg.echarts?.showLabel
  previewResult.value = null
}

function reset() {
  name.value = ''
  datasetId.value = 0
  type.value = 'bar'
  dataMode.value = 'aggregate'
  dimensions.value = []
  metrics.value = []
  sortAlias.value = ''
  limit.value = 500
  decimals.value = 2
  legend.value = true
  stack.value = false
  area.value = false
  showLabel.value = false
  previewResult.value = null
  previewError.value = ''
  columns.value = []
}

watch(
  () => props.modelValue,
  async (open) => {
    if (!open) return
    previewError.value = ''
    if (props.chart) {
      await hydrate(props.chart)
      // Show data immediately instead of an empty preview panel.
      await runPreview()
    } else {
      reset()
      await loadDatasets()
      if (!datasets.value.length) {
        ElMessage.warning('暂无可用的数据集，请先由管理员创建数据集')
      }
    }
  },
)

async function runPreview() {
  if (!datasetId.value) {
    ElMessage.warning('请先选择数据集')
    return
  }
  previewing.value = true
  previewError.value = ''
  try {
    previewResult.value = await chartApi.previewData({
      datasetId: datasetId.value,
      type: type.value,
      config: currentConfig.value,
    })
  } catch (e: any) {
    previewError.value = '预览失败，请检查字段配置'
    previewResult.value = null
  } finally {
    previewing.value = false
  }
}

async function save() {
  if (!name.value || !datasetId.value) {
    ElMessage.warning('请填写图表名称并选择数据集')
    return
  }
  if (type.value === 'kpi' && currentConfig.value.metrics.length !== 1) {
    ElMessage.warning('KPI 需要且仅需要一个指标')
    return
  }
  if (dataMode.value === 'aggregate' && !currentConfig.value.metrics.length) {
    ElMessage.warning('请至少配置一个指标')
    return
  }
  const badBase = baseMetrics.value.find((m) => !m.field)
  if (dataMode.value === 'aggregate' && badBase) {
    ElMessage.warning('存在未选择字段的基础指标')
    return
  }
  if (!currentConfig.value.dimensions.length) {
    ElMessage.warning('请至少配置一个维度')
    return
  }
  saving.value = true
  try {
    const payload = {
      name: name.value,
      description: '',
      datasetId: datasetId.value,
      type: type.value,
      config: currentConfig.value,
    }
    const chart = props.chart
      ? await chartApi.update(props.chart.id, payload)
      : await chartApi.create(payload)
    ElMessage.success('图表已保存')
    emit('saved', chart)
    emit('update:modelValue', false)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.metric-buttons {
  display: flex;
  gap: 8px;
}
.add-btn {
  margin-top: 4px;
}
.empty-hint {
  font-size: 12px;
  color: #a8abb2;
  padding: 4px 0 8px;
  line-height: 1.6;
}
.empty-hint code {
  background: #f4f4f5;
  border-radius: 3px;
  padding: 1px 4px;
}
.calc-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  margin-bottom: 6px;
  background: #fdf6ec;
  border: 1px solid #faecd8;
  border-radius: 6px;
}
.calc-alias {
  font-size: 12px;
  font-weight: 600;
  color: #303133;
  max-width: 96px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.calc-expr {
  flex: 1;
  font-family: Consolas, monospace;
  font-size: 11px;
  color: #606266;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.calc-input :deep(textarea) {
  font-family: Consolas, monospace;
  font-size: 12px;
}
.calc-help {
  margin-top: 6px;
  width: 100%;
}
.alias-chip {
  margin: 4px 4px 0 0;
  cursor: pointer;
}
.template-list {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.editor {
  display: flex;
  gap: 16px;
  height: 66vh;
}
@media (max-width: 1000px) {
  .editor {
    flex-direction: column;
    height: auto;
    max-height: 72vh;
    overflow-y: auto;
  }
  .config-panel {
    width: 100% !important;
    overflow: visible;
    padding-right: 4px;
  }
  .preview-panel {
    min-height: 320px;
  }
}
.panel {
  background: #fff;
  border: 1px solid #ebeef5;
  border-radius: 8px;
}
.config-panel {
  width: 470px;
  overflow-y: auto;
  padding: 12px 16px 12px 4px;
}
.preview-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 12px;
  min-width: 0;
}
.preview-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}
.preview-body {
  flex: 1;
  min-height: 0;
}
.field-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
}
.metric-card {
  border: 1px solid #ebeef5;
  border-radius: 6px;
  padding: 8px;
  margin-bottom: 8px;
  background: #fafbfc;
}
.metric-line {
  display: flex;
  align-items: center;
  gap: 8px;
}
.metric-line + .metric-line {
  margin-top: 6px;
}
.metric-field {
  flex: 1 1 auto;
  min-width: 0;
}
.metric-agg {
  width: 96px;
  flex: 0 0 auto;
}
.metric-del {
  flex: 0 0 auto;
}
.metric-alias {
  flex: 1 1 auto;
  min-width: 0;
}
.metric-format {
  width: 92px;
  flex: 0 0 auto;
}
.error-text {
  color: #f56c6c;
  font-size: 12px;
}
</style>
