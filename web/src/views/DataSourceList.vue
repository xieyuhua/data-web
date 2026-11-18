<template>
  <div class="page-card">
    <div class="page-header">
      <span class="page-title">数据源</span>
      <el-button type="primary" :icon="Plus" @click="openCreate">新建数据源</el-button>
    </div>

    <div class="table-scroll">
      <el-table :data="list" v-loading="loading" border stripe>
      <el-table-column prop="name" label="名称" min-width="140" />
      <el-table-column prop="type" label="类型" width="110">
        <template #default="{ row }">
          <el-tag effect="plain">{{ typeLabel(row.type) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="连接信息" min-width="240">
        <template #default="{ row }">
          <span class="muted">{{ connectionText(row) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="150">
        <template #default="{ row }">
          <el-tag v-if="row.lastTestStatus === 'success'" type="success" size="small">连接正常</el-tag>
          <el-tag v-else-if="row.lastTestStatus === 'failed'" type="danger" size="small">连接失败</el-tag>
          <span v-else class="muted">未测试</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="230" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="testSaved(row)">测试</el-button>
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="dialogVisible" :title="editing ? '编辑数据源' : '新建数据源'" :width="dialogWidth(560)" destroy-on-close>
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="例如：销售库" />
        </el-form-item>
        <el-form-item label="类型" prop="type">
          <el-radio-group v-model="form.type" @change="onTypeChange">
            <el-radio-button value="mysql">MySQL</el-radio-button>
            <el-radio-button value="postgres">PostgreSQL</el-radio-button>
            <el-radio-button value="oracle">Oracle</el-radio-button>
            <el-radio-button value="sqlite">SQLite</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <template v-if="form.type !== 'sqlite'">
          <el-form-item label="主机">
            <el-input v-model="form.host" placeholder="127.0.0.1" />
          </el-form-item>
          <el-form-item label="端口">
            <el-input-number v-model="form.port" :min="0" :max="65535" controls-position="right" style="width: 180px" />
          </el-form-item>
        </template>
        <el-form-item :label="form.type === 'sqlite' ? '文件路径' : form.type === 'oracle' ? '服务名 / SID' : '数据库名'">
          <el-input v-model="form.database" :placeholder="form.type === 'sqlite' ? 'data/sample.db' : form.type === 'oracle' ? 'ORCLPDB1' : 'sales'" />
        </el-form-item>
        <template v-if="form.type !== 'sqlite'">
          <el-form-item label="用户名">
            <el-input v-model="form.username" />
          </el-form-item>
          <el-form-item label="密码">
            <el-input v-model="form.password" type="password" show-password :placeholder="editing ? '留空则保持不变' : ''" />
          </el-form-item>
        </template>
        <el-form-item label="附加参数">
          <el-input v-model="form.dsnExtra" placeholder="可选，例如 sslmode=require 或 parseTime=true" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button :loading="testing" @click="testForm">测试连接</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { dialogWidth } from '@/utils/responsive'
import { ElMessage, ElMessageBox, type FormInstance } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { dataSourceApi } from '@/api'
import type { DataSource } from '@/types'

const list = ref<DataSource[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const editing = ref<DataSource | null>(null)
const saving = ref(false)
const testing = ref(false)
const formRef = ref<FormInstance>()

const form = reactive({
  name: '',
  type: 'mysql',
  host: '127.0.0.1',
  port: 3306,
  database: '',
  username: '',
  password: '',
  dsnExtra: '',
})

const rules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择类型', trigger: 'change' }],
}

function typeLabel(t: string) {
  return { postgres: 'PostgreSQL', sqlite: 'SQLite', oracle: 'Oracle', mysql: 'MySQL' }[t] || t
}

function connectionText(ds: DataSource) {
  if (ds.type === 'sqlite') return ds.database
  return `${ds.host}:${ds.port}/${ds.database}`
}

async function load() {
  loading.value = true
  try {
    list.value = await dataSourceApi.list()
  } finally {
    loading.value = false
  }
}

function resetForm() {
  Object.assign(form, {
    name: '',
    type: 'mysql',
    host: '127.0.0.1',
    port: 3306,
    database: '',
    username: '',
    password: '',
    dsnExtra: '',
  })
}

function openCreate() {
  editing.value = null
  resetForm()
  dialogVisible.value = true
}

function openEdit(ds: DataSource) {
  editing.value = ds
  Object.assign(form, {
    name: ds.name,
    type: ds.type,
    host: ds.host,
    port: ds.port,
    database: ds.database,
    username: ds.username,
    password: '',
    dsnExtra: ds.dsnExtra || '',
  })
  dialogVisible.value = true
}

function payload() {
  const p: any = {
    name: form.name,
    type: form.type,
    host: form.type === 'sqlite' ? '' : form.host,
    port: form.type === 'sqlite' ? 0 : form.port,
    database: form.database,
    username: form.type === 'sqlite' ? '' : form.username,
    password: form.type === 'sqlite' ? '' : form.password,
    dsnExtra: form.dsnExtra,
  }
  if (!p.password) delete p.password
  return p
}

const defaultPorts: Record<string, number> = { mysql: 3306, postgres: 5432, oracle: 1521 }

function onTypeChange(t: string) {
  const p = defaultPorts[t]
  if (p) form.port = p
}

async function testForm() {
  testing.value = true
  try {
    const res = await dataSourceApi.test(payload())
    ElMessage.success(res.message || '连接成功')
  } finally {
    testing.value = false
  }
}

async function save() {
  await formRef.value?.validate()
  saving.value = true
  try {
    if (editing.value) {
      await dataSourceApi.update(editing.value.id, payload())
    } else {
      await dataSourceApi.create(payload())
    }
    ElMessage.success('已保存')
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

async function testSaved(ds: DataSource) {
  const res = await dataSourceApi.testById(ds.id)
  ElMessage.success(res.message || '连接成功')
  await load()
}

async function remove(ds: DataSource) {
  await ElMessageBox.confirm(`确定删除数据源「${ds.name}」吗？引用它的数据集将无法执行。`, '删除确认', { type: 'warning' })
  await dataSourceApi.remove(ds.id)
  ElMessage.success('已删除')
  await load()
}

onMounted(load)
</script>
