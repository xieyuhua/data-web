<!-- 组织架构管理页（企业微信通讯录样式）：
     左侧为部门树（集团 → 大区 → 区域 → 门店，带人数角标），点击任意部门后
     右侧展示该部门下的成员列表，支持成员增删改与通讯录导入导出。
     组织本身的增删改、Excel 导入导出与企业微信异步同步仍在本页左侧完成。 -->
<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  Building2,
  Download,
  Landmark,
  MapPin,
  Pencil,
  Plus,
  Search,
  Store,
  Trash2,
  Upload,
} from 'lucide-vue-next'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, getErrorMessage } from '../../api/client'
import { findOrgById, findOrgChain, flattenOrgs, ORG_TYPES, orgTypeLabel } from '../../utils/org'
import EmptyBlock from '../../components/EmptyBlock.vue'
import OrgMemberPanel from '../../components/OrgMemberPanel.vue'
import OrgTreeSelect from '../../components/OrgTreeSelect.vue'
import WecomSyncPanel from '../../components/WecomSyncPanel.vue'

const tree = ref([])
// 组织人数统计：{ [orgID]: { direct, total, active } }
const stats = ref({})
const loading = ref(true)
const keyword = ref('')
const activeId = ref(null)
const saving = ref(false)
const importing = ref(false)
const exporting = ref(false)
const lastImport = ref('')
const fileInput = ref(null)
const treeRef = ref(null)

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
const disabledCount = computed(() => flat.value.filter((item) => item.status !== 'active').length)

// 当前选中的组织节点与祖先链路（面包屑）
const activeOrg = computed(() => findOrgById(tree.value, activeId.value))
const activePath = computed(() => findOrgChain(tree.value, activeId.value))
// 当前节点的人数统计，未命中时回退为 0
const activeStat = computed(() => stats.value[activeId.value] || { direct: 0, total: 0, active: 0 })

// 默认展开前两级，避免大机构一次性铺开过长
const defaultExpanded = computed(() => {
  const ids = []
  const walk = (nodes, level) => {
    for (const node of nodes || []) {
      if (level <= 2) ids.push(node.id)
      if (level < 2) walk(node.children, level + 1)
    }
  }
  walk(tree.value, 1)
  return ids
})

// 搜索过滤：命中名称、编码或负责人时保留节点及其祖先
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

// el-tree 的过滤方法，与关键字联动
function filterNode(value, data) {
  if (!value) return true
  const text = value.toLowerCase()
  return (
    (data.name || '').toLowerCase().includes(text) ||
    (data.code || '').toLowerCase().includes(text) ||
    (data.manager_name || '').toLowerCase().includes(text)
  )
}

watch(keyword, (value) => {
  treeRef.value?.filter(value)
})

// 上级组织候选（编辑时排除自身及下级，避免形成环）
const parentTree = computed(() => {
  if (!editingId.value) return tree.value
  const prune = (nodes) =>
    (nodes || [])
      .filter((node) => node.id !== editingId.value)
      .map((node) => ({ ...node, children: prune(node.children) }))
  return prune(tree.value)
})

// 子节点默认类型：集团 → 大区 → 区域 → 门店
const childType = {
  company: 'big_region',
  big_region: 'region',
  region: 'store',
  store: 'store',
}

// 按组织类型返回图标
function iconFor(type) {
  if (type === 'big_region') return Landmark
  if (type === 'region') return MapPin
  if (type === 'store') return Store
  return Building2
}

// 节点人数角标：优先展示含下级总人数
function memberBadge(orgId) {
  const stat = stats.value[orgId]
  return stat?.total || 0
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

// 新建组织（可指定上级）
function openCreate(parent = null) {
  resetForm()
  if (parent) {
    form.parent_id = parent.id
    form.type = childType[parent.type] || 'region'
  }
  dialogVisible.value = true
}

// 编辑组织
function openEdit(row) {
  if (!row) return
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

// 加载组织树，并在首次进入时默认选中第一个根节点
async function load() {
  loading.value = true
  try {
    tree.value = (await api.get('/orgs/tree')) || []
    if (!findOrgById(tree.value, activeId.value)) {
      activeId.value = tree.value[0]?.id || null
    }
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

// 单独刷新人数统计（成员增删改后调用，避免整棵树重绘）
async function loadStats() {
  try {
    const list = (await api.get('/orgs/member-stats')) || []
    const map = {}
    for (const item of list) {
      map[item.org_id] = item
    }
    stats.value = map
  } catch (error) {
    // 统计失败不影响主流程，角标降级为 0
    console.warn('组织人数统计加载失败', error)
  }
}

// 选中部门
function onNodeClick(data) {
  activeId.value = data.id
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
    await Promise.all([load(), loadStats()])
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    saving.value = false
  }
}

// 删除组织节点
async function remove(row) {
  if (!row?.id) {
    ElMessage.warning('请先在左侧选择要删除的组织')
    return
  }
  try {
    await ElMessageBox.confirm(
      `确认删除「${row.name}」？存在下级组织或关联成员时将无法删除。`,
      '删除组织',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
    await api.delete(`/admin/orgs/${row.id}`)
    ElMessage.success('组织已删除')
    if (activeId.value === row.id) activeId.value = null
    await Promise.all([load(), loadStats()])
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(getErrorMessage(error))
  }
}

// 导出组织架构 Excel
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

// 触发导入文件选择
function pickFile() {
  fileInput.value?.click()
}

// 导入组织架构 Excel
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
    await Promise.all([load(), loadStats()])
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    importing.value = false
  }
}

onMounted(async () => {
  await load()
  await loadStats()
})
</script>

<template>
  <div class="org-workbench">
    <!-- 左栏：部门树 -->
    <aside class="org-panel">
      <div class="org-panel-head">
        <div class="org-panel-title">
          <Building2 :size="15" />
          <strong>组织架构</strong>
          <span class="tag">{{ flat.length }}</span>
          <span v-if="disabledCount" class="tag tag-red">停用 {{ disabledCount }}</span>
        </div>
        <div class="org-panel-actions">
          <button class="icon-btn" type="button" title="新建顶级组织" @click="openCreate()">
            <Plus :size="15" />
          </button>
          <button class="icon-btn" type="button" title="编辑当前组织" :disabled="!activeOrg" @click="openEdit(activeOrg)">
            <Pencil :size="15" />
          </button>
          <button
            class="icon-btn"
            type="button"
            title="删除当前组织"
            :disabled="!activeOrg"
            @click="remove(activeOrg)"
          >
            <Trash2 :size="15" />
          </button>
        </div>
        <div class="search-box org-search">
          <Search :size="14" />
          <input v-model="keyword" type="search" placeholder="搜索部门、门店或负责人" />
        </div>
        <div class="org-panel-tools">
          <button class="btn btn-ghost btn-sm" type="button" :disabled="exporting" @click="exportOrgs">
            <Download :size="13" /> {{ exporting ? '导出中…' : '导出' }}
          </button>
          <button class="btn btn-ghost btn-sm" type="button" :disabled="importing" @click="pickFile">
            <Upload :size="13" /> {{ importing ? '导入中…' : '导入' }}
          </button>
          <!-- 企业微信异步同步入口，完成后刷新组织树与人数 -->
          <WecomSyncPanel @synced="load()" />
          <input ref="fileInput" type="file" accept=".xlsx,.xls" style="display: none" @change="handleFile" />
        </div>
      </div>

      <div class="org-tree-wrap">
        <div v-if="loading" class="org-tree-loading">
          <div class="skeleton" style="height: 280px" />
        </div>
        <el-tree
          v-else
          ref="treeRef"
          class="org-tree"
          :data="filteredTree"
          node-key="id"
          :props="{ label: 'name', children: 'children' }"
          :current-node-key="activeId"
          :default-expanded-keys="defaultExpanded"
          :expand-on-click-node="false"
          :filter-node-method="filterNode"
          highlight-current
          @node-click="onNodeClick"
        >
          <template #default="{ data }">
            <div class="org-node">
              <span class="org-node-icon" :class="data.type">
                <component :is="iconFor(data.type)" :size="13" />
              </span>
              <span class="org-node-name" :title="`${orgTypeLabel(data.type)} · ${data.name}`">{{ data.name }}</span>
              <span v-if="data.status !== 'active'" class="org-node-off">停用</span>
              <span class="org-node-count">{{ memberBadge(data.id) }}</span>
              <button
                v-if="data.type !== 'store'"
                class="org-node-add"
                type="button"
                title="新增下级"
                @click.stop="openCreate(data)"
              >
                <Plus :size="12" />
              </button>
            </div>
          </template>
        </el-tree>

        <EmptyBlock
          v-if="!loading && !filteredTree.length"
          :title="tree.length ? '没有匹配的部门' : '还没有组织数据'"
          :description="tree.length ? '换一个关键词试试。' : '可以新建集团节点，或从企业微信同步 / Excel 导入。'"
        >
          <button class="btn btn-primary btn-sm" type="button" @click="openCreate()">
            <Plus :size="14" /> 新建组织
          </button>
        </EmptyBlock>
      </div>

      <div class="org-panel-foot">
        <span>{{ storeCount }} 家门店 · 点击部门查看成员</span>
        <span v-if="lastImport" class="org-panel-import">{{ lastImport }}</span>
      </div>
    </aside>

    <!-- 右栏：选中部门的成员列表 -->
    <OrgMemberPanel
      :org="activeOrg"
      :path="activePath"
      :org-tree="tree"
      :total-count="activeStat.total"
      @changed="loadStats"
    />

    <!-- 组织表单弹窗 -->
    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑组织' : '新建组织'" width="620px" destroy-on-close>
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

<style scoped>
/* 工作台布局：左部门树固定宽度，右成员列表自适应；整页等高，内部各自滚动 */
.org-workbench {
  display: grid;
  grid-template-columns: 296px minmax(0, 1fr);
  gap: 16px;
  height: calc(100vh - 108px);
  min-height: 560px;
}

.org-panel {
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 16px;
  overflow: hidden;
}

.org-panel-head {
  position: relative;
  padding: 14px 14px 10px;
  border-bottom: 1px solid var(--line-soft);
}

.org-panel-title {
  display: flex;
  align-items: center;
  gap: 7px;
  /* 给右上角的图标按钮留出空间，避免标题与按钮重叠 */
  padding-right: 108px;
  color: var(--ink);
}

.org-panel-title strong {
  font-size: 13.5px;
  font-weight: 650;
}

.org-panel-actions {
  display: flex;
  gap: 4px;
  position: absolute;
  top: 12px;
  right: 12px;
}

.org-search {
  margin-top: 10px;
  width: 100%;
}

.org-panel-tools {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 8px;
}

.org-tree-wrap {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 8px 8px 12px;
}

.org-tree-loading {
  padding: 8px;
}

/* 树节点：类型图标 + 名称 + 人数角标 + 悬浮“新增下级” */
.org-node {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 100%;
  min-width: 0;
  padding-right: 4px;
}

.org-node-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 7px;
  background: var(--brand-soft);
  color: var(--brand);
  flex: none;
}

.org-node-icon.big_region {
  background: #eef2ff;
  color: #4f5bd5;
}

.org-node-icon.region {
  background: #fff5e6;
  color: #b45309;
}

.org-node-icon.store {
  background: #eef7f1;
  color: #15803d;
}

.org-node-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12.5px;
}

.org-node-off {
  color: var(--rose);
  font-size: 10.5px;
}

.org-node-count {
  min-width: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background: #f2f6f3;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 17px;
  text-align: center;
  font-variant-numeric: tabular-nums;
  flex: none;
}

.org-node-add {
  display: none;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border: 0;
  border-radius: 6px;
  background: var(--brand-soft);
  color: var(--brand);
  cursor: pointer;
  flex: none;
}

.org-node:hover .org-node-add {
  display: inline-flex;
}

.org-panel-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 9px 14px;
  border-top: 1px solid var(--line-soft);
  color: var(--muted-light);
  font-size: 11px;
  background: #fbfdfc;
}

.org-panel-import {
  color: var(--brand);
}

/* Element Plus 树主题微调，贴近企业微信通讯录的紧凑行高 */
.org-tree :deep(.el-tree-node__content) {
  height: 34px;
  border-radius: 8px;
}

.org-tree :deep(.el-tree-node__content:hover) {
  background: #f2f8f5;
}

.org-tree :deep(.el-tree-node.is-current > .el-tree-node__content) {
  background: var(--brand-soft);
  color: var(--brand-deep);
  font-weight: 600;
}

@media (max-width: 960px) {
  .org-workbench {
    grid-template-columns: 1fr;
    height: auto;
  }

  .org-panel {
    max-height: 360px;
  }

  /* 窄屏改为上下堆叠时，成员面板需要固定高度，表格才能在内部滚动 */
  .org-workbench > .member-panel {
    height: min(640px, calc(100vh - 200px));
    min-height: 480px;
  }
}
</style>
