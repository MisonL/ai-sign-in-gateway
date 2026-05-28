<script setup lang="ts">
import {
  HistoryOutlined,
  InfoCircleOutlined,
  MoreOutlined,
  ReloadOutlined,
  SettingOutlined,
  SyncOutlined,
  ToolOutlined,
} from '@ant-design/icons-vue'
import type { GatewayRoute } from '../../types'

defineProps<{
  route: GatewayRoute
  routeProbing: boolean
  balanceProbing: boolean
}>()

const emit = defineEmits<{
  (event: 'toggle', route: GatewayRoute): void
  (event: 'reset-circuit', route: GatewayRoute): void
  (event: 'probe', route: GatewayRoute): void
  (event: 'probe-balance', route: GatewayRoute): void
  (event: 'configure-models', route: GatewayRoute): void
  (event: 'enable-only', route: GatewayRoute): void
  (event: 'priority', route: GatewayRoute): void
  (event: 'diagnose', route: GatewayRoute): void
  (event: 'history', route: GatewayRoute): void
}>()
</script>

<template>
  <a-space size="small" class="gateway-actions-cell">
    <a-button size="small" :danger="route.is_enabled" @click.stop="emit('toggle', route)">
      {{ route.is_enabled ? '禁用' : '启用' }}
    </a-button>
    <a-dropdown :trigger="['click']">
      <a-tooltip title="更多操作">
        <a-button
          size="small"
          class="gateway-actions-menu-button"
          :aria-label="`${route.site_name || route.base_url}更多操作`"
          :loading="routeProbing || balanceProbing"
          @click.stop
        >
          <template #icon><MoreOutlined /></template>
        </a-button>
      </a-tooltip>
      <template #overlay>
        <a-menu @click.stop>
          <a-menu-item
            key="reset-circuit"
            :disabled="route.circuit_state === 'closed'"
            @click="emit('reset-circuit', route)"
          >
            <ReloadOutlined />
            <span>重置熔断</span>
          </a-menu-item>
          <a-menu-item key="probe" :disabled="routeProbing" @click="emit('probe', route)">
            <SyncOutlined />
            <span>探测</span>
          </a-menu-item>
          <a-menu-item key="balance" :disabled="balanceProbing" @click="emit('probe-balance', route)">
            <InfoCircleOutlined />
            <span>余额</span>
          </a-menu-item>
          <a-menu-item key="supported-models" @click="emit('configure-models', route)">
            <ToolOutlined />
            <span>路由配置</span>
          </a-menu-item>
          <a-menu-divider />
          <a-menu-item key="enable-only" @click="emit('enable-only', route)">
            <SettingOutlined />
            <span>禁用其他</span>
          </a-menu-item>
          <a-menu-item key="priority" @click="emit('priority', route)">
            <SettingOutlined />
            <span>优先权</span>
          </a-menu-item>
          <a-menu-item key="diagnosis" @click="emit('diagnose', route)">
            <ToolOutlined />
            <span>诊断</span>
          </a-menu-item>
          <a-menu-item key="history" @click="emit('history', route)">
            <HistoryOutlined />
            <span>历史</span>
          </a-menu-item>
        </a-menu>
      </template>
    </a-dropdown>
  </a-space>
</template>
