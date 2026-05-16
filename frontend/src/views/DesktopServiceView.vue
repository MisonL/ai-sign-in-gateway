<script setup lang="ts">
import {
  ApiOutlined,
  CopyOutlined,
  DashboardOutlined,
  GlobalOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  SettingOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons-vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  getGatewayOverview,
  getOverview,
  getSettings,
  isAbortError,
  logout,
  openRuntimeUrl,
} from '../api'
import { useToast } from '../toast'
import type { GatewayOverview, OverviewData, SettingsData } from '../types'
import SettingsView from './SettingsView.vue'

const toast = useToast()
const router = useRouter()

const loading = ref(false)
const overview = ref<OverviewData | null>(null)
const gatewayOverview = ref<GatewayOverview | null>(null)
let refreshTimer: number | null = null
let mounted = false
let refreshController: AbortController | null = null
let refreshInFlight = false

const settings = reactive<SettingsData>({
  timezone: 'Asia/Shanghai',
  schedule_enabled: true,
  daily_run_time: '09:00',
  checkin_concurrency: 1,
  checkin_global_concurrency: 4,
  checkin_interval_seconds: 1,
  retry_count: 1,
  request_timeout: 20,
  only_enabled_sites: true,
  desktop_keep_running: false,
  database_backup_enabled: false,
  database_backup_dir: '',
  database_backup_interval_minutes: 1440,
  database_backup_retention: 7,
  log_retention_days: 5,
  gateway_pricing_active_scheme_id: 'official',
  gateway_pricing_schemes: [],
  feature_flags: {},
  features: [],
  desktop_frontend_default_port: 3721,
  desktop_frontend_port: 0,
  desktop_frontend_url: '',
  desktop_frontend_default_port_occupant: '',
  desktop_backend_default_port: 8972,
  desktop_backend_port: 0,
  desktop_backend_url: '',
  desktop_backend_default_port_occupant: '',
  desktop_gateway_url: '',
  runtime_config_dir: '',
  runtime_default_config_dir: '',
  runtime_database_path: '',
  runtime_pending_config_dir: '',
  security_warnings: [],
})

const adminUrl = computed(() => joinAppPath(settings.desktop_frontend_url || window.location.origin, '/overview'))
const gatewayApiUrl = computed(() => settings.desktop_gateway_url || joinAppPath(settings.desktop_backend_url || window.location.origin, '/api/gateway'))

const serviceItems = computed(() => [
  {
    key: 'frontend',
    label: '管理前端',
    value: settings.desktop_frontend_url || '-',
    meta: settings.desktop_frontend_port ? `:${settings.desktop_frontend_port}` : '未监听',
    ok: Boolean(settings.desktop_frontend_url),
  },
  {
    key: 'backend',
    label: '后台服务',
    value: settings.desktop_backend_url || '-',
    meta: settings.desktop_backend_port ? `:${settings.desktop_backend_port}` : '未监听',
    ok: Boolean(settings.desktop_backend_url),
  },
  {
    key: 'gateway',
    label: '网关出口',
    value: gatewayApiUrl.value || '-',
    meta: gatewayOverview.value ? `${gatewayOverview.value.healthy_routes}/${gatewayOverview.value.total_routes} 可用` : '待刷新',
    ok: Boolean(settings.desktop_gateway_url),
  },
])

const summaryItems = computed(() => [
  {
    key: 'balance',
    label: '总余额',
    value: gatewayOverview.value?.total_balance_display || '暂无',
    icon: DashboardOutlined,
    tone: 'blue',
  },
  {
    key: 'routes',
    label: '可用路由',
    value: gatewayOverview.value ? `${gatewayOverview.value.healthy_routes}/${gatewayOverview.value.total_routes}` : '-',
    icon: ApiOutlined,
    tone: 'green',
  },
  {
    key: 'sites',
    label: '启用站点',
    value: overview.value ? `${overview.value.enabled_site_count}/${overview.value.site_count}` : '-',
    icon: GlobalOutlined,
    tone: 'slate',
  },
  {
    key: 'runs',
    label: '今日执行',
    value: overview.value ? `${overview.value.today_success}/${overview.value.today_failed}` : '-',
    icon: ThunderboltOutlined,
    tone: 'amber',
  },
])

const runtimeProblems = computed(() =>
  [
    meaningfulPortOccupant(settings.desktop_frontend_default_port_occupant)
      ? `前端默认端口占用：${settings.desktop_frontend_default_port_occupant}`
      : '',
    meaningfulPortOccupant(settings.desktop_backend_default_port_occupant)
      ? `后端默认端口占用：${settings.desktop_backend_default_port_occupant}`
      : '',
    gatewayOverview.value?.open_circuit_routes ? `熔断路由：${gatewayOverview.value.open_circuit_routes}` : '',
    overview.value?.today_failed ? `今日失败：${overview.value.today_failed}` : '',
  ].filter(Boolean),
)

function meaningfulPortOccupant(value: string) {
  const normalized = value.trim()
  return Boolean(normalized && normalized !== '当前程序' && normalized !== '未占用')
}

function joinAppPath(base: string, path: string) {
  try {
    return new URL(path, base.endsWith('/') ? base : `${base}/`).toString()
  } catch {
    return path
  }
}

async function openUrl(url: string) {
  if (!url || url === '-') {
    toast.error('地址暂不可用。')
    return
  }
  try {
    await openRuntimeUrl(url)
  } catch (err) {
    if (url.startsWith(window.location.origin)) {
      window.location.href = url
      return
    }
    window.open(url, '_blank', 'noopener,noreferrer')
    toast.error(err instanceof Error ? err.message : '系统浏览器打开失败')
  }
}

async function copyUrl(url: string) {
  if (!url || url === '-') {
    toast.error('地址暂不可用。')
    return
  }
  try {
    await navigator.clipboard.writeText(url)
    toast.success('地址已复制。')
  } catch {
    toast.error('复制失败。')
  }
}

async function refreshAll(showToast = false) {
  if (refreshInFlight || !mounted) {
    return
  }
  refreshInFlight = true
  refreshController?.abort()
  const controller = new AbortController()
  refreshController = controller
  loading.value = true
  try {
    const [settingsData, overviewData, gatewayData] = await Promise.all([
      getSettings({ signal: controller.signal }),
      getOverview({ signal: controller.signal }),
      getGatewayOverview({ signal: controller.signal }),
    ])
    if (!mounted || controller.signal.aborted) {
      return
    }
    Object.assign(settings, settingsData)
    overview.value = overviewData
    gatewayOverview.value = gatewayData
    if (showToast) {
      toast.success('服务状态已刷新。')
    }
  } catch (err) {
    if (isAbortError(err) || !mounted) {
      return
    }
    toast.error(err instanceof Error ? err.message : '刷新失败')
  } finally {
    if (refreshController === controller) {
      refreshController = null
    }
    refreshInFlight = false
    if (mounted) {
      loading.value = false
    }
  }
}

function signOut() {
  logout()
  router.push('/login')
}

onMounted(async () => {
  mounted = true
  await refreshAll()
  if (!mounted) {
    return
  }
  refreshTimer = window.setInterval(() => refreshAll(), 30_000)
})

onBeforeUnmount(() => {
  mounted = false
  refreshController?.abort()
  refreshController = null
  if (refreshTimer !== null) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }
})
</script>

<template>
  <div class="desktop-console">
    <header class="desktop-topbar">
      <div class="desktop-brand">
        <div class="desktop-brand__mark">签</div>
        <div>
          <strong>爱签网关服务控制台</strong>
          <span>ai-sign-in-gateway</span>
        </div>
      </div>

      <a-space wrap class="desktop-actions">
        <a-button type="primary" @click="openUrl(adminUrl)">
          <template #icon><GlobalOutlined /></template>
          打开管理中心
        </a-button>
        <a-button @click="openUrl(gatewayApiUrl)">
          <template #icon><ApiOutlined /></template>
          打开网关
        </a-button>
        <a-button :loading="loading" @click="refreshAll(true)">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
        <a-button @click="signOut">
          <template #icon><PoweroffOutlined /></template>
          退出登录
        </a-button>
      </a-space>
    </header>

    <main class="desktop-main">
      <section class="desktop-hero">
        <div class="summary-grid">
          <div
            v-for="item in summaryItems"
            :key="item.key"
            class="summary-tile"
            :class="`summary-tile--${item.tone}`"
          >
            <span class="summary-tile__icon">
              <component :is="item.icon" />
            </span>
            <span class="summary-tile__label">{{ item.label }}</span>
            <strong>{{ item.value }}</strong>
          </div>
        </div>

        <div class="service-strip">
          <div
            v-for="item in serviceItems"
            :key="item.key"
            class="service-line"
            :class="{ 'service-line--ok': item.ok }"
          >
            <span class="service-line__dot"></span>
            <div>
              <strong>{{ item.label }}</strong>
              <p>{{ item.value }}</p>
            </div>
            <em>{{ item.meta }}</em>
            <a-button
              v-if="item.key === 'gateway'"
              class="service-line__copy"
              size="small"
              type="text"
              title="复制网关地址"
              @click="copyUrl(gatewayApiUrl)"
            >
              <template #icon><CopyOutlined /></template>
            </a-button>
          </div>
        </div>
      </section>

      <section v-if="runtimeProblems.length" class="desktop-alerts">
        <a-alert
          type="warning"
          show-icon
          :message="runtimeProblems.join(' / ')"
        />
      </section>

      <section class="desktop-settings-section">
        <div class="section-heading">
          <SettingOutlined />
          <div>
            <strong>服务设置</strong>
            <span>调度、运行、数据库、配置目录与账号</span>
          </div>
        </div>
        <SettingsView />
      </section>
    </main>
  </div>
</template>

<style scoped>
.desktop-console {
  min-height: 100vh;
  background:
    linear-gradient(135deg, rgba(15, 118, 110, 0.08), transparent 34%),
    linear-gradient(315deg, rgba(245, 158, 11, 0.11), transparent 30%),
    #f4f7f9;
  color: #1f2a44;
}

.desktop-topbar {
  position: sticky;
  top: 0;
  z-index: 20;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 16px;
  min-height: 68px;
  padding: 12px 20px;
  border-bottom: 1px solid rgba(151, 161, 179, 0.34);
  background: rgba(248, 250, 252, 0.92);
  backdrop-filter: blur(16px);
}

.desktop-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.desktop-brand__mark {
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  border-radius: 8px;
  background: linear-gradient(145deg, #eff6ff, #dbeafe);
  color: #1d4ed8;
  font-weight: 800;
  letter-spacing: 0.08em;
}

.desktop-brand strong,
.section-heading strong {
  display: block;
  color: #172033;
  font-size: 15px;
  line-height: 1.2;
}

.desktop-brand span,
.section-heading span,
.service-line p {
  color: #647084;
  font-size: 12px;
}

.desktop-main {
  display: grid;
  gap: 14px;
  width: min(1320px, 100%);
  margin: 0 auto;
  padding: 18px;
}

.desktop-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, 0.72fr);
  gap: 14px;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.summary-tile,
.service-strip,
.desktop-alerts,
.desktop-settings-section {
  border: 1px solid rgba(203, 213, 225, 0.72);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.86);
  box-shadow: 0 12px 34px rgba(35, 48, 68, 0.08);
}

.summary-tile {
  position: relative;
  min-height: 116px;
  padding: 16px;
  overflow: hidden;
}

.summary-tile::after {
  position: absolute;
  right: -22px;
  bottom: -28px;
  width: 98px;
  height: 98px;
  border: 1px solid currentColor;
  border-radius: 50%;
  content: '';
  opacity: 0.18;
}

.summary-tile__icon {
  position: absolute;
  right: 13px;
  bottom: 13px;
  z-index: 1;
  display: grid;
  place-items: center;
  width: 46px;
  height: 46px;
  color: currentColor;
  font-size: 42px;
  opacity: 0.5;
}

.summary-tile__label {
  position: relative;
  z-index: 2;
  display: block;
  color: #5f6c7e;
  font-size: 12px;
  font-weight: 700;
}

.summary-tile strong {
  position: relative;
  z-index: 2;
  display: block;
  margin-top: 5px;
  color: #152034;
  font-size: 27px;
  line-height: 1.1;
  overflow-wrap: anywhere;
}

.summary-tile--blue {
  color: #2563eb;
  background: linear-gradient(135deg, #ffffff, #eaf2ff);
}

.summary-tile--green {
  color: #0f8f62;
  background: linear-gradient(135deg, #ffffff, #e7f7ef);
}

.summary-tile--slate {
  color: #475569;
  background: linear-gradient(135deg, #ffffff, #eef2f7);
}

.summary-tile--amber {
  color: #b45309;
  background: linear-gradient(135deg, #ffffff, #fff3d6);
}

.service-strip {
  display: grid;
  align-content: stretch;
  gap: 0;
  overflow: hidden;
}

.service-line {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 10px;
  min-height: 72px;
  padding: 12px 14px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.88);
}

.service-line:last-child {
  border-bottom: 0;
}

.service-line__dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  background: #ef4444;
  box-shadow: 0 0 0 4px rgba(239, 68, 68, 0.12);
}

.service-line--ok .service-line__dot {
  background: #16a34a;
  box-shadow: 0 0 0 4px rgba(22, 163, 74, 0.12);
}

.service-line strong,
.service-line p {
  display: block;
  margin: 0;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.service-line em {
  color: #334155;
  font-size: 12px;
  font-style: normal;
  font-weight: 700;
}

.service-line__copy {
  display: inline-grid;
  place-items: center;
  width: 28px;
  height: 28px;
  color: #0f766e;
}

.desktop-alerts {
  display: grid;
  gap: 8px;
  padding: 10px;
}

.desktop-settings-section {
  min-width: 0;
  padding: 14px;
}

.section-heading {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}

.section-heading > .anticon {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: #e7f1ef;
  color: #0f766e;
}

@media (max-width: 1080px) {
  .desktop-hero {
    grid-template-columns: 1fr;
  }

  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 760px) {
  .desktop-topbar {
    grid-template-columns: 1fr;
    align-items: start;
  }

  .desktop-topbar {
    position: static;
  }

  .desktop-actions,
  .desktop-actions :deep(.ant-space-item),
  .desktop-actions :deep(.ant-btn) {
    width: 100%;
  }

  .summary-grid {
    grid-template-columns: 1fr;
  }
}
</style>
