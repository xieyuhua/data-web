<!-- 分享管理页：查看、创建、复制与撤销授权分享链接，展示访问次数与到期时间。 -->
<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { Check, Copy, Link2, Plus, RefreshCw, ShieldCheck, Trash2 } from 'lucide-vue-next'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, getErrorMessage } from '../api/client'
import { formatDate } from '../utils/format'
import EmptyBlock from '../components/EmptyBlock.vue'

const shares = ref([])
const dashboards = ref([])
const loading = ref(true)
const refreshing = ref(false)
const createVisible = ref(false)
const creating = ref(false)
const created = ref(null)

const form = reactive({
  dashboard_id: null,
  expire_hours: 24,
  max_views: 0,
})

const dashboardMap = computed(() => {
  const map = {}
  for (const item of dashboards.value) map[item.id] = item.name
  return map
})

const stats = computed(() => {
  const list = shares.value
  return {
    total: list.length,
    active: list.filter((item) => shareStatus(item).key === 'active').length,
    views: list.reduce((sum, item) => sum + (item.views || 0), 0),
  }
})

// 计算分享链接状态（有效 / 已过期 / 已撤销）
function shareStatus(item) {
  if (item.revoked) return { key: 'revoked', label: '已撤销', className: 'tag-red' }
  if (item.expires_at && new Date(item.expires_at).getTime() < Date.now()) {
    return { key: 'expired', label: '已过期', className: 'tag-amber' }
  }
  if (item.max_views > 0 && item.views >= item.max_views) {
    return { key: 'exhausted', label: '次数用尽', className: 'tag-amber' }
  }
  return { key: 'active', label: '有效', className: 'tag-green' }
}

// 加载分享链接列表
async function load(silent = false) {
  if (silent) refreshing.value = true
  else loading.value = true
  try {
    const [shareList, dashboardList] = await Promise.all([
      api.get('/shares'),
      api.get('/dashboards').catch(() => []),
    ])
    shares.value = shareList || []
    dashboards.value = dashboardList || []
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

// 打开新建分享弹窗
function openCreate() {
  form.dashboard_id = dashboards.value[0]?.id || null
  form.expire_hours = 24
  form.max_views = 0
  created.value = null
  createVisible.value = true
}

// 创建新的授权分享链接
async function createShare() {
  if (!form.dashboard_id) {
    ElMessage.warning('请选择要分享的看板')
    return
  }
  creating.value = true
  try {
    created.value = await api.post('/shares', {
      dashboard_id: Number(form.dashboard_id),
      expire_hours: Number(form.expire_hours) || 24,
      max_views: Number(form.max_views) || 0,
    })
    ElMessage.success('授权链接已生成')
    await load(true)
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    creating.value = false
  }
}

// 复制分享链接到剪贴板
async function copyLink() {
  if (!created.value?.url) return
  try {
    await navigator.clipboard.writeText(created.value.url)
  } catch {
    const input = document.createElement('textarea')
    input.value = created.value.url
    document.body.appendChild(input)
    input.select()
    document.execCommand('copy')
    input.remove()
  }
  ElMessage.success('链接已复制')
}

// 撤销分享链接，撤销后立即失效
async function revoke(item) {
  try {
    await ElMessageBox.confirm('撤销后该链接会立即失效，无法恢复。', '撤销分享链接', {
      type: 'warning',
      confirmButtonText: '撤销',
      cancelButtonText: '取消',
    })
    await api.delete(`/shares/${item.id}`)
    ElMessage.success('链接已撤销')
    await load(true)
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(getErrorMessage(error))
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="section-head">
      <div>
        <h2>授权分享</h2>
        <p>链接绑定创建者的数据边界，可设置有效期和最大访问次数</p>
      </div>
      <div class="inline-actions">
        <button class="icon-btn" type="button" title="刷新" @click="load(true)">
          <RefreshCw :size="15" :class="{ spin: refreshing }" />
        </button>
        <button class="btn btn-primary btn-sm" type="button" @click="openCreate">
          <Plus :size="14" /> 创建分享链接
        </button>
      </div>
    </div>

    <div class="metric-mini-grid">
      <div class="metric-mini">
        <span>链接总数</span>
        <strong>{{ stats.total }}</strong>
      </div>
      <div class="metric-mini">
        <span>当前有效</span>
        <strong>{{ stats.active }}</strong>
      </div>
      <div class="metric-mini">
        <span>累计访问</span>
        <strong>{{ stats.views }}</strong>
      </div>
      <div class="metric-mini">
        <span>安全策略</span>
        <strong style="font-size: 16px">哈希存储</strong>
      </div>
    </div>

    <div class="scope-note">
      <ShieldCheck :size="14" />
      <span>
        分享链接不回传明文 Token，只能在创建时复制。访问时由服务端校验有效期、访问次数与创建者授权组织范围。
      </span>
    </div>

    <div class="panel">
      <div class="panel-head" style="padding: 16px 19px 12px">
        <div>
          <div class="panel-title">分享记录</div>
          <div class="panel-subtitle">包含你创建的链接；管理员可查看全部</div>
        </div>
      </div>

      <div v-if="loading" style="padding: 26px">
        <div v-for="index in 4" :key="index" class="skeleton" style="height: 46px; margin-bottom: 10px" />
      </div>

      <template v-else-if="shares.length">
        <div v-for="item in shares" :key="item.id" class="share-list-card">
          <div class="share-icon"><Link2 :size="17" /></div>
          <div class="share-main">
            <h3>{{ dashboardMap[item.dashboard_id] || `看板 #${item.dashboard_id}` }}</h3>
            <p>
              创建于 {{ formatDate(item.created_at, true) }} ·
              到期 {{ formatDate(item.expires_at, true) }}
              <span v-if="item.last_viewed_at"> · 最近访问 {{ formatDate(item.last_viewed_at, true) }}</span>
            </p>
          </div>
          <div class="share-stats">
            <div class="share-stat">
              <span>访问次数</span>
              <strong>{{ item.views }}</strong>
            </div>
            <div class="share-stat">
              <span>次数上限</span>
              <strong>{{ item.max_views || '不限' }}</strong>
            </div>
            <div class="share-stat">
              <span>状态</span>
              <strong><span class="tag" :class="shareStatus(item).className">{{ shareStatus(item).label }}</span></strong>
            </div>
          </div>
          <button
            class="btn btn-ghost btn-sm"
            type="button"
            :disabled="item.revoked"
            @click="revoke(item)"
          >
            <Trash2 :size="13" /> 撤销
          </button>
        </div>
      </template>

      <EmptyBlock v-else title="还没有分享链接" description="从经营看板生成授权链接，外部同学无需登录即可查看授权范围内的数据。">
        <button class="btn btn-primary btn-sm" type="button" @click="openCreate">
          <Plus :size="14" /> 创建第一个链接
        </button>
      </EmptyBlock>
    </div>

    <div class="security-grid">
      <div class="security-card">
        <div class="security-icon"><ShieldCheck :size="16" /></div>
        <h3>授权范围不变</h3>
        <p>看板 SQL 会按创建者的组织 ID 集合过滤，访问者无法通过修改参数越权。</p>
      </div>
      <div class="security-card">
        <div class="security-icon"><Link2 :size="16" /></div>
        <h3>可控生命周期</h3>
        <p>支持小时级有效期与最大访问次数，管理员可随时撤销链接。</p>
      </div>
      <div class="security-card">
        <div class="security-icon"><Check :size="16" /></div>
        <h3>只读展示</h3>
        <p>公共分享页不提供编辑、导出源 SQL 等能力，只展示看板查询结果。</p>
      </div>
    </div>

    <el-dialog v-model="createVisible" title="创建授权分享链接" width="520px">
      <template v-if="!created">
        <div class="share-dialog-intro">
          <div class="share-dialog-icon"><ShieldCheck :size="20" /></div>
          <div>
            <strong>链接将继承你的数据权限</strong>
            <p>访问者无需登录，但只能看到你当前账号可访问的组织数据。</p>
          </div>
        </div>
        <div class="form-grid" style="margin-top: 18px">
          <label class="field-label form-span-2">
            选择看板
            <el-select v-model="form.dashboard_id" style="width: 100%; margin-top: 8px" placeholder="请选择看板">
              <el-option v-for="item in dashboards" :key="item.id" :label="item.name" :value="item.id" />
            </el-select>
          </label>
          <label class="field-label">
            有效期
            <el-select v-model="form.expire_hours" style="width: 100%; margin-top: 8px">
              <el-option :value="1" label="1 小时" />
              <el-option :value="8" label="8 小时" />
              <el-option :value="24" label="24 小时" />
              <el-option :value="168" label="7 天" />
              <el-option :value="720" label="30 天" />
            </el-select>
          </label>
          <label class="field-label">
            最大访问次数
            <el-input-number v-model="form.max_views" :min="0" :max="100000" style="width: 100%; margin-top: 8px" />
          </label>
        </div>
        <p style="margin-top: 8px; color: var(--muted-light); font-size: 9px">访问次数填 0 表示不限制。</p>
      </template>

      <template v-else>
        <div class="share-result">
          <div class="share-result-label"><Link2 :size="13" /> 已生成的授权链接（仅显示一次）</div>
          <div class="share-result-row">
            <el-input :model-value="created.url" readonly />
            <button class="btn btn-secondary" type="button" @click="copyLink"><Copy :size="14" /> 复制</button>
          </div>
          <div class="share-result-meta">
            <span><Check :size="12" /> 到期时间：{{ formatDate(created.expires_at, true) }}</span>
            <span>最大访问：{{ created.max_views || '不限' }}</span>
          </div>
        </div>
      </template>

      <template #footer>
        <div class="inline-actions" style="justify-content: flex-end">
          <button class="btn btn-secondary" type="button" @click="createVisible = false">
            {{ created ? '关闭' : '取消' }}
          </button>
          <button v-if="!created" class="btn btn-primary" type="button" :disabled="creating" @click="createShare">
            {{ creating ? '生成中…' : '生成链接' }}
          </button>
          <button v-else class="btn btn-primary" type="button" @click="copyLink"><Copy :size="14" /> 复制链接</button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>
