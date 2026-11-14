<!-- 看板列表页（首页）：展示当前账号有权查看的全部看板，支持进入与新建。 -->
<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  ArrowUpRight,
  Building2,
  GitBranch,
  Network,
  Plus,
  Search,
  ShieldCheck,
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
    const matchText =
      !text ||
      item.name.toLowerCase().includes(text) ||
      (item.description || '').toLowerCase().includes(text)
    return matchCategory && matchText
  })
})

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
        <button v-if="canCreate" class="btn btn-primary btn-sm" type="button" @click="router.push('/dashboards/new')">
          <Plus :size="14" /> 新建看板
        </button>
      </div>
    </div>

    <div class="filter-pills" style="width: max-content; max-width: 100%; margin-bottom: 18px; overflow-x: auto">
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

    <div v-if="loading" class="dashboard-grid">
      <div v-for="index in 6" :key="index" class="panel" style="height: 252px; padding: 18px">
        <div class="skeleton" style="height: 100%" />
      </div>
    </div>

    <div v-else-if="filtered.length || canCreate" class="dashboard-grid">
      <article v-for="(item, index) in filtered" :key="item.id" class="dashboard-card" @click="open(item)">
        <div class="card-preview">
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
              <span><Building2 :size="11" /> {{ item.category || '经营分析' }}</span>
              <span><TrendingUp :size="11" /> {{ formatDate(item.updated_at) }}</span>
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
        :description="dashboards.length ? '换一个关键词或分类试试。' : '你的账号还没有可见的看板，请联系管理员发布看板。'"
      >
        <button v-if="dashboards.length" class="btn btn-secondary btn-sm" type="button" @click="keyword = ''; category = '全部'">
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
