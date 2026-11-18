<template>
  <div class="login-page">
    <div class="login-panel">
      <div class="brand">
        <span class="logo-mark">B</span>
        <div>
          <div class="brand-title">Biaoge BI</div>
          <div class="brand-sub">连接数据，构建洞察</div>
        </div>
      </div>

      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="submit">
        <el-form-item prop="username" label="账号">
          <el-input
            v-model="form.username"
            size="large"
            placeholder="用户名 / 企业微信账号"
            :prefix-icon="User"
            autocomplete="username"
            autofocus
            clearable
            @keyup.enter="submit"
          />
        </el-form-item>
        <el-form-item prop="password" label="密码">
          <el-input
            v-model="form.password"
            size="large"
            type="password"
            placeholder="请输入密码"
            :prefix-icon="Lock"
            show-password
            autocomplete="current-password"
            @keyup.enter="submit"
          />
        </el-form-item>

        <el-alert
          v-if="errorMessage"
          :title="errorMessage"
          type="error"
          show-icon
          :closable="false"
          class="login-error"
        />

        <el-button type="primary" size="large" class="submit-btn" :loading="loading" @click="submit">
          {{ loading ? '登录中…' : '登录' }}
        </el-button>
      </el-form>

      <template v-if="wecomEnabled">
        <el-divider class="login-divider">或</el-divider>
        <el-button size="large" class="wecom-btn" :loading="wecomLoading" :icon="ChatDotRound" @click="loginWithWeCom">
          企业微信扫码登录
        </el-button>
      </template>

      <p class="hint">首次部署请使用管理员账号登录，并在「系统设置」中完善配置。</p>
    </div>
    <p class="footer-note">Biaoge BI · 自托管数据分析平台</p>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { ChatDotRound, Lock, User } from '@element-plus/icons-vue'
import { authApi } from '@/api'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const formRef = ref<FormInstance>()
const form = reactive({ username: '', password: '' })
const rules: FormRules = {
  username: [{ required: true, message: '请输入账号', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

const loading = ref(false)
const wecomLoading = ref(false)
const wecomEnabled = ref(false)
const errorMessage = ref('')

function targetPath(): string {
  const redirect = route.query.redirect as string | undefined
  return redirect && redirect.startsWith('/') ? redirect : '/dashboards'
}

async function submit() {
  if (loading.value) return
  errorMessage.value = ''
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  loading.value = true
  try {
    await auth.login(form.username.trim(), form.password)
    router.replace(targetPath())
  } catch (e: any) {
    errorMessage.value = e?.response?.data?.message || '登录失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

async function loginWithWeCom() {
  if (wecomLoading.value) return
  wecomLoading.value = true
  try {
    // Preserve the deep link across the SSO round-trip.
    const res = await authApi.wecomAuthorizeURL(btoa(targetPath()))
    location.href = res.url
  } catch {
    wecomLoading.value = false
    ElMessage.error('无法获取企业微信登录地址，请确认服务端配置')
  }
}

onMounted(async () => {
  try {
    const st = await authApi.wecomStatus()
    wecomEnabled.value = st.enabled
  } catch {
    wecomEnabled.value = false
  }
})
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  min-height: 100dvh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px 16px;
  background:
    radial-gradient(circle at 15% 15%, rgba(64, 158, 255, 0.35), transparent 45%),
    radial-gradient(circle at 85% 80%, rgba(111, 66, 193, 0.4), transparent 45%),
    linear-gradient(135deg, #111827 0%, #1f2937 55%, #312e81 100%);
}

.login-panel {
  width: 100%;
  max-width: 400px;
  background: #fff;
  border-radius: 16px;
  padding: 36px 32px 28px;
  box-shadow: 0 24px 64px rgba(0, 0, 0, 0.35);
}

.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 28px;
}
.logo-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 46px;
  min-width: 46px;
  height: 46px;
  border-radius: 12px;
  background: linear-gradient(135deg, #409eff, #6f42c1);
  color: #fff;
  font-size: 24px;
  font-weight: 700;
}
.brand-title {
  font-size: 21px;
  font-weight: 700;
  color: #1f2d3d;
  line-height: 1.2;
}
.brand-sub {
  font-size: 12px;
  color: #909399;
  margin-top: 2px;
}

.login-error {
  margin-bottom: 14px;
}

.submit-btn {
  width: 100%;
  margin-top: 4px;
  font-weight: 600;
}

.login-divider {
  margin: 22px 0 16px;
  color: #b1b3b8;
  font-size: 12px;
}

.wecom-btn {
  width: 100%;
}

.hint {
  margin: 20px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: #a8abb2;
  text-align: center;
}

.footer-note {
  margin: 20px 0 0;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
  text-align: center;
}

/* Narrow phones: reduce chrome so the form stays above the fold. */
@media (max-width: 480px) {
  .login-page {
    padding: 16px 12px;
    justify-content: flex-start;
    padding-top: 12vh;
  }
  .login-panel {
    padding: 26px 20px 22px;
    border-radius: 14px;
  }
  .brand {
    margin-bottom: 22px;
  }
  .brand-title {
    font-size: 19px;
  }
  .logo-mark {
    width: 40px;
    min-width: 40px;
    height: 40px;
    font-size: 20px;
  }
}

/* Short landscape screens (phones rotated). */
@media (max-height: 560px) {
  .login-page {
    justify-content: flex-start;
    padding-top: 16px;
  }
  .brand {
    margin-bottom: 16px;
  }
  .hint,
  .footer-note {
    display: none;
  }
}
</style>
