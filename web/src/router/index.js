import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const routes = [
  {
    path: '/login',
    name: 'login',
    component: () => import('../views/LoginView.vue'),
    meta: { title: '登录', public: true },
  },
  {
    path: '/share/:token',
    name: 'public-share',
    component: () => import('../views/PublicDashboardView.vue'),
    meta: { title: '授权看板', public: true },
  },
  {
    path: '/',
    component: () => import('../layouts/AppLayout.vue'),
    redirect: '/dashboards',
    children: [
      {
        path: 'dashboards',
        name: 'dashboards',
        component: () => import('../views/dashboard/DashboardListView.vue'),
        meta: { title: '经营看板', subtitle: '让每一个经营决策，都有数据依据' },
      },
      {
        path: 'dashboards/new',
        name: 'dashboard-create',
        component: () => import('../views/dashboard/DashboardEditorView.vue'),
        meta: { title: '新建看板', subtitle: '组合指标与图表，搭建业务视图', permission: 'dashboard:create' },
      },
      {
        path: 'dashboards/:id',
        name: 'dashboard-view',
        component: () => import('../views/dashboard/DashboardView.vue'),
        meta: { title: '经营看板', subtitle: '实时经营洞察' },
      },
      {
        path: 'dashboards/:id/edit',
        name: 'dashboard-edit',
        component: () => import('../views/dashboard/DashboardEditorView.vue'),
        meta: { title: '编辑看板', subtitle: '调整布局、指标与数据源', permission: 'dashboard:create' },
      },
      {
        path: 'templates',
        name: 'templates',
        component: () => import('../views/TemplateGalleryView.vue'),
        meta: { title: '模板中心', subtitle: '从成熟分析框架快速起步' },
      },
      {
        path: 'drill',
        name: 'organization-drill',
        component: () => import('../views/OrganizationDrillView.vue'),
        meta: { title: '组织下钻', subtitle: '按角色逐级查看区域、门店与员工经营数据' },
      },
      {
        path: 'shares',
        name: 'shares',
        component: () => import('../views/ShareListView.vue'),
        meta: { title: '分享管理', subtitle: '管理带数据权限的授权访问链接' },
      },
      {
        path: 'access',
        name: 'access',
        component: () => import('../views/AccessView.vue'),
        meta: { title: '数据权限', subtitle: '看清组织边界与可见数据范围' },
      },
      {
        path: 'admin/orgs',
        name: 'admin-orgs',
        component: () => import('../views/admin/OrganizationView.vue'),
        meta: { title: '组织架构', subtitle: '维护集团、大区、区域与门店层级', admin: true },
      },
      {
        path: 'admin/users',
        name: 'admin-users',
        component: () => import('../views/admin/UserView.vue'),
        meta: { title: '用户与角色', subtitle: '账号、角色与组织数据范围', admin: true },
      },
      {
        path: 'admin/datasets',
        name: 'admin-datasets',
        component: () => import('../views/admin/DatasetView.vue'),
        meta: { title: '数据模型', subtitle: '受控 SQL 数据集与字段预览', admin: true },
      },
      {
        path: 'admin/datasources',
        name: 'admin-datasources',
        component: () => import('../views/admin/DatasourceView.vue'),
        meta: { title: '数据源管理', subtitle: '对接 MySQL、PostgreSQL、Oracle、SQL Server 与 SQLite', admin: true },
      },
      {
        path: 'admin/targets',
        name: 'admin-targets',
        component: () => import('../views/admin/TargetView.vue'),
        meta: { title: '经营目标', subtitle: '目标导入与达成率跟踪', admin: true },
      },
      {
        path: 'admin/push',
        name: 'admin-push',
        component: () => import('../views/admin/PushCenterView.vue'),
        meta: { title: '推送中心', subtitle: '企业微信与微信公众号定时触达', admin: true },
      },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/dashboards' },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior: () => ({ top: 0 }),
})

router.beforeEach(async (to) => {
  document.title = `${to.meta.title || '经营分析'} · 青衡 BI`
  if (to.meta.public) return true
  const auth = useAuthStore()
  if (!auth.token) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (!auth.user) {
    try {
      await auth.fetchMe()
    } catch {
      return { name: 'login', query: { redirect: to.fullPath } }
    }
  }
  if (to.meta.admin && !auth.isAdmin) {
    return { name: 'dashboards' }
  }
  if (to.meta.permission && !auth.can(to.meta.permission)) {
    return { name: 'dashboards' }
  }
  return true
})

export default router
