<!-- 企业微信组织同步面板（后台管理）：
     配置 CorpID / AgentID / Secret 与部门层级映射规则，
     发起异步同步任务并实时轮询展示进度，完成后通知父页面刷新组织树 -->
<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  CircleCheck,
  CircleX,
  CloudDownload,
  Loader2,
  PlugZap,
  RefreshCcw,
  Settings2,
  Smartphone,
} from 'lucide-vue-next'
import { api, getErrorMessage } from '../api/client'

const emit = defineEmits(['synced'])

// 抽屉显隐与各操作加载状态
const visible = ref(false)
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const syncing = ref(false)
const roles = ref([])
const testResult = ref(null)
const job = ref(null)
const jobs = ref([])
const pollTimer = ref(null)

// 配置表单：字段与后端 WecomConfigView 对齐，secret 只用于提交，不回填明文
const form = reactive({
  corp_id: '',
  agent_id: '',
  secret: '',
  secret_configured: false,
  secret_masked: '',
  enabled: false,
  sync_users: true,
  login_enabled: false,
  login_mode: 'auto',
  auto_login: true,
  default_role_code: '',
  code_prefix: 'WECOM_DEPT_',
})

// 部门层级 → 组织类型的映射（d1~d4 对应一至四级），last 为更深层级的兜底类型
const rules = reactive({ d1: 'company', d2: 'big_region', d3: 'region', d4: 'store', last: 'store' })
// 发起同步时的任务级选项
const syncOptions = reactive({ sync_users: true, disable_missing: false })

const typeOptions = [
  { value: 'company', label: '集团 / 公司' },
  { value: 'big_region', label: '大区' },
  { value: 'region', label: '区域' },
  { value: 'store', label: '门店' },
]

// 同步任务状态 → 中文标签与标签样式
const statusMap = {
  pending: { label: '等待中', cls: 'tag-amber' },
  running: { label: '运行中', cls: 'tag-blue' },
  success: { label: '已完成', cls: 'tag-green' },
  partial: { label: '部分成功', cls: 'tag-amber' },
  failed: { label: '失败', cls: 'tag-red' },
}

// 是否存在正在执行的任务（用于禁用重复发起）
const running = computed(() => job.value && ['pending', 'running'].includes(job.value.status))
// 任务进度百分比：total 未知时给一个占位值，完成后固定 100%
const progress = computed(() => {
  if (!job.value) return 0
  const total = job.value.total || 0
  if (!total) return job.value.status === 'success' ? 100 : 4
  return Math.min(100, Math.round(((job.value.processed || 0) / total) * 100))
})

// 根据状态码取展示信息
function statusOf(status) {
  return statusMap[status] || { label: status || '未知', cls: '' }
}

// 统一格式化任务起止时间
function formatTime(value) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  const pad = (n) => String(n).padStart(2, '0')
  return `${date.getMonth() + 1}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

// 打开抽屉并加载配置、角色与历史任务
async function open() {
  visible.value = true
  await loadAll()
}

// 一次性加载抽屉所需的全部数据
async function loadAll() {
  loading.value = true
  try {
    const [configData, roleData, jobData] = await Promise.all([
      api.get('/admin/wecom/config'),
      api.get('/roles').catch(() => []),
      api.get('/admin/wecom/jobs?limit=8').catch(() => []),
    ])
    applyConfig(configData || {})
    roles.value = roleData || []
    jobs.value = jobData || []
    const active = jobs.value.find((item) => ['pending', 'running'].includes(item.status))
    if (active) {
      job.value = active
      startPolling()
    }
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

// 将后端配置回填到本地表单
function applyConfig(data) {
  Object.assign(form, {
    corp_id: data.corp_id || '',
    agent_id: data.agent_id || '',
    secret: '',
    secret_configured: Boolean(data.secret_configured),
    secret_masked: data.secret_masked || '',
    enabled: Boolean(data.enabled),
    sync_users: data.sync_users !== false,
    login_enabled: Boolean(data.login_enabled),
    login_mode: data.login_mode || 'auto',
    auto_login: data.auto_login !== false,
    default_role_code: data.default_role_code || '',
    code_prefix: data.code_prefix || 'WECOM_DEPT_',
  })
  const types = data.type_rules?.depth_types || []
  rules.d1 = types[0] || 'company'
  rules.d2 = types[1] || 'big_region'
  rules.d3 = types[2] || 'region'
  rules.d4 = types[3] || 'store'
  rules.last = data.type_rules?.last_type || 'store'
  syncOptions.sync_users = form.sync_users
}

// 组装保存 / 测试连接使用的请求体
function payload() {
  return {
    corp_id: form.corp_id.trim(),
    agent_id: form.agent_id.trim(),
    secret: form.secret.trim(),
    enabled: form.enabled,
    sync_users: form.sync_users,
    login_enabled: form.login_enabled,
    login_mode: form.login_mode,
    auto_login: form.auto_login,
    default_role_code: form.default_role_code,
    code_prefix: form.code_prefix.trim(),
    type_rules: {
      depth_types: [rules.d1, rules.d2, rules.d3, rules.d4],
      last_type: rules.last,
    },
  }
}

// 保存配置：Secret 留空或为掩码时后端会保留原密文
async function save() {
  if (!form.corp_id.trim()) {
    ElMessage.warning('请填写企业微信 CorpID')
    return
  }
  if (!form.secret_configured && !form.secret.trim()) {
    ElMessage.warning('请填写企业微信应用 Secret')
    return
  }
  saving.value = true
  try {
    const data = await api.put('/admin/wecom/config', payload())
    applyConfig(data || {})
    ElMessage.success('企业微信配置已保存')
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    saving.value = false
  }
}

// 测试企业微信应用凭证是否可用，并展示部门/成员数量
async function testConnection() {
  if (!form.corp_id.trim()) {
    ElMessage.warning('请先填写 CorpID')
    return
  }
  testing.value = true
  testResult.value = null
  try {
    testResult.value = await api.post('/admin/wecom/test', payload())
    if (testResult.value?.ok) {
      ElMessage.success(testResult.value.message || '连接成功')
    } else {
      ElMessage.error(testResult.value?.message || '连接失败')
    }
  } catch (error) {
    testResult.value = { ok: false, message: getErrorMessage(error) }
    ElMessage.error(getErrorMessage(error))
  } finally {
    testing.value = false
  }
}

// 发起异步同步任务，立即返回任务快照后开始轮询
async function startSync() {
  if (!form.enabled) {
    ElMessage.warning('请先保存配置并开启「组织同步」')
    return
  }
  syncing.value = true
  try {
    const data = await api.post('/admin/wecom/sync', {
      sync_users: syncOptions.sync_users,
      disable_missing: syncOptions.disable_missing,
    })
    job.value = data
    await loadJobs()
    startPolling()
    ElMessage.success('同步任务已创建，正在后台执行')
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    syncing.value = false
  }
}

// 刷新最近同步任务列表
async function loadJobs() {
  try {
    jobs.value = (await api.get('/admin/wecom/jobs?limit=8')) || []
  } catch {
    /* 忽略历史任务加载失败 */
  }
}

// 轮询入口：每 1.5 秒拉取一次任务进度
function startPolling() {
  stopPolling()
  pollTimer.value = setInterval(pollJob, 1500)
}

// 清理轮询定时器，避免组件销毁后继续请求
function stopPolling() {
  if (pollTimer.value) {
    clearInterval(pollTimer.value)
    pollTimer.value = null
  }
}

// 拉取任务进度；进入终态后停止轮询并通知父组件刷新组织树
async function pollJob() {
  if (!job.value?.id) {
    stopPolling()
    return
  }
  try {
    const data = await api.get(`/admin/wecom/jobs/${job.value.id}`)
    job.value = data
    if (!['pending', 'running'].includes(data?.status)) {
      stopPolling()
      await loadJobs()
      emit('synced')
      if (data?.status === 'success') {
        ElMessage.success('企业微信组织同步完成')
      } else if (data?.status === 'partial') {
        ElMessage.warning('同步部分成功，请查看失败明细')
      } else {
        ElMessage.error('企业微信组织同步失败')
      }
    }
  } catch {
    stopPolling()
  }
}

// 抽屉打开时先加载历史任务；组件销毁时清理定时器
onMounted(loadJobs)
onBeforeUnmount(stopPolling)
</script>

<template>
  <button class="btn btn-secondary btn-sm" type="button" @click="open">
    <RefreshCcw :size="14" /> 企业微信同步
  </button>

  <el-drawer v-model="visible" title="企业微信组织同步" size="580px" destroy-on-close>
    <div v-if="loading" class="wecom-loading">
      <div class="skeleton" style="height: 220px" />
    </div>

    <div v-else class="wecom-panel">
      <!-- 配置概览：组织同步 / 企业微信登录 / Secret 状态 -->
      <div class="wecom-summary">
        <div class="wecom-summary-item">
          <span>组织同步</span>
          <strong :class="form.enabled ? 'tag-green' : 'tag-red'">{{ form.enabled ? '已启用' : '未启用' }}</strong>
        </div>
        <div class="wecom-summary-item">
          <span>企微登录</span>
          <strong :class="form.login_enabled ? 'tag-green' : 'tag-red'">
            {{ form.login_enabled ? '已启用' : '未启用' }}
          </strong>
        </div>
        <div class="wecom-summary-item">
          <span>Secret</span>
          <strong class="tag">{{ form.secret_configured ? '已配置' : '未配置' }}</strong>
        </div>
      </div>

      <!-- 应用凭证：CorpID、AgentID 与加密保存的 Secret -->
      <section class="wecom-section">
        <h4><Settings2 :size="13" /> 应用配置</h4>
        <label class="field-label">
          CorpID
          <el-input v-model="form.corp_id" placeholder="企业微信 CorpID，例如 ww1234567890abcdef" style="margin-top: 8px" />
        </label>
        <label class="field-label" style="margin-top: 14px">
          AgentID
          <el-input v-model="form.agent_id" placeholder="自建应用 AgentID，扫码登录必填" style="margin-top: 8px" />
        </label>
        <label class="field-label" style="margin-top: 14px">
          应用 Secret
          <el-input
            v-model="form.secret"
            type="password"
            show-password
            :placeholder="form.secret_configured ? form.secret_masked + '（留空表示不修改）' : '企业微信自建应用 Secret'"
            style="margin-top: 8px"
          />
        </label>
        <p class="wecom-hint">
          Secret 使用 AES-GCM 加密存储，接口只返回掩码。请在企业微信后台配置可信域名与网页授权域名。
        </p>
      </section>

      <!-- 组织同步规则：开关、默认角色、编码前缀与层级映射 -->
      <section class="wecom-section">
        <h4><CloudDownload :size="13" /> 同步设置</h4>
        <div class="wecom-switch-row">
          <div>
            <strong>启用组织同步</strong>
            <span>开启后才允许发起通讯录同步任务</span>
          </div>
          <el-switch v-model="form.enabled" />
        </div>
        <div class="wecom-switch-row">
          <div>
            <strong>同步成员</strong>
            <span>按企业微信 UserID 创建或更新平台账号</span>
          </div>
          <el-switch v-model="form.sync_users" />
        </div>
        <label class="field-label" style="margin-top: 12px">
          新成员默认角色
          <el-select v-model="form.default_role_code" clearable placeholder="留空使用权限最低的角色" style="width: 100%; margin-top: 8px">
            <el-option v-for="role in roles" :key="role.code" :value="role.code" :label="`${role.name}（${role.code}）`" />
          </el-select>
        </label>
        <label class="field-label" style="margin-top: 14px">
          组织编码前缀
          <el-input v-model="form.code_prefix" placeholder="WECOM_DEPT_" style="margin-top: 8px" />
        </label>
        <div class="wecom-rules">
          <span class="wecom-rules-title">部门层级 → 组织类型</span>
          <div class="wecom-rules-grid">
            <el-select v-model="rules.d1" size="small">
              <el-option v-for="item in typeOptions" :key="item.value" :value="item.value" :label="`一级 · ${item.label}`" />
            </el-select>
            <el-select v-model="rules.d2" size="small">
              <el-option v-for="item in typeOptions" :key="item.value" :value="item.value" :label="`二级 · ${item.label}`" />
            </el-select>
            <el-select v-model="rules.d3" size="small">
              <el-option v-for="item in typeOptions" :key="item.value" :value="item.value" :label="`三级 · ${item.label}`" />
            </el-select>
            <el-select v-model="rules.d4" size="small">
              <el-option v-for="item in typeOptions" :key="item.value" :value="item.value" :label="`四级 · ${item.label}`" />
            </el-select>
          </div>
          <el-select v-model="rules.last" size="small" style="margin-top: 8px; width: 100%">
            <el-option v-for="item in typeOptions" :key="item.value" :value="item.value" :label="`更深的层级统一为 · ${item.label}`" />
          </el-select>
        </div>
      </section>

      <!-- 登录设置：企业微信授权登录方式与客户端内自动登录 -->
      <section class="wecom-section">
        <h4><Smartphone :size="13" /> 登录设置</h4>
        <div class="wecom-switch-row">
          <div>
            <strong>启用企业微信登录</strong>
            <span>员工可在登录页使用企业微信授权登录</span>
          </div>
          <el-switch v-model="form.login_enabled" />
        </div>
        <label class="field-label" style="margin-top: 12px">
          登录方式
          <el-select v-model="form.login_mode" style="width: 100%; margin-top: 8px">
            <el-option value="auto" label="自动（企微内静默授权，浏览器扫码）" />
            <el-option value="oauth" label="仅网页静默授权" />
            <el-option value="qr" label="仅扫码登录" />
          </el-select>
        </label>
        <div class="wecom-switch-row" style="margin-top: 12px">
          <div>
            <strong>企微内自动登录</strong>
            <span>在企业微信客户端内打开登录页时自动发起授权</span>
          </div>
          <el-switch v-model="form.auto_login" />
        </div>
      </section>

      <!-- 保存与连接测试 -->
      <div class="wecom-actions">
        <button class="btn btn-secondary btn-sm" type="button" :disabled="testing" @click="testConnection">
          <Loader2 v-if="testing" :size="14" class="spin" />
          <PlugZap v-else :size="14" /> {{ testing ? '测试中…' : '测试连接' }}
        </button>
        <button class="btn btn-primary btn-sm" type="button" :disabled="saving" @click="save">
          {{ saving ? '保存中…' : '保存配置' }}
        </button>
      </div>

      <!-- 连接测试结果 -->
      <div v-if="testResult" class="wecom-test" :class="testResult.ok ? 'ok' : 'fail'">
        <CircleCheck v-if="testResult.ok" :size="14" />
        <CircleX v-else :size="14" />
        <div>
          <strong>{{ testResult.message }}</strong>
          <span v-if="testResult.ok">
            部门 {{ testResult.department_count }} 个 · 成员 {{ testResult.user_count }} 人 · 耗时
            {{ testResult.latency_ms }}ms
          </span>
        </div>
      </div>

      <!-- 发起通讯录同步并展示当前任务进度 -->
      <section class="wecom-section">
        <h4><RefreshCcw :size="13" /> 通讯录同步</h4>
        <div class="wecom-sync-options">
          <el-checkbox v-model="syncOptions.sync_users">同步成员账号</el-checkbox>
          <el-checkbox v-model="syncOptions.disable_missing">停用企业微信中已移除的部门</el-checkbox>
        </div>
        <button class="btn btn-primary btn-sm" type="button" :disabled="syncing || running" @click="startSync">
          <RefreshCcw :size="14" /> {{ running ? '同步执行中…' : '立即同步' }}
        </button>

        <div v-if="job" class="wecom-job">
          <div class="wecom-job-head">
            <strong>任务 #{{ job.id }}</strong>
            <span class="tag" :class="statusOf(job.status).cls">{{ statusOf(job.status).label }}</span>
          </div>
          <div class="wecom-job-bar">
            <div class="wecom-job-bar-fill" :style="{ width: progress + '%' }" />
          </div>
          <div class="wecom-job-metrics">
            <span>进度 <b>{{ job.processed || 0 }}/{{ job.total || 0 }}</b></span>
            <span>部门 <b>{{ job.departments || 0 }}</b></span>
            <span>新增 <b>{{ job.created || 0 }}</b></span>
            <span>更新 <b>{{ job.updated || 0 }}</b></span>
            <span>跳过 <b>{{ job.skipped || 0 }}</b></span>
            <span>失败 <b>{{ job.failed || 0 }}</b></span>
          </div>
          <p class="wecom-job-msg">{{ job.message || '—' }}</p>
          <p class="wecom-job-time">
            开始 {{ formatTime(job.started_at) }} · 结束 {{ formatTime(job.finished_at) }}
          </p>
          <pre v-if="job.error" class="wecom-job-error">{{ job.error }}</pre>
        </div>
      </section>

      <!-- 最近同步记录，便于回溯失败原因 -->
      <section v-if="jobs.length" class="wecom-section">
        <h4>最近同步记录</h4>
        <div class="wecom-history">
          <div v-for="item in jobs" :key="item.id" class="wecom-history-row">
            <span class="tag" :class="statusOf(item.status).cls">{{ statusOf(item.status).label }}</span>
            <span class="mono">#{{ item.id }}</span>
            <span>{{ item.departments || 0 }} 部门</span>
            <span>{{ item.message || '—' }}</span>
            <span class="wecom-history-time">{{ formatTime(item.created_at) }}</span>
          </div>
        </div>
      </section>
    </div>
  </el-drawer>
</template>

<style scoped>
.wecom-loading {
  padding: 8px;
}

.wecom-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding-bottom: 24px;
}

.wecom-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.wecom-summary-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px 14px;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: #fbfcfb;
}

.wecom-summary-item span {
  color: var(--muted);
  font-size: 9px;
}

.wecom-summary-item strong {
  align-self: flex-start;
}

.wecom-section {
  padding: 16px;
  border: 1px solid var(--line);
  border-radius: 14px;
  background: white;
}

.wecom-section h4 {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 14px;
  font-size: 12px;
  font-weight: 700;
}

.wecom-hint {
  margin-top: 10px;
  color: var(--muted);
  font-size: 9px;
  line-height: 1.7;
}

.wecom-switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 0;
}

.wecom-switch-row strong {
  display: block;
  font-size: 10.5px;
}

.wecom-switch-row span {
  display: block;
  margin-top: 3px;
  color: var(--muted);
  font-size: 9px;
  line-height: 1.5;
}

.wecom-rules {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px dashed var(--line);
}

.wecom-rules-title {
  display: block;
  margin-bottom: 8px;
  color: var(--muted);
  font-size: 9px;
}

.wecom-rules-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.wecom-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.wecom-test {
  display: flex;
  gap: 8px;
  padding: 12px 14px;
  border-radius: 12px;
  font-size: 9.5px;
  line-height: 1.6;
}

.wecom-test strong {
  display: block;
  font-size: 10.5px;
}

.wecom-test span {
  color: var(--muted);
}

.wecom-test.ok {
  border: 1px solid #cdeadd;
  background: #f2fbf6;
  color: #16794f;
}

.wecom-test.fail {
  border: 1px solid #f1cdd2;
  background: #fdf4f5;
  color: #b33a4c;
}

.wecom-sync-options {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 18px;
  margin-bottom: 12px;
}

.wecom-job {
  margin-top: 14px;
  padding: 14px;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: #fbfcfb;
}

.wecom-job-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
  font-size: 10.5px;
}

.wecom-job-bar {
  height: 6px;
  overflow: hidden;
  border-radius: 6px;
  background: #e9efeb;
}

.wecom-job-bar-fill {
  height: 100%;
  border-radius: 6px;
  background: var(--brand);
  transition: width 0.4s ease;
}

.wecom-job-metrics {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 16px;
  margin-top: 10px;
  color: var(--muted);
  font-size: 9px;
}

.wecom-job-metrics b {
  color: var(--ink);
  font-family: var(--font-number);
  font-variant-numeric: tabular-nums;
}

.wecom-job-msg {
  margin-top: 8px;
  font-size: 9.5px;
  line-height: 1.6;
}

.wecom-job-time {
  margin-top: 4px;
  color: var(--muted-light);
  font-size: 8.5px;
}

.wecom-job-error {
  max-height: 160px;
  margin-top: 10px;
  overflow: auto;
  padding: 10px;
  border-radius: 8px;
  background: #fdf4f5;
  color: #b33a4c;
  font-size: 8.5px;
  line-height: 1.6;
  white-space: pre-wrap;
}

.wecom-history {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.wecom-history-row {
  display: grid;
  grid-template-columns: 62px 42px 62px 1fr auto;
  align-items: center;
  gap: 8px;
  font-size: 9px;
}

.wecom-history-row > span:nth-child(4) {
  overflow: hidden;
  color: var(--muted);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.wecom-history-time {
  color: var(--muted-light);
  font-size: 8.5px;
}

/* 窄屏自适应：概览与层级映射改为单列，历史记录隐藏次要列 */
@media (max-width: 640px) {
  .wecom-summary,
  .wecom-rules-grid {
    grid-template-columns: 1fr;
  }

  .wecom-history-row {
    grid-template-columns: 62px 40px 1fr auto;
  }

  .wecom-history-row > span:nth-child(3) {
    display: none;
  }
}
</style>

<style>
/* 抽屉挂载在 body 下，窄屏时限制最大宽度避免横向溢出 */
.el-drawer {
  max-width: 100vw;
}
</style>
