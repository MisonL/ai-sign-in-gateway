<script setup lang="ts">
const props = defineProps<{
  pluginKey: string | null | undefined
  label?: string | null | undefined
}>()

const PLUGIN_LABELS: Record<string, string> = {
  'yellowpeach-newapi': '黄桃 New API',
  'sub2api-platform': 'sub2api 平台',
  'http-relay-station': '正式 HTTP 中转站',
}

const PLUGIN_COLORS: Record<string, string> = {
  'yellowpeach-newapi': 'gold',
  'sub2api-platform': 'geekblue',
  'http-relay-station': 'cyan',
}

function labelFor(pluginKey: string | null | undefined, label: string | null | undefined) {
  if (label && label.trim()) {
    return label.trim()
  }
  if (!pluginKey) {
    return '未知插件'
  }
  return PLUGIN_LABELS[pluginKey] ?? pluginKey
}

function colorFor(pluginKey: string | null | undefined) {
  if (!pluginKey) {
    return 'default'
  }
  return PLUGIN_COLORS[pluginKey] ?? 'blue'
}
</script>

<template>
  <a-tag
    class="plugin-tag"
    :color="colorFor(props.pluginKey)"
    :title="props.pluginKey ?? 'unknown-plugin'"
  >
    {{ labelFor(props.pluginKey, props.label) }}
  </a-tag>
</template>
