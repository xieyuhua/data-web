<template>
  <div class="share-page" v-loading="loading">
    <div class="share-header">
      <div class="brand">
        <span class="logo-mark">B</span>
        <span class="brand-name">Biaoge BI</span>
      </div>
      <span class="share-title">{{ view?.name || '' }}</span>
      <el-button :icon="Refresh" circle size="small" @click="refreshData" />
    </div>

    <div v-if="error" class="share-error">
      <el-result icon="error" title="无法访问该分享" :sub-title="error" />
    </div>

    <template v-else>
      <FilterBar v-if="filters.length" :filters="filters" :options="options" @change="onFilterChange" />
      <el-empty v-if="!widgets.length" description="该仪表盘暂无内容" />
      <GridCanvas v-else :layout="layout" :editable="false">
        <template #default="{ item }">
          <WidgetFrame :widget="item.widget">
            <ChartRenderer
              v-if="item.widget.widgetType === 'chart'"
              :type="item.widget.chartType || 'table'"
              :result="results[item.widget.id]?.result"
              :config="item.widget.chartConfig"
            />
            <div v-else class="text-widget">{{ item.widget.text }}</div>
            <div v-if="results[item.widget.id]?.error" class="widget-error">
              {{ results[item.widget.id]?.error }}
            </div>
          </WidgetFrame>
        </template>
      </GridCanvas>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Refresh } from '@element-plus/icons-vue'
import { publicApi } from '@/api'
import type { ChartConfig, PublicDashboard } from '@/types'
import { useDashboardRuntime } from '@/composables/useDashboardData'
import FilterBar from '@/components/dashboard/FilterBar.vue'
import GridCanvas from '@/components/dashboard/GridCanvas.vue'
import WidgetFrame from '@/components/dashboard/WidgetFrame.vue'
import ChartRenderer from '@/components/charts/ChartRenderer.vue'

const route = useRoute()
const token = String(route.params.token)

const view = ref<PublicDashboard | null>(null)
const error = ref('')
const loading = ref(true)
const lastValues = ref<Record<string, any>>({})

const { results, options, loading: dataLoading, refresh } = useDashboardRuntime({
  loadData: (v) => publicApi.data(token, v) as any,
  loadFilterOptions: (fid) => publicApi.filterOptions(token, fid),
})

const widgets = computed(() => view.value?.widgets || [])
const filters = computed(() => view.value?.filters || [])

const layout = computed(() =>
  widgets.value.map((w) => {
    let config: ChartConfig | null = null
    try {
      config = w.chartConfig ? JSON.parse(w.chartConfig) : null
    } catch {}
    let text = ''
    try {
      text = JSON.parse(w.chartConfig || '{}').text || ''
    } catch {}
    return {
      i: String(w.id),
      x: w.x,
      y: w.y,
      w: w.w,
      h: w.h,
      // Keep chartType from the public payload; without it every chart falls
      // back to a table renderer.
      widget: { ...w, chartType: w.chartType || 'table', chartConfig: config, text },
    }
  }),
)

async function refreshData() {
  if (!view.value) return
  await refresh(filters.value as any, { ...lastValues.value })
}

async function onFilterChange(values: Record<string, any>) {
  lastValues.value = { ...values }
  await refreshData()
}

onMounted(async () => {
  try {
    view.value = await publicApi.view(token)
    // Seed filter defaults so the first request carries the same values the
    // FilterBar will emit on mount.
    for (const f of filters.value) {
      if (!f.defaultValueJson) continue
      try {
        lastValues.value[f.name] = JSON.parse(f.defaultValueJson)
      } catch {}
    }
    await refreshData()
  } catch (e: any) {
    error.value = e?.response?.data?.message || '分享链接不存在或已过期'
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.share-page {
  min-height: 100vh;
  padding: 16px 20px;
  background: #f5f7fa;
}
.share-header {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 14px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 8px;
}
.logo-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 7px;
  background: linear-gradient(135deg, #409eff, #6f42c1);
  color: #fff;
  font-weight: 700;
}
.brand-name {
  font-weight: 600;
  color: #303133;
}
.share-title {
  flex: 1;
  font-size: 17px;
  font-weight: 600;
  color: #1f2d3d;
}
.share-error {
  background: #fff;
  border-radius: 8px;
  padding: 60px 0;
}
.text-widget {
  white-space: pre-wrap;
  color: #303133;
  font-size: 14px;
  line-height: 1.7;
  height: 100%;
  overflow: auto;
}
.widget-error {
  color: #f56c6c;
  font-size: 12px;
  padding: 4px 6px;
}
</style>
