<template>
  <div class="settings-page">
    <div class="page-card section">
      <div class="section-head">
        <div>
          <div class="section-title">平台概览</div>
          <div class="muted">当前运行配置与能力开关</div>
        </div>
        <el-tag size="small" type="success" effect="plain">运行中</el-tag>
      </div>
      <div class="stat-grid">
        <div v-for="s in overview" :key="s.label" class="stat">
          <div class="stat-label">{{ s.label }}</div>
          <div class="stat-value" :class="{ 'stat-value--muted': s.muted }">{{ s.value }}</div>
        </div>
      </div>
    </div>

    <div class="page-card section">
      <div class="section-head">
        <div>
          <div class="section-title">企业微信集成</div>
          <div class="muted">扫码登录与消息推送</div>
        </div>
        <el-tag size="small" :type="wecom.enabled ? 'success' : 'info'" effect="plain">
          {{ wecom.enabled ? '已启用' : '未启用' }}
        </el-tag>
      </div>

      <el-empty v-if="!wecom.enabled" description="未启用企业微信集成" :image-size="70">
        <div class="config-hint">
          在 <code>config.yaml</code> 中配置 <code>wecom.enabled</code>、<code>wecom.corpId</code>、
          <code>wecom.agentId</code>、<code>wecom.secret</code>、<code>wecom.redirectUri</code> 后重启服务。
        </div>
      </el-empty>

      <template v-else>
        <el-descriptions :column="descColumns" border class="wecom-desc">
          <el-descriptions-item label="企业 ID">{{ wecom.corpId || '-' }}</el-descriptions-item>
          <el-descriptions-item label="AgentId">{{ wecom.agentId || '-' }}</el-descriptions-item>
          <el-descriptions-item label="通讯录同步">
            {{ wecom.lastSyncAt ? fmt(wecom.lastSyncAt) : '从未同步' }}
            <el-tag v-if="wecom.lastStatus === 'success'" type="success" size="small" style="margin-left: 6px">成功</el-tag>
            <el-tag v-else-if="wecom.lastStatus === 'failed'" type="danger" size="small" style="margin-left: 6px">失败</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="推送能力">应用消息（卡片 / 文本）</el-descriptions-item>
        </el-descriptions>

        <div class="flex-gap actions">
          <el-button type="primary" :icon="OfficeBuilding" @click="router.push('/org')">前往组织架构</el-button>
          <span class="muted">部门与成员管理、通讯录同步已移至「组织架构」菜单。</span>
        </div>
      </template>
    </div>

    <div class="page-card section">
      <div class="section-head">
        <div>
          <div class="section-title">快捷入口</div>
          <div class="muted">常用管理操作</div>
        </div>
      </div>
      <div class="shortcut-grid">
        <div v-for="s in shortcuts" :key="s.path" class="shortcut" @click="router.push(s.path)">
          <el-icon :size="22" class="shortcut-icon"><component :is="s.icon" /></el-icon>
          <div>
            <div class="shortcut-title">{{ s.title }}</div>
            <div class="muted">{{ s.desc }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Coin, Document, OfficeBuilding, Tickets, User } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { authApi, settingsApi } from '@/api'

const router = useRouter()

const settings = reactive<{
  version?: string
  smtpEnabled?: boolean
  schedulerEnabled?: boolean
  queryTimeoutSec?: number
  maxResultRows?: number
}>({})
const wecom = reactive<any>({ enabled: false })
const departments = ref<Array<{ wecomId: number; name: string; parentWecomId: number; depth: number }>>([])
const isNarrow = ref(false)

function syncViewport() {
  isNarrow.value = window.innerWidth < 900
}
onMounted(() => {
  syncViewport()
  window.addEventListener('resize', syncViewport)
})
onBeforeUnmount(() => window.removeEventListener('resize', syncViewport))

const descColumns = computed(() => (isNarrow.value ? 1 : 2))

const overview = computed(() => [
  { label: '版本', value: settings.version || '-', muted: false },
  { label: '定时报表', value: settings.schedulerEnabled ? '已启用' : '未启用', muted: !settings.schedulerEnabled },
  { label: '邮件通知', value: settings.smtpEnabled ? '已配置' : '未配置', muted: !settings.smtpEnabled },
  { label: '查询超时', value: settings.queryTimeoutSec ? `${settings.queryTimeoutSec} 秒` : '-', muted: false },
  { label: '结果行上限', value: settings.maxResultRows ? settings.maxResultRows.toLocaleString() : '-', muted: false },
])

const shortcuts = [
  { path: '/datasources', title: '数据源', desc: '管理数据库连接', icon: Coin },
  { path: '/datasets', title: '数据集', desc: '编写与预览 SQL', icon: Document },
  { path: '/users', title: '用户管理', desc: '账号与角色', icon: User },
  { path: '/audit-logs', title: '审计日志', desc: '操作留痕', icon: Tickets },
]


function fmt(t: string) {
  return dayjs(t).format('YYYY-MM-DD HH:mm')
}

async function load() {
  try {
    Object.assign(settings, await settingsApi.get())
  } catch {}
  try {
    Object.assign(wecom, await authApi.wecomStatus())
  } catch {}
  if (wecom.enabled) {
  }
}


onMounted(load)
</script>

<style scoped>
.settings-page {
  display: flex;
  flex-direction: column;
  gap: var(--bi-gap);
}
.section {
  padding: 18px 20px;
}
.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.section-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--bi-text);
}

.stat-grid {
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
}
.stat {
  background: #f8fafc;
  border: 1px solid #eef2f7;
  border-radius: 8px;
  padding: 12px 14px;
}
.stat-label {
  font-size: 12px;
  color: var(--bi-muted);
}
.stat-value {
  margin-top: 6px;
  font-size: 18px;
  font-weight: 600;
  color: var(--bi-text);
  word-break: break-word;
}
.stat-value--muted {
  color: #a8abb2;
  font-size: 15px;
}

.config-hint {
  max-width: 560px;
  font-size: 12px;
  line-height: 1.8;
  color: #909399;
  text-align: center;
}
code {
  background: #f4f4f5;
  border-radius: 3px;
  padding: 1px 5px;
  font-size: 11px;
}

.wecom-desc {
  margin-bottom: 16px;
}
.wecom-desc :deep(.el-descriptions__content) {
  word-break: break-all;
}
.actions {
  margin-bottom: 20px;
}

.shortcut {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px;
  border: 1px solid #eef2f7;
  border-radius: 8px;
  cursor: pointer;
  transition: border-color 0.15s, box-shadow 0.15s, transform 0.15s;
}
.shortcut:hover {
  border-color: #c6e2ff;
  box-shadow: 0 4px 14px rgba(64, 158, 255, 0.12);
  transform: translateY(-1px);
}
.shortcut-icon {
  color: #409eff;
  flex-shrink: 0;
}
.shortcut-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}

@media (max-width: 640px) {
  .section {
    padding: 14px;
  }
  .stat-value {
    font-size: 16px;
  }
}
</style>
