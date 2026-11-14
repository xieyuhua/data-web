<!-- 数据权限说明页：展示当前账号的组织归属、角色、数据范围与可见组织边界，并提供企业微信绑定入口。 -->
<script setup>
import { computed, onMounted, ref } from 'vue'
import { Database, KeyRound, ShieldCheck, Store, Users } from 'lucide-vue-next'
import { ElMessage } from 'element-plus'
import { api, getErrorMessage } from '../api/client'
import WecomBindingCard from '../components/WecomBindingCard.vue'
import { useAuthStore } from '../stores/auth'
import { dataScopeLabel, findOrgChain, flattenOrgs, orgTypeLabel } from '../utils/org'

const auth = useAuthStore()
const roles = ref([])
const orgTree = ref([])
const loading = ref(true)

const scopeLabel = computed(() => dataScopeLabel(auth.dataScope))
const orgChain = computed(() => findOrgChain(orgTree.value, auth.user?.organization_id))
const orgs = computed(() => flattenOrgs(orgTree.value))
const storeCount = computed(() => orgs.value.filter((item) => item.type === 'store').length)
const currentRole = computed(() => roles.value.find((item) => item.code === auth.role?.code) || auth.role)

const permissionLabels = {
  '*': '全部权限',
  'dashboard:view': '查看看板',
  'dashboard:create': '创建与编辑看板',
  'share:create': '创建授权分享',
  'push:view': '查看推送',
}

const permissionList = computed(() => {
  if (!auth.permissions?.length) return ['暂无额外权限']
  return auth.permissions.map((item) => permissionLabels[item] || item)
})

// 加载当前账号的数据权限详情（角色、数据范围、可见组织列表）
async function load() {
  loading.value = true
  try {
    const [roleList, tree] = await Promise.all([
      api.get('/roles'),
      api.get('/orgs/tree').catch(() => []),
    ])
    roles.value = roleList || []
    orgTree.value = tree || []
    if (!auth.user) await auth.fetchMe()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <section class="access-hero">
      <div class="eyebrow" style="color: var(--mint)">授权边界</div>
      <h2>你看到的每一行数据<br />都经过组织权限校验</h2>
      <p>
        平台按角色配置数据范围：全部组织、本组织及下级、仅本组织。看板 SQL 在服务端自动展开为组织 ID 集合，
        前端筛选和分享链接都无法绕过该边界。
      </p>
      <div class="access-current">
        <ShieldCheck :size="20" />
        <div>
          <span>当前账号</span>
          <strong>{{ auth.displayName }} · {{ currentRole?.name || '—' }} · {{ scopeLabel }}</strong>
        </div>
      </div>
    </section>

    <div class="scope-note" style="margin-top: 18px">
      <ShieldCheck :size="14" />
      <span>
        组织：<strong>{{ auth.orgName || '全部组织' }}</strong> ·
        可见节点 {{ orgs.length }} 个，其中门店 {{ storeCount }} 家 ·
        权限项：<strong>{{ permissionList.join('、') }}</strong>
      </span>
    </div>

    <div class="section-head">
      <div>
        <h2>角色权限矩阵</h2>
        <p>从集团老总到门店店长，数据范围逐级收敛</p>
      </div>
      <span class="tag tag-green">{{ roles.length }} 个角色</span>
    </div>

    <div v-if="loading" class="role-grid">
      <div v-for="index in 5" :key="index" class="panel" style="height: 150px; padding: 14px">
        <div class="skeleton" style="height: 100%" />
      </div>
    </div>

    <div v-else class="role-grid">
      <article
        v-for="role in roles"
        :key="role.code"
        class="role-card"
        :class="{ current: role.code === auth.role?.code }"
      >
        <div class="role-level">LEVEL {{ role.level }}</div>
        <h3>{{ role.name }}</h3>
        <span class="role-scope">{{ dataScopeLabel(role.data_scope) }}</span>
        <p>{{ role.description || '按组织层级控制数据可见范围。' }}</p>
        <div class="tag" :class="role.code === auth.role?.code ? 'tag-green' : ''" style="margin-top: 11px">
          {{ role.code === auth.role?.code ? '当前角色' : role.code }}
        </div>
      </article>
    </div>

    <div class="admin-layout" style="margin-top: 18px">
      <div class="admin-main">
        <div class="panel">
          <div class="panel-head">
            <div>
              <div class="panel-title">当前组织链路</div>
              <div class="panel-subtitle">权限从所属组织开始向下展开</div>
            </div>
            <Users :size="17" style="color: var(--brand)" />
          </div>
          <div style="padding: 2px 20px 20px">
            <div class="scope-chain">
              <div
                v-for="(org, index) in orgChain"
                :key="org.id"
                class="scope-chain-item"
                :class="{ active: index === orgChain.length - 1 }"
              >
                <span>{{ org.name }}</span>
                <span class="tag">{{ orgTypeLabel(org.type) }}</span>
                <span style="color: var(--muted-light); font-size: 8px">{{ org.code }}</span>
              </div>
              <div v-if="!orgChain.length" class="scope-chain-item active">
                <span>未绑定组织，按角色兜底策略处理</span>
              </div>
            </div>
            <div class="scope-card" style="margin-top: 16px; background: #f8faf8">
              <h3>数据范围说明</h3>
              <p>
                <template v-if="auth.dataScope === 'all'">当前角色可以查看全部组织节点，看板汇总集团、大区、区域与门店数据。</template>
                <template v-else-if="auth.dataScope === 'own_org_tree'">当前角色可以查看所属组织及其全部下级节点，但不包含兄弟组织。</template>
                <template v-else>当前角色仅能查看所属组织本身的数据，通常对应单个门店。</template>
              </p>
            </div>
          </div>
        </div>
      </div>

      <aside class="admin-aside">
        <!-- 当前账号的企业微信绑定状态与绑定/解绑入口 -->
        <WecomBindingCard />
        <div class="scope-card">
          <h3><KeyRound :size="13" style="vertical-align: -2px" /> 权限项</h3>
          <p>角色权限决定可执行的操作，数据范围决定可见的组织数据。</p>
          <div style="display: flex; flex-wrap: wrap; gap: 5px; margin-top: 12px">
            <span v-for="item in permissionList" :key="item" class="tag tag-green">{{ item }}</span>
          </div>
        </div>
        <div class="scope-card">
          <h3><Database :size="13" style="vertical-align: -2px" /> SQL 安全</h3>
          <p>仅允许 SELECT / WITH 查询，禁止多语句、注释与危险关键字，最多返回 2000 行。</p>
        </div>
        <div class="scope-card">
          <h3><Store :size="13" style="vertical-align: -2px" /> 层级示例</h3>
          <p>集团老总：全部 15 家门店；大区经理：华东 9 家；区域经理：上海 3 家；店长：仅静安旗舰店。</p>
        </div>
      </aside>
    </div>

    <div class="security-grid">
      <div class="security-card">
        <div class="security-icon"><ShieldCheck :size="16" /></div>
        <h3>查询层过滤</h3>
        <p>数据集必须包含 <span class="mono" v-text="'{{org_scope}}'"></span> 占位符，由服务端渲染为授权组织 ID 集合。</p>
      </div>
      <div class="security-card">
        <div class="security-icon"><KeyRound :size="16" /></div>
        <h3>最小权限</h3>
        <p>店长与员工默认没有看板编辑、数据模型管理和推送配置权限。</p>
      </div>
      <div class="security-card">
        <div class="security-icon"><Database :size="16" /></div>
        <h3>分享继承</h3>
        <p>公共分享链接保存创建时的组织范围与有效期，撤销后立即失效。</p>
      </div>
    </div>
  </div>
</template>
