<script setup>
import { computed } from 'vue'
import { toTreeOptions } from '../utils/org'

const props = defineProps({
  modelValue: { type: [Number, String, Array], default: null },
  data: { type: Array, default: () => [] },
  placeholder: { type: String, default: '请选择组织' },
  clearable: { type: Boolean, default: true },
  multiple: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
})

const emit = defineEmits(['update:modelValue'])
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
