<!-- 组织架构管理页：维护集团/大区/区域/门店层级，支持手工维护、Excel 导入导出通讯录与企业微信异步同步。 -->
<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  Building2,
  Download,
  FileSpreadsheet,
  Info,
  Landmark,
  MapPin,
  Pencil,
  Plus,
  Search,
  ShieldCheck,
  Store,
  Trash2,
  Upload,
} from 'lucide-vue-next'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, getErrorMessage } from '../../api/client'
import { flattenOrgs, ORG_TYPES, orgTypeLabel } from '../../utils/org'
import EmptyBlock from '../../components/EmptyBlock.vue'
import OrgTreeSelect from '../../components/OrgTreeSelect.vue'
import WecomSyncPanel from '../../components/WecomSyncPanel.vue'

const tree = ref([])
const loading = ref(true)
const keyword = ref('')
const saving = ref(false)
const importing = ref(false)
const exporting = ref(false)
const lastImport = ref('')
const fileInput = ref(null)

const dialogVisible = ref(false)
const editingId = ref(null)
const form = reactive({
  parent_id: null,
  name: '',
  code: '',
  type: 'company',
  manager_name: '',
  phone: '',
  address: '',
  status: 'active',
  sort: 0,
})

const flat = computed(() => flattenOrgs(tree.value))
const storeCount = computed(() => flat.value.filter((item) => item.type === 'store').length)
const regionCount = computed(
  () => flat.value.filter((item) => item.type === 'region' || item.type === 'big_region').length,
)
const disabledCount = computed(() => flat.value.filter((item) => item.status !== 'active').length)

const filteredTree = computed(() => {
  const text = keyword.value.trim().toLowerCase()
  if (!text) return tree.value
  const filter = (nodes) => {
    const result = []
    for (const node of nodes || []) {
      const children = filter(node.children)
      const hit =
        (node.name || '').toLowerCase().includes(text) ||
        (node.code || '').toLowerCase().includes(text) ||
        (node.manager_name || '').toLowerCase().includes(text)
      if (hit || children.length) result.push({ ...node, children })
    }
    return result
  }
  return filter(tree.value)
})

const parentTree = computed(() => {
  if (!editingId.value) return tree.value
  const prune = (nodes) =>
    (nodes || [])
      .filter((node) => node.id !== editingId.value)
      .map((node) => ({ ...node, children: prune(node.children) }))
  return prune(tree.value)
})

const childType = {
  company: 'big_region',
  big_region: 'region',
  region: 'store',
  store: 'store',
}

// 重置组织表单
function resetForm() {
  editingId.value = null
  Object.assign(form, {
    parent_id: null,
    name: '',
    code: '',
    type: 'company',
    manager_name: '',
    phone: '',
    address: '',
    status: 'active',
    sort: 0,
  })
}

// 打开新建组织弹窗（可指定上级组织）
function openCreate(parent = null) {
  resetForm()
  if (parent) {
    form.parent_id = parent.id
    form.type = childType[parent.type] || 'region'
    form.sort = 0
  }
  dialogVisible.value = true
}

// 打开编辑组织弹窗
function openEdit(row) {
  resetForm()
  editingId.value = row.id
  Object.assign(form, {
    parent_id: row.parent_id || null,
    name: row.name || '',
    code: row.code || '',
    type: row.type || 'company',
    manager_name: row.manager_name || '',
    phone: row.phone || '',
    address: row.address || '',
    status: row.status || 'active',
    sort: row.sort || 0,
  })
  dialogVisible.value = true
}

// 按组织类型返回对应图标
function iconFor(type) {
  if (type === 'big_region') return Landmark
  if (type === 'region') return MapPin
  if (type === 'store') return Store
  return Building2
}

// 加载组织树
async function load() {
  loading.value = true
  try {
    tree.value = (await api.get('/orgs/tree')) || []
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

// 保存组织节点
async function save() {
  if (!form.name.trim()) {
    ElMessage.warning('请填写组织名称')
    return
  }
  if (!form.code.trim()) {
    ElMessage.warning('请填写组织编码')
    return
  }
  saving.value = true
  try {
    const payload = {
      parent_id: form.parent_id || null,
      name: form.name.trim(),
      code: form.code.trim(),
      type: form.type,
      manager_name: form.manager_name.trim(),
      phone: form.phone.trim(),
      address: form.address.trim(),
      status: form.status,
      sort: Number(form.sort) || 0,
    }
    if (editingId.value) {
      await api.put(`/admin/orgs/${editingId.value}`, payload)
      ElMessage.success('组织信息已更新')
    } else {
      await api.post('/admin/orgs', payload)
      ElMessage.success('组织已创建')
    }
    dialogVisible.value = false
    await load()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    saving.value = false
  }
}

// 删除组织节点
async function remove(row) {
  try {
    await ElMessageBox.confirm(
      `确认删除「${row.name}」？存在下级组织或关联用户时将无法删除。`,
      '删除组织',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
    await api.delete(`/admin/orgs/${row.id}`)
    ElMessage.success('组织已删除')
    await load()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(getErrorMessage(error))
  }
}

// 导出组织通讯录（Excel）
async function exportOrgs() {
  exporting.value = true
  try {
    await api.download('/admin/orgs/export', 'organizations.xlsx')
    ElMessage.success('组织架构已导出')
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    exporting.value = false
  }
}

// 触发文件选择框
function pickFile() {
  fileInput.value?.click()
}

// 上传并导入组织通讯录
async function handleFile(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  if (!/\.(xlsx|xls)$/i.test(file.name)) {
    ElMessage.warning('请选择 Excel 文件（.xlsx / .xls）')
    return
  }
  importing.value = true
  try {
    const result = await api.upload('/admin/orgs/import', file)
    lastImport.value = `已导入 ${result?.imported || 0} 条组织数据`
    ElMessage.success(lastImport.value)
    await load()
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
        <span>组织节点</span>
        <strong>{{ flat.length }}</strong>
      </div>
      <div class="metric-mini">
        <span>大区 / 区域</span>
        <strong>{{ regionCount }}</strong>
      </div>
      <div class="metric-mini">
        <span>门店</span>
        <strong>{{ storeCount }}</strong>
      </div>
      <div class="metric-mini">
        <span>停用节点</span>
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
                <input v-model="keyword" type="search" placeholder="搜索组织名称、编码或负责人" />
              </div>
              <span class="tag">{{ flat.length }} 个节点</span>
            </div>
            <div class="inline-actions">
              <!-- 企业微信异步同步入口，完成后自动刷新组织树 -->
              <WecomSyncPanel @synced="load" />
              <button class="btn btn-secondary btn-sm" type="button" :disabled="exporting" @click="exportOrgs">
                <Download :size="14" /> {{ exporting ? '导出中…' : '导出 Excel' }}
              </button>
              <button class="btn btn-secondary btn-sm" type="button" :disabled="importing" @click="pickFile">
                <Upload :size="14" /> {{ importing ? '导入中…' : '导入 Excel' }}
              </button>
              <button class="btn btn-primary btn-sm" type="button" @click="openCreate()">
                <Plus :size="14" /> 新建组织
              </button>
            </div>
          </div>

          <div v-if="loading" style="padding: 20px">
            <div class="skeleton" style="height: 300px" />
          </div>

          <el-table
            v-else
            :data="filteredTree"
            row-key="id"
            :tree-props="{ children: 'children' }"
            default-expand-all
            style="width: 100%"
          >
            <el-table-column label="组织名称" min-width="270">
              <template #default="{ row }">
                <div class="org-name-cell">
                  <span class="org-type-icon" :class="row.type === 'region' || row.type === 'store' ? row.type : ''">
                    <component :is="iconFor(row.type)" :size="14" />
                  </span>
                  <div>
                    <strong style="display: block; font-size: 10.5px">{{ row.name }}</strong>
                    <span class="mono" style="color: var(--muted-light); font-size: 8px">{{ row.code }}</span>
                  </div>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="类型" width="92">
              <template #default="{ row }">
                <span class="tag">{{ orgTypeLabel(row.type) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="manager_name" label="负责人" width="110">
              <template #default="{ row }">{{ row.manager_name || '—' }}</template>
            </el-table-column>
            <el-table-column prop="phone" label="电话" width="130">
              <template #default="{ row }">{{ row.phone || '—' }}</template>
            </el-table-column>
            <el-table-column prop="address" label="地址" min-width="150" show-overflow-tooltip>
              <template #default="{ row }">{{ row.address || '—' }}</template>
            </el-table-column>
            <el-table-column label="状态" width="84">
              <template #default="{ row }">
                <span class="tag" :class="row.status === 'active' ? 'tag-green' : 'tag-red'">
                  {{ row.status === 'active' ? '启用' : '停用' }}
                </span>
              </template>
            </el-table-column>
            <el-table-column prop="sort" label="排序" width="70" />
            <el-table-column label="操作" width="190" fixed="right">
              <template #default="{ row }">
                <div class="inline-actions">
                  <button
                    v-if="row.type !== 'store'"
                    class="btn btn-ghost btn-sm"
                    type="button"
                    title="新增下级"
                    @click="openCreate(row)"
                  >
                    <Plus :size="13" />
                  </button>
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

          <div v-if="!loading && !filteredTree.length" class="panel" style="border: 0">
            <EmptyBlock
              title="没有匹配的组织"
              :description="tree.length ? '换一个关键词试试。' : '先创建集团节点，再逐级维护大区、区域和门店。'"
            >
              <button class="btn btn-primary btn-sm" type="button" @click="openCreate()">
                <Plus :size="14" /> 新建组织
              </button>
            </EmptyBlock>
          </div>
        </div>
      </div>

      <aside class="admin-aside">
        <div class="scope-card">
          <h3><ShieldCheck :size="13" style="vertical-align: -2px" /> 企业微信通讯录同步</h3>
          <p>
            配置 CorpID、AgentID 与 Secret 后，可异步拉取企业微信部门与成员，按层级映射为集团、大区、区域与门店，
            并按企业微信 UserID 幂等更新平台账号。同步任务支持实时查看进度、新增/更新/失败明细与历史记录。
          </p>
          <p style="margin-top: 8px">手工 Excel 导入导出仍然可用，两种方式可以并行维护。</p>
        </div>
        <div class="scope-card">
          <h3><FileSpreadsheet :size="13" style="vertical-align: -2px" /> Excel 导入</h3>
          <p>
            表头固定为「类型、名称、编码、上级编码、负责人、电话、地址、状态」，按组织编码执行新增或更新。
            建议先导出模板，填写后再导入。
          </p>
          <div class="sql-hints">
            <span class="sql-hint">company</span>
            <span class="sql-hint">big_region</span>
            <span class="sql-hint">region</span>
            <span class="sql-hint">store</span>
          </div>
          <p v-if="lastImport" style="margin-top: 10px; color: var(--brand)">{{ lastImport }}</p>
          <button class="btn btn-secondary btn-sm" type="button" style="margin-top: 12px" @click="pickFile">
            <Upload :size="14" /> 选择 Excel 文件
          </button>
          <input
            ref="fileInput"
            type="file"
            accept=".xlsx,.xls"
            style="display: none"
            @change="handleFile"
          />
        </div>

        <div class="scope-card">
          <h3><Info :size="13" style="vertical-align: -2px" /> 层级与数据权限</h3>
          <p>
            组织层级决定数据范围：集团老总看全部，大区经理看本大区及下级，
            区域经理看本区域及门店，店长仅看当前门店。调整上级组织会立即影响下级账号的可见范围。
          </p>
        </div>

        <div class="scope-card">
          <h3><ShieldCheck :size="13" style="vertical-align: -2px" /> 删除限制</h3>
          <p>存在下级组织或仍有用户绑定时无法删除。请先转移用户、删除下级节点。</p>
        </div>
      </aside>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="editingId ? '编辑组织' : '新建组织'"
      width="620px"
      destroy-on-close
    >
      <div class="form-grid">
        <label class="field-label">
          上级组织
          <OrgTreeSelect
            v-model="form.parent_id"
            :data="parentTree"
            placeholder="不选表示顶级组织"
            style="margin-top: 8px"
          />
        </label>
        <label class="field-label">
          组织类型
          <el-select v-model="form.type" style="width: 100%; margin-top: 8px">
            <el-option v-for="item in ORG_TYPES" :key="item.value" :value="item.value" :label="item.label" />
          </el-select>
        </label>
        <label class="field-label">
          组织名称
          <el-input v-model="form.name" maxlength="100" placeholder="例如：华东大区" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          组织编码
          <el-input v-model="form.code" maxlength="64" placeholder="例如：BR_EAST" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          负责人
          <el-input v-model="form.manager_name" maxlength="64" placeholder="负责人姓名" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          联系电话
          <el-input v-model="form.phone" maxlength="32" placeholder="联系电话" style="margin-top: 8px" />
        </label>
        <label class="field-label form-span-2">
          地址
          <el-input v-model="form.address" maxlength="255" placeholder="门店或办公地址" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          状态
          <el-select v-model="form.status" style="width: 100%; margin-top: 8px">
            <el-option value="active" label="启用" />
            <el-option value="disabled" label="停用" />
          </el-select>
        </label>
        <label class="field-label">
          排序
          <el-input-number v-model="form.sort" :min="0" :max="9999" style="width: 100%; margin-top: 8px" />
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
