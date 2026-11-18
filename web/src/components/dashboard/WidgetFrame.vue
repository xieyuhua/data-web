<template>
  <div class="widget">
    <div class="widget-header">
      <span class="widget-title">{{ widget.title || '未命名组件' }}</span>
      <div class="widget-actions">
        <slot name="actions" />
      </div>
    </div>
    <div class="widget-body">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ widget: { title: string } }>()
</script>

<style scoped>
.widget {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #fff;
  border-radius: 8px;
  overflow: hidden;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.06);
}
.widget-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 38px;
  padding: 0 12px;
  border-bottom: 1px solid #f0f2f5;
  flex-shrink: 0;
}
.widget-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.widget-actions {
  display: flex;
  gap: 4px;
  opacity: 0;
  transition: opacity 0.15s;
}
.widget:hover .widget-actions {
  opacity: 1;
}
.widget-body {
  flex: 1;
  min-height: 0;
  padding: 8px;
}
</style>
