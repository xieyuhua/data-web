<!-- 企业微信授权回调页：用一次性 ticket 换取登录态后跳转目标页面，避免长期 JWT 出现在 URL 中。 -->
<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { CircleCheck, CircleX, Loader2 } from 'lucide-vue-next'
import AppBrand from '../components/AppBrand.vue'
import { useAuthStore } from '../stores/auth'
import { getErrorMessage } from '../api/client'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

// 回调页状态：exchanging 校验中 / success 登录成功 / bound 绑定成功 / error 失败
const state = ref('exchanging')
const message = ref('正在校验企业微信授权…')
// 登录成功后的站内跳转地址
const redirect = ref('/dashboards')

// 根据状态计算页面标题
const title = computed(() => {
  if (state.value === 'bound') return '企业微信绑定成功'
  if (state.value === 'success') return '登录成功，正在进入工作台'
  if (state.value === 'error') return '企业微信授权失败'
  return '正在登录'
})

// 只允许站内相对路径，防止开放重定向
function safeRedirect(value) {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//') || value.includes('\\')) {
    return '/dashboards'
  }
  return value
}

onMounted(async () => {
  const query = route.query
  redirect.value = safeRedirect(query.redirect)

  // 场景一：从「绑定企业微信」流程返回
  if (query.bound === '1') {
    state.value = 'bound'
    message.value = '当前账号已成功绑定企业微信，可在企业微信客户端内直接登录。'
    return
  }

  // 场景二：后端回调失败时会把原因带到 error 参数
  if (typeof query.error === 'string' && query.error) {
    state.value = 'error'
    message.value = query.error
    return
  }

  // 场景三：使用一次性 ticket 换取正式登录态（长期 JWT 不经过 URL）
  const ticket = typeof query.ticket === 'string' ? query.ticket : ''
  if (!ticket) {
    state.value = 'error'
    message.value = '缺少登录凭证，请返回登录页重新发起企业微信登录。'
    return
  }

  try {
    await auth.exchangeWecom(ticket)
    state.value = 'success'
    message.value = '身份校验通过，正在加载你的角色与数据范围…'
    router.replace(redirect.value)
  } catch (error) {
    state.value = 'error'
    message.value = getErrorMessage(error)
  }
})

// 授权失败时返回登录页重新发起
function goLogin() {
  router.replace('/login')
}

// 绑定成功时返回数据权限页查看绑定状态
function goHome() {
  router.replace('/access')
}
</script>

<template>
  <div class="wecom-callback-page">
    <div class="wecom-callback-card">
      <AppBrand />
      <!-- 状态图标：加载中 / 成功 / 失败 -->
      <div class="wecom-callback-icon" :class="state">
        <Loader2 v-if="state === 'exchanging'" :size="26" class="spin" />
        <CircleCheck v-else-if="state === 'success' || state === 'bound'" :size="26" />
        <CircleX v-else :size="26" />
      </div>
      <h2>{{ title }}</h2>
      <p>{{ message }}</p>
      <!-- 失败可重新登录，绑定成功可返回权限页 -->
      <div v-if="state === 'error'" class="wecom-callback-actions">
        <button class="btn btn-primary btn-sm" type="button" @click="goLogin">返回登录页</button>
      </div>
      <div v-else-if="state === 'bound'" class="wecom-callback-actions">
        <button class="btn btn-primary btn-sm" type="button" @click="goHome">返回数据权限页</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.wecom-callback-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  padding: 24px;
  background:
    radial-gradient(circle at 20% 20%, rgba(23, 121, 85, 0.08), transparent 42%),
    linear-gradient(180deg, #f7faf8, #eef4f0);
}

.wecom-callback-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 100%;
  max-width: 420px;
  padding: 40px 34px;
  border: 1px solid var(--line);
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: var(--shadow-sm);
  text-align: center;
}

.wecom-callback-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  margin-top: 26px;
  border-radius: 50%;
  background: #f1f7f3;
  color: var(--brand);
}

.wecom-callback-icon.success,
.wecom-callback-icon.bound {
  background: #e8f8ef;
  color: #16794f;
}

.wecom-callback-icon.error {
  background: #fdf0f2;
  color: #b33a4c;
}

.wecom-callback-card h2 {
  margin-top: 18px;
  font-family: var(--font-display);
  font-size: 24px;
  font-weight: 600;
  letter-spacing: -0.02em;
}

.wecom-callback-card p {
  margin-top: 10px;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.7;
}

.wecom-callback-actions {
  margin-top: 22px;
}
</style>
