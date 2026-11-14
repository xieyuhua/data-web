<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowUpRight, Layers, Plus, Sparkles } from 'lucide-vue-next'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, getErrorMessage } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { formatDate } from '../utils/format'
import EmptyBlock from '../components/EmptyBlock.vue'

const auth = useAuthStore()
const router = useRouter()

const templates = ref([])
const datasets = ref([])
const loading = ref(true)
const category = ref('全部')
const usingId = ref(0)
const dialogVisible = ref(false)
const previewVisible = ref(false)
const activeTemplate = ref(null)
const saving = ref(false)

const form = reactive({
  name: '',
  description: '',
  category: '经营分析',
  cover: 'emerald',
})

const categories = computed(() => {
  const set = new Set(templates.value.map((item) => item.category).filter(Boolean))
  return ['全部', ...Array.from(set)]
})

const filtered = computed(() =>
  templates.value.filter((item) => category.value === '全部' || item.category === category.value),
)

const isAdmin = computed(() => auth.isAdmin)

const coverBars = [
  [32, 54, 44, 72, 58, 86],
  [48, 38, 62, 50, 78, 66],
  [40, 66, 52, 80, 60, 92],
]

function widgetCount(template) {
  return template.layout?.widgets?.length || 0
}

async function load() {
  loading.value = true
  try {
    const [templateList, datasetList] = await Promise.all([
      api.get('/templates'),
      api.get('/datasets').catch(() => []),
    ])
    templates.value = templateList || []
    datasets.value = datasetList || []
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

async function useTemplate(template) {
  if (usingId.value) return
  usingId.value = template.id
  try {
    const dashboard = await api.post(`/templates/${template.id}/use`)
    ElMessage.success('模板已生成新看板')
    router.push(`/dashboards/${dashboard.id}/edit`)
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    usingId.value = 0
  }
}

function openPreview(template) {
  activeTemplate.value = template
  previewVisible.value = true
}

async function removeTemplate(template) {
  try {
    await ElMessageBox.confirm(`确定删除模板「${template.name}」？`, '删除模板', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    await api.delete(`/templates/${template.id}`)
    ElMessage.success('模板已删除')
    await load()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  }
}

function buildLayout() {
  const summary = datasets.value.find((item) => item.code === 'sales_summary')
  const trend = datasets.value.find((item) => item.code === 'sales_trend')
  const region = datasets.value.find((item) => item.code === 'sales_region')
  const channel = datasets.value.find((item) => item.code === 'sales_channel')
  const widgets = []
  if (summary) {
    widgets.push({
      id: 'kpi', title: '核心经营指标', subtitle: '当前权限范围 · 实时汇总',
      chart_type: 'kpi', dataset_id: summary.id, x: 0, y: 0, w: 12, h: 2,
      config: {
        columns: [
          { field: 'revenue', label: '销售额', unit: '元', format: 'currency' },
          { field: 'orders', label: '订单量', unit: '笔', format: 'integer' },
          { field: 'customers', label: '客流', unit: '人', format: 'integer' },
          { field: 'gross_margin', label: '毛利率', unit: '%', format: 'number' },
        ],
      },
    })
  }
  if (trend) {
    widgets.push({
      id: 'trend', title: '销售趋势', subtitle: '销售额与毛利走势',
      chart_type: 'line', dataset_id: trend.id, x: 0, y: 2, w: 8, h: 6,
      config: {
        xField: 'date',
        yFields: [
          { field: 'revenue', name: '销售额', color: '#0F766E' },
          { field: 'profit', name: '毛利', color: '#E8A13A' },
        ],
        unit: '元',
      },
    })
  }
  if (region) {
    widgets.push({
      id: 'region', title: '区域销售排行', subtitle: '按区域销售贡献',
      chart_type: 'bar', dataset_id: region.id, x: 8, y: 2, w: 4, h: 6,
      config: { xField: 'region', yField: 'revenue', unit: '元', color: '#14B8A6' },
    })
  }
  if (channel) {
    widgets.push({
      id: 'channel', title: '渠道销售构成', subtitle: '全渠道销售占比',
      chart_type: 'donut', dataset_id: channel.id, x: 4, y: 8, w: 8, h: 6,
      config: { xField: 'channel', yField: 'revenue', unit: '元' },
    })
  }
  return { settings: { refresh_seconds: 0, show_title: true }, widgets }
}

function openCreate() {
  form.name = ''
  form.description = ''
  form.category = '经营分析'
  form.cover = 'emerald'
  dialogVisible.value = true
}

async function createTemplate() {
  if (!form.name.trim()) {
    ElMessage.warning('请填写模板名称')
    return
  }
  saving.value = true
  try {
    await api.post('/templates', {
      name: form.name.trim(),
      description: form.description,
      category: form.category || '经营分析',
      theme: 'light',
      cover: form.cover,
      layout: buildLayout(),
    })
    ElMessage.success('模板已创建')
    dialogVisible.value = false
    await load()
  } catch (error) {
    ElMessage.error(getErrorMessage(error))
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="section-head">
      <div>
        <h2>分析模板</h2>
        <p>沉淀成熟的分析框架，按角色数据范围一键生成新看板</p>
      </div>
      <div class="inline-actions">
        <button v-if="isAdmin" class="btn btn-primary btn-sm" type="button" @click="openCreate">
          <Plus :size="14" /> 新建模板
        </button>
      </div>
    </div>

    <div class="filter-pills" style="width: max-content; max-width: 100%; margin-bottom: 18px; overflow-x: auto">
      <button
        v-for="item in categories"
        :key="item"
        class="filter-pill"
        :class="{ active: category === item }"
        type="button"
        @click="category = item"
      >
        {{ item }}
      </button>
    </div>

    <div v-if="loading" class="template-grid">
      <div v-for="index in 3" :key="index" class="panel" style="height: 330px; padding: 16px">
        <div class="skeleton" style="height: 100%" />
      </div>
    </div>

    <div v-else-if="filtered.length" class="template-grid">
      <article v-for="(template, index) in filtered" :key="template.id" class="template-card">
        <div class="template-cover" :class="template.cover">
          <div class="cover-window">
            <div class="cover-window-top"><i /><i /><i /></div>
            <div class="cover-lines"><i /><i /><i /></div>
            <div class="cover-bars">
              <i
                v-for="(height, barIndex) in coverBars[index % coverBars.length]"
                :key="barIndex"
                :style="{ '--h': `${height}%` }"
              />
            </div>
          </div>
        </div>
        <div class="template-body">
          <div class="inline-actions" style="justify-content: space-between; margin-bottom: 7px">
            <span class="tag tag-green">{{ template.category || '经营分析' }}</span>
            <span v-if="template.built_in" class="tag">内置</span>
          </div>
          <h3>{{ template.name }}</h3>
          <p>{{ template.description || '基于成熟经营分析框架的看板模板。' }}</p>
          <div class="template-foot">
            <span><Layers :size="11" style="vertical-align: -1px" /> {{ widgetCount(template) }} 个组件 · {{ formatDate(template.updated_at) }}</span>
            <div class="inline-actions">
              <button class="btn btn-ghost btn-sm" type="button" @click="openPreview(template)">查看结构</button>
              <button
                v-if="isAdmin && !template.built_in"
                class="btn btn-ghost btn-sm"
                type="button"
                @click="removeTemplate(template)"
              >
                删除
              </button>
              <button class="btn btn-primary btn-sm" type="button" :disabled="usingId === template.id" @click="useTemplate(template)">
                <Sparkles :size="12" /> {{ usingId === template.id ? '生成中…' : '使用模板' }}
              </button>
            </div>
          </div>
        </div>
      </article>
    </div>

    <div v-else class="panel">
      <EmptyBlock title="暂无模板" description="管理员可以创建模板，沉淀常用分析框架。">
        <button v-if="isAdmin" class="btn btn-primary btn-sm" type="button" @click="openCreate">
          <Plus :size="14" /> 新建模板
        </button>
      </EmptyBlock>
    </div>

    <div class="security-grid">
      <div class="security-card">
        <div class="security-icon"><Layers :size="16" /></div>
        <h3>结构可复用</h3>
        <p>模板只保存组件结构与字段配置，生成看板后再绑定当前可用的数据集。</p>
      </div>
      <div class="security-card">
        <div class="security-icon"><Sparkles :size="16" /></div>
        <h3>开箱即用</h3>
        <p>指标卡、趋势、区域排行和渠道构成一次带入，减少重复搭建成本。</p>
      </div>
      <div class="security-card">
        <div class="security-icon"><ArrowUpRight :size="16" /></div>
        <h3>权限不变</h3>
        <p>从模板生成的看板仍按使用者的组织范围查询，不会扩大数据可见性。</p>
      </div>
    </div>

    <el-dialog v-model="previewVisible" title="模板结构" width="560px">
      <template v-if="activeTemplate">
        <div class="inline-actions" style="justify-content: space-between; margin-bottom: 14px">
          <div>
            <strong style="font-size: 13px">{{ activeTemplate.name }}</strong>
            <p style="margin-top: 4px; color: var(--muted); font-size: 9px">{{ activeTemplate.description }}</p>
          </div>
          <span class="tag tag-green">{{ activeTemplate.category }}</span>
        </div>
        <div v-for="widget in activeTemplate.layout?.widgets || []" :key="widget.id" class="widget-list-item" style="cursor: default">
          <span class="tag">{{ widget.chart_type }}</span>
          <span>{{ widget.title }}</span>
          <small style="color: var(--muted-light)">{{ widget.w }}/12 · 数据集 {{ widget.dataset_id }}</small>
        </div>
        <p v-if="!activeTemplate.layout?.widgets?.length" style="color: var(--muted); font-size: 10px">该模板暂无组件。</p>
      </template>
    </el-dialog>

    <el-dialog v-model="dialogVisible" title="新建模板" width="540px">
      <div class="form-grid">
        <label class="field-label form-span-2">
          模板名称
          <el-input v-model="form.name" placeholder="例如：大区经营周报" style="margin-top: 8px" />
        </label>
        <label class="field-label">
          分类
          <el-select v-model="form.category" style="width: 100%; margin-top: 8px" filterable allow-create>
            <el-option label="经营分析" value="经营分析" />
            <el-option label="销售分析" value="销售分析" />
            <el-option label="区域管理" value="区域管理" />
          </el-select>
        </label>
        <label class="field-label">
          封面色
          <el-select v-model="form.cover" style="width: 100%; margin-top: 8px">
            <el-option label="墨绿" value="emerald" />
            <el-option label="琥珀" value="amber" />
            <el-option label="深蓝" value="blue" />
          </el-select>
        </label>
        <label class="field-label form-span-2">
          模板说明
          <el-input v-model="form.description" type="textarea" :rows="3" style="margin-top: 8px" placeholder="说明适用角色与分析目标" />
        </label>
      </div>
      <p style="margin-top: 10px; color: var(--muted-light); font-size: 9px">
        新模板会基于当前演示数据集自动生成指标卡、趋势、区域排行与渠道构成组件。
      </p>
      <template #footer>
        <div class="inline-actions" style="justify-content: flex-end">
          <button class="btn btn-secondary" type="button" @click="dialogVisible = false">取消</button>
          <button class="btn btn-primary" type="button" :disabled="saving" @click="createTemplate">
            {{ saving ? '保存中…' : '创建模板' }}
          </button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>
