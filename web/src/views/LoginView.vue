<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowRight, Eye, EyeOff, LockKeyhole, ShieldCheck, UserRound } from 'lucide-vue-next'
import AppBrand from '../components/AppBrand.vue'
import { useAuthStore } from '../stores/auth'
import { getErrorMessage } from '../api/client'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const form = reactive({ username: 'laozong', password: '123456' })
const showPassword = ref(false)
const error = ref('')
const submitting = ref(false)

const demoAccounts = [
  { username: 'laozong', password: '123456', name: '集团老总', scope: '全部组织数据' },
  { username: 'huadong', password: '123456', name: '华东大区经理', scope: '本大区及下级' },
  { username: 'shanghai', password: '123456', name: '上海区域经理', scope: '本区域及门店' },
  { username: 'store01', password: '123456', name: '静安旗舰店店长', scope: '仅本门店' },
  { username: 'admin', password: 'admin123', name: '平台管理员', scope: '全部 + 后台' },
  { username: 'jiangsu', password: '123456', name: '江苏区域经理', scope: '本区域及门店' },
]

onMounted(async () => {
  if (!auth.token) return
  try {
    await auth.fetchMe()
    router.replace(route.query.redirect || '/dashboards')
  } catch {
    /* 登录态失效时停留登录页 */
  }
})

function pickDemo(account) {
  form.username = account.username
  form.password = account.password
  error.value = ''
}

async function submit() {
  if (!form.username || !form.password) {
    error.value = '请输入用户名和密码'
    return
  }
  submitting.value = true
  error.value = ''
  try {
    await auth.login(form.username.trim(), form.password)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboards'
    router.replace(redirect)
  } catch (err) {
    error.value = getErrorMessage(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <section class="login-visual">
      <div class="login-brand">
        <AppBrand />
        <strong>青衡 BI</strong>
        <span>RETAIL INSIGHT PLATFORM</span>
      </div>

      <div class="login-copy">
        <div class="eyebrow">数据驱动的经营决策</div>
        <h1>经营看得清<em>增长有依据</em></h1>
        <p>
          从集团老总到门店店长，同一套指标体系、不同层级的数据边界。自定义 SQL 数据集、可视化看板、
          组织架构权限与企业微信 / 微信定时推送，全部在一个平台内完成。
        </p>
      </div>

      <div class="login-metrics">
        <div class="login-metric">
          <strong>6 级</strong>
          <span>组织与角色权限</span>
        </div>
        <div class="login-metric">
          <strong>实时</strong>
          <span>SQL 数据集看板</span>
        </div>
        <div class="login-metric">
          <strong>2 通道</strong>
          <span>企业微信 · 微信</span>
        </div>
      </div>

      <svg class="login-signal" viewBox="0 0 300 150" fill="none" aria-hidden="true">
        <defs>
          <linearGradient id="loginSignalFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stop-color="#A7F3D0" stop-opacity="0.34" />
            <stop offset="1" stop-color="#A7F3D0" stop-opacity="0" />
          </linearGradient>
        </defs>
        <path d="M0 118C24 108 34 84 56 88C78 92 84 60 108 64C132 68 140 96 162 82C184 68 190 34 214 42C238 50 244 78 266 58C280 45 290 30 300 22V150H0V118Z" fill="url(#loginSignalFill)" />
        <path d="M0 118C24 108 34 84 56 88C78 92 84 60 108 64C132 68 140 96 162 82C184 68 190 34 214 42C238 50 244 78 266 58C280 45 290 30 300 22" stroke="#A7F3D0" stroke-width="1.5" stroke-linecap="round" />
      </svg>
    </section>

    <section class="login-form-side">
      <div class="login-box">
        <div class="eyebrow">欢迎回来</div>
        <h2>登录经营工作台</h2>
        <p>使用企业账号登录，系统会自动加载你的角色与组织数据范围。</p>

        <form class="login-form" @submit.prevent="submit">
          <label class="field-label" for="username">账号</label>
          <div class="input-wrap">
            <UserRound :size="17" />
            <input id="username" v-model="form.username" autocomplete="username" placeholder="请输入用户名" />
          </div>

          <label class="field-label" for="password">密码</label>
          <div class="input-wrap">
            <LockKeyhole :size="17" />
            <input
              id="password"
              v-model="form.password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="current-password"
              placeholder="请输入密码"
            />
            <button class="password-toggle" type="button" aria-label="切换密码可见" @click="showPassword = !showPassword">
              <EyeOff v-if="showPassword" :size="16" />
              <Eye v-else :size="16" />
            </button>
          </div>

          <div class="login-error">{{ error }}</div>

          <button class="btn btn-primary login-submit" type="submit" :disabled="submitting">
            {{ submitting ? '正在登录…' : '进入工作台' }}
            <ArrowRight v-if="!submitting" :size="16" />
          </button>
        </form>

        <div class="demo-section">
          <div class="demo-title">
            <span>演示账号 · 点击快速填充</span>
            <span><ShieldCheck :size="11" style="vertical-align: -1px" /> 密码 123456</span>
          </div>
          <div class="demo-accounts">
            <button
              v-for="account in demoAccounts"
              :key="account.username"
              class="demo-account"
              type="button"
              @click="pickDemo(account)"
            >
              <strong>{{ account.name }}</strong>
              <span>{{ account.username }} · {{ account.scope }}</span>
            </button>
          </div>
        </div>

        <p class="login-foot">
          演示环境使用 SQLite 与模拟推送通道。生产部署前请更换 JWT_SECRET、配置只读数据库账号，
          并填写企业微信 / 微信公众号凭证。
        </p>
      </div>
    </section>
  </div>
</template>
