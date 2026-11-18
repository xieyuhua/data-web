<template>
  <div class="page-card">
    <div class="page-header">
      <span class="page-title">审计日志</span>
      <div class="flex-gap">
        <el-select v-model="action" clearable placeholder="全部动作" style="width: 190px" @change="load" filterable>
          <el-option v-for="a in actions" :key="a" :label="actionLabel(a)" :value="a" />
        </el-select>
        <el-button :icon="Refresh" @click="load">刷新</el-button>
      </div>
    </div>

    <div class="table-scroll">
      <el-table :data="list" v-loading="loading" border stripe size="small">
      <el-table-column label="时间" width="170">
        <template #default="{ row }">{{ fmt(row.createdAt) }}</template>
      </el-table-column>
      <el-table-column label="动作" width="190">
        <template #default="{ row }">
          <el-tag size="small" :type="tagType(row.action)" effect="plain">{{ actionLabel(row.action) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="resourceType" label="资源" width="120" />
      <el-table-column prop="resourceId" label="资源 ID" width="90" />
      <el-table-column label="用户 ID" width="90">
        <template #default="{ row }">{{ row.userId || '-' }}</template>
      </el-table-column>
      <el-table-column prop="ip" label="IP" width="130" />
      <el-table-column prop="detailJson" label="详情" min-width="240" show-overflow-tooltip />
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
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { auditApi } from '@/api'

const list = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 50
const action = ref('')
const loading = ref(false)

const actions = [
  'login', 'login_failed', 'datasource_create', 'datasource_update', 'datasource_delete',
  'dataset_create', 'dataset_delete', 'chart_delete', 'dashboard_create', 'dashboard_grant',
  'share_create', 'share_revoke', 'user_password_reset',
]

const labels: Record<string, string> = {
  login: '登录成功',
  login_failed: '登录失败',
  datasource_create: '新建数据源',
  datasource_update: '修改数据源',
  datasource_delete: '删除数据源',
  dataset_create: '新建数据集',
  dataset_delete: '删除数据集',
  chart_delete: '删除图表',
  dashboard_create: '新建仪表盘',
  dashboard_grant: '仪表盘授权',
  share_create: '创建分享',
  share_revoke: '撤销分享',
  user_password_reset: '重置密码',
}

function actionLabel(a: string) {
  return labels[a] || a
}

function tagType(a: string) {
  if (a === 'login_failed') return 'danger'
  if (a.endsWith('_delete') || a.endsWith('_revoke')) return 'warning'
  return 'info'
}

function fmt(t: string) {
  return t ? dayjs(t).format('YYYY-MM-DD HH:mm:ss') : ''
}

async function load() {
  loading.value = true
  try {
    const res = await auditApi.list({ page: page.value, pageSize, action: action.value })
    list.value = res.items
    total.value = res.total
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
