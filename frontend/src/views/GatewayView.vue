<script setup lang="ts">
import { PlusOutlined, CopyOutlined, ReloadOutlined, SettingOutlined, SyncOutlined, InfoCircleOutlined, HistoryOutlined } from '@ant-design/icons-vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref, type ComponentPublicInstance } from 'vue'
import {
  createSite,
  getGatewayLogs,
  getGatewayOverview,
  getGatewayRouteLogs,
  probeGatewayRoute,
  probeGatewayRoutes,
  getGatewayRoutes,
  getGatewaySettings,
  getSiteGroups,
  probeGatewayRouteBalance,
  refreshSiteSummaries,
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
import { balanceTone, formatBalance, formatGroupNames, normalizeGroupNames, parseGroupNames } from '../format'
import { useToast } from '../toast'
import type { GatewayLog, GatewayOverview, GatewayRoute, GatewayRouteProbeResult, GatewaySettingsData, GatewayStrategyStat, GatewayTrendBucket, SiteGroup, SiteSummary } from '../types'

const toast = useToast()
const loading = ref(false)
const autoRefreshing = ref(false)
const probeLoading = ref(false)
const settingsLoading = ref(false)
const settingsOpen = ref(false)
const logsDrawerOpen = ref(false)
const routeLogsDrawerOpen = ref(false)
const addUpstreamOpen = ref(false)
const addUpstreamLoading = ref(false)
type ApiFormatOption = 'openai' | 'anthropic' | 'gemini' | 'general'
const addUpstreamForm = reactive<{
  name: string
  base_url: string
  api_key: string
  api_format: ApiFormatOption
  group_name: string
  preferred_model: string
}>({
  name: '',
  base_url: '',
  api_key: '',
  api_format: 'openai',
  group_name: '',
  preferred_model: '',
})

function resetAddUpstreamForm() {
  addUpstreamForm.name = ''
  addUpstreamForm.base_url = ''
  addUpstreamForm.api_key = ''
  addUpstreamForm.api_format = 'openai'
  addUpstreamForm.group_name = ''
  addUpstreamGroupNames.value = []
  addUpstreamForm.preferred_model = ''
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
const logs = ref<GatewayLog[]>([])
const routeLogs = ref<GatewayLog[]>([])
const siteGroups = ref<SiteGroup[]>([])
const routeLogsLoading = ref(false)
const routeLogsRoute = ref<GatewayRoute | null>(null)
const includeDisabled = ref(true)
let autoRefreshTimer: number | null = null
let lastAutoRefreshAt = 0
const gatewayTablePageSize = 20
const gatewayAutoRefreshMs = 10_000
const selectedGroups = ref<string[]>([])
const addUpstreamGroupNames = ref<string[]>([])
const selectedRouteTypes = ref<Array<GatewayRoute['route_type']>>([])
const selectedCircuitStates = ref<Array<'closed' | 'open' | 'half_open' | 'paused'>>([])
const selectedPackageStates = ref<Array<'with_package' | 'without_package'>>([])
const selectedBalanceStates = ref<Array<'with_balance' | 'without_balance' | 'zero_or_negative'>>([])
const selectedIssueStates = ref<Array<'with_error' | 'without_error'>>([])
const probingRouteIds = ref<number[]>([])
const balanceProbingRouteIds = ref<number[]>([])
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
  route_concurrency_limit: 5,
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
  { title: '套餐', key: 'package', width: 180, sorter: (a: GatewayRoute, b: GatewayRoute) => String(a.package_display ?? '').localeCompare(String(b.package_display ?? ''), 'zh-CN') },
  { title: '分组', key: 'group', width: 110, sorter: (a: GatewayRoute, b: GatewayRoute) => String(a.group_name ?? '').localeCompare(String(b.group_name ?? ''), 'zh-CN') },
  { title: '优先级', key: 'priority', width: 90, sorter: (a: GatewayRoute, b: GatewayRoute) => a.route_priority - b.route_priority },
  { title: '权重', key: 'weight', width: 80, sorter: (a: GatewayRoute, b: GatewayRoute) => a.weight - b.weight },
  { title: '当前并发', key: 'concurrency', width: 110, sorter: (a: GatewayRoute, b: GatewayRoute) => a.active_concurrency - b.active_concurrency },
  { title: '熔断状态', key: 'circuit', width: 120, sorter: (a: GatewayRoute, b: GatewayRoute) => String(a.circuit_state).localeCompare(String(b.circuit_state), 'zh-CN') },
  { title: '成功率', key: 'success_rate', width: 110, sorter: (a: GatewayRoute, b: GatewayRoute) => a.success_rate - b.success_rate },
  { title: '延迟', key: 'latency', width: 138, sorter: (a: GatewayRoute, b: GatewayRoute) => (a.last_latency_ms ?? a.avg_latency_ms ?? Infinity) - (b.last_latency_ms ?? b.avg_latency_ms ?? Infinity) },
  { title: '最后异常', key: 'error', width: 220, sorter: (a: GatewayRoute, b: GatewayRoute) => new Date(routeLastUpdateTime(a) ?? 0).getTime() - new Date(routeLastUpdateTime(b) ?? 0).getTime() },
  { title: '操作', key: 'actions', width: 360, fixed: 'right' as const },
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

const circuitStateOptions: Array<{ label: string; value: 'closed' | 'open' | 'half_open' | 'paused' }> = [
  { label: '健康', value: 'closed' },
  { label: '熔断中', value: 'open' },
  { label: '半开探测', value: 'half_open' },
  { label: '已停用', value: 'paused' },
]

const packageStateOptions: Array<{ label: string; value: 'with_package' | 'without_package' }> = [
  { label: '有套餐', value: 'with_package' },
  { label: '无套餐', value: 'without_package' },
]

const balanceStateOptions: Array<{ label: string; value: 'with_balance' | 'without_balance' | 'zero_or_negative' }> = [
  { label: '有余额', value: 'with_balance' },
  { label: '无余额信息', value: 'without_balance' },
  { label: '零/负余额', value: 'zero_or_negative' },
]

const issueStateOptions: Array<{ label: string; value: 'with_error' | 'without_error' }> = [
  { label: '有异常', value: 'with_error' },
  { label: '无异常', value: 'without_error' },
]

const logColumns = [
  { title: '时间', key: 'created_at', width: 180, sorter: (a: GatewayLog, b: GatewayLog) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime() },
  { title: '请求', key: 'request', width: 210, sorter: (a: GatewayLog, b: GatewayLog) => `${a.method} ${a.target_path}`.localeCompare(`${b.method} ${b.target_path}`, 'zh-CN') },
  { title: '路由', key: 'route', width: 300, sorter: (a: GatewayLog, b: GatewayLog) => logRouteLabel(a).localeCompare(logRouteLabel(b), 'zh-CN') },
  { title: '结果', key: 'status', width: 120, sorter: (a: GatewayLog, b: GatewayLog) => Number(a.success) - Number(b.success) },
  { title: '延迟', key: 'latency', width: 100, sorter: (a: GatewayLog, b: GatewayLog) => (a.latency_ms ?? Infinity) - (b.latency_ms ?? Infinity) },
  { title: '尝试', key: 'attempt', width: 90, sorter: (a: GatewayLog, b: GatewayLog) => a.attempt_index - b.attempt_index },
  { title: '说明', key: 'reason' },
]

type MetricTone = 'primary' | 'success' | 'warning' | 'info' | 'neutral'

const gatewayRouteStrategyOptions: Array<{ label: string; value: GatewaySettingsData['route_strategy']; description: string }> = [
  { label: '智能综合评分', value: 'smart', description: '综合延迟、当前并发、失败记录、优先级和权重，自动挑选当前最合适的路由。' },
  { label: '轮询均衡', value: 'round_robin', description: '在健康路由之间按顺序轮换，并尊重权重，适合希望请求分散到多个上游的场景。' },
  { label: '低延迟优先', value: 'latency_first', description: '优先使用历史延迟更低的路由，适合更看重响应速度的场景。' },
  { label: '优先级优先', value: 'priority', description: '优先选择 priority 数值更小的路由，再结合权重和健康状态排序，适合固定主备线路。' },
]

const gatewayOverflowStrategyOptions: Array<{ label: string; value: GatewaySettingsData['concurrency_overflow_strategy']; description: string }> = [
  { label: '低延迟优先', value: 'latency_first', description: '所有可用路由都达到并发上限时，仍优先尝试延迟较低的路由。' },
  { label: '按顺序优先', value: 'sequential', description: '所有可用路由都达到并发上限时，按当前策略排序继续尝试后备路由。' },
]

const metricCards = computed<Array<{ title: string; value: string | number; tone: MetricTone }>>(() => {
  if (!overview.value) {
    return []
  }
  const ov = overview.value
  return [
    { title: '总路由数', value: ov.total_routes, tone: 'primary' },
    { title: '健康路由', value: ov.healthy_routes, tone: 'success' },
    { title: '熔断中', value: ov.open_circuit_routes, tone: 'warning' },
    { title: '总额度', value: ov.total_balance_display || '暂无', tone: 'info' },
    { title: '当前并发', value: ov.active_concurrency, tone: 'primary' },
    { title: '24h 请求数', value: ov.request_count_24h, tone: 'info' },
    { title: '24h 成功率', value: `${ov.success_rate_24h}%`, tone: 'success' },
    {
      title: '24h 平均延迟',
      value: ov.avg_latency_ms_24h !== null ? `${ov.avg_latency_ms_24h} ms` : '暂无',
      tone: 'primary',
    },
    { title: '已同步站点', value: ov.quantified_balance_site_count, tone: 'neutral' },
  ]
})

function strategyLabel(strategy: GatewayStrategyStat['route_strategy']) {
  return gatewayRouteStrategyOptions.find((item) => item.value === strategy)?.label ?? strategy
}

const selectedRouteStrategyDescription = computed(() =>
  gatewayRouteStrategyOptions.find((item) => item.value === settingsForm.route_strategy)?.description ?? '',
)

const selectedOverflowStrategyDescription = computed(() =>
  gatewayOverflowStrategyOptions.find((item) => item.value === settingsForm.concurrency_overflow_strategy)?.description ?? '',
)

const gatewayStrategyDescriptionItems = computed(() => [
  { label: '路由策略', value: selectedRouteStrategyDescription.value },
  { label: '并发溢出', value: selectedOverflowStrategyDescription.value },
  { label: '自动模型类型', value: '请求体里的 model 包含 claude / gpt / gemini 时，网关会自动选择对应类型路由；仍可用 type 参数手动指定。' },
])

const strategyBreakdownCards = computed(() => {
  if (!overview.value) {
    return []
  }
  return overview.value.strategy_breakdown_24h.map((item) => ({
    key: item.route_strategy,
    title: strategyLabel(item.route_strategy),
    requestCount: item.request_count,
    successRate: `${item.success_rate}%`,
    avgLatency: item.avg_latency_ms !== null ? `${item.avg_latency_ms} ms` : '暂无',
    streamRequestCount: item.stream_request_count,
    streamSuccessRate: item.stream_request_count > 0 ? `${item.stream_success_rate}%` : '暂无',
    avgStreamTtfb: item.avg_stream_ttfb_ms !== null ? `${item.avg_stream_ttfb_ms} ms` : '暂无',
  }))
})

function bucketTimeLabel(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) {
    return '--:--'
  }
  const hh = String(date.getHours()).padStart(2, '0')
  const mm = String(date.getMinutes()).padStart(2, '0')
  const ss = String(date.getSeconds()).padStart(2, '0')
  return `${hh}:${mm}:${ss}`
}

interface TrendBar {
  key: string
  label: string
  total: number
  success: number
  failed: number
  stream: number
  successHeight: number
  failedHeight: number
  hasData: boolean
  avgLatency: number | null
  showLabel: boolean
}

const trendBarDisplayCount = 20

const trendBars = computed<TrendBar[]>(() => {
  if (!overview.value) {
    return []
  }
  const sourceBuckets: GatewayTrendBucket[] = overview.value.recent_trend_5m
  const groupSize = Math.max(1, Math.ceil(sourceBuckets.length / trendBarDisplayCount))
  const buckets: GatewayTrendBucket[] = []
  for (let index = 0; index < sourceBuckets.length; index += groupSize) {
    const chunk = sourceBuckets.slice(index, index + groupSize)
    if (!chunk.length) {
      continue
    }
    const requestCount = chunk.reduce((acc, item) => acc + item.request_count, 0)
    const successCount = chunk.reduce((acc, item) => acc + item.success_count, 0)
    const failureCount = chunk.reduce((acc, item) => acc + item.failure_count, 0)
    const streamRequestCount = chunk.reduce((acc, item) => acc + item.stream_request_count, 0)
    const latencySamples = chunk
      .filter((item) => item.avg_latency_ms !== null && item.request_count > 0)
      .map((item) => Number(item.avg_latency_ms))
    buckets.push({
      bucket_start: chunk[0].bucket_start,
      request_count: requestCount,
      success_count: successCount,
      failure_count: failureCount,
      stream_request_count: streamRequestCount,
      avg_latency_ms: latencySamples.length
        ? Math.round(latencySamples.reduce((acc, item) => acc + item, 0) / latencySamples.length)
        : null,
    })
  }
  if (!buckets.length) {
    return []
  }
  const peak = buckets.reduce((acc, item) => Math.max(acc, item.request_count), 0)
  return buckets.map((item, index) => {
    const safePeak = peak > 0 ? peak : 1
    const successRatio = item.request_count > 0 ? item.success_count / safePeak : 0
    const failedRatio = item.request_count > 0 ? item.failure_count / safePeak : 0
    const label = bucketTimeLabel(item.bucket_start)
    return {
      key: item.bucket_start,
      label,
      total: item.request_count,
      success: item.success_count,
      failed: item.failure_count,
      stream: item.stream_request_count,
      successHeight: Math.round(successRatio * 100),
      failedHeight: Math.round(failedRatio * 100),
      hasData: item.request_count > 0,
      avgLatency: item.avg_latency_ms,
      showLabel: index % 10 === 0 || index === buckets.length - 1,
    }
  })
})

const trendSummary = computed(() => {
  if (!overview.value) {
    return null
  }
  const buckets = overview.value.recent_trend_5m
  if (!buckets.length) {
    return null
  }
  const total = buckets.reduce((acc, item) => acc + item.request_count, 0)
  const success = buckets.reduce((acc, item) => acc + item.success_count, 0)
  const failed = buckets.reduce((acc, item) => acc + item.failure_count, 0)
  const stream = buckets.reduce((acc, item) => acc + item.stream_request_count, 0)
  const samples: number[] = []
  buckets.forEach((item) => {
    if (item.avg_latency_ms !== null && item.request_count > 0) {
      samples.push(item.avg_latency_ms)
    }
  })
  const avgLatency = samples.length
    ? Math.round(samples.reduce((acc, value) => acc + value, 0) / samples.length)
    : null
  const successRate = total > 0 ? Math.round((success / total) * 1000) / 10 : 0
  return { total, success, failed, stream, successRate, avgLatency }
})

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
    { label: '站点', value: `#${route.site_id}` },
    { label: 'Key', value: routeKeyLabel(route) },
  ]
}

function routeTypeLabel(routeType: GatewayRoute['route_type']) {
  return routeTypeOptions.find((item) => item.value === routeType)?.label ?? routeType
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

function hasPackage(route: GatewayRoute) {
  return Boolean(String(route.package_display ?? '').trim())
}

function hasBalance(route: GatewayRoute) {
  return route.last_balance !== null && route.last_balance !== undefined && !Number.isNaN(route.last_balance)
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

function asLog(record: unknown) {
  return record as GatewayLog
}

function routeRowKey(record: GatewayRoute) {
  return record.id
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
  const selectedCircuitStateSet = new Set(selectedCircuitStates.value)
  const selectedPackageStateSet = new Set(selectedPackageStates.value)
  const selectedBalanceStateSet = new Set(selectedBalanceStates.value)
  const selectedIssueStateSet = new Set(selectedIssueStates.value)
  return routes.value.filter((route) =>
    (!selectedGroupSet.size || parseGroupNames(route.group_name).some((groupName) => selectedGroupSet.has(groupName))) &&
    (!selectedRouteTypeSet.size || selectedRouteTypeSet.has(route.route_type)) &&
    (!selectedCircuitStateSet.size || selectedCircuitStateSet.has(routeCircuitState(route))) &&
    (!selectedPackageStateSet.size ||
      (selectedPackageStateSet.has('with_package') && hasPackage(route)) ||
      (selectedPackageStateSet.has('without_package') && !hasPackage(route))) &&
    (!selectedBalanceStateSet.size ||
      (selectedBalanceStateSet.has('with_balance') && hasBalance(route)) ||
      (selectedBalanceStateSet.has('without_balance') && !hasBalance(route)) ||
      (selectedBalanceStateSet.has('zero_or_negative') && hasBalance(route) && Number(route.last_balance) <= 0)) &&
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
  selectedCircuitStates.value.length +
  selectedPackageStates.value.length +
  selectedBalanceStates.value.length +
  selectedIssueStates.value.length +
  (routeSearch.value.trim() ? 1 : 0),
)

function clearRouteFilters() {
  routeSearch.value = ''
  selectedGroups.value = []
  selectedRouteTypes.value = []
  selectedCircuitStates.value = []
  selectedPackageStates.value = []
  selectedBalanceStates.value = []
  selectedIssueStates.value = []
}

function applySiteSummary(summary: SiteSummary) {
  routes.value = routes.value.map((route) =>
    route.site_id === summary.site_id
      ? {
          ...route,
          last_balance: summary.last_balance,
          balance_display: summary.balance_display,
          package_display: summary.package_display,
          checkin_status: summary.checkin_status,
        }
      : route,
  )
}

function applyProbeResult(result: GatewayRouteProbeResult) {
  routes.value = routes.value.map((route) =>
    route.id === result.id
      ? {
          ...route,
          last_status_code: result.last_status_code,
          last_error: result.last_error,
          last_latency_ms: result.last_latency_ms,
          last_success_at: result.last_success_at,
          last_failure_at: result.last_failure_at,
        }
      : route,
  )
}

function applyRouteBalanceResult(result: { site_id: number; route_id: number; last_balance: number | null; remaining: number | null; unit?: string }) {
  routes.value = routes.value.map((route) =>
    route.site_id === result.site_id
      ? {
          ...route,
          last_balance: result.last_balance ?? result.remaining,
          balance_display: formatBalance(result.last_balance ?? result.remaining, result.unit),
        }
      : route,
  )
}

function isRouteProbing(routeId: number) {
  return probingRouteIds.value.includes(routeId)
}

function isRouteBalanceProbing(routeId: number) {
  return balanceProbingRouteIds.value.includes(routeId)
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
  await refreshRouteSummaries()
}

async function loadData() {
  loading.value = true
  try {
    const [overviewData, settingsData, routeData, logData, groupData] = await Promise.all([
      getGatewayOverview(),
      getGatewaySettings(),
      getGatewayRoutes({ includeDisabled: includeDisabled.value }),
      getGatewayLogs(80),
      getSiteGroups(),
    ])
    overview.value = overviewData
    Object.assign(settingsForm, settingsData)
    routes.value = routeData
    logs.value = logData
    siteGroups.value = groupData
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
    const [overviewData, routeData, logData] = await Promise.all([
      getGatewayOverview(),
      getGatewayRoutes({ includeDisabled: includeDisabled.value }),
      getGatewayLogs(80),
    ])
    overview.value = overviewData
    routes.value = routeData
    logs.value = logData
  } catch {
    // 自动刷新静默失败，避免请求波动时持续打扰。
  } finally {
    autoRefreshing.value = false
  }
}

function startAutoRefresh() {
  stopAutoRefresh()
  autoRefreshTimer = window.setInterval(refreshRealtimeData, gatewayAutoRefreshMs)
}

function stopAutoRefresh() {
  if (autoRefreshTimer !== null) {
    window.clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

function handleVisibilityChange() {
  if (document.visibilityState === 'visible') {
    void refreshRealtimeData()
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
    toast.success(`已同步 ${result.route_count} 条网关路由。`)
    await loadData()
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
  routes.value = routes.value.map((item) => (item.id === route.id ? { ...item, route_type: routeType } : item))
  try {
    const updated = await updateGatewayRouteType(route.id, routeType)
    routes.value = routes.value.map((item) => (item.id === route.id ? updated : item))
    toast.success(`${loadRouteLabel(route)} 已切换为 ${routeTypeLabel(routeType)}。`)
  } catch (err) {
    routes.value = routes.value.map((item) => (item.id === route.id ? { ...item, route_type: previousType } : item))
    toast.error(err instanceof Error ? err.message : '类型切换失败')
  }
}

async function handleRouteTypeSelect(route: GatewayRoute, value: unknown) {
  if (value !== 'claude' && value !== 'codex' && value !== 'gemini') {
    return
  }
  await handleRouteTypeChange(route, value)
}

async function handleProbeAll() {
  const routeIds = routes.value.map((route) => route.id)
  if (!routeIds.length) {
    toast.error('当前没有可探测的网关路由。')
    return
  }

  probeLoading.value = true
  try {
    const results = await probeGatewayRoutes(routeIds)
    results.forEach(applyProbeResult)
    const successCount = results.filter((item) => item.ok).length
    const failed = results.filter((item) => !item.ok)
    if (!failed.length) {
      toast.success(`路由探测完成，${successCount} 条全部可用。`)
      return
    }
    const sample = failed
      .slice(0, 2)
      .map((item) => `${item.site_name}${item.key_name ? ` · ${item.key_name}` : ''}`)
      .join('，')
    toast.error(`路由探测完成，成功 ${successCount} 条，失败 ${failed.length} 条：${sample}`)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '路由探测失败')
  } finally {
    probeLoading.value = false
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
    if (result.ok) {
      toast.success(`${loadRouteLabel(route)} 余额读取成功：${formatBalance(result.remaining, result.unit)}（${result.base_url}）`)
    } else {
      toast.error(`${loadRouteLabel(route)} 余额读取失败：${result.message}`)
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '余额读取失败')
  } finally {
    balanceProbingRouteIds.value = balanceProbingRouteIds.value.filter((item) => item !== route.id)
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
    <div class="page-stack page-stack--dashboard">
      <div class="page-toolbar page-toolbar--actions">
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
        <a-space>
          <a-button :loading="loading" @click="handleRefresh">
            <template #icon>
              <ReloadOutlined />
            </template>
            刷新
          </a-button>
          <a-button :loading="loading" @click="handleSync">
            <template #icon>
              <SyncOutlined />
            </template>
            同步路由
          </a-button>
          <a-button :loading="probeLoading" @click="handleProbeAll">探测全部</a-button>
          <a-button type="primary" @click="addUpstreamOpen = true">
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
          <a-button @click="logsDrawerOpen = true">最近请求</a-button>
        </a-space>
      </div>

      <div class="gateway-metrics">
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
        <div
          v-if="strategyBreakdownCards.length || trendBars.length"
          class="gateway-summary-row"
        >
        <section
          v-if="strategyBreakdownCards.length"
          class="gateway-strategy-section gateway-summary-row__strategy"
        >
          <div class="gateway-strategy-section__body">
            <div class="gateway-strategy-grid">
              <a-card
                v-for="item in strategyBreakdownCards"
                :key="item.key"
                :bordered="false"
                class="admin-card gateway-strategy-card"
              >
                <div class="gateway-strategy-card__title">{{ item.title }}</div>
                <div class="gateway-strategy-card__metrics">
                  <div>
                    <div class="gateway-strategy-card__label">24h 请求数</div>
                    <div class="gateway-strategy-card__value">{{ item.requestCount }}</div>
                  </div>
                  <div>
                    <div class="gateway-strategy-card__label">成功率</div>
                    <div class="gateway-strategy-card__value">{{ item.successRate }}</div>
                  </div>
                  <div>
                    <div class="gateway-strategy-card__label">平均延迟</div>
                    <div class="gateway-strategy-card__value">{{ item.avgLatency }}</div>
                  </div>
                  <div>
                    <div class="gateway-strategy-card__label">流式请求</div>
                    <div class="gateway-strategy-card__value">{{ item.streamRequestCount }}</div>
                  </div>
                  <div>
                    <div class="gateway-strategy-card__label">流式成功率</div>
                    <div class="gateway-strategy-card__value">{{ item.streamSuccessRate }}</div>
                  </div>
                  <div>
                    <div class="gateway-strategy-card__label">流式平均 TTFB</div>
                    <div class="gateway-strategy-card__value">{{ item.avgStreamTtfb }}</div>
                  </div>
                </div>
              </a-card>
            </div>
          </div>
        </section>

        <a-card
          v-if="trendBars.length"
          :bordered="false"
          class="admin-card gateway-trend-card gateway-summary-row__trend"
        >
          <div class="gateway-trend-header">
            <div class="gateway-trend-header__title">近 1 分钟趋势</div>
            <div v-if="trendSummary" class="gateway-trend-header__metrics">
              <span>请求 <strong>{{ trendSummary.total }}</strong></span>
              <span>成功 <strong>{{ trendSummary.success }}</strong></span>
              <span>失败 <strong>{{ trendSummary.failed }}</strong></span>
              <span>流式 <strong>{{ trendSummary.stream }}</strong></span>
              <span>成功率 <strong>{{ trendSummary.successRate }}%</strong></span>
              <span>平均延迟 <strong>{{ trendSummary.avgLatency !== null ? `${trendSummary.avgLatency} ms` : '暂无' }}</strong></span>
            </div>
          </div>
          <div class="gateway-trend-bars">
            <div
              v-for="bar in trendBars"
              :key="bar.key"
              class="gateway-trend-bar"
              :class="{ 'gateway-trend-bar--empty': !bar.hasData }"
              :title="`${bar.label}\n请求 ${bar.total}\n成功 ${bar.success}\n失败 ${bar.failed}\n流式 ${bar.stream}\n平均延迟 ${bar.avgLatency !== null ? bar.avgLatency + ' ms' : '暂无'}`"
            >
              <div class="gateway-trend-bar__stack">
                <span
                  class="gateway-trend-bar__segment gateway-trend-bar__segment--failed"
                  :style="{ height: `${bar.failedHeight}%` }"
                />
                <span
                  class="gateway-trend-bar__segment gateway-trend-bar__segment--success"
                  :style="{ height: `${bar.successHeight}%` }"
                />
              </div>
              <div class="gateway-trend-bar__count" :class="{ 'gateway-trend-bar__count--zero': !bar.hasData }">
                {{ bar.hasData ? bar.total : '·' }}
              </div>
              <div class="gateway-trend-bar__time">{{ bar.showLabel ? bar.label : '' }}</div>
            </div>
          </div>
          <div class="gateway-trend-legend">
            <span><i class="dot dot--success" />成功</span>
            <span><i class="dot dot--failed" />失败</span>
            <span><i class="dot dot--empty" />无请求</span>
            <span class="gateway-trend-legend__hint">统计窗口 60 秒；按 3 秒合并为 20 根柱，无请求显示为灰色柱</span>
          </div>
        </a-card>
      </div>

      <a-row :gutter="[16, 16]" class="page-grid-fill">
        <a-col :xs="24">
          <a-card :bordered="false" class="admin-card admin-card--fill">
            <template #title>
              <div class="route-pool-title">
                <span>路由池</span>
                <a-space size="small" wrap>
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
                </a-space>
              </div>
            </template>
            <template #extra>
              <a-space wrap>
                <a-input
                  v-model:value="routeSearch"
                  placeholder="搜索路由 / 域名 / 分组"
                  allow-clear
                  style="width: 220px"
                />
                <a-select
                  v-model:value="selectedGroups"
                  mode="multiple"
                  allow-clear
                  :options="groupOptions"
                  placeholder="按分组筛选"
                  style="width: 240px"
                />
                <a-select
                  v-model:value="selectedCircuitStates"
                  mode="multiple"
                  allow-clear
                  :options="circuitStateOptions"
                  placeholder="按熔断状态"
                  style="width: 180px"
                />
                <a-select
                  v-model:value="selectedPackageStates"
                  mode="multiple"
                  allow-clear
                  :options="packageStateOptions"
                  placeholder="按套餐"
                  style="width: 140px"
                />
                <a-select
                  v-model:value="selectedBalanceStates"
                  mode="multiple"
                  allow-clear
                  :options="balanceStateOptions"
                  placeholder="按余额"
                  style="width: 170px"
                />
                <a-select
                  v-model:value="selectedIssueStates"
                  mode="multiple"
                  allow-clear
                  :options="issueStateOptions"
                  placeholder="按异常"
                  style="width: 140px"
                />
                <a-switch
                  v-model:checked="includeDisabled"
                  checked-children="含停用"
                  un-checked-children="仅启用"
                  @change="loadData"
                />
                <a-button size="small" :disabled="!activeRouteFilterCount" @click="clearRouteFilters">
                  清空筛选
                </a-button>
              </a-space>
            </template>

            <div class="card-shell">
              <div :ref="bindPageTableContainer" class="table-fill table-fill--management">
                <a-table
                  :columns="routeColumns"
                  :data-source="filteredRoutes"
                  :pagination="{ pageSize: gatewayTablePageSize }"
                  :row-key="routeRowKey"
                  size="middle"
                  :scroll="{ x: 2040, y: pageTableY }"
                >
                  <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'route'">
                      <div class="table-cell-compact">
                        <div class="table-cell-compact__head">
                          <strong class="table-cell-compact__title">{{ loadRouteLabel(asRoute(record)) }}</strong>
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
                    <template v-else-if="column.key === 'package'">
                      {{ asRoute(record).package_display || '暂无' }}
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
                      <span :class="['gateway-concurrency', { 'gateway-concurrency--active': asRoute(record).active_concurrency > 0 }]">
                        {{ asRoute(record).active_concurrency }}
                      </span>
                    </template>
                    <template v-else-if="column.key === 'circuit'">
                      <a-tooltip v-if="asRoute(record).circuit_open_until" :title="`冷却到 ${formatTime(asRoute(record).circuit_open_until)}`">
                        <div class="participation-cell">
                          <StatusPill :value="asRoute(record).is_enabled ? asRoute(record).circuit_state : 'paused'" />
                        </div>
                      </a-tooltip>
                      <div v-else class="participation-cell">
                        <StatusPill :value="asRoute(record).is_enabled ? asRoute(record).circuit_state : 'paused'" />
                      </div>
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
                        <a-button size="small" :danger="asRoute(record).is_enabled" @click="handleToggle(asRoute(record))">
                          {{ asRoute(record).is_enabled ? '禁用' : '启用' }}
                        </a-button>
                        <a-button
                          size="small"
                          type="primary"
                          ghost
                          :disabled="asRoute(record).circuit_state === 'closed'"
                          @click="handleResetCircuit(asRoute(record))"
                        >
                          重置熔断
                        </a-button>
                        <a-button
                          size="small"
                          :loading="isRouteProbing(asRoute(record).id)"
                          @click="handleProbeRoute(asRoute(record))"
                        >
                          探测
                        </a-button>
                        <a-button
                          size="small"
                          :loading="isRouteBalanceProbing(asRoute(record).id)"
                          @click="handleProbeRouteBalance(asRoute(record))"
                        >
                          余额
                        </a-button>
                        <a-button size="small" @click="openRouteLogs(asRoute(record))">
                          <template #icon>
                            <HistoryOutlined />
                          </template>
                          历史
                        </a-button>
                      </a-space>
                    </template>
                  </template>
                </a-table>
              </div>
            </div>
          </a-card>
        </a-col>
      </a-row>
      </div>

      <a-modal
        v-model:open="settingsOpen"
        title="网关策略"
        width="720px"
        :confirm-loading="settingsLoading"
        @ok="saveSettings"
      >
        <a-form layout="vertical">
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

          <a-row :gutter="16">
            <a-col :xs="24" :md="12">
              <a-form-item label="单路由并发上限">
                <a-input-number
                  v-model:value="settingsForm.route_concurrency_limit"
                  style="width: 100%"
                  :min="0"
                  :max="1000"
                />
                <small class="field-help">例如填 5，某条路由达到 5 个当前并发后，新请求会优先转到其他路由；填 0 表示不限制。</small>
              </a-form-item>
            </a-col>
            <a-col :xs="24" :md="12">
              <a-form-item label="并发溢出优先级">
                <a-select
                  v-model:value="settingsForm.concurrency_overflow_strategy"
                  :options="gatewayOverflowStrategyOptions.map(({ label, value }) => ({ label, value }))"
                />
                <small class="field-help">{{ selectedOverflowStrategyDescription }}</small>
              </a-form-item>
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
            <small class="field-help">保存后客户端需使用 Authorization: Bearer 这个 Key；留空时会回退到后端环境变量。</small>
          </a-form-item>

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
            :scroll="{ x: 1100, y: drawerTableY }"
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
            :scroll="{ x: 1100, y: drawerTableY }"
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

.table-info-icon {
  flex: 0 0 auto;
  color: #94a3b8;
  font-size: 13px;
  cursor: help;
}

.table-info-icon:hover {
  color: #5b6f8f;
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

.gateway-summary-row__strategy {
  flex: 0 1 360px;
  min-width: 260px;
  max-width: 360px;
  display: flex;
  flex-direction: column;
  min-height: 0;
  box-sizing: border-box;
}

.gateway-summary-row__trend {
  flex: 1 1 0%;
  min-width: 0;
  height: 160px;
  margin-bottom: 0 !important;
  display: flex;
  flex-direction: column;
  min-height: 0;
  box-sizing: border-box;
}

.gateway-summary-row__trend :deep(.ant-card-body) {
  height: 100%;
  padding: 14px 14px 12px;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.gateway-strategy-section {
  margin-bottom: 0;
}

.gateway-strategy-section__body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding-right: 4px;
  display: flex;
}

.gateway-strategy-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 16px;
  min-height: 100%;
  width: 100%;
  align-content: stretch;
}

.gateway-strategy-card {
  height: 100%;
}

.gateway-strategy-card__title {
  font-size: 13px;
  font-weight: 600;
  color: #24334d;
  margin-bottom: 8px;
}

.gateway-strategy-card__metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px 10px;
}

.gateway-strategy-card :deep(.ant-card-body) {
  padding: 12px 14px;
}

.gateway-strategy-card__label {
  font-size: 11px;
  color: #6b7280;
  margin-bottom: 5px;
  line-height: 1.25;
}

.gateway-strategy-card__value {
  font-size: 12px;
  font-weight: 600;
  color: #24334d;
  line-height: 1.3;
}

.gateway-trend-card {
  margin-bottom: 0;
}

.gateway-trend-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px 16px;
  margin-bottom: 6px;
  flex: 0 0 auto;
}

.gateway-trend-header__title {
  font-size: 13px;
  font-weight: 600;
  color: #24334d;
}

.gateway-trend-header__metrics {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 14px;
  font-size: 11px;
  color: #6b7280;
}

.gateway-trend-header__metrics strong {
  color: #24334d;
  margin-left: 4px;
  font-weight: 600;
}

.gateway-trend-bars {
  display: grid;
  grid-template-columns: repeat(20, minmax(0, 1fr));
  gap: 3px;
  align-items: stretch;
  flex: 1 1 auto;
  min-height: 0;
  width: 100%;
}

.gateway-trend-bar {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  min-width: 0;
  min-height: 0;
}

.gateway-trend-bar__stack {
  position: relative;
  width: 100%;
  flex: 1 1 auto;
  min-height: 24px;
  border-radius: 4px;
  background: #f1f4f9;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
}

.gateway-trend-bar__segment {
  display: block;
  width: 100%;
  transition: height 0.2s ease;
}

.gateway-trend-bar__segment--success {
  background: #2f8a4a;
}

.gateway-trend-bar__segment--failed {
  background: #d65a4a;
}

.gateway-trend-bar__count {
  font-size: 11px;
  font-weight: 600;
  color: #24334d;
}

.gateway-trend-bar__count--zero {
  color: #94a3b8;
  font-weight: 400;
}

.gateway-trend-bar__time {
  font-size: 10px;
  color: #94a3b8;
  font-family: 'IBM Plex Mono', monospace;
  min-height: 14px;
  white-space: nowrap;
}

.gateway-trend-bar--empty .gateway-trend-bar__stack {
  background: #e5e9f1;
}

.gateway-trend-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  align-items: center;
  margin-top: 4px;
  font-size: 11px;
  color: #6b7280;
  flex: 0 0 auto;
}

.gateway-trend-legend .dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 2px;
  margin-right: 4px;
  vertical-align: middle;
}

.gateway-trend-legend .dot--success {
  background: #2f8a4a;
}

.gateway-trend-legend .dot--failed {
  background: #d65a4a;
}

.gateway-trend-legend .dot--empty {
  background: #e5e9f1;
  border: 1px solid #cbd5e1;
}

.gateway-trend-legend__hint {
  margin-left: auto;
  color: #94a3b8;
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
  grid-template-columns: repeat(2, minmax(0, 1fr));
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
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-top: 8px;
}

.gateway-access {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: nowrap;
  min-width: 0;
  max-width: min(74vw, 980px);
}

.gateway-access code {
  display: block;
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  padding: 6px 10px;
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-control);
  background: var(--bg-panel);
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
  padding: 0 8px;
  border: 1px solid #b8d8ff;
  border-radius: var(--radius-control);
  background: #f2f7ff;
  color: #1e5ea8;
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

.route-pool-title {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.route-pool-title > span {
  font-weight: 700;
}

.route-type-select :deep(.ant-select-selector) {
  border-color: transparent !important;
  color: #fff !important;
  font-weight: 700;
}

.route-issue-tag {
  margin-left: 6px;
  font-size: 11px;
  line-height: 18px;
}

.route-type-select :deep(.ant-select-selection-item),
.route-type-select :deep(.ant-select-arrow) {
  color: #fff !important;
}

.route-type-select--claude :deep(.ant-select-selector) {
  background: #f97316 !important;
}

.route-type-select--codex :deep(.ant-select-selector) {
  background: #111827 !important;
}

.route-type-select--gemini :deep(.ant-select-selector) {
  background: #0891b2 !important;
}

.route-type-option {
  display: inline-flex;
  align-items: center;
  min-width: 74px;
  padding: 2px 9px;
  border-radius: 999px;
  color: #fff;
  font-size: 12px;
  font-weight: 800;
}

.route-type-option--claude {
  background: #f97316;
}

.route-type-option--codex {
  background: #111827;
}

.route-type-option--gemini {
  background: #0891b2;
}

.gateway-concurrency {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 34px;
  height: 26px;
  padding: 0 10px;
  border: 1px solid rgba(148, 163, 184, 0.32);
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.1);
  color: #475569;
  font-family: 'IBM Plex Mono', monospace;
  font-size: 12px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.gateway-concurrency--active {
  border-color: rgba(14, 165, 233, 0.5);
  background: rgba(14, 165, 233, 0.14);
  color: #0369a1;
}

.gateway-latency {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px;
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.12);
  color: #cbd5e1;
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

@media (max-width: 720px) {
  .gateway-policy-help__grid,
  .gateway-policy-help__notes {
    grid-template-columns: 1fr;
  }
}
</style>
