<script setup lang="ts">
/** 字典下拉：设备类型 / 负责人统一控件。
 *  - 从设置页字典渲染选项
 *  - 当前值不在字典中时（旧数据）保留显示
 *  - 样式与全局表单 select 一致
 */
const props = defineProps<{
  modelValue: string
  options: string[]
  placeholder?: string
}>()
const emit = defineEmits<{ 'update:modelValue': [string] }>()
</script>

<template>
  <select :value="props.modelValue"
          @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
          class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-normal bg-white">
    <option value="">{{ props.placeholder || '未指定' }}</option>
    <option v-for="o in props.options" :key="o" :value="o">{{ o }}</option>
    <option v-if="props.modelValue && !props.options.includes(props.modelValue)" :value="props.modelValue">
      {{ props.modelValue }}（旧值）
    </option>
  </select>
</template>
