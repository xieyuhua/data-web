<!-- 通用空状态占位组件：图标 + 标题 + 描述 + 可选插槽（如“去创建”按钮） -->
<script setup>
import { Inbox } from 'lucide-vue-next'

defineProps({
  // 空状态标题，默认“暂无数据”
  title: { type: String, default: '暂无数据' },
  // 补充说明文案，为空时不渲染
  description: { type: String, default: '' },
})
</script>

<template>
  <div class="empty-state">
    <div class="empty-state-icon">
      <Inbox :size="23" />
    </div>
    <h3>{{ title }}</h3>
    <p v-if="description">{{ description }}</p>
    <slot />
  </div>
</template>
