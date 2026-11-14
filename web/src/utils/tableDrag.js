// 列表横向拖动（拖拽平移）
//
// 全站所有「可横向滚动」的列表都支持按住鼠标左右拖动：
// - Element Plus 表格（.el-table 的 body 滚动层）
// - 看板数据表格 / 数据集预览等原生 table 包装层
// 拖动超过阈值后才会进入平移状态，避免影响单击行内按钮、排序表头等交互；
// 拖动结束时吞掉随后的那一次 click，防止误触「编辑 / 删除」。

// 允许拖动的滚动容器（必须自身是横向滚动层）
const PAN_SELECTOR = [
  '.el-table__body-wrapper .el-scrollbar__wrap',
  '.el-table__header-wrapper .el-scrollbar__wrap',
  '.table-wrap',
  '.preview-table',
  '.drill-table-wrap',
].join(',')

// 扫描用选择器：命中的容器若确实有横向溢出，就打上 is-pannable 以便显示抓取手势
const MARK_SELECTOR = '.el-scrollbar__wrap, .table-wrap, .preview-table, .drill-table-wrap'

// 横向拖动阈值（px）：小于该距离视为普通点击
const DRAG_THRESHOLD = 5

let installed = false
// 当前拖动会话：{ el, startX, startY, startLeft, pointerId, moved }
let session = null
// 拖动结束后需要吞掉一次 click
let suppressClick = false
let markTimer = 0

// 判断元素是否可横向滚动
function canScrollX(el) {
  if (!el || el.nodeType !== 1) return false
  return el.scrollWidth - el.clientWidth > 1
}

// 从事件目标向上找到「真正承担横向滚动」的容器
function findScroller(target) {
  if (!(target instanceof Element)) return null
  // 表头与表体是两个滚动层，拖表头时统一驱动表体（Element Plus 会同步表头横向位置）
  const header = target.closest('.el-table__header-wrapper')
  if (header) {
    const table = header.closest('.el-table')
    const body = table?.querySelector('.el-table__body-wrapper .el-scrollbar__wrap')
    if (body && canScrollX(body)) return body
  }
  const hit = target.closest(PAN_SELECTOR)
  if (hit && canScrollX(hit)) return hit
  return null
}

// 给有横向溢出的容器打标记，用于 CSS 显示 grab 手势
export function markPannableTables(root = document) {
  root.querySelectorAll?.(MARK_SELECTOR).forEach((el) => {
    el.classList.toggle('is-pannable', canScrollX(el))
  })
}

// 合并多次 DOM 变化，300ms 内只扫描一次
function scheduleMark() {
  window.clearTimeout(markTimer)
  markTimer = window.setTimeout(() => markPannableTables(), 300)
}

function startPan(event) {
  // 触屏保留浏览器原生滑动，避免与页面纵向滚动冲突
  if (event.pointerType === 'touch' || event.button !== 0) return
  const scroller = findScroller(event.target)
  if (!scroller) return
  session = {
    el: scroller,
    startX: event.clientX,
    startY: event.clientY,
    startLeft: scroller.scrollLeft,
    pointerId: event.pointerId,
    moved: false,
  }
}

function movePan(event) {
  if (!session || event.pointerId !== session.pointerId) return
  const dx = event.clientX - session.startX
  const dy = event.clientY - session.startY
  if (!session.moved) {
    // 横向位移明显大于纵向时才判定为拖动，保留纵向滚动 / 点击
    if (Math.abs(dx) < DRAG_THRESHOLD || Math.abs(dx) <= Math.abs(dy)) return
    session.moved = true
    session.el.classList.add('is-panning')
    document.body.classList.add('is-panning-table')
  }
  event.preventDefault()
  session.el.scrollLeft = session.startLeft - dx
}

function endPan(event) {
  if (!session) return
  if (event && event.pointerId !== undefined && event.pointerId !== session.pointerId) return
  const { el, moved } = session
  el.classList.remove('is-panning')
  document.body.classList.remove('is-panning-table')
  session = null
  if (moved) suppressClick = true
}

// 拖动完成后拦截一次 click（捕获阶段直接拦下，避免触发行内按钮）
function swallowClick(event) {
  if (!suppressClick) return
  suppressClick = false
  event.stopPropagation()
  event.preventDefault()
}

export function installTableDragPan() {
  if (installed || typeof window === 'undefined') return
  installed = true

  document.addEventListener('pointerdown', startPan)
  // pointermove 需要 preventDefault，必须显式声明非 passive
  document.addEventListener('pointermove', movePan, { passive: false })
  document.addEventListener('pointerup', endPan)
  document.addEventListener('pointercancel', endPan)
  document.addEventListener('click', swallowClick, true)
  // 拖动时禁止浏览器把表格文字当成选区拖拽
  document.addEventListener('dragstart', (event) => {
    if (session?.moved) event.preventDefault()
  })

  window.addEventListener('resize', scheduleMark)
  // 数据加载 / 切换页面后重新标记哪些表格需要显示抓取手势
  const observer = new MutationObserver(scheduleMark)
  observer.observe(document.body, { childList: true, subtree: true })
  scheduleMark()
}
