<template>
  <div class="page-card">
    <div class="page-header">
      <span class="page-title">数据集</span>
      <el-button type="primary" :icon="Plus" @click="openCreate">新建数据集</el-button>
    </div>

    <div class="table-scroll">
      <el-table :data="list" v-loading="loading" border stripe>
      <el-table-column prop="name" label="名称" min-width="160" />
      <el-table-column prop="description" label="描述" min-width="200" show-overflow-tooltip />
      <el-table-column label="数据源" width="160">
        <template #default="{ row }">{{ dsName(row.dataSourceId) }}</template>
      </el-table-column>
      <el-table-column label="参数" width="180">
        <template #default="{ row }">
          <el-tag v-for="p in row.params" :key="p.name" size="small" effect="plain" style="margin-right: 4px">
            {{ p.name }}
          </el-tag>
          <span v-if="!row.params?.length" class="muted">无</span>
        </template>
      </el-table-column>
      <el-table-column label="更新时间" width="170">
        <template #default="{ row }">{{ fmt(row.updatedAt) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="260" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="primary" @click="openPreview(row)">预览</el-button>
          <el-button link type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="dialogVisible" :title="editing ? '编辑数据集' : '新建数据集'" :width="dialogWidth(860)" top="4vh" destroy-on-close>
      <el-form label-width="90px">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="名称">
              <el-input v-model="form.name" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="数据源">
              <el-select v-model="form.dataSourceId" style="width: 100%" filterable>
                <el-option v-for="ds in dataSources" :key="ds.id" :label="ds.name" :value="ds.id" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="描述">
          <el-input v-model="form.description" />
        </el-form-item>
        <el-form-item label="SQL">
          <div class="sql-block">
            <el-input
              ref="sqlInputRef"
              v-model="form.sqlText"
              type="textarea"
              :rows="10"
              class="sql-input"
              placeholder="SELECT region, order_date, amount FROM sales WHERE order_date >= {{from_date}}"
              @click="rememberCursor"
              @keyup="rememberCursor"
            />
            <div class="sql-toolbar">
              <el-button size="small" type="primary" :icon="VideoPlay" :loading="testing" @click="runTest">
                执行测试
              </el-button>
              <el-button size="small" :icon="MagicStick" @click="autoParams">
                自动生成参数
              </el-button>
              <template v-if="aiEnabled">
                <el-divider direction="vertical" />
                <el-button size="small" type="primary" plain :icon="Promotion" @click="aiDialog = true">
                  AI 生成 SQL
                </el-button>
                <el-button size="small" :loading="aiBusy === 'params'" @click="aiParams">
                  AI 补全参数
                </el-button>
                <el-button size="small" :loading="aiBusy === 'meta'" @click="aiMetadata">
                  AI 补全名称/描述
                </el-button>
                <el-button size="small" :loading="aiBusy === 'complete'" @click="aiComplete">
                  AI 续写
                </el-button>
              </template>
              <el-tag v-else size="small" type="info" effect="plain">AI 未启用</el-tag>
              <span class="flex-spacer" />
              <span v-if="testResult" class="muted">
                {{ testResult.rowCount }} 行 / {{ testResult.elapsedMs }}ms
                <el-tag v-if="testResult.truncated" size="small" type="warning">已截断</el-tag>
              </span>
            </div>

            <el-alert
              v-if="testError"
              :title="testError"
              type="error"
              show-icon
              :closable="true"
              class="test-result"
              @close="testError = ''"
            />

            <template v-if="testResult">
              <div class="result-tabs">
                <el-tabs v-model="resultTab" size="small">
                  <el-tab-pane :label="`结果 (${testResult.rowCount})`" name="rows" />
                  <el-tab-pane :label="`列信息 (${testResult.columns.length})`" name="columns" />
                  <el-tab-pane :label="`参数建议 (${testResult.paramSuggestions.length})`" name="params" />
                </el-tabs>
              </div>

              <div v-if="resultTab === 'rows'" class="table-scroll result-box">
                <el-table :data="testRows" border size="small" max-height="260">
                  <el-table-column
                    v-for="(c, i) in testResult.columns"
                    :key="c.name"
                    :label="c.name"
                    min-width="120"
                    show-overflow-tooltip
                  >
                    <template #default="{ row }">{{ row[i] ?? '' }}</template>
                  </el-table-column>
                </el-table>
              </div>

              <div v-else-if="resultTab === 'columns'" class="table-scroll result-box">
                <el-table :data="testResult.columns" border size="small" max-height="260">
                  <el-table-column prop="name" label="列名" min-width="140" />
                  <el-table-column prop="dbType" label="数据库类型" width="140" />
                  <el-table-column prop="logicalType" label="逻辑类型" width="110" />
                  <el-table-column label="可空" width="80">
                    <template #default="{ row }">{{ row.nullable ? '是' : '否' }}</template>
                  </el-table-column>
                </el-table>
              </div>

              <div v-else class="result-box">
                <div class="flex-gap" style="margin-bottom: 8px">
                  <el-button size="small" type="primary" @click="applySuggestedParams">
                    应用全部参数
                  </el-button>
                  <span class="muted">根据 SQL 中的占位符自动推断</span>
                </div>
                <div class="table-scroll">
                  <el-table :data="testResult.paramSuggestions" border size="small" max-height="220">
                    <el-table-column prop="name" label="参数名" width="150" />
                    <el-table-column prop="label" label="显示名" width="130" />
                    <el-table-column prop="type" label="类型" width="90" />
                    <el-table-column label="必填" width="70">
                      <template #default="{ row }">{{ row.required ? '是' : '否' }}</template>
                    </el-table-column>
                    <el-table-column prop="defaultValue" label="默认值" min-width="120" />
                    <el-table-column prop="reason" label="来源" width="100" />
                  </el-table>
                </div>
              </div>
            </template>

            <div class="muted sql-hint">
              仅允许单条 SELECT/WITH 查询；使用 <code v-pre>{{param_name}}</code> 声明参数，参数值以绑定方式传入，防止 SQL 注入。
            </div>
          </div>
        </el-form-item>
        <el-form-item label="参数">
          <div style="width: 100%">
            <div class="table-scroll">
              <el-table :data="form.params" size="small" border>
              <el-table-column label="参数名" width="170">
                <template #default="{ row }"><el-input v-model="row.name" size="small" placeholder="from_date" /></template>
              </el-table-column>
              <el-table-column label="显示名" width="150">
                <template #default="{ row }"><el-input v-model="row.label" size="small" /></template>
              </el-table-column>
              <el-table-column label="类型" width="130">
                <template #default="{ row }">
                  <el-select v-model="row.type" size="small" filterable>
                    <el-option label="字符串" value="string" />
                    <el-option label="数字" value="number" />
                    <el-option label="布尔" value="bool" />
                    <el-option label="日期" value="date" />
                    <el-option label="列表" value="list" />
                  </el-select>
                </template>
              </el-table-column>
              <el-table-column label="必填" width="70">
                <template #default="{ row }"><el-switch v-model="row.required" size="small" /></template>
              </el-table-column>
              <el-table-column label="默认值">
                <template #default="{ row }"><el-input v-model="row.defaultValue" size="small" /></template>
              </el-table-column>
              <el-table-column width="60">
                <template #default="{ $index }">
                  <el-button link type="danger" @click="form.params.splice($index, 1)">删除</el-button>
                </template>
              </el-table-column>
              </el-table>
            </div>
            <div class="flex-gap" style="margin-top: 8px">
              <el-button size="small" @click="addParam">添加参数</el-button>
              <el-button size="small" @click="autoParams">从 SQL 自动生成</el-button>
            </div>
          </div>
        </el-form-item>
        <el-form-item label="列信息">
          <div v-if="columns.length" class="columns">
            <el-tag v-for="c in columns" :key="c.name" effect="plain" type="info">
              {{ c.name }} <span class="muted">({{ c.logicalType }})</span>
            </el-tag>
          </div>
          <span v-else class="muted">保存后自动推断</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="aiDialog" title="AI 生成 SQL" :width="dialogWidth(620)" append-to-body>
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 12px">
        描述你想查询的内容，AI 会结合当前数据源的表结构生成只读 SQL，生成后自动执行测试并推断参数。
      </el-alert>
      <el-form label-position="top">
        <el-form-item label="需求描述">
          <el-input
            v-model="aiPrompt"
            type="textarea"
            :rows="4"
            placeholder="例如：按地区统计 2026 年每月销售额与利润，只要华东和华南，按月份升序"
          />
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="aiUseCurrent">在当前 SQL 基础上修改</el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="aiDialog = false">取消</el-button>
        <el-button type="primary" :loading="aiBusy === 'sql'" @click="aiGenerateSQL">生成并填入</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="previewVisible" title="数据集预览" width="90%" top="4vh">
      <div class="flex-gap" style="margin-bottom: 12px">
        <template v-for="p in previewParams" :key="p.name">
          <span class="muted">{{ p.label || p.name }}：</span>
          <el-date-picker v-if="p.type === 'date'" v-model="paramValues[p.name]" type="date" value-format="YYYY-MM-DD" placeholder="选择日期" />
          <el-input v-else v-model="paramValues[p.name]" :placeholder="p.name" style="width: 160px" />
        </template>
        <el-button type="primary" :loading="previewing" @click="runPreview">执行预览</el-button>
        <span v-if="previewResult" class="muted">
          共 {{ previewResult.rowCount }} 行{{ previewResult.truncated ? '（已截断）' : '' }}，耗时 {{ previewResult.elapsedMs }}ms
        </span>
      </div>
      <div class="table-scroll">
        <el-table v-if="previewResult" :data="previewRows" border size="small" max-height="560">
        <el-table-column v-for="(c, i) in previewResult.columns" :key="c.name" :label="c.name" min-width="130">
          <template #default="{ row }">{{ row[i] ?? '' }}</template>
        </el-table-column>
        </el-table>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { dialogWidth } from '@/utils/responsive'
import { ElMessage, ElMessageBox } from 'element-plus'
import { MagicStick, Plus, Promotion, VideoPlay } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { aiApi, dataSourceApi, datasetApi } from '@/api'
import type { ParamSuggestion, TestSQLResult } from '@/api'
import type { ColumnMeta, DataSource, Dataset, DatasetParam, ResultSet } from '@/types'

const list = ref<Dataset[]>([])
const dataSources = ref<DataSource[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const editing = ref<Dataset | null>(null)
const saving = ref(false)
const columns = ref<ColumnMeta[]>([])

const form = reactive({
  name: '',
  description: '',
  dataSourceId: 0 as number,
  sqlText: '',
  params: [] as DatasetParam[],
})

// ---------- SQL test / AI ----------
const sqlInputRef = ref<any>()
const testing = ref(false)
const testResult = ref<TestSQLResult | null>(null)
const testError = ref('')
const resultTab = ref('rows')
const testRows = computed(() => testResult.value?.rows || [])
const aiEnabled = ref(false)
const aiDialog = ref(false)
const aiPrompt = ref('')
const aiUseCurrent = ref(true)
const aiBusy = ref<'' | 'sql' | 'params' | 'meta' | 'complete'>('')
let cursorPos = 0

function rememberCursor() {
  const el = sqlInputRef.value?.textarea as HTMLTextAreaElement | undefined
  if (el && typeof el.selectionStart === 'number') {
    cursorPos = el.selectionStart
  }
}

/** Run the unsaved SQL against the selected datasource. */
async function runTest(silent = false) {
  if (!form.dataSourceId) {
    if (!silent) ElMessage.warning('请先选择数据源')
    return
  }
  if (!form.sqlText.trim()) {
    if (!silent) ElMessage.warning('请填写 SQL')
    return
  }
  testing.value = true
  testError.value = ''
  try {
    const paramValues: Record<string, string> = {}
    for (const p of form.params) {
      if (p.defaultValue) paramValues[p.name] = p.defaultValue
    }
    testResult.value = await aiApi.testSQL({
      dataSourceId: form.dataSourceId,
      sqlText: form.sqlText,
      params: form.params,
      paramValues,
      limit: 50,
    })
    resultTab.value = 'rows'
  } catch (e: any) {
    testResult.value = null
    testError.value = e?.response?.data?.message || '执行失败'
    if (!silent) ElMessage.error(testError.value)
  } finally {
    testing.value = false
  }
}

/** Generate parameter rows from {{placeholders}} without calling AI. */
async function autoParams() {
  if (!form.sqlText.trim()) {
    ElMessage.warning('请先填写 SQL')
    return
  }
  const suggestions = await aiApi.inferParams(form.sqlText, form.params)
  mergeSuggestions(suggestions, '已自动生成 %d 个参数')
}

function mergeSuggestions(suggestions: ParamSuggestion[], successMsg: string) {
  const byName = new Map(form.params.map((p) => [p.name.toLowerCase(), p]))
  let added = 0
  let updated = 0
  for (const sg of suggestions) {
    const key = sg.name.toLowerCase()
    const exist = byName.get(key)
    if (exist) {
      exist.label = sg.label || exist.label
      exist.type = sg.type as any
      exist.required = sg.required
      exist.defaultValue = sg.defaultValue
      updated++
    } else {
      form.params.push({
        name: sg.name, label: sg.label, type: sg.type as any,
        required: sg.required, defaultValue: sg.defaultValue,
      })
      added++
    }
  }
  if (added + updated === 0) {
    ElMessage.info('SQL 中没有找到参数占位符')
    return
  }
  ElMessage.success(successMsg.replace('%d', String(added + updated)))
}

function applySuggestedParams() {
  if (!testResult.value) return
  mergeSuggestions(testResult.value.paramSuggestions, '已应用 %d 个参数')
}

async function aiGenerateSQL() {
  if (!form.dataSourceId) {
    ElMessage.warning('请先选择数据源')
    return
  }
  if (!aiPrompt.value.trim()) {
    ElMessage.warning('请描述你的需求')
    return
  }
  aiBusy.value = 'sql'
  try {
    const res = await aiApi.generateSQL(
      form.dataSourceId,
      aiPrompt.value,
      aiUseCurrent.value ? form.sqlText : '',
    )
    form.sqlText = res.sqlText
    aiDialog.value = false
    await nextTick()
    await runTest(true)
    await autoParamsSilent()
  } finally {
    aiBusy.value = ''
  }
}

async function autoParamsSilent() {
  if (!form.sqlText.trim()) return
  try {
    const suggestions = await aiApi.inferParams(form.sqlText, form.params)
    mergeSuggestions(suggestions, '已生成 %d 个参数')
  } catch {
    // silent: the test run already surfaced the error
  }
}

async function aiParams() {
  if (!form.sqlText.trim()) {
    ElMessage.warning('请先填写 SQL')
    return
  }
  aiBusy.value = 'params'
  try {
    const suggestions = await aiApi.suggestParams(form.sqlText, form.params)
    mergeSuggestions(suggestions, 'AI 已补全 %d 个参数')
  } finally {
    aiBusy.value = ''
  }
}

async function aiMetadata() {
  if (!form.sqlText.trim()) {
    ElMessage.warning('请先填写 SQL')
    return
  }
  aiBusy.value = 'meta'
  try {
    const cols = testResult.value?.columns || columns.value
    const res = await aiApi.suggestMetadata(form.sqlText, cols)
    if (res.name) form.name = res.name
    if (res.description) form.description = res.description
    ElMessage.success('已填入名称与描述')
  } finally {
    aiBusy.value = ''
  }
}

async function aiComplete() {
  if (!form.dataSourceId) {
    ElMessage.warning('请先选择数据源')
    return
  }
  if (!form.sqlText.trim()) {
    ElMessage.warning('请先输入部分 SQL')
    return
  }
  aiBusy.value = 'complete'
  try {
    rememberCursor()
    const pos = cursorPos || form.sqlText.length
    const res = await aiApi.completeSQL(form.dataSourceId, form.sqlText, pos)
    if (res.completion) {
      const before = form.sqlText.slice(0, pos)
      const after = form.sqlText.slice(pos)
      const sep = before.endsWith(' ') || before.endsWith('\n') ? '' : ' '
      form.sqlText = before + sep + res.completion + after
      ElMessage.success('已补全')
    } else {
      ElMessage.info('AI 未返回可插入的内容')
    }
  } finally {
    aiBusy.value = ''
  }
}

function resetSqlState() {
  testResult.value = null
  testError.value = ''
  resultTab.value = 'rows'
  aiPrompt.value = ''
  cursorPos = 0
}

const previewVisible = ref(false)

const previewing = ref(false)
const previewResult = ref<ResultSet | null>(null)
const previewParams = ref<DatasetParam[]>([])
const paramValues = reactive<Record<string, string>>({})
const previewRows = computed(() => previewResult.value?.rows || [])

function dsName(id: number) {
  return dataSources.value.find((d) => d.id === id)?.name || `#${id}`
}

function fmt(t: string) {
  return t ? dayjs(t).format('YYYY-MM-DD HH:mm') : ''
}

async function load() {
  loading.value = true
  try {
    const [ds, sources] = await Promise.all([datasetApi.list(), dataSourceApi.list()])
    list.value = ds
    dataSources.value = sources
  } finally {
    loading.value = false
  }
}

function addParam() {
  form.params.push({ name: '', label: '', type: 'string', required: false, defaultValue: '' })
}

function openCreate() {
  editing.value = null
  resetSqlState()
  columns.value = []
  Object.assign(form, {
    name: '',
    description: '',
    dataSourceId: dataSources.value[0]?.id || 0,
    sqlText: '',
    params: [],
  })
  dialogVisible.value = true
}

function openEdit(ds: Dataset) {
  editing.value = ds
  resetSqlState()
  try {
    columns.value = JSON.parse(ds.columnMetaJson || '[]')
  } catch {
    columns.value = []
  }
  Object.assign(form, {
    name: ds.name,
    description: ds.description,
    dataSourceId: ds.dataSourceId,
    sqlText: ds.sqlText,
    params: (ds.params || []).map((p) => ({ ...p })),
  })
  dialogVisible.value = true
}

async function save() {
  if (!form.name || !form.dataSourceId || !form.sqlText) {
    ElMessage.warning('请填写名称、数据源和 SQL')
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await datasetApi.update(editing.value.id, { ...form })
    } else {
      await datasetApi.create({ ...form })
    }
    if (testResult.value) {
      columns.value = testResult.value.columns
    }
    ElMessage.success('已保存')
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

function openPreview(ds: Dataset) {
  editing.value = ds
  previewParams.value = ds.params || []
  for (const k of Object.keys(paramValues)) delete paramValues[k]
  for (const p of previewParams.value) paramValues[p.name] = p.defaultValue || ''
  previewResult.value = null
  previewVisible.value = true
}

async function runPreview() {
  if (!editing.value) return
  previewing.value = true
  try {
    previewResult.value = await datasetApi.preview(editing.value.id, { params: { ...paramValues }, limit: 100 })
  } finally {
    previewing.value = false
  }
}

async function remove(ds: Dataset) {
  await ElMessageBox.confirm(`确定删除数据集「${ds.name}」吗？引用它的图表将无法取数。`, '删除确认', { type: 'warning' })
  await datasetApi.remove(ds.id)
  ElMessage.success('已删除')
  await load()
}

onMounted(async () => {
  await load()
  try {
    const st = await aiApi.status()
    aiEnabled.value = st.enabled
  } catch {
    aiEnabled.value = false
  }
})
</script>

<style scoped>
.sql-block {
  width: 100%;
}
.sql-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 8px;
}
.flex-spacer {
  flex: 1 1 auto;
}
.sql-hint {
  margin-top: 6px;
  line-height: 1.6;
}
.test-result {
  margin-top: 8px;
}
.result-tabs {
  margin-top: 4px;
}
.result-tabs :deep(.el-tabs__header) {
  margin-bottom: 6px;
}
.result-box {
  border: 1px solid #ebeef5;
  border-radius: 6px;
  padding: 8px;
  background: #fafbfc;
}
.sql-input :deep(textarea) {
  font-family: 'Cascadia Code', Consolas, monospace;
  font-size: 13px;
}
.columns {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
</style>
