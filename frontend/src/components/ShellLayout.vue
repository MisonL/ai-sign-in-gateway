<script setup lang="ts">
import {
  ApiOutlined,
  ClusterOutlined,
  DashboardOutlined,
  DeploymentUnitOutlined,
  FundProjectionScreenOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MessageOutlined,
  SettingOutlined,
  ThunderboltOutlined,
  UserOutlined,
} from '@ant-design/icons-vue'
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getGatewayOverview, getMe, isAbortError, logout } from '../api'
import { onGatewayOverviewChanged } from '../gatewayOverviewEvents'
import GroupManagerButton from './GroupManagerButton.vue'
import type { AdminUser, GatewayOverview } from '../types'

const route = useRoute()
const router = useRouter()
const admin = ref<AdminUser | null>(null)
const collapsed = ref(false)
const gatewayOverview = ref<GatewayOverview | null>(null)
let kpiTimer: number | null = null
let stopGatewayOverviewListener: (() => void) | null = null
let mounted = false
let adminController: AbortController | null = null
let kpiController: AbortController | null = null
let kpiLoading = false

const enabledFeatureKeys = new Set([
  'overview',
  'sites',
  'gateway-routes',
  'gateway-monitor',
  'chat-test',
  'settings',
])

const navigation = [
  { key: 'overview', label: '总览', to: '/overview', icon: DashboardOutlined, description: '关键指标、执行状态与异常站点。' },
  { key: 'sites', label: '站点中心', to: '/sites', icon: ClusterOutlined, description: '站点授权、批量签到与状态巡检。' },
  { key: 'gateway-routes', label: '路由管理', to: '/gateway/routes', icon: DeploymentUnitOutlined, description: '统一出口路由池、熔断状态与上游维护。' },
  { key: 'gateway-monitor', label: '网关监控', to: '/gateway/monitor', icon: FundProjectionScreenOutlined, description: '请求趋势、策略统计与网关访问配置。' },
  { key: 'chat-test', label: '对话', to: '/chat-test', icon: MessageOutlined, description: '按站点读取模型并发起对话或图片生成。' },
  { key: 'settings', label: '设置', to: '/settings', icon: SettingOutlined, description: '调度计划、超时与执行策略。' },
]

const visibleNavigation = computed(() => navigation.filter((item) => enabledFeatureKeys.has(item.key)))
const selectedNavigationKeys = computed(() => {
  const matched = visibleNavigation.value.find((item) => route.path === item.to || route.path.startsWith(`${item.to}/`))
  return [matched?.to ?? route.path]
})
const activeNavigation = computed(() =>
  visibleNavigation.value.find((item) => route.path === item.to || route.path.startsWith(`${item.to}/`)) ?? visibleNavigation.value[0] ?? navigation[0],
)

const headerKpis = computed(() => {
  const ov = gatewayOverview.value
  if (!ov) {
    return []
  }
  return [
    {
      key: 'balance',
      label: '总额度',
      value: ov.total_balance_display || '暂无',
      tone: 'info' as const,
    },
    {
      key: 'requests',
      label: '24h 请求',
      value: String(ov.request_count_24h ?? 0),
      tone: 'primary' as const,
    },
    {
      key: 'success',
      label: '成功率',
      value: `${ov.success_rate_24h ?? 0}%`,
      tone: 'success' as const,
    },
    {
      key: 'concurrency',
      label: '当前并发',
      value: String(ov.active_concurrency ?? 0),
      tone: 'neutral' as const,
    },
    {
      key: 'today-peak-concurrency',
      label: '今日峰值',
      value: String(ov.max_concurrency_today ?? 0),
      tone: 'warning' as const,
    },
    {
      key: 'all-time-peak-concurrency',
      label: '历史峰值',
      value: String(ov.max_concurrency_all_time ?? 0),
      tone: 'info' as const,
    },
  ]
})

async function loadAdmin() {
  adminController?.abort()
  const controller = new AbortController()
  adminController = controller
  try {
    const adminData = await getMe({ signal: controller.signal })
    if (!mounted || controller.signal.aborted) {
      return
    }
    admin.value = adminData
  } catch (err) {
    if (isAbortError(err) || !mounted) {
      return
    }
    logout()
    router.push('/login')
  } finally {
    if (adminController === controller) {
      adminController = null
    }
  }
}

async function loadGatewayKpi() {
  if (kpiLoading || !mounted) {
    return
  }
  kpiLoading = true
  kpiController?.abort()
  const controller = new AbortController()
  kpiController = controller
  try {
    const overview = await getGatewayOverview({ signal: controller.signal })
    if (mounted && !controller.signal.aborted) {
      gatewayOverview.value = overview
    }
  } catch (err) {
    if (!isAbortError(err)) {
      console.warn('网关概览刷新失败', err)
    }
  } finally {
    if (kpiController === controller) {
      kpiController = null
    }
    kpiLoading = false
  }
}

function navigate(to: string) {
  router.push(to)
}

function signOut() {
  logout()
  router.push('/login')
}

function adminRoleLabel(role?: string) {
  return role === 'super_admin' ? '超级管理员' : '管理员'
}

function toggleCollapsed() {
  collapsed.value = !collapsed.value
}

onMounted(async () => {
  mounted = true
  await loadAdmin()
  if (!mounted) {
    return
  }
  await loadGatewayKpi()
  if (!mounted) {
    return
  }
  stopGatewayOverviewListener = onGatewayOverviewChanged(loadGatewayKpi)
  kpiTimer = window.setInterval(loadGatewayKpi, 30_000)
})

onBeforeUnmount(() => {
  mounted = false
  stopGatewayOverviewListener?.()
  stopGatewayOverviewListener = null
  adminController?.abort()
  adminController = null
  kpiController?.abort()
  kpiController = null
  if (kpiTimer !== null) {
    window.clearInterval(kpiTimer)
    kpiTimer = null
  }
})
</script>

<template>
  <a-layout class="app-shell">
    <a-layout-sider
      v-model:collapsed="collapsed"
      class="app-sider"
      :width="248"
      :collapsed-width="88"
      breakpoint="lg"
      theme="light"
    >
      <div class="brand-panel">
        <div class="brand-mark" aria-hidden="true">
          <ThunderboltOutlined />
        </div>
        <div v-if="!collapsed">
          <strong>爱签网关</strong>
          <p>签到与网关管理后台</p>
        </div>
      </div>

      <a-menu :selected-keys="selectedNavigationKeys" mode="inline" class="app-menu">
        <a-menu-item
          v-for="item in visibleNavigation"
          :key="item.to"
          @click="navigate(item.to)"
        >
          <template #icon>
            <component :is="item.icon" />
          </template>
          <span>{{ item.label }}</span>
        </a-menu-item>
      </a-menu>

      <div v-if="!collapsed" class="sider-footer">
        <div class="sider-footer__panel">
          <div class="sider-footer__head">
            <span>系统状态</span>
            <span class="sider-footer__badge">稳定</span>
          </div>
          <div class="sider-footer__status">
            <span class="sider-footer__dot"></span>
            <strong>网关运行中</strong>
          </div>
          <div class="sider-footer__meta">
            <span>当前用户</span>
            <strong>{{ admin?.username ?? 'admin' }}</strong>
          </div>
          <div class="sider-footer__meta">
            <span>权限</span>
            <strong>{{ adminRoleLabel(admin?.role) }}</strong>
          </div>
          <a-button class="sider-footer__button" block @click="navigate('/gateway/monitor')">
            <template #icon>
              <ApiOutlined />
            </template>
            网关监控
          </a-button>
        </div>
        <a-button class="sider-collapse-button" block @click="toggleCollapsed">
          <template #icon>
            <MenuFoldOutlined />
          </template>
          收起导航
        </a-button>
      </div>

      <div v-else class="sider-footer sider-footer--collapsed">
        <a-button class="sider-collapse-button" shape="circle" @click="toggleCollapsed" aria-label="展开导航">
          <template #icon>
            <MenuUnfoldOutlined />
          </template>
        </a-button>
      </div>
    </a-layout-sider>

    <a-layout class="app-main">
      <a-layout-header class="app-header">
        <div class="app-header__page">
          <span class="app-header__page-icon" aria-hidden="true">
            <component :is="activeNavigation.icon" />
          </span>
          <div>
            <strong>{{ activeNavigation.label }}</strong>
            <p>{{ activeNavigation.description }}</p>
          </div>
        </div>

        <div class="app-header__summary" v-if="headerKpis.length">
          <div
            v-for="kpi in headerKpis"
            :key="kpi.key"
            class="header-status"
            :class="`header-status--${kpi.tone}`"
          >
            <span class="header-status__label">{{ kpi.label }}</span>
            <span class="header-status__value">{{ kpi.value }}</span>
          </div>
        </div>

        <a-space class="app-header__actions">
          <GroupManagerButton />
          <a-tag color="processing" class="app-header__user">
            <UserOutlined />
            {{ admin?.username ?? 'admin' }}
          </a-tag>
          <a-tag :color="admin?.role === 'super_admin' ? 'gold' : 'default'" class="app-header__user">
            {{ adminRoleLabel(admin?.role) }}
          </a-tag>
          <a-button @click="signOut">
            <template #icon>
              <LogoutOutlined />
            </template>
            退出
          </a-button>
        </a-space>
      </a-layout-header>

      <a-layout-content class="app-content">
        <slot />
      </a-layout-content>
    </a-layout>
  </a-layout>
</template>

<style scoped>
.app-header {
  display: grid !important;
  grid-template-columns: minmax(180px, 1fr) minmax(0, auto) auto !important;
  align-items: center;
  gap: 10px 16px;
}

.app-header__page {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.app-header__page-icon {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-container);
  background: var(--accent-soft);
  color: var(--accent);
  flex: 0 0 auto;
}

.app-header__page strong {
  display: block;
  color: var(--text-main);
  font-size: 15px;
  line-height: 1.2;
}

.app-header__page p {
  margin: 2px 0 0;
  overflow: hidden;
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.25;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.app-header__summary {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: nowrap;
  min-width: 0;
  max-width: 100%;
  overflow-x: auto;
  scrollbar-width: none;
}

.app-header__summary::-webkit-scrollbar {
  display: none;
}

.header-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  padding: 6px 10px;
  border-radius: var(--radius-control);
  border: 1px solid var(--border-soft);
  background: var(--bg-panel);
  color: var(--text-muted);
  flex: 0 0 auto;
  line-height: 1;
}

.header-status__label {
  font-size: 10px;
  font-weight: 600;
  white-space: nowrap;
}

.header-status__value {
  color: var(--text-main);
  font-size: 13px;
  font-weight: 700;
  font-feature-settings: 'tnum';
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 140px;
}

.header-status--primary,
.header-status--info {
  border-color: rgba(37, 99, 235, 0.22);
  background: var(--accent-soft);
}

.header-status--primary .header-status__value,
.header-status--info .header-status__value {
  color: var(--accent-strong);
}

.header-status--success {
  border-color: rgba(22, 163, 74, 0.24);
  background: var(--success-soft);
}

.header-status--success .header-status__value {
  color: var(--success);
}

.header-status--warning {
  border-color: rgba(245, 158, 11, 0.28);
  background: var(--warning-soft);
}

.header-status--warning .header-status__value {
  color: #a16207;
}

.app-header__actions {
  justify-self: end;
}

.app-header__user {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 32px;
}

@media (max-width: 1180px) {
  .app-header {
    grid-template-columns: minmax(180px, 1fr) auto !important;
  }

  .app-header__summary {
    grid-column: 1 / -1;
    justify-content: flex-start;
    order: 3;
  }
}

@media (max-width: 860px) {
  .app-header {
    grid-template-columns: minmax(0, 1fr) !important;
  }

  .app-header__actions {
    justify-self: start;
  }

  .app-header__page p {
    white-space: normal;
  }

  .header-status__value {
    max-width: 110px;
  }
}
</style>
