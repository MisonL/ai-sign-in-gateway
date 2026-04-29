<script setup lang="ts">
const props = defineProps<{
  value: string | null | undefined
}>()

function labelFor(value: string | null | undefined) {
  if (!value) return '未执行'
  if (value === 'success') return '成功'
  if (value === 'failed') return '失败'
  if (value === 'scheduled') return '计划任务'
  if (value === 'batch') return '批量执行'
  if (value === 'active') return '进行中'
  if (value === 'pending') return '待处理'
  if (value === 'open') return '熔断中'
  if (value === 'half_open') return '半开探测'
  if (value === 'closed') return '健康'
  if (value === 'paused') return '已停用'
  if (value === 'idle') return '未加入'
  if (value === 'unsupported') return '不支持'
  return value
}

function colorFor(value: string | null | undefined) {
  if (value === 'success') return 'success'
  if (value === 'failed') return 'error'
  if (value === 'active') return 'processing'
  if (value === 'pending') return 'default'
  if (value === 'open') return 'error'
  if (value === 'half_open') return 'warning'
  if (value === 'closed') return 'success'
  if (value === 'paused') return 'default'
  if (value === 'idle') return 'default'
  if (value === 'unsupported') return 'default'
  return 'default'
}
</script>

<template>
  <a-tag :color="colorFor(props.value)">{{ labelFor(props.value) }}</a-tag>
</template>
