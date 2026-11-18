<template>
  <div class="callback-page">
    <el-result v-if="error" icon="error" title="企业微信登录失败" :sub-title="error">
      <template #extra>
        <el-button type="primary" @click="router.push('/login')">返回登录</el-button>
      </template>
    </el-result>
    <div v-else v-loading="true" class="loading-box">
      <p>正在登录，请稍候…</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { authApi } from '@/api'
import { TOKEN_KEY } from '@/api/http'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const error = ref('')

onMounted(async () => {
  const code = String(route.query.code || '')
  if (!code) {
    error.value = '缺少企业微信授权码，请在企业微信中重新打开'
    return
  }
  try {
    const res = await authApi.wecomLogin(code)
    localStorage.setItem(TOKEN_KEY, res.token)
    auth.token = res.token
    auth.user = res.user
    auth.loaded = true
    const state = String(route.query.state || '')
    // State carries the intended path (base64) so deep links survive SSO.
    let target = '/dashboards'
    if (state) {
      try {
        const decoded = atob(state)
        if (decoded.startsWith('/')) target = decoded
      } catch {}
    }
    router.replace(target)
  } catch (e: any) {
    error.value = e?.response?.data?.message || '登录失败，请联系管理员'
  }
})
</script>

<style scoped>
.callback-page {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f5f7fa;
}
.loading-box {
  color: #606266;
}
</style>
