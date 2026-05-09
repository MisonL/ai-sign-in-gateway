<script setup lang="ts">
import { PlusOutlined, CopyOutlined, ReloadOutlined, SettingOutlined, SyncOutlined, InfoCircleOutlined, QuestionCircleOutlined, HistoryOutlined, ToolOutlined, MoreOutlined } from '@ant-design/icons-vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref, type ComponentPublicInstance } from 'vue'
import {
  createSite,
  diagnoseGatewayRoute,
  disableAllGatewayRoutes,
  enableOnlyGatewayRoute,
  getGatewayActiveRequests,
  getGatewayLogs,
  getGatewayOverview,
  getGatewayRouteLogs,
  getGatewayUsage,
  probeGatewayRoute,
  getGatewayRoutes,
  getGatewaySettings,
  getSiteGroups,
  probeGatewayRouteBalance,
  refreshSiteSummaries,
  reorderGatewayRoutePriorities,
  resetGatewayRouteCircuit,
  syncGatewayRoutes,
  toggleGatewayRoute,
  updateGatewayRouteType,
  updateGatewaySettings,
} from '../api'
import ShellLayout from '../components/ShellLayout.vue'
import StatusPill from '../components/StatusPill.vue'
import { useDebouncedTask } from '../composables/useDebouncedTask'
import { useTableScrollHeights } from '../composables/useTableScrollHeights'
import { balanceTone, formatBalance, formatGroupNames, normalizeBalanceUnit, normalizeGroupNames, parseGroupNames } from '../format'
import { applyGatewayActiveConcurrency } from '../gatewayRouteConcurrency'
import { useToast } from '../toast'
import type { BalanceProbeResult, GatewayActiveRequest, GatewayLog, GatewayOverview, GatewayRoute, GatewayRouteDiagnosis, GatewayRouteProbeResult, GatewaySettingsData, GatewayStrategyStat, GatewayUsage, GatewayUsageRoute, SiteGroup, SiteSummary } from '../types'

const props = withDefaults(
  defineProps<{
    section?: 'routes' | 'monitor'
  }>(),
  {
    section: 'routes',
  },
)

const isRouteManagement = computed(() => props.section === 'routes')
const isGatewayMonitor = computed(() => props.section === 'monitor')
const toast = useToast()
const loading = ref(false)
const autoRefreshing = ref(false)
const activeRequestsRefreshing = ref(false)
const probeLoading = ref(false)
const settingsLoading = ref(false)
const settingsOpen = ref(false)
const logsDrawerOpen = ref(false)
const routeLogsDrawerOpen = ref(false)
const addUpstreamOpen = ref(false)
const addUpstreamLoading = ref(false)
const priorityDialogOpen = ref(false)
const priorityDialogLoading = ref(false)
const priorityRoute = ref<GatewayRoute | null>(null)
const priorityInsertIndex = ref<number | undefined>(undefined)
type ApiFormatOption = 'openai' | 'anthropic' | 'gemini' | 'general'
const addUpstreamForm = reactive<{
  name: string
  base_url: string
  api_key: string
  api_format: ApiFormatOption
  group_name: string
  preferred_model: string
  supported_models: string[]
}>({
  name: '',
  base_url: '',
  api_key: '',
  api_format: 'openai',
  group_name: '',
  preferred_model: '',
  supported_models: [],
})
const routeModelsDialogOpen = ref(false)
const routeModelsDialogSaving = ref(false)
const routeModelsDialogRoute = ref<GatewayRoute | null>(null)
const routeModelsDialogValue = ref<string[]>([])

function resetAddUpstreamForm() {
  addUpstreamForm.name = ''
  addUpstreamForm.base_url = ''
  addUpstreamForm.api_key = ''
  addUpstreamForm.api_format = 'openai'
  addUpstreamForm.group_name = ''
  addUpstreamGroupNames.value = []
  addUpstreamForm.preferred_model = ''
  addUpstreamForm.supported_models = []
}

async function submitAddUpstream() {
  const name = addUpstreamForm.name.trim()
  const baseUrl = addUpstreamForm.base_url.trim()
  const apiKey = addUpstreamForm.api_key.trim()
  if (!name || !baseUrl || !apiKey) {
    toast.error('名称 / Base URL / API Key 都需要填写。')
    return
  }
  if (!/^https?:\/\//i.test(baseUrl)) {
    toast.error('Base URL 必须以 http:// 或 https:// 开头。')
    return
  }
  addUpstreamLoading.value = true
  try {
    await createSite({
      name,
      base_url: baseUrl,
      plugin_key: 'api-supplier',
      group_name: normalizeGroupNames(addUpstreamGroupNames.value.length ? addUpstreamGroupNames.value : addUpstreamForm.group_name),
      supported_models: normalizeModelList(addUpstreamForm.supported_models),
      is_enabled: true,
      notes: '',
      credentials: {
        account: '',
        api_key: apiKey,
      },
      plugin_config: {
        api_format: addUpstreamForm.api_format,
        endpoint_url: '',
        preferred_model: addUpstreamForm.preferred_model.trim(),
      },
    })
    toast.success(`已添加上游「${name}」，可在路由池中调整 priority/weight。`)
    addUpstreamOpen.value = false
    resetAddUpstreamForm()
    await handleSync()
    await loadData()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '添加失败')
  } finally {
    addUpstreamLoading.value = false
  }
}
const overview = ref<GatewayOverview | null>(null)
const routes = ref<GatewayRoute[]>([])
const priorityRoutes = ref<GatewayRoute[]>([])
const logs = ref<GatewayLog[]>([])
const activeRequests = ref<GatewayActiveRequest[]>([])
const routeLogs = ref<GatewayLog[]>([])
const gatewayUsage = ref<GatewayUsage | null>(null)
const usageLoading = ref(false)
const usageRange = reactive({
  start: '',
  end: '',
})
const routeDiagnosis = ref<GatewayRouteDiagnosis | null>(null)
const siteGroups = ref<SiteGroup[]>([])
const routeLogsLoading = ref(false)
const routeDiagnosisLoading = ref(false)
const routeLogsRoute = ref<GatewayRoute | null>(null)
const routeDiagnosisOpen = ref(false)
const includeDisabled = ref(true)
let autoRefreshTimer: number | null = null
let activeRequestRefreshTimer: number | null = null
let lastAutoRefreshAt = 0
let lastActiveRequestRefreshAt = 0
const gatewayTablePageSize = 20
const gatewayRouteAutoRefreshMs = 60_000
const gatewayMonitorAutoRefreshMs = 60_000
const gatewayActiveRequestRefreshMs = 1_000
const selectedGroups = ref<string[]>([])
const addUpstreamGroupNames = ref<string[]>([])
const selectedRouteTypes = ref<Array<GatewayRoute['route_type']>>([])
const selectedIssueStates = ref<Array<'with_error' | 'without_error'>>([])
const probingRouteIds = ref<number[]>([])
const balanceProbingRouteIds = ref<number[]>([])
type RouteBatchProgress = { total: number; done: number; success: number; failed: number }
const probeAllProgress = ref<RouteBatchProgress | null>(null)
const balanceProbeAllLoading = ref(false)
const balanceProbeAllProgress = ref<RouteBatchProgress | null>(null)
const balanceProbeManualOpen = ref(false)
const balanceProbeManualLoading = ref(false)
const balanceProbeManualRoute = ref<GatewayRoute | null>(null)
const balanceProbeManualURL = ref('')
const balanceProbeManualMessage = ref('')
const routeSearch = ref('')
const logSearch = ref('')
const routeLogSearch = ref('')
const { pageTableY, pageTableContainer, drawerTableY } = useTableScrollHeights()

function bindPageTableContainer(element: Element | ComponentPublicInstance | null) {
  pageTableContainer.value = element instanceof HTMLElement ? element : null
}

const gatewayRequestUrl = computed(() => {
  const apiBase = String(import.meta.env.VITE_API_BASE || '/api').trim()
  if (!apiBase) {
    return `${window.location.origin}/api/gateway`
  }

  try {
    const normalizedApiBase = apiBase.startsWith('http')
      ? apiBase
      : new URL(apiBase, window.location.origin).toString()
    return new URL('./gateway', normalizedApiBase.endsWith('/') ? normalizedApiBase : `${normalizedApiBase}/`).toString()
  } catch {
    return `${window.location.origin}/api/gateway`
  }
})

const codexGatewayRequestUrl = computed(() => `${gatewayRequestUrl.value.replace(/\/$/, '')}/v1`)
const codexGatewayTooltip = computed(
  () => `Codex CLI 的 Base URL 需要使用 ${codexGatewayRequestUrl.value}，也就是在网关地址后追加 /v1。`,
)

const settingsForm = reactive<GatewaySettingsData>({
  route_strategy: 'round_robin',
  failure_threshold: 3,
  cooldown_seconds: 180,
  request_timeout: 60,
  max_attempts: 0,
  failure_retry_mode: 'retryable',
  route_concurrency_limit: 5,
  concurrency_transfer_strategy: 'limit_only',
  concurrency_overflow_strategy: 'latency_first',
  smart_latency_bias: 1.0,
  smart_concurrency_bias: 1.5,
  smart_failure_bias: 1.0,
  smart_priority_bias: 0.5,
  gateway_api_key: '',
})

const maskedGatewayApiKey = computed(() => {
  const value = settingsForm.gateway_api_key.trim()
  if (!value) {
    return '未配置 GATEWAY_API_KEY'
  }
  if (value.length <= 12) {
    return '*'.repeat(value.length)
  }
  return `${value.slice(0, 6)}...${value.slice(-6)}`
})

const routeColumns = [
  { title: '路由', key: 'route', width: 240, sorter: (a: GatewayRoute, b: GatewayRoute) => loadRouteLabel(a).localeCompare(loadRouteLabel(b), 'zh-CN') },
  { title: '类型', key: 'type', width: 130, sorter: (a: GatewayRoute, b: GatewayRoute) => a.route_type.localeCompare(b.route_type, 'zh-CN') },
  { title: '余额', key: 'balance', width: 160, sorter: (a: GatewayRoute, b: GatewayRoute) => (a.last_balance ?? -Infinity) - (b.last_balance ?? -Infinity) },
  { title: '分组', key: 'group', width: 110, sorter: (a: GatewayRoute, b: GatewayRoute) => String(a.group_name ?? '').localeCompare(String(b.group_name ?? ''), 'zh-CN') },
  { title: '优先级', key: 'priority', width: 90, sorter: (a: GatewayRoute, b: GatewayRoute) => a.route_priority - b.route_priority },
  { title: '权重', key: 'weight', width: 80, sorter: (a: GatewayRoute, b: GatewayRoute) => a.weight - b.weight },
  { title: '并发/最大转移', key: 'concurrency', width: 150, sorter: (a: GatewayRoute, b: GatewayRoute) => a.active_concurrency - b.active_concurrency },
  { title: '成功率', key: 'success_rate', width: 110, sorter: (a: GatewayRoute, b: GatewayRoute) => a.success_rate - b.success_rate },
  { title: '延迟', key: 'latency', width: 138, sorter: (a: GatewayRoute, b: GatewayRoute) => (a.last_latency_ms ?? a.avg_latency_ms ?? Infinity) - (b.last_latency_ms ?? b.avg_latency_ms ?? Infinity) },
  { title: '最后异常', key: 'error', width: 220, sorter: (a: GatewayRoute, b: GatewayRoute) => new Date(routeLastUpdateTime(a) ?? 0).getTime() - new Date(routeLastUpdateTime(b) ?? 0).getTime() },
  { title: '操作', key: 'actions', width: 136, fixed: 'right' as const },
]

const priorityDialogColumns = [
  { title: '路由名称', key: 'route', width: 280 },
  { title: '优先级', key: 'priority', width: 90 },
  { title: '分组', key: 'group', width: 170 },
]

const routeTypeOptions: Array<{ label: string; value: GatewayRoute['route_type'] }> = [
  { label: 'Claude', value: 'claude' },
  { label: 'GPT', value: 'codex' },
  { label: 'Gemini', value: 'gemini' },
]

const routeTypeFilterOptions: Array<{ label: string; value: GatewayRoute['route_type'] }> = [
  { label: 'Claude', value: 'claude' },
  { label: 'GPT', value: 'codex' },
  { label: 'Gemini', value: 'gemini' },
]

const issueStateOptions: Array<{ label: string; value: 'with_error' | 'without_error' }> = [
  { label: '有异常', value: 'with_error' },
  { label: '无异常', value: 'without_error' },
]

const logColumns = [
  { title: '时间', key: 'created_at', width: 180, sorter: (a: GatewayLog, b: GatewayLog) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime() },
  { title: '请求', key: 'request', width: 210, sorter: (a: GatewayLog, b: GatewayLog) => `${a.method} ${a.target_path}`.localeCompare(`${b.method} ${b.target_path}`, 'zh-CN') },
  { title: '路由', key: 'route', width: 300, sorter: (a: GatewayLog, b: GatewayLog) => logRouteLabel(a).localeCompare(logRouteLabel(b), 'zh-CN') },
  { title: '模型', key: 'model', width: 260, sorter: (a: GatewayLog, b: GatewayLog) => logModelMeta(a).localeCompare(logModelMeta(b), 'zh-CN') },
  { title: '结果', key: 'status', width: 120, sorter: (a: GatewayLog, b: GatewayLog) => Number(a.success) - Number(b.success) },
  { title: '延迟', key: 'latency', width: 100, sorter: (a: GatewayLog, b: GatewayLog) => (a.latency_ms ?? Infinity) - (b.latency_ms ?? Infinity) },
  { title: '尝试', key: 'attempt', width: 90, sorter: (a: GatewayLog, b: GatewayLog) => a.attempt_index - b.attempt_index },
  { title: '说明', key: 'reason' },
]

const usageColumns = [
  { title: '路由', key: 'route', width: 300, sorter: (a: GatewayUsageRoute, b: GatewayUsageRoute) => usageRouteLabel(a).localeCompare(usageRouteLabel(b), 'zh-CN') },
  { title: '请求', key: 'requests', width: 90, sorter: (a: GatewayUsageRoute, b: GatewayUsageRoute) => a.request_count - b.request_count },
  { title: '成功率', key: 'success_rate', width: 100, sorter: (a: GatewayUsageRoute, b: GatewayUsageRoute) => a.success_rate - b.success_rate },
  { title: '流式', key: 'stream', width: 80, sorter: (a: GatewayUsageRoute, b: GatewayUsageRoute) => a.stream_request_count - b.stream_request_count },
  { title: '输入', key: 'prompt_tokens', width: 110, sorter: (a: GatewayUsageRoute, b: GatewayUsageRoute) => a.prompt_tokens - b.prompt_tokens },
  { title: '缓存输入', key: 'cached_input_tokens', width: 110, sorter: (a: GatewayUsageRoute, b: GatewayUsageRoute) => a.cached_input_tokens - b.cached_input_tokens },
  { title: '输出', key: 'completion_tokens', width: 130, sorter: (a: GatewayUsageRoute, b: GatewayUsageRoute) => a.completion_tokens - b.completion_tokens },
  { title: '总消耗', key: 'total_tokens', width: 120, sorter: (a: GatewayUsageRoute, b: GatewayUsageRoute) => a.total_tokens - b.total_tokens },
  { title: '模型费用', key: 'computed_total_cost', width: 120, sorter: (a: GatewayUsageRoute, b: GatewayUsageRoute) => a.computed_total_cost - b.computed_total_cost },
  { title: '平均延迟', key: 'avg_latency', width: 110, sorter: (a: GatewayUsageRoute, b: GatewayUsageRoute) => (a.avg_latency_ms ?? Infinity) - (b.avg_latency_ms ?? Infinity) },
  { title: '最后使用', key: 'last_used_at', width: 170, sorter: (a: GatewayUsageRoute, b: GatewayUsageRoute) => new Date(a.last_used_at ?? 0).getTime() - new Date(b.last_used_at ?? 0).getTime() },
]

type MetricTone = 'primary' | 'success' | 'warning' | 'info' | 'neutral'

const gatewayRouteStrategyOptions: Array<{ label: string; value: GatewaySettingsData['route_strategy']; description: string }> = [
  { label: '智能综合评分', value: 'smart', description: '综合延迟、当前并发、失败记录、优先级和权重，自动挑选当前最合适的路由。' },
  { label: '轮询均衡', value: 'round_robin', description: '在健康路由之间按顺序轮换，并尊重权重，适合希望请求分散到多个上游的场景。' },
  { label: '低延迟优先', value: 'latency_first', description: '优先使用历史延迟更低的路由，适合更看重响应速度的场景。' },
  { label: '优先级优先', value: 'priority', description: '优先选择 priority 数值更小的路由，再结合权重和健康状态排序，适合固定主备线路。' },
]

const gatewayOverflowStrategyOptions: Array<{ label: string; value: GatewaySettingsData['concurrency_overflow_strategy']; description: string }> = [
  { label: '低延迟优先', value: 'latency_first', description: '只有所有可用路由都达到转移阈值后才生效，溢出请求优先尝试延迟较低的路由。' },
  { label: '按顺序优先', value: 'sequential', description: '只有所有可用路由都达到转移阈值后才生效，溢出请求按当前策略顺序继续尝试。' },
]

const gatewayConcurrencyTransferOptions: Array<{ label: string; value: GatewaySettingsData['concurrency_transfer_strategy']; description: string }> = [
  { label: '并发达阈值转移', value: 'limit_only', description: '保持当前策略排序；某条路由达到最大转移阈值后，新请求会优先转到其他未达阈值路由。' },
  { label: '并发均衡转移', value: 'balance', description: '在未达阈值的候选路由中，优先使用当前并发更低的路由，让请求更主动地摊开。' },
]

const gatewayFailureRetryModeOptions: Array<{ label: string; value: GatewaySettingsData['failure_retry_mode']; description: string }> = [
  { label: '可重试错误', value: 'retryable', description: '网络错误、429、5xx 和首包前流式失败会切换路由；400/401/403 等参数或鉴权错误会在网关层停止。' },
  { label: '所有上游错误', value: 'all', description: '任意非 2xx 上游响应都会继续切换其他路由，适合希望最大化请求成功率的场景。' },
]

function balanceUnitOrder(unit: string) {
  const normalized = normalizeBalanceUnit(unit)
  if (normalized === '$') return 0
  if (normalized === '¥') return 1
  if (normalized === '€') return 2
  if (normalized === '£') return 3
  return 10
}

function routeBalanceUnit(route: GatewayRoute) {
  const display = String(route.balance_display ?? '').trim()
  if (/^[$¥€£]/.test(display)) {
    return display[0]
  }
  const suffix = display.match(/\s([^\s]+)$/)
  if (suffix) {
    return normalizeBalanceUnit(suffix[1])
  }
  return normalizeBalanceUnit(route.balance_unit)
}

const routeBalancesByUnit = computed(() => {
  const totals = new Map<string, number>()
  for (const route of routes.value) {
    if (route.last_balance === null || route.last_balance === undefined || Number.isNaN(route.last_balance)) {
      continue
    }
    const unit = routeBalanceUnit(route)
    totals.set(unit, (totals.get(unit) ?? 0) + Number(route.last_balance))
  }
  return totals
})

const routeTotalBalanceSummary = computed(() => {
  if (!routeBalancesByUnit.value.size) {
    return '暂无'
  }
  return [...routeBalancesByUnit.value.entries()]
    .sort(([left], [right]) => balanceUnitOrder(left) - balanceUnitOrder(right) || left.localeCompare(right, 'zh-CN'))
    .map(([unit, value]) => formatBalance(value, unit))
    .join(' / ')
})

const metricCards = computed<Array<{
  title: string
  value: string | number
  tone: MetricTone
}>>(() => {
  if (!overview.value) {
    return []
  }
  const ov = overview.value
  return [
    {
      title: '总额度',
      value: routeTotalBalanceSummary.value,
      tone: 'primary',
    },
    {
      title: '24H 请求',
      value: formatNumber(ov.request_count_24h),
      tone: 'info',
    },
    {
      title: '成功率',
      value: `${ov.success_rate_24h}%`,
      tone: 'success',
    },
    {
      title: '并发数',
      value: formatNumber(ov.active_concurrency),
      tone: 'neutral',
    },
    {
      title: '24H 模型费用',
      value: formatUSD(ov.usage_cost_24h?.total_cost),
      tone: ov.usage_cost_24h?.unknown_requests ? 'warning' : 'primary',
    },
  ]
})

const routePoolStatusCards = computed<Array<{
  key: string
  label: string
  value: number
  tone: 'success' | 'warning' | 'danger' | 'neutral'
  ratio: number
}>>(() => {
  if (!overview.value) {
    return []
  }
  const ov = overview.value
  const total = Math.max(ov.total_routes, 1)
  return [
    {
      key: 'healthy',
      label: '健康路由',
      value: ov.healthy_routes,
      tone: 'success',
      ratio: ov.healthy_routes / total,
    },
    {
      key: 'half_open',
      label: '半开探测',
      value: ov.half_open_routes,
      tone: 'warning',
      ratio: ov.half_open_routes / total,
    },
    {
      key: 'open',
      label: '熔断中',
      value: ov.open_circuit_routes,
      tone: 'danger',
      ratio: ov.open_circuit_routes / total,
    },
    {
      key: 'disabled',
      label: '停用路由',
      value: ov.disabled_routes,
      tone: 'neutral',
      ratio: ov.disabled_routes / total,
    },
  ]
})

const routePoolPreviewRoutes = computed(() =>
  routes.value
    .slice()
    .sort(
      (a, b) =>
        b.active_concurrency - a.active_concurrency ||
        b.request_count - a.request_count ||
        Number(Boolean(b.last_error)) - Number(Boolean(a.last_error)) ||
        a.route_priority - b.route_priority,
    )
    .slice(0, 5),
)

const gatewayStrategyCards = computed(() => {
  if (!overview.value?.strategy_breakdown_24h?.length) {
    return []
  }
  const sorted = overview.value.strategy_breakdown_24h
    .slice()
    .sort((a, b) => b.request_count - a.request_count)
  const peak = Math.max(...sorted.map((item) => item.request_count), 1)
  return sorted.slice(0, 4).map((item, index) => ({
    key: item.route_strategy,
    title: strategyLabel(item.route_strategy),
    value: formatNumber(item.request_count),
    width: `${Math.max(18, (item.request_count / peak) * 100)}%`,
    tone: (['primary', 'info', 'success', 'warning'] as MetricTone[])[index] ?? 'neutral',
  }))
})

function strategyLabel(strategy: GatewayStrategyStat['route_strategy']) {
  return gatewayRouteStrategyOptions.find((item) => item.value === strategy)?.label ?? strategy
}

function padDatePart(value: number) {
  return String(value).padStart(2, '0')
}

function toDatetimeLocalValue(date: Date) {
  return [
    date.getFullYear(),
    padDatePart(date.getMonth() + 1),
    padDatePart(date.getDate()),
  ].join('-') + `T${padDatePart(date.getHours())}:${padDatePart(date.getMinutes())}`
}

function datetimeLocalToISOString(value: string) {
  if (!value) {
    return ''
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toISOString()
}

function resetUsageRangeToToday() {
  const now = new Date()
  const start = new Date(now)
  start.setHours(0, 0, 0, 0)
  usageRange.start = toDatetimeLocalValue(start)
  usageRange.end = toDatetimeLocalValue(now)
}

function formatNumber(value: number | null | undefined) {
  const numeric = Number(value ?? 0)
  return Number.isFinite(numeric) ? numeric.toLocaleString('zh-CN') : '0'
}

function normalizeModelList(values: unknown): string[] {
  if (Array.isArray(values)) {
    return values
      .map((item) => String(item ?? '').trim())
      .filter((item, index, source) => item && source.indexOf(item) === index)
  }
  return String(values ?? '')
    .split(/[\n\r,，\t]+/)
    .map((item) => item.trim())
    .filter((item, index, source) => item && source.indexOf(item) === index)
}

function supportedModelsPreview(values: unknown, limit = 2) {
  const normalized = normalizeModelList(values)
  if (!normalized.length) {
    return '未声明'
  }
  const preview = normalized.slice(0, limit).join(' / ')
  return normalized.length > limit ? `${preview} 等 ${normalized.length} 个` : preview
}

function formatUSD(value: number | null | undefined) {
  const numeric = Number(value ?? 0)
  if (!Number.isFinite(numeric)) {
    return '$0'
  }
  return `$${numeric.toLocaleString('zh-CN', {
    minimumFractionDigits: numeric > 0 && numeric < 0.01 ? 6 : 2,
    maximumFractionDigits: numeric > 0 && numeric < 0.01 ? 6 : 2,
  })}`
}

function usageRowKey(record: GatewayUsageRoute) {
  return record.route_id ?? `${record.site_id ?? 'unknown'}-${record.key_fingerprint || record.route_label}`
}

function usageRouteLabel(route: GatewayUsageRoute) {
  const label = String(route.route_label ?? '').trim()
  if (label) {
    return label
  }
  const parts = [
    route.route_id ? `#${route.route_id}` : '',
    route.site_name || (route.site_id ? `站点 #${route.site_id}` : ''),
    route.key_name,
  ].filter(Boolean)
  return parts.length ? parts.join(' · ') : '未知路由'
}

function usageRouteMeta(route: GatewayUsageRoute) {
  return [
    route.route_id ? `Route #${route.route_id}` : 'Route 未知',
    route.model ? `模型 ${route.model}` : '',
    route.site_id ? `站点 #${route.site_id}` : '',
    route.route_type ? `类型 ${routeTypeLabel(route.route_type)}` : '',
    route.group_name ? `分组 ${formatGroupNames(route.group_name)}` : '',
    route.key_fingerprint ? `Key ${shortFingerprint(route.key_fingerprint)}` : '',
  ].filter(Boolean).join(' · ')
}

const usageSummaryCards = computed<Array<{ title: string; value: string; tone: MetricTone }>>(() => {
  if (!gatewayUsage.value) {
    return []
  }
  return [
    { title: '模型费用', value: formatUSD(gatewayUsage.value.computed_total_cost), tone: gatewayUsage.value.computed_cost_mixed ? 'warning' : 'primary' },
    { title: '时间段请求', value: formatNumber(gatewayUsage.value.request_count), tone: 'primary' },
    { title: '成功请求', value: formatNumber(gatewayUsage.value.success_count), tone: 'success' },
    { title: '总 Token', value: formatNumber(gatewayUsage.value.total_tokens), tone: 'info' },
  ]
})

const selectedRouteStrategyDescription = computed(() =>
  gatewayRouteStrategyOptions.find((item) => item.value === settingsForm.route_strategy)?.description ?? '',
)

const selectedOverflowStrategyDescription = computed(() =>
  gatewayOverflowStrategyOptions.find((item) => item.value === settingsForm.concurrency_overflow_strategy)?.description ?? '',
)

const selectedConcurrencyTransferDescription = computed(() =>
  gatewayConcurrencyTransferOptions.find((item) => item.value === settingsForm.concurrency_transfer_strategy)?.description ?? '',
)

const selectedFailureRetryModeDescription = computed(() =>
  gatewayFailureRetryModeOptions.find((item) => item.value === settingsForm.failure_retry_mode)?.description ?? '',
)

const routeConcurrencyLimitLabel = computed(() => {
  const limit = Number(settingsForm.route_concurrency_limit)
  return Number.isFinite(limit) && limit > 0 ? String(Math.trunc(limit)) : '不限'
})

const gatewayStrategyDescriptionItems = computed(() => [
  { label: '路由策略', value: selectedRouteStrategyDescription.value },
  { label: '并发转移', value: selectedConcurrencyTransferDescription.value },
  { label: '并发溢出', value: selectedOverflowStrategyDescription.value },
  { label: '错误切换', value: selectedFailureRetryModeDescription.value },
  { label: '自动模型类型', value: '请求体里的 model 包含 claude / gpt / gemini 时，网关会自动选择对应类型路由；仍可用 type 参数手动指定。' },
])

const groupOptions = computed(() => {
  const labels = new Set<string>()
  siteGroups.value.forEach((group) => labels.add(group.name))
  routes.value.forEach((route) => {
    parseGroupNames(route.group_name).forEach((groupName) => labels.add(groupName))
  })
  selectedGroups.value.forEach((groupName) => labels.add(groupName))
  addUpstreamGroupNames.value.forEach((groupName) => labels.add(groupName))
  return [...labels]
    .sort((a, b) => a.localeCompare(b, 'zh-CN'))
    .map((groupName) => ({
      label: groupName,
      value: groupName,
    }))
})

function formatTime(value: string | null) {
  if (!value) return '暂无'
  return new Date(value).toLocaleString('zh-CN')
}

function loadRouteLabel(route: GatewayRoute) {
  const siteName = String(route.site_name || route.site_name_snapshot || route.site_base_url_snapshot || `站点 #${route.site_id}`).trim()
  return `${siteName}${route.key_name ? ` · ${route.key_name}` : ''}`
}

function routePriorityLabel(route: GatewayRoute | null) {
  return route ? String(route.route_priority) : '暂无'
}

function routeSnapshotLabel(route: GatewayRoute) {
  return route.site_name_snapshot || route.site_base_url_snapshot || ''
}

function routeKeyLabel(route: GatewayRoute) {
  return route.key_fingerprint ? `Key ${route.key_fingerprint}` : 'Key 未知'
}

function routeIssueLabels(route: GatewayRoute) {
  const labels: string[] = []
  if (route.site_missing) {
    labels.push('站点已删除')
  }
  if (route.has_api_key === false) {
    labels.push('缺少 API Key')
  }
  return labels
}

function routeDetailItems(route: GatewayRoute) {
  return [
    { label: '快照', value: routeSnapshotLabel(route) || '未记录' },
    { label: '当前出口', value: route.request_base_url || route.base_url || route.site_base_url_snapshot || '未记录' },
    { label: '出口候选', value: routeRequestBasePreview(route) },
    { label: '余额接口', value: route.balance_probe_url || '自动探测' },
    { label: '支持模型', value: supportedModelsPreview(route.supported_models, 3) },
    { label: '站点', value: `#${route.site_id}` },
    { label: 'Key', value: routeKeyLabel(route) },
  ]
}

function routeTypeLabel(routeType: string) {
  return routeTypeOptions.find((item) => item.value === routeType)?.label ?? routeType
}

function activeRequestRouteTypeLabel(routeType: GatewayActiveRequest['route_type']) {
  const normalized = String(routeType ?? '').trim()
  return routeTypeOptions.find((item) => item.value === normalized)?.label ?? (normalized || '未知')
}

function isRouteTypeFilterActive(routeType: GatewayRoute['route_type']) {
  return selectedRouteTypes.value.includes(routeType)
}

function toggleRouteTypeFilter(routeType: GatewayRoute['route_type']) {
  if (isRouteTypeFilterActive(routeType)) {
    selectedRouteTypes.value = selectedRouteTypes.value.filter((item) => item !== routeType)
    return
  }
  selectedRouteTypes.value = [...selectedRouteTypes.value, routeType]
}

function clearRouteTypeFilter() {
  selectedRouteTypes.value = []
}

function routeCircuitState(route: GatewayRoute): 'closed' | 'open' | 'half_open' | 'paused' {
  if (!route.is_enabled) {
    return 'paused'
  }
  if (route.circuit_state === 'open' || route.circuit_state === 'half_open') {
    return route.circuit_state
  }
  return 'closed'
}

function hasIssue(route: GatewayRoute) {
  return Boolean(String(route.last_error ?? '').trim())
}

function routeLastUpdateTime(route: GatewayRoute) {
  if (hasIssue(route)) {
    return route.last_failure_at || route.last_used_at || route.last_success_at
  }
  return route.last_success_at || route.last_used_at || route.last_failure_at
}

function routeLastUpdateLabel(route: GatewayRoute) {
  if (hasIssue(route)) {
    return '异常'
  }
  if (route.last_success_at) {
    return '成功'
  }
  if (route.last_used_at) {
    return '使用'
  }
  return '更新'
}

function asRoute(record: unknown) {
  return record as GatewayRoute
}

function normalizeGatewayRoute(route: GatewayRoute): GatewayRoute {
  const balanceUnit = normalizeBalanceUnit(route.balance_unit)
  return {
    ...route,
    balance_unit: balanceUnit,
    balance_display: route.balance_display || formatBalance(route.last_balance, balanceUnit),
    package_unit: normalizeBalanceUnit(route.package_unit, ''),
    supported_models: normalizeModelList(route.supported_models),
  }
}

function applyActiveRequestSnapshot(items: GatewayActiveRequest[]) {
  routes.value = applyGatewayActiveConcurrency(routes.value, items)
  priorityRoutes.value = applyGatewayActiveConcurrency(priorityRoutes.value, items)
  if (overview.value) {
    const activeConcurrency = items.length
    if (overview.value.active_concurrency !== activeConcurrency) {
      overview.value = {
        ...overview.value,
        active_concurrency: activeConcurrency,
      }
    }
  }
}

function asLog(record: unknown) {
  return record as GatewayLog
}

function routeRowKey(record: GatewayRoute) {
  return record.id
}

function priorityRouteRowClassName(record: GatewayRoute) {
  return record.id === priorityRoute.value?.id ? 'priority-route-row priority-route-row--current' : 'priority-route-row'
}

function logRowKey(record: GatewayLog) {
  return record.id
}

function shortFingerprint(value: string | null | undefined) {
  const raw = String(value ?? '').trim()
  if (!raw) {
    return ''
  }
  return raw.length <= 10 ? raw : `${raw.slice(0, 6)}...${raw.slice(-4)}`
}

function logRouteLabel(log: GatewayLog) {
  const label = String(log.route_label ?? '').trim()
  if (label) {
    return label
  }
  const parts = [
    log.route_id ? `#${log.route_id}` : '',
    log.site_name || (log.site_id ? `站点 #${log.site_id}` : ''),
    log.key_name,
  ].filter(Boolean)
  return parts.length ? parts.join(' · ') : '未知路由'
}

function logRouteMeta(log: GatewayLog) {
  const values = [
    log.route_id ? `Route #${log.route_id}` : 'Route 未知',
    log.site_id ? `站点 #${log.site_id}` : '',
    log.key_fingerprint ? `Key ${shortFingerprint(log.key_fingerprint)}` : '',
  ].filter(Boolean)
  return values.join(' · ')
}

function logRequestedModel(log: GatewayLog) {
  return String(log.requested_model || log.model || '').trim() || '未声明'
}

function logActualModel(log: GatewayLog) {
  return String(log.actual_model || log.requested_model || log.model || '').trim() || '未记录'
}

function logModelMeta(log: GatewayLog) {
  return `请求 ${logRequestedModel(log)} · 命中 ${logActualModel(log)}`
}

function activeRequestRouteLabel(item: GatewayActiveRequest) {
  const label = String(item.route_label ?? '').trim()
  if (label) {
    return label
  }
  const parts = [
    item.route_id ? `#${item.route_id}` : '',
    item.site_name || (item.site_id ? `站点 #${item.site_id}` : ''),
    item.key_name,
  ].filter(Boolean)
  return parts.length ? parts.join(' · ') : '未知路由'
}

function activeRequestMeta(item: GatewayActiveRequest) {
  return [
    item.route_id ? `Route #${item.route_id}` : 'Route 未知',
    item.site_id ? `站点 #${item.site_id}` : '',
    item.key_fingerprint ? `Key ${shortFingerprint(item.key_fingerprint)}` : '',
    item.request_base_url,
  ].filter(Boolean)
}

function formatElapsed(ms: number | null | undefined) {
  if (ms === null || ms === undefined || Number.isNaN(ms)) {
    return '0 ms'
  }
  if (ms < 1000) {
    return `${ms} ms`
  }
  if (ms < 60_000) {
    return `${Math.round(ms / 100) / 10} s`
  }
  const minutes = Math.floor(ms / 60_000)
  const seconds = Math.floor((ms % 60_000) / 1000)
  return `${minutes}m ${seconds}s`
}

const activeRouteFeed = computed(() =>
  activeRequests.value.map((item) => ({
    ...item,
    kind: 'active',
    label: activeRequestRouteLabel(item),
    meta: activeRequestMeta(item),
    elapsedLabel: formatElapsed(item.elapsed_ms),
    routeTypeLabel: activeRequestRouteTypeLabel(item.route_type),
    requestedModelLabel: String(item.requested_model || '').trim() || '未声明',
    actualModelLabel: String(item.actual_model || item.requested_model || '').trim() || '待返回',
    groupLabel: formatGroupNames(item.group_name) || '未分组',
    targetLabel: `${item.method} ${item.target_path || '/'}`,
    strategyLabel: strategyLabel(item.route_strategy as GatewayStrategyStat['route_strategy']),
    primaryBadge: `并发 ${item.active_concurrency}`,
    primaryBadgeColor: 'processing',
    secondaryBadge: formatElapsed(item.elapsed_ms),
    attemptLabel: `尝试 ${item.attempt_index}`,
    timeLabel: `开始 ${formatTime(item.started_at)}`,
  })),
)

const recentRouteFeed = computed(() =>
  logs.value.slice(0, 8).map((item) => ({
    id: `log-${item.id}`,
    kind: 'completed',
    label: logRouteLabel(item),
    meta: [
      item.route_id ? `Route #${item.route_id}` : 'Route 未知',
      item.site_id ? `站点 #${item.site_id}` : '',
      item.key_fingerprint ? `Key ${shortFingerprint(item.key_fingerprint)}` : '',
    ].filter(Boolean),
    elapsedLabel: item.latency_ms !== null ? `${item.latency_ms} ms` : '暂无延迟',
    routeTypeLabel: '',
    requestedModelLabel: logRequestedModel(item),
    actualModelLabel: logActualModel(item),
    groupLabel: formatGroupNames(item.group_name) || '未分组',
    targetLabel: `${item.method} ${item.target_path || '/'}`,
    strategyLabel: strategyLabel(item.route_strategy as GatewayStrategyStat['route_strategy']),
    primaryBadge: item.success ? '成功' : '失败',
    primaryBadgeColor: item.success ? 'success' : 'error',
    secondaryBadge: item.latency_ms !== null ? `${item.latency_ms} ms` : '暂无延迟',
    attemptLabel: `尝试 ${item.attempt_index}`,
    timeLabel: `完成 ${formatTime(item.created_at)}`,
    is_stream: item.is_stream,
  })),
)

const routeActivityFeed = computed(() => [...activeRouteFeed.value, ...recentRouteFeed.value].slice(0, 12))

function balanceClass(balance: number | null | undefined) {
  const tone = balanceTone(balance)
  return tone === 'empty' ? '' : `balance-value balance-value--${tone}`
}

function primaryLatency(route: GatewayRoute) {
  return route.ewma_latency_ms ?? route.last_latency_ms ?? route.avg_latency_ms ?? null
}

function latencyTone(latencyMs: number | null | undefined): 'low' | 'medium' | 'high' | 'empty' {
  if (latencyMs === null || latencyMs === undefined || Number.isNaN(latencyMs)) {
    return 'empty'
  }
  if (latencyMs < 1200) {
    return 'low'
  }
  if (latencyMs < 3200) {
    return 'medium'
  }
  return 'high'
}

function latencyClass(latencyMs: number | null | undefined) {
  const tone = latencyTone(latencyMs)
  return tone === 'empty' ? 'gateway-latency' : `gateway-latency gateway-latency--${tone}`
}

function formatLatency(latencyMs: number | null | undefined) {
  if (latencyMs === null || latencyMs === undefined || Number.isNaN(latencyMs)) {
    return '暂无'
  }
  return `${latencyMs} ms`
}

function routeRequestBaseList(route: GatewayRoute): string[] {
  const raw = [
    ...(route.request_base_urls ?? []),
    route.request_base_url,
    route.base_url,
    route.site_base_url_snapshot,
  ]
  return raw
    .map((item) => String(item ?? '').trim())
    .filter((item, index, source) => item && source.indexOf(item) === index)
}

function routeRequestBasePreview(route: GatewayRoute): string {
  const urls = routeRequestBaseList(route)
  if (!urls.length) {
    return '未记录'
  }
  if (urls.length === 1) {
    return urls[0]
  }
  const preview = urls.slice(0, 2).join(' -> ')
  return urls.length > 2 ? `${preview} 等 ${urls.length} 个` : preview
}

function compactText(value: string | null | undefined, limit = 36) {
  const text = String(value ?? '').trim()
  if (!text) {
    return ''
  }
  return text.length > limit ? `${text.slice(0, limit)}...` : text
}

function routeLatencyDetails(route: GatewayRoute) {
  const items: string[] = []
  if (route.last_latency_ms !== null) {
    items.push(`最近 ${route.last_latency_ms} ms`)
  }
  if (route.ewma_latency_ms !== null) {
    items.push(`EWMA ${route.ewma_latency_ms} ms`)
  }
  if (route.avg_latency_ms !== null) {
    items.push(`均值 ${route.avg_latency_ms} ms`)
  }
  return items
}

function routeErrorDetails(route: GatewayRoute) {
  const items: string[] = []
  if (route.last_error) {
    items.push(route.last_error)
  }
  if (routeLastUpdateTime(route)) {
    items.push(`${routeLastUpdateLabel(route)}：${formatTime(routeLastUpdateTime(route))}`)
  }
  return items
}

function includesSearch(values: Array<string | number | boolean | null | undefined>, keyword: string) {
  if (!keyword) {
    return true
  }
  return values.some((value) => String(value ?? '').toLowerCase().includes(keyword))
}

const filteredRoutes = computed(() => {
  const keyword = routeSearch.value.trim().toLowerCase()
  const selectedGroupSet = new Set(selectedGroups.value)
  const selectedRouteTypeSet = new Set(selectedRouteTypes.value)
  const selectedIssueStateSet = new Set(selectedIssueStates.value)
  return routes.value.filter((route) =>
    (!selectedGroupSet.size || parseGroupNames(route.group_name).some((groupName) => selectedGroupSet.has(groupName))) &&
    (!selectedRouteTypeSet.size || selectedRouteTypeSet.has(route.route_type)) &&
    (!selectedIssueStateSet.size ||
      (selectedIssueStateSet.has('with_error') && hasIssue(route)) ||
      (selectedIssueStateSet.has('without_error') && !hasIssue(route))) &&
    includesSearch(
      [
        route.site_name,
        route.key_name,
        route.route_type,
        routeTypeLabel(route.route_type),
        routeCircuitState(route),
        route.request_base_url,
        routeRequestBaseList(route).join(' '),
        route.base_url,
        formatGroupNames(route.group_name),
        route.group_name,
        route.last_error,
        route.balance_display,
        route.package_display,
      ],
      keyword,
    ),
  )
})

const filteredLogs = computed(() => {
  const keyword = logSearch.value.trim().toLowerCase()
  return logs.value.filter((log) =>
    includesSearch(
      [
        log.site_name,
        log.model,
        log.requested_model,
        log.actual_model,
        log.route_id,
        log.route_label,
        log.key_name,
        log.key_fingerprint,
        log.site_id,
        log.target_path,
        log.method,
        log.failure_reason,
        log.route_strategy,
      ],
      keyword,
    ),
  )
})

const filteredRouteLogs = computed(() => {
  const keyword = routeLogSearch.value.trim().toLowerCase()
  return routeLogs.value.filter((log) =>
    includesSearch(
      [
        log.site_name,
        log.model,
        log.requested_model,
        log.actual_model,
        log.route_id,
        log.route_label,
        log.key_name,
        log.key_fingerprint,
        log.site_id,
        log.target_path,
        log.method,
        log.failure_reason,
        log.route_strategy,
      ],
      keyword,
    ),
  )
})

const activeRouteFilterCount = computed(() =>
  selectedGroups.value.length +
  selectedRouteTypes.value.length +
  selectedIssueStates.value.length +
  (routeSearch.value.trim() ? 1 : 0),
)

function clearRouteFilters() {
  routeSearch.value = ''
  selectedGroups.value = []
  selectedRouteTypes.value = []
  selectedIssueStates.value = []
}

function applySiteSummary(summary: SiteSummary) {
  routes.value = routes.value.map((route) =>
    route.site_id === summary.site_id
      ? normalizeGatewayRoute({
          ...route,
          package_remaining: summary.package_remaining,
          package_total: summary.package_total,
          package_used: summary.package_used,
          package_unit: summary.package_unit,
          package_display: summary.package_display,
          checkin_status: summary.checkin_status,
        })
      : route,
  )
}

function applyProbeResult(result: GatewayRouteProbeResult) {
  routes.value = routes.value.map((route) =>
    route.id === result.id
      ? normalizeGatewayRoute({
          ...route,
          last_status_code: result.last_status_code,
	          last_error: result.last_error,
	          last_latency_ms: result.last_latency_ms,
	          last_success_at: result.last_success_at,
	          last_failure_at: result.last_failure_at,
	          supported_models: result.supported_models ?? result.models ?? route.supported_models,
	          model_probe_status: result.model_probe_status ?? route.model_probe_status,
	          model_probe_message: result.model_probe_message ?? result.message ?? route.model_probe_message,
	          model_probe_updated_at: result.model_probe_updated_at ?? result.checked_at ?? route.model_probe_updated_at,
	        })
      : route,
  )
}

function applyRouteBalanceResult(result: BalanceProbeResult) {
  routes.value = routes.value.map((route) =>
    route.id === result.route_id
      ? normalizeGatewayRoute({
          ...route,
          last_balance: result.last_balance ?? result.remaining,
          balance_display: result.balance_display || formatBalance(result.last_balance ?? result.remaining, result.unit),
          balance_unit: normalizeBalanceUnit(result.unit ?? route.balance_unit),
          balance_probe_url: result.balance_probe_url ?? route.balance_probe_url,
        })
      : route,
  )
}

async function probeRouteBalances(routeIds: number[], options: { silent?: boolean; progress?: { value: RouteBatchProgress | null } } = {}) {
  const ids = [...new Set(routeIds.filter((id) => Number.isFinite(id) && id > 0))]
  if (!ids.length) {
    return { success: 0, failed: 0 }
  }
  balanceProbingRouteIds.value = [...new Set([...balanceProbingRouteIds.value, ...ids])]
  if (options.progress) {
    options.progress.value = { total: ids.length, done: 0, success: 0, failed: 0 }
  }
  let success = 0
  let failed = 0
  try {
    for (const routeId of ids) {
      try {
        const result = await probeGatewayRouteBalance(routeId)
        applyRouteBalanceResult(result)
        if (result.ok) {
          success += 1
        } else {
          failed += 1
        }
      } catch {
        failed += 1
      } finally {
        if (options.progress?.value) {
          options.progress.value = {
            ...options.progress.value,
            done: options.progress.value.done + 1,
            success,
            failed,
          }
        }
      }
    }
    try {
      overview.value = await getGatewayOverview()
    } catch {
      // 余额已写入路由，概览统计刷新失败不阻断当前操作。
    }
    if (!options.silent) {
      if (failed > 0) {
        toast.error(`余额探测完成，成功 ${success} 条，失败 ${failed} 条。`)
      } else {
        toast.success(`余额探测完成，${success} 条全部读取成功。`)
      }
    }
    return { success, failed }
  } finally {
    balanceProbingRouteIds.value = balanceProbingRouteIds.value.filter((id) => !ids.includes(id))
  }
}

function isRouteProbing(routeId: number) {
  return probingRouteIds.value.includes(routeId)
}

function isRouteBalanceProbing(routeId: number) {
  return balanceProbingRouteIds.value.includes(routeId)
}

const probeAllProgressPercent = computed(() => {
  const progress = probeAllProgress.value
  if (!progress || progress.total <= 0) {
    return 0
  }
  return Math.min(100, Math.round((progress.done / progress.total) * 100))
})

const balanceProbeAllProgressPercent = computed(() => {
  const progress = balanceProbeAllProgress.value
  if (!progress || progress.total <= 0) {
    return 0
  }
  return Math.min(100, Math.round((progress.done / progress.total) * 100))
})

function applyReorderedRoutes(routeData: GatewayRoute[]) {
  const normalized = routeData.map(normalizeGatewayRoute)
  priorityRoutes.value = normalized
  routes.value = normalized.filter((route) => includeDisabled.value || route.is_enabled)
}

async function openPriorityDialog(route: GatewayRoute) {
  priorityRoute.value = route
  priorityInsertIndex.value = undefined
  priorityDialogOpen.value = true
  priorityRoutes.value = routes.value
  priorityDialogLoading.value = true
  try {
    priorityRoutes.value = (await getGatewayRoutes({ includeDisabled: true })).map(normalizeGatewayRoute)
    priorityRoute.value = priorityRoutes.value.find((item) => item.id === route.id) ?? route
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '优先级列表加载失败')
  } finally {
    priorityDialogLoading.value = false
  }
}

async function handlePriorityMove() {
  if (!priorityRoute.value) {
    return
  }
  const target = priorityInsertIndex.value
  if (target === undefined || target === null || Number.isNaN(Number(target))) {
    toast.error('请输入目标优先级。')
    return
  }
  priorityDialogLoading.value = true
  try {
    const routeData = await reorderGatewayRoutePriorities({
      route_id: priorityRoute.value.id,
      mode: 'move',
      index: Math.trunc(Number(target)),
    })
    applyReorderedRoutes(routeData)
    priorityRoute.value = priorityRoutes.value.find((route) => route.id === priorityRoute.value?.id) ?? priorityRoute.value
    toast.success('优先级已更新。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '优先级更新失败')
  } finally {
    priorityDialogLoading.value = false
  }
}

async function handlePriorityPreset(mode: 'package' | 'balance') {
  priorityDialogLoading.value = true
  try {
    const routeData = await reorderGatewayRoutePriorities({ mode })
    applyReorderedRoutes(routeData)
    priorityInsertIndex.value = undefined
    if (priorityRoute.value) {
      priorityRoute.value = priorityRoutes.value.find((route) => route.id === priorityRoute.value?.id) ?? priorityRoute.value
    }
    toast.success(mode === 'package' ? '已按套餐优先重排。' : '已按余额优先重排。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '优先级重排失败')
  } finally {
    priorityDialogLoading.value = false
  }
}

async function refreshRouteSummaries() {
  const siteIds = [...new Set(routes.value.map((route) => route.site_id))]
  if (!siteIds.length) {
    return
  }
  try {
    const summaries = await refreshSiteSummaries({ site_ids: siteIds })
    summaries.forEach(applySiteSummary)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '路由摘要刷新失败')
  }
}

const { schedule: scheduleRouteSummaryRefresh } = useDebouncedTask(refreshRouteSummaries)

async function handleRefresh() {
  await loadData()
  await probeRouteBalances(routes.value.map((route) => route.id), { silent: true })
  await refreshRouteSummaries()
}

async function loadGatewayUsage(silent = false) {
  if (!isGatewayMonitor.value) {
    gatewayUsage.value = null
    return
  }
  const start = datetimeLocalToISOString(usageRange.start)
  const end = datetimeLocalToISOString(usageRange.end)
  if (!start || !end) {
    if (!silent) {
      toast.error('请选择有效的开始和结束时间')
    }
    return
  }
  usageLoading.value = true
  try {
    gatewayUsage.value = await getGatewayUsage({ start, end })
  } catch (err) {
    if (!silent) {
      toast.error(err instanceof Error ? err.message : '网关消耗加载失败')
    }
  } finally {
    usageLoading.value = false
  }
}

async function handleUsageQuery() {
  await loadGatewayUsage()
}

async function handleUsageToday() {
  resetUsageRangeToToday()
  await loadGatewayUsage()
}

async function loadActiveRequests(silent = false) {
  try {
    const snapshot = await getGatewayActiveRequests()
    activeRequests.value = snapshot
    applyActiveRequestSnapshot(snapshot)
  } catch (err) {
    if (!silent) {
      toast.error(err instanceof Error ? err.message : '网关实时请求加载失败')
    }
  }
}

async function loadData() {
  loading.value = true
  try {
    const [overviewData, settingsData, routeData, logData, groupData, usageData] = await Promise.all([
      getGatewayOverview(),
      getGatewaySettings(),
      getGatewayRoutes({ includeDisabled: includeDisabled.value }),
      getGatewayLogs(80),
      getSiteGroups(),
      isGatewayMonitor.value ? getGatewayUsage({
        start: datetimeLocalToISOString(usageRange.start),
        end: datetimeLocalToISOString(usageRange.end),
      }) : Promise.resolve(null),
    ])
    overview.value = overviewData
    Object.assign(settingsForm, settingsData)
    const normalizedRoutes = routeData.map(normalizeGatewayRoute)
    priorityRoutes.value = normalizedRoutes
    routes.value = normalizedRoutes
    logs.value = logData
    siteGroups.value = groupData
    gatewayUsage.value = usageData
    await loadActiveRequests()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '网关数据加载失败')
  } finally {
    loading.value = false
  }
}

async function refreshRealtimeData() {
  if (autoRefreshing.value || document.visibilityState !== 'visible') {
    return
  }
  const now = Date.now()
  if (now - lastAutoRefreshAt < 1800) {
    return
  }
  lastAutoRefreshAt = now
  autoRefreshing.value = true
  try {
    const [overviewData, routeData, logData, usageData] = await Promise.all([
      getGatewayOverview(),
      getGatewayRoutes({ includeDisabled: includeDisabled.value }),
      getGatewayLogs(80),
      isGatewayMonitor.value ? getGatewayUsage({
        start: datetimeLocalToISOString(usageRange.start),
        end: datetimeLocalToISOString(usageRange.end),
      }) : Promise.resolve(null),
    ])
    overview.value = overviewData
    routes.value = routeData.map(normalizeGatewayRoute)
    gatewayUsage.value = usageData
    if (!priorityDialogOpen.value) {
      priorityRoutes.value = routeData.map(normalizeGatewayRoute)
    }
    logs.value = logData
    await loadActiveRequests(true)
  } catch {
    // 自动刷新静默失败，避免请求波动时持续打扰。
  } finally {
    autoRefreshing.value = false
  }
}

async function refreshActiveRequests(silent = true) {
  if (activeRequestsRefreshing.value || document.visibilityState !== 'visible') {
    return
  }
  const now = Date.now()
  if (now - lastActiveRequestRefreshAt < 500) {
    return
  }
  lastActiveRequestRefreshAt = now
  activeRequestsRefreshing.value = true
  try {
    await loadActiveRequests(silent)
  } finally {
    activeRequestsRefreshing.value = false
  }
}

function startAutoRefresh() {
  stopAutoRefresh()
  autoRefreshTimer = window.setInterval(refreshRealtimeData, isGatewayMonitor.value ? gatewayMonitorAutoRefreshMs : gatewayRouteAutoRefreshMs)
  activeRequestRefreshTimer = window.setInterval(() => {
    void refreshActiveRequests(true)
  }, gatewayActiveRequestRefreshMs)
}

function stopAutoRefresh() {
  if (autoRefreshTimer !== null) {
    window.clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
  if (activeRequestRefreshTimer !== null) {
    window.clearInterval(activeRequestRefreshTimer)
    activeRequestRefreshTimer = null
  }
}

function handleVisibilityChange() {
  if (document.visibilityState === 'visible') {
    void refreshRealtimeData()
    void refreshActiveRequests(true)
  }
}

async function handleSiteGroupsChanged() {
  try {
    siteGroups.value = await getSiteGroups()
  } catch {
    // header 分组变更后的轻量刷新，失败时保持现有选项。
  }
}

async function handleSync() {
  loading.value = true
  try {
    const result = await syncGatewayRoutes()
    await loadData()
    const balances = await probeRouteBalances(routes.value.map((route) => route.id), { silent: true })
    toast.success(`已同步 ${result.route_count} 条网关路由，余额读取成功 ${balances.success} 条。`)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '同步失败')
  } finally {
    loading.value = false
  }
}

async function handleToggle(route: GatewayRoute) {
  try {
    await toggleGatewayRoute(route.id)
    toast.success(route.is_enabled ? '已禁用该路由。' : '已重新启用该路由。')
    await loadData()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '切换失败')
  }
}

async function handleDisableAllRoutes() {
  if (!window.confirm('确认禁用全部路由？禁用后网关将没有可用路由，直到重新启用。')) {
    return
  }
  try {
    const result = await disableAllGatewayRoutes()
    toast.success(`已禁用 ${result.disabled_count} 条路由。`)
    await loadData()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '禁用全部失败')
  }
}

async function handleEnableOnlyRoute(route: GatewayRoute) {
  if (!window.confirm(`确认仅启用「${loadRouteLabel(route)}」，并禁用其他全部路由？`)) {
    return
  }
  try {
    await enableOnlyGatewayRoute(route.id)
    toast.success('已仅启用该路由，其他路由已禁用。')
    await loadData()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '禁用其他失败')
  }
}

async function handleResetCircuit(route: GatewayRoute) {
  try {
    await resetGatewayRouteCircuit(route.id)
    toast.success('已重置该路由熔断状态。')
    await loadData()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '重置失败')
  }
}

async function handleRouteTypeChange(route: GatewayRoute, routeType: GatewayRoute['route_type']) {
  const previousType = route.route_type
  routes.value = routes.value.map((item) => (item.id === route.id ? normalizeGatewayRoute({ ...item, route_type: routeType }) : item))
  try {
    const updated = await updateGatewayRouteType(route.id, {
      route_type: routeType,
      supported_models: route.supported_models ?? [],
    })
    const normalizedUpdated = normalizeGatewayRoute(updated)
    routes.value = routes.value.map((item) => (item.id === route.id ? normalizedUpdated : item))
    priorityRoutes.value = priorityRoutes.value.map((item) => (item.id === route.id ? normalizedUpdated : item))
    toast.success(`${loadRouteLabel(route)} 已切换为 ${routeTypeLabel(routeType)}。`)
  } catch (err) {
    routes.value = routes.value.map((item) => (item.id === route.id ? normalizeGatewayRoute({ ...item, route_type: previousType }) : item))
    toast.error(err instanceof Error ? err.message : '类型切换失败')
  }
}

async function handleRouteTypeSelect(route: GatewayRoute, value: unknown) {
  if (value !== 'claude' && value !== 'codex' && value !== 'gemini') {
    return
  }
  await handleRouteTypeChange(route, value)
}

function openRouteModelsDialog(route: GatewayRoute) {
  routeModelsDialogRoute.value = route
  routeModelsDialogValue.value = [...normalizeModelList(route.supported_models)]
  routeModelsDialogOpen.value = true
}

async function saveRouteModelsDialog() {
  const route = routeModelsDialogRoute.value
  if (!route) {
    return
  }
  routeModelsDialogSaving.value = true
  try {
    const updated = normalizeGatewayRoute(await updateGatewayRouteType(route.id, {
      route_type: route.route_type,
      supported_models: normalizeModelList(routeModelsDialogValue.value),
    }))
    routes.value = routes.value.map((item) => (item.id === route.id ? updated : item))
    priorityRoutes.value = priorityRoutes.value.map((item) => (item.id === route.id ? updated : item))
    routeModelsDialogOpen.value = false
    toast.success('路由支持模型已更新。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    routeModelsDialogSaving.value = false
  }
}

async function handleProbeAll() {
  const routeIds = routes.value.map((route) => route.id)
  if (!routeIds.length) {
    toast.error('当前没有可探测的网关路由。')
    return
  }

  probeLoading.value = true
  probeAllProgress.value = { total: routeIds.length, done: 0, success: 0, failed: 0 }
  probingRouteIds.value = [...new Set([...probingRouteIds.value, ...routeIds])]
  const failedResults: GatewayRouteProbeResult[] = []
  try {
    for (const routeId of routeIds) {
      try {
        const result = await probeGatewayRoute(routeId)
        applyProbeResult(result)
        if (result.ok) {
          probeAllProgress.value.success += 1
        } else {
          probeAllProgress.value.failed += 1
          failedResults.push(result)
        }
      } catch (err) {
        probeAllProgress.value.failed += 1
        const route = routes.value.find((item) => item.id === routeId)
        const message = err instanceof Error ? err.message : '探测请求失败'
        failedResults.push({
          id: routeId,
          site_id: route?.site_id ?? 0,
          site_name: route ? loadRouteLabel(route) : `Route #${routeId}`,
          request_base_url: route?.request_base_url,
          key_name: route?.key_name ?? '',
          key_fingerprint: route?.key_fingerprint,
          ok: false,
          status_code: null,
          latency_ms: null,
          message,
          models: [],
          supported_models: route?.supported_models ?? [],
          last_status_code: route?.last_status_code ?? null,
          last_error: route?.last_error ?? message,
          last_latency_ms: route?.last_latency_ms ?? null,
          last_success_at: route?.last_success_at ?? null,
          last_failure_at: route?.last_failure_at ?? null,
          checked_at: new Date().toISOString(),
        })
      } finally {
        probeAllProgress.value.done += 1
        probingRouteIds.value = probingRouteIds.value.filter((item) => item !== routeId)
      }
    }
    const successCount = probeAllProgress.value.success
    if (!failedResults.length) {
      toast.success(`路由探测完成，${successCount} 条全部可用。`)
      return
    }
    const sample = failedResults
      .slice(0, 2)
      .map((item) => `${item.site_name}${item.key_name ? ` · ${item.key_name}` : ''}`)
      .join('，')
    toast.error(`路由探测完成，成功 ${successCount} 条，失败 ${failedResults.length} 条：${sample}`)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '路由探测失败')
  } finally {
    probeLoading.value = false
    probingRouteIds.value = probingRouteIds.value.filter((item) => !routeIds.includes(item))
    window.setTimeout(() => {
      if (!probeLoading.value) {
        probeAllProgress.value = null
      }
    }, 1600)
  }
}

async function handleUpdateAllBalances() {
  const routeIds = routes.value.map((route) => route.id)
  if (!routeIds.length) {
    toast.error('当前没有可更新余额的网关路由。')
    return
  }
  if (probeLoading.value) {
    toast.error('路由探测仍在运行，请稍后再更新余额。')
    return
  }

  balanceProbeAllLoading.value = true
  try {
    const result = await probeRouteBalances(routeIds, { silent: true, progress: balanceProbeAllProgress })
    await refreshRouteSummaries()
    if (result.failed > 0) {
      toast.error(`余额更新完成，成功 ${result.success} 条，失败 ${result.failed} 条。`)
      return
    }
    toast.success(`余额更新完成，${result.success} 条全部读取成功。`)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '余额更新失败')
  } finally {
    balanceProbeAllLoading.value = false
    window.setTimeout(() => {
      if (!balanceProbeAllLoading.value) {
        balanceProbeAllProgress.value = null
      }
    }, 1600)
  }
}

async function handleProbeRoute(route: GatewayRoute) {
  probingRouteIds.value = [...probingRouteIds.value, route.id]
  try {
    const result = await probeGatewayRoute(route.id)
    applyProbeResult(result)
    if (result.ok) {
      toast.success(`${loadRouteLabel(route)} 探测成功${result.latency_ms !== null ? `，${result.latency_ms} ms` : ''}。`)
    } else {
      toast.error(`${loadRouteLabel(route)} 探测失败：${result.message}`)
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '路由探测失败')
  } finally {
    probingRouteIds.value = probingRouteIds.value.filter((item) => item !== route.id)
  }
}

async function handleProbeRouteBalance(route: GatewayRoute) {
  balanceProbingRouteIds.value = [...balanceProbingRouteIds.value, route.id]
  try {
    const result = await probeGatewayRouteBalance(route.id)
    applyRouteBalanceResult(result)
    await refreshRouteSummaries()
    if (result.ok) {
      toast.success(`${loadRouteLabel(route)} 余额读取成功：${result.balance_display || formatBalance(result.remaining, result.unit)}（${result.base_url}）`)
    } else {
      toast.error(`${loadRouteLabel(route)} 余额读取失败：${result.message}`)
      openRouteBalanceProbeManualDialog(route, result.message)
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '余额读取失败')
  } finally {
    balanceProbingRouteIds.value = balanceProbingRouteIds.value.filter((item) => item !== route.id)
  }
}

function openRouteBalanceProbeManualDialog(route: GatewayRoute, message = '') {
  const latest = routes.value.find((item) => item.id === route.id) ?? route
  balanceProbeManualRoute.value = latest
  balanceProbeManualURL.value = latest.balance_probe_url?.trim() || ''
  balanceProbeManualMessage.value = message
  balanceProbeManualOpen.value = true
}

async function submitManualRouteBalanceProbe() {
  const route = balanceProbeManualRoute.value
  const balanceProbeURL = balanceProbeManualURL.value.trim()
  if (!route) {
    return
  }
  if (!balanceProbeURL) {
    toast.error('请填写余额探测接口地址。')
    return
  }
  if (!/^https?:\/\//i.test(balanceProbeURL) && !balanceProbeURL.startsWith('/')) {
    toast.error('探测接口地址需要是完整 URL，或以 / 开头的相对路径。')
    return
  }
  balanceProbeManualLoading.value = true
  balanceProbingRouteIds.value = [...balanceProbingRouteIds.value, route.id]
  try {
    const result = await probeGatewayRouteBalance(route.id, { balance_probe_url: balanceProbeURL })
    applyRouteBalanceResult(result)
    await refreshRouteSummaries()
    if (result.ok) {
      toast.success(`${loadRouteLabel(route)} 余额读取成功：${result.balance_display || formatBalance(result.remaining, result.unit)}。`)
      balanceProbeManualOpen.value = false
      balanceProbeManualRoute.value = null
      balanceProbeManualMessage.value = ''
      return
    }
    balanceProbeManualMessage.value = result.message
    toast.error(`${loadRouteLabel(route)} 余额读取失败：${result.message}`)
  } catch (err) {
    const message = err instanceof Error ? err.message : '余额读取失败'
    balanceProbeManualMessage.value = message
    toast.error(message)
  } finally {
    balanceProbeManualLoading.value = false
    balanceProbingRouteIds.value = balanceProbingRouteIds.value.filter((item) => item !== route.id)
  }
}

async function openRouteDiagnosis(route: GatewayRoute) {
  routeDiagnosisOpen.value = true
  routeDiagnosis.value = null
  routeDiagnosisLoading.value = true
  try {
    routeDiagnosis.value = await diagnoseGatewayRoute(route.id)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '路由诊断失败')
  } finally {
    routeDiagnosisLoading.value = false
  }
}

async function openRouteLogs(route: GatewayRoute) {
  routeLogsDrawerOpen.value = true
  routeLogsRoute.value = route
  routeLogSearch.value = ''
  routeLogsLoading.value = true
  try {
    routeLogs.value = await getGatewayRouteLogs(route.id, 120)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '路由请求历史加载失败')
    routeLogs.value = []
  } finally {
    routeLogsLoading.value = false
  }
}

async function copyGatewayRequestUrl() {
  try {
    await navigator.clipboard.writeText(gatewayRequestUrl.value)
    toast.success('网关请求地址已复制。')
  } catch {
    toast.error('复制失败，请手动复制。')
  }
}

async function copyGatewayApiKey() {
  const value = settingsForm.gateway_api_key.trim()
  if (!value) {
    toast.error('后端未配置 GATEWAY_API_KEY。')
    return
  }
  try {
    await navigator.clipboard.writeText(value)
    toast.success('网关 API Key 已复制。')
  } catch {
    toast.error('复制失败，请手动复制。')
  }
}

async function saveSettings() {
  settingsLoading.value = true
  try {
    Object.assign(settingsForm, await updateGatewaySettings(settingsForm))
    settingsOpen.value = false
    toast.success('网关策略已保存。')
    await loadData()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    settingsLoading.value = false
  }
}

onMounted(async () => {
  window.addEventListener('site-groups:changed', handleSiteGroupsChanged)
  document.addEventListener('visibilitychange', handleVisibilityChange)
  resetUsageRangeToToday()
  await loadData()
  startAutoRefresh()
  scheduleRouteSummaryRefresh()
})

onBeforeUnmount(() => {
  stopAutoRefresh()
  window.removeEventListener('site-groups:changed', handleSiteGroupsChanged)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>

<template>
  <ShellLayout>
    <div
      class="page-stack"
      :class="isRouteManagement ? 'page-stack--fit gateway-route-page' : 'page-stack--dashboard gateway-monitor-page'"
    >
      <div class="page-toolbar page-toolbar--actions">
        <div v-if="isGatewayMonitor" class="gateway-monitor-toolbar">
          <div class="gateway-access">
            <code>地址 {{ gatewayRequestUrl }}</code>
            <a-tooltip placement="bottom" :title="codexGatewayTooltip">
              <span class="gateway-access__hint">
                <InfoCircleOutlined />
                Codex /v1
              </span>
            </a-tooltip>
            <a-button size="small" @click="copyGatewayRequestUrl">
              <template #icon>
                <CopyOutlined />
              </template>
              复制
            </a-button>
            <code>Key {{ maskedGatewayApiKey }}</code>
            <a-button size="small" :disabled="!settingsForm.gateway_api_key" @click="copyGatewayApiKey">
              <template #icon>
                <CopyOutlined />
              </template>
              复制
            </a-button>
          </div>
        </div>
        <div v-else class="route-management-toolbar">
          <div class="route-management-heading">
            <strong>路由池</strong>
            <span>{{ filteredRoutes.length }} / {{ routes.length }}</span>
          </div>
          <div class="gateway-access gateway-access--route">
            <code>地址 {{ gatewayRequestUrl }}</code>
            <a-tooltip placement="bottom" :title="codexGatewayTooltip">
              <span class="gateway-access__hint">
                <InfoCircleOutlined />
                Codex /v1
              </span>
            </a-tooltip>
            <a-button size="small" @click="copyGatewayRequestUrl">
              <template #icon>
                <CopyOutlined />
              </template>
              复制
            </a-button>
            <code>Key {{ maskedGatewayApiKey }}</code>
            <a-button size="small" :disabled="!settingsForm.gateway_api_key" @click="copyGatewayApiKey">
              <template #icon>
                <CopyOutlined />
              </template>
              复制
            </a-button>
          </div>
        </div>
        <a-space>
          <a-button :loading="loading" @click="handleRefresh">
            <template #icon>
              <ReloadOutlined />
            </template>
            刷新
          </a-button>
          <a-button v-if="isRouteManagement" :loading="loading" :disabled="probeLoading || balanceProbeAllLoading" @click="handleSync">
            <template #icon>
              <SyncOutlined />
            </template>
            同步路由
          </a-button>
          <div v-if="isRouteManagement" class="route-probe-control">
            <a-button :loading="probeLoading" :disabled="!routes.length || balanceProbeAllLoading" @click="handleProbeAll">探测全部</a-button>
            <div v-if="probeAllProgress" class="route-probe-progress">
              <div class="route-probe-progress__meta">
                <span>{{ probeAllProgress.done }}/{{ probeAllProgress.total }}</span>
                <span>{{ probeAllProgress.success }} 成功 · {{ probeAllProgress.failed }} 失败</span>
              </div>
              <div class="route-probe-progress__bar" aria-hidden="true">
                <span :style="{ width: `${probeAllProgressPercent}%` }"></span>
              </div>
            </div>
          </div>
          <div v-if="isRouteManagement" class="route-probe-control">
            <a-button :loading="balanceProbeAllLoading" :disabled="!routes.length || probeLoading" @click="handleUpdateAllBalances">更新余额</a-button>
            <div v-if="balanceProbeAllProgress" class="route-probe-progress route-probe-progress--balance">
              <div class="route-probe-progress__meta">
                <span>{{ balanceProbeAllProgress.done }}/{{ balanceProbeAllProgress.total }}</span>
                <span>{{ balanceProbeAllProgress.success }} 成功 · {{ balanceProbeAllProgress.failed }} 失败</span>
              </div>
              <div class="route-probe-progress__bar" aria-hidden="true">
                <span :style="{ width: `${balanceProbeAllProgressPercent}%` }"></span>
              </div>
            </div>
          </div>
          <a-button v-if="isRouteManagement" danger :disabled="!routes.length" @click="handleDisableAllRoutes">禁用全部</a-button>
          <a-button v-if="isRouteManagement" type="primary" @click="addUpstreamOpen = true">
            <template #icon>
              <PlusOutlined />
            </template>
            添加上游
          </a-button>
          <a-button @click="settingsOpen = true">
            <template #icon>
              <SettingOutlined />
            </template>
            网关策略
          </a-button>
          <a-button v-if="isGatewayMonitor" @click="logsDrawerOpen = true">最近请求</a-button>
        </a-space>
      </div>

      <div v-if="isGatewayMonitor" class="gateway-metrics">
        <a-card
          v-for="item in metricCards"
          :key="item.title"
          :bordered="false"
          class="admin-card gateway-metric-card"
          :class="`gateway-metric-card--${item.tone}`"
        >
          <div class="gateway-metric-card__label">{{ item.title }}</div>
          <div class="gateway-metric-card__value">{{ item.value }}</div>
        </a-card>
      </div>

      <div class="gateway-fill">
        <a-card
          v-if="isGatewayMonitor"
          :bordered="false"
          class="admin-card gateway-overview-shell"
        >
          <div class="gateway-overview-grid">
            <section class="gateway-panel gateway-panel--usage">
              <div class="gateway-panel__head">
                <div>
                  <div class="gateway-panel__title">时间段消耗</div>
                </div>
                <a-space wrap>
                  <input v-model="usageRange.start" class="gateway-usage-input" type="datetime-local" />
                  <span class="gateway-usage-card__sep">至</span>
                  <input v-model="usageRange.end" class="gateway-usage-input" type="datetime-local" />
                  <a-button @click="handleUsageToday">今日</a-button>
                  <a-button type="primary" :loading="usageLoading" @click="handleUsageQuery">查询</a-button>
                </a-space>
              </div>
              <div v-if="usageSummaryCards.length" class="gateway-usage-summary">
                <div
                  v-for="item in usageSummaryCards"
                  :key="item.title"
                  class="gateway-usage-summary__item"
                  :class="`gateway-usage-summary__item--${item.tone}`"
                >
                  <span>{{ item.title }}</span>
                  <strong>{{ item.value }}</strong>
                </div>
              </div>
              <a-table
                class="gateway-usage-table"
                :columns="usageColumns"
                :data-source="gatewayUsage?.routes ?? []"
                :pagination="{ pageSize: 6, showSizeChanger: false }"
                :row-key="usageRowKey"
                size="small"
                :loading="usageLoading"
                :scroll="{ x: 1210 }"
              >
                <template #bodyCell="{ column, record }">
                  <template v-if="column.key === 'route'">
                    <div class="table-cell-compact">
                      <div class="table-cell-compact__head">
                        <strong class="table-cell-compact__title">{{ usageRouteLabel(record as GatewayUsageRoute) }}</strong>
                        <a-tooltip placement="right" :title="usageRouteMeta(record as GatewayUsageRoute)">
                          <InfoCircleOutlined class="table-info-icon" />
                        </a-tooltip>
                      </div>
                    </div>
                  </template>
                  <template v-else-if="column.key === 'requests'">
                    {{ formatNumber((record as GatewayUsageRoute).request_count) }}
                  </template>
                  <template v-else-if="column.key === 'success_rate'">
                    {{ (record as GatewayUsageRoute).success_rate }}%
                  </template>
                  <template v-else-if="column.key === 'stream'">
                    {{ formatNumber((record as GatewayUsageRoute).stream_request_count) }}
                  </template>
                  <template v-else-if="column.key === 'prompt_tokens'">
                    {{ formatNumber((record as GatewayUsageRoute).prompt_tokens) }}
                  </template>
                  <template v-else-if="column.key === 'cached_input_tokens'">
                    {{ formatNumber((record as GatewayUsageRoute).cached_input_tokens) }}
                  </template>
                  <template v-else-if="column.key === 'completion_tokens'">
                    {{ formatNumber((record as GatewayUsageRoute).completion_tokens) }}
                  </template>
                  <template v-else-if="column.key === 'total_tokens'">
                    <strong>{{ formatNumber((record as GatewayUsageRoute).total_tokens) }}</strong>
                  </template>
                  <template v-else-if="column.key === 'computed_total_cost'">
                    <a-tooltip :title="`输入 ${formatUSD((record as GatewayUsageRoute).computed_input_cost)} / 缓存 ${formatUSD((record as GatewayUsageRoute).computed_cached_cost)} / 输出 ${formatUSD((record as GatewayUsageRoute).computed_output_cost)}`">
                      <strong>{{ formatUSD((record as GatewayUsageRoute).computed_total_cost) }}</strong>
                    </a-tooltip>
                  </template>
                  <template v-else-if="column.key === 'avg_latency'">
                    {{ (record as GatewayUsageRoute).avg_latency_ms !== null ? `${(record as GatewayUsageRoute).avg_latency_ms} ms` : '暂无' }}
                  </template>
                  <template v-else-if="column.key === 'last_used_at'">
                    {{ formatTime((record as GatewayUsageRoute).last_used_at) }}
                  </template>
                </template>
              </a-table>
            </section>

            <section class="gateway-panel gateway-panel--activity">
              <div class="gateway-panel__head">
                <div>
                  <div class="gateway-panel__title">实时调用</div>
                </div>
                <span class="gateway-active-feed-panel__pulse" :class="{ 'gateway-active-feed-panel__pulse--active': activeRouteFeed.length > 0 }">
                  {{ activeRouteFeed.length > 0 ? '运行中' : '空闲' }}
                </span>
              </div>
              <div v-if="routeActivityFeed.length" class="gateway-active-feed gateway-active-feed--embedded">
                <div
                  v-for="item in routeActivityFeed"
                  :key="item.id"
                  class="gateway-active-feed__item"
                  :class="{ 'gateway-active-feed__item--completed': item.kind === 'completed' }"
                >
                  <div class="gateway-active-feed__rail">
                    <span class="gateway-active-feed__dot" />
                  </div>
                  <div class="gateway-active-feed__main">
                    <strong class="gateway-active-feed__route">{{ item.label }}</strong>
                    <a-space size="small" wrap class="gateway-active-feed__badges">
                      <a-tag :color="item.primaryBadgeColor">{{ item.primaryBadge }}</a-tag>
                      <a-tag>{{ item.secondaryBadge }}</a-tag>
                      <a-tag v-if="item.is_stream" color="blue">流式</a-tag>
                    </a-space>
                    <div class="gateway-active-feed__request">
                      <span>{{ item.targetLabel }}</span>
                      <span>请求 {{ item.requestedModelLabel }}</span>
                      <span>命中 {{ item.actualModelLabel }}</span>
                      <span v-if="item.routeTypeLabel">{{ item.routeTypeLabel }}</span>
                      <span>{{ item.groupLabel }}</span>
                      <span>{{ item.strategyLabel }}</span>
                      <span>{{ item.attemptLabel }}</span>
                    </div>
                    <div class="gateway-active-feed__meta">
                      <span v-for="meta in item.meta" :key="meta">{{ meta }}</span>
                      <span>{{ item.timeLabel }}</span>
                    </div>
                  </div>
                </div>
              </div>
              <a-empty v-else description="等待网关请求进入路由池。" />
            </section>

            <section class="gateway-panel gateway-panel--route-status">
              <div class="gateway-panel__head">
                <div>
                  <div class="gateway-panel__title">路由池状态</div>
                </div>
              </div>
              <div class="route-pool-status">
                <div
                  v-for="item in routePoolStatusCards"
                  :key="item.key"
                  class="route-pool-status__card"
                  :class="`route-pool-status__card--${item.tone}`"
                >
                  <div class="route-pool-status__head">
                    <span>{{ item.label }}</span>
                    <strong>{{ item.value }}</strong>
                  </div>
                  <div class="route-pool-status__bar">
                    <span :style="{ width: `${Math.max(8, item.ratio * 100)}%` }" />
                  </div>
                </div>
              </div>

              <div class="route-pool-preview">
                <div
                  v-for="route in routePoolPreviewRoutes"
                  :key="route.id"
                  class="route-pool-preview__row"
                >
                  <div class="route-pool-preview__main">
                    <strong>{{ loadRouteLabel(route) }}</strong>
                    <span>{{ formatGroupNames(route.group_name) || '未分组' }} · {{ routeTypeLabel(route.route_type) }}</span>
                  </div>
                  <div class="route-pool-preview__meta">
                    <span :class="latencyClass(primaryLatency(route))">
                      <span class="gateway-latency__dot"></span>
                      <span class="gateway-latency__value">{{ formatLatency(primaryLatency(route)) }}</span>
                    </span>
                    <span :class="['gateway-concurrency', { 'gateway-concurrency--active': route.active_concurrency > 0 }]">
                      <span class="gateway-concurrency__current">{{ route.active_concurrency }}</span>
                      <span class="gateway-concurrency__separator">/</span>
                      <span class="gateway-concurrency__limit">{{ routeConcurrencyLimitLabel }}</span>
                    </span>
                  </div>
                </div>
              </div>
            </section>

            <section class="gateway-panel gateway-panel--strategy">
              <div class="gateway-panel__head">
                <div>
                  <div class="gateway-panel__title">策略分布</div>
                </div>
              </div>
              <div class="gateway-strategy-board">
                <div
                  v-for="item in gatewayStrategyCards"
                  :key="item.key"
                  class="gateway-strategy-board__item"
                  :class="`gateway-strategy-board__item--${item.tone}`"
                >
                  <div class="gateway-strategy-board__summary">
                    <span>{{ item.title }}</span>
                    <strong>{{ item.value }}</strong>
                  </div>
                  <div class="gateway-strategy-board__track">
                    <span :style="{ width: item.width }" />
                  </div>
                </div>
                <a-empty v-if="!gatewayStrategyCards.length" description="暂无策略统计数据" />
              </div>
            </section>
          </div>
        </a-card>

      <a-card v-if="isRouteManagement" :bordered="false" class="admin-card admin-card--fill route-pool-card route-pool-card--standalone">
        <div class="card-shell">
          <div class="route-pool-filters">
            <div class="route-pool-type-tabs">
              <a-button
                size="small"
                :type="selectedRouteTypes.length === 0 ? 'primary' : 'default'"
                @click="clearRouteTypeFilter"
              >
                全部
              </a-button>
              <a-button
                v-for="item in routeTypeFilterOptions"
                :key="item.value"
                size="small"
                :type="isRouteTypeFilterActive(item.value) ? 'primary' : 'default'"
                @click="toggleRouteTypeFilter(item.value)"
              >
                {{ item.label }}
              </a-button>
            </div>
            <div class="route-pool-searchbar">
              <a-input
                id="route-pool-search"
                v-model:value="routeSearch"
                class="route-pool-filter route-pool-filter--search"
                name="route_pool_search"
                placeholder="搜索路由 / 域名 / 分组"
                allow-clear
              />
              <a-select
                v-model:value="selectedGroups"
                class="route-pool-filter route-pool-filter--group"
                mode="multiple"
                allow-clear
                :options="groupOptions"
                placeholder="按分组筛选"
              />
              <a-select
                v-model:value="selectedIssueStates"
                class="route-pool-filter route-pool-filter--compact"
                mode="multiple"
                allow-clear
                :options="issueStateOptions"
                placeholder="按异常"
              />
              <a-switch
                v-model:checked="includeDisabled"
                class="route-pool-filter-switch"
                checked-children="含停用"
                un-checked-children="仅启用"
                @change="loadData"
              />
              <a-button class="route-pool-clear" size="small" :disabled="!activeRouteFilterCount" @click="clearRouteFilters">
                清空筛选
              </a-button>
            </div>
          </div>
          <div :ref="bindPageTableContainer" class="table-fill table-fill--management">
            <a-table
              :columns="routeColumns"
              :data-source="filteredRoutes"
              :pagination="{ pageSize: gatewayTablePageSize }"
              :row-key="routeRowKey"
              size="middle"
              :scroll="{ x: 1760, y: pageTableY }"
            >
              <template #headerCell="{ column }">
                <template v-if="column.key === 'weight'">
                  <span class="table-header-help">
                    <span>权重</span>
                    <a-tooltip
                      placement="top"
                      title="用于加权轮询和智能评分。权重越大，在健康且满足并发/熔断条件时获得请求的概率越高；智能策略还会结合延迟、并发、失败记录和优先级共同计算。"
                    >
                      <QuestionCircleOutlined class="table-info-icon" />
                    </a-tooltip>
                  </span>
                </template>
              </template>
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'route'">
                  <div class="table-cell-compact">
                    <div class="table-cell-compact__head">
                      <a-tooltip placement="topLeft" :title="loadRouteLabel(asRoute(record))">
                        <strong class="table-cell-compact__title">{{ loadRouteLabel(asRoute(record)) }}</strong>
                      </a-tooltip>
                      <a-tooltip placement="right">
                        <template #title>
                          <div class="tooltip-detail-list">
                            <div v-for="item in routeDetailItems(asRoute(record))" :key="item.label">
                              <strong>{{ item.label }}</strong>
                              <span>{{ item.value }}</span>
                            </div>
                          </div>
                        </template>
                        <InfoCircleOutlined class="table-info-icon" />
                      </a-tooltip>
                    </div>
                    <div class="table-cell-compact__meta">
                      <span class="table-cell-compact__meta-label">模型能力</span>
                      <span>{{ supportedModelsPreview(asRoute(record).supported_models) }}</span>
                    </div>
                    <div v-if="routeIssueLabels(asRoute(record)).length" class="table-cell-compact__tags">
                      <a-tag
                        v-for="label in routeIssueLabels(asRoute(record))"
                        :key="label"
                        color="error"
                        class="route-issue-tag"
                      >
                        {{ label }}
                      </a-tag>
                    </div>
                  </div>
                </template>
                <template v-else-if="column.key === 'type'">
                  <a-select
                    :value="asRoute(record).route_type"
                    :class="['route-type-select', `route-type-select--${asRoute(record).route_type}`]"
                    size="small"
                    :options="routeTypeOptions"
                    style="width: 104px"
                    @change="(value) => handleRouteTypeSelect(asRoute(record), value)"
                  >
                    <template #option="{ label, value }">
                      <span :class="['route-type-option', `route-type-option--${value}`]">{{ label }}</span>
                    </template>
                  </a-select>
                </template>
                <template v-else-if="column.key === 'balance'">
                  <span :class="balanceClass(asRoute(record).last_balance)">
                    {{ asRoute(record).balance_display || '暂无' }}
                  </span>
                </template>
                <template v-else-if="column.key === 'group'">
                  {{ formatGroupNames(asRoute(record).group_name) || '未分组' }}
                </template>
                <template v-else-if="column.key === 'priority'">
                  {{ asRoute(record).route_priority }}
                </template>
                <template v-else-if="column.key === 'weight'">
                  {{ asRoute(record).weight }}
                </template>
                <template v-else-if="column.key === 'concurrency'">
                  <a-tooltip :title="`当前并发 ${asRoute(record).active_concurrency} / 最大转移 ${routeConcurrencyLimitLabel}；达到后优先转发，不是硬上限`">
                    <span :class="['gateway-concurrency', { 'gateway-concurrency--active': asRoute(record).active_concurrency > 0 }]">
                      <span class="gateway-concurrency__current">{{ asRoute(record).active_concurrency }}</span>
                      <span class="gateway-concurrency__separator">/</span>
                      <span class="gateway-concurrency__limit">{{ routeConcurrencyLimitLabel }}</span>
                    </span>
                  </a-tooltip>
                </template>
                <template v-else-if="column.key === 'success_rate'">
                  {{ asRoute(record).success_rate }}%
                </template>
                <template v-else-if="column.key === 'latency'">
                  <a-tooltip v-if="routeLatencyDetails(asRoute(record)).length" placement="topLeft">
                    <template #title>
                      <div class="tooltip-detail-list">
                        <div v-for="item in routeLatencyDetails(asRoute(record))" :key="item">
                          <span>{{ item }}</span>
                        </div>
                      </div>
                    </template>
                    <div class="participation-cell">
                      <span :class="latencyClass(primaryLatency(asRoute(record)))">
                        <span class="gateway-latency__dot"></span>
                        <span class="gateway-latency__value">{{ formatLatency(primaryLatency(asRoute(record))) }}</span>
                      </span>
                    </div>
                  </a-tooltip>
                  <div v-else class="participation-cell">
                    <span :class="latencyClass(primaryLatency(asRoute(record)))">
                      <span class="gateway-latency__dot"></span>
                      <span class="gateway-latency__value">{{ formatLatency(primaryLatency(asRoute(record))) }}</span>
                    </span>
                  </div>
                </template>
                <template v-else-if="column.key === 'error'">
                  <a-tooltip v-if="routeErrorDetails(asRoute(record)).length" placement="topLeft">
                    <template #title>
                      <div class="tooltip-detail-list">
                        <div v-for="item in routeErrorDetails(asRoute(record))" :key="item">
                          <span>{{ item }}</span>
                        </div>
                      </div>
                    </template>
                    <span class="table-ellipsis">{{ compactText(asRoute(record).last_error, 42) || '-' }}</span>
                  </a-tooltip>
                  <span v-else>-</span>
                </template>
                <template v-else-if="column.key === 'actions'">
                  <a-space size="small" class="gateway-actions-cell">
                    <a-button size="small" :danger="asRoute(record).is_enabled" @click.stop="handleToggle(asRoute(record))">
                      {{ asRoute(record).is_enabled ? '禁用' : '启用' }}
                    </a-button>
                    <a-dropdown :trigger="['click']">
                      <a-tooltip title="更多操作">
                        <a-button
                          size="small"
                          class="gateway-actions-menu-button"
                          :loading="isRouteProbing(asRoute(record).id) || isRouteBalanceProbing(asRoute(record).id)"
                          @click.stop
                        >
                          <template #icon><MoreOutlined /></template>
                        </a-button>
                      </a-tooltip>
                      <template #overlay>
                        <a-menu @click.stop>
                          <a-menu-item
                            key="reset-circuit"
                            :disabled="asRoute(record).circuit_state === 'closed'"
                            @click="handleResetCircuit(asRoute(record))"
                          >
                            <ReloadOutlined />
                            <span>重置熔断</span>
                          </a-menu-item>
                          <a-menu-item
                            key="probe"
                            :disabled="isRouteProbing(asRoute(record).id)"
                            @click="handleProbeRoute(asRoute(record))"
                          >
                            <SyncOutlined />
                            <span>探测</span>
                          </a-menu-item>
                          <a-menu-item
                            key="balance"
                            :disabled="isRouteBalanceProbing(asRoute(record).id)"
                            @click="handleProbeRouteBalance(asRoute(record))"
                          >
                            <InfoCircleOutlined />
                            <span>余额</span>
                          </a-menu-item>
                          <a-menu-item key="supported-models" @click="openRouteModelsDialog(asRoute(record))">
                            <ToolOutlined />
                            <span>支持模型</span>
                          </a-menu-item>
                          <a-menu-divider />
                          <a-menu-item key="enable-only" @click="handleEnableOnlyRoute(asRoute(record))">
                            <SettingOutlined />
                            <span>禁用其他</span>
                          </a-menu-item>
                          <a-menu-item key="priority" @click="openPriorityDialog(asRoute(record))">
                            <SettingOutlined />
                            <span>优先权</span>
                          </a-menu-item>
                          <a-menu-item key="diagnosis" @click="openRouteDiagnosis(asRoute(record))">
                            <ToolOutlined />
                            <span>诊断</span>
                          </a-menu-item>
                          <a-menu-item key="history" @click="openRouteLogs(asRoute(record))">
                            <HistoryOutlined />
                            <span>历史</span>
                          </a-menu-item>
                        </a-menu>
                      </template>
                    </a-dropdown>
                  </a-space>
                </template>
              </template>
            </a-table>
          </div>
        </div>
      </a-card>
      </div>

      <a-modal
        v-model:open="priorityDialogOpen"
        title="设置优先权"
        width="920px"
        :footer="null"
      >
        <a-spin :spinning="priorityDialogLoading">
          <div class="priority-dialog">
            <a-table
              :columns="priorityDialogColumns"
              :data-source="priorityRoutes"
              :pagination="{ pageSize: 8 }"
              :row-key="routeRowKey"
              :row-class-name="priorityRouteRowClassName"
              size="small"
              :scroll="{ x: 760, y: 360 }"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'route'">
                  <div class="priority-route-name">
                    <a-tooltip placement="topLeft" :title="loadRouteLabel(asRoute(record))">
                      <strong>{{ loadRouteLabel(asRoute(record)) }}</strong>
                    </a-tooltip>
                    <a-tag v-if="asRoute(record).id === priorityRoute?.id" color="processing">当前</a-tag>
                  </div>
                </template>
                <template v-else-if="column.key === 'priority'">
                  <span class="priority-number">{{ asRoute(record).route_priority }}</span>
                </template>
                <template v-else-if="column.key === 'group'">
                  {{ formatGroupNames(asRoute(record).group_name) || '未分组' }}
                </template>
              </template>
            </a-table>

            <div class="priority-editor">
              <div class="priority-editor__summary">
                <span>当前路由</span>
                <strong>{{ priorityRoute ? loadRouteLabel(priorityRoute) : '未选择' }}</strong>
                <span>当前优先级 {{ routePriorityLabel(priorityRoute) }}</span>
              </div>
              <div class="priority-editor__actions">
                <a-input-number
                  v-model:value="priorityInsertIndex"
                  class="priority-editor__input"
                  :min="0"
                  :max="Math.max(priorityRoutes.length - 1, 0)"
                  :precision="0"
                  placeholder="目标优先级"
                />
                <a-button type="primary" :disabled="!priorityRoute" @click="handlePriorityMove">
                  移动到优先级
                </a-button>
                <a-button @click="handlePriorityPreset('package')">优先套餐</a-button>
                <a-button @click="handlePriorityPreset('balance')">优先余额</a-button>
              </div>
            </div>
          </div>
        </a-spin>
      </a-modal>

      <a-modal
        v-model:open="balanceProbeManualOpen"
        title="余额探测接口"
        width="640px"
        :confirm-loading="balanceProbeManualLoading"
        ok-text="重试探测"
        @ok="submitManualRouteBalanceProbe"
      >
        <a-form layout="vertical">
          <a-alert
            v-if="balanceProbeManualMessage"
            type="warning"
            show-icon
            :message="balanceProbeManualMessage"
            style="margin-bottom: 12px"
          />
          <a-form-item :label="balanceProbeManualRoute ? loadRouteLabel(balanceProbeManualRoute) : '当前路由'">
            <a-input
              v-model:value="balanceProbeManualURL"
              placeholder="https://example.com/v1/usage 或 /api/usage/token/"
              autocomplete="off"
            />
            <small class="field-help">成功后会保存到当前路由，后续余额探测优先使用这个接口，并使用该路由自己的 API Key。</small>
          </a-form-item>
        </a-form>
      </a-modal>

      <a-modal
        v-model:open="settingsOpen"
        title="网关策略"
        width="980px"
        :confirm-loading="settingsLoading"
        @ok="saveSettings"
      >
        <a-form layout="vertical">
          <div class="gateway-policy-settings-layout">
            <div class="gateway-policy-settings-layout__main">
              <a-row :gutter="16">
                <a-col :xs="24" :md="12">
                  <a-form-item label="路由策略">
                    <a-select
                      v-model:value="settingsForm.route_strategy"
                      :options="gatewayRouteStrategyOptions.map(({ label, value }) => ({ label, value }))"
                    />
                    <small class="field-help">{{ selectedRouteStrategyDescription }}</small>
                  </a-form-item>
                </a-col>
                <a-col :xs="24" :md="12">
                  <a-form-item label="最大尝试次数">
                    <a-input-number
                      v-model:value="settingsForm.max_attempts"
                      style="width: 100%"
                      :min="0"
                      :max="50"
                    />
                    <small class="field-help">填 0 表示当前池里所有健康路由都可参与失败切换。</small>
                  </a-form-item>
                </a-col>
              </a-row>

              <a-row :gutter="16">
                <a-col :xs="24" :md="12">
                  <a-form-item label="熔断阈值">
                    <a-input-number
                      v-model:value="settingsForm.failure_threshold"
                      style="width: 100%"
                      :min="1"
                      :max="20"
                    />
                  </a-form-item>
                </a-col>
                <a-col :xs="24" :md="12">
                  <a-form-item label="熔断冷却时间（秒）">
                    <a-input-number
                      v-model:value="settingsForm.cooldown_seconds"
                      style="width: 100%"
                      :min="10"
                      :max="3600"
                    />
                  </a-form-item>
                </a-col>
              </a-row>

              <a-form-item label="网关请求超时（秒）">
                <a-input-number
                  v-model:value="settingsForm.request_timeout"
                  style="width: 100%"
                  :min="5"
                  :max="180"
                />
              </a-form-item>

              <a-form-item label="上游错误切换">
                <a-select
                  v-model:value="settingsForm.failure_retry_mode"
                  :options="gatewayFailureRetryModeOptions.map(({ label, value }) => ({ label, value }))"
                />
                <small class="field-help">{{ selectedFailureRetryModeDescription }}</small>
              </a-form-item>

              <a-row :gutter="16">
                <a-col :xs="24" :md="12">
                  <a-form-item label="单路由最大转移">
                    <a-input-number
                      v-model:value="settingsForm.route_concurrency_limit"
                      style="width: 100%"
                      :min="0"
                      :max="1000"
                    />
                    <small class="field-help">例如填 5，某条路由达到 5 个当前并发后，新请求会优先转到其他未达阈值路由；如果所有路由都已达到阈值，仍会继续选择并累加并发。填 0 表示不主动转移。</small>
                  </a-form-item>
                </a-col>
                <a-col :xs="24" :md="12">
                  <a-form-item label="并发转移策略">
                    <a-select
                      v-model:value="settingsForm.concurrency_transfer_strategy"
                      :options="gatewayConcurrencyTransferOptions.map(({ label, value }) => ({ label, value }))"
                    />
                    <small class="field-help">{{ selectedConcurrencyTransferDescription }}</small>
                  </a-form-item>
                </a-col>
              </a-row>

              <a-row :gutter="16">
                <a-col :xs="24" :md="12">
                  <a-form-item label="并发溢出优先级">
                    <a-select
                      v-model:value="settingsForm.concurrency_overflow_strategy"
                      :options="gatewayOverflowStrategyOptions.map(({ label, value }) => ({ label, value }))"
                    />
                    <small class="field-help">{{ selectedOverflowStrategyDescription }}</small>
                  </a-form-item>
                </a-col>
                <a-col :xs="24" :md="12">
                  <a-alert
                    type="info"
                    show-icon
                    message="策略关系"
                    description="并发转移策略决定未达到阈值时是否主动均衡；并发溢出优先级只在所有可用路由都达到转移阈值后参与排序。"
                  />
                </a-col>
              </a-row>

              <div v-if="settingsForm.route_strategy === 'smart'" class="smart-bias-panel">
                <div class="smart-bias-panel__title">Smart 评分权重</div>
                <a-row :gutter="16">
                  <a-col :xs="24" :md="12">
                    <a-form-item label="延迟敏感度">
                      <a-input-number
                        v-model:value="settingsForm.smart_latency_bias"
                        style="width: 100%"
                        :min="0"
                        :max="5"
                        :step="0.1"
                      />
                      <small class="field-help">越大越偏向选择 EWMA 延迟更低的路由（默认 1.0）。</small>
                    </a-form-item>
                  </a-col>
                  <a-col :xs="24" :md="12">
                    <a-form-item label="并发敏感度">
                      <a-input-number
                        v-model:value="settingsForm.smart_concurrency_bias"
                        style="width: 100%"
                        :min="0"
                        :max="5"
                        :step="0.1"
                      />
                      <small class="field-help">越大越偏向当前空闲的路由（默认 1.5）。</small>
                    </a-form-item>
                  </a-col>
                </a-row>
                <a-row :gutter="16">
                  <a-col :xs="24" :md="12">
                    <a-form-item label="失败惩罚强度">
                      <a-input-number
                        v-model:value="settingsForm.smart_failure_bias"
                        style="width: 100%"
                        :min="0"
                        :max="5"
                        :step="0.1"
                      />
                      <small class="field-help">控制连续失败 / 最近失败 / 失败率三类信号的权重（默认 1.0）。</small>
                    </a-form-item>
                  </a-col>
                  <a-col :xs="24" :md="12">
                    <a-form-item label="优先级 / 权重偏好">
                      <a-input-number
                        v-model:value="settingsForm.smart_priority_bias"
                        style="width: 100%"
                        :min="0"
                        :max="5"
                        :step="0.1"
                      />
                      <small class="field-help">越大越遵循路由的 priority / weight 设置（默认 0.5）。</small>
                    </a-form-item>
                  </a-col>
                </a-row>
              </div>

              <a-form-item label="GATEWAY_API_KEY">
                <a-input-password
                  v-model:value="settingsForm.gateway_api_key"
                  placeholder="用于 cc-switch / OpenAI 客户端请求网关的 Bearer Key"
                  allow-clear
                />
                <small class="field-help">保存后客户端需使用 Authorization: Bearer 这个 Key；留空时公开网关会被禁用。</small>
              </a-form-item>
            </div>

            <aside class="gateway-policy-settings-layout__side">
              <div class="gateway-policy-help">
                <div class="gateway-policy-help__title">策略说明</div>
                <div class="gateway-policy-help__grid">
                  <div
                    v-for="item in gatewayRouteStrategyOptions"
                    :key="item.value"
                    class="gateway-policy-help__item"
                    :class="{ 'gateway-policy-help__item--active': item.value === settingsForm.route_strategy }"
                  >
                    <strong>{{ item.label }}</strong>
                    <span>{{ item.description }}</span>
                  </div>
                </div>
                <div class="gateway-policy-help__notes">
                  <div v-for="item in gatewayStrategyDescriptionItems" :key="item.label">
                    <strong>{{ item.label }}</strong>
                    <span>{{ item.value }}</span>
                  </div>
                </div>
              </div>
            </aside>
          </div>
        </a-form>
      </a-modal>

      <a-modal
        v-model:open="addUpstreamOpen"
        title="添加上游 (api-supplier)"
        :confirm-loading="addUpstreamLoading"
        :ok-text="`保存并加入路由池`"
        cancel-text="取消"
        width="640px"
        @ok="submitAddUpstream"
        @cancel="resetAddUpstreamForm"
      >
        <a-alert
          type="info"
          show-icon
          message="该上游仅参与网关转发，不参与签到 / 同步。"
          description="保存后会作为 api-supplier 站点写入数据库，自动出现在路由池中，可独立启用/禁用、调整 priority/weight 与 route type。"
          style="margin-bottom: 12px"
        />
        <a-form layout="vertical">
          <a-row :gutter="16">
            <a-col :xs="24" :md="12">
              <a-form-item label="名称" required>
                <a-input
                  v-model:value="addUpstreamForm.name"
                  placeholder="便于识别，例如 acme-anthropic-1"
                  autocomplete="off"
                />
              </a-form-item>
            </a-col>
            <a-col :xs="24" :md="12">
              <a-form-item label="API 格式" required>
                <a-select
                  v-model:value="addUpstreamForm.api_format"
                  :options="[
                    { label: 'OpenAI / Codex', value: 'openai' },
                    { label: 'Anthropic / Claude', value: 'anthropic' },
                    { label: 'Gemini', value: 'gemini' },
                    { label: '通用 (general)', value: 'general' },
                  ]"
                />
                <small class="field-help">
                  决定路由分类（claude / codex / gemini）。
                </small>
              </a-form-item>
            </a-col>
          </a-row>
          <a-form-item label="Base URL" required>
            <a-input
              v-model:value="addUpstreamForm.base_url"
              placeholder="https://example.com 或 https://example.com/v1"
              autocomplete="off"
            />
            <small class="field-help">
              上游入口的根地址。如果上游需要带 /v1 前缀，可直接写在这里。
            </small>
          </a-form-item>
          <a-form-item label="API Key" required>
            <a-input-password
              v-model:value="addUpstreamForm.api_key"
              placeholder="sk-..."
              autocomplete="off"
            />
          </a-form-item>
          <a-row :gutter="16">
            <a-col :xs="24" :md="12">
              <a-form-item label="分组（可选）">
                <a-select
                  v-model:value="addUpstreamGroupNames"
                  mode="multiple"
                  :options="groupOptions"
                  :max-tag-count="4"
                  placeholder="选择分组"
                />
                <small class="field-help">
                  分组在全局 header 右上角维护，这里只选择。
                </small>
              </a-form-item>
            </a-col>
            <a-col :xs="24" :md="12">
              <a-form-item label="默认模型（可选）">
                <a-input
                  v-model:value="addUpstreamForm.preferred_model"
                  placeholder="claude-sonnet-4-6 / gemini-2.5-pro"
                  autocomplete="off"
                />
              </a-form-item>
            </a-col>
          </a-row>
          <a-form-item label="支持模型（可选）">
            <a-select
              v-model:value="addUpstreamForm.supported_models"
              mode="tags"
              :token-separators="[',', '，', '\n', '\t']"
              placeholder="留空表示该路由不会接收带 model 的精确匹配请求"
            />
            <small class="field-help">用于声明这个上游明确支持的模型 ID。请求体带 `model` 时，网关只会把请求发给这里精确声明过该模型的路由。</small>
          </a-form-item>
        </a-form>
      </a-modal>

      <a-modal
        v-model:open="routeModelsDialogOpen"
        title="编辑支持模型"
        width="640px"
        :confirm-loading="routeModelsDialogSaving"
        @ok="saveRouteModelsDialog"
      >
        <a-form layout="vertical">
          <a-form-item :label="routeModelsDialogRoute ? loadRouteLabel(routeModelsDialogRoute) : '当前路由'">
            <a-select
              v-model:value="routeModelsDialogValue"
              mode="tags"
              :token-separators="[',', '，', '\n', '\t']"
              placeholder="留空表示该路由不会接收带 model 的精确匹配请求"
            />
            <small class="field-help">这里配置的是路由当前生效的模型能力。请求体带 `model` 时，只有精确包含该模型 ID 的同类型路由会参与调度。</small>
          </a-form-item>
        </a-form>
      </a-modal>

      <a-drawer
        v-model:open="logsDrawerOpen"
        title="最近请求"
        width="min(1280px, 100vw)"
        placement="right"
      >
        <a-input
          v-model:value="logSearch"
          allow-clear
          placeholder="搜索路由 / 路径 / 失败原因"
          style="margin-bottom: 12px"
        />
        <div class="table-fill table-fill--management table-fill--drawer">
          <a-table
            :columns="logColumns"
            :data-source="filteredLogs"
            :pagination="{ pageSize: gatewayTablePageSize }"
            :row-key="logRowKey"
            size="small"
            :scroll="{ x: 1360, y: drawerTableY }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'created_at'">
                {{ formatTime(asLog(record).created_at) }}
              </template>
              <template v-else-if="column.key === 'request'">
                <span>{{ asLog(record).method }} {{ asLog(record).target_path }}</span>
                <a-tag v-if="asLog(record).is_stream" color="processing" class="stream-tag">流式</a-tag>
              </template>
              <template v-else-if="column.key === 'route'">
                <div class="table-cell-compact">
                  <div class="table-cell-compact__head">
                    <strong class="table-cell-compact__title">{{ logRouteLabel(asLog(record)) }}</strong>
                    <a-tooltip placement="right" :title="logRouteMeta(asLog(record))">
                      <InfoCircleOutlined class="table-info-icon" />
                    </a-tooltip>
                  </div>
                </div>
              </template>
              <template v-else-if="column.key === 'model'">
                <div class="table-cell-compact gateway-log-models">
                  <div class="table-cell-compact__meta">
                    <span class="table-cell-compact__meta-label">请求</span>
                    <span class="table-cell-compact__title">{{ logRequestedModel(asLog(record)) }}</span>
                  </div>
                  <div class="table-cell-compact__meta">
                    <span class="table-cell-compact__meta-label">命中</span>
                    <span class="table-cell-compact__title">{{ logActualModel(asLog(record)) }}</span>
                  </div>
                </div>
              </template>
              <template v-else-if="column.key === 'status'">
                <StatusPill :value="asLog(record).success ? 'success' : 'failed'" />
              </template>
              <template v-else-if="column.key === 'latency'">
                {{ asLog(record).latency_ms ? `${asLog(record).latency_ms} ms` : '暂无' }}
              </template>
              <template v-else-if="column.key === 'attempt'">
                {{ asLog(record).attempt_index }}
              </template>
              <template v-else-if="column.key === 'reason'">
                <a-tooltip v-if="asLog(record).failure_reason" placement="topLeft" :title="asLog(record).failure_reason">
                  <span class="table-ellipsis">{{ compactText(asLog(record).failure_reason, 44) }}</span>
                </a-tooltip>
                <span v-else>请求成功</span>
              </template>
            </template>
          </a-table>
        </div>
      </a-drawer>

      <a-drawer
        v-model:open="routeLogsDrawerOpen"
        :title="`路由请求历史 · ${routeLogsRoute ? loadRouteLabel(routeLogsRoute) : ''}`"
        width="min(1280px, 100vw)"
        placement="right"
      >
        <a-input
          v-model:value="routeLogSearch"
          allow-clear
          placeholder="搜索路径 / 失败原因 / 路由"
          style="margin-bottom: 12px"
        />
        <div class="table-fill table-fill--management table-fill--drawer">
          <a-table
            :columns="logColumns"
            :data-source="filteredRouteLogs"
            :loading="routeLogsLoading"
            :pagination="{ pageSize: gatewayTablePageSize }"
            :row-key="logRowKey"
            size="small"
            :scroll="{ x: 1360, y: drawerTableY }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'created_at'">
                {{ formatTime(asLog(record).created_at) }}
              </template>
              <template v-else-if="column.key === 'request'">
                <span>{{ asLog(record).method }} {{ asLog(record).target_path }}</span>
                <a-tag v-if="asLog(record).is_stream" color="processing" class="stream-tag">流式</a-tag>
              </template>
              <template v-else-if="column.key === 'route'">
                <div class="table-cell-compact">
                  <div class="table-cell-compact__head">
                    <strong class="table-cell-compact__title">{{ logRouteLabel(asLog(record)) }}</strong>
                    <a-tooltip placement="right" :title="logRouteMeta(asLog(record))">
                      <InfoCircleOutlined class="table-info-icon" />
                    </a-tooltip>
                  </div>
                </div>
              </template>
              <template v-else-if="column.key === 'model'">
                <div class="table-cell-compact gateway-log-models">
                  <div class="table-cell-compact__meta">
                    <span class="table-cell-compact__meta-label">请求</span>
                    <span class="table-cell-compact__title">{{ logRequestedModel(asLog(record)) }}</span>
                  </div>
                  <div class="table-cell-compact__meta">
                    <span class="table-cell-compact__meta-label">命中</span>
                    <span class="table-cell-compact__title">{{ logActualModel(asLog(record)) }}</span>
                  </div>
                </div>
              </template>
              <template v-else-if="column.key === 'status'">
                <StatusPill :value="asLog(record).success ? 'success' : 'failed'" />
              </template>
              <template v-else-if="column.key === 'latency'">
                {{ asLog(record).latency_ms ? `${asLog(record).latency_ms} ms` : '暂无' }}
              </template>
              <template v-else-if="column.key === 'attempt'">
                {{ asLog(record).attempt_index }}
              </template>
              <template v-else-if="column.key === 'reason'">
                <a-tooltip v-if="asLog(record).failure_reason" placement="topLeft" :title="asLog(record).failure_reason">
                  <span class="table-ellipsis">{{ compactText(asLog(record).failure_reason, 44) }}</span>
                </a-tooltip>
                <span v-else>请求成功</span>
              </template>
            </template>
          </a-table>
        </div>
      </a-drawer>

      <a-drawer
        v-model:open="routeDiagnosisOpen"
        :title="`路由诊断 · ${routeDiagnosis?.route_label || ''}`"
        width="520px"
        placement="right"
      >
        <a-spin :spinning="routeDiagnosisLoading">
          <div v-if="routeDiagnosis" class="route-diagnosis">
            <a-alert
              :type="routeDiagnosis.healthy ? 'success' : 'error'"
              show-icon
              :message="routeDiagnosis.healthy ? '路由关键检查通过' : '路由存在阻断项'"
              :description="`当前并发 ${routeDiagnosis.active_count}，检查时间 ${formatTime(routeDiagnosis.checked_at)}`"
            />
            <div class="route-diagnosis__list">
              <div
                v-for="item in routeDiagnosis.diagnostics"
                :key="item.label"
                class="route-diagnosis__item"
                :class="`route-diagnosis__item--${item.severity}`"
              >
                <div class="route-diagnosis__head">
                  <strong>{{ item.label }}</strong>
                  <a-tag :color="item.severity === 'ok' ? 'success' : item.severity === 'warning' ? 'warning' : 'error'">
                    {{ item.severity === 'ok' ? '正常' : item.severity === 'warning' ? '注意' : '异常' }}
                  </a-tag>
                </div>
                <div class="route-diagnosis__message">{{ item.message }}</div>
                <div class="route-diagnosis__detail">{{ item.detail }}</div>
              </div>
            </div>
          </div>
        </a-spin>
      </a-drawer>
    </div>
  </ShellLayout>
</template>

<style scoped>
.stream-tag {
  margin-left: 6px;
  font-size: 11px;
  line-height: 18px;
}

.gateway-log-route {
  display: grid;
  gap: 2px;
}

.table-cell-compact {
  display: grid;
  gap: 6px;
  min-width: 0;
}

.table-cell-compact__head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.table-cell-compact__title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.table-cell-compact__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.table-cell-compact__meta {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: #64748b;
  font-size: 12px;
}

.table-cell-compact__meta-label {
  flex: 0 0 auto;
  color: #94a3b8;
}

.gateway-log-models {
  gap: 3px;
}

.gateway-log-models .table-cell-compact__meta {
  gap: 6px;
}

.gateway-log-models .table-cell-compact__title {
  color: #24334d;
  font-family: 'IBM Plex Mono', monospace;
  font-size: 11px;
  font-weight: 700;
}

.table-info-icon {
  flex: 0 0 auto;
  color: #94a3b8;
  font-size: 13px;
  cursor: help;
}

.table-info-icon:hover {
  color: #5b6f8f;
}

.table-header-help {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.route-diagnosis {
  display: grid;
  gap: 14px;
}

.route-diagnosis__list {
  display: grid;
  gap: 10px;
}

.route-diagnosis__item {
  border: 1px solid #e5e7eb;
  border-left-width: 3px;
  border-radius: 8px;
  padding: 10px 12px;
  background: #ffffff;
}

.route-diagnosis__item--ok {
  border-left-color: #22c55e;
}

.route-diagnosis__item--warning {
  border-left-color: #f59e0b;
}

.route-diagnosis__item--error {
  border-left-color: #ef4444;
}

.route-diagnosis__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.route-diagnosis__message {
  margin-top: 6px;
  color: #24334d;
  font-size: 13px;
}

.route-diagnosis__detail {
  margin-top: 4px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.tooltip-detail-list {
  display: grid;
  gap: 6px;
  max-width: 360px;
}

.tooltip-detail-list strong {
  display: inline-block;
  margin-right: 6px;
}

.tooltip-detail-list span {
  word-break: break-word;
}

.table-ellipsis {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}

.gateway-actions-cell {
  flex-wrap: nowrap;
}

.gateway-actions-menu-button {
  width: 28px;
  padding-inline: 0;
}

.smart-bias-panel {
  border: 1px dashed var(--border-soft);
  border-radius: var(--radius-control);
  padding: 12px 16px 4px;
  margin-bottom: 16px;
  background: var(--bg-panel);
}

.smart-bias-panel__title {
  font-size: 13px;
  font-weight: 600;
  color: #24334d;
  margin-bottom: 8px;
}

.gateway-summary-row {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 16px;
  align-items: stretch;
  flex: 0 0 auto;
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
}

.gateway-fill {
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
  height: 100%;
}

.gateway-fill > .page-grid-fill {
  flex: 1 1 auto;
  min-height: 0;
  min-width: 0;
  margin-top: 0 !important;
}

.gateway-fill > .route-pool-card--standalone {
  flex: 1 1 auto;
  min-height: 0;
}

.gateway-monitor-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.gateway-overview-shell {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
  background:
    radial-gradient(circle at top left, rgba(37, 99, 235, 0.1), transparent 34%),
    linear-gradient(180deg, #ffffff, #f8fbff) !important;
  box-shadow:
    0 18px 42px rgba(30, 41, 59, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.9) !important;
}

.gateway-overview-shell::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(rgba(37, 99, 235, 0.04) 1px, transparent 1px),
    linear-gradient(90deg, rgba(37, 99, 235, 0.04) 1px, transparent 1px);
  background-size: 24px 24px;
  pointer-events: none;
  mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.46), transparent 100%);
}

.gateway-overview-shell :deep(.ant-card-body) {
  position: relative;
  z-index: 1;
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;
  padding: 18px;
  gap: 16px;
}

.gateway-overview-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.55fr) minmax(320px, 0.95fr);
  grid-template-areas:
    'usage route'
    'usage strategy'
    'activity activity';
  gap: 16px;
  min-height: 0;
  flex: 1 1 auto;
}

.gateway-panel {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.28);
  border-radius: 22px;
  background: linear-gradient(180deg, #ffffff, #f8fbff);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.9),
    0 16px 34px rgba(30, 41, 59, 0.08);
}

.gateway-panel::after {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(135deg, rgba(37, 99, 235, 0.08), transparent 34%);
}

.gateway-panel--usage {
  grid-area: usage;
}

.gateway-panel--activity {
  grid-area: activity;
  min-height: 320px;
}

.gateway-panel--route-status {
  grid-area: route;
}

.gateway-panel--strategy {
  grid-area: strategy;
}

.gateway-panel__head {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  padding: 18px 18px 0;
}

.gateway-panel__title {
  color: #172033;
  font-size: 16px;
  font-weight: 700;
}

.gateway-usage-card__sep {
  color: #64748b;
  font-size: 12px;
}

.gateway-usage-input {
  height: 34px;
  min-width: 196px;
  border: 1px solid rgba(148, 163, 184, 0.34);
  border-radius: 12px;
  padding: 0 12px;
  color: #172033;
  background: #ffffff;
  font-size: 13px;
  outline: none;
}

.gateway-usage-input:focus {
  border-color: rgba(110, 168, 255, 0.5);
  box-shadow: 0 0 0 2px rgba(77, 144, 254, 0.18);
}

.gateway-usage-summary {
  position: relative;
  z-index: 1;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  padding: 14px 18px 0;
}

.gateway-usage-table {
  position: relative;
  z-index: 1;
  padding: 14px 18px 18px;
}

.gateway-usage-table :deep(.ant-table),
.gateway-usage-table :deep(.ant-table-container),
.gateway-usage-table :deep(.ant-table-content) {
  background: transparent !important;
}

.gateway-usage-table :deep(.ant-table-thead > tr > th) {
  background: #f1f5f9 !important;
  color: #334155;
  border-bottom-color: rgba(148, 163, 184, 0.2) !important;
}

.gateway-usage-table :deep(.ant-table-tbody > tr > td) {
  color: #172033;
  background: rgba(255, 255, 255, 0.82);
  border-bottom-color: rgba(148, 163, 184, 0.16) !important;
}

.gateway-usage-summary__item {
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 18px;
  padding: 14px 16px;
  background: linear-gradient(180deg, #ffffff, #f8fbff);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.9);
}

.gateway-usage-summary__item span {
  display: block;
  margin-bottom: 6px;
  color: #64748b;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.gateway-usage-summary__item strong {
  color: #172033;
  font-size: 22px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.gateway-usage-summary__item--primary {
  background: linear-gradient(145deg, #eff6ff, #ffffff);
}

.gateway-usage-summary__item--success {
  background: linear-gradient(145deg, #ecfdf5, #ffffff);
}

.gateway-usage-summary__item--warning {
  background: linear-gradient(145deg, #fff7ed, #ffffff);
}

.gateway-usage-summary__item--info {
  background: linear-gradient(145deg, #ecfeff, #ffffff);
}

.gateway-active-feed-panel__pulse {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 12px;
  border: 1px solid rgba(148, 163, 184, 0.26);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.82);
  color: #64748b;
  font-family: 'IBM Plex Mono', monospace;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
}

.gateway-active-feed-panel__pulse::before {
  content: '';
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: currentColor;
}

.gateway-active-feed-panel__pulse--active {
  border-color: rgba(37, 99, 235, 0.3);
  background: rgba(37, 99, 235, 0.08);
  color: #1d4ed8;
}

.gateway-active-feed {
  display: grid;
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 2px 0 0;
}

.gateway-active-feed--embedded {
  position: relative;
  z-index: 1;
  align-content: start;
  max-height: none;
  margin: 0 18px 18px;
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 18px;
  background: rgba(248, 251, 255, 0.86);
  padding: 8px 14px;
}

.gateway-active-feed__item {
  display: grid;
  grid-template-columns: 18px minmax(0, 1fr);
  gap: 10px;
  min-width: 0;
  padding: 11px 0;
  border-bottom: 1px solid rgba(148, 163, 184, 0.16);
}

.gateway-active-feed__item:last-child {
  border-bottom: 0;
}

.gateway-active-feed__item--completed {
  color: #64748b;
}

.gateway-active-feed__rail {
  position: relative;
  display: flex;
  justify-content: center;
  min-height: 100%;
}

.gateway-active-feed__dot {
  width: 10px;
  height: 10px;
  margin-top: 6px;
  border-radius: 999px;
  background: #39c8ff;
  box-shadow: 0 0 0 4px rgba(57, 200, 255, 0.14);
}

.gateway-active-feed__item--completed .gateway-active-feed__dot {
  background: #f08a5a;
  box-shadow: 0 0 0 4px rgba(240, 138, 90, 0.12);
}

.gateway-active-feed__main {
  display: grid;
  grid-template-columns: minmax(160px, 1.2fr) auto minmax(260px, 1.5fr) minmax(220px, 1.2fr);
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.gateway-active-feed__route {
  min-width: 0;
  overflow: hidden;
  color: #172033;
  font-size: 13px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gateway-active-feed__badges {
  flex: 0 0 auto;
  flex-wrap: nowrap;
  white-space: nowrap;
}

.gateway-active-feed__request,
.gateway-active-feed__meta {
  display: flex;
  flex-wrap: nowrap;
  gap: 6px 10px;
  min-width: 0;
  overflow: hidden;
}

.gateway-active-feed__request span,
.gateway-active-feed__meta span {
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gateway-active-feed__request span {
  color: #475569;
  font-family: 'IBM Plex Mono', monospace;
  font-size: 12px;
  font-weight: 600;
}

.gateway-active-feed__meta span {
  color: #64748b;
  font-size: 11px;
}

.gateway-metric-card {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
  background: linear-gradient(180deg, #ffffff, #f8fbff) !important;
  box-shadow:
    0 16px 34px rgba(30, 41, 59, 0.08),
    inset 0 1px 0 rgba(255, 255, 255, 0.9) !important;
}

.gateway-metric-card::before {
  content: '';
  position: absolute;
  inset: 0;
  background: radial-gradient(circle at right top, rgba(37, 99, 235, 0.08), transparent 42%);
  pointer-events: none;
}

.gateway-metric-card :deep(.ant-card-body) {
  position: relative;
  z-index: 1;
  display: grid;
  gap: 6px;
  padding: 18px 18px 16px;
}

.gateway-metric-card__label {
  color: #64748b;
  font-size: 12px;
  font-weight: 600;
}

.gateway-metric-card__value {
  color: #172033;
  font-size: clamp(2rem, 2vw, 2.4rem);
  font-weight: 800;
  letter-spacing: -0.02em;
  line-height: 1;
}

.gateway-metric-card--primary {
  background: linear-gradient(180deg, #ffffff, #f8fbff) !important;
}

.gateway-metric-card--info {
  background: linear-gradient(180deg, #ffffff, #f8fbff) !important;
}

.gateway-metric-card--success {
  background: linear-gradient(180deg, #ffffff, #f8fbff) !important;
}

.gateway-metric-card--warning {
  background: linear-gradient(180deg, #ffffff, #f8fbff) !important;
}

.gateway-metric-card--neutral {
  background: linear-gradient(180deg, #ffffff, #f8fbff) !important;
}

.gateway-policy-settings-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 300px;
  gap: 18px;
  align-items: start;
}

.gateway-policy-settings-layout__main,
.gateway-policy-settings-layout__side {
  min-width: 0;
}

.gateway-policy-help {
  border: 1px solid #d9e2ef;
  border-radius: 8px;
  background: #f8fafc;
  padding: 14px;
}

.gateway-policy-help__title {
  font-size: 13px;
  font-weight: 700;
  color: #24334d;
  margin-bottom: 10px;
}

.gateway-policy-help__grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 8px;
}

.gateway-policy-help__item,
.gateway-policy-help__notes > div {
  border: 1px solid #e3eaf4;
  border-radius: 6px;
  background: #ffffff;
  padding: 10px;
}

.gateway-policy-help__item--active {
  border-color: #7aa7d8;
  background: #eef6ff;
}

.gateway-policy-help strong {
  display: block;
  color: #24334d;
  font-size: 12px;
  margin-bottom: 4px;
}

.gateway-policy-help span {
  display: block;
  color: #5f6f86;
  font-size: 12px;
  line-height: 1.55;
}

.gateway-policy-help__notes {
  display: grid;
  grid-template-columns: 1fr;
  gap: 8px;
  margin-top: 8px;
}

.gateway-access {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  min-width: 0;
  max-width: min(100%, 980px);
}

.gateway-access code {
  display: block;
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  padding: 7px 12px;
  border: 1px solid rgba(148, 163, 184, 0.3);
  border-radius: 12px;
  background: #ffffff;
  color: #24334d;
  font-family: 'IBM Plex Mono', monospace;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gateway-access code:first-child {
  max-width: min(44vw, 560px);
}

.gateway-access code:nth-of-type(2) {
  max-width: min(24vw, 300px);
}

.gateway-access__hint {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
  height: 30px;
  padding: 0 10px;
  border: 1px solid rgba(37, 99, 235, 0.24);
  border-radius: 999px;
  background: rgba(37, 99, 235, 0.08);
  color: #1d4ed8;
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
  cursor: help;
  white-space: nowrap;
}

.gateway-access__hint svg {
  width: 13px;
  height: 13px;
}

.gateway-access .ant-btn {
  flex: 0 0 auto;
}

.route-management-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-width: 0;
}

.route-management-heading {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.route-management-heading strong {
  color: #24334d;
  font-size: 15px;
  font-weight: 800;
}

.route-management-heading span {
  display: inline-flex;
  align-items: center;
  height: 26px;
  padding: 0 10px;
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-control);
  background: #fff;
  color: #5f6f86;
  font-family: 'IBM Plex Mono', monospace;
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.gateway-access--route {
  flex: 1 1 520px;
  max-width: min(100%, 920px);
}

.gateway-access--route code:first-child {
  max-width: min(28vw, 380px);
}

.gateway-access--route code:nth-of-type(2) {
  max-width: min(18vw, 220px);
}

.route-probe-control {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.route-probe-progress {
  display: grid;
  gap: 4px;
  width: 168px;
  min-width: 140px;
}

.route-probe-progress__meta {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  color: #526684;
  font-size: 11px;
  font-weight: 700;
  line-height: 1;
  white-space: nowrap;
}

.route-probe-progress__bar {
  height: 5px;
  overflow: hidden;
  border-radius: 999px;
  background: rgba(170, 190, 222, 0.32);
}

.route-probe-progress__bar span {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #2f6ff6, #19a66a);
  transition: width 180ms ease;
}

.route-probe-progress--balance .route-probe-progress__bar span {
  background: linear-gradient(90deg, #0f9f7a, #2f6ff6);
}

.route-pool-status {
  position: relative;
  z-index: 1;
  display: grid;
  gap: 10px;
  padding: 14px 18px 0;
}

.route-pool-status__card {
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 16px;
  padding: 12px 14px;
  background: rgba(255, 255, 255, 0.82);
}

.route-pool-status__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}

.route-pool-status__head span {
  color: #64748b;
  font-size: 12px;
}

.route-pool-status__head strong {
  color: #172033;
  font-size: 18px;
  font-weight: 700;
}

.route-pool-status__bar {
  width: 100%;
  height: 8px;
  border-radius: 999px;
  overflow: hidden;
  background: rgba(148, 163, 184, 0.12);
}

.route-pool-status__bar span {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, rgba(110, 168, 255, 0.86), rgba(79, 214, 200, 0.9));
}

.route-pool-status__card--success .route-pool-status__bar span {
  background: linear-gradient(90deg, #3ddc97, #82f4a8);
}

.route-pool-status__card--warning .route-pool-status__bar span {
  background: linear-gradient(90deg, #ffb34d, #ffd27f);
}

.route-pool-status__card--danger .route-pool-status__bar span {
  background: linear-gradient(90deg, #ff7d61, #ffaf72);
}

.route-pool-preview {
  position: relative;
  z-index: 1;
  display: grid;
  gap: 10px;
  padding: 14px 18px 18px;
}

.route-pool-preview__row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
  align-items: center;
  padding: 12px 14px;
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.82);
}

.route-pool-preview__main {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.route-pool-preview__main strong {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #172033;
  font-size: 13px;
}

.route-pool-preview__main span {
  color: #64748b;
  font-size: 12px;
}

.route-pool-preview__meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.gateway-strategy-board {
  position: relative;
  z-index: 1;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  padding: 14px 18px 18px;
}

.gateway-strategy-board__item {
  display: grid;
  gap: 10px;
  padding: 14px 14px 12px;
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.82);
}

.gateway-strategy-board__summary {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: baseline;
}

.gateway-strategy-board__summary span {
  color: #64748b;
  font-size: 12px;
}

.gateway-strategy-board__summary strong {
  color: #172033;
  font-size: 20px;
  font-weight: 700;
}

.gateway-strategy-board__track {
  height: 8px;
  border-radius: 999px;
  overflow: hidden;
  background: rgba(148, 163, 184, 0.12);
}

.gateway-strategy-board__track span {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #6ea8ff, #7af8d0);
}

.route-pool-card :deep(.ant-card-body) {
  padding: 0;
}

.route-pool-card .card-shell {
  gap: 0;
}

.route-pool-filters {
  display: flex;
  flex-wrap: nowrap;
  justify-content: space-between;
  gap: 12px;
  align-items: center;
  margin-bottom: 0;
  min-width: 0;
  padding: var(--table-filter-gutter);
  border-bottom: 1px solid var(--border-muted);
  background: var(--bg-panel);
}

.route-pool-type-tabs {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 0 0 auto;
  min-width: 0;
}

.route-pool-searchbar {
  display: flex;
  flex: 1 1 auto;
  flex-wrap: wrap;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.route-pool-filter {
  flex: 1 1 160px;
  min-width: 140px;
  max-width: 260px;
}

.route-pool-filter--search,
.route-pool-filter--group {
  flex-basis: 220px;
  max-width: 320px;
}

.route-pool-filter--compact {
  flex-basis: 128px;
  max-width: 180px;
}

.route-pool-filter :deep(.ant-select-selector),
.route-pool-filter :deep(.ant-input) {
  min-width: 0;
}

.route-pool-filter-switch,
.route-pool-clear {
  flex: 0 0 auto;
  white-space: nowrap;
}

.route-type-select :deep(.ant-select-selector) {
  border-color: rgba(148, 163, 184, 0.28) !important;
  color: #24334d !important;
  font-weight: 700;
}

.route-issue-tag {
  margin-left: 6px;
  font-size: 11px;
  line-height: 18px;
}

.route-type-select :deep(.ant-select-selection-item),
.route-type-select :deep(.ant-select-arrow) {
  color: inherit !important;
}

.route-type-select--claude :deep(.ant-select-selector) {
  background: #fff7ed !important;
  color: #c2410c !important;
}

.route-type-select--codex :deep(.ant-select-selector) {
  background: #eff6ff !important;
  color: #1d4ed8 !important;
}

.route-type-select--gemini :deep(.ant-select-selector) {
  background: #ecfeff !important;
  color: #0e7490 !important;
}

.route-type-option {
  display: inline-flex;
  align-items: center;
  min-width: 74px;
  padding: 2px 9px;
  border-radius: 999px;
  color: #24334d;
  font-size: 12px;
  font-weight: 800;
}

.route-type-option--claude {
  background: #fff7ed;
  color: #c2410c;
}

.route-type-option--codex {
  background: #eff6ff;
  color: #1d4ed8;
}

.route-type-option--gemini {
  background: #ecfeff;
  color: #0e7490;
}

.priority-dialog {
  display: grid;
  gap: 14px;
}

.priority-route-name {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.priority-route-name strong {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.priority-number {
  font-family: 'IBM Plex Mono', monospace;
  font-weight: 700;
}

.priority-route-row--current > td {
  background: #eef6ff !important;
}

.priority-editor {
  display: grid;
  gap: 10px;
  padding: 12px;
  border: 1px solid var(--border-muted);
  border-radius: var(--radius-control);
  background: var(--bg-panel);
}

.priority-editor__summary,
.priority-editor__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 10px;
}

.priority-editor__summary {
  color: #64748b;
  font-size: 12px;
}

.priority-editor__summary strong {
  color: #24334d;
  font-size: 13px;
}

.priority-editor__input {
  width: 150px;
}

.gateway-concurrency {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-width: 62px;
  height: 26px;
  padding: 0 10px;
  border: 1px solid #cbd5e1;
  border-radius: 999px;
  background: #f8fafc;
  color: #0f172a;
  font-family: 'IBM Plex Mono', monospace;
  font-size: 12px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.gateway-concurrency--active {
  border-color: #10b981;
  background: #ecfdf5;
  color: #064e3b;
  box-shadow: inset 0 0 0 1px rgba(16, 185, 129, 0.16);
}

.gateway-concurrency__current {
  color: #0f172a;
}

.gateway-concurrency__separator,
.gateway-concurrency__limit {
  color: #475569;
}

.gateway-concurrency--active .gateway-concurrency__current {
  color: #047857;
}

.gateway-concurrency--active .gateway-concurrency__separator,
.gateway-concurrency--active .gateway-concurrency__limit {
  color: #0f172a;
}

.gateway-latency {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px;
  border-radius: 999px;
  background: #f1f5f9;
  color: #475569;
  font-variant-numeric: tabular-nums;
}

.gateway-latency__dot {
  width: 9px;
  height: 9px;
  border-radius: 999px;
  background: currentColor;
  box-shadow: 0 0 0 0 currentColor;
}

.gateway-latency__value {
  font-weight: 600;
  letter-spacing: 0.01em;
}

.gateway-latency--low {
  background: rgba(34, 197, 94, 0.14);
  color: #22c55e;
}

.gateway-latency--medium {
  background: rgba(245, 158, 11, 0.16);
  color: #f59e0b;
}

.gateway-latency--high {
  background: rgba(239, 68, 68, 0.18);
  color: #ef4444;
}

/* 全站固定明色主题：覆盖网关监控旧样式，仅作用于本页。 */
.gateway-panel__title,
.gateway-usage-summary__item strong,
.gateway-active-feed__route,
.gateway-metric-card__value {
  color: #172033;
}

.gateway-usage-card__sep,
.gateway-usage-summary__item span,
.gateway-active-feed-panel__pulse,
.gateway-active-feed__request span,
.gateway-active-feed__meta span,
.gateway-metric-card__label {
  color: #64748b;
}

.gateway-active-feed-panel__pulse {
  border-color: rgba(148, 163, 184, 0.26);
  background: rgba(255, 255, 255, 0.82);
}

.gateway-overview-shell {
  border-color: rgba(148, 163, 184, 0.28) !important;
  background:
    radial-gradient(circle at top left, rgba(37, 99, 235, 0.1), transparent 34%),
    linear-gradient(180deg, #ffffff, #f8fbff) !important;
  box-shadow:
    0 18px 42px rgba(30, 41, 59, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.9) !important;
}

.gateway-overview-shell::before {
  background:
    linear-gradient(rgba(37, 99, 235, 0.04) 1px, transparent 1px),
    linear-gradient(90deg, rgba(37, 99, 235, 0.04) 1px, transparent 1px);
}

.gateway-panel,
.gateway-metric-card {
  border-color: rgba(148, 163, 184, 0.28) !important;
  background: linear-gradient(180deg, #ffffff, #f8fbff) !important;
  box-shadow:
    0 16px 34px rgba(30, 41, 59, 0.08),
    inset 0 1px 0 rgba(255, 255, 255, 0.9) !important;
}

.gateway-panel::after,
.gateway-metric-card::before {
  background: radial-gradient(circle at right top, rgba(37, 99, 235, 0.08), transparent 42%);
}

.gateway-usage-input {
  border-color: rgba(148, 163, 184, 0.34);
  background: #ffffff;
  color: #172033;
}

.gateway-usage-table :deep(.ant-table-thead > tr > th) {
  background: #f1f5f9 !important;
  color: #334155;
  border-bottom-color: rgba(148, 163, 184, 0.2) !important;
}

.gateway-usage-table :deep(.ant-table-tbody > tr > td) {
  color: #172033;
  background: rgba(255, 255, 255, 0.82);
  border-bottom-color: rgba(148, 163, 184, 0.16) !important;
}

.gateway-usage-summary__item,
.gateway-usage-summary__item--primary,
.gateway-usage-summary__item--success,
.gateway-usage-summary__item--warning,
.gateway-usage-summary__item--info {
  border-color: rgba(148, 163, 184, 0.24);
  background: linear-gradient(180deg, #ffffff, #f8fbff);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.9);
}

.gateway-active-feed--embedded {
  border-color: rgba(148, 163, 184, 0.24);
  background: rgba(248, 251, 255, 0.86);
}

.gateway-active-feed__item {
  border-bottom-color: rgba(148, 163, 184, 0.16);
}

.gateway-metric-card--primary,
.gateway-metric-card--info,
.gateway-metric-card--success,
.gateway-metric-card--warning,
.gateway-metric-card--neutral {
  background: linear-gradient(180deg, #ffffff, #f8fbff) !important;
}

.gateway-route-page {
  gap: 16px;
}

.gateway-route-page .page-toolbar {
  padding: 4px 0 2px;
}

.gateway-route-page .route-management-heading strong {
  color: #1c2f57;
  font-size: 16px;
  font-weight: 800;
}

.gateway-route-page .route-management-heading span {
  border-color: rgba(148, 163, 184, 0.28);
  background: rgba(255, 255, 255, 0.86);
  color: #5b6f94;
}

.gateway-route-page .route-pool-card--standalone {
  border: 1px solid rgba(178, 200, 235, 0.48) !important;
  border-radius: 24px !important;
  background:
    radial-gradient(circle at top right, rgba(92, 149, 255, 0.08), transparent 32%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(246, 249, 255, 0.96)) !important;
  box-shadow:
    0 18px 40px rgba(66, 113, 230, 0.09),
    inset 0 1px 0 rgba(255, 255, 255, 0.92) !important;
}

.gateway-route-page .route-pool-card--standalone :deep(.ant-card-head) {
  min-height: 70px;
  padding: 0 18px;
  border-bottom-color: rgba(178, 200, 235, 0.34);
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(247, 250, 255, 0.98));
}

.gateway-route-page .route-pool-card--standalone :deep(.ant-card-head-title) {
  padding: 20px 0;
  color: #274284;
  font-size: 22px;
  font-weight: 700;
}

.gateway-route-page .route-pool-card--standalone :deep(.ant-card-extra) {
  padding: 14px 0;
}

.gateway-route-page .route-pool-card :deep(.ant-card-body) {
  border-radius: 0 0 24px 24px;
  overflow: hidden;
}

.gateway-route-page .route-pool-filters {
  padding: 12px 14px;
  border-bottom-color: rgba(178, 200, 235, 0.3);
  background: rgba(255, 255, 255, 0.82);
}

.gateway-route-page .route-pool-filter :deep(.ant-select-selector),
.gateway-route-page .route-pool-filter :deep(.ant-input) {
  border-radius: 12px !important;
  border-color: rgba(188, 211, 245, 0.88) !important;
  background: rgba(255, 255, 255, 0.92) !important;
}

.gateway-route-page .route-pool-clear.ant-btn,
.gateway-route-page .route-pool-filter-switch {
  border-radius: 12px;
}

.gateway-route-page .route-pool-card .card-shell {
  background: transparent;
}

.gateway-route-page .table-fill--management {
  background: transparent;
}

.gateway-route-page .table-fill--management :deep(.ant-table-thead > tr > th) {
  height: 40px;
  padding: 0 14px !important;
  background: linear-gradient(180deg, #f1f5ff, #edf3ff) !important;
  color: #516cae !important;
  font-size: 13px;
}

.gateway-route-page .table-fill--management :deep(.ant-table-tbody > tr > td) {
  padding: 9px 14px !important;
  font-size: 13px;
  color: #2e4381;
  border-bottom: 1px solid rgba(104, 141, 241, 0.08);
  background: rgba(255, 255, 255, 0.88);
}

.gateway-route-page .table-fill--management :deep(.ant-table-tbody > tr:hover > td) {
  background: linear-gradient(180deg, rgba(246, 249, 255, 0.96), rgba(241, 246, 255, 0.92)) !important;
}

.gateway-route-page .route-pool-preview__row,
.gateway-route-page .gateway-active-feed__item {
  border-bottom-color: rgba(148, 163, 184, 0.16);
}

.gateway-route-page .gateway-actions-cell {
  flex-wrap: nowrap;
}

.gateway-route-page .gateway-actions-menu-button {
  width: 28px;
  padding-inline: 0;
}

@media (max-width: 720px) {
  .route-management-toolbar,
  .route-pool-filters,
  .route-pool-searchbar {
    align-items: stretch;
    flex-direction: column;
  }

  .gateway-access--route {
    flex-basis: auto;
  }

  .gateway-access--route code:first-child,
  .gateway-access--route code:nth-of-type(2) {
    max-width: 100%;
  }

  .route-pool-type-tabs {
    flex-wrap: wrap;
  }

  .route-probe-control {
    align-items: stretch;
    flex-direction: column;
  }

  .route-probe-progress {
    width: 100%;
  }

  .route-pool-filter {
    flex-basis: 100%;
    max-width: none;
  }

  .gateway-policy-help__grid,
  .gateway-policy-help__notes {
    grid-template-columns: 1fr;
  }

  .gateway-metrics {
    grid-auto-columns: minmax(220px, 1fr);
  }

  .gateway-usage-summary,
  .gateway-strategy-board {
    grid-template-columns: 1fr;
  }

  .route-pool-preview__row {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 960px) {
  .gateway-policy-settings-layout {
    grid-template-columns: 1fr;
  }

  .gateway-overview-grid {
    grid-template-columns: 1fr;
    grid-template-areas:
      'usage'
      'route'
      'strategy'
      'activity';
  }

  .gateway-usage-input {
    width: 100%;
  }

  .gateway-usage-summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .gateway-panel__head {
    flex-direction: column;
  }

  .gateway-active-feed__main {
    grid-template-columns: 1fr;
    align-items: start;
  }

  .gateway-active-feed__request,
  .gateway-active-feed__meta {
    flex-wrap: wrap;
  }

  .gateway-strategy-board {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (min-width: 961px) and (max-width: 1320px) {
  .gateway-overview-grid {
    grid-template-columns: minmax(0, 1.2fr) minmax(300px, 0.9fr);
  }

  .gateway-strategy-board {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
