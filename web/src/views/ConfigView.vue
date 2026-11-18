<template>
  <div class="config-page">
    <div class="page-card">
      <div class="page-header">
        <div>
          <span class="page-title">系统配置</span>
          <div class="muted">保存后立即生效，无需重启服务；配置写入数据库，重启后仍保留</div>
        </div>
        <div class="flex-gap">
          <el-button :icon="RefreshLeft" :disabled="!dirtyCount" @click="resetAll">
            恢复默认（{{ dirtyCount }}）
          </el-button>
          <el-button type="primary" :icon="Check" :loading="saving" :disabled="!dirtyCount" @click="save">
            保存并应用
          </el-button>
        </div>
      </div>

      <el-alert
        v-if="!dirtyCount"
        type="info"
        :closable="false"
        show-icon
        title="所有配置均为当前生效值。修改任意项后点击「保存并应用」即时生效。"
        style="margin-bottom: 16px"
      />

      <el-tabs v-model="activeTab" tab-position="left" class="config-tabs">
        <el-tab-pane v-for="sec in sections" :key="sec.name" :label="sec.label" :name="sec.name">
          <div class="section-body">
            <div class="section-title">{{ sec.label }}配置</div>
            <el-form label-width="200px" label-position="left" class="config-form">
              <el-form-item v-for="f in sec.fields" :key="f.key" :label="f.label">
                <div class="field-row">
                  <template v-if="f.kind === 'bool'">
                    <el-switch v-model="form[f.key]" @change="markDirty(f.key)" />
                  </template>
                  <template v-else-if="f.options && f.options.length">
                    <el-select
                      v-model="form[f.key]"
                      filterable
                      style="width: 220px"
                      @change="markDirty(f.key)"
                    >
                      <el-option v-for="o in f.options" :key="o" :label="o" :value="o" />
                    </el-select>
                  </template>
                  <template v-else-if="f.kind === 'int'">
                    <el-input-number
                      v-model="numForm[f.key]"
                      :min="0"
                      :max="1000000"
                      controls-position="right"
                      style="width: 180px"
                      @change="onNumChange(f.key)"
                    />
                  </template>
                  <template v-else-if="f.secret">
                    <el-input
                      v-model="form[f.key]"
                      type="password"
                      show-password
                      style="width: 320px"
                      :placeholder="f.set ? '已设置，留空表示不修改' : '未设置'"
                      @input="markDirty(f.key)"
                    />
                  </template>
                  <template v-else>
                    <el-input
                      v-model="form[f.key]"
                      style="width: 320px"
                      @input="markDirty(f.key)"
                    />
                  </template>

                  <el-tag v-if="dirty[f.key]" size="small" type="warning" effect="plain">待保存</el-tag>
                  <el-tag v-else-if="f.set" size="small" type="success" effect="plain">已覆盖</el-tag>
                  <el-button
                    v-if="f.set"
                    link
                    type="primary"
                    size="small"
                    @click="resetOne(f.key)"
                  >
                    恢复默认
                  </el-button>
                </div>
                <div v-if="f.hint" class="muted field-hint">{{ f.hint }}</div>
                <div class="muted field-hint key-hint">{{ f.key }}</div>
              </el-form-item>
            </el-form>
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Check, RefreshLeft } from '@element-plus/icons-vue'
import { configApi, type ConfigField, type ConfigSection } from '@/api'

const sections = ref<ConfigSection[]>([])
const activeTab = ref('server')
const saving = ref(false)

const form = reactive<Record<string, any>>({})
const numForm = reactive<Record<string, number>>({})
const original = reactive<Record<string, string>>({})
const dirty = reactive<Record<string, boolean>>({})
const fieldMap = reactive<Record<string, ConfigField>>({})

const dirtyCount = computed(() => Object.values(dirty).filter(Boolean).length)

function markDirty(key: string) {
  dirty[key] = String(form[key] ?? '') !== String(original[key] ?? '')
}

function onNumChange(key: string) {
  form[key] = String(numForm[key] ?? 0)
  markDirty(key)
}

async function load() {
  const res = await configApi.list()
  sections.value = res.sections
  for (const sec of res.sections) {
    for (const f of sec.fields) {
      fieldMap[f.key] = f
      // Secrets come back masked; start from an empty input so "save" without
      // typing does not overwrite the stored value with asterisks.
      const initial = f.secret ? '' : f.value
      form[f.key] = f.kind === 'bool' ? f.value === 'true' : initial
      if (f.kind === 'int') numForm[f.key] = Number(f.value || 0)
      original[f.key] = f.kind === 'bool' ? f.value : initial
      dirty[f.key] = false
    }
  }
  if (res.sections.length && !res.sections.some((s) => s.name === activeTab.value)) {
    activeTab.value = res.sections[0].name
  }
}

function collectChanges(): Record<string, string> {
  const out: Record<string, string> = {}
  for (const key of Object.keys(dirty)) {
    if (!dirty[key]) continue
    const f = fieldMap[key]
    if (!f) continue
    if (f.kind === 'bool') {
      out[key] = form[key] ? 'true' : 'false'
    } else {
      const v = String(form[key] ?? '').trim()
      // Empty secret means "leave unchanged".
      if (f.secret && v === '') continue
      out[key] = v
    }
  }
  return out
}

async function save() {
  const values = collectChanges()
  if (!Object.keys(values).length) {
    ElMessage.info('没有修改')
    return
  }
  saving.value = true
  try {
    await configApi.update(values)
    ElMessage.success('配置已保存并热生效')
    await load()
  } finally {
    saving.value = false
  }
}

async function resetOne(key: string) {
  await ElMessageBox.confirm(`将「${fieldMap[key]?.label || key}」恢复为文件/默认值？`, '恢复默认', {
    type: 'warning',
  })
  await configApi.reset([key])
  ElMessage.success('已恢复默认')
  await load()
}

async function resetAll() {
  const keys = Object.keys(dirty).filter((k) => fieldMap[k]?.set)
  if (!keys.length) {
    // Nothing overridden — just discard unsaved edits.
    await load()
    ElMessage.info('已撤销未保存的修改')
    return
  }
  await ElMessageBox.confirm(`将恢复 ${keys.length} 项已覆盖的配置为文件/默认值？`, '恢复默认', {
    type: 'warning',
  })
  await configApi.reset(keys)
  ElMessage.success('已恢复默认')
  await load()
}

onMounted(load)
</script>

<style scoped>
.config-tabs {
  min-height: 420px;
}
.section-body {
  padding-left: 12px;
}
.section-title {
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 14px;
  color: var(--bi-text);
}
.config-form :deep(.el-form-item) {
  margin-bottom: 18px;
}
.field-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.field-hint {
  font-size: 11px;
  line-height: 1.6;
  width: 100%;
}
.key-hint {
  color: #c0c4cc;
  font-family: Consolas, monospace;
}
@media (max-width: 900px) {
  .config-tabs :deep(.el-tabs__header) {
    display: none;
  }
  .config-form :deep(.el-form-item__label) {
    width: auto !important;
    float: none;
  }
}
</style>
