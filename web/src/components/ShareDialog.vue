<script setup>
import { reactive, ref, watch } from 'vue'
import { Check, Copy, Link2, ShieldCheck } from 'lucide-vue-next'
import { ElMessage } from 'element-plus'
import { api, getErrorMessage } from '../api/client'
import { formatDate } from '../utils/format'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  dashboardId: { type: [Number, String], default: 0 },
  dashboardName: { type: String, default: '经营看板' },
})

const emit = defineEmits(['update:modelValue', 'created'])
const form = reactive({ expire_hours: 24, max_views: 0 })
const loading = ref(false)
const result = ref(null)

function close() {
  emit('update:modelValue', false)
}

watch(
  () => props.modelValue,
  (visible) => {
    if (visible) {
      result.value = null
      form.expire_hours = 24
      form.max_views = 0
    }
  },
)

async function createLink() {
  if (!props.dashboardId) return
  loading.value = true
  try {
    result.value = await api.post('/shares', {
      dashboard_id: Number(props.dashboardId),
      expire_hours: Number(form.expire_hours) || 24,
      max_views: Number(form.max_views) || 0,
    })
    emit('created', result.value)
    ElMessage.success('授权链接已生成')
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

async function copyLink() {
  if (!result.value?.url) return
  try {
    await navigator.clipboard.writeText(result.value.url)
  } catch {
    const input = document.createElement('textarea')
    input.value = result.value.url
    document.body.appendChild(input)
    input.select()
    document.execCommand('copy')
    input.remove()
  }
  ElMessage.success('链接已复制')
}
</script>

<template>
  <el-dialog :model-value="modelValue" title="授权分享" width="500px" @update:model-value="emit('update:modelValue', $event)">
    <div class="share-dialog-intro">
      <div class="share-dialog-icon"><ShieldCheck :size="20" /></div>
      <div>
        <strong>{{ dashboardName }}</strong>
        <p>链接会绑定当前账号的数据权限范围，访问者无需登录，但只能看到你可查看的数据。</p>
      </div>
    </div>

    <template v-if="!result">
      <div class="form-grid" style="margin-top: 20px">
        <label class="field-label">
          有效期
          <el-select v-model="form.expire_hours" style="margin-top: 8px">
            <el-option :value="1" label="1 小时" />
            <el-option :value="8" label="8 小时" />
            <el-option :value="24" label="24 小时" />
            <el-option :value="168" label="7 天" />
            <el-option :value="720" label="30 天" />
          </el-select>
        </label>
        <label class="field-label">
          最大访问次数
          <el-input-number v-model="form.max_views" :min="0" :max="100000" style="width: 100%; margin-top: 8px" />
        </label>
      </div>
      <p style="margin-top: 8px; color: var(--muted-light); font-size: 9px">访问次数填 0 表示不限制。</p>
    </template>

    <template v-else>
      <div class="share-result">
        <div class="share-result-label"><Link2 :size="13" /> 已生成的授权链接</div>
        <div class="share-result-row">
          <el-input :model-value="result.url" readonly />
          <button class="btn btn-secondary" type="button" @click="copyLink"><Copy :size="14" /> 复制</button>
        </div>
        <div class="share-result-meta">
          <span><Check :size="12" /> 到期时间：{{ formatDate(result.expires_at, true) }}</span>
          <span>最大访问：{{ result.max_views || '不限' }}</span>
        </div>
      </div>
    </template>

    <template #footer>
      <div class="inline-actions" style="justify-content: flex-end">
        <button class="btn btn-secondary" type="button" @click="close">关闭</button>
        <button v-if="!result" class="btn btn-primary" type="button" :disabled="loading" @click="createLink">
          {{ loading ? '生成中…' : '生成授权链接' }}
        </button>
        <button v-else class="btn btn-primary" type="button" @click="copyLink"><Copy :size="14" /> 复制链接</button>
      </div>
    </template>
  </el-dialog>
</template>
