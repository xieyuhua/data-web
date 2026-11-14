// 按需引入 ECharts 模块，避免把完整图表库打进主包。
// 新增图表类型时，在这里注册对应的 charts / components 即可。
import * as echarts from 'echarts/core'
import { BarChart, GaugeChart, LineChart, PieChart, ScatterChart } from 'echarts/charts'
import {
  GraphicComponent,
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TitleComponent,
  TooltipComponent,
} from 'echarts/components'
import { LabelLayout, UniversalTransition } from 'echarts/features'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([
  BarChart,
  GaugeChart,
  LineChart,
  PieChart,
  ScatterChart,
  GraphicComponent,
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TitleComponent,
  TooltipComponent,
  LabelLayout,
  UniversalTransition,
  CanvasRenderer,
])

export default echarts
