<script setup lang="ts">
const theme = {
  token: {
    borderRadius: 4,
    borderRadiusLG: 4,
    borderRadiusSM: 2,
    borderRadiusXS: 2,
    colorPrimary: '#0f766e',
    colorInfo: '#2563eb',
    colorBgLayout: '#eef2f6',
    colorBorder: '#d8dee7',
    fontFamily: "'IBM Plex Sans', sans-serif",
    controlHeight: 32,
    controlHeightLG: 36,
    controlHeightSM: 26,
  },
  components: {
    Card: {
      borderRadiusLG: 4,
      paddingLG: 16,
    },
    Button: {
      borderRadius: 4,
      controlHeight: 32,
    },
    Input: {
      borderRadius: 6,
      controlHeight: 32,
    },
    InputNumber: {
      borderRadius: 6,
      controlHeight: 32,
    },
    Select: {
      borderRadius: 6,
      controlHeight: 32,
    },
  },
}
</script>

<template>
  <a-config-provider :theme="theme">
    <router-view />
  </a-config-provider>
</template>
