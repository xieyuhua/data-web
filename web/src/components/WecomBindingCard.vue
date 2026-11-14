<!-- 企业微信账号绑定卡片：
     已绑定展示 UserID，未绑定引导跳转企业微信 OAuth 授权；
     仅在企业微信登录开关打开时展示 -->
<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Link2, Loader2, Smartphone, Unlink } from 'lucide-vue-next'
import { api, getErrorMessage } from '../api/client'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
// 企业微信登录开关：未启用时整个卡片不展示
const enabled = ref(false)
const loading = ref(true)
const binding = ref(false)
const unbinding = ref(false)

// 当前账号绑定的企业微信 UserID
const wecomUserID = computed(() => auth.user?.wecom_user_id || '')
const bound = computed(() => Boolean(wecomUserID.value))

// 读取公开的企业微信登录配置，判断是否需要展示绑定入口
async function load() {
  loading.value = true
  try {
    const data = await api.get('/auth/wecom/config')
    enabled.value = Boolean(data?.enabled)
  } catch {
    enabled.value = false
  } finally {
    loading.value = false
  }
}

// 跳转企业微信授权页完成绑定（回调地址固定回数据权限页）
async function bind() {
  binding.value = true
  try {
    const data = await api.get('/auth/wecom/bind-url?redirect=/access')
    if (!data?.url) {
      ElMessage.error('无法生成企业微信授权地址')
      return
    }
    window.location.href = data.url
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    binding.value = false
  }
}

// 解除当前账号与企业微信的绑定关系
async function unbind() {
  try {
    await ElMessageBox.confirm('解除绑定后将无法使用企业微信直接登录，需重新绑定才能恢复。', '解除企业微信绑定', {
      type: 'warning',
      confirmButtonText: '解除绑定',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  unbinding.value = true
  try {
    await api.delete('/auth/wecom/bind')
    await auth.fetchMe()
    ElMessage.success('已解除企业微信绑定')
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    unbinding.value = false
  }
}

onMounted(load)
</script>

<template>
  <div v-if="!loading && enabled" class="scope-card">
    <h3><Smartphone :size="13" style="vertical-align: -2px" /> 企业微信绑定</h3>
    <!-- 已绑定展示 UserID，未绑定引导授权 -->
    <p v-if="bound">
      当前账号已绑定企业微信 <strong class="mono">{{ wecomUserID }}</strong>，可在企业微信客户端内一键登录。
    </p>
    <p v-else>尚未绑定企业微信。绑定后可在企业微信客户端内直接授权登录，无需输入账号密码。</p>
    <div class="inline-actions" style="margin-top: 12px">
      <button v-if="!bound" class="btn btn-primary btn-sm" type="button" :disabled="binding" @click="bind">
        <Loader2 v-if="binding" :size="13" class="spin" />
        <Link2 v-else :size="13" /> {{ binding ? '正在跳转…' : '绑定企业微信' }}
      </button>
      <button v-else class="btn btn-secondary btn-sm" type="button" :disabled="unbinding" @click="unbind">
        <Unlink :size="13" /> {{ unbinding ? '解除中…' : '解除绑定' }}
      </button>
    </div>
  </div>
</template>
