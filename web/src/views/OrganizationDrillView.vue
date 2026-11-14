<!-- 组织下钻页面：
     直接复用看板内的 OrgDrillExplorer 组件，保证「菜单入口」和「看板内下钻」
     使用同一套交互（逐级下钻、全部门店、全部员工、指标趋势、目标达成率），
     避免两块代码长期分叉。 -->
<script setup>
import OrgDrillExplorer from '../components/OrgDrillExplorer.vue'
</script>

<template>
  <OrgDrillExplorer api-base="/drill" initial-mode="level" />
</template>
