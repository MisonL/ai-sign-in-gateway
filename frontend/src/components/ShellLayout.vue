<script setup lang="ts">
import {
  ClusterOutlined,
  DashboardOutlined,
  DeploymentUnitOutlined,
  FundProjectionScreenOutlined,
  MessageOutlined,
  SettingOutlined,
} from '@ant-design/icons-vue'
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import sidebarGatewayArtwork from '../assets/design/sidebar-gateway.png'
import sidebarSkylineArtwork from '../assets/design/sidebar-skyline.png'
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
      // 静默失败，避免在登录前/无权限时刷新报错
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
          <img :src="sidebarGatewayArtwork" alt="" />
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
          <div class="sider-footer__meta">
            <span>控制台</span>
            <strong>爱签网关</strong>
          </div>
          <a-button class="sider-footer__button" block @click="navigate('/gateway/monitor')">查看运行日志</a-button>
        </div>
        <p>© 2025 爱签网关</p>
      </div>

      <div v-else class="sider-footer sider-footer--collapsed" aria-hidden="true">
        <span class="sider-footer__dot"></span>
      </div>

      <div class="sider-visual" :style="{ '--sider-skyline': `url(${sidebarSkylineArtwork})` }" aria-hidden="true"></div>
    </a-layout-sider>

    <a-layout class="app-main">
      <a-layout-header class="app-header">
        <div class="app-header__kpis" v-if="headerKpis.length">
          <div
            v-for="kpi in headerKpis"
            :key="kpi.key"
            class="header-kpi"
            :class="`header-kpi--${kpi.tone}`"
          >
            <span class="header-kpi__label">{{ kpi.label }}</span>
            <span class="header-kpi__value">{{ kpi.value }}</span>
          </div>
        </div>
        <a-space>
          <GroupManagerButton />
          <a-tag color="processing">{{ admin?.username ?? 'admin' }}</a-tag>
          <a-tag :color="admin?.role === 'super_admin' ? 'gold' : 'default'">{{ adminRoleLabel(admin?.role) }}</a-tag>
          <a-button @click="signOut">退出登录</a-button>
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
  grid-template-columns: minmax(0, 1fr) auto !important;
  align-items: center;
  gap: 12px 20px;
}

.app-header__kpis {
  display: flex;
  align-items: stretch;
  gap: 10px;
  flex-wrap: nowrap;
  min-width: 0;
  max-width: 100%;
  overflow-x: auto;
  scrollbar-width: none;
}

.app-header__kpis::-webkit-scrollbar {
  display: none;
}

.header-kpi {
  display: grid;
  gap: 2px;
  padding: 6px 14px 6px 14px;
  border-radius: var(--radius-control);
  border: 1px solid transparent;
  min-width: 96px;
  flex: 0 0 auto;
  position: relative;
  overflow: hidden;
  transition: transform 0.18s ease, box-shadow 0.18s ease, filter 0.18s ease;
}

.header-kpi:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 16px rgba(15, 32, 68, 0.08);
  filter: saturate(1.1);
}

.header-kpi__label {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  opacity: 0.85;
}

.header-kpi__value {
  font-size: 14px;
  font-weight: 700;
  font-feature-settings: 'tnum';
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 220px;
  letter-spacing: 0.01em;
}

.header-kpi--primary {
  background: linear-gradient(135deg, #e9efff 0%, #d6e1ff 100%);
  border-color: rgba(79, 124, 255, 0.32);
  color: #2c4cb8;
}

.header-kpi--info {
  background: linear-gradient(135deg, #dff0ff 0%, #cfe2ff 100%);
  border-color: rgba(56, 142, 220, 0.32);
  color: #1f5da8;
}

.header-kpi--success {
  background: linear-gradient(135deg, #e1f5e7 0%, #c9ecd4 100%);
  border-color: rgba(40, 154, 70, 0.32);
  color: #1f7a37;
}

.header-kpi--warning {
  background: linear-gradient(135deg, #fff4d8 0%, #ffe3a6 100%);
  border-color: rgba(217, 142, 0, 0.32);
  color: #9a5b00;
}

.header-kpi--neutral {
  background: linear-gradient(135deg, #f1f1f5 0%, #e0e3eb 100%);
  border-color: rgba(100, 116, 139, 0.32);
  color: #475467;
}

.header-kpi__label {
  color: inherit;
}

.header-kpi__value {
  color: inherit;
}

@media (max-width: 1180px) {
  .header-kpi {
    min-width: 80px;
    padding: 5px 11px;
  }
  .header-kpi__value {
    font-size: 13px;
  }
}

@media (max-width: 860px) {
  .app-header__kpis {
    overflow-x: auto;
    flex-wrap: nowrap;
    scrollbar-width: none;
  }

  .app-header__kpis::-webkit-scrollbar {
    display: none;
  }
}
</style>
