<!-- 推送中心：配置定时推送任务与推送模板（企业微信 / 微信），自定义卡片样式，查看发送记录并支持重试。 -->
<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  Clock3,
  Eye,
  MessageCircle,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Send,
  Settings2,
  Trash2,
  Users,
} from 'lucide-vue-next'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, getErrorMessage } from '../../api/client'
import { formatDate, statusMeta } from '../../utils/format'
import { flattenOrgs } from '../../utils/org'
import EmptyBlock from '../../components/EmptyBlock.vue'
import OrgTreeSelect from '../../components/OrgTreeSelect.vue'

const activeTab = ref('tasks')
const loading = ref(true)

const subscriptions = ref([])
const templates = ref([])
const records = ref([])
const dashboards = ref([])
const users = ref([])
const roles = ref([])
const orgTree = ref([])

const orgs = computed(() => flattenOrgs(orgTree.value))
const userMap = computed(() => Object.fromEntries(users.value.map((item) => [item.id, item.name])))
const roleMap = computed(() => Object.fromEntries(roles.value.map((item) => [item.code, item.name])))
const orgMap = computed(() => Object.fromEntries(orgs.value.map((item) => [item.id, item.name])))
const dashboardMap = computed(() => Object.fromEntries(dashboards.value.map((item) => [item.id, item.name])))

const channelLabels = {
  wecom: '企业微信',
  wechat: '微信公众号',
}

const templateChannelLabels = {
  wecom_text: '企业微信文本',
  wecom_textcard: '企业微信卡片',
  wechat: '公众号模板',
}

const cronPresets = [
  { label: '每天 09:00', value: '0 9 * * *' },
  { label: '每天 18:00', value: '0 18 * * *' },
  { label: '每周一 09:00', value: '0 9 * * 1' },
  { label: '每月 1 日 09:00', value: '0 9 1 * *' },
]

// 安全解析 JSON 字符串
function safeParse(value) {
  if (!value) return {}
  if (typeof value === 'object') return value
  try {
    return JSON.parse(value) || {}
  } catch {
    return {}
  }
}

const sampleValues = {
  DashboardName: '销售经营日报',
  RecipientName: '张店长',
  Period: '2026-11-13',
  Revenue: '128,560.00',
  Orders: '328',
  GrossProfit: '47,320.00',
  Customers: '291',
  URL: 'https://example.com/share/preview',
}

// 把后端错误信息转换为更友好的提示
function friendly(text) {
  return String(text || '').replace(/\{\{\s*\.?([A-Za-z0-9_]+)\s*\}\}/g, (_, key) => sampleValues[key] ?? '')
}

// 解析推送模板的卡片样式配置
function styleOf(template) {
  const style = safeParse(template?.style_json)
  return {
    button_text: style.button_text || '查看经营看板',
    accent_color: style.accent_color || '#0F766E',
    bg_color: style.bg_color || '#F2FBF7',
  }
}

// 推送渠道码转中文
function channelLabel(channel) {
  return channelLabels[channel] || channel
}

// 模板渠道码转中文
function templateChannelLabel(channel) {
  return templateChannelLabels[channel] || channel
}

// 收件人展示文案（按角色 / 组织 / 用户）
function recipientText(subscription) {
  const rule = subscription?.recipients || {}
  const parts = []
  for (const id of rule.user_ids || []) {
    if (userMap.value[id]) parts.push(userMap.value[id])
  }
  for (const code of rule.roles || []) {
    if (roleMap.value[code]) parts.push(roleMap.value[code])
  }
  for (const id of rule.org_ids || []) {
    if (orgMap.value[id]) parts.push(orgMap.value[id])
  }
  return parts.length ? parts.join('、') : '全部启用用户'
}

// 加载推送任务列表
async function loadTasks() {
  subscriptions.value = (await api.get('/admin/push/subscriptions')) || []
}

// 加载推送模板列表
async function loadTemplates() {
  templates.value = (await api.get('/admin/push/templates')) || []
}

// 加载发送记录
async function loadRecords() {
  records.value = (await api.get('/admin/push/records?limit=100')) || []
}

const wecom = reactive({
  enabled: false,
  corp_id: '',
  agent_id: 1000002,
  secret: '',
  webhook_url: '',
  dry_run: true,
})

const wechat = reactive({
  enabled: false,
  app_id: '',
  secret: '',
  template_id: '',
  dry_run: true,
})

// 应用渠道配置到本地状态
function applyChannels(items) {
  for (const item of items || []) {
    const config = safeParse(item.config)
    if (item.channel === 'wecom') {
      wecom.enabled = Boolean(item.enabled)
      wecom.corp_id = config.corp_id || ''
      wecom.agent_id = Number(config.agent_id) || 1000002
      wecom.secret = config.secret || ''
      wecom.webhook_url = config.webhook_url || ''
      wecom.dry_run = config.dry_run !== false
    }
    if (item.channel === 'wechat') {
      wechat.enabled = Boolean(item.enabled)
      wechat.app_id = config.app_id || ''
      wechat.secret = config.secret || ''
      wechat.template_id = config.template_id || ''
      wechat.dry_run = config.dry_run !== false
    }
  }
}

// 加载企业微信 / 微信公众号渠道配置
async function loadChannels() {
  const items = (await api.get('/admin/push/channels')) || []
  applyChannels(items)
}

// 一次性加载推送中心全部数据
async function loadAll() {
  loading.value = true
  try {
    const [subs, tpls, dash, userList, roleList, tree] = await Promise.all([
      api.get('/admin/push/subscriptions').catch(() => []),
      api.get('/admin/push/templates').catch(() => []),
      api.get('/dashboards').catch(() => []),
      api.get('/users').catch(() => []),
      api.get('/roles').catch(() => []),
      api.get('/orgs/tree').catch(() => []),
    ])
    subscriptions.value = subs || []
    templates.value = tpls || []
    dashboards.value = dash || []
    users.value = userList || []
    roles.value = roleList || []
    orgTree.value = tree || []
    await loadChannels()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

/* ---------------- 推送任务 ---------------- */

const taskVisible = ref(false)
const taskSaving = ref(false)
const editingTaskId = ref(null)
const taskForm = reactive({
  name: '',
  dashboard_id: null,
  cron: '0 9 * * *',
  timezone: 'Asia/Shanghai',
  channels: ['wecom'],
  user_ids: [],
  roles: [],
  org_ids: [],
  template_id: null,
  enabled: true,
})

// 重置推送任务表单
function resetTaskForm() {
  editingTaskId.value = null
  Object.assign(taskForm, {
    name: '',
    dashboard_id: dashboards.value[0]?.id || null,
    cron: '0 9 * * *',
    timezone: 'Asia/Shanghai',
    channels: ['wecom'],
    user_ids: [],
    roles: [],
    org_ids: [],
    template_id: templates.value[0]?.id || null,
    enabled: true,
  })
}

// 打开新建推送任务弹窗
function openTaskCreate() {
  resetTaskForm()
  taskVisible.value = true
}

// 打开编辑推送任务弹窗
function openTaskEdit(row) {
  resetTaskForm()
  editingTaskId.value = row.id
  const rule = row.recipients || {}
  Object.assign(taskForm, {
    name: row.name || '',
    dashboard_id: row.dashboard_id || null,
    cron: row.cron || '0 9 * * *',
    timezone: row.timezone || 'Asia/Shanghai',
    channels: row.channels?.length ? [...row.channels] : ['wecom'],
    user_ids: [...(rule.user_ids || [])],
    roles: [...(rule.roles || [])],
    org_ids: [...(rule.org_ids || [])],
    template_id: row.template_id || null,
    enabled: row.enabled !== false,
  })
  taskVisible.value = true
}

// 保存推送任务（含定时规则）
async function saveTask() {
  if (!taskForm.name.trim()) {
    ElMessage.warning('请填写任务名称')
    return
  }
  if (!taskForm.dashboard_id) {
    ElMessage.warning('请选择推送看板')
    return
  }
  if (!taskForm.cron.trim()) {
    ElMessage.warning('请填写 Cron 表达式')
    return
  }
  if (!taskForm.channels.length) {
    ElMessage.warning('请至少选择一个推送通道')
    return
  }
  taskSaving.value = true
  try {
    const payload = {
      name: taskForm.name.trim(),
      dashboard_id: Number(taskForm.dashboard_id),
      cron: taskForm.cron.trim(),
      timezone: taskForm.timezone.trim() || 'Asia/Shanghai',
      channels: taskForm.channels,
      recipients: {
        user_ids: taskForm.user_ids.map(Number),
        roles: taskForm.roles,
        org_ids: taskForm.org_ids.map(Number),
      },
      template_id: taskForm.template_id ? Number(taskForm.template_id) : null,
      enabled: taskForm.enabled,
    }
    if (editingTaskId.value) {
      await api.put(`/admin/push/subscriptions/${editingTaskId.value}`, payload)
      ElMessage.success('推送任务已更新')
    } else {
      await api.post('/admin/push/subscriptions', payload)
      ElMessage.success('推送任务已创建')
    }
    taskVisible.value = false
    await loadTasks()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    taskSaving.value = false
  }
}

// 删除推送任务
async function removeTask(row) {
  try {
    await ElMessageBox.confirm(`确认删除推送任务「${row.name}」？`, '删除任务', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
    await api.delete(`/admin/push/subscriptions/${row.id}`)
    ElMessage.success('推送任务已删除')
    await loadTasks()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(getErrorMessage(error))
  }
}

// 立即执行一次推送
async function sendNow(row) {
  try {
    await ElMessageBox.confirm(
      `立即向「${row.name}」的接收人发送一次销售数据卡片？未配置凭证时将以模拟方式记录。`,
      '立即发送',
      { confirmButtonText: '发送', cancelButtonText: '取消' },
    )
    const result = await api.post(`/admin/push/subscriptions/${row.id}/send`)
    await ElMessageBox.alert(
      `共处理 ${result?.total || 0} 条：成功 ${result?.sent || 0}，模拟 ${result?.simulated || 0}，` +
        `跳过 ${result?.skipped || 0}，失败 ${result?.failed || 0}。`,
      '推送结果',
      { type: result?.failed ? 'warning' : 'success', confirmButtonText: '知道了' },
    )
    await Promise.all([loadTasks(), loadRecords()])
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(getErrorMessage(error))
  }
}

// 重试失败的推送记录
async function retryRecord(row) {
  if (!row.subscription_id) {
    ElMessage.warning('该记录未关联定时任务，无法重试')
    return
  }
  try {
    await ElMessageBox.confirm('将重新执行该记录所属的推送任务，并生成新的推送记录。', '重试推送', {
      confirmButtonText: '重试',
      cancelButtonText: '取消',
    })
    const result = await api.post(`/admin/push/records/${row.id}/retry`)
    ElMessage.success(
      `重试完成：成功 ${result?.sent || 0}，模拟 ${result?.simulated || 0}，失败 ${result?.failed || 0}`,
    )
    await Promise.all([loadTasks(), loadRecords()])
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(getErrorMessage(error))
  }
}

/* ---------------- 推送模板 ---------------- */

const templateVisible = ref(false)
const templateSaving = ref(false)
const editingTemplateId = ref(null)
const templateForm = reactive({
  name: '',
  channel: 'wecom_textcard',
  title_template: '{{.DashboardName}} 经营日报',
  content_template:
    '{{.RecipientName}}，您好！\n统计周期：{{.Period}}\n销售额：¥{{.Revenue}}\n订单数：{{.Orders}}\n毛利：¥{{.GrossProfit}}\n客流：{{.Customers}}\n\n点击查看完整经营看板。',
  button_text: '查看经营看板',
  accent_color: '#0F766E',
  bg_color: '#F2FBF7',
  enabled: true,
})

// 重置模板表单
function resetTemplateForm() {
  editingTemplateId.value = null
  Object.assign(templateForm, {
    name: '',
    channel: 'wecom_textcard',
    title_template: '{{.DashboardName}} 经营日报',
    content_template:
      '{{.RecipientName}}，您好！\n统计周期：{{.Period}}\n销售额：¥{{.Revenue}}\n订单数：{{.Orders}}\n毛利：¥{{.GrossProfit}}\n客流：{{.Customers}}\n\n点击查看完整经营看板。',
    button_text: '查看经营看板',
    accent_color: '#0F766E',
    bg_color: '#F2FBF7',
    enabled: true,
  })
}

// 打开新建推送模板弹窗
function openTemplateCreate() {
  resetTemplateForm()
  templateVisible.value = true
}

// 打开编辑推送模板弹窗
function openTemplateEdit(row) {
  resetTemplateForm()
  const style = styleOf(row)
  editingTemplateId.value = row.id
  Object.assign(templateForm, {
    name: row.name || '',
    channel: row.channel || 'wecom_textcard',
    title_template: row.title_template || '',
    content_template: row.content_template || '',
    button_text: style.button_text,
    accent_color: style.accent_color,
    bg_color: style.bg_color,
    enabled: row.enabled !== false,
  })
  templateVisible.value = true
}

// 保存推送模板（卡片样式可自定义）
async function saveTemplate() {
  if (!templateForm.name.trim() || !templateForm.title_template.trim() || !templateForm.content_template.trim()) {
    ElMessage.warning('请填写模板名称、标题和正文')
    return
  }
  templateSaving.value = true
  try {
    const payload = {
      name: templateForm.name.trim(),
      channel: templateForm.channel,
      title_template: templateForm.title_template,
      content_template: templateForm.content_template,
      style_json: JSON.stringify({
        button_text: templateForm.button_text,
        accent_color: templateForm.accent_color,
        bg_color: templateForm.bg_color,
      }),
      enabled: templateForm.enabled,
    }
    if (editingTemplateId.value) {
      await api.put(`/admin/push/templates/${editingTemplateId.value}`, payload)
      ElMessage.success('推送模板已更新')
    } else {
      await api.post('/admin/push/templates', payload)
      ElMessage.success('推送模板已创建')
    }
    templateVisible.value = false
    await loadTemplates()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    templateSaving.value = false
  }
}

// 删除推送模板
async function removeTemplate(row) {
  try {
    await ElMessageBox.confirm(`确认删除推送模板「${row.name}」？`, '删除模板', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
    await api.delete(`/admin/push/templates/${row.id}`)
    ElMessage.success('推送模板已删除')
    await loadTemplates()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(getErrorMessage(error))
  }
}

/* ---------------- 模板预览 ---------------- */

const previewVisible = ref(false)
const previewLoading = ref(false)
const previewTemplateId = ref(null)
const previewDashboardId = ref(null)
const previewResult = ref(null)

// 打开推送内容预览
function openPreview(row) {
  previewTemplateId.value = row.id
  previewDashboardId.value = dashboards.value[0]?.id || null
  previewResult.value = null
  previewVisible.value = true
  loadPreview()
}

// 加载推送预览数据
async function loadPreview() {
  if (!previewTemplateId.value || !previewDashboardId.value) return
  previewLoading.value = true
  try {
    const query = new URLSearchParams({
      template_id: String(previewTemplateId.value),
      dashboard_id: String(previewDashboardId.value),
    })
    previewResult.value = await api.get(`/admin/push/preview?${query.toString()}`)
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    previewLoading.value = false
  }
}

const previewStyle = computed(() => {
  const style = previewResult.value?.style || {}
  return {
    button_text: style.button_text || '查看经营看板',
    accent_color: style.accent_color || '#0F766E',
    bg_color: style.bg_color || '#F2FBF7',
  }
})

/* ---------------- 通道配置 ---------------- */

const channelSaving = ref('')

// 保存渠道配置（Secret 加密存储）
async function saveChannel(key) {
  channelSaving.value = key
  try {
    const config =
      key === 'wecom'
        ? {
            corp_id: wecom.corp_id,
            agent_id: Number(wecom.agent_id) || 0,
            secret: wecom.secret,
            webhook_url: wecom.webhook_url,
            dry_run: wecom.dry_run,
          }
        : {
            app_id: wechat.app_id,
            secret: wechat.secret,
            template_id: wechat.template_id,
            dry_run: wechat.dry_run,
          }
    await api.put(`/admin/push/channels/${key}`, {
      name: key === 'wecom' ? '企业微信' : '微信公众号',
      enabled: key === 'wecom' ? wecom.enabled : wechat.enabled,
      config,
    })
    ElMessage.success('通道配置已保存')
    await loadChannels()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    channelSaving.value = ''
  }
}

watch(activeTab, (tab) => {
  if (tab === 'records' && !records.value.length) loadRecords().catch(() => {})
})

onMounted(loadAll)
</script>

<template>
  <div>
    <el-tabs v-model="activeTab" class="push-tabs">
      <el-tab-pane label="推送任务" name="tasks">
        <div class="section-head">
          <div>
            <h2>定时推送任务</h2>
            <p>按 Cron 定时把销售数据卡片推送到企业微信或微信公众号，卡片链接带服务端授权。</p>
          </div>
          <button class="btn btn-primary btn-sm" type="button" @click="openTaskCreate">
            <Plus :size="14" /> 新建任务
          </button>
        </div>

        <div v-if="loading" class="schedule-grid">
          <div v-for="index in 4" :key="index" class="panel" style="height: 190px; padding: 18px">
            <div class="skeleton" style="height: 100%" />
          </div>
        </div>

        <div v-else-if="subscriptions.length" class="schedule-grid">
          <article v-for="item in subscriptions" :key="item.id" class="schedule-card">
            <div class="schedule-head">
              <div>
                <h3>{{ item.name }}</h3>
                <p>{{ item.dashboard_name || dashboardMap[item.dashboard_id] || '未知看板' }}</p>
              </div>
              <span class="tag" :class="item.enabled ? 'tag-green' : 'tag-red'">
                {{ item.enabled ? '启用' : '停用' }}
              </span>
            </div>
            <div class="schedule-details">
              <div class="schedule-detail">
                <span>Cron 表达式</span>
                <strong>{{ item.cron }}</strong>
              </div>
              <div class="schedule-detail">
                <span>推送通道</span>
                <strong>{{ (item.channels || []).map(channelLabel).join('、') }}</strong>
              </div>
              <div class="schedule-detail">
                <span>接收人</span>
                <strong :title="recipientText(item)" style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                  {{ recipientText(item) }}
                </strong>
              </div>
              <div class="schedule-detail">
                <span>上次状态</span>
                <strong>
                  <span class="tag" :class="statusMeta(item.last_status).className">
                    {{ statusMeta(item.last_status).label }}
                  </span>
                </strong>
              </div>
              <div class="schedule-detail">
                <span>下次执行</span>
                <strong style="font-size: 11px">
                  {{ item.enabled && item.next_run_at ? formatDate(item.next_run_at, true) : item.enabled ? '待调度' : '已停用' }}
                </strong>
              </div>
            </div>
            <div class="schedule-foot">
              <span style="color: var(--muted-light); font-size: 10.5px">
                {{ item.last_run_at ? `上次 ${formatDate(item.last_run_at, true)}` : '尚未执行' }}
                <template v-if="item.template_name"> · {{ item.template_name }}</template>
              </span>
              <div class="inline-actions">
                <button class="btn btn-secondary btn-sm" type="button" @click="sendNow(item)">
                  <Play :size="13" /> 发送
                </button>
                <button class="btn btn-ghost btn-sm" type="button" @click="openTaskEdit(item)">
                  <Pencil :size="13" />
                </button>
                <button class="btn btn-ghost btn-sm" type="button" style="color: var(--rose)" @click="removeTask(item)">
                  <Trash2 :size="13" />
                </button>
              </div>
            </div>
          </article>
        </div>

        <div v-else class="panel">
          <EmptyBlock title="还没有推送任务" description="创建定时任务，把经营看板按日或按周推送到店长和区域经理。">
            <button class="btn btn-primary btn-sm" type="button" @click="openTaskCreate">
              <Plus :size="14" /> 新建任务
            </button>
          </EmptyBlock>
        </div>
      </el-tab-pane>

      <el-tab-pane label="推送模板" name="templates">
        <div class="section-head">
          <div>
            <h2>消息样式模板</h2>
            <p>后台自定义标题、正文与卡片按钮样式，支持企业微信文本、文本卡片和公众号模板。</p>
          </div>
          <button class="btn btn-primary btn-sm" type="button" @click="openTemplateCreate">
            <Plus :size="14" /> 新建模板
          </button>
        </div>

        <div v-if="loading" class="push-template-grid">
          <div v-for="index in 3" :key="index" class="panel" style="height: 240px; padding: 18px">
            <div class="skeleton" style="height: 100%" />
          </div>
        </div>

        <div v-else-if="templates.length" class="push-template-grid">
          <article v-for="item in templates" :key="item.id" class="push-style-card">
            <div style="display: flex; align-items: center; justify-content: space-between; gap: 8px">
              <span class="tag tag-green">{{ templateChannelLabel(item.channel) }}</span>
              <span class="tag" :class="item.enabled ? 'tag-green' : 'tag-red'">
                {{ item.enabled ? '启用' : '停用' }}
              </span>
            </div>
            <div class="template-name" style="margin-top: 11px">{{ item.name }}</div>
            <div
              class="wecom-preview"
              :style="{ background: styleOf(item).bg_color, borderColor: `${styleOf(item).accent_color}22` }"
            >
              <h4>{{ friendly(item.title_template) }}</h4>
              <p>{{ friendly(item.content_template) }}</p>
              <span class="fake-btn" :style="{ background: styleOf(item).accent_color }">
                {{ styleOf(item).button_text }}
              </span>
            </div>
            <div class="push-template-meta">
              <span style="color: var(--muted-light); font-size: 10.5px">更新 {{ formatDate(item.updated_at) }}</span>
              <div class="inline-actions">
                <button class="btn btn-ghost btn-sm" type="button" @click="openPreview(item)">
                  <Eye :size="13" />
                </button>
                <button class="btn btn-ghost btn-sm" type="button" @click="openTemplateEdit(item)">
                  <Pencil :size="13" />
                </button>
                <button class="btn btn-ghost btn-sm" type="button" style="color: var(--rose)" @click="removeTemplate(item)">
                  <Trash2 :size="13" />
                </button>
              </div>
            </div>
          </article>
        </div>

        <div v-else class="panel">
          <EmptyBlock title="还没有推送模板" description="创建企业微信文本卡片或公众号模板，供定时任务引用。">
            <button class="btn btn-primary btn-sm" type="button" @click="openTemplateCreate">
              <Plus :size="14" /> 新建模板
            </button>
          </EmptyBlock>
        </div>
      </el-tab-pane>

      <el-tab-pane label="通道配置" name="channels">
        <div class="section-head">
          <div>
            <h2>推送通道</h2>
            <p>配置企业微信应用、企业微信机器人或微信公众号。开启演练模式时只记录模拟发送。</p>
          </div>
          <button class="btn btn-secondary btn-sm" type="button" @click="loadChannels">
            <RefreshCw :size="14" /> 刷新
          </button>
        </div>

        <div class="channel-config-grid">
          <section class="channel-card">
            <div class="channel-head">
              <div class="channel-logo"><MessageCircle :size="20" /></div>
              <div>
                <h3>企业微信</h3>
                <p>应用消息支持文本与文本卡片，也支持群机器人 Webhook。</p>
              </div>
              <el-switch v-model="wecom.enabled" />
            </div>
            <div class="form-grid">
              <label class="field-label">
                CorpID
                <el-input v-model="wecom.corp_id" placeholder="企业 ID" style="margin-top: 8px" />
              </label>
              <label class="field-label">
                AgentID
                <el-input-number v-model="wecom.agent_id" :min="0" style="width: 100%; margin-top: 8px" />
              </label>
              <label class="field-label form-span-2">
                Secret
                <el-input
                  v-model="wecom.secret"
                  type="password"
                  show-password
                  placeholder="已保存时显示为 ••••••••，留空表示不修改"
                  style="margin-top: 8px"
                />
              </label>
              <label class="field-label form-span-2">
                群机器人 Webhook
                <el-input
                  v-model="wecom.webhook_url"
                  placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..."
                  style="margin-top: 8px"
                />
              </label>
            </div>
            <div class="scope-note" style="margin-top: 14px">
              <el-switch v-model="wecom.dry_run" />
              <span>演练模式：不真实发送，仅记录模拟推送结果</span>
            </div>
            <button
              class="btn btn-primary btn-sm"
              type="button"
              style="margin-top: 14px"
              :disabled="channelSaving === 'wecom'"
              @click="saveChannel('wecom')"
            >
              <Settings2 :size="14" /> {{ channelSaving === 'wecom' ? '保存中…' : '保存企业微信配置' }}
            </button>
          </section>

          <section class="channel-card">
            <div class="channel-head">
              <div class="channel-logo wechat"><MessageCircle :size="20" /></div>
              <div>
                <h3>微信公众号</h3>
                <p>通过模板消息推送销售卡片，点击后打开授权看板。</p>
              </div>
              <el-switch v-model="wechat.enabled" />
            </div>
            <div class="form-grid">
              <label class="field-label">
                AppID
                <el-input v-model="wechat.app_id" placeholder="公众号 AppID" style="margin-top: 8px" />
              </label>
              <label class="field-label">
                模板 ID
                <el-input v-model="wechat.template_id" placeholder="模板消息 ID" style="margin-top: 8px" />
              </label>
              <label class="field-label form-span-2">
                AppSecret
                <el-input
                  v-model="wechat.secret"
                  type="password"
                  show-password
                  placeholder="已保存时显示为 ••••••••，留空表示不修改"
                  style="margin-top: 8px"
                />
              </label>
            </div>
            <div class="scope-note" style="margin-top: 14px">
              <el-switch v-model="wechat.dry_run" />
              <span>演练模式：不真实发送，仅记录模拟推送结果</span>
            </div>
            <p style="margin-top: 12px; color: var(--muted-light); font-size: 11px; line-height: 1.7">
              公众号模板消息需要已认证服务号，模板字段固定为 first / keyword1 / keyword2 / remark。
              接收人需在用户管理中维护微信 OpenID。
            </p>
            <button
              class="btn btn-primary btn-sm"
              type="button"
              style="margin-top: 14px"
              :disabled="channelSaving === 'wechat'"
              @click="saveChannel('wechat')"
            >
              <Settings2 :size="14" /> {{ channelSaving === 'wechat' ? '保存中…' : '保存公众号配置' }}
            </button>
          </section>
        </div>
      </el-tab-pane>

      <el-tab-pane label="推送记录" name="records">
        <div class="section-head">
          <div>
            <h2>最近推送记录</h2>
            <p>查看每次推送的通道、接收人、状态与错误信息，最多展示最近 100 条。</p>
          </div>
          <button class="btn btn-secondary btn-sm" type="button" @click="loadRecords">
            <RefreshCw :size="14" /> 刷新记录
          </button>
        </div>

        <div class="panel">
          <el-table :data="records" style="width: 100%">
            <el-table-column label="发送时间" width="145">
              <template #default="{ row }">{{ formatDate(row.sent_at, true) }}</template>
            </el-table-column>
            <el-table-column label="任务 / 看板" min-width="180">
              <template #default="{ row }">
                <div style="font-weight: 600">{{ dashboardMap[row.dashboard_id] || '—' }}</div>
                <div style="color: var(--muted-light); font-size: 10.5px">
                  任务 #{{ row.subscription_id || '—' }}
                </div>
              </template>
            </el-table-column>
            <el-table-column label="通道" width="110">
              <template #default="{ row }">
                <span class="tag">{{ channelLabel(row.channel) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="recipient" label="接收人" width="100" />
            <el-table-column label="状态" width="100">
              <template #default="{ row }">
                <span class="tag" :class="statusMeta(row.status).className">
                  {{ statusMeta(row.status).label }}
                </span>
              </template>
            </el-table-column>
            <el-table-column prop="title" label="标题" min-width="180" show-overflow-tooltip />
            <el-table-column label="错误信息" min-width="170" show-overflow-tooltip>
              <template #default="{ row }">
                <span style="color: var(--rose); font-size: 11px">{{ row.error || '—' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="90" fixed="right">
              <template #default="{ row }">
                <button
                  class="btn btn-ghost btn-sm"
                  type="button"
                  :disabled="row.status !== 'failed' || !row.subscription_id"
                  :title="row.subscription_id ? '重新执行该任务' : '记录未关联任务'"
                  @click="retryRecord(row)"
                >
                  <RefreshCw :size="13" /> 重试
                </button>
              </template>
            </el-table-column>
          </el-table>
          <div v-if="!records.length" class="empty-state" style="padding: 28px">
            暂无推送记录。创建任务后可点击「发送」进行测试。
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>

    <el-dialog
      v-model="taskVisible"
      :title="editingTaskId ? '编辑推送任务' : '新建推送任务'"
      width="720px"
      destroy-on-close
    >
      <div class="form-grid">
        <label class="field-label">
          任务名称
          <el-input v-model="taskForm.name" maxlength="100" placeholder="例如：每日销售数据卡片" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          推送看板
          <el-select v-model="taskForm.dashboard_id" style="width: 100%; margin-top: 8px" filterable>
            <el-option v-for="item in dashboards" :key="item.id" :value="item.id" :label="item.name" />
          </el-select>
        </label>
        <label class="field-label form-span-2">
          Cron 表达式
          <el-input v-model="taskForm.cron" placeholder="0 9 * * *" style="margin-top: 8px" />
          <div class="sql-hints" style="margin-top: 8px">
            <button
              v-for="preset in cronPresets"
              :key="preset.value"
              class="sql-hint"
              type="button"
              style="cursor: pointer"
              @click="taskForm.cron = preset.value"
            >
              {{ preset.label }}
            </button>
          </div>
        </label>
        <label class="field-label">
          时区
          <el-input v-model="taskForm.timezone" placeholder="Asia/Shanghai" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          推送模板
          <el-select v-model="taskForm.template_id" clearable style="width: 100%; margin-top: 8px" placeholder="按通道自动选择">
            <el-option
              v-for="item in templates"
              :key="item.id"
              :value="item.id"
              :label="`${item.name} · ${templateChannelLabel(item.channel)}`"
            />
          </el-select>
        </label>
        <div class="field-label form-span-2">
          推送通道
          <el-checkbox-group v-model="taskForm.channels" style="margin-top: 10px">
            <el-checkbox value="wecom">企业微信</el-checkbox>
            <el-checkbox value="wechat">微信公众号</el-checkbox>
          </el-checkbox-group>
        </div>
        <label class="field-label form-span-2">
          指定接收人
          <el-select
            v-model="taskForm.user_ids"
            multiple
            filterable
            clearable
            placeholder="不选则按角色和组织匹配"
            style="width: 100%; margin-top: 8px"
          >
            <el-option v-for="user in users" :key="user.id" :value="user.id" :label="`${user.name} · ${user.username}`" />
          </el-select>
        </label>
        <label class="field-label">
          按角色
          <el-select v-model="taskForm.roles" multiple clearable placeholder="例如：店长" style="width: 100%; margin-top: 8px">
            <el-option v-for="role in roles" :key="role.code" :value="role.code" :label="role.name" />
          </el-select>
        </label>
        <label class="field-label">
          按组织
          <OrgTreeSelect
            v-model="taskForm.org_ids"
            :data="orgTree"
            multiple
            placeholder="选择组织及其下级用户"
            style="margin-top: 8px"
          />
        </label>
        <div class="field-label form-span-2">
          任务状态
          <div class="scope-note" style="margin-top: 8px; width: 100%">
            <el-switch v-model="taskForm.enabled" />
            <span>{{ taskForm.enabled ? '启用，调度器会按 Cron 执行' : '停用，调度器将跳过该任务' }}</span>
          </div>
        </div>
      </div>
      <div class="scope-note" style="margin-top: 14px">
        <Users :size="14" />
        <span>接收规则取并集：指定用户 + 角色匹配 + 组织及下级用户。三类都不选时推送给全部启用用户。</span>
      </div>
      <template #footer>
        <div class="inline-actions" style="justify-content: flex-end">
          <button class="btn btn-secondary" type="button" @click="taskVisible = false">取消</button>
          <button class="btn btn-primary" type="button" :disabled="taskSaving" @click="saveTask">
            {{ taskSaving ? '保存中…' : '保存任务' }}
          </button>
        </div>
      </template>
    </el-dialog>

    <el-dialog
      v-model="templateVisible"
      :title="editingTemplateId ? '编辑推送模板' : '新建推送模板'"
      width="780px"
      destroy-on-close
    >
      <div class="form-grid">
        <label class="field-label">
          模板名称
          <el-input v-model="templateForm.name" maxlength="100" placeholder="例如：企业微信销售数据卡片" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          通道类型
          <el-select v-model="templateForm.channel" style="width: 100%; margin-top: 8px">
            <el-option value="wecom_textcard" label="企业微信文本卡片" />
            <el-option value="wecom_text" label="企业微信文本" />
            <el-option value="wechat" label="微信公众号模板" />
          </el-select>
        </label>
        <label class="field-label form-span-2">
          标题模板
          <el-input v-model="templateForm.title_template" maxlength="255" style="margin-top: 8px" />
        </label>
        <label class="field-label form-span-2">
          正文模板
          <el-input v-model="templateForm.content_template" type="textarea" :rows="7" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          按钮文字
          <el-input v-model="templateForm.button_text" maxlength="32" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          强调色
          <el-input v-model="templateForm.accent_color" maxlength="32" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          卡片背景色
          <el-input v-model="templateForm.bg_color" maxlength="32" style="margin-top: 8px" />
        </label>
        <div class="field-label">
          模板状态
          <div class="scope-note" style="margin-top: 8px; width: 100%">
            <el-switch v-model="templateForm.enabled" />
            <span>{{ templateForm.enabled ? '启用' : '停用' }}</span>
          </div>
        </div>
      </div>
      <div class="push-template-grid" style="margin-top: 16px; grid-template-columns: 1fr">
        <div class="wecom-preview" :style="{ background: templateForm.bg_color }">
          <h4>{{ friendly(templateForm.title_template) }}</h4>
          <p>{{ friendly(templateForm.content_template) }}</p>
          <span class="fake-btn" :style="{ background: templateForm.accent_color }">
            {{ templateForm.button_text || '查看数据' }}
          </span>
        </div>
      </div>
      <div class="sql-hints" style="margin-top: 12px">
        <span class="sql-hint" v-text="'{{.DashboardName}}'"></span>
        <span class="sql-hint" v-text="'{{.RecipientName}}'"></span>
        <span class="sql-hint" v-text="'{{.Period}}'"></span>
        <span class="sql-hint" v-text="'{{.Revenue}}'"></span>
        <span class="sql-hint" v-text="'{{.Orders}}'"></span>
        <span class="sql-hint" v-text="'{{.GrossProfit}}'"></span>
        <span class="sql-hint" v-text="'{{.Customers}}'"></span>
        <span class="sql-hint" v-text="'{{.URL}}'"></span>
      </div>
      <template #footer>
        <div class="inline-actions" style="justify-content: flex-end">
          <button class="btn btn-secondary" type="button" @click="templateVisible = false">取消</button>
          <button class="btn btn-primary" type="button" :disabled="templateSaving" @click="saveTemplate">
            {{ templateSaving ? '保存中…' : '保存模板' }}
          </button>
        </div>
      </template>
    </el-dialog>

    <el-dialog v-model="previewVisible" title="模板预览" width="560px">
      <div class="inline-actions" style="margin-bottom: 14px">
        <el-select v-model="previewDashboardId" size="small" style="width: 240px" @change="loadPreview">
          <el-option v-for="item in dashboards" :key="item.id" :value="item.id" :label="item.name" />
        </el-select>
        <button class="btn btn-secondary btn-sm" type="button" :disabled="previewLoading" @click="loadPreview">
          <RefreshCw :size="13" /> {{ previewLoading ? '渲染中…' : '刷新预览' }}
        </button>
      </div>
      <div v-if="previewResult" class="wecom-preview" :style="{ background: previewStyle.bg_color }">
        <h4>{{ previewResult.title }}</h4>
        <p>{{ previewResult.content }}</p>
        <span class="fake-btn" :style="{ background: previewStyle.accent_color }">
          {{ previewStyle.button_text }}
        </span>
      </div>
      <div v-else class="skeleton" style="height: 170px" />
      <p style="margin-top: 12px; color: var(--muted-light); font-size: 11px; line-height: 1.7">
        预览使用示例指标数据渲染，真实推送时会替换为接收人授权范围内的实时销售数据。
        卡片链接会自动生成带组织权限的分享令牌。
      </p>
    </el-dialog>
  </div>
</template>
