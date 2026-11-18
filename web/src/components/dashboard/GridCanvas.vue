<template>
  <grid-layout
    :layout="effective"
    :col-num="colNum"
    :row-height="rowHeight"
    :is-draggable="editable"
    :is-resizable="editable"
    :vertical-compact="true"
    :use-css-transforms="true"
    :margin="margin"
    @layout-updated="onLayoutUpdated"
  >
    <grid-item
      v-for="item in effective"
      :key="item.i"
      :x="item.x"
      :y="item.y"
      :w="item.w"
      :h="item.h"
      :i="item.i"
      drag-allow-from=".widget-header"
      drag-ignore-from=".widget-actions"
    >
      <slot :item="item" />
    </grid-item>
  </grid-layout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { GridLayout, GridItem } from 'grid-layout-plus'

export interface GridItemModel {
  i: string
  x: number
  y: number
  w: number
  h: number
  [key: string]: any
}

const props = defineProps<{
  layout: GridItemModel[]
  editable?: boolean
}>()

// On narrow screens widgets stack full-width in their original vertical order
// instead of being scaled down to unreadable slivers. This is a purely visual
// transform: the saved 12-column layout is never mutated while viewing.
function effectiveLayout(): GridItemModel[] {
  if (props.editable || viewport.value >= 700) return props.layout
  return [...props.layout]
    .sort((a, b) => (a.y - b.y) || (a.x - b.x))
    .map((item) => ({ ...item, x: 0, w: 12 }))
}

// The layout is authored on a 12-column grid; keep colNum fixed so stored x/w
// coordinates stay valid at every viewport. Narrow screens only shrink the row
// height and margins, letting the grid scale down proportionally.
const viewport = ref(typeof window === 'undefined' ? 1280 : window.innerWidth)
let timer = 0
function onResize() {
  window.clearTimeout(timer)
  timer = window.setTimeout(() => { viewport.value = window.innerWidth }, 120)
}
onMounted(() => window.addEventListener('resize', onResize))
onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  window.clearTimeout(timer)
})

const effective = computed(effectiveLayout)
const colNum = computed(() => 12)
const rowHeight = computed(() => (viewport.value < 700 ? 30 : 38))
const margin = computed<[number, number]>(() => (viewport.value < 700 ? [6, 6] : [10, 10]))

const emit = defineEmits<{
  (e: 'layout-updated', layout: GridItemModel[]): void
}>()

function onLayoutUpdated(layout: any[]) {
  emit('layout-updated', layout as GridItemModel[])
}
</script>

<style scoped>
:deep(.vgl-item) {
  transition: all 0.2s;
}
:deep(.vgl-item--placeholder) {
  background: #dbeafe;
  border: 1px dashed #409eff;
  border-radius: 8px;
  opacity: 0.7;
}
</style>
