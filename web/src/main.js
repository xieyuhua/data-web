// 前端入口：注册 Pinia（全局状态）、Vue Router（路由）、Element Plus（UI 组件库，中文语言包）
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import 'element-plus/dist/index.css'
import './assets/styles.css'
import App from './App.vue'
import router from './router'
import { installTableDragPan } from './utils/tableDrag'

const app = createApp(App)
// 状态管理：登录用户、角色、权限、数据范围等全局状态
app.use(createPinia())
// 路由：页面访问权限在 router/index.js 的全局守卫中统一校验
app.use(router)
// UI 组件库全局注册，统一使用简体中文文案
app.use(ElementPlus, { locale: zhCn })
app.mount('#app')
// 全站列表支持横向拖动查看（Element Plus 表格 + 原生数据表格）
installTableDragPan()
