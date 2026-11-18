import { onBeforeUnmount, onMounted, ref } from 'vue'

/** Dialog width capped to the viewport so wide editors stay usable on tablets. */
export function dialogWidth(max: number): string {
  const vw = typeof window === 'undefined' ? 1024 : window.innerWidth
  return `min(${max}px, calc(100vw - 24px))`
}

/** Reactive viewport width with a caller-provided debounce. */
export function useViewport() {
  const width = ref(typeof window === 'undefined' ? 1280 : window.innerWidth)
  let timer = 0
  function onResize() {
    window.clearTimeout(timer)
    timer = window.setTimeout(() => {
      width.value = window.innerWidth
    }, 100)
  }
  onMounted(() => window.addEventListener('resize', onResize))
  onBeforeUnmount(() => {
    window.removeEventListener('resize', onResize)
    window.clearTimeout(timer)
  })
  return {
    width,
    isMobile: () => width.value < 640,
    isTablet: () => width.value < 900,
    isCompact: () => width.value < 1200,
  }
}
