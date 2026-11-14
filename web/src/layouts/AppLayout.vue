<!-- 后台整体布局：
     - 桌面端左侧固定导航栏，移动端折叠为抽屉 + 底部导航（自适应）
     - 「管理后台」菜单仅超级管理员可见
     - 顶部展示当前账号的组织与数据权限范围，防止误判可见数据 -->
<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Building2,
  ChevronDown,
  Database,
  LayoutDashboard,
  LayoutTemplate,
  Menu,
  Network,
  Send,
  Server,
  Share2,
  ShieldCheck,
  ScrollText,
  Target,
  Users,
} from 'lucide-vue-next'
import AppBrand from '../components/AppBrand.vue'
import { useAuthStore } from '../stores/auth'
import { dataScopeLabel } from '../utils/org'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const drawerOpen = ref(false)

// 侧边导航分组：分析工作台对所有登录用户开放，管理后台仅管理员可见
const groups = computed(() => {
  const result = [
    {
      label: '分析工作台',
      items: [
        { to: '/dashboards', label: '经营看板', icon: LayoutDashboard },
        { to: '/drill', label: '组织下钻', icon: Network },
        { to: '/templates', label: '模板中心', icon: LayoutTemplate },
        { to: '/shares', label: '分享管理', icon: Share2 },
        { to: '/access', label: '数据权限', icon: ShieldCheck },
      ],
    },
  ]
  if (auth.isAdmin) {
    result.push({
      label: '管理后台',
      items: [
        { to: '/admin/orgs', label: '组织架构', icon: Building2 },
        { to: '/admin/users', label: '用户与角色', icon: Users },
        { to: '/admin/datasets', label: '数据模型', icon: Database },
        { to: '/admin/datasources', label: '数据源管理', icon: Server },
        { to: '/admin/targets', label: '经营目标', icon: Target },
        { to: '/admin/push', label: '推送中心', icon: Send },
        { to: '/admin/audit', label: '审计日志', icon: ScrollText },
      ],
    })
  }
  return result
})

// 当前数据权限范围文案
const scopeLabel = computed(() => dataScopeLabel(auth.dataScope))
const roleName = computed(() => auth.role?.name || '访客')
const pageTitle = computed(() => route.meta.title || '经营看板')
const pageSubtitle = computed(() => route.meta.subtitle || '让每一个经营决策，都有数据依据')

// 路由切换后自动关闭移动端抽屉
watch(
  () => route.fullPath,
  () => {
    drawerOpen.value = false
  },
)

// 顶部用户下拉菜单命令处理
function handleCommand(command) {
  if (command === 'logout') {
    auth.logout()
    router.push('/login')
  }
  if (command === 'access') {
    router.push('/access')
  }
}
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar" :class="{ open: drawerOpen }">
      <div class="brand">
        <AppBrand />
        <div class="brand-word">
          <strong>青衡 BI</strong>
          <span>Insight Platform</span>
        </div>
      </div>

      <nav class="sidebar-nav">
        <div v-for="group in groups" :key="group.label" class="nav-group">
          <div class="nav-label">{{ group.label }}</div>
          <router-link v-for="item in group.items" :key="item.to" :to="item.to" class="nav-item">
            <component :is="item.icon" :size="17" />
            <span>{{ item.label }}</span>
          </router-link>
        </div>
      </nav>

      <div class="sidebar-foot">
        <div class="security-line"><span class="pulse" /> 数据权限已启用</div>
        <p>{{ auth.orgName || '组织范围' }} · {{ scopeLabel }}</p>
        <p style="margin-top: 4px">所有查询自动按组织边界过滤，分享链接同样继承授权范围。</p>
      </div>
    </aside>

    <div v-if="drawerOpen" class="mobile-drawer-backdrop" @click="drawerOpen = false" />

    <div class="main-shell">
      <header class="topbar">
        <div class="topbar-left">
          <button class="icon-btn mobile-only" type="button" aria-label="打开菜单" @click="drawerOpen = true">
            <Menu :size="18" />
          </button>
          <div>
            <div class="topbar-title">{{ pageTitle }}</div>
            <div class="topbar-sub">{{ pageSubtitle }}</div>
          </div>
        </div>

        <div class="topbar-right">
          <div class="scope-pill">
            <ShieldCheck :size="14" />
            <span>{{ auth.orgName || '全部组织' }} · {{ scopeLabel }}</span>
          </div>
          <el-dropdown trigger="click" @command="handleCommand">
            <button class="user-trigger" type="button">
              <span class="avatar">{{ auth.initials }}</span>
              <span class="user-meta">
                <strong>{{ auth.displayName }}</strong>
                <span>{{ roleName }}</span>
              </span>
              <ChevronDown :size="13" />
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="access">数据权限说明</el-dropdown-item>
                <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>

      <main class="page-body">
        <router-view />
      </main>
    </div>

    <nav class="mobile-bottom-nav mobile-only">
      <router-link to="/dashboards">
        <LayoutDashboard :size="18" />
        <span>看板</span>
      </router-link>
      <router-link to="/templates">
        <LayoutTemplate :size="18" />
        <span>模板</span>
      </router-link>
      <router-link to="/shares">
        <Share2 :size="18" />
        <span>分享</span>
      </router-link>
      <router-link to="/access">
        <ShieldCheck :size="18" />
        <span>权限</span>
      </router-link>
      <button type="button" @click="drawerOpen = true">
        <Menu :size="18" />
        <span>更多</span>
      </button>
    </nav>
  </div>
</template>
