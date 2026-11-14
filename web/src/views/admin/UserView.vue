<!-- 用户与角色管理页：维护账号、角色与组织数据范围，支持通讯录导入导出。 -->
<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  Download,
  FileSpreadsheet,
  KeyRound,
  MessageCircle,
  Pencil,
  Plus,
  Search,
  ShieldCheck,
  Trash2,
  Upload,
  Users,
} from 'lucide-vue-next'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, getErrorMessage } from '../../api/client'
import { useAuthStore } from '../../stores/auth'
import { formatDate, statusMeta } from '../../utils/format'
import { dataScopeLabel, flattenOrgs } from '../../utils/org'
import EmptyBlock from '../../components/EmptyBlock.vue'
import OrgTreeSelect from '../../components/OrgTreeSelect.vue'

const auth = useAuthStore()

const users = ref([])
const roles = ref([])
const orgTree = ref([])
const loading = ref(true)
const saving = ref(false)
const importing = ref(false)
const fileInput = ref(null)
const keyword = ref('')
const roleFilter = ref('')

const dialogVisible = ref(false)
const editingId = ref(null)
const form = reactive({
  name: '',
  username: '',
  password: '',
  phone: '',
  email: '',
  role_code: '',
  organization_id: null,
  wecom_user_id: '',
  wechat_open_id: '',
  status: 'active',
})

const orgs = computed(() => flattenOrgs(orgTree.value))
const orgMap = computed(() => Object.fromEntries(orgs.value.map((item) => [item.id, item])))
const roleMap = computed(() => Object.fromEntries(roles.value.map((item) => [item.code, item])))
const activeCount = computed(() => users.value.filter((item) => item.status === 'active').length)
const wecomCount = computed(() => users.value.filter((item) => item.wecom_user_id).length)
const wechatCount = computed(() => users.value.filter((item) => item.wechat_open_id).length)

// 重置用户表单
function resetForm() {
  editingId.value = null
  Object.assign(form, {
    name: '',
    username: '',
    password: '',
    phone: '',
    email: '',
    role_code: roles.value[0]?.code || '',
    organization_id: null,
    wecom_user_id: '',
    wechat_open_id: '',
    status: 'active',
  })
}

// 打开新建用户弹窗
function openCreate() {
  resetForm()
  dialogVisible.value = true
}

// 打开编辑用户弹窗
function openEdit(row) {
  resetForm()
  editingId.value = row.id
  Object.assign(form, {
    name: row.name || '',
    username: row.username || '',
    password: '',
    phone: row.phone || '',
    email: row.email || '',
    role_code: row.role_code || '',
    organization_id: row.organization_id || null,
    wecom_user_id: row.wecom_user_id || '',
    wechat_open_id: row.wechat_open_id || '',
    status: row.status || 'active',
  })
  dialogVisible.value = true
}

// 加载用户列表
async function loadUsers() {
  loading.value = true
  try {
    const params = new URLSearchParams()
    if (keyword.value.trim()) params.set('keyword', keyword.value.trim())
    if (roleFilter.value) params.set('role', roleFilter.value)
    const query = params.toString()
    users.value = (await api.get(`/users${query ? `?${query}` : ''}`)) || []
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

// 加载用户与角色数据
async function load() {
  loading.value = true
  try {
    const [roleList, tree] = await Promise.all([
      api.get('/roles'),
      api.get('/orgs/tree'),
    ])
    roles.value = roleList || []
    orgTree.value = tree || []
    await loadUsers()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

// 保存用户（含角色与组织数据范围）
async function save() {
  if (!form.name.trim() || !form.username.trim()) {
    ElMessage.warning('请填写姓名和用户名')
    return
  }
  if (!form.role_code) {
    ElMessage.warning('请选择角色')
    return
  }
  if (!form.organization_id) {
    ElMessage.warning('请选择所属组织')
    return
  }
  if (!editingId.value && (form.password || '').length < 6) {
    ElMessage.warning('新建用户密码至少 6 位')
    return
  }
  if (editingId.value && form.password && form.password.length < 6) {
    ElMessage.warning('密码至少 6 位')
    return
  }
  saving.value = true
  try {
    const payload = {
      name: form.name.trim(),
      username: form.username.trim(),
      password: form.password,
      phone: form.phone.trim(),
      email: form.email.trim(),
      role_code: form.role_code,
      organization_id: Number(form.organization_id),
      wecom_user_id: form.wecom_user_id.trim(),
      wechat_open_id: form.wechat_open_id.trim(),
      status: form.status,
    }
    if (editingId.value) {
      await api.put(`/users/${editingId.value}`, payload)
      ElMessage.success('用户信息已更新')
    } else {
      await api.post('/users', payload)
      ElMessage.success('用户已创建')
    }
    dialogVisible.value = false
    await loadUsers()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    saving.value = false
  }
}

// 删除用户
async function remove(row) {
  if (row.id === auth.user?.id) {
    ElMessage.warning('不能删除当前登录账号')
    return
  }
  try {
    await ElMessageBox.confirm(`确认删除用户「${row.name}」？删除后该账号将无法登录。`, '删除用户', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
    await api.delete(`/users/${row.id}`)
    ElMessage.success('用户已删除')
    await loadUsers()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(getErrorMessage(error))
  }
}

// 组织 ID 转组织名称
function orgName(id) {
  return orgMap.value[id]?.name || '—'
}

// 角色码转角色名称
function roleName(code) {
  return roleMap.value[code]?.name || code || '—'
}

// 取姓名前两个字作为头像文字
function initials(name) {
  return (name || '用户').slice(-2)
}

// 按关键字搜索用户
function onSearch() {
  loadUsers()
}

// 导出通讯录（Excel）
async function exportContacts() {
  try {
    await api.download('/users/export', `通讯录-${new Date().toISOString().slice(0, 10)}.xlsx`)
    ElMessage.success('通讯录已导出')
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  }
}

// 下载通讯录导入模板
async function downloadTemplate() {
  try {
    await api.download('/users/template', '通讯录导入模板.xlsx')
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  }
}

// 触发导入文件选择
function pickImport() {
  fileInput.value?.click()
}

// 导入通讯录并展示结果
async function onImportChange(event) {
  const file = event.target?.files?.[0]
  if (event.target) event.target.value = ''
  if (!file) return
  importing.value = true
  try {
    const result = await api.upload('/users/import', file)
    ElMessage.success(
      `导入完成：新增 ${result?.created || 0} 个，更新 ${result?.updated || 0} 个，` +
        `使用默认密码 ${result?.default_password || 0} 个，跳过 ${result?.skipped || 0} 个`,
    )
    await loadUsers()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    importing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="metric-mini-grid">
      <div class="metric-mini">
        <span>账号总数</span>
        <strong>{{ users.length }}</strong>
      </div>
      <div class="metric-mini">
        <span>启用账号</span>
        <strong>{{ activeCount }}</strong>
      </div>
      <div class="metric-mini">
        <span>企业微信绑定</span>
        <strong>{{ wecomCount }}</strong>
      </div>
      <div class="metric-mini">
        <span>微信绑定</span>
        <strong>{{ wechatCount }}</strong>
      </div>
    </div>

    <div class="admin-layout">
      <div class="admin-main">
        <div class="panel">
          <div class="table-toolbar">
            <div class="table-toolbar-left">
              <div class="search-box">
                <Search :size="14" />
                <input
                  v-model="keyword"
                  type="search"
                  placeholder="搜索姓名、用户名"
                  @keyup.enter="onSearch"
                />
              </div>
              <el-select
                v-model="roleFilter"
                placeholder="全部角色"
                clearable
                size="small"
                style="width: 150px"
                @change="onSearch"
              >
                <el-option v-for="role in roles" :key="role.code" :value="role.code" :label="role.name" />
              </el-select>
              <button class="btn btn-secondary btn-sm" type="button" @click="onSearch">查询</button>
            </div>
            <div class="inline-actions">
              <input
                ref="fileInput"
                type="file"
                accept=".xlsx,.xls"
                style="display: none"
                @change="onImportChange"
              />
              <button class="btn btn-secondary btn-sm" type="button" @click="downloadTemplate">
                <FileSpreadsheet :size="13" /> 模板
              </button>
              <button class="btn btn-secondary btn-sm" type="button" :disabled="importing" @click="pickImport">
                <Upload :size="13" /> {{ importing ? '导入中…' : '导入通讯录' }}
              </button>
              <button class="btn btn-secondary btn-sm" type="button" @click="exportContacts">
                <Download :size="13" /> 导出通讯录
              </button>
              <button class="btn btn-primary btn-sm" type="button" @click="openCreate">
                <Plus :size="14" /> 新建用户
              </button>
            </div>
          </div>

          <div v-if="loading" style="padding: 20px">
            <div class="skeleton" style="height: 320px" />
          </div>

          <el-table v-else :data="users" style="width: 100%">
            <el-table-column label="用户" min-width="168">
              <template #default="{ row }">
                <div class="user-cell">
                  <span class="avatar">{{ initials(row.name) }}</span>
                  <div>
                    <strong>{{ row.name }}</strong>
                    <span>@{{ row.username }}</span>
                  </div>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="角色" width="104">
              <template #default="{ row }">
                <span class="tag tag-green">{{ roleName(row.role_code) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="所属组织" min-width="118">
              <template #default="{ row }">
                <div class="org-name-cell" style="font-weight: 500">{{ orgName(row.organization_id) }}</div>
                <span style="color: var(--muted-light); font-size: 10.5px">
                  {{ orgMap[row.organization_id]?.code || '' }}
                </span>
              </template>
            </el-table-column>
            <el-table-column label="消息绑定" min-width="158">
              <template #default="{ row }">
                <div class="bind-cell" style="display: flex; flex-direction: column; gap: 3px">
                  <span class="tag" :class="row.wecom_user_id ? 'tag-green' : ''">
                    企微 {{ row.wecom_user_id || '未绑定' }}
                  </span>
                  <span class="tag" :class="row.wechat_open_id ? 'tag-green' : ''">
                    微信 {{ row.wechat_open_id || '未绑定' }}
                  </span>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="66">
              <template #default="{ row }">
                <span class="tag" :class="statusMeta(row.status).className">
                  {{ statusMeta(row.status).label }}
                </span>
              </template>
            </el-table-column>
            <el-table-column label="最后登录" width="126">
              <template #default="{ row }">
                <span class="login-cell">
                  {{ row.last_login_at ? formatDate(row.last_login_at, true) : '从未登录' }}
                </span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="118" fixed="right">
              <template #default="{ row }">
                <div class="inline-actions">
                  <button class="btn btn-ghost btn-sm" type="button" @click="openEdit(row)">
                    <Pencil :size="13" /> 编辑
                  </button>
                  <button
                    class="btn btn-ghost btn-sm"
                    type="button"
                    style="color: var(--rose)"
                    :disabled="row.id === auth.user?.id"
                    @click="remove(row)"
                  >
                    <Trash2 :size="13" />
                  </button>
                </div>
              </template>
            </el-table-column>
          </el-table>

          <div v-if="!loading && !users.length" class="panel" style="border: 0">
            <EmptyBlock title="没有匹配的用户" description="调整关键词或角色筛选，或创建新的账号。">
              <button class="btn btn-primary btn-sm" type="button" @click="openCreate">
                <Plus :size="14" /> 新建用户
              </button>
            </EmptyBlock>
          </div>
        </div>
      </div>

      <aside class="admin-aside">
        <div class="scope-card">
          <h3><ShieldCheck :size="13" style="vertical-align: -2px" /> 数据范围</h3>
          <p>用户的数据可见范围由角色决定，组织绑定决定范围的起点。</p>
          <div class="scope-chain" style="margin-top: 10px">
            <div v-for="role in roles" :key="role.code" class="scope-chain-item">
              <span>{{ role.name }}</span>
              <span class="tag">{{ dataScopeLabel(role.data_scope) }}</span>
            </div>
          </div>
        </div>

        <div class="scope-card">
          <h3><MessageCircle :size="13" style="vertical-align: -2px" /> 消息绑定</h3>
          <p>
            企业微信推送填写企业微信 UserID；微信公众号推送填写 OpenID。
            未绑定对应 ID 的账号在推送时会被跳过，不影响其他接收人。
          </p>
        </div>

        <div class="scope-card">
          <h3><KeyRound :size="13" style="vertical-align: -2px" /> 账号安全</h3>
          <p>编辑用户时密码留空表示不修改。当前登录账号不能删除，建议生产环境使用强密码。</p>
        </div>
      </aside>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="editingId ? '编辑用户' : '新建用户'"
      width="660px"
      destroy-on-close
    >
      <div class="form-grid">
        <label class="field-label">
          姓名
          <el-input v-model="form.name" maxlength="64" placeholder="真实姓名" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          用户名
          <el-input v-model="form.username" maxlength="64" placeholder="登录用户名" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          {{ editingId ? '重置密码' : '初始密码' }}
          <el-input
            v-model="form.password"
            type="password"
            show-password
            maxlength="64"
            :placeholder="editingId ? '留空表示不修改' : '至少 6 位'"
            style="margin-top: 8px"
          />
        </label>
        <label class="field-label">
          角色
          <el-select v-model="form.role_code" style="width: 100%; margin-top: 8px">
            <el-option
              v-for="role in roles"
              :key="role.code"
              :value="role.code"
              :label="`${role.name} · ${dataScopeLabel(role.data_scope)}`"
            />
          </el-select>
        </label>
        <label class="field-label form-span-2">
          所属组织
          <OrgTreeSelect
            v-model="form.organization_id"
            :data="orgTree"
            placeholder="选择用户所属组织"
            style="margin-top: 8px"
          />
        </label>
        <label class="field-label">
          手机号
          <el-input v-model="form.phone" maxlength="32" placeholder="手机号" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          邮箱
          <el-input v-model="form.email" maxlength="128" placeholder="邮箱" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          企业微信 UserID
          <el-input v-model="form.wecom_user_id" maxlength="128" placeholder="用于企业微信推送" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          微信 OpenID
          <el-input v-model="form.wechat_open_id" maxlength="128" placeholder="用于公众号推送" style="margin-top: 8px" />
        </label>
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
  </div>
</template>

<style scoped>
/* 消息绑定列：企微 / 微信 ID 超长时省略，避免撑破列宽 */
.bind-cell .tag {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  line-height: 1.5;
}

/* 最后登录列：小字号 + 单行，保持表格行高一致 */
.login-cell {
  display: inline-block;
  color: var(--muted);
  font-size: 10.5px;
  white-space: nowrap;
}
</style>
