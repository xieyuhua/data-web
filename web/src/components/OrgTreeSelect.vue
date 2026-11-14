<!-- 组织结构树选择器：把后端返回的组织树转换为 Element Plus 树选择器所需结构 -->
<script setup>
import { computed } from 'vue'
import { toTreeOptions } from '../utils/org'

const props = defineProps({
  // 选中值：单选时为组织 ID，多选时为 ID 数组
  modelValue: { type: [Number, String, Array], default: null },
  // 组织树原始数据（含 children 嵌套）
  data: { type: Array, default: () => [] },
  placeholder: { type: String, default: '请选择组织' },
  clearable: { type: Boolean, default: true },
  // 是否多选；多选时开启 check-strictly，父子节点互不联动，便于精确勾选
  multiple: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
})

const emit = defineEmits(['update:modelValue'])
// 把组织树转换成 { value, label, children } 结构，源数据变化时自动重新计算
const options = computed(() => toTreeOptions(props.data))
</script>

<template>
  <el-tree-select
    :model-value="modelValue"
    :data="options"
    :props="{ label: 'label', children: 'children' }"
    node-key="value"
    :placeholder="placeholder"
    :clearable="clearable"
    :multiple="multiple"
    :disabled="disabled"
    :check-strictly="multiple"
    :render-after-expand="false"
    filterable
    style="width: 100%"
    @update:model-value="emit('update:modelValue', $event)"
  />
</template>
