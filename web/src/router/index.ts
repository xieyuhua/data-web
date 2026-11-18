import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const routes = [
  { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue'), meta: { public: true } },
  {
    path: '/wecom/callback',
    name: 'wecom-callback',
    component: () => import('@/views/WeComCallback.vue'),
    meta: { public: true },
  },
  {
    path: '/share/:token',
    name: 'share',
    component: () => import('@/views/ShareView.vue'),
    meta: { public: true },
  },
  {
    path: '/',
    component: () => import('@/layouts/MainLayout.vue'),
    children: [
      { path: '', redirect: '/dashboards' },
      { path: 'dashboards', name: 'dashboards', component: () => import('@/views/DashboardList.vue') },
      { path: 'dashboards/:id/view', name: 'dashboard-view', component: () => import('@/views/DashboardView.vue') },
      { path: 'dashboards/:id/edit', name: 'dashboard-edit', component: () => import('@/views/DashboardEditor.vue') },
      { path: 'datasets', name: 'datasets', component: () => import('@/views/DatasetList.vue'), meta: { admin: true } },
      { path: 'datasources', name: 'datasources', component: () => import('@/views/DataSourceList.vue'), meta: { admin: true } },
      { path: 'charts', name: 'charts', component: () => import('@/views/ChartList.vue') },
      { path: 'users', name: 'users', component: () => import('@/views/AdminUsers.vue'), meta: { admin: true } },
      { path: 'audit-logs', name: 'audit-logs', component: () => import('@/views/AuditLogs.vue'), meta: { admin: true } },
      { path: 'settings', name: 'settings', component: () => import('@/views/SettingsView.vue'), meta: { admin: true } },
      { path: 'config', name: 'config', component: () => import('@/views/ConfigView.vue'), meta: { admin: true } },
      { path: 'org', name: 'org', component: () => import('@/views/OrgTree.vue'), meta: { admin: true } },
      { path: 'stores', name: 'stores', component: () => import('@/views/StoreList.vue') },
      { path: 'schedules', name: 'schedules', component: () => import('@/views/ScheduleList.vue') },
    ],
  },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach(async (to) => {
  if (to.meta.public) return true
  const auth = useAuthStore()
  if (!auth.token) return { path: '/login', query: { redirect: to.fullPath } }
  if (!auth.user) {
    try {
      await auth.fetchMe()
    } catch {
      auth.logout()
      return { path: '/login' }
    }
  }
  if (to.meta.admin && !auth.isAdmin) {
    return { path: '/dashboards' }
  }
  return true
})
