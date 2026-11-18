<template>
  <el-container class="app-shell">
    <!-- Desktop sidebar -->
    <el-aside v-if="!isMobile" :width="collapsed ? '64px' : '220px'" class="aside">
      <div class="logo" :class="{ 'logo--mini': collapsed }">
        <span class="logo-mark">B</span>
        <span v-if="!collapsed" class="logo-text">Biaoge BI</span>
      </div>
      <el-menu
        :default-active="activeMenu"
        :collapse="collapsed"
        router
        class="menu"
        background-color="#1f2937"
        text-color="#cbd5e1"
        active-text-color="#fff"
      >
        <el-menu-item v-for="item in menuItems" :key="item.path" :index="item.path">
          <el-icon><component :is="item.icon" /></el-icon>
          <template #title>{{ item.label }}</template>
        </el-menu-item>
      </el-menu>
      <div class="aside-footer">
        <el-button link class="collapse-btn" @click="collapsed = !collapsed">
          <el-icon><component :is="collapsed ? Expand : Fold" /></el-icon>
        </el-button>
      </div>
    </el-aside>

    <!-- Mobile drawer -->
    <el-drawer
      v-if="isMobile"
      v-model="drawerOpen"
      direction="ltr"
      size="228px"
      :with-header="false"
      class="nav-drawer"
    >
      <div class="drawer-inner">
        <div class="logo">
          <span class="logo-mark">B</span>
          <span class="logo-text">Biaoge BI</span>
        </div>
        <el-menu :default-active="activeMenu" router class="menu" background-color="#1f2937" text-color="#cbd5e1" active-text-color="#fff">
          <el-menu-item v-for="item in menuItems" :key="item.path" :index="item.path" @click="drawerOpen = false">
            <el-icon><component :is="item.icon" /></el-icon>
            <span>{{ item.label }}</span>
          </el-menu-item>
        </el-menu>
      </div>
    </el-drawer>

    <el-container class="main-container">
      <el-header class="header" :class="{ 'header--mobile': isMobile }">
        <div class="header-left">
          <el-button v-if="isMobile" link class="hamburger" @click="drawerOpen = true">
            <el-icon :size="20"><Menu /></el-icon>
          </el-button>
          <span class="page-title">{{ pageTitle }}</span>
        </div>
        <div class="header-right">
          <el-dropdown trigger="click" @command="onCommand">
            <span class="user-chip">
              <el-avatar :size="26" class="user-avatar">{{ initial }}</el-avatar>
              <span v-if="!isMobile" class="user-name">{{ auth.displayName }}</span>
              <el-tag v-if="!isMobile" size="small" :type="auth.isAdmin ? 'danger' : 'info'" effect="plain">
                {{ auth.isAdmin ? '管理员' : '用户' }}
              </el-tag>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item disabled>{{ auth.user?.username }}</el-dropdown-item>
                <el-dropdown-item command="password" divided>修改密码</el-dropdown-item>
                <el-dropdown-item command="logout">退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>
      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>

  <el-dialog v-model="pwdVisible" title="修改密码" :width="isMobile ? '100%' : '420px'">
    <el-form label-position="top">
      <el-form-item label="原密码">
        <el-input v-model="pwd.old" type="password" show-password autocomplete="current-password" />
      </el-form-item>
      <el-form-item label="新密码">
        <el-input v-model="pwd.next" type="password" show-password autocomplete="new-password" placeholder="至少 8 位" />
      </el-form-item>
      <el-form-item label="确认新密码">
        <el-input v-model="pwd.confirm" type="password" show-password autocomplete="new-password" @keyup.enter="savePassword" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="pwdVisible = false">取消</el-button>
      <el-button type="primary" :loading="pwdSaving" @click="savePassword">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  Coin, DataBoard, Document, Expand, Fold, Histogram, Menu, OfficeBuilding, Setting, Shop, Tickets, Timer, Tools, User,
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { authApi } from '@/api'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const MOBILE_BREAKPOINT = 900

const isMobile = ref(false)
const drawerOpen = ref(false)
const collapsed = ref(localStorage.getItem('bi_sidebar_collapsed') === '1')

function syncViewport() {
  const mobile = window.innerWidth < MOBILE_BREAKPOINT
  isMobile.value = mobile
  if (!mobile) drawerOpen.value = false
}

let resizeTimer = 0
function onResize() {
  window.clearTimeout(resizeTimer)
  resizeTimer = window.setTimeout(syncViewport, 120)
}

onMounted(() => {
  syncViewport()
  window.addEventListener('resize', onResize)
  // Restore the persisted collapse state on non-mobile viewports.
  if (window.innerWidth < 1200) collapsed.value = true
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  window.clearTimeout(resizeTimer)
})

const titles: Record<string, string> = {
  '/dashboards': '仪表盘',
  '/charts': '图表',
  '/datasets': '数据集',
  '/datasources': '数据源',
  '/users': '用户管理',
  '/org': '组织架构',
  '/stores': '门店列表',
  '/schedules': '定时报表',
  '/audit-logs': '审计日志',
  '/settings': '系统设置',
  '/config': '系统配置',
}

const activeMenu = computed(() => {
  if (route.path.startsWith('/dashboards')) return '/dashboards'
  return route.path
})
const pageTitle = computed(() => titles[activeMenu.value] || 'Biaoge BI')
const initial = computed(() => (auth.displayName || 'U').slice(0, 1).toUpperCase())

interface MenuItem {
  path: string
  label: string
  icon: any
  admin?: boolean
}

const menuItems = computed<MenuItem[]>(() => {
  const all: MenuItem[] = [
    { path: '/dashboards', label: '仪表盘', icon: DataBoard },
    { path: '/charts', label: '图表', icon: Histogram },
    { path: '/datasets', label: '数据集', icon: Document, admin: true },
    { path: '/datasources', label: '数据源', icon: Coin, admin: true },
    { path: '/schedules', label: '定时报表', icon: Timer },
    { path: '/users', label: '用户管理', icon: User, admin: true },
    { path: '/stores', label: '门店列表', icon: Shop },
    { path: '/org', label: '组织架构', icon: OfficeBuilding, admin: true },
    { path: '/settings', label: '系统设置', icon: Setting, admin: true },
    { path: '/config', label: '系统配置', icon: Tools, admin: true },
    { path: '/audit-logs', label: '审计日志', icon: Tickets, admin: true },
  ]
  return all.filter((i) => !i.admin || auth.isAdmin)
})

const pwdVisible = ref(false)
const pwdSaving = ref(false)
const pwd = reactive({ old: '', next: '', confirm: '' })

function onCommand(cmd: string) {
  if (cmd === 'logout') {
    auth.logout()
    router.push('/login')
  } else if (cmd === 'password') {
    pwd.old = ''
    pwd.next = ''
    pwd.confirm = ''
    pwdVisible.value = true
  }
}

async function savePassword() {
  if (!pwd.old || !pwd.next) {
    ElMessage.warning('请填写完整')
    return
  }
  if (pwd.next !== pwd.confirm) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  pwdSaving.value = true
  try {
    await authApi.changePassword(pwd.old, pwd.next)
    ElMessage.success('密码已更新')
    pwdVisible.value = false
  } finally {
    pwdSaving.value = false
  }
}

watch(collapsed, (v) => localStorage.setItem('bi_sidebar_collapsed', v ? '1' : '0'))
</script>

<style scoped>
.app-shell {
  height: 100vh;
  height: 100dvh;
  overflow: hidden;
}
.aside {
  background: #1f2937;
  display: flex;
  flex-direction: column;
  transition: width 0.2s ease;
  overflow: hidden;
}
.logo {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 60px;
  padding: 0 20px;
  color: #fff;
  flex-shrink: 0;
  overflow: hidden;
}
.logo--mini {
  padding: 0;
  justify-content: center;
}
.logo-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  min-width: 30px;
  height: 30px;
  border-radius: 8px;
  background: linear-gradient(135deg, #409eff, #6f42c1);
  font-weight: 700;
}
.logo-text {
  font-size: 17px;
  font-weight: 600;
  white-space: nowrap;
}
.menu {
  border-right: none;
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
}
.menu:not(.el-menu--collapse) {
  width: 100%;
}
.aside-footer {
  padding: 8px;
  display: flex;
  justify-content: flex-end;
  flex-shrink: 0;
}
.collapse-btn {
  color: #94a3b8;
}
.collapse-btn:hover {
  color: #fff;
}
.drawer-inner {
  height: 100%;
  background: #1f2937;
  display: flex;
  flex-direction: column;
}
.main-container {
  min-width: 0;
}
.header {
  background: #fff;
  border-bottom: 1px solid var(--bi-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 20px;
  height: 60px;
  flex-shrink: 0;
}
.header--mobile {
  padding: 0 12px;
  height: 52px;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.header-left .page-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hamburger {
  color: #303133;
}
.header-right {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}
.user-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  color: #303133;
  outline: none;
}
.user-avatar {
  background: linear-gradient(135deg, #409eff, #6f42c1);
  color: #fff;
  font-size: 13px;
}
.user-name {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.main {
  padding: var(--bi-gap);
  overflow: auto;
  min-width: 0;
}
@media (max-width: 640px) {
  .main {
    padding: 10px;
  }
}
</style>
