<!-- 数据源管理页：配置 MySQL / PostgreSQL / Oracle / SQL Server / SQLite 连接，支持测试连接与启用停用。 -->
<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  CheckCircle2,
  Database,
  Pencil,
  PlugZap,
  Plus,
  Search,
  Server,
  ShieldCheck,
  Trash2,
  XCircle,
} from 'lucide-vue-next'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, getErrorMessage } from '../../api/client'
import { formatDate } from '../../utils/format'
import EmptyBlock from '../../components/EmptyBlock.vue'

const DRIVERS = [
  { value: 'mysql', label: 'MySQL', className: 'tag-green' },
  { value: 'postgres', label: 'PostgreSQL', className: 'tag-blue' },
  { value: 'sqlserver', label: 'SQL Server', className: 'tag-amber' },
  { value: 'oracle', label: 'Oracle', className: 'tag-red' },
  { value: 'sqlite', label: 'SQLite', className: 'tag' },
]

const DSN_EXAMPLES = {
  mysql: 'user:pass@tcp(127.0.0.1:3306)/bi?charset=utf8mb4&parseTime=True&loc=Local',
  postgres: 'host=127.0.0.1 user=bi password=secret dbname=bi port=5432 sslmode=disable',
  sqlserver: 'sqlserver://user:pass@127.0.0.1:1433?database=bi',
  oracle: 'oracle://user:pass@127.0.0.1:1521/ORCLPDB1',
  sqlite: 'data/demo.db',
}

const items = ref([])
const loading = ref(true)
const saving = ref(false)
const testing = ref(false)
const keyword = ref('')
const testState = reactive({})
const dialogVisible = ref(false)
const editingId = ref(null)

const form = reactive({
  name: '',
  code: '',
  driver: 'mysql',
  dsn: '',
  status: 'active',
  read_only: true,
  max_open_conns: 10,
  max_idle_conns: 3,
  remark: '',
})

const filtered = computed(() => {
  const text = keyword.value.trim().toLowerCase()
  if (!text) return items.value
  return items.value.filter(
    (item) =>
      (item.name || '').toLowerCase().includes(text) ||
      (item.code || '').toLowerCase().includes(text) ||
      (item.driver || '').toLowerCase().includes(text),
  )
})

const activeCount = computed(() => items.value.filter((item) => item.status === 'active').length)
const testedCount = computed(() => items.value.filter((item) => item.last_test_ok).length)
const dsnExample = computed(() => DSN_EXAMPLES[form.driver] || '')

// 数据库驱动的展示名称与图标信息
function driverMeta(driver) {
  return DRIVERS.find((item) => item.value === driver) || { label: driver || '—', className: '' }
}

// 重置数据源表单
function resetForm() {
  editingId.value = null
  Object.assign(form, {
    name: '',
    code: '',
    driver: 'mysql',
    dsn: '',
    status: 'active',
    read_only: true,
    max_open_conns: 10,
    max_idle_conns: 3,
    remark: '',
  })
}

// 打开新建数据源弹窗
function openCreate() {
  resetForm()
  dialogVisible.value = true
}

// 打开编辑数据源弹窗
function openEdit(row) {
  resetForm()
  editingId.value = row.id
  Object.assign(form, {
    name: row.name || '',
    code: row.code || '',
    driver: row.driver || 'mysql',
    dsn: '',
    status: row.status || 'active',
    read_only: row.read_only !== false,
    max_open_conns: row.max_open_conns || 10,
    max_idle_conns: row.max_idle_conns || 3,
    remark: row.remark || '',
  })
  dialogVisible.value = true
}

// 加载数据源列表
async function load() {
  loading.value = true
  try {
    items.value = (await api.get('/admin/datasources')) || []
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

// 保存数据源配置（密码加密存储）
async function save() {
  if (!form.name.trim() || !form.code.trim()) {
    ElMessage.warning('请填写数据源名称和编码')
    return
  }
  if (!editingId.value && !form.dsn.trim()) {
    ElMessage.warning('请填写连接串 DSN')
    return
  }
  saving.value = true
  try {
    const payload = {
      name: form.name.trim(),
      code: form.code.trim(),
      driver: form.driver,
      dsn: form.dsn,
      status: form.status,
      read_only: form.read_only,
      max_open_conns: Number(form.max_open_conns) || 10,
      max_idle_conns: Number(form.max_idle_conns) || 3,
      remark: form.remark.trim(),
    }
    if (editingId.value) {
      await api.put(`/admin/datasources/${editingId.value}`, payload)
      ElMessage.success('数据源已更新')
    } else {
      await api.post('/admin/datasources', payload)
      ElMessage.success('数据源已创建')
    }
    dialogVisible.value = false
    await load()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    saving.value = false
  }
}

// 测试已保存数据源的连接
async function testConnection(row) {
  testState[row.id] = { loading: true }
  try {
    const result = await api.post(`/admin/datasources/${row.id}/test`, {})
    testState[row.id] = { loading: false, ...result }
    if (result.ok) ElMessage.success(`连接成功，耗时 ${result.latency_ms}ms`)
    else ElMessage.error(result.message || '连接失败')
    await load()
  } catch (error) {
    testState[row.id] = { loading: false, ok: false, message: getErrorMessage(error) }
    ElMessage.error(getErrorMessage(error))
  }
}

// 测试表单中尚未保存的连接参数
async function testForm() {
  if (!form.driver) {
    ElMessage.warning('请选择驱动')
    return
  }
  if (!editingId.value && !form.dsn.trim()) {
    ElMessage.warning('请填写连接串 DSN')
    return
  }
  testing.value = true
  try {
    const result = await api.post(`/admin/datasources/${editingId.value || 0}/test`, {
      driver: form.driver,
      dsn: form.dsn,
    })
    if (result.ok) {
      ElMessage.success(`连接成功，耗时 ${result.latency_ms}ms`)
    } else {
      ElMessage.error(result.message || '连接失败')
    }
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    testing.value = false
  }
}

// 删除数据源
async function remove(row) {
  try {
    await ElMessageBox.confirm(`确认删除数据源「${row.name}」？引用该数据源的数据集将无法查询。`, '删除数据源', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
    await api.delete(`/admin/datasources/${row.id}`)
    ElMessage.success('数据源已删除')
    await load()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(getErrorMessage(error))
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="metric-mini-grid">
      <div class="metric-mini">
        <span>数据源</span>
        <strong>{{ items.length }}</strong>
      </div>
      <div class="metric-mini">
        <span>启用中</span>
        <strong>{{ activeCount }}</strong>
      </div>
      <div class="metric-mini">
        <span>连接正常</span>
        <strong>{{ testedCount }}</strong>
      </div>
      <div class="metric-mini">
        <span>支持驱动</span>
        <strong>5</strong>
        <small>MySQL / PG / Oracle / SQLServer / SQLite</small>
      </div>
    </div>

    <div class="admin-layout">
      <div class="admin-main">
        <div class="panel">
          <div class="table-toolbar">
            <div class="table-toolbar-left">
              <div class="search-box">
                <Search :size="14" />
                <input v-model="keyword" type="search" placeholder="搜索名称、编码或驱动" />
              </div>
              <span class="tag">{{ filtered.length }} 个数据源</span>
            </div>
            <button class="btn btn-primary btn-sm" type="button" @click="openCreate">
              <Plus :size="14" /> 新建数据源
            </button>
          </div>

          <div v-if="loading" style="padding: 20px">
            <div class="skeleton" style="height: 300px" />
          </div>

          <div v-else-if="filtered.length" class="dataset-grid" style="padding: 18px">
            <article v-for="item in filtered" :key="item.id" class="dataset-card">
              <div class="dataset-card-head">
                <div class="dataset-icon"><Server :size="17" /></div>
                <div class="inline-actions">
                  <span class="tag" :class="driverMeta(item.driver).className">{{ driverMeta(item.driver).label }}</span>
                  <span class="tag" :class="item.status === 'active' ? 'tag-green' : 'tag-red'">
                    {{ item.status === 'active' ? '启用' : '停用' }}
                  </span>
                </div>
              </div>
              <h3>{{ item.name }}</h3>
              <p>{{ item.remark || '未填写备注' }}</p>
              <div>
                <span class="dataset-code mono">{{ item.code }}</span>
                <span v-if="item.read_only" class="tag tag-green" style="margin-left: 6px">
                  <ShieldCheck :size="10" /> 只读
                </span>
              </div>
              <div
                class="mono"
                style="margin-top: 10px; padding: 8px 10px; border-radius: 8px; background: var(--surface-soft); color: var(--muted); font-size: 10.5px; word-break: break-all"
              >
                {{ item.dsn_masked || '—' }}
              </div>
              <div style="margin-top: 8px; display: flex; align-items: center; gap: 6px; font-size: 11px">
                <template v-if="testState[item.id]?.loading">
                  <PlugZap :size="12" /> 测试中…
                </template>
                <template v-else-if="testState[item.id]">
                  <CheckCircle2 v-if="testState[item.id].ok" :size="12" style="color: #0f766e" />
                  <XCircle v-else :size="12" style="color: var(--rose)" />
                  <span :style="{ color: testState[item.id].ok ? '#0f766e' : 'var(--rose)' }">
                    {{ testState[item.id].message }} · {{ testState[item.id].latency_ms }}ms
                  </span>
                </template>
                <template v-else-if="item.last_test_at">
                  <CheckCircle2 v-if="item.last_test_ok" :size="12" style="color: #0f766e" />
                  <XCircle v-else :size="12" style="color: var(--rose)" />
                  <span :style="{ color: item.last_test_ok ? '#0f766e' : 'var(--rose)' }">
                    {{ item.last_test_msg || (item.last_test_ok ? '连接成功' : '连接失败') }}
                    · {{ formatDate(item.last_test_at, true) }}
                  </span>
                </template>
                <template v-else>
                  <span style="color: var(--muted-light)">尚未测试连接</span>
                </template>
              </div>
              <div class="dataset-foot">
                <span>连接池 {{ item.max_open_conns }} / {{ item.max_idle_conns }}</span>
                <div class="inline-actions">
                  <button class="btn btn-ghost btn-sm" type="button" :disabled="testState[item.id]?.loading" @click="testConnection(item)">
                    <PlugZap :size="13" /> 测试
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
              title="还没有业务数据源"
              description="默认使用平台主库；也可以接入 MySQL、PostgreSQL、Oracle、SQL Server 或 SQLite 只读业务库。"
            >
              <button class="btn btn-primary btn-sm" type="button" @click="openCreate">
                <Plus :size="14" /> 新建数据源
              </button>
            </EmptyBlock>
          </div>
        </div>
      </div>

      <aside class="admin-aside">
        <div class="scope-card">
          <h3><ShieldCheck :size="13" style="vertical-align: -2px" /> 连接串安全</h3>
          <p>
            DSN 使用 AES-GCM 加密后入库，接口仅返回掩码。修改 JWT_SECRET 会导致已保存的 DSN 无法解密，
            请提前备份或在数据源页面重新填写。
          </p>
        </div>
        <div class="scope-card">
          <h3><Database :size="13" style="vertical-align: -2px" /> 生产建议</h3>
          <p>为 BI 单独创建只读账号，只授权业务明细视图，并限制来源 IP 与最大连接数。</p>
        </div>
        <div class="scope-card">
          <h3><PlugZap :size="13" style="vertical-align: -2px" /> DSN 示例</h3>
          <p class="mono" style="word-break: break-all">MySQL：{{ DSN_EXAMPLES.mysql }}</p>
          <p class="mono" style="word-break: break-all; margin-top: 6px">PostgreSQL：{{ DSN_EXAMPLES.postgres }}</p>
          <p class="mono" style="word-break: break-all; margin-top: 6px">Oracle：{{ DSN_EXAMPLES.oracle }}</p>
          <p class="mono" style="word-break: break-all; margin-top: 6px">SQL Server：{{ DSN_EXAMPLES.sqlserver }}</p>
          <p class="mono" style="word-break: break-all; margin-top: 6px">SQLite：{{ DSN_EXAMPLES.sqlite }}</p>
          <p style="margin-top: 8px">
            Oracle 也可以使用服务名参数形式：<span class="mono">oracle://user:pass@127.0.0.1:1521/?service_name=ORCLPDB1</span>。
            SQL 分页会自动改写为 <span class="mono">ROWNUM</span> 语法，无需手写 LIMIT。
          </p>
        </div>
      </aside>
    </div>

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑数据源' : '新建数据源'" width="680px" destroy-on-close>
      <div class="form-grid">
        <label class="field-label">
          数据源名称
          <el-input v-model="form.name" maxlength="100" placeholder="例如：门店业务库" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          编码
          <el-input v-model="form.code" maxlength="64" placeholder="例如：store_biz" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          驱动
          <el-select v-model="form.driver" style="width: 100%; margin-top: 8px">
            <el-option v-for="item in DRIVERS" :key="item.value" :value="item.value" :label="item.label" />
          </el-select>
        </label>
        <label class="field-label">
          状态
          <el-select v-model="form.status" style="width: 100%; margin-top: 8px">
            <el-option value="active" label="启用" />
            <el-option value="disabled" label="停用" />
          </el-select>
        </label>
        <label class="field-label form-span-2">
          连接串 DSN
          <el-input
            v-model="form.dsn"
            type="textarea"
            :rows="3"
            class="mono"
            :placeholder="editingId ? '留空表示不修改，当前连接串保持不变' : dsnExample"
            style="margin-top: 8px"
          />
          <span style="display: block; margin-top: 6px; color: var(--muted-light); font-size: 11px">
            示例：<span v-text="dsnExample" />
          </span>
        </label>
        <label class="field-label">
          最大连接数
          <el-input-number v-model="form.max_open_conns" :min="1" :max="200" style="width: 100%; margin-top: 8px" />
        </label>
        <label class="field-label">
          空闲连接数
          <el-input-number v-model="form.max_idle_conns" :min="1" :max="100" style="width: 100%; margin-top: 8px" />
        </label>
        <div class="field-label">
          只读数据源
          <div class="scope-note" style="margin-top: 8px; width: 100%">
            <el-switch v-model="form.read_only" />
            <span>{{ form.read_only ? '仅用于查询' : '允许写入（不推荐）' }}</span>
          </div>
        </div>
        <label class="field-label">
          备注
          <el-input v-model="form.remark" maxlength="255" placeholder="可选" style="margin-top: 8px" />
        </label>
      </div>
      <template #footer>
        <div class="inline-actions" style="justify-content: flex-end">
          <button class="btn btn-secondary" type="button" :disabled="testing" @click="testForm">
            <PlugZap :size="13" /> {{ testing ? '测试中…' : '测试连接' }}
          </button>
          <button class="btn btn-secondary" type="button" @click="dialogVisible = false">取消</button>
          <button class="btn btn-primary" type="button" :disabled="saving" @click="save">
            {{ saving ? '保存中…' : '保存' }}
          </button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>
