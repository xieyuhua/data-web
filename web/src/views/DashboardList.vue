<template>
  <div class="page-card">
    <div class="page-header">
      <span class="page-title">仪表盘</span>
      <div class="flex-gap">
        <el-input v-model="keyword" placeholder="搜索名称" clearable class="search-input" @keyup.enter="load" @clear="load" />
        <el-button type="primary" :icon="Plus" @click="createDashboard">新建仪表盘</el-button>
      </div>
    </div>

    <div v-loading="loading" class="resp-grid dash-grid">
      <el-empty v-if="!loading && !list.length" description="暂无仪表盘，点击右上角新建" style="grid-column: 1 / -1" />
      <div v-for="d in list" :key="d.id" class="dash-card">
        <div class="dash-cover">
          <el-icon :size="38" color="#409eff"><DataBoard /></el-icon>
        </div>
        <div class="dash-body">
          <div class="dash-name" :title="d.name">{{ d.name }}</div>
          <div class="dash-desc">{{ d.description || '暂无描述' }}</div>
          <div class="dash-meta muted">更新于 {{ fmt(d.updatedAt) }}</div>
          <div class="dash-actions">
            <el-button size="small" type="primary" plain @click="router.push(`/dashboards/${d.id}/view`)">查看</el-button>
            <el-button size="small" @click="router.push(`/dashboards/${d.id}/edit`)">编辑</el-button>
            <el-dropdown trigger="click" @command="(cmd: string) => onCommand(cmd, d)">
              <el-button size="small" :icon="More" />
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="duplicate">复制仪表盘</el-dropdown-item>
                  <el-dropdown-item command="share">分享与授权</el-dropdown-item>
                  <el-dropdown-item command="delete" divided>删除</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>
      </div>
    </div>

    <el-pagination
      v-if="total > pageSize"
      style="margin-top: 16px"
      layout="total, prev, pager, next"
      :total="total"
      :page-size="pageSize"
      :current-page="page"
      @current-change="(p: number) => { page = p; load() }"
    />

    <el-dialog v-model="createVisible" title="新建仪表盘" width="420px">
      <el-form label-width="70px">
        <el-form-item label="名称">
          <el-input v-model="newName" placeholder="例如：销售分析" @keyup.enter="doCreate" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="newDesc" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="doCreate">创建并编辑</el-button>
      </template>
    </el-dialog>

    <ShareDialog v-model="shareVisible" :dashboard="shareTarget" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { More, Plus } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { dashboardApi } from '@/api'
import type { Dashboard } from '@/types'
import ShareDialog from '@/components/dashboard/ShareDialog.vue'

const router = useRouter()
const list = ref<Dashboard[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 12
const keyword = ref('')
const loading = ref(false)

const createVisible = ref(false)
const newName = ref('')
const newDesc = ref('')
const creating = ref(false)

const shareVisible = ref(false)
const shareTarget = ref<Dashboard | null>(null)

function fmt(t: string) {
  return t ? dayjs(t).format('YYYY-MM-DD HH:mm') : ''
}

async function load() {
  loading.value = true
  try {
    const res = await dashboardApi.list({ page: page.value, pageSize, keyword: keyword.value })
    list.value = res.items
    total.value = res.total
  } finally {
    loading.value = false
  }
}

function createDashboard() {
  newName.value = ''
  newDesc.value = ''
  createVisible.value = true
}

async function doCreate() {
  if (!newName.value.trim()) {
    ElMessage.warning('请输入名称')
    return
  }
  creating.value = true
  try {
    const d = await dashboardApi.create({ name: newName.value.trim(), description: newDesc.value })
    createVisible.value = false
    router.push(`/dashboards/${d.id}/edit`)
  } finally {
    creating.value = false
  }
}

function onCommand(cmd: string, d: Dashboard) {
  if (cmd === 'share') {
    shareTarget.value = d
    shareVisible.value = true
  } else if (cmd === 'delete') {
    deleteDashboard(d)
  } else if (cmd === 'duplicate') {
    duplicateDashboard(d)
  }
}

async function duplicateDashboard(d: Dashboard) {
  await dashboardApi.duplicate(d.id)
  ElMessage.success('已复制')
  await load()
}

async function deleteDashboard(d: Dashboard) {
  await ElMessageBox.confirm(`确定删除仪表盘「${d.name}」吗？`, '删除确认', { type: 'warning' })
  await dashboardApi.remove(d.id)
  ElMessage.success('已删除')
  await load()
}

onMounted(load)
</script>

<style scoped>
.dash-grid {
  min-height: 200px;
}
.search-input {
  width: 220px;
}
@media (max-width: 640px) {
  .search-input {
    width: 100%;
  }
}
.dash-card {
  border: 1px solid #ebeef5;
  border-radius: 10px;
  overflow: hidden;
  background: #fff;
  transition: box-shadow 0.2s, transform 0.2s;
}
.dash-card:hover {
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.08);
  transform: translateY(-2px);
}
.dash-cover {
  height: 88px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #eff6ff, #f5f3ff);
}
.dash-body {
  padding: 12px 14px 14px;
}
.dash-name {
  font-weight: 600;
  font-size: 15px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dash-desc {
  margin-top: 4px;
  color: #606266;
  font-size: 12px;
  height: 32px;
  overflow: hidden;
}
.dash-meta {
  margin-top: 4px;
}
.dash-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 10px;
}
</style>
