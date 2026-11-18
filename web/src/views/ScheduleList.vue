<template>
  <div class="page-card">
    <div class="page-header">
      <span class="page-title">定时报表</span>
      <el-button type="primary" :icon="Plus" @click="openCreate">新建定时任务</el-button>
    </div>

    <el-alert
      v-if="!smtpEnabled"
      type="warning"
      show-icon
      :closable="false"
      title="SMTP 未配置：任务仍会生成报表文件，但不会发送邮件。可在配置文件中启用 SMTP。"
      style="margin-bottom: 12px"
    />

    <div class="table-scroll">
      <el-table :data="list" v-loading="loading" border stripe>
      <el-table-column prop="name" label="名称" min-width="150" />
      <el-table-column label="仪表盘" min-width="150">
        <template #default="{ row }">{{ dashboardName(row.dashboardId) }}</template>
      </el-table-column>
      <el-table-column label="渠道" width="120">
        <template #default="{ row }">
          <el-tag size="small" :type="row.deliveryChannel === 'wecom' ? 'primary' : 'info'" effect="plain">
            {{ row.deliveryChannel === 'wecom' ? (row.messageFormat === 'text' ? '企微文本' : '企微卡片') : '邮件' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Cron" width="150">
        <template #default="{ row }"><code>{{ row.cronExpr }}</code></template>
      </el-table-column>
      <el-table-column prop="timezone" label="时区" width="110" />
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <el-tag size="small" :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '停用' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="上次执行" width="160">
        <template #default="{ row }">
          <el-tag v-if="row.lastRunStatus === 'success'" size="small" type="success">成功</el-tag>
          <el-tag v-else-if="row.lastRunStatus === 'failed'" size="small" type="danger">失败</el-tag>
          <span v-else class="muted">未执行</span>
          <div class="muted" v-if="row.lastRunAt">{{ fmt(row.lastRunAt) }}</div>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="300" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" :loading="runningId === row.id" @click="runNow(row)">立即执行</el-button>
          <el-button link type="primary" @click="openRuns(row)">执行记录</el-button>
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="dialogVisible" :title="editing ? '编辑任务' : '新建任务'" :width="dialogWidth(560)" destroy-on-close>
      <el-form label-width="100px">
        <el-form-item label="名称">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="仪表盘">
          <el-select v-model="form.dashboardId" filterable style="width: 100%" :disabled="!!editing">
            <el-option v-for="d in dashboards" :key="d.id" :label="d.name" :value="d.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="Cron 表达式">
          <el-input v-model="form.cronExpr" placeholder="例如 0 0 9 * * *（每天 9:00，含秒）" />
          <div class="muted" style="margin-top: 4px">
            <span v-for="p in cronPresets" :key="p.label" class="preset" @click="form.cronExpr = p.value">{{ p.label }}</span>
          </div>
        </el-form-item>
        <el-form-item label="时区">
          <el-input v-model="form.timezone" placeholder="Local 或 Asia/Shanghai" />
        </el-form-item>
        <el-form-item label="推送渠道">
          <el-radio-group v-model="form.deliveryChannel">
            <el-radio-button value="email">邮件</el-radio-button>
            <el-radio-button value="wecom">企业微信</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.deliveryChannel === 'wecom'" label="消息形式">
          <el-radio-group v-model="form.messageFormat">
            <el-radio-button value="card">卡片消息</el-radio-button>
            <el-radio-button value="text">文本消息</el-radio-button>
          </el-radio-group>
          <div class="muted" style="width: 100%; margin-top: 4px">
            卡片包含关键指标与跳转按钮；文本为纯文字 + 链接。
          </div>
        </el-form-item>
        <el-form-item v-if="form.deliveryChannel === 'wecom'" label="链接有效期">
          <el-select v-model="form.linkExpireHours" style="width: 180px" filterable>
            <el-option label="1 小时" :value="1" />
            <el-option label="24 小时" :value="24" />
            <el-option label="72 小时（默认）" :value="72" />
            <el-option label="7 天" :value="168" />
            <el-option label="30 天" :value="720" />
          </el-select>
          <span class="muted" style="margin-left: 8px">每次推送生成新的临时分享链接</span>
        </el-form-item>
        <el-form-item :label="form.deliveryChannel === 'wecom' ? '企微账号' : '收件人'">
          <el-select
            v-model="form.recipients"
            multiple
            filterable
            allow-create
            default-first-option
            style="width: 100%"
            :placeholder="form.deliveryChannel === 'wecom' ? '输入企业微信 userid 后回车，如 boss' : '输入邮箱后回车'" collapse-tags collapse-tags-tooltip>
            <el-option v-if="form.deliveryChannel === 'wecom'" v-for="m in wecomMembers" :key="m.username" :label="`${m.displayName || m.username} (${m.username})`" :value="m.username" />
          </el-select>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="runsVisible" title="执行记录" :width="dialogWidth(820)">
      <div class="table-scroll">
        <el-table :data="runs" size="small" border max-height="480">
        <el-table-column label="触发" width="90">
          <template #default="{ row }">{{ row.triggeredBy === 'cron' ? '定时' : '手动' }}</template>
        </el-table-column>
        <el-table-column label="开始时间" width="170">
          <template #default="{ row }">{{ fmt(row.startedAt) }}</template>
        </el-table-column>
        <el-table-column label="耗时">
          <template #default="{ row }">{{ duration(row) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.status === 'success' ? 'success' : row.status === 'failed' ? 'danger' : 'warning'">
              {{ statusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="errorMessage" label="错误信息" min-width="180" show-overflow-tooltip />
        <el-table-column width="100">
          <template #default="{ row }">
            <el-button v-if="row.filePath" link type="primary" @click="downloadRun(row)">下载</el-button>
          </template>
        </el-table-column>
        </el-table>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { dialogWidth } from '@/utils/responsive'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { dashboardApi, scheduleApi, settingsApi, userApi } from '@/api'
import type { Dashboard, ScheduleRun, ScheduleTask } from '@/types'

const list = ref<ScheduleTask[]>([])
const dashboards = ref<Dashboard[]>([])
const loading = ref(false)
const smtpEnabled = ref(true)
const runningId = ref<number | null>(null)

const dialogVisible = ref(false)
const editing = ref<ScheduleTask | null>(null)
const saving = ref(false)
const form = reactive({
  name: '',
  dashboardId: undefined as number | undefined,
  cronExpr: '0 0 9 * * *',
  timezone: 'Local',
  enabled: true,
  deliveryChannel: 'email',
  messageFormat: 'card',
  linkExpireHours: 72,
  recipients: [] as string[],
})
const wecomMembers = ref<Array<{ username: string; displayName: string }>>([])

const runsVisible = ref(false)
const runs = ref<ScheduleRun[]>([])

const cronPresets = [
  { label: '每天 9:00', value: '0 0 9 * * *' },
  { label: '每周一 9:00', value: '0 0 9 * * 1' },
  { label: '每小时', value: '0 0 * * * *' },
  { label: '每分钟(测试)', value: '0 * * * * *' },
]

function fmt(t?: string) {
  return t ? dayjs(t).format('YYYY-MM-DD HH:mm:ss') : ''
}
function duration(row: ScheduleRun) {
  if (!row.finishedAt) return '-'
  return `${dayjs(row.finishedAt).diff(dayjs(row.startedAt), 'second')}s`
}
function statusLabel(s: string) {
  return { running: '执行中', success: '成功', failed: '失败' }[s] || s
}
function dashboardName(id: number) {
  return dashboards.value.find((d) => d.id === id)?.name || `#${id}`
}

async function load() {
  loading.value = true
  try {
    const [tasks, dash] = await Promise.all([scheduleApi.list(), dashboardApi.list({ page: 1, pageSize: 200 })])
    list.value = tasks
    dashboards.value = dash.items
    try {
      const s = await settingsApi.get()
      smtpEnabled.value = s.smtpEnabled
    } catch {}
    try {
      const res = await userApi.list({ page: 1, pageSize: 200 })
      wecomMembers.value = res.items
        .filter((u: any) => u.wecomUserId || u.source === 'wecom')
        .map((u: any) => ({ username: u.username, displayName: u.displayName }))
    } catch {
      wecomMembers.value = []
    }
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  Object.assign(form, {
    name: '', dashboardId: dashboards.value[0]?.id, cronExpr: '0 0 9 * * *',
    timezone: 'Local', enabled: true, deliveryChannel: 'email',
    messageFormat: 'card', linkExpireHours: 72, recipients: [],
  })
  dialogVisible.value = true
}

function openEdit(t: ScheduleTask) {
  editing.value = t
  let recipients: string[] = []
  try {
    recipients = JSON.parse(t.recipientsJson || '[]')
  } catch {}
  Object.assign(form, {
    name: t.name,
    dashboardId: t.dashboardId,
    cronExpr: t.cronExpr,
    timezone: t.timezone,
    enabled: t.enabled,
    deliveryChannel: t.deliveryChannel || 'email',
    messageFormat: t.messageFormat || 'card',
    linkExpireHours: t.linkExpireHours || 72,
    recipients,
  })
  dialogVisible.value = true
}

async function save() {
  if (!form.name || !form.dashboardId || !form.cronExpr) {
    ElMessage.warning('请填写名称、仪表盘和 Cron 表达式')
    return
  }
  saving.value = true
  try {
    const payload = {
      name: form.name,
      dashboardId: form.dashboardId,
      cronExpr: form.cronExpr,
      timezone: form.timezone,
      enabled: form.enabled,
      deliveryChannel: form.deliveryChannel,
      messageFormat: form.messageFormat,
      linkExpireHours: form.linkExpireHours,
      recipients: form.recipients,
    }
    if (editing.value) {
      await scheduleApi.update(editing.value.id, payload)
    } else {
      await scheduleApi.create(payload)
    }
    ElMessage.success('已保存')
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

async function runNow(t: ScheduleTask) {
  runningId.value = t.id
  try {
    const run = await scheduleApi.run(t.id)
    if (run.status === 'success') {
      ElMessage.success('执行成功，报表已生成')
    } else {
      ElMessage.error(run.errorMessage || '执行失败')
    }
    await load()
  } finally {
    runningId.value = null
  }
}

async function openRuns(t: ScheduleTask) {
  runs.value = await scheduleApi.runs(t.id)
  runsVisible.value = true
}

async function downloadRun(run: ScheduleRun) {
  await scheduleApi.download(run.id)
}

async function remove(t: ScheduleTask) {
  await ElMessageBox.confirm(`确定删除定时任务「${t.name}」吗？`, '删除确认', { type: 'warning' })
  await scheduleApi.remove(t.id)
  ElMessage.success('已删除')
  await load()
}

onMounted(load)
</script>

<style scoped>
code {
  background: #f4f4f5;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
}
.preset {
  display: inline-block;
  margin-right: 10px;
  color: #409eff;
  cursor: pointer;
}
</style>
