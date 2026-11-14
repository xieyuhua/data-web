<!-- 组织成员面板：
     左侧组织树选中某个部门/门店后，右侧展示该部门下的成员列表（企业微信通讯录交互）。
     支持「包含下级部门」开关、成员搜索/角色筛选、新增/编辑/删除成员，以及通讯录模板下载、导入与导出。 -->
<script setup>
import { computed, reactive, ref, watch } from 'vue'
import {
  ChevronRight,
  Download,
  FileSpreadsheet,
  Pencil,
  Plus,
  Search,
  Trash2,
  Upload,
} from 'lucide-vue-next'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, getErrorMessage } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { formatDate, statusMeta } from '../utils/format'
import { flattenOrgs } from '../utils/org'
import EmptyBlock from './EmptyBlock.vue'
import OrgTreeSelect from './OrgTreeSelect.vue'

const props = defineProps({
  // 当前选中的组织节点（null 表示未选择，展示全部范围内成员）
  org: { type: Object, default: null },
  // 选中节点的祖先链路，用于面包屑
  path: { type: Array, default: () => [] },
  // 完整组织树，用于成员「所属组织」展示与表单选择
  orgTree: { type: Array, default: () => [] },
  // 含下级部门的总人数（来自父级统计接口），用于标题角标
  totalCount: { type: Number, default: 0 },
})


// changed：成员增删改或导入后通知父级刷新人数统计
const emit = defineEmits(['changed'])

const auth = useAuthStore()

const users = ref([])
const roles = ref([])
const loading = ref(true)
const saving = ref(false)
const importing = ref(false)
const keyword = ref('')
const roleFilter = ref('')
// 是否包含下级部门成员，默认开启：符合“点部门看整片团队”的使用习惯
const includeChildren = ref(true)
const fileInput = ref(null)

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

// 组织平铺表：用于成员所属组织名称 / 编码展示
const orgs = computed(() => flattenOrgs(props.orgTree))
const orgMap = computed(() => Object.fromEntries(orgs.value.map((item) => [item.id, item])))
const activeCount = computed(() => users.value.filter((item) => item.status === 'active').length)
const boundCount = computed(() => users.value.filter((item) => item.wecom_user_id || item.wechat_open_id).length)

// 面板标题：选中部门名称或“全部成员”
const title = computed(() => props.org?.name || '全部成员')
// 面包屑文案由父级计算，这里只做拼接展示
const pathText = computed(() => props.path.map((item) => item.name).join(' / '))
// 成员所属组织名称
function orgName(id) {
  return orgMap.value[id]?.name || '—'
}
// 角色名称
function roleName(code) {
  return roles.value.find((item) => item.code === code)?.name || code || '—'
}
// 头像文字：取姓名最后两个字
function initials(name) {
  const text = (name || '').trim()
  if (!text) return '?'
  return text.length <= 2 ? text : text.slice(-2)
}

// 重置成员表单
function resetForm() {
  editingId.value = null
  Object.assign(form, {
    name: '',
    username: '',
    password: '',
    phone: '',
    email: '',
    role_code: roles.value[0]?.code || '',
    organization_id: props.org?.id || null,
    wecom_user_id: '',
    wechat_open_id: '',
    status: 'active',
  })
}

// 新建成员：默认归属于当前选中部门
function openCreate() {
  resetForm()
  dialogVisible.value = true
}

// 编辑成员
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

// 加载角色字典（只取一次）
async function loadRoles() {
  if (roles.value.length) return
  try {
    roles.value = (await api.get('/roles')) || []
  } catch (error) {
    // 角色加载失败不阻断列表展示，仅影响下拉选项
    console.warn('角色列表加载失败', error)
  }
}

// 按当前部门 + 搜索条件加载成员
async function loadUsers() {
  loading.value = true
  try {
    const params = new URLSearchParams()
    if (props.org?.id) {
      params.set('org_id', String(props.org.id))
      params.set('include_children', includeChildren.value ? '1' : '0')
    }
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

// 保存成员（新增 / 编辑）
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
    ElMessage.warning('新建成员密码至少 6 位')
    return
  }
  saving.value = true
  try {
    const payload = {
      name: form.name.trim(),
      username: form.username.trim(),
      password: form.password || undefined,
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
      ElMessage.success('成员信息已更新')
    } else {
      await api.post('/users', payload)
      ElMessage.success('成员已新增')
    }
    dialogVisible.value = false
    await loadUsers()
    emit('changed')
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    saving.value = false
  }
}

// 删除成员（不能删除当前登录账号）
async function remove(row) {
  try {
    await ElMessageBox.confirm(`确认移除成员「${row.name}」？该操作会删除账号，不可恢复。`, '移除成员', {
      type: 'warning',
      confirmButtonText: '移除',
      cancelButtonText: '取消',
    })
    await api.delete(`/users/${row.id}`)
    ElMessage.success('成员已移除')
    await loadUsers()
    emit('changed')
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(getErrorMessage(error))
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

// 导出通讯录
async function exportContacts() {
  try {
    await api.download('/users/export', '通讯录.xlsx')
    ElMessage.success('通讯录已导出')
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  }
}

// 触发导入文件选择
function pickImport() {
  fileInput.value?.click()
}

// 导入通讯录 Excel
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
    emit('changed')
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    importing.value = false
  }
}

// 切换部门或“包含下级”后重新取数
watch(
  () => [props.org?.id, includeChildren.value],
  () => {
    loadUsers()
  },
)

// 初始化：角色字典 + 首批成员
loadRoles()
loadUsers()
</script>

<template>
  <section class="member-panel">
    <!-- 头部：面包屑 + 汇总数据 + 操作按钮 -->
    <header class="member-head">
      <div class="member-head-main">
        <div class="member-breadcrumb">
          <span v-for="(item, index) in path" :key="item.id" class="crumb">
            <span>{{ item.name }}</span>
            <ChevronRight v-if="index < path.length - 1" :size="12" />
          </span>
          <span v-if="!path.length" class="crumb">全部成员</span>
        </div>
        <div class="member-title-row">
          <h2>{{ title }}</h2>
          <span class="tag">{{ users.length }} 人</span>
          <span v-if="totalCount > users.length" class="tag tag-green">含下级 {{ totalCount }} 人</span>
          <span v-if="activeCount !== users.length" class="tag">启用 {{ activeCount }}</span>
          <span v-if="boundCount" class="tag tag-blue">已绑定 {{ boundCount }}</span>
        </div>
      </div>

      <div class="member-head-actions">
        <input ref="fileInput" type="file" accept=".xlsx,.xls" style="display: none" @change="onImportChange" />
        <button class="btn btn-ghost btn-sm" type="button" title="下载导入模板" @click="downloadTemplate">
          <FileSpreadsheet :size="13" /> 模板
        </button>
        <button class="btn btn-ghost btn-sm" type="button" :disabled="importing" @click="pickImport">
          <Upload :size="13" /> {{ importing ? '导入中…' : '导入' }}
        </button>
        <button class="btn btn-ghost btn-sm" type="button" @click="exportContacts">
          <Download :size="13" /> 导出
        </button>
        <button class="btn btn-primary btn-sm" type="button" @click="openCreate">
          <Plus :size="14" /> 新增成员
        </button>
      </div>
    </header>

    <!-- 筛选行：关键词 / 角色 / 是否包含下级部门 -->
    <div class="member-toolbar">
      <div class="search-box">
        <Search :size="14" />
        <input
          v-model="keyword"
          type="search"
          placeholder="搜索姓名、账号或手机号"
          @keyup.enter="loadUsers"
        />
      </div>
      <el-select v-model="roleFilter" placeholder="全部角色" clearable size="small" style="width: 148px" @change="loadUsers">
        <el-option v-for="role in roles" :key="role.code" :value="role.code" :label="role.name" />
      </el-select>
      <el-checkbox v-model="includeChildren" size="small">包含下级部门</el-checkbox>
      <span v-if="pathText" class="member-path">{{ pathText }}</span>
      <button class="btn btn-secondary btn-sm" type="button" @click="loadUsers">查询</button>
    </div>

    <!-- 成员列表 -->
    <div class="member-body">
      <div v-if="loading" class="member-loading">
        <div class="skeleton" style="height: 260px" />
      </div>

      <el-table v-else :data="users" height="100%" style="width: 100%" row-key="id" size="small">
        <el-table-column label="成员" min-width="176">
          <template #default="{ row }">
            <div class="user-cell">
              <span class="avatar">{{ initials(row.name) }}</span>
              <div>
                <div class="member-name-line">
                  <strong>{{ row.name }}</strong>
                  <span v-if="row.status !== 'active'" class="tag member-mini-tag">{{ statusMeta(row.status).label }}</span>
                  <span v-if="row.wecom_user_id" class="tag tag-green member-mini-tag">企微</span>
                </div>
                <span>@{{ row.username }}</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="角色" width="112">
          <template #default="{ row }">
            <span class="tag tag-green">{{ roleName(row.role_code) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="所属组织" min-width="124">
          <template #default="{ row }">
            <span>{{ orgName(row.organization_id) }}</span>
            <span class="mono member-org-code">{{ orgMap[row.organization_id]?.code || '' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="联系方式" min-width="132">
          <template #default="{ row }">
            <div class="member-contact">
              <span>{{ row.phone || '—' }}</span>
              <span>{{ row.email || '—' }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="最近登录" width="124">
          <template #default="{ row }">
            <span class="member-login">{{ row.last_login_at ? formatDate(row.last_login_at, true) : '从未登录' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="88" fixed="right">
          <template #default="{ row }">
            <div class="inline-actions">
              <button class="btn btn-ghost btn-sm" type="button" title="编辑成员" @click="openEdit(row)">
                <Pencil :size="13" />
              </button>
              <button
                class="btn btn-ghost btn-sm"
                type="button"
                title="删除成员"
                style="color: var(--rose)"
                :disabled="row.id === auth.user?.id"
                @click="remove(row)"
              >
                <Trash2 :size="13" />
              </button>
            </div>
          </template>
        </el-table-column>
        <template #empty>
          <EmptyBlock
            :title="keyword || roleFilter ? '没有匹配的成员' : '该部门下暂无成员'"
            :description="keyword || roleFilter ? '换个关键词或清空角色筛选再试。' : '可以直接新增成员，或导入通讯录 Excel。'"
          >
            <button class="btn btn-primary btn-sm" type="button" @click="openCreate">
              <Plus :size="14" /> 新增成员
            </button>
          </EmptyBlock>
        </template>
      </el-table>
    </div>
  </section>

  <!-- 成员表单弹窗 -->
  <el-dialog v-model="dialogVisible" :title="editingId ? '编辑成员' : '新增成员'" width="620px" destroy-on-close>
    <div class="form-grid">
      <label class="field-label">
        姓名
        <el-input v-model="form.name" maxlength="64" placeholder="真实姓名" style="margin-top: 8px" />
      </label>
      <label class="field-label">
        登录账号
        <el-input v-model="form.username" maxlength="64" placeholder="用于账号密码登录" style="margin-top: 8px" />
      </label>
      <label class="field-label">
        密码
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
        <el-select v-model="form.role_code" style="width: 100%; margin-top: 8px" placeholder="请选择角色">
          <el-option v-for="role in roles" :key="role.code" :value="role.code" :label="role.name" />
        </el-select>
      </label>
      <label class="field-label">
        所属组织
        <OrgTreeSelect v-model="form.organization_id" :data="orgTree" style="margin-top: 8px" />
      </label>
      <label class="field-label">
        手机号
        <el-input v-model="form.phone" maxlength="32" placeholder="用于企业微信/短信触达" style="margin-top: 8px" />
      </label>
      <label class="field-label">
        邮箱
        <el-input v-model="form.email" maxlength="128" placeholder="选填" style="margin-top: 8px" />
      </label>
      <label class="field-label">
        企业微信 UserID
        <el-input v-model="form.wecom_user_id" maxlength="128" placeholder="企微授权登录标识" style="margin-top: 8px" />
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
</template>

<style scoped>
/* 右侧成员面板：铺满父容器高度，表格内部滚动 */
.member-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 16px;
  overflow: hidden;
}

.member-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 20px 12px;
  border-bottom: 1px solid var(--line-soft);
}

.member-head-main {
  min-width: 0;
}

.member-breadcrumb {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px;
  color: var(--muted);
  font-size: 11.5px;
}

.crumb {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}

.member-title-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 6px;
}

.member-title-row h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 650;
  letter-spacing: -0.02em;
}

.member-head-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  flex: none;
}

.member-toolbar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  padding: 10px 20px;
  border-bottom: 1px solid var(--line-soft);
  background: #fbfdfc;
}

.member-path {
  color: var(--muted-light);
  font-size: 11.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 320px;
  margin-left: auto;
}

.member-body {
  flex: 1;
  min-height: 0;
  padding: 4px 8px 8px;
}

/* 通讯录列表行高压缩，一屏能看到更多成员 */
.member-body :deep(.el-table td.el-table__cell) {
  padding: 7px 0;
}

.member-body :deep(.el-table .cell) {
  line-height: 1.45;
}

.member-body :deep(.el-table .tag) {
  white-space: nowrap;
}

.member-loading {
  padding: 16px 8px;
}

.member-org-code {
  display: block;
  color: var(--muted-light);
  font-size: 10.5px;
}

/* 成员单元格：姓名 + 状态/企微徽标同一行，账号在下一行 */
.member-name-line {
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
}

.member-name-line strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.member-mini-tag {
  flex: none;
  padding: 0 4px;
  font-size: 10px;
  line-height: 15px;
}

.member-contact {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 11.5px;
  color: var(--muted);
  overflow: hidden;
}

.member-contact span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.member-login {
  display: block;
  font-size: 11.5px;
  color: var(--muted);
  white-space: nowrap;
}

@media (max-width: 900px) {
  .member-head {
    flex-direction: column;
  }

  .member-path {
    display: none;
  }
}
</style>
