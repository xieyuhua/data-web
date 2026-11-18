<template>
  <div class="page-card">
    <div class="page-header">
      <span class="page-title">用户管理</span>
      <div class="flex-gap">
        <el-input v-model="keyword" placeholder="搜索用户名/姓名/邮箱" clearable style="width: 220px" @keyup.enter="load" @clear="load" />
        <el-select v-model="deptFilter" clearable placeholder="全部部门" style="width: 180px" @change="load" filterable>
          <el-option
            v-for="d in departments"
            :key="d.wecomId"
            :label="`${'　'.repeat(d.depth)}${d.name}`"
            :value="d.wecomId"
          />
        </el-select>
        <el-select v-model="sourceFilter" clearable placeholder="全部来源" style="width: 130px" @change="load" filterable>
          <el-option label="企业微信" value="wecom" />
          <el-option label="本地账号" value="local" />
        </el-select>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建用户</el-button>
      </div>
    </div>

    <div class="table-scroll">
      <el-table :data="list" v-loading="loading" border stripe>
      <el-table-column prop="username" label="用户名" min-width="130" />
      <el-table-column prop="displayName" label="姓名" min-width="130" />
      <el-table-column prop="email" label="邮箱" min-width="180" show-overflow-tooltip />
      <el-table-column label="来源" width="110">
        <template #default="{ row }">
          <el-tag v-if="row.source === 'wecom'" size="small" type="primary" effect="plain">企业微信</el-tag>
          <el-tag v-else size="small" type="info" effect="plain">本地</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="部门" min-width="170">
        <template #default="{ row }">
          <template v-if="deptNames(row).length">
            <el-tag v-for="n in deptNames(row)" :key="n" size="small" effect="plain" class="dept-tag">{{ n }}</el-tag>
          </template>
          <span v-else class="muted">-</span>
        </template>
      </el-table-column>
      <el-table-column prop="position" label="职位" min-width="130">
        <template #default="{ row }">{{ row.position || '-' }}</template>
      </el-table-column>
      <el-table-column label="角色" width="110">
        <template #default="{ row }">
          <el-tag :type="row.role === 'admin' ? 'danger' : 'info'" effect="plain">
            {{ row.role === 'admin' ? '管理员' : '普通用户' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-switch :model-value="row.enabled" :disabled="row.id === auth.user?.id" @change="(v: boolean) => toggleEnabled(row, v)" />
        </template>
      </el-table-column>
      <el-table-column label="最近登录" width="170">
        <template #default="{ row }">{{ row.lastLoginAt ? fmt(row.lastLoginAt) : '从未' }}</template>
      </el-table-column>
      <el-table-column label="操作" width="220" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button v-if="row.source !== 'wecom'" link type="primary" @click="openReset(row)">重置密码</el-button>
          <el-button link type="danger" :disabled="row.id === auth.user?.id" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
      </el-table>
    </div>

    <el-pagination
      style="margin-top: 12px"
      layout="total, prev, pager, next"
      :total="total"
      :page-size="pageSize"
      :current-page="page"
      @current-change="(p: number) => { page = p; load() }"
    />

    <el-dialog v-model="dialogVisible" :title="editing ? '编辑用户' : '新建用户'" :width="dialogWidth(460)" destroy-on-close>
      <el-form label-width="80px">
        <el-form-item label="用户名">
          <el-input v-model="form.username" :disabled="!!editing" />
        </el-form-item>
        <el-form-item v-if="!editing" label="密码">
          <el-input v-model="form.password" type="password" show-password placeholder="至少 8 位" />
        </el-form-item>
        <el-form-item label="姓名">
          <el-input v-model="form.displayName" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="form.email" />
        </el-form-item>
        <el-form-item label="角色">
          <el-radio-group v-model="form.role">
            <el-radio-button value="user">普通用户</el-radio-button>
            <el-radio-button value="admin">管理员</el-radio-button>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="resetVisible" title="重置密码" :width="dialogWidth(420)" destroy-on-close>
      <el-form label-width="90px">
        <el-form-item label="新密码">
          <el-input v-model="resetPassword" type="password" show-password placeholder="至少 8 位" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="resetVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="doReset">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { dialogWidth } from '@/utils/responsive'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { userApi, wecomApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import type { User } from '@/types'

const auth = useAuthStore()
const list = ref<User[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const keyword = ref('')
const deptFilter = ref<number | null>(null)
const sourceFilter = ref<string>('')
const departments = ref<Array<{ wecomId: number; name: string; parentWecomId: number; depth: number }>>([])
const loading = ref(false)

const deptNameByID = computed(() => {
  const m = new Map<number, string>()
  for (const d of departments.value) m.set(d.wecomId, d.name)
  return m
})

function deptNames(row: any): string[] {
  const raw = row.wecomDeptIds
  if (!raw) return []
  try {
    const ids: number[] = JSON.parse(raw)
    return ids.map((id) => deptNameByID.value.get(id) || `#${id}`)
  } catch {
    return []
  }
}

const dialogVisible = ref(false)
const editing = ref<User | null>(null)
const saving = ref(false)
const form = reactive({ username: '', password: '', displayName: '', email: '', role: 'user' })

const resetVisible = ref(false)
const resetPassword = ref('')
const resetTarget = ref<User | null>(null)

function fmt(t: string) {
  return dayjs(t).format('YYYY-MM-DD HH:mm')
}

async function load() {
  loading.value = true
  try {
    const res = await userApi.list({ page: page.value, pageSize: 200, keyword: keyword.value })
    let items = res.items
    if (sourceFilter.value) items = items.filter((u: any) => (u.source || 'local') === sourceFilter.value)
    if (deptFilter.value) {
      items = items.filter((u: any) => {
        if (!u.wecomDeptIds) return false
        try {
          return (JSON.parse(u.wecomDeptIds) as number[]).includes(deptFilter.value!)
        } catch {
          return false
        }
      })
    }
    total.value = items.length
    list.value = items.slice((page.value - 1) * pageSize, page.value * pageSize)
  } finally {
    loading.value = false
  }
}

async function loadDepartments() {
  try {
    departments.value = await wecomApi.departments()
  } catch {
    departments.value = []
  }
}

function openCreate() {
  editing.value = null
  Object.assign(form, { username: '', password: '', displayName: '', email: '', role: 'user' })
  dialogVisible.value = true
}

function openEdit(u: User) {
  editing.value = u
  Object.assign(form, { username: u.username, password: '', displayName: u.displayName, email: u.email, role: u.role })
  dialogVisible.value = true
}

async function save() {
  if (!form.username || (!editing.value && !form.password)) {
    ElMessage.warning('请填写用户名和密码')
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await userApi.update(editing.value.id, {
        displayName: form.displayName,
        email: form.email,
        role: form.role,
      } as any)
    } else {
      await userApi.create({ ...form } as any)
    }
    ElMessage.success('已保存')
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

function openReset(u: User) {
  resetTarget.value = u
  resetPassword.value = ''
  resetVisible.value = true
}

async function doReset() {
  if (!resetTarget.value || resetPassword.value.length < 8) {
    ElMessage.warning('密码至少 8 位')
    return
  }
  saving.value = true
  try {
    await userApi.resetPassword(resetTarget.value.id, resetPassword.value)
    ElMessage.success('密码已重置')
    resetVisible.value = false
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(u: User, enabled: boolean) {
  await userApi.update(u.id, { enabled } as any)
  u.enabled = enabled
  ElMessage.success(enabled ? '已启用' : '已禁用')
}

async function remove(u: User) {
  await ElMessageBox.confirm(`确定删除用户「${u.username}」吗？`, '删除确认', { type: 'warning' })
  await userApi.remove(u.id)
  ElMessage.success('已删除')
  await load()
}

onMounted(async () => {
  await loadDepartments()
  await load()
})
</script>
