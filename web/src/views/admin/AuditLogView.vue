<!-- 审计日志页：按操作人、动作与结果筛选查看登录、变更、导出及分享访问记录。 -->
<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  AlertTriangle,
  ChevronLeft,
  ChevronRight,
  ClipboardList,
  RefreshCw,
  ScrollText,
  Search,
  ShieldCheck,
  Users,
} from 'lucide-vue-next'
import { ElMessage } from 'element-plus'
import { api, getErrorMessage } from '../../api/client'
import EmptyBlock from '../../components/EmptyBlock.vue'

const items = ref([])
const stats = ref({ total: 0, today: 0, failures: 0, users: 0 })
const actions = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(true)
const refreshing = ref(false)

const filters = reactive({
  keyword: '',
  action: '',
  status: '',
  range: [],
})

const detailVisible = ref(false)
const current = ref(null)

const actionLabels = {
  GET: '查询',
  POST: '新增',
  PUT: '修改',
  PATCH: '更新',
  DELETE: '删除',
  LOGIN: '登录',
  LOGIN_FAILED: '登录失败',
}

const actionOptions = computed(() => {
  const known = actions.value.map((item) => ({ value: item, label: actionLabels[item] || item }))
  const extra = Object.keys(actionLabels)
    .filter((key) => !actions.value.includes(key))
    .map((key) => ({ value: key, label: actionLabels[key] }))
  return [...known, ...extra]
})

const visiblePages = computed(() => {
  const last = Math.max(1, Math.ceil(total.value / pageSize.value))
  const start = Math.max(1, Math.min(page.value - 2, last - 4))
  const end = Math.min(last, start + 4)
  const pages = []
  for (let index = start; index <= end; index += 1) pages.push(index)
  return pages
})

const lastPage = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

// 操作类型码转中文名称
function actionLabel(action) {
  return actionLabels[action] || action || '—'
}

// 操作类型对应的标签样式
function actionTagClass(action) {
  if (action === 'DELETE') return 'tag-red'
  if (action === 'LOGIN_FAILED') return 'tag-red'
  if (action === 'POST') return 'tag-blue'
  if (action === 'PUT' || action === 'PATCH') return 'tag-amber'
  if (action === 'LOGIN') return 'tag-green'
  return ''
}

// 结果状态对应的标签样式
function statusTagClass(status) {
  const code = Number(status) || 0
  if (code >= 500) return 'tag-red'
  if (code >= 400) return 'tag-amber'
  if (code >= 200 && code < 300) return 'tag-green'
  return ''
}

// 解析详情 JSON，异常时返回原始文本
function parseDetail(row) {
  if (row?.detail) {
    try {
      return JSON.parse(row.detail)
    } catch {
      /* ignore */
    }
  }
  return {}
}

// 展示请求路径（优先取详情中的 path）
function rowPath(row) {
  return row.path || parseDetail(row).path || row.resource || '—'
}

// 格式化详情文本用于列表展示
function formattedDetail(row) {
  const detail = parseDetail(row)
  if (!Object.keys(detail).length) return row.detail || '无附加信息'
  return JSON.stringify(detail, null, 2)
}

// 格式化审计时间
function formatTime(value) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return String(value)
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date)
}

// 按筛选条件加载审计日志
async function load(silent = false) {
  if (silent) refreshing.value = true
  else loading.value = true
  try {
    const params = new URLSearchParams()
    params.set('page', String(page.value))
    params.set('page_size', String(pageSize.value))
    if (filters.keyword.trim()) params.set('keyword', filters.keyword.trim())
    if (filters.action) params.set('action', filters.action)
    if (filters.status) params.set('status', filters.status)
    if (Array.isArray(filters.range) && filters.range.length === 2) {
      params.set('start', filters.range[0])
      params.set('end', filters.range[1])
    }
    const data = await api.get(`/admin/audit-logs?${params.toString()}`)
    items.value = data?.items || []
    total.value = data?.total || 0
    stats.value = data?.stats || { total: 0, today: 0, failures: 0, users: 0 }
    actions.value = data?.actions || []
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

// 执行查询并回到第一页
function search() {
  page.value = 1
  load()
}

// 重置全部筛选条件
function resetFilters() {
  filters.keyword = ''
  filters.action = ''
  filters.status = ''
  filters.range = []
  page.value = 1
  load()
}

// 分页跳转
function goPage(target) {
  const next = Math.min(lastPage.value, Math.max(1, target))
  if (next === page.value) return
  page.value = next
  load()
}

// 打开日志详情弹窗
function openDetail(row) {
  current.value = row
  detailVisible.value = true
}

onMounted(load)
</script>

<template>
  <div>
    <div class="metric-mini-grid">
      <div class="metric-mini">
        <span>日志总数</span>
        <strong>{{ stats.total }}</strong>
      </div>
      <div class="metric-mini">
        <span>今日操作</span>
        <strong>{{ stats.today }}</strong>
      </div>
      <div class="metric-mini">
        <span>异常请求</span>
        <strong>{{ stats.failures }}</strong>
      </div>
      <div class="metric-mini">
        <span>涉及账号</span>
        <strong>{{ stats.users }}</strong>
      </div>
    </div>

    <div class="scope-note">
      <ShieldCheck :size="14" />
      <span>
        审计日志自动记录后台写操作、登录、导出与分享访问；分享 Token 与敏感凭据在落库前已脱敏。
      </span>
    </div>

    <div class="panel">
      <div class="table-toolbar" style="flex-wrap: wrap; gap: 10px">
        <div class="table-toolbar-left" style="flex-wrap: wrap">
          <div class="search-box">
            <Search :size="14" />
            <input
              v-model="filters.keyword"
              type="search"
              placeholder="搜索账号、路径"
              @keyup.enter="search"
            />
          </div>
          <el-select
            v-model="filters.action"
            placeholder="全部操作"
            clearable
            size="small"
            style="width: 130px"
            @change="search"
          >
            <el-option v-for="item in actionOptions" :key="item.value" :value="item.value" :label="item.label" />
          </el-select>
          <el-select
            v-model="filters.status"
            placeholder="全部状态"
            clearable
            size="small"
            style="width: 130px"
            @change="search"
          >
            <el-option value="success" label="成功 (2xx)" />
            <el-option value="failed" label="失败 (4xx+)" />
            <el-option value="server" label="异常 (5xx+)" />
          </el-select>
          <el-date-picker
            v-model="filters.range"
            type="daterange"
            size="small"
            value-format="YYYY-MM-DD"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            style="width: 240px"
            @change="search"
          />
          <button class="btn btn-secondary btn-sm" type="button" @click="search">查询</button>
          <button class="btn btn-ghost btn-sm" type="button" @click="resetFilters">重置</button>
        </div>
        <div class="inline-actions">
          <button class="btn btn-secondary btn-sm" type="button" @click="load(true)">
            <RefreshCw :size="13" :class="{ spin: refreshing }" /> 刷新
          </button>
        </div>
      </div>

      <div v-if="loading" style="padding: 20px">
        <div class="skeleton" style="height: 360px" />
      </div>

      <template v-else-if="items.length">
        <el-table :data="items" style="width: 100%" @row-click="openDetail">
          <el-table-column label="时间" width="180">
            <template #default="{ row }">
              <span style="font-variant-numeric: tabular-nums">{{ formatTime(row.created_at) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="账号" width="130">
            <template #default="{ row }">
              <span v-if="row.username">{{ row.username }}</span>
              <span v-else style="color: var(--muted-light)">匿名访问</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="100">
            <template #default="{ row }">
              <span class="tag" :class="actionTagClass(row.action)">{{ actionLabel(row.action) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="请求方式" width="95">
            <template #default="{ row }">
              <code class="audit-method">{{ row.method || parseDetail(row).method || '—' }}</code>
            </template>
          </el-table-column>
          <el-table-column label="路径" min-width="240" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="audit-path">{{ rowPath(row) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <span class="tag" :class="statusTagClass(row.status)">
                {{ row.status || parseDetail(row).status || '—' }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="耗时" width="90">
            <template #default="{ row }">
              <span style="font-variant-numeric: tabular-nums">
                {{ row.duration_ms ? `${row.duration_ms} ms` : '—' }}
              </span>
            </template>
          </el-table-column>
          <el-table-column prop="ip" label="IP" width="140" show-overflow-tooltip />
        </el-table>

        <div class="audit-pager">
          <span>共 {{ total }} 条 · 第 {{ page }} / {{ lastPage }} 页</span>
          <div class="inline-actions">
            <button class="icon-btn" type="button" :disabled="page <= 1" @click="goPage(page - 1)">
              <ChevronLeft :size="14" />
            </button>
            <button
              v-for="item in visiblePages"
              :key="item"
              class="audit-page-btn"
              :class="{ active: item === page }"
              type="button"
              @click="goPage(item)"
            >
              {{ item }}
            </button>
            <button class="icon-btn" type="button" :disabled="page >= lastPage" @click="goPage(page + 1)">
              <ChevronRight :size="14" />
            </button>
            <el-select v-model="pageSize" size="small" style="width: 100px" @change="search">
              <el-option :value="20" label="20 条/页" />
              <el-option :value="50" label="50 条/页" />
              <el-option :value="100" label="100 条/页" />
            </el-select>
          </div>
        </div>
      </template>

      <EmptyBlock
        v-else
        title="暂无审计记录"
        description="当账号登录、修改配置、导出数据或访问分享链接时，会自动记录在这里。"
      />
    </div>

    <div class="security-grid">
      <div class="security-card">
        <div class="security-icon"><ScrollText :size="16" /></div>
        <h3>关键操作留痕</h3>
        <p>新增、修改、删除与导入导出全部落库，包含账号、IP、状态码与耗时。</p>
      </div>
      <div class="security-card">
        <div class="security-icon"><ClipboardList :size="16" /></div>
        <h3>敏感信息脱敏</h3>
        <p>分享链接 Token 仅存哈希，审计日志中的访问路径同样以占位符替换。</p>
      </div>
      <div class="security-card">
        <div class="security-icon"><AlertTriangle :size="16" /></div>
        <h3>异常快速定位</h3>
        <p>按状态码与账号筛选失败请求，快速定位越权尝试与接口异常。</p>
      </div>
    </div>

    <el-drawer v-model="detailVisible" title="审计详情" size="520px">
      <template v-if="current">
        <div class="audit-detail-grid">
          <div><span>记录 ID</span><strong>{{ current.id }}</strong></div>
          <div><span>时间</span><strong>{{ formatTime(current.created_at) }}</strong></div>
          <div><span>账号</span><strong>{{ current.username || '匿名访问' }}</strong></div>
          <div><span>IP</span><strong>{{ current.ip || '—' }}</strong></div>
          <div><span>操作</span><strong>{{ actionLabel(current.action) }}</strong></div>
          <div><span>请求方式</span><strong>{{ current.method || parseDetail(current).method || '—' }}</strong></div>
          <div><span>状态码</span><strong>{{ current.status || parseDetail(current).status || '—' }}</strong></div>
          <div><span>耗时</span><strong>{{ current.duration_ms ? `${current.duration_ms} ms` : '—' }}</strong></div>
          <div style="grid-column: 1 / -1"><span>路径</span><strong class="audit-path">{{ rowPath(current) }}</strong></div>
          <div style="grid-column: 1 / -1"><span>资源</span><strong>{{ current.resource || '—' }}</strong></div>
        </div>
        <div class="audit-detail-block">
          <span>原始明细</span>
          <pre>{{ formattedDetail(current) }}</pre>
        </div>
      </template>
    </el-drawer>
  </div>
</template>
