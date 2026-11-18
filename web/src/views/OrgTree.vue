<template>
  <div class="org-page">
    <div class="page-card org-aside" v-loading="loadingDepts">
      <div class="aside-head">
        <span class="page-title">组织架构</span>
        <el-tag v-if="wecomEnabled" size="small" type="success" effect="plain">企业微信</el-tag>
        <el-tag v-else size="small" type="info" effect="plain">未同步</el-tag>
      </div>

      <el-input v-model="deptKeyword" placeholder="搜索部门" clearable size="small" class="dept-search" />

      <div class="dept-actions">
        <el-button v-if="wecomEnabled" size="small" :icon="Refresh" :loading="syncing" @click="syncNow">
          同步通讯录
        </el-button>
        <el-dropdown trigger="click" @command="onBulk">
          <el-button size="small" :icon="Upload">导入导出</el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="export">导出组织架构</el-dropdown-item>
              <el-dropdown-item command="import">导入并核对</el-dropdown-item>
              <el-dropdown-item command="report">生成核对报告</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <span v-if="lastSync" class="muted last-sync">{{ lastSync }}</span>
      </div>

      <el-tree
        ref="treeRef"
        :data="tree"
        :props="{ label: 'name', children: 'children' }"
        node-key="key"
        highlight-current
        default-expand-all
        :expand-on-click-node="false"
        :filter-node-method="filterNode"
        class="dept-tree"
        @node-click="onDeptClick"
      >
        <template #default="{ data }">
          <span class="tree-node">
            <span class="tree-name">{{ data.name }}</span>
            <span class="tree-count">{{ data.total }}</span>
          </span>
        </template>
      </el-tree>

      <el-empty v-if="!tree.length && !loadingDepts" description="暂无组织数据" :image-size="64">
        <div class="muted empty-hint">
          请在 <code>config.yaml</code> 启用企业微信后点击上方同步。
        </div>
      </el-empty>
    </div>

    <div class="page-card org-main">
      <div class="page-header">
        <div class="flex-gap">
          <span class="page-title">{{ currentDept ? currentDept.name : '全部成员' }}</span>
          <el-tag v-if="scopeLabel" size="small" effect="plain">{{ scopeLabel }}</el-tag>
          <span class="muted">{{ filtered.length }} 人</span>
        </div>
        <div class="flex-gap">
          <el-input
            v-model="memberKeyword"
            placeholder="搜索姓名/账号/职位"
            clearable
            size="small"
            class="member-search"
          />
          <el-select v-model="sourceFilter" clearable placeholder="全部来源" size="small" style="width: 120px" filterable>
            <el-option label="企业微信" value="wecom" />
            <el-option label="本地" value="local" />
          </el-select>
          <el-checkbox v-model="includeChildren" size="small" :disabled="!currentDept">
            含子部门
          </el-checkbox>
        </div>
      </div>

      <div class="table-scroll">
        <el-table :data="paged" border stripe size="small" max-height="calc(100vh - 280px)">
          <el-table-column label="姓名" min-width="120" fixed>
            <template #default="{ row }">
              <div class="member-cell">
                <el-avatar :size="24" class="member-avatar">{{ (row.displayName || row.username).slice(0, 1) }}</el-avatar>
                <span>{{ row.displayName || row.username }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="username" label="账号" min-width="130" show-overflow-tooltip />
          <el-table-column label="部门" min-width="180">
            <template #default="{ row }">
              <el-tag v-for="n in deptNames(row)" :key="n" size="small" effect="plain" class="dept-tag">{{ n }}</el-tag>
              <span v-if="!deptNames(row).length" class="muted">-</span>
            </template>
          </el-table-column>
          <el-table-column label="职位" min-width="130" show-overflow-tooltip>
            <template #default="{ row }">{{ row.position || '-' }}</template>
          </el-table-column>
          <el-table-column label="角色" width="100">
            <template #default="{ row }">
              <el-tag size="small" :type="row.role === 'admin' ? 'danger' : 'info'" effect="plain">
                {{ row.role === 'admin' ? '管理员' : '成员' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag size="small" :type="row.enabled ? 'success' : 'info'">
                {{ row.enabled ? '在职' : '停用' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="来源" width="100">
            <template #default="{ row }">
              <el-tag v-if="row.source === 'wecom'" size="small" type="primary" effect="plain">企微</el-tag>
              <el-tag v-else size="small" type="info" effect="plain">本地</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="email" label="邮箱" min-width="180" show-overflow-tooltip>
            <template #default="{ row }">{{ row.email || '-' }}</template>
          </el-table-column>
        </el-table>
      </div>

      <el-pagination
        v-if="filtered.length > pageSize"
        class="pager"
        layout="total, prev, pager, next"
        :total="filtered.length"
        :page-size="pageSize"
        :current-page="page"
        @current-change="(p: number) => { page = p }"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Upload } from '@element-plus/icons-vue'
import type { ElTree } from 'element-plus'
import { authApi, territoryApi, userApi, wecomApi } from '@/api'
import { dialogWidth } from '@/utils/responsive'
import type { ImportResult } from '@/api'

interface DeptNode {
  key: string
  name: string
  wecomId: number
  parentWecomId: number
  own: number
  total: number
  members: any[]
  children: DeptNode[]
}

const loadingDepts = ref(false)
const fileInput = ref<HTMLInputElement>()
const importDialog = ref(false)
const importResult = ref<ImportResult | null>(null)
let pendingMode: 'import' | 'report' = 'import'

function onBulk(cmd: string) {
  if (cmd === 'export') {
    territoryApi.exportOrg()
    return
  }
  pendingMode = cmd === 'report' ? 'report' : 'import'
  fileInput.value?.click()
}

async function onFilePicked(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (pendingMode === 'report') {
    await territoryApi.orgImportReport(file)
    return
  }
  importResult.value = await territoryApi.importOrg(file)
  importDialog.value = true
  await load()
}
const syncing = ref(false)
const wecomEnabled = ref(false)
const lastSync = ref('')

const departments = ref<Array<{ wecomId: number; name: string; parentWecomId: number; depth: number }>>([])
const members = ref<any[]>([])
const treeRef = ref<InstanceType<typeof ElTree>>()

const deptKeyword = ref('')
const currentDept = ref<DeptNode | null>(null)
const includeChildren = ref(true)
const memberKeyword = ref('')
const sourceFilter = ref('')
const page = ref(1)
const pageSize = 50

const deptNameByID = computed(() => {
  const m = new Map<number, string>()
  for (const d of departments.value) m.set(d.wecomId, d.name)
  return m
})

function deptIDsOf(member: any): number[] {
  if (!member.wecomDeptIds) return []
  try {
    return JSON.parse(member.wecomDeptIds)
  } catch {
    return []
  }
}

function deptNames(member: any): string[] {
  return deptIDsOf(member).map((id) => deptNameByID.value.get(id) || `#${id}`)
}

/** Build the tree with per-department member lists (members may hold several depts). */
const tree = computed<DeptNode[]>(() => {
  const nodes = new Map<number, DeptNode>()
  for (const d of departments.value) {
    nodes.set(d.wecomId, {
      key: `d${d.wecomId}`,
      name: d.name,
      wecomId: d.wecomId,
      parentWecomId: d.parentWecomId,
      own: 0,
      total: 0,
      members: [],
      children: [],
    })
  }
  const roots: DeptNode[] = []
  for (const d of departments.value) {
    const node = nodes.get(d.wecomId)!
    const parent = d.parentWecomId ? nodes.get(d.parentWecomId) : undefined
    if (parent) parent.children.push(node)
    else roots.push(node)
  }
  // Attach members to every department they belong to. Local accounts have no
  // department binding, so they never inflate org counts.
  for (const m of members.value) {
    if (!(m as any).wecomUserId && !(m as any).wecomDeptIds) continue
    for (const id of deptIDsOf(m)) {
      const node = nodes.get(id)
      if (!node) continue
      node.own++
      node.members.push(m)
    }
  }
  // A member may belong to several departments; totals must count people once.
  const seenKey = (m: any) => String(m.id ?? m.username)
  const rollup = (n: DeptNode): Map<string, any> => {
    const all = new Map<string, any>()
    for (const m of n.members) all.set(seenKey(m), m)
    for (const c of n.children) {
      for (const [k, m] of rollup(c)) all.set(k, m)
    }
    n.total = all.size
    return all
  }
  roots.forEach(rollup)
  return roots
})

const scopeLabel = computed(() => {
  if (!currentDept.value) return ''
  return includeChildren.value ? '含子部门' : '仅直属'
})

const filtered = computed(() => {
  let list = members.value
  if (currentDept.value) {
    const ids = new Set<number>()
    const collect = (n: DeptNode) => {
      ids.add(n.wecomId)
      if (includeChildren.value) n.children.forEach(collect)
    }
    collect(currentDept.value)
    list = list.filter((m) => deptIDsOf(m).some((id) => ids.has(id)))
  }
  if (sourceFilter.value) {
    list = list.filter((m) => (m.source || 'local') === sourceFilter.value)
  }
  const kw = memberKeyword.value.trim().toLowerCase()
  if (kw) {
    list = list.filter((m) =>
      [m.displayName, m.username, m.position, m.email]
        .filter(Boolean)
        .some((v: string) => String(v).toLowerCase().includes(kw)),
    )
  }
  return list
})

const paged = computed(() => filtered.value.slice((page.value - 1) * pageSize, page.value * pageSize))

watch([memberKeyword, sourceFilter, includeChildren, currentDept], () => {
  page.value = 1
})

watch(deptKeyword, (v) => treeRef.value?.filter(v))

function filterNode(value: string, data: any) {
  if (!value) return true
  return String(data.name).toLowerCase().includes(value.toLowerCase())
}

function onDeptClick(data: DeptNode) {
  currentDept.value = data
}

async function syncNow() {
  syncing.value = true
  try {
    const res = await wecomApi.sync()
    ElMessage.success(`同步完成：${res.departments} 个部门，${res.users} 名成员`)
    await load()
  } finally {
    syncing.value = false
  }
}

async function load() {
  loadingDepts.value = true
  try {
    try {
      const st = await authApi.wecomStatus()
      wecomEnabled.value = st.enabled
      if (st.lastSyncAt) {
        lastSync.value = `最近同步 ${new Date(st.lastSyncAt).toLocaleString('zh-CN')}`
      }
    } catch {
      wecomEnabled.value = false
    }
    try {
      departments.value = await wecomApi.departments()
    } catch {
      departments.value = []
    }
    try {
      const res = await userApi.list({ page: 1, pageSize: 200 })
      members.value = res.items
    } catch {
      members.value = []
    }
  } finally {
    loadingDepts.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.org-page {
  display: flex;
  gap: var(--bi-gap);
  align-items: flex-start;
}
.org-aside {
  width: 300px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 100px);
}
.aside-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.aside-head .page-title {
  font-size: 16px;
}
.dept-search {
  margin-bottom: 8px;
}
.dept-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.dept-tree {
  overflow: auto;
  flex: 1;
}
.tree-node {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding-right: 6px;
}
.tree-name {
  font-size: 13px;
}
.tree-count {
  margin-left: auto;
  font-size: 11px;
  color: #909399;
  background: #f2f3f5;
  border-radius: 8px;
  padding: 0 6px;
  line-height: 16px;
}
.last-sync {
  font-size: 11px;
}
.empty-hint {
  text-align: center;
  padding: 0 12px;
}
.org-main {
  flex: 1;
  min-width: 0;
}
.member-search {
  width: 200px;
}
.member-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}
.member-avatar {
  background: linear-gradient(135deg, #409eff, #6f42c1);
  color: #fff;
  font-size: 12px;
}
.dept-tag {
  margin: 0 4px 2px 0;
}
.pager {
  margin-top: 12px;
}

@media (max-width: 900px) {
  .org-page {
    flex-direction: column;
  }
  .org-aside {
    width: 100%;
    max-height: 320px;
  }
  .member-search {
    width: 140px;
  }
}
</style>
