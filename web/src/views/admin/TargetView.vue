<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  Download,
  FileSpreadsheet,
  Pencil,
  Plus,
  RefreshCw,
  Target as TargetIcon,
  Trash2,
  Upload,
} from 'lucide-vue-next'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, getErrorMessage } from '../../api/client'
import { toNumber } from '../../utils/format'
import EmptyBlock from '../../components/EmptyBlock.vue'
import NumText from '../../components/NumText.vue'
import OrgTreeSelect from '../../components/OrgTreeSelect.vue'

const loading = ref(true)
const saving = ref(false)
const importing = ref(false)
const fileInput = ref(null)

const periodType = ref('month')
const period = ref(currentPeriod('month'))
const metric = ref('revenue')
const taskType = ref('')

const metrics = ref([])
const achievement = ref(null)
const targets = ref([])
const extraPeriods = ref([])
const orgTree = ref([])

const dialogVisible = ref(false)
const editingId = ref(null)
const form = reactive({
  name: '',
  org_id: null,
  period_type: 'month',
  period: currentPeriod('month'),
  metric: 'revenue',
  task_type: 'base',
  target_value: null,
  unit: '元',
  remark: '',
})
const customRange = ref([])

const metricInfo = computed(() => metrics.value.find((item) => item.key === metric.value) || { key: 'revenue', label: '销售额', unit: '元' })
const metricFormat = computed(() => {
  const key = metricInfo.value.key
  if (key === 'gross_margin') return 'percent'
  if (key === 'revenue' || key === 'gross_profit') return 'currency'
  return 'integer'
})
const summary = computed(() => achievement.value?.summary || { target_value: 0, actual_value: 0, rate: 0, count: 0 })
const baseSummary = computed(() => achievement.value?.base_summary || { target_value: 0, actual_value: 0, rate: 0, count: 0 })
const challengeSummary = computed(
  () => achievement.value?.challenge_summary || { target_value: 0, actual_value: 0, rate: 0, count: 0 },
)
const rows = computed(() => achievement.value?.targets || [])
const targetMap = computed(() => Object.fromEntries(targets.value.map((item) => [item.id, item])))

const monthOptions = computed(() => {
  const now = new Date()
  return Array.from({ length: 24 }, (_, index) => {
    const date = new Date(now.getFullYear(), now.getMonth() - index, 1)
    return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`
  })
})

const quarterOptions = computed(() => {
  const now = new Date()
  const current = Math.floor(now.getMonth() / 3)
  return Array.from({ length: 12 }, (_, index) => {
    const quarter = current - index
    const year = now.getFullYear() + Math.floor(quarter / 4)
    const normalized = ((quarter % 4) + 4) % 4
    return `${year}-Q${normalized + 1}`
  })
})

const yearOptions = computed(() => {
  const now = new Date().getFullYear()
  return Array.from({ length: 6 }, (_, index) => String(now - index))
})

const periodOptions = computed(() => {
  let base = monthOptions.value
  if (periodType.value === 'quarter') base = quarterOptions.value
  if (periodType.value === 'year') base = yearOptions.value
  const merged = new Set([...base, ...extraPeriods.value])
  return [...merged]
})

const aheadCount = computed(() => rows.value.filter((item) => item.status === 'ahead').length)
const behindCount = computed(() => rows.value.filter((item) => item.status === 'behind').length)

function currentPeriod(type) {
  const now = new Date()
  if (type === 'quarter') return `${now.getFullYear()}-Q${Math.floor(now.getMonth() / 3) + 1}`
  if (type === 'year') return String(now.getFullYear())
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

function statusMeta(status) {
  const map = {
    ahead: { label: '超前', className: 'tag-green' },
    normal: { label: '正常', className: 'tag-amber' },
    behind: { label: '落后', className: 'tag-red' },
  }
  return map[status] || { label: status || '—', className: '' }
}

function periodTypeLabel(type) {
  const map = { month: '月度', quarter: '季度', year: '年度', custom: '区间' }
  return map[type] || type || '—'
}

function taskTypeLabel(value) {
  return value === 'challenge' ? '挑战任务' : '基础任务'
}

async function load() {
  loading.value = true
  try {
    const params = new URLSearchParams({
      period_type: periodType.value,
      period: period.value,
      metric: metric.value,
    })
    if (taskType.value) params.set('task_type', taskType.value)
    const [ach, list, periodList] = await Promise.all([
      api.get(`/targets/achievement?${params.toString()}`),
      api.get(`/targets?${params.toString()}`),
      api.get(`/targets/periods?period_type=${periodType.value}`),
    ])
    achievement.value = ach
    targets.value = list || []
    extraPeriods.value = periodList || []
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

async function loadMeta() {
  try {
    const [metricList, tree] = await Promise.all([api.get('/targets/metrics'), api.get('/orgs/tree')])
    metrics.value = metricList || []
    orgTree.value = tree || []
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  }
}

function changePeriodType(value) {
  period.value = currentPeriod(value)
  extraPeriods.value = []
  load()
}

function resetForm() {
  editingId.value = null
  Object.assign(form, {
    name: '',
    org_id: null,
    period_type: periodType.value,
    period: period.value,
    metric: metric.value,
    task_type: taskType.value || 'base',
    target_value: null,
    unit: metricInfo.value.unit || '元',
    remark: '',
  })
  customRange.value = []
}

function openCreate() {
  resetForm()
  dialogVisible.value = true
}

function openEdit(row) {
  const detail = targetMap.value[row.id]
  if (!detail) {
    ElMessage.warning('任务详情已失效，请刷新后重试')
    return
  }
  resetForm()
  editingId.value = detail.id
  Object.assign(form, {
    name: detail.name || '',
    org_id: detail.org_id,
    period_type: detail.period_type || 'month',
    period: detail.period || currentPeriod(detail.period_type || 'month'),
    metric: detail.metric || 'revenue',
    task_type: detail.task_type || 'base',
    target_value: detail.target_value,
    unit: detail.unit || '元',
    remark: detail.remark || '',
  })
  if (detail.period_type === 'custom' && detail.period?.includes('~')) {
    customRange.value = detail.period.split('~')
  }
  dialogVisible.value = true
}

async function save() {
  if (!form.org_id) {
    ElMessage.warning('请选择任务所属组织')
    return
  }
  if (!form.metric) {
    ElMessage.warning('请选择指标')
    return
  }
  if (!(Number(form.target_value) > 0)) {
    ElMessage.warning('任务值必须大于 0')
    return
  }
  let periodValue = form.period
  if (form.period_type === 'custom') {
    if (!customRange.value?.[0] || !customRange.value?.[1]) {
      ElMessage.warning('请选择自定义周期日期范围')
      return
    }
    periodValue = `${customRange.value[0]}~${customRange.value[1]}`
  }
  if (!periodValue) {
    ElMessage.warning('请选择任务周期')
    return
  }
  saving.value = true
  try {
    const payload = {
      name: form.name.trim(),
      org_id: Number(form.org_id),
      period_type: form.period_type,
      period: periodValue,
      metric: form.metric,
      task_type: form.task_type,
      target_value: Number(form.target_value),
      unit: form.unit,
      remark: form.remark,
    }
    if (editingId.value) {
      await api.put(`/targets/${editingId.value}`, payload)
      ElMessage.success('任务已更新')
    } else {
      await api.post('/targets', payload)
      ElMessage.success('任务已创建')
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
    await ElMessageBox.confirm(`确认删除任务「${row.name}」？`, '删除任务', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
    await api.delete(`/targets/${row.id}`)
    ElMessage.success('任务已删除')
    await load()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(getErrorMessage(error))
  }
}

async function exportTargets() {
  try {
    await api.download('/targets/export', `经营任务-${new Date().toISOString().slice(0, 10)}.xlsx`)
    ElMessage.success('目标已导出')
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  }
}

async function downloadTemplate() {
  try {
    await api.download('/targets/template', '经营任务导入模板.xlsx')
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  }
}

function pickImport() {
  fileInput.value?.click()
}

async function onImportChange(event) {
  const file = event.target?.files?.[0]
  if (event.target) event.target.value = ''
  if (!file) return
  importing.value = true
  try {
    const result = await api.upload('/targets/import', file)
    ElMessage.success(`导入完成：新增 ${result?.created || 0} 条，更新 ${result?.updated || 0} 条`)
    await load()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    importing.value = false
  }
}

onMounted(async () => {
  await loadMeta()
  await load()
})
</script>

<template>
  <div>
    <div class="panel" style="padding: 16px 18px; margin-bottom: 14px">
      <div class="table-toolbar" style="padding: 0; margin: 0">
        <div class="table-toolbar-left">
          <el-select v-model="periodType" size="small" style="width: 100px" @change="changePeriodType">
            <el-option value="month" label="按月" />
            <el-option value="quarter" label="按季" />
            <el-option value="year" label="按年" />
          </el-select>
          <el-select v-model="period" size="small" filterable style="width: 130px" @change="load">
            <el-option v-for="item in periodOptions" :key="item" :value="item" :label="item" />
          </el-select>
          <el-select v-model="metric" size="small" style="width: 130px" @change="load">
            <el-option v-for="item in metrics" :key="item.key" :value="item.key" :label="item.label" />
          </el-select>
          <el-select v-model="taskType" size="small" style="width: 120px" @change="load">
            <el-option value="" label="全部任务" />
            <el-option value="base" label="基础任务" />
            <el-option value="challenge" label="挑战任务" />
          </el-select>
          <button class="btn btn-secondary btn-sm" type="button" :disabled="loading" @click="load">
            <RefreshCw :size="13" /> 刷新
          </button>
        </div>
        <div class="inline-actions">
          <input ref="fileInput" type="file" accept=".xlsx,.xls" style="display: none" @change="onImportChange" />
          <button class="btn btn-secondary btn-sm" type="button" @click="downloadTemplate">
            <FileSpreadsheet :size="13" /> 模板
          </button>
          <button class="btn btn-secondary btn-sm" type="button" :disabled="importing" @click="pickImport">
            <Upload :size="13" /> {{ importing ? '导入中…' : '导入任务' }}
          </button>
          <button class="btn btn-secondary btn-sm" type="button" @click="exportTargets">
            <Download :size="13" /> 导出
          </button>
          <button class="btn btn-primary btn-sm" type="button" @click="openCreate">
            <Plus :size="14" /> 新增任务
          </button>
        </div>
      </div>
    </div>

    <div class="metric-mini-grid">
      <div class="metric-mini">
        <span>基础任务 · {{ metricInfo.label }}</span>
        <strong><NumText :value="baseSummary.target_value" :format="metricFormat" start-compact /></strong>
        <small>
          实际 <NumText :value="baseSummary.actual_value" :format="metricFormat" start-compact /> ·
          {{ baseSummary.target_value ? `${toNumber(baseSummary.rate).toFixed(1)}%` : '—' }}
        </small>
      </div>
      <div class="metric-mini">
        <span>挑战任务 · {{ metricInfo.label }}</span>
        <strong><NumText :value="challengeSummary.target_value" :format="metricFormat" start-compact /></strong>
        <small>
          实际 <NumText :value="challengeSummary.actual_value" :format="metricFormat" start-compact /> ·
          {{ challengeSummary.target_value ? `${toNumber(challengeSummary.rate).toFixed(1)}%` : '—' }}
        </small>
      </div>
      <div class="metric-mini">
        <span>{{ metricInfo.label }}整体达成率</span>
        <strong :class="['rate-text', summary.rate >= 100 ? 'ahead' : summary.rate >= 80 ? 'normal' : 'behind']">
          {{ summary.target_value ? `${toNumber(summary.rate).toFixed(1)}%` : '—' }}
        </strong>
        <small>实际 <NumText :value="summary.actual_value" :format="metricFormat" start-compact /> · 顶层任务不重复累加</small>
      </div>
      <div class="metric-mini">
        <span>任务状态</span>
        <strong>{{ aheadCount }} / {{ rows.length }}</strong>
        <small>超前 {{ aheadCount }} · 落后 {{ behindCount }} · {{ achievement?.start_date || '—' }} 至 {{ achievement?.end_date || '—' }}</small>
      </div>
    </div>

    <div class="panel" style="margin-top: 14px">
      <div class="panel-head">
        <div>
          <div class="panel-title">任务达成明细</div>
          <div class="panel-subtitle">{{ metricInfo.label }} · {{ period }} · 按当前账号数据权限过滤</div>
        </div>
        <span class="tag tag-green">{{ rows.length }} 条</span>
      </div>

      <div v-if="loading" style="padding: 20px">
        <div class="skeleton" style="height: 280px" />
      </div>
      <el-table v-else-if="rows.length" :data="rows" style="width: 100%">
        <el-table-column label="组织" min-width="180">
          <template #default="{ row }">
            <div>
              <strong style="font-size: 10.5px">{{ row.org_name || '—' }}</strong>
              <div style="color: var(--muted-light); font-size: 8px; margin-top: 2px">{{ row.name }}</div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="周期" width="150">
          <template #default="{ row }">
            <span class="tag">{{ periodTypeLabel(row.period_type) }}</span>
            <span style="margin-left: 5px; font-size: 9px; color: var(--muted)">{{ row.period }}</span>
          </template>
        </el-table-column>
        <el-table-column label="任务类型" width="105">
          <template #default="{ row }">
            <span class="tag" :class="row.task_type === 'challenge' ? 'tag-amber' : 'tag-green'">
              {{ row.task_type_label || taskTypeLabel(row.task_type) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="任务值" min-width="120" align="right">
          <template #default="{ row }"><NumText :value="row.target_value" :format="metricFormat" /></template>
        </el-table-column>
        <el-table-column label="实际值" min-width="120" align="right">
          <template #default="{ row }">
            <strong><NumText :value="row.actual_value" :format="metricFormat" /></strong>
          </template>
        </el-table-column>
        <el-table-column label="达成率" min-width="170">
          <template #default="{ row }">
            <div class="rate-cell">
              <div class="rate-track">
                <span
                  :class="['rate-fill', row.status]"
                  :style="{ width: `${Math.min(100, Math.max(2, toNumber(row.rate)))}%` }"
                />
              </div>
              <b :class="['rate-text', row.status]">{{ toNumber(row.rate).toFixed(1) }}%</b>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <span class="tag" :class="statusMeta(row.status).className">{{ statusMeta(row.status).label }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <div class="inline-actions">
              <button class="btn btn-ghost btn-sm" type="button" @click="openEdit(row)">
                <Pencil :size="13" /> 编辑
              </button>
              <button class="btn btn-ghost btn-sm" type="button" style="color: var(--rose)" @click="remove(row)">
                <Trash2 :size="13" />
              </button>
            </div>
          </template>
        </el-table-column>
      </el-table>
      <EmptyBlock
        v-else
        title="当前周期还没有任务"
        description="任务需要手动添加后才会出现在这里，可按基础任务 / 挑战任务分别添加或批量导入。"
      >
        <button class="btn btn-primary btn-sm" type="button" @click="openCreate">
          <Plus :size="14" /> 新增任务
        </button>
      </EmptyBlock>
    </div>

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑任务' : '新增任务'" width="640px" destroy-on-close>
      <div class="form-grid">
        <label class="field-label form-span-2">
          所属组织
          <OrgTreeSelect v-model="form.org_id" :data="orgTree" placeholder="选择目标归属组织" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          周期类型
          <el-select v-model="form.period_type" style="width: 100%; margin-top: 8px">
            <el-option value="month" label="按月" />
            <el-option value="quarter" label="按季" />
            <el-option value="year" label="按年" />
            <el-option value="custom" label="自定义区间" />
          </el-select>
        </label>
        <label class="field-label">
          指标
          <el-select v-model="form.metric" style="width: 100%; margin-top: 8px">
            <el-option v-for="item in metrics" :key="item.key" :value="item.key" :label="item.label" />
          </el-select>
        </label>
        <label class="field-label form-span-2">
          任务类型
          <el-radio-group v-model="form.task_type" style="margin-top: 8px">
            <el-radio-button value="base">基础任务</el-radio-button>
            <el-radio-button value="challenge">挑战任务</el-radio-button>
          </el-radio-group>
        </label>
        <label v-if="form.period_type !== 'custom'" class="field-label form-span-2">
          周期
          <el-input
            v-model="form.period"
            style="margin-top: 8px"
            :placeholder="form.period_type === 'quarter' ? '2026-Q4' : form.period_type === 'year' ? '2026' : '2026-11'"
          />
        </label>
        <label v-else class="field-label form-span-2">
          自定义区间
          <el-date-picker
            v-model="customRange"
            type="daterange"
            value-format="YYYY-MM-DD"
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            style="width: 100%; margin-top: 8px"
          />
        </label>
        <label class="field-label">
          任务值
          <el-input-number
            v-model="form.target_value"
            :min="0"
            :controls="false"
            style="width: 100%; margin-top: 8px"
            placeholder="0"
          />
        </label>
        <label class="field-label">
          单位
          <el-input v-model="form.unit" maxlength="16" style="margin-top: 8px" placeholder="元" />
        </label>
        <label class="field-label form-span-2">
          任务名称
          <el-input v-model="form.name" maxlength="100" style="margin-top: 8px" placeholder="留空则自动生成" />
        </label>
        <label class="field-label form-span-2">
          备注
          <el-input v-model="form.remark" maxlength="255" style="margin-top: 8px" placeholder="可选" />
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
  </div>
</template>

<style scoped>
.rate-text.ahead { color: #0f766e; }
.rate-text.normal { color: #b7791f; }
.rate-text.behind { color: #c53030; }
.rate-cell { display: flex; align-items: center; gap: 8px; }
.rate-track { flex: 1; min-width: 52px; height: 5px; border-radius: 999px; background: #e9efe9; overflow: hidden; }
.rate-fill { display: block; height: 100%; border-radius: inherit; background: #0f766e; }
.rate-fill.normal { background: #d69e2e; }
.rate-fill.behind { background: #c53030; }
.rate-cell b { font-size: 9px; white-space: nowrap; }
</style>
