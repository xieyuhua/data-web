import { reactive, ref } from 'vue'
import type { ChartConfig, DashboardData, FilterOption, ResultSet } from '@/types'

export interface DashboardRuntimeWidget {
  id: number
  widgetType: string
  chartId?: number
  title: string
  chartType?: string
  chartConfig?: ChartConfig | null
}

export interface DashboardRuntimeFilter {
  id: number
  name: string
  label: string
  filterType: string
  hasOptions?: boolean
  datasetId?: number
  defaultValueJson?: string
}

export interface DataLoader {
  loadData(filterValues: Record<string, any>): Promise<DashboardData>
  loadFilterOptions(filterId: number): Promise<FilterOption[]>
}

/** Shared runtime state used by the viewer and the editor preview. */
export function useDashboardRuntime(loader: DataLoader) {
  const results = reactive<Record<number, { result?: ResultSet; error?: string }>>({})
  const options = reactive<Record<number, FilterOption[]>>({})
  const loading = ref(false)
  const elapsedMs = ref(0)

  async function refresh(filters: DashboardRuntimeFilter[], values: Record<string, any>) {
    loading.value = true
    try {
      const payload = await loader.loadData(values)
      // The response wraps widget results in `widgets`; iterating the payload
      // itself would key results by "widgets"/"elapsedMs" and render nothing.
      const widgetResults = payload?.widgets || {}
      for (const k of Object.keys(results)) delete results[k as any]
      for (const [widgetId, wr] of Object.entries(widgetResults)) {
        results[Number(widgetId)] = { result: wr?.result, error: wr?.error }
      }
      elapsedMs.value = payload?.elapsedMs || 0
      await loadSelectOptions(filters)
    } catch (e) {
      // error surfaced by http interceptor
    } finally {
      loading.value = false
    }
  }

  async function loadSelectOptions(filters: DashboardRuntimeFilter[]) {
    for (const f of filters) {
      if (f.filterType !== 'select' || !f.hasOptions || options[f.id]) continue
      try {
        options[f.id] = await loader.loadFilterOptions(f.id)
      } catch {
        options[f.id] = []
      }
    }
  }

  return { results, options, loading, elapsedMs, refresh }
}
