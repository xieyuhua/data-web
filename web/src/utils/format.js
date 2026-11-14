// 数字 / 日期 / 状态 的统一格式化工具：
// 指标数字使用 compactValue 支持“点击在原始值与 万/亿 缩写之间切换”
// 任意值安全转成数字，空值与非法值统一返回 0
export function toNumber(value) {
  if (value === null || value === undefined || value === '') return 0
  const number = Number(value)
  return Number.isFinite(number) ? number : 0
}

// 按指定格式渲染指标值：number 普通数值、currency 金额、integer 整数、percent 百分比
export function formatValue(value, format = 'number', unit = '') {
  if (value === null || value === undefined || value === '') return '—'
  // 非数字字符串（如维度名称）原样展示，不做数值转换
  if (typeof value === 'string' && Number.isNaN(Number(value))) return value
  const number = toNumber(value)
  let text = ''
  switch (format) {
    case 'currency':
      // 金额：千分位，最多保留 2 位小数
      text = number.toLocaleString('zh-CN', { minimumFractionDigits: 0, maximumFractionDigits: 2 })
      break
    case 'integer':
      // 整数：四舍五入后千分位展示
      text = Math.round(number).toLocaleString('zh-CN')
      break
    case 'percent':
      // 百分比：数值本身已是百分数，直接追加 % 号
      text = `${number.toLocaleString('zh-CN', { maximumFractionDigits: 1 })}%`
      return text
    case 'number':
    default:
      text = number.toLocaleString('zh-CN', { maximumFractionDigits: 1 })
  }
  return unit ? `${text} ${unit}` : text
}

// 固定规则的 万 / 亿 缩写，供图表轴标签等场景使用
export function compactNumber(value) {
  const number = toNumber(value)
  if (Math.abs(number) >= 100000000) return `${(number / 100000000).toFixed(2)}亿`
  if (Math.abs(number) >= 10000) return `${(number / 10000).toFixed(1)}万`
  return number.toLocaleString('zh-CN', { maximumFractionDigits: 0 })
}

// 去掉小数末尾多余的 0，例如 1.00 -> 1、1.20 -> 1.2
function trimTrailingZero(text) {
  return text.includes('.') ? text.replace(/0+$/, '').replace(/\.$/, '') : text
}

// 数字缩写，用于“点击切换原始值”场景：10000 -> 1万、1250000 -> 125万、320000000 -> 3.2亿
export function compactValue(value, unit = '') {
  const number = toNumber(value)
  const abs = Math.abs(number)
  let text
  if (abs >= 100000000) {
    text = `${trimTrailingZero((number / 100000000).toFixed(2))}亿`
  } else if (abs >= 10000) {
    text = `${trimTrailingZero((number / 10000).toFixed(2))}万`
  } else {
    text = number.toLocaleString('zh-CN', { maximumFractionDigits: 2 })
  }
  return unit ? `${text}${unit}` : text
}

export function formatDate(value, withTime = false) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return String(value)
  const options = {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    ...(withTime ? { hour: '2-digit', minute: '2-digit' } : {}),
  }
  return new Intl.DateTimeFormat('zh-CN', options).format(date)
}

export function todayISO(offsetDays = 0) {
  const date = new Date()
  date.setDate(date.getDate() + offsetDays)
  return toISODate(date)
}

// 把 Date 对象格式化为 YYYY-MM-DD，供日期选择器与接口参数使用
function toISODate(date) {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

// 生成最近 N 天的日期区间 [开始日, 今天]，days = 0 表示仅当天
export function dateRange(days) {
  if (days === 0) return [todayISO(), todayISO()]
  return [todayISO(-(days - 1)), todayISO()]
}

// offset = 0 本月，-1 上月。
export function monthRange(offset = 0) {
  const now = new Date()
  const start = new Date(now.getFullYear(), now.getMonth() + offset, 1)
  const end = new Date(now.getFullYear(), now.getMonth() + offset + 1, 0)
  return [toISODate(start), toISODate(end)]
}

// 今年 1 月 1 日至今，用于“本年累计”等场景
export function yearToDateRange() {
  const now = new Date()
  return [`${now.getFullYear()}-01-01`, toISODate(now)]
}

// 状态码 => 中文文案与标签样式类名，列表页统一渲染
export function statusMeta(status) {
  const map = {
    active: { label: '启用', className: 'tag-green' },
    published: { label: '已发布', className: 'tag-green' },
    draft: { label: '草稿', className: 'tag-amber' },
    disabled: { label: '已停用', className: 'tag-red' },
    sent: { label: '已发送', className: 'tag-green' },
    simulated: { label: '模拟发送', className: 'tag-blue' },
    skipped: { label: '已跳过', className: 'tag-amber' },
    failed: { label: '失败', className: 'tag-red' },
    partial: { label: '部分成功', className: 'tag-amber' },
    success: { label: '成功', className: 'tag-green' },
  }
  return map[status] || { label: status || '未知', className: '' }
}
