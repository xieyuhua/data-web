<template>
  <el-dialog
    :model-value="modelValue"
    title="分享与授权"
    :width="dialogWidth(640)"
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
    @open="load"
  >
    <el-tabs v-model="tab">
      <el-tab-pane label="公开链接" name="share">
        <div class="flex-gap" style="margin-bottom: 12px">
          <span class="muted">有效期</span>
          <el-select v-model="expiresDays" style="width: 140px" filterable>
            <el-option label="永久有效" :value="0" />
            <el-option label="7 天" :value="7" />
            <el-option label="30 天" :value="30" />
            <el-option label="90 天" :value="90" />
          </el-select>
          <el-button type="primary" @click="createShare">生成链接</el-button>
        </div>
        <el-alert
          v-if="justCreated"
          type="success"
          :closable="true"
          show-icon
          style="margin-bottom: 12px"
        >
          <template #title>
            <div class="flex-gap" style="flex-wrap: wrap">
              <span>链接已生成（仅显示一次）：</span>
              <el-input :model-value="justCreated" readonly style="width: 340px" size="small" />
              <el-button size="small" @click="copy(justCreated!)">复制</el-button>
              <el-button size="small" type="primary" link @click="openIt(justCreated!)">打开</el-button>
            </div>
          </template>
        </el-alert>
        <el-table :data="shares" size="small" border>
          <el-table-column label="创建时间" min-width="150">
            <template #default="{ row }">{{ fmt(row.createdAt) }}</template>
          </el-table-column>
          <el-table-column label="过期时间" min-width="150">
            <template #default="{ row }">{{ row.expiresAt ? fmt(row.expiresAt) : '永久' }}</template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag size="small" :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '有效' : '禁用' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column width="90">
            <template #default="{ row }">
              <el-button link type="danger" @click="revokeShare(row)">撤销</el-button>
            </template>
          </el-table-column>
        </el-table>
        <div class="muted" style="margin-top: 8px">公开链接无需登录即可只读查看，不包含 SQL 和数据源信息。</div>
      </el-tab-pane>

      <el-tab-pane label="部门授权" name="depts">
        <el-alert
          v-if="!departments.length"
          type="info"
          :closable="false"
          show-icon
          title="暂无部门数据。请先在「系统设置」中同步企业微信通讯录。"
          style="margin-bottom: 12px"
        />
        <div v-else class="flex-gap" style="margin-bottom: 12px">
          <el-select v-model="deptTarget" filterable placeholder="选择部门" style="width: 260px">
            <el-option
              v-for="d in departments"
              :key="d.wecomId"
              :label="`${'　'.repeat(d.depth)}${d.name}`"
              :value="d.wecomId"
            />
          </el-select>
          <el-select v-model="deptPermission" style="width: 130px" filterable>
            <el-option label="只读查看" value="view" />
            <el-option label="可编辑" value="edit" />
          </el-select>
          <el-button type="primary" :disabled="!deptTarget" @click="addDeptGrant">授权部门</el-button>
        </div>
        <el-table :data="deptGrants" size="small" border>
          <el-table-column label="部门" min-width="200">
            <template #default="{ row }">{{ row.deptName || `部门 #${row.wecomDeptId}` }}</template>
          </el-table-column>
          <el-table-column label="权限" width="110">
            <template #default="{ row }">
              <el-tag size="small" :type="row.permission === 'edit' ? 'warning' : 'info'">
                {{ row.permission === 'edit' ? '可编辑' : '只读' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="授权时间" min-width="150">
            <template #default="{ row }">{{ fmt(row.createdAt) }}</template>
          </el-table-column>
          <el-table-column width="90">
            <template #default="{ row }">
              <el-button link type="danger" @click="removeDeptGrant(row)">取消</el-button>
            </template>
          </el-table-column>
        </el-table>
        <div class="muted" style="margin-top: 8px">
          授权给上级部门后，其所有子部门员工均可查看；员工部门来自企业微信通讯录同步。
        </div>
      </el-tab-pane>

      <el-tab-pane label="用户授权" name="grants">
        <div class="flex-gap" style="margin-bottom: 12px">
          <el-select v-model="grantUser" filterable placeholder="选择用户" style="width: 220px">
            <el-option v-for="u in users" :key="u.id" :label="`${u.displayName || u.username} (${u.username})`" :value="u.id" />
          </el-select>
          <el-select v-model="grantPermission" style="width: 130px" filterable>
            <el-option label="只读查看" value="view" />
            <el-option label="可编辑" value="edit" />
          </el-select>
          <el-button type="primary" @click="addGrant">授权</el-button>
        </div>
        <el-table :data="grants" size="small" border>
          <el-table-column label="用户" min-width="160">
            <template #default="{ row }">{{ userName(row.userId) }}</template>
          </el-table-column>
          <el-table-column label="权限" width="110">
            <template #default="{ row }">
              <el-tag size="small" :type="row.permission === 'edit' ? 'warning' : 'info'">
                {{ row.permission === 'edit' ? '可编辑' : '只读' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="授权时间" min-width="150">
            <template #default="{ row }">{{ fmt(row.createdAt) }}</template>
          </el-table-column>
          <el-table-column width="90">
            <template #default="{ row }">
              <el-button link type="danger" @click="removeGrant(row)">取消</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import dayjs from 'dayjs'
import { dashboardApi, userApi, wecomApi } from '@/api'
import { dialogWidth } from '@/utils/responsive'
import type { Dashboard, Grant, ShareLink, User } from '@/types'

const props = defineProps<{
  modelValue: boolean
  dashboard?: Dashboard | null
}>()
const emit = defineEmits<{ (e: 'update:modelValue', v: boolean): void }>()

const tab = ref('share')
const shares = ref<ShareLink[]>([])
const grants = ref<Grant[]>([])
const users = ref<User[]>([])
const expiresDays = ref(0)
const justCreated = ref<string | null>(null)
const grantUser = ref<number | null>(null)
const grantPermission = ref('view')
const departments = ref<Array<{ wecomId: number; name: string; depth: number }>>([])
const deptGrants = ref<Array<any>>([])
const deptTarget = ref<number | null>(null)
const deptPermission = ref('view')

function fmt(t?: string) {
  return t ? dayjs(t).format('YYYY-MM-DD HH:mm') : ''
}

function userName(id: number) {
  const u = users.value.find((x) => x.id === id)
  return u ? `${u.displayName || u.username} (${u.username})` : `#${id}`
}

async function load() {
  if (!props.dashboard) return
  justCreated.value = null
  const [s, g] = await Promise.all([
    dashboardApi.shares(props.dashboard.id),
    dashboardApi.grants(props.dashboard.id),
  ])
  shares.value = s
  grants.value = g
  try {
    const res = await userApi.list({ page: 1, pageSize: 200 })
    users.value = res.items
  } catch {
    users.value = []
  }
  await loadDeptGrants()
}

async function loadDeptGrants() {
  if (!props.dashboard) return
  try {
    departments.value = await wecomApi.departments()
  } catch {
    departments.value = []
  }
  try {
    deptGrants.value = await wecomApi.deptGrants(props.dashboard.id)
  } catch {
    deptGrants.value = []
  }
}

async function addDeptGrant() {
  if (!props.dashboard || !deptTarget.value) {
    ElMessage.warning('请选择部门')
    return
  }
  await wecomApi.addDeptGrant(props.dashboard.id, deptTarget.value, deptPermission.value)
  ElMessage.success('已授权部门')
  deptGrants.value = await wecomApi.deptGrants(props.dashboard.id)
}

async function removeDeptGrant(row: any) {
  if (!props.dashboard) return
  await wecomApi.removeDeptGrant(props.dashboard.id, row.id)
  ElMessage.success('已取消部门授权')
  deptGrants.value = await wecomApi.deptGrants(props.dashboard.id)
}

async function createShare() {
  if (!props.dashboard) return
  const link = await dashboardApi.createShare(props.dashboard.id, expiresDays.value)
  const origin = location.origin
  justCreated.value = link.token ? `${origin}/share/${link.token}` : ''
  await refreshShares()
}

async function refreshShares() {
  if (!props.dashboard) return
  shares.value = await dashboardApi.shares(props.dashboard.id)
}

async function revokeShare(row: ShareLink) {
  if (!props.dashboard) return
  await dashboardApi.revokeShare(props.dashboard.id, row.id)
  ElMessage.success('已撤销')
  await refreshShares()
}

async function addGrant() {
  if (!props.dashboard || !grantUser.value) {
    ElMessage.warning('请选择用户')
    return
  }
  await dashboardApi.addGrant(props.dashboard.id, grantUser.value, grantPermission.value)
  ElMessage.success('已授权')
  grants.value = await dashboardApi.grants(props.dashboard.id)
}

async function removeGrant(row: Grant) {
  if (!props.dashboard) return
  await dashboardApi.removeGrant(props.dashboard.id, row.id)
  ElMessage.success('已取消授权')
  grants.value = await dashboardApi.grants(props.dashboard.id)
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制')
  } catch {
    ElMessage.warning('复制失败，请手动选择复制')
  }
}

function openIt(url: string) {
  window.open(url, '_blank')
}
</script>
