<!-- 看板列表页（首页）：展示当前账号有权查看的全部看板，支持进入与新建。 -->
<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  ArrowUpRight,
  Building2,
  Clock,
  GitBranch,
  Network,
  Pin,
  Plus,
  Search,
  ShieldCheck,
  Star,
  Store,
  TrendingUp,
} from 'lucide-vue-next'
import { ElMessage } from 'element-plus'
import { api, getErrorMessage } from '../../api/client'
import { useAuthStore } from '../../stores/auth'
import { statusMeta, formatDate } from '../../utils/format'
import { flattenOrgs, dataScopeLabel } from '../../utils/org'
import EmptyBlock from '../../components/EmptyBlock.vue'

const auth = useAuthStore()
const router = useRouter()

const dashboards = ref([])
const orgTree = ref([])
const loading = ref(true)
const keyword = ref('')
const category = ref('全部')
// 排序方式：default 默认 / recent 最近浏览 / updated 最近更新 / views 访问最多 / name 名称
const sortKey = ref('default')
// 仅展示已收藏的看板
const onlyFavorite = ref(false)

const sortOptions = [
  { value: 'default', label: '默认排序' },
  { value: 'recent', label: '最近浏览' },
  { value: 'updated', label: '最近更新' },
  { value: 'views', label: '访问最多' },
  { value: 'name', label: '名称排序' },
]

const orgs = computed(() => flattenOrgs(orgTree.value))
const storeCount = computed(() => orgs.value.filter((org) => org.type === 'store').length)
const regionCount = computed(() => orgs.value.filter((org) => org.type === 'region').length)
const scopeLabel = computed(() => dataScopeLabel(auth.dataScope))

const categories = computed(() => {
  const set = new Set(dashboards.value.map((item) => item.category).filter(Boolean))
  return ['全部', ...Array.from(set)]
})

const filtered = computed(() => {
  const text = keyword.value.trim().toLowerCase()
  return dashboards.value.filter((item) => {
    const matchCategory = category.value === '全部' || item.category === category.value
    const matchFavorite = !onlyFavorite.value || item.favorite
    const matchText =
      !text ||
      item.name.toLowerCase().includes(text) ||
      (item.description || '').toLowerCase().includes(text)
    return matchCategory && matchFavorite && matchText
  })
})

// 排序规则：
// - 默认：置顶 > 收藏 > 最近更新（常用 BI 首页的默认习惯）
// - 最近浏览：按个人最近打开时间倒序，未打开过的排在后面
// - 其余为单一维度排序
const sorted = computed(() => {
  const list = [...filtered.value]
  const updatedDesc = (a, b) => new Date(b.updated_at || 0) - new Date(a.updated_at || 0)
  const viewedDesc = (a, b) => {
    const left = a.last_viewed_at ? new Date(a.last_viewed_at).getTime() : 0
    const right = b.last_viewed_at ? new Date(b.last_viewed_at).getTime() : 0
    return right - left
  }
  switch (sortKey.value) {
    case 'recent':
      return list.sort((a, b) => viewedDesc(a, b) || updatedDesc(a, b))
    case 'updated':
      return list.sort(updatedDesc)
    case 'views':
      return list.sort((a, b) => (b.view_count || 0) - (a.view_count || 0) || updatedDesc(a, b))
    case 'name':
      return list.sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))
    default:
      return list.sort(
        (a, b) =>
          Number(Boolean(b.pinned)) - Number(Boolean(a.pinned)) ||
          Number(Boolean(b.favorite)) - Number(Boolean(a.favorite)) ||
          updatedDesc(a, b),
      )
  }
})

// 收藏数量与最近浏览看板（最多 4 个），用于首页顶部快捷入口
const favoriteCount = computed(() => dashboards.value.filter((item) => item.favorite).length)
const recent = computed(() =>
  dashboards.value
    .filter((item) => item.last_viewed_at)
    .sort((a, b) => new Date(b.last_viewed_at) - new Date(a.last_viewed_at))
    .slice(0, 4),
)

const canCreate = computed(() => auth.can('dashboard:create'))

const cardBars = [
  [38, 62, 48, 76, 58, 88, 70],
  [52, 44, 68, 58, 82, 64, 90],
  [46, 70, 56, 84, 62, 74, 96],
]

// 加载当前账号可见的看板列表
async function load() {
  loading.value = true
  try {
    const [list, tree] = await Promise.all([
      api.get('/dashboards'),
      api.get('/orgs/tree').catch(() => []),
    ])
    dashboards.value = list || []
    orgTree.value = tree || []
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

// 打开指定看板
function open(item) {
  router.push(`/dashboards/${item.id}`)
}

// 切换收藏 / 置顶；只提交发生变化的字段，后端返回最新偏好
async function togglePref(item, field, event) {
  event?.stopPropagation?.()
  const next = !item[field]
  try {
    const result = await api.put(`/dashboard-prefs/${item.id}`, { [field]: next })
    Object.assign(item, result)
    ElMessage.success(
      field === 'favorite'
        ? next ? '已加入我的收藏' : '已取消收藏'
        : next ? '已置顶该看板' : '已取消置顶',
    )
  } catch (err) {
    ElMessage.error(getErrorMessage(err))
  }
}

// 相对时间：用于"最近浏览"区块，超过 7 天显示具体日期
function relativeTime(value) {
  if (!value) return ''
  const diff = Date.now() - new Date(value).getTime()
  if (Number.isNaN(diff) || diff < 0) return formatDate(value)
  const minute = 60 * 1000
  const hour = 60 * minute
  const day = 24 * hour
  if (diff < minute) return '刚刚'
  if (diff < hour) return `${Math.floor(diff / minute)} 分钟前`
  if (diff < day) return `${Math.floor(diff / hour)} 小时前`
  if (diff < 7 * day) return `${Math.floor(diff / day)} 天前`
  return formatDate(value)
}

onMounted(load)
</script>

<template>
  <div>
    <section class="overview-hero">
      <div class="hero-copy">
        <div class="eyebrow">经营数据工作台</div>
        <h2>让每一个经营决策<br />都有清晰的数据依据</h2>
        <p>
          当前账号 {{ auth.displayName }} · {{ auth.orgName || '全部组织' }}。
          所有看板查询都会自动注入组织权限，你看到的就是你被授权看到的数据边界。
        </p>
        <div class="hero-metrics">
          <div class="hero-metric">
            <span>可用看板</span>
            <strong>{{ dashboards.length }}<small>个</small></strong>
          </div>
          <div class="hero-metric">
            <span>组织节点</span>
            <strong>{{ orgs.length }}<small>个</small></strong>
          </div>
          <div class="hero-metric">
            <span>授权门店</span>
            <strong>{{ storeCount }}<small>家</small></strong>
          </div>
        </div>
        <div class="inline-actions" style="margin-top: 18px">
          <button class="btn btn-primary btn-sm" type="button" @click="router.push('/drill')">
            <Network :size="14" /> 进入组织经营下钻
          </button>
        </div>
      </div>
      <div class="hero-chart" aria-hidden="true">
        <svg viewBox="0 0 430 142" fill="none">
          <defs>
            <linearGradient id="heroFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0" stop-color="#A7F3D0" stop-opacity="0.35" />
              <stop offset="1" stop-color="#A7F3D0" stop-opacity="0" />
            </linearGradient>
          </defs>
          <path d="M0 112C30 104 45 84 72 88C99 92 112 60 140 64C168 68 178 96 206 82C234 68 246 34 274 42C302 50 312 78 340 58C368 38 392 24 430 16V142H0V112Z" fill="url(#heroFill)" />
          <path d="M0 112C30 104 45 84 72 88C99 92 112 60 140 64C168 68 178 96 206 82C234 68 246 34 274 42C302 50 312 78 340 58C368 38 392 24 430 16" stroke="#A7F3D0" stroke-width="1.6" stroke-linecap="round" />
          <g fill="#A7F3D0" opacity="0.7">
            <circle cx="206" cy="82" r="3" />
            <circle cx="340" cy="58" r="3" />
          </g>
        </svg>
      </div>
      <div class="hero-watermark">AUTHORIZED DATA SCOPE · {{ scopeLabel }}</div>
    </section>

    <div class="scope-note">
      <ShieldCheck :size="14" />
      <span>
        数据范围：<strong>{{ scopeLabel }}</strong> · 角色：<strong>{{ auth.role?.name || '—' }}</strong>
        · 可见区域 {{ regionCount }} 个、门店 {{ storeCount }} 家。看板 SQL 自动追加组织过滤条件。
      </span>
    </div>

    <div class="section-head">
      <div>
        <h2>我的看板</h2>
        <p>按业务主题浏览授权范围内的经营视图</p>
      </div>
      <div class="inline-actions">
        <div class="search-box">
          <Search :size="14" />
          <input v-model="keyword" type="search" placeholder="搜索看板名称或说明" />
        </div>
        <!-- 排序方式：默认 / 最近浏览 / 最近更新 / 访问最多 / 名称 -->
        <el-select v-model="sortKey" size="small" style="width: 128px">
          <el-option v-for="option in sortOptions" :key="option.value" :label="option.label" :value="option.value" />
        </el-select>
        <button v-if="canCreate" class="btn btn-primary btn-sm" type="button" @click="router.push('/dashboards/new')">
          <Plus :size="14" /> 新建看板
        </button>
      </div>
    </div>

    <div class="filter-pills" style="width: max-content; max-width: 100%; margin-bottom: 18px; overflow-x: auto">
      <button
        class="filter-pill favorite-pill"
        :class="{ active: onlyFavorite }"
        type="button"
        @click="onlyFavorite = !onlyFavorite"
      >
        <Star :size="11" /> 我的收藏
        <span v-if="favoriteCount" class="pill-count">{{ favoriteCount }}</span>
      </button>
      <button
        v-for="item in categories"
        :key="item"
        class="filter-pill"
        :class="{ active: category === item }"
        type="button"
        @click="category = item"
      >
        {{ item }}
      </button>
    </div>

    <!-- 最近浏览：直接回到上次查看的看板 -->
    <section v-if="recent.length" class="recent-strip">
      <div class="recent-strip-head">
        <h2><Clock :size="13" /> 最近浏览</h2>
        <button class="link-btn" type="button" @click="sortKey = 'recent'">按浏览时间排序</button>
      </div>
      <div class="recent-row">
        <button v-for="item in recent" :key="item.id" class="recent-card" type="button" @click="open(item)">
          <span class="recent-name">{{ item.name }}</span>
          <span class="recent-time">{{ relativeTime(item.last_viewed_at) }}</span>
        </button>
      </div>
    </section>

    <div v-if="loading" class="dashboard-grid">
      <div v-for="index in 6" :key="index" class="panel" style="height: 252px; padding: 18px">
        <div class="skeleton" style="height: 100%" />
      </div>
    </div>

    <div v-else-if="sorted.length || canCreate" class="dashboard-grid">
      <article v-for="(item, index) in sorted" :key="item.id" class="dashboard-card" @click="open(item)">
        <div class="card-preview">
          <!-- 收藏 / 置顶快捷操作：不触发卡片跳转 -->
          <div class="card-tools">
            <button
              class="card-tool"
              :class="{ active: item.favorite }"
              type="button"
              :title="item.favorite ? '取消收藏' : '收藏看板'"
              @click.stop="togglePref(item, 'favorite', $event)"
            >
              <Star :size="13" />
            </button>
            <button
              class="card-tool pin"
              :class="{ active: item.pinned }"
              type="button"
              :title="item.pinned ? '取消置顶' : '置顶看板'"
              @click.stop="togglePref(item, 'pinned', $event)"
            >
              <Pin :size="13" />
            </button>
          </div>
          <div class="preview-top">
            <div v-for="kpi in 3" :key="kpi" class="preview-kpi">
              <i />
              <b style="width: 58%" />
            </div>
          </div>
          <div class="preview-body">
            <div class="preview-chart" />
            <div class="preview-side">
              <div class="preview-bar">
                <i
                  v-for="(height, barIndex) in cardBars[index % cardBars.length]"
                  :key="barIndex"
                  :style="{ '--h': `${height}%` }"
                />
              </div>
            </div>
          </div>
        </div>
        <div class="dashboard-card-body">
          <div class="dashboard-card-top">
            <div>
              <h3>{{ item.name }}</h3>
              <p>{{ item.description || '授权范围内的经营数据视图' }}</p>
            </div>
            <span class="tag" :class="item.status === 'published' ? 'tag-green' : 'tag-amber'">
              {{ statusMeta(item.status).label }}
            </span>
          </div>
          <div class="dashboard-card-foot">
            <div class="card-meta">
              <span v-if="item.pinned" class="tag tag-blue" style="padding: 1px 6px">置顶</span>
              <span><Building2 :size="11" /> {{ item.category || '经营分析' }}</span>
              <span><TrendingUp :size="11" /> {{ formatDate(item.updated_at) }}</span>
              <span v-if="item.last_viewed_at"><Clock :size="11" /> {{ relativeTime(item.last_viewed_at) }}</span>
              <span><GitBranch :size="11" /> {{ item.drill_enabled ? '支持下钻' : '标准视图' }}</span>
              <span><ShieldCheck :size="11" /> {{ item.scope_mode === 'all' ? '全量数据' : '按组织层级' }}</span>
            </div>
            <span class="round-link"><ArrowUpRight :size="14" /></span>
          </div>
        </div>
      </article>

      <button v-if="canCreate" class="new-dashboard-card" type="button" @click="router.push('/dashboards/new')">
        <span class="plus"><Plus :size="19" /></span>
        <strong>新建经营看板</strong>
        <span>从空白画布或模板开始</span>
      </button>
    </div>

    <div v-else class="panel">
      <EmptyBlock
        title="没有匹配的看板"
        :description="dashboards.length ? '换一个关键词、分类或关闭「我的收藏」筛选试试。' : '你的账号还没有可见的看板，请联系管理员发布看板。'"
      >
        <button
          v-if="dashboards.length"
          class="btn btn-secondary btn-sm"
          type="button"
          @click="keyword = ''; category = '全部'; onlyFavorite = false"
        >
          清空筛选
        </button>
      </EmptyBlock>
    </div>

    <div class="security-grid">
      <div class="security-card">
        <div class="security-icon"><ShieldCheck :size="16" /></div>
        <h3>组织级数据隔离</h3>
        <p>看板 SQL 会自动展开为当前账号可访问的组织 ID 集合，越权数据在查询层就被排除。</p>
      </div>
      <div class="security-card">
        <div class="security-icon"><Store :size="16" /></div>
        <h3>总部到门店逐级下钻</h3>
        <p>集团老总看全局，大区 / 区域经理看所辖组织，店长与员工只看当前门店。</p>
      </div>
      <div class="security-card">
        <div class="security-icon"><TrendingUp :size="16" /></div>
        <h3>可复用的分析模板</h3>
        <p>成熟看板可保存为模板，新看板按模板快速创建，再按角色授权范围自动过滤。</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 卡片右上角的收藏 / 置顶快捷按钮，默认隐藏，悬浮或聚焦时出现 */
.card-tools {
  position: absolute;
  top: 9px;
  right: 9px;
  z-index: 2;
  display: flex;
  gap: 6px;
  opacity: 0;
  transform: translateY(-2px);
  transition: 0.18s ease;
}
.dashboard-card:hover .card-tools,
.card-tools:focus-within {
  opacity: 1;
  transform: none;
}
.card-tool {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: 1px solid var(--line);
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.94);
  color: var(--muted);
  cursor: pointer;
  transition: 0.16s ease;
}
.card-tool:hover {
  border-color: var(--brand);
  color: var(--brand);
}
/* 收藏选中：琥珀色；置顶选中：品牌绿，便于一眼区分 */
.card-tool.active {
  border-color: #f0c58c;
  background: var(--amber-soft);
  color: #b45309;
}
.card-tool.pin.active {
  border-color: #b8dacf;
  background: var(--brand-soft);
  color: var(--brand);
}

/* 最近浏览区块 */
.recent-strip {
  margin-bottom: 20px;
}
.recent-strip-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
.recent-strip-head h2 {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--ink);
}
.link-btn {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--brand);
  font-size: 11.5px;
  cursor: pointer;
}
.link-btn:hover {
  text-decoration: underline;
}
.recent-row {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}
.recent-card {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.72);
  text-align: left;
  cursor: pointer;
  transition: 0.18s ease;
}
.recent-card:hover {
  border-color: var(--brand);
  background: #fff;
  transform: translateY(-1px);
}
.recent-name {
  max-width: 100%;
  overflow: hidden;
  color: var(--ink);
  font-size: 11px;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.recent-time {
  color: var(--muted-light);
  font-size: 11px;
}

/* 收藏筛选胶囊 */
.favorite-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.pill-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 8px;
  background: rgba(180, 83, 9, 0.12);
  color: #b45309;
  font-size: 11px;
}

@media (max-width: 900px) {
  .recent-row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 560px) {
  .recent-row {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
