import { onBeforeUnmount, ref } from 'vue'

// 通用全屏切换：支持传入页面根元素 ref，未传入时使用 documentElement。
export function useFullscreen(targetRef) {
  const isFullscreen = ref(false)

  function sync() {
    isFullscreen.value = Boolean(document.fullscreenElement)
  }

  async function toggleFullscreen() {
    try {
      if (document.fullscreenElement) {
        await document.exitFullscreen()
        return
      }
      const element = targetRef?.value || document.documentElement
      if (element.requestFullscreen) {
        await element.requestFullscreen()
      } else if (element.webkitRequestFullscreen) {
        element.webkitRequestFullscreen()
      }
    } catch {
      /* 浏览器拒绝全屏时保持原状 */
    }
  }

  document.addEventListener('fullscreenchange', sync)
  onBeforeUnmount(() => document.removeEventListener('fullscreenchange', sync))

  return { isFullscreen, toggleFullscreen }
}
