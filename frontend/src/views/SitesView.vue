<script setup lang="ts">
import {
  DeleteOutlined,
  PlusOutlined,
  ReloadOutlined,
  ExportOutlined,
  ExperimentOutlined,
  KeyOutlined,
  ShareAltOutlined,
  DollarCircleOutlined,
  MoreOutlined,
  QuestionCircleOutlined,
} from '@ant-design/icons-vue'
import { Modal } from 'ant-design-vue'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch, type ComponentPublicInstance } from 'vue'
import siteEditorAccountArtwork from '../assets/site-editor-account.png'
import siteEditorCloudArtwork from '../assets/site-editor-cloud.png'
import siteEditorGatewayArtwork from '../assets/site-editor-gateway.png'
import {
  analyzeLocalStorage,
  convertCCSwitchSql,
  createRegistrationBatchSites,
  createSite,
  deleteSite,
  exportCCSwitchConfig,
  getCheckinSites,
  getDuplicateSites,
  getPlugins,
  getRuns,
  getSettings,
  getSite,
  getSiteGroups,
  getSites,
  importCCSwitchConfig,
  importCCSwitchSql,
  mergeDuplicateSites,
  probeSiteBalance,
  previewSiteTotp,
  refreshOneSiteApiKeys,
  refreshSiteApiKeys,
  refreshSiteInvites,
  refreshSiteSummaries,
  runSchedulerNow,
  runSiteCheckin,
  syncGatewayRoutes,
  testSite,
  toggleSite,
  updateCheckinParticipation,
  updateSettings,
  updateSite,
} from '../api'
import ShellLayout from '../components/ShellLayout.vue'
import StatusPill from '../components/StatusPill.vue'
import { useDebouncedTask } from '../composables/useDebouncedTask'
import { useTableScrollHeights } from '../composables/useTableScrollHeights'
import { balanceTone, formatBalance, formatGroupNames, normalizeBalanceUnit, normalizeGroupNames, parseGroupNames } from '../format'
import {
  apiKeyImageEditPath,
  apiKeyImageGenerationPath,
  apiKeyRoutePath,
  apiKeyRequestBaseURLs,
  apiKeyEntryValue,
  apiKeyValue,
  equivalentApiKeyEntryExists,
  isManualApiKeyEntry,
  mergeApiKeyEntries,
  setApiKeyImagePaths,
  setApiKeyRoutePath,
  setApiKeyRequestBaseURLs,
  removeSiteApiKeyCredential,
  storedApiKeyEntries,
  type SiteApiKeyRecord,
} from '../siteApiKeyCredentials'
import { useToast } from '../toast'
import type {
  CheckinRun,
  CheckinSite,
  DuplicateSiteGroup,
  LocalStorageAnalyzeResult,
  PluginMeta,
  SettingsData,
  Site,
  SiteApiKeyRefreshResult,
  SiteGroup,
  SiteInviteRefreshResult,
  SitePayload,
  SiteRegistrationBatchResult,
  SiteSummary,
  TotpPreview,
} from '../types'

const toast = useToast()
const plugins = ref<PluginMeta[]>([])
const sites = ref<Site[]>([])
const siteGroups = ref<SiteGroup[]>([])
const selectedId = ref<number | null>(null)
const busy = ref(false)
const totpPreviewOpen = ref(false)
const totpPreviewLoading = ref(false)
const totpPreview = ref<TotpPreview | null>(null)
const testFeedback = ref<{ type: 'success' | 'error'; title: string; message: string } | null>(null)
const saveFeedback = ref<string | null>(null)
const lastSavedEditorSnapshot = ref('')
const drawerOpen = ref(false)
const editingId = ref<number | null>(null)
const apiKeyDialogOpen = ref(false)
const apiKeyDialogSaving = ref(false)
const apiKeyDialogSiteId = ref<number | null>(null)
const inviteDialogOpen = ref(false)
const inviteDialogLoading = ref(false)
const inviteRefreshAllLoading = ref(false)
const inviteLoadingSiteIds = ref<number[]>([])
const apiKeyRefreshAllLoading = ref(false)
const apiKeyRefreshingSiteIds = ref<number[]>([])
const inviteDialogSiteId = ref<number | null>(null)
const inviteDialogSiteName = ref('')
const inviteDialogLink = ref('')
const inviteDialogCode = ref('')
const ccSwitchConfigOpen = ref(false)
const ccSwitchConfigTab = ref<'import' | 'export'>('import')
const ccSwitchImportLoading = ref(false)
const ccSwitchExportLoading = ref(false)
const ccSwitchSqlPreviewLoading = ref(false)
const ccSwitchImportMode = ref<'json' | 'sql'>('json')
const ccSwitchImportText = ref('')
const ccSwitchExportText = ref('')
const ccSwitchFileInput = ref<HTMLInputElement | null>(null)
const ccSwitchSelectedSections = ref<string[]>([])
const siteSearch = ref('')
const ccSwitchPreviewSearch = ref('')
const ccSwitchResolvedPayload = ref<Record<string, unknown> | null>(null)
const ccSwitchResolveError = ref('')
const duplicateCheckOpen = ref(false)
const duplicateCheckLoading = ref(false)
const duplicateMergeLoading = ref(false)
const duplicateChecked = ref(false)
const balanceProbeIds = ref<number[]>([])
const duplicateGroups = ref<DuplicateSiteGroup[]>([])
const duplicateSearch = ref('')
const localStorageAnalyzeLoading = ref(false)
const localStorageRawText = ref('')
const storageAnalyzeTimer = ref<ReturnType<typeof window.setTimeout> | null>(null)
const lastAutoAnalyzedStorageRaw = ref('')
const batchRegisterEnabled = ref(false)
const batchRegisterResult = ref<SiteRegistrationBatchResult | null>(null)
const storageManagedTimers = new Set<ReturnType<typeof window.setTimeout>>()
const storageDelayResolvers = new Set<() => void>()
let mounted = false

const { pageTableY, pageTableContainer, modalTableY, drawerTableY } = useTableScrollHeights()
const tablePageSize = 20
const editorGroupNames = ref<string[]>([])

const checkinMeta = ref(new Map<number, CheckinSite>())
const checkinRuns = ref<CheckinRun[]>([])
const selectedCheckinIds = ref<number[]>([])
const checkinConfigOpen = ref(false)
const checkinLogsOpen = ref(false)
const checkinSettingsBusy = ref(false)
const checkinRunSearch = ref('')
const connectivitySweepProgress = ref<{ total: number; done: number; success: number; failed: number } | null>(null)
const inviteRefreshProgress = ref<{ total: number; done: number; success: number; failed: number } | null>(null)
const apiKeyRefreshProgress = ref<{ total: number; done: number; success: number; failed: number } | null>(null)
const checkinBatchProgress = ref<{ total: number; done: number; success: number; failed: number } | null>(null)
const checkinConfigForm = reactive<SettingsData>({
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

const apiKeyDialogForm = reactive({
  site_name: '',
  request_api_urls: '',
  endpoint_hint: '',
  image_generation_path: '',
  image_edit_path: '',
})

const apiKeyRequestUrlDrafts = reactive<Record<string, string>>({})
const apiKeyRoutePathDrafts = reactive<Record<string, string>>({})
const apiKeyImageGenerationPathDrafts = reactive<Record<string, string>>({})
const apiKeyImageEditPathDrafts = reactive<Record<string, string>>({})

const manualApiKeyForm = reactive({
  name: '',
  key: '',
  route_type: 'codex',
  route_path: 'responses',
  request_base_urls: '',
  image_generation_path: '',
  image_edit_path: '',
})

const batchRegisterForm = reactive({
  email_pattern: '',
  password: '',
  count: 3,
  start_index: 1,
})

const emailPatternExamples = [
  '{n}@example.com',
  'user+{n}@example.com',
  'user+{n:03}@example.com',
  'user+{rand:[0-9]{6}}@example.com',
  'user+{rand:[a-z]{8}}@example.com',
  'user+{rand:[A-Z]{8}}@example.com',
  'user+{rand:[A-Za-z0-9]{10}}@example.com',
]

type SiteApiKeyEntry = {
  id: string
  entryIndex: number
  name: string
  key: string
  status: string
  isPrimary: boolean
  source: string
  routeType: string
  routePath: string
  requestBaseURLs: string[]
  imageGenerationPath: string
  imageEditPath: string
  isManual: boolean
}

function bindPageTableContainer(element: Element | ComponentPublicInstance | null) {
  pageTableContainer.value = element instanceof HTMLElement ? element : null
}

function scheduleManagedTimeout(callback: () => void, delay = 0) {
  const timer = window.setTimeout(() => {
    storageManagedTimers.delete(timer)
    if (mounted) {
      callback()
    }
  }, delay)
  storageManagedTimers.add(timer)
  return timer
}

function clearManagedTimeout(timer: ReturnType<typeof window.setTimeout> | null) {
  if (!timer) {
    return
  }
  window.clearTimeout(timer)
  storageManagedTimers.delete(timer)
}

function waitStorageDelay(ms: number) {
  return new Promise<void>((resolve) => {
    let timer: ReturnType<typeof window.setTimeout>
    const done = () => {
      storageManagedTimers.delete(timer)
      storageDelayResolvers.delete(done)
      resolve()
    }
    timer = window.setTimeout(done, ms)
    storageManagedTimers.add(timer)
    storageDelayResolvers.add(done)
  })
}

type CCSwitchPreviewRow = {
  key: string
  sectionKey: string
  app: string
  order: number
  isCurrent: boolean
  name: string
  website: string
  apiKeyStatus: string
  hasAuth: boolean
  note: string
}


function parseCCSwitchJsonPayload(text: string): Record<string, unknown> | null {
  const normalized = text.trim()
  if (!normalized) {
    return null
  }
  try {
    const payload = JSON.parse(normalized) as Record<string, unknown>
    return payload && typeof payload === 'object' ? payload : null
  } catch {
    return null
  }
}

function isStorageJsonCandidate(text: string): boolean {
  const normalized = text.trim()
  if (!normalized) {
    return false
  }
  try {
    const parsed = JSON.parse(normalized)
    if (parsed && typeof parsed === 'object') {
      return true
    }
    if (typeof parsed === 'string') {
      const inner = JSON.parse(parsed.trim())
      return Boolean(inner && typeof inner === 'object')
    }
  } catch {
    return false
  }
  return false
}

const editor = reactive<SitePayload>({
  name: '',
  base_url: '',
  plugin_key: '',
  group_name: '',
  supported_models: null,
  is_enabled: true,
  notes: '',
  credentials: {},
  plugin_config: {},
})

const selectedSite = computed(() =>
  selectedId.value !== null ? sites.value.find((item) => item.id === selectedId.value) ?? null : null,
)

const editingSite = computed(() =>
  editingId.value !== null ? sites.value.find((item) => item.id === editingId.value) ?? null : null,
)

const currentPlugin = computed(
  () => plugins.value.find((item) => item.key === editor.plugin_key) ?? null,
)

const pluginOptions = computed(() =>
  {
    const visibleOptions = plugins.value
      .filter((plugin) => plugin.key !== 'api-supplier')
      .map((plugin) => ({
        label: plugin.name,
        value: plugin.key,
      }))

    if (editor.plugin_key === 'api-supplier') {
      return [
        { label: '导入记录', value: 'api-supplier' },
        ...visibleOptions,
      ]
    }
    return visibleOptions
  },
)

function pluginForKey(pluginKey: string) {
  return plugins.value.find((plugin) => plugin.key === pluginKey) ?? null
}

const enabledSiteCount = computed(() => sites.value.filter((item) => item.is_enabled).length)
const groupedSiteCount = computed(() => sites.value.filter((item) => parseGroupNames(item.group_name).length).length)
const availableGroupNames = computed(() => {
  const labels = new Set<string>()
  siteGroups.value.forEach((group) => labels.add(group.name))
  parseGroupNames(editor.group_name).forEach((groupName) => labels.add(groupName))
  return [...labels].sort((a, b) => a.localeCompare(b, 'zh-CN'))
})
const groupOptions = computed(() =>
  availableGroupNames.value.map((groupName) => ({
    label: groupName,
    value: groupName,
  })),
)
const readyGatewayCount = computed(() =>
  sites.value.filter((site) => {
    const creds = site.credentials ?? {}
    const hasApiKey = Boolean(String(creds.api_key ?? '').trim())
    const hasApiKeys = Array.isArray(creds.api_keys) && creds.api_keys.length > 0
    return site.is_enabled && (hasApiKey || hasApiKeys)
  }).length,
)
const successSiteCount = computed(() => sites.value.filter((site) => site.connection_status === 'success').length)
const failedSiteCount = computed(() => sites.value.filter((site) => site.connection_status === 'failed').length)
const pendingSiteCount = computed(() =>
  sites.value.filter((site) => !site.connection_status || ['pending', 'active'].includes(String(site.connection_status))).length,
)
const totalBalancesByUnit = computed(() => {
  const totals = new Map<string, number>()
  for (const site of sites.value) {
    if (site.last_balance === null || site.last_balance === undefined || Number.isNaN(site.last_balance)) {
      continue
    }
    const display = String(site.balance_display ?? '').trim()
    let unit = '$'
    if (/^[\$¥€£]/.test(display)) {
      unit = display[0]
    } else {
      const match = display.match(/\s([^\s]+)$/)
      if (match) {
        unit = normalizeBalanceUnit(match[1])
      }
    }
    totals.set(unit, (totals.get(unit) ?? 0) + Number(site.last_balance))
  }
  return totals
})
const totalBalanceSummary = computed(() => {
  if (!totalBalancesByUnit.value.size) {
    return '暂无'
  }
  return [...totalBalancesByUnit.value.entries()]
    .map(([unit, value]) => formatBalance(value, unit))
    .join(' / ')
})
const totalBalanceTone = computed<'positive' | 'negative' | 'zero' | 'empty'>(() => {
  const totals = totalBalancesByUnit.value
  if (!totals.size) return 'empty'
  let hasNegative = false
  let hasPositive = false
  for (const value of totals.values()) {
    if (value < 0) hasNegative = true
    else if (value > 0) hasPositive = true
  }
  if (hasNegative) return 'negative'
  if (hasPositive) return 'positive'
  return 'zero'
})
const quantifiedBalanceSiteCount = computed(() =>
  sites.value.filter((site) => site.last_balance !== null && site.last_balance !== undefined && !Number.isNaN(site.last_balance)).length,
)

function ccSwitchSectionLabel(section: string) {
  if (section === 'codex' || section === 'openai' || section === 'opencode' || section === 'openclaw' || section === 'hermes') {
    return section === 'codex' ? 'Codex' : section.charAt(0).toUpperCase() + section.slice(1)
  }
  if (section === 'claude') return 'Claude'
  if (section === 'gemini') return 'Gemini'
  return section.charAt(0).toUpperCase() + section.slice(1)
}

function parseCCSwitchPreview(payload: Record<string, unknown>): CCSwitchPreviewRow[] {
  const rows: CCSwitchPreviewRow[] = []
  for (const [sectionKey, section] of Object.entries(payload)) {
    if (!section || typeof section !== 'object') {
      continue
    }
    const current = String((section as { current?: unknown }).current ?? '').trim()
    const providers = (section as { providers?: unknown }).providers
    if (!providers || typeof providers !== 'object') {
      continue
    }
    let order = 0
    for (const [providerId, rawProvider] of Object.entries(providers)) {
      if (!rawProvider || typeof rawProvider !== 'object') {
        continue
      }
      order += 1
      const provider = rawProvider as Record<string, unknown>
      const settingsConfig = typeof provider.settingsConfig === 'object' && provider.settingsConfig !== null
        ? provider.settingsConfig as Record<string, unknown>
        : {}
      const env = typeof settingsConfig.env === 'object' && settingsConfig.env !== null
        ? settingsConfig.env as Record<string, unknown>
        : {}
      const auth = typeof settingsConfig.auth === 'object' && settingsConfig.auth !== null
        ? settingsConfig.auth as Record<string, unknown>
        : {}
      const hasApiKey = Boolean(
        String(auth.OPENAI_API_KEY ?? env.OPENAI_API_KEY ?? env.ANTHROPIC_AUTH_TOKEN ?? env.GEMINI_API_KEY ?? '').trim(),
      )
      rows.push({
        key: `${sectionKey}:${providerId}`,
        sectionKey,
        app: ccSwitchSectionLabel(sectionKey),
        order,
        isCurrent: providerId === current,
        name: String(provider.name ?? providerId),
        website: String(provider.websiteUrl ?? ''),
        apiKeyStatus: hasApiKey ? '已带入' : '留空',
        hasAuth: hasApiKey,
        note: String(provider.notes ?? ''),
      })
    }
  }
  return rows
}

const ccSwitchSectionOptions = computed(() => {
  const seen = new Set<string>()
  return ccSwitchPreviewRows.value
    .filter((row) => {
      if (seen.has(row.sectionKey)) {
        return false
      }
      seen.add(row.sectionKey)
      return true
    })
    .map((row) => ({
      label: `${row.app} (${ccSwitchPreviewRows.value.filter((item) => item.sectionKey === row.sectionKey).length})`,
      value: row.sectionKey,
    }))
})

const ccSwitchFilteredPreviewRows = computed(() => {
  const keyword = ccSwitchPreviewSearch.value.trim().toLowerCase()
  if (!ccSwitchSelectedSections.value.length) {
    return ccSwitchPreviewRows.value.filter((row) =>
      includesSearch([row.app, row.name, row.website, row.note, row.apiKeyStatus], keyword),
    )
  }
  const selected = new Set(ccSwitchSelectedSections.value)
  return ccSwitchPreviewRows.value.filter(
    (row) =>
      selected.has(row.sectionKey) &&
      includesSearch([row.app, row.name, row.website, row.note, row.apiKeyStatus], keyword),
  )
})

const ccSwitchPreviewPayload = computed<Record<string, unknown> | null>(() => {
  if (ccSwitchImportMode.value === 'sql') {
    return ccSwitchResolvedPayload.value
  }
  return parseCCSwitchJsonPayload(ccSwitchImportText.value)
})

const ccSwitchPreviewError = computed(() => {
  const text = ccSwitchImportText.value.trim()
  if (!text) {
    return ''
  }
  if (ccSwitchImportMode.value === 'sql') {
    return ccSwitchResolveError.value
  }
  return ccSwitchPreviewPayload.value ? '' : '当前内容不是有效 JSON。'
})

const ccSwitchPreviewRows = computed<CCSwitchPreviewRow[]>(() => {
  const payload = ccSwitchPreviewPayload.value
  if (!payload) {
    return []
  }
  return parseCCSwitchPreview(payload)
})

const ccSwitchImportPlaceholder = computed(() =>
  ccSwitchImportMode.value === 'sql'
    ? '粘贴 cc-switch 桌面版 SQL 备份内容，解析后会转换为可导入的供应商配置'
    : '粘贴 .web.json 内容，导入后会完整替换当前供应商列表',
)

const ccSwitchFileButtonLabel = computed(() => (ccSwitchImportMode.value === 'sql' ? '选择 SQL 文件' : '选择 JSON 文件'))

const ccSwitchImportOkText = computed(() => (ccSwitchImportMode.value === 'sql' ? '解析并替换' : '开始替换'))

const siteColumns = [
  { title: '站点', key: 'site', width: 280, sorter: (a: Site, b: Site) => a.name.localeCompare(b.name, 'zh-CN') },
  { title: '平台标签', key: 'plugin', width: 148, sorter: (a: Site, b: Site) => pluginNameFor(a.plugin_key).localeCompare(pluginNameFor(b.plugin_key), 'zh-CN') },
  { title: '余额', key: 'balance', width: 132, sorter: (a: Site, b: Site) => (a.last_balance ?? -Infinity) - (b.last_balance ?? -Infinity) },
  { title: '套餐', key: 'package', width: 144, sorter: (a: Site, b: Site) => String(a.package_display ?? '').localeCompare(String(b.package_display ?? ''), 'zh-CN') },
  { title: '签到状态', key: 'checkin_status', width: 116, sorter: (a: Site, b: Site) => String(visibleCheckinStatus(a) ?? '').localeCompare(String(visibleCheckinStatus(b) ?? ''), 'zh-CN') },
  { title: '分组', key: 'group', width: 150, sorter: (a: Site, b: Site) => String(a.group_name ?? '').localeCompare(String(b.group_name ?? ''), 'zh-CN') },
  { title: '连通状态', key: 'status', width: 112, sorter: (a: Site, b: Site) => String(a.connection_status ?? '').localeCompare(String(b.connection_status ?? ''), 'zh-CN') },
  { title: '启用', key: 'enabled', width: 84, sorter: (a: Site, b: Site) => Number(a.is_enabled) - Number(b.is_enabled) },
  { title: '可签到', key: 'participation', width: 100, sorter: (a: Site, b: Site) => Number(checkinMeta.value.get(b.id)?.include_in_checkin ?? false) - Number(checkinMeta.value.get(a.id)?.include_in_checkin ?? false) },
  { title: '操作', key: 'actions', width: 128, fixed: 'right' as const },
]

const checkinRunColumns = [
  { title: '站点', key: 'site', width: 260, sorter: (a: CheckinRun, b: CheckinRun) => String(a.site_name ?? '').localeCompare(String(b.site_name ?? ''), 'zh-CN') },
  { title: '结果', key: 'status', width: 120, sorter: (a: CheckinRun, b: CheckinRun) => a.status.localeCompare(b.status, 'zh-CN') },
  { title: '触发方式', key: 'trigger_type', width: 120, sorter: (a: CheckinRun, b: CheckinRun) => a.trigger_type.localeCompare(b.trigger_type, 'zh-CN') },
  { title: '消息', key: 'message' },
  { title: '开始时间', key: 'started_at', width: 190, sorter: (a: CheckinRun, b: CheckinRun) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime() },
]

const ccSwitchPreviewColumns = [
  { title: '应用', key: 'app', width: 90, sorter: (a: CCSwitchPreviewRow, b: CCSwitchPreviewRow) => a.app.localeCompare(b.app, 'zh-CN') },
  { title: '顺序', key: 'order', width: 80, sorter: (a: CCSwitchPreviewRow, b: CCSwitchPreviewRow) => a.order - b.order },
  { title: '默认', key: 'current', width: 80, sorter: (a: CCSwitchPreviewRow, b: CCSwitchPreviewRow) => Number(a.isCurrent) - Number(b.isCurrent) },
  { title: '名称', key: 'name', width: 180, sorter: (a: CCSwitchPreviewRow, b: CCSwitchPreviewRow) => a.name.localeCompare(b.name, 'zh-CN') },
  { title: '站点', key: 'website', width: 260, sorter: (a: CCSwitchPreviewRow, b: CCSwitchPreviewRow) => a.website.localeCompare(b.website, 'zh-CN') },
  { title: '认证', key: 'apiKeyStatus', width: 90, sorter: (a: CCSwitchPreviewRow, b: CCSwitchPreviewRow) => Number(a.hasAuth) - Number(b.hasAuth) },
  { title: '备注', key: 'note', sorter: (a: CCSwitchPreviewRow, b: CCSwitchPreviewRow) => a.note.localeCompare(b.note, 'zh-CN') },
]

const duplicateColumns = [
  { title: '基础 URL', key: 'base_url', width: 260, sorter: (a: DuplicateSiteGroup, b: DuplicateSiteGroup) => a.base_url.localeCompare(b.base_url, 'zh-CN') },
  { title: '账号', key: 'account', width: 160, sorter: (a: DuplicateSiteGroup, b: DuplicateSiteGroup) => a.account.localeCompare(b.account, 'zh-CN') },
  { title: '密码', key: 'password', width: 90, sorter: (a: DuplicateSiteGroup, b: DuplicateSiteGroup) => Number(a.password_present) - Number(b.password_present) },
  { title: '保留建议', key: 'suggested', width: 280 },
  { title: '重复记录', key: 'sites' },
]

function pluginNameFor(pluginKey: string) {
  return plugins.value.find((plugin) => plugin.key === pluginKey)?.name ?? pluginKey
}

function isImportedSitePayload(payload: { plugin_config?: Record<string, unknown> }) {
  return payload.plugin_config?.cc_switch_source_version !== undefined
}

function displayPluginLabel(site: Site) {
  if (site.plugin_key === 'api-supplier' && isImportedSitePayload(site)) {
    return '导入记录'
  }
  return pluginNameFor(site.plugin_key)
}

function displayGroupName(site: Pick<Site, 'group_name' | 'plugin_config'>) {
  const groupName = formatGroupNames(site.group_name)
  return groupName || '未分组'
}

function rowKey(record: Site) {
  return record.id
}

function ccSwitchPreviewRowKey(record: CCSwitchPreviewRow) {
  return record.key
}

function duplicateGroupRowKey(record: unknown) {
  const group = record as DuplicateSiteGroup
  return `${group.base_url}:${group.account}:${group.suggested_keep_id}`
}

function duplicateSuggestedSiteName(record: unknown) {
  const group = record as DuplicateSiteGroup
  return group.sites.find((site) => site.suggested_keep)?.name || '-'
}

function asSite(record: unknown) {
  return record as Site
}

function normalizeSite(site: Site): Site {
  const balanceUnit = normalizeBalanceUnit(site.balance_unit)
  return {
    ...site,
    balance_unit: balanceUnit,
    balance_display: site.balance_display || formatBalance(site.last_balance, balanceUnit),
    package_unit: normalizeBalanceUnit(site.package_unit, ''),
  }
}

function includesSearch(values: Array<string | number | boolean | null | undefined>, keyword: string) {
  if (!keyword) {
    return true
  }
  return values.some((value) => String(value ?? '').toLowerCase().includes(keyword))
}

const filteredSites = computed(() => {
  const keyword = siteSearch.value.trim().toLowerCase()
  return sites.value.filter((site) =>
    includesSearch(
      [
        site.name,
        site.base_url,
        displayGroupName(site),
        site.group_name,
        displayPluginLabel(site),
        site.notes,
        site.last_message,
        site.balance_display,
        site.package_display,
        siteApiKeyCountLabel(site),
      ],
      keyword,
    ),
  )
})

const filteredDuplicateGroups = computed(() => {
  const keyword = duplicateSearch.value.trim().toLowerCase()
  return duplicateGroups.value.filter((group) =>
    includesSearch(
      [
        group.base_url,
        group.account,
        group.sites.map((site) => site.name).join(' '),
        group.sites.map((site) => site.plugin_key).join(' '),
        group.sites.map((site) => site.notes).join(' '),
      ],
      keyword,
    ),
  )
})

function isBoxyingSite(baseUrl: string) {
  return baseUrl.trim().toLowerCase().includes('boxying.com')
}

function applyPluginConfigDefaults(options?: { force?: boolean }) {
  const force = options?.force ?? false
  const isTargetPlugin = editor.plugin_key === 'yellowpeach-newapi'
  if (!isTargetPlugin || !isBoxyingSite(editor.base_url)) {
    return
  }

  const defaults: Record<string, string> = {
    checkin_mode: 'reward_center',
    reward_calendar_scope: 'gift_calendar_v2',
    reward_claim_action_code: 'daily_gift_claim_v2',
  }

  for (const [key, value] of Object.entries(defaults)) {
    const current = String(editor.plugin_config[key] ?? '').trim()
    if (force || !current) {
      editor.plugin_config[key] = value
    }
  }
}

const manualLoginFieldNames = new Set(['username', 'email', 'password'])
const totpFieldNames = new Set(['totp_secret', 'totp_otpauth_url'])

const primaryCredentialFields = computed(() => {
  const plugin = currentPlugin.value
  if (!plugin) {
    return []
  }
  return plugin.credential_fields.filter(
    (field) => !manualLoginFieldNames.has(field.name) && !totpFieldNames.has(field.name),
  )
})

const manualLoginFields = computed(() => {
  const plugin = currentPlugin.value
  if (!plugin) {
    return []
  }
  return plugin.credential_fields.filter((field) => manualLoginFieldNames.has(field.name))
})

const totpCredentialFields = computed(() => {
  const plugin = currentPlugin.value
  if (!plugin) {
    return []
  }
  return plugin.credential_fields.filter((field) => totpFieldNames.has(field.name))
})

function detectRecommendedPluginKey(baseUrl: string): string | null {
  const normalized = baseUrl.trim().toLowerCase()
  if (!normalized) {
    return null
  }
  if (
    normalized.includes('yellowpeachxgp.com') ||
    normalized.includes('aifamily.vip') ||
    normalized.includes('boxying.com')
  ) {
    return 'yellowpeach-newapi'
  }
  if (normalized.includes('sub2api')) {
    return 'sub2api-platform'
  }
  return null
}

const recommendedPluginKey = computed(() => detectRecommendedPluginKey(editor.base_url))
const recommendedPlugin = computed(
  () => plugins.value.find((item) => item.key === recommendedPluginKey.value) ?? null,
)
const pluginMismatch = computed(
  () =>
    Boolean(recommendedPluginKey.value) &&
    Boolean(editor.plugin_key) &&
    recommendedPluginKey.value !== editor.plugin_key,
)

const officialSiteUrl = computed(() => buildExternalUrl(editor.base_url))
const authEntryUrl = computed(() =>
  buildExternalUrl(editor.base_url, currentPlugin.value?.auth_entry_path ?? ''),
)
const authEntryLabel = computed(() => (currentPlugin.value?.auth_entry_label || '').trim())
const showAuthEntryButton = computed(() => {
  if (!authEntryLabel.value || !authEntryUrl.value) {
    return false
  }
  if (authEntryUrl.value === officialSiteUrl.value && authEntryLabel.value === '打开官网') {
    return false
  }
  return true
})
const currentPluginCapabilities = computed(() => new Set(currentPlugin.value?.capabilities ?? []))
const isRelayOnlyEditor = computed(() => currentPluginCapabilities.value.has('relay_only'))
const canBatchRegisterEditor = computed(() => currentPluginCapabilities.value.has('account_registration'))
const testActionLabel = computed(() => (isRelayOnlyEditor.value ? '验证出口' : '测试连接'))
const primaryActionLabel = computed(() => {
  const capabilities = currentPluginCapabilities.value
  const customCheckinUrl = String(editor.plugin_config.checkin_url ?? '').trim()
  const disableSub2ApiCheckin =
    editor.plugin_key === 'sub2api-platform' &&
    !customCheckinUrl &&
    ['1', 'true', 'yes', 'on'].includes(String(editor.plugin_config.disable_checkin ?? '').trim().toLowerCase())
  if (capabilities.has('checkin') && !disableSub2ApiCheckin) {
    return '立即签到'
  }
  if (capabilities.has('api_key_sync')) {
    return '同步资料'
  }
  return '执行同步'
})

const mismatchAcknowledged = ref(false)

function assignEditor(site?: Site | null) {
  const fallbackPlugin = plugins.value[0]?.key ?? ''
  editor.name = site?.name ?? ''
  editor.base_url = site?.base_url ?? ''
  editor.plugin_key =
    site?.plugin_key ??
    detectRecommendedPluginKey(site?.base_url ?? '') ??
    fallbackPlugin
  editor.group_name = site?.group_name ?? ''
  editor.supported_models = normalizeSupportedModels(site?.supported_models ?? null)
  editor.is_enabled = site?.is_enabled ?? true
  editor.notes = site?.notes ?? ''
  editor.credentials = { ...(site?.credentials ?? {}) }
  editor.plugin_config = { ...(site?.plugin_config ?? {}) }
  editorGroupNames.value = parseGroupNames(editor.group_name)
  applyPluginConfigDefaults()
  mismatchAcknowledged.value = false
}

function checkinMetaFor(site: Site) {
  return checkinMeta.value.get(site.id)
}

function siteCanCheckin(site: Site) {
  return Boolean(checkinMetaFor(site)?.can_checkin)
}

function siteIncludedInCheckin(site: Site) {
  return Boolean(checkinMetaFor(site)?.include_in_checkin)
}

function visibleCheckinStatus(site: Site) {
  if (!siteCanCheckin(site) || !siteIncludedInCheckin(site)) {
    return null
  }
  return site.checkin_status ?? null
}

function siteSupportsInvite(site: Site) {
  if (isRelayOnlySitePayload(site)) {
    return false
  }
  const plugin = plugins.value.find((item) => item.key === site.plugin_key)
  return Boolean(plugin?.capabilities?.includes('account_status'))
}

function readSiteInviteInfo(site: Site) {
  return {
    link: String(site.plugin_config?.invite_link ?? '').trim(),
    code: String(site.plugin_config?.invite_code ?? '').trim(),
  }
}

function siteRunnableForCheckin(site: Site) {
  return site.is_enabled && siteCanCheckin(site) && siteIncludedInCheckin(site)
}

const includedCheckinCount = computed(() =>
  sites.value.filter((site) => siteIncludedInCheckin(site)).length,
)

const connectivitySweepLabel = computed(() => {
  if (!connectivitySweepProgress.value) {
    return '连通测试'
  }
  const progress = connectivitySweepProgress.value
  return `连通中 ${progress.done}/${progress.total}`
})

const inviteRefreshAllLabel = computed(() => {
  if (!inviteRefreshProgress.value) {
    return '刷新邀请'
  }
  const progress = inviteRefreshProgress.value
  return `邀请中 ${progress.done}/${progress.total}`
})

const apiKeyRefreshAllLabel = computed(() => {
  if (!apiKeyRefreshProgress.value) {
    return '更新全部 API Key'
  }
  const progress = apiKeyRefreshProgress.value
  return `更新中 ${progress.done}/${progress.total}`
})

async function syncRoutesAfterApiKeyUpdate(successCount: number) {
  if (successCount <= 0) {
    return
  }
  await syncRoutesAfterSiteChange()
}

async function syncRoutesAfterSiteChange() {
  try {
    const result = await syncGatewayRoutes()
    toast.success(`路由池已同步：${result.route_count} 条路由。`)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '路由池同步失败')
  }
}

const checkinAllIncludedLabel = computed(() => {
  if (!checkinBatchProgress.value) {
    return '签到全部已加入'
  }
  const progress = checkinBatchProgress.value
  return `签到中 ${progress.done}/${progress.total}`
})

const filteredCheckinRuns = computed(() => {
  const keyword = checkinRunSearch.value.trim().toLowerCase()
  if (!keyword) {
    return checkinRuns.value
  }
  return checkinRuns.value.filter((run) =>
    [run.site_name, run.status, run.trigger_type, run.message]
      .some((value) => String(value ?? '').toLowerCase().includes(keyword)),
  )
})

const checkinRowSelection = computed(() => ({
  selectedRowKeys: selectedCheckinIds.value,
  onChange: (keys: Array<string | number>) => {
    selectedCheckinIds.value = keys
      .map((item) => Number(item))
      .filter((item) => Number.isFinite(item))
  },
  getCheckboxProps: (record: Site) => ({
    disabled: !siteRunnableForCheckin(record),
  }),
}))

function syncSelectedCheckinIds() {
  const available = new Set(sites.value.filter(siteRunnableForCheckin).map((site) => site.id))
  selectedCheckinIds.value = selectedCheckinIds.value.filter((id) => available.has(id))
}

async function runSiteBatch<T>(items: T[], worker: (item: T) => Promise<void>, concurrency = 4) {
  let nextIndex = 0
  const workerCount = Math.min(Math.max(1, concurrency), items.length)
  await Promise.all(
    Array.from({ length: workerCount }, async () => {
      while (nextIndex < items.length) {
        const item = items[nextIndex]
        nextIndex += 1
        await worker(item)
      }
    }),
  )
}

async function loadCheckinExtras() {
  try {
    const [siteMeta, runs, settingsData] = await Promise.all([getCheckinSites(), getRuns(60), getSettings()])
    const map = new Map<number, CheckinSite>()
    siteMeta.forEach((item) => map.set(item.id, item))
    checkinMeta.value = map
    checkinRuns.value = runs
    Object.assign(checkinConfigForm, settingsData)
    syncSelectedCheckinIds()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '签到信息加载失败')
  }
}

async function handleParticipationToggle(site: Site, checked: boolean | string | number) {
  const include = checked === true || checked === 'true' || checked === 1
  try {
    await updateCheckinParticipation(site.id, include)
    toast.success(include ? '已加入签到任务。' : '已移出签到任务。')
    await loadCheckinExtras()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '更新失败')
  }
}

async function executeCheckinBatch(siteIds: number[], onlyEnabled: boolean) {
  const targets = siteIds.length
    ? sites.value.filter((site) => siteIds.includes(site.id))
    : sites.value.filter((site) => (onlyEnabled ? siteRunnableForCheckin(site) : true))

  if (!targets.length) {
    toast.error('当前没有可执行的站点。')
    return
  }

  busy.value = true
  checkinBatchProgress.value = { total: targets.length, done: 0, success: 0, failed: 0 }
  try {
    for (const site of targets) {
      try {
        const result = await runSiteCheckin(site.id)
        applyCheckinResultForSite(site.id, result)
        if (result.status === 'success') {
          checkinBatchProgress.value.success += 1
        } else {
          checkinBatchProgress.value.failed += 1
        }
      } catch (err) {
        checkinBatchProgress.value.failed += 1
        applyCheckinResultForSite(site.id, {
          status: 'failed',
          message: err instanceof Error ? err.message : '执行失败',
          balance: site.last_balance,
          balance_unit: null,
        })
      } finally {
        checkinBatchProgress.value.done += 1
      }
    }
    toast.success(
      `签到完成：成功 ${checkinBatchProgress.value.success}，失败 ${checkinBatchProgress.value.failed}。`,
    )
    await loadCheckinExtras()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '签到执行失败')
  } finally {
    checkinBatchProgress.value = null
    busy.value = false
  }
}

async function handleCheckinAllIncluded() {
  await executeCheckinBatch([], true)
}

async function handleCheckinSelected() {
  if (!selectedCheckinIds.value.length) {
    return
  }
  await executeCheckinBatch([...selectedCheckinIds.value], false)
}

async function handleRunSchedulerNow() {
  busy.value = true
  try {
    const result = await runSchedulerNow()
    toast.success(result.message)
    await Promise.all([loadData(selectedId.value), loadCheckinExtras()])
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '执行失败')
  } finally {
    busy.value = false
  }
}

async function saveCheckinConfig() {
  checkinSettingsBusy.value = true
  try {
    Object.assign(checkinConfigForm, await updateSettings(checkinConfigForm))
    toast.success('签到配置已保存。')
    checkinConfigOpen.value = false
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    checkinSettingsBusy.value = false
  }
}

function formatCheckinRunTime(value: string | null) {
  if (!value) return '暂无'
  return new Date(value).toLocaleString('zh-CN')
}

async function loadData(
  preferredId: number | null = selectedId.value,
  options: { preserveEditor?: boolean } = {},
) {
  busy.value = true
  try {
    const [pluginData, siteData, groupData] = await Promise.all([getPlugins(), getSites(), getSiteGroups()])
    plugins.value = pluginData
    sites.value = siteData.map(normalizeSite)
    siteGroups.value = groupData

    const nextSelected =
      preferredId !== null ? sites.value.find((item) => item.id === preferredId) ?? sites.value[0] ?? null : sites.value[0] ?? null

    if (nextSelected) {
      selectedId.value = nextSelected.id
    } else {
      selectedId.value = null
    }

    if (editingId.value !== null && !options.preserveEditor) {
      const refreshedEditing = siteData.find((item) => item.id === editingId.value) ?? null
      if (refreshedEditing) {
        const fullSite = await getSite(refreshedEditing.id)
        assignEditor(fullSite)
      }
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '加载失败')
  } finally {
    busy.value = false
  }
}

async function reloadDataWithCheckinExtras(
  preferredId: number | null = selectedId.value,
  options: { preserveEditor?: boolean } = {},
) {
  await Promise.all([loadData(preferredId, options), loadCheckinExtras()])
}

function openCreateDrawer() {
  editingId.value = null
  assignEditor(null)
  localStorageRawText.value = ''
  testFeedback.value = null
  saveFeedback.value = null
  lastSavedEditorSnapshot.value = ''
  resetBatchRegisterForm()
  drawerOpen.value = true
}

function resetBatchRegisterForm() {
  batchRegisterEnabled.value = false
  batchRegisterResult.value = null
  batchRegisterForm.email_pattern = ''
  batchRegisterForm.password = ''
  batchRegisterForm.count = 3
  batchRegisterForm.start_index = 1
}

async function openEditDrawer(site: Site) {
  busy.value = true
  editingId.value = site.id
  localStorageRawText.value = ''
  testFeedback.value = null
  saveFeedback.value = null
  try {
    const fullSite = await getSite(site.id)
    assignEditor(fullSite)
    lastSavedEditorSnapshot.value = JSON.stringify(editor)
    drawerOpen.value = true
  } catch (err) {
    editingId.value = null
    toast.error(err instanceof Error ? err.message : '站点详情加载失败')
  } finally {
    busy.value = false
  }
}

function closeDrawer() {
  drawerOpen.value = false
  testFeedback.value = null
  saveFeedback.value = null
  lastSavedEditorSnapshot.value = ''
}

function selectSite(site: Site) {
  selectedId.value = site.id
  testFeedback.value = null
}

function ensureField(model: Record<string, any>, key: string, type: string) {
  if (model[key] === undefined) {
    model[key] = type === 'number' ? 0 : ''
  }
}

function configTextValue(key: string): string | number | undefined {
  const value = editor.plugin_config[key] as unknown
  if (value === undefined || value === null) {
    return undefined
  }
  if (Array.isArray(value)) {
    return value.map((item) => String(item ?? '').trim()).filter(Boolean).join('\n')
  }
  if (typeof value === 'number') {
    return value
  }
  if (typeof value === 'string' || typeof value === 'boolean') {
    return String(value)
  }
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

function configNumberValue(key: string): number | undefined {
  const value = editor.plugin_config[key]
  if (typeof value === 'number') {
    return value
  }
  if (typeof value === 'string' && value.trim()) {
    const numeric = Number(value)
    if (Number.isFinite(numeric)) {
      return numeric
    }
  }
  return undefined
}

function updateConfigField(key: string, value: string | number | null) {
  editor.plugin_config[key] = value ?? ''
}

function normalizeStringList(values: unknown): string[] {
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

function normalizeSupportedModels(values: unknown): string[] | null {
  const normalized = normalizeStringList(values)
  return normalized.length ? normalized : null
}

function apiKeyDraftKey(entry: Pick<SiteApiKeyEntry, 'id' | 'entryIndex' | 'key'>): string {
  return `${entry.id || entry.key}:${entry.entryIndex}`
}

function resetApiKeyRequestUrlDrafts(entries: SiteApiKeyEntry[]) {
  for (const key of Object.keys(apiKeyRequestUrlDrafts)) {
    delete apiKeyRequestUrlDrafts[key]
  }
  for (const key of Object.keys(apiKeyRoutePathDrafts)) {
    delete apiKeyRoutePathDrafts[key]
  }
  for (const key of Object.keys(apiKeyImageGenerationPathDrafts)) {
    delete apiKeyImageGenerationPathDrafts[key]
  }
  for (const key of Object.keys(apiKeyImageEditPathDrafts)) {
    delete apiKeyImageEditPathDrafts[key]
  }
  for (const entry of entries) {
    const key = apiKeyDraftKey(entry)
    apiKeyRequestUrlDrafts[key] = entry.requestBaseURLs.join('\n')
    apiKeyRoutePathDrafts[key] = entry.routePath
    apiKeyImageGenerationPathDrafts[key] = entry.imageGenerationPath
    apiKeyImageEditPathDrafts[key] = entry.imageEditPath
  }
}

function apiKeyRequestUrlDraft(entry: SiteApiKeyEntry): string {
  return apiKeyRequestUrlDrafts[apiKeyDraftKey(entry)] ?? entry.requestBaseURLs.join('\n')
}

function updateApiKeyRequestUrlDraft(entry: SiteApiKeyEntry, value: string) {
  const key = apiKeyDraftKey(entry)
  apiKeyRequestUrlDrafts[key] = value
  upsertApiKeyDialogSiteCredentials((site, credentials) => setApiKeyRequestBaseURLs({
    ...credentials,
    api_keys: mergeApiKeyEntries(storedApiKeyEntriesForEdit({ ...site, credentials })),
  }, entry.key, value, entry.entryIndex))
}

function apiKeyRoutePathDraft(entry: SiteApiKeyEntry): string {
  return apiKeyRoutePathDrafts[apiKeyDraftKey(entry)] ?? entry.routePath
}

function updateApiKeyRoutePathDraft(entry: SiteApiKeyEntry, value: unknown) {
  const key = apiKeyDraftKey(entry)
  const routePath = typeof value === 'string' ? value : ''
  apiKeyRoutePathDrafts[key] = routePath
  upsertApiKeyDialogSiteCredentials((site, credentials) => setApiKeyRoutePath({
    ...credentials,
    api_keys: mergeApiKeyEntries(storedApiKeyEntriesForEdit({ ...site, credentials })),
  }, entry.key, routePath, entry.entryIndex))
}

function apiKeyImageGenerationPathDraft(entry: SiteApiKeyEntry): string {
  return apiKeyImageGenerationPathDrafts[apiKeyDraftKey(entry)] ?? entry.imageGenerationPath
}

function apiKeyImageEditPathDraft(entry: SiteApiKeyEntry): string {
  return apiKeyImageEditPathDrafts[apiKeyDraftKey(entry)] ?? entry.imageEditPath
}

function updateApiKeyImagePathDraft(entry: SiteApiKeyEntry, field: 'generation' | 'edit', value: string) {
  const key = apiKeyDraftKey(entry)
  if (field === 'generation') {
    apiKeyImageGenerationPathDrafts[key] = value
  } else {
    apiKeyImageEditPathDrafts[key] = value
  }
  upsertApiKeyDialogSiteCredentials((site, credentials) => setApiKeyImagePaths({
    ...credentials,
    api_keys: mergeApiKeyEntries(storedApiKeyEntriesForEdit({ ...site, credentials })),
  }, entry.key, apiKeyImageGenerationPathDrafts[key] ?? entry.imageGenerationPath, apiKeyImageEditPathDrafts[key] ?? entry.imageEditPath, entry.entryIndex))
}

function supportedModelsPreview(values: unknown, limit = 2): string {
  const normalized = normalizeStringList(values)
  if (!normalized.length) {
    return '未声明'
  }
  const preview = normalized.slice(0, limit).join(' / ')
  return normalized.length > limit ? `${preview} 等 ${normalized.length} 个` : preview
}

function normalizeApiKeyRouteType(value: unknown): string {
  const normalized = String(value ?? '').trim().toLowerCase()
  if (['general', 'auto', 'any', 'none', 'default'].includes(normalized)) {
    return 'general'
  }
  if (normalized === 'claude' || normalized === 'anthropic') {
    return 'claude'
  }
  if (normalized === 'gemini' || normalized === 'google') {
    return 'gemini'
  }
  if (['gpt', 'openai', 'chatgpt', 'chat', 'chat_completions', 'chat-completions'].includes(normalized)) {
    return 'gpt'
  }
  if (['codex', 'response', 'responses'].includes(normalized)) {
    return 'codex'
  }
  return ''
}

function defaultApiKeyRouteType(site: Pick<Site, 'plugin_config'>): string {
  const config = site.plugin_config as Record<string, unknown>
  return normalizeApiKeyRouteType(config?.gateway_route_type) || normalizeApiKeyRouteType(config?.api_format) || 'codex'
}

function defaultApiKeyRoutePath(routeType: string): string {
  const normalized = normalizeApiKeyRouteType(routeType)
  if (normalized === 'gpt') {
    return 'chat/completions'
  }
  if (normalized === 'codex') {
    return 'responses'
  }
  return ''
}

function storedApiKeyEntriesForEdit(site: Pick<Site, 'credentials' | 'plugin_config'>): SiteApiKeyRecord[] {
  const credentials = site.credentials as Record<string, unknown>
  const entries = storedApiKeyEntries(credentials)
  const primaryKey = String(credentials?.api_key ?? '').trim()
  if (primaryKey && !entries.some((item) => apiKeyEntryValue(item, 'key') === primaryKey)) {
    return [
      {
        id: 'primary',
        name: '默认 Key',
        key: primaryKey,
        status: 'active',
        source: 'manual',
        route_type: defaultApiKeyRouteType(site),
        api_type: defaultApiKeyRouteType(site),
      },
      ...entries,
    ]
  }
  return entries
}

function siteApiKeyEntries(site: Pick<Site, 'credentials'>): SiteApiKeyEntry[] {
  const credentials = site.credentials as Record<string, unknown>
  const raw = storedApiKeyEntries(credentials)
  if (raw.length) {
    return raw
      .map((item, index) => {
        const entry = item
        const key = apiKeyEntryValue(entry, 'key')
        if (!key) {
          return null
        }
        const source = apiKeyEntryValue(entry, 'source')
        const routeType = normalizeApiKeyRouteType(
          entry?.route_type ?? entry?.api_type ?? entry?.api_format ?? entry?.type,
        )
        return {
          id: apiKeyEntryValue(entry, 'id') || `${source || 'api-key'}-${index}`,
          entryIndex: index,
          name: apiKeyEntryValue(entry, 'name') || `Key ${index + 1}`,
          key,
          status: apiKeyEntryValue(entry, 'status') || 'unknown',
          isPrimary: Boolean(entry?.is_primary) || key === apiKeyValue(credentials),
          source,
          routeType,
          routePath: apiKeyRoutePath(entry),
          requestBaseURLs: apiKeyRequestBaseURLs(entry),
          imageGenerationPath: apiKeyImageGenerationPath(entry),
          imageEditPath: apiKeyImageEditPath(entry),
          isManual: isManualApiKeyEntry(entry),
        }
      })
      .filter((item): item is SiteApiKeyEntry => Boolean(item))
  }

  const fallback = apiKeyValue(site.credentials as Record<string, unknown>)
  if (!fallback) {
    return []
  }
  return [
    {
      id: 'primary',
      entryIndex: 0,
      name: '默认 Key',
      key: fallback,
      status: 'active',
      isPrimary: true,
      source: '',
      routeType: '',
      routePath: '',
      requestBaseURLs: [],
      imageGenerationPath: '',
      imageEditPath: '',
      isManual: false,
    },
  ]
}

function siteApiKeyCount(site: Pick<Site, 'credentials'>): number {
  return siteApiKeyEntries(site).length
}

function siteSupportsApiKeySync(site: Pick<Site, 'plugin_key'>): boolean {
  return Boolean(pluginForKey(site.plugin_key)?.capabilities.includes('api_key_sync'))
}

function siteApiKeyCountNeedsEndpoint(site: Pick<Site, 'credentials' | 'plugin_key'>): boolean {
  return siteApiKeyCount(site) === 0 && siteSupportsApiKeySync(site)
}

function siteApiKeyCountLabel(site: Pick<Site, 'credentials' | 'plugin_key'>): string {
  const count = siteApiKeyCount(site)
  if (count > 0) {
    return `${count} 个`
  }
  if (siteApiKeyCountNeedsEndpoint(site)) {
    return '补充 apikey 接口路径'
  }
  return '0'
}

function siteApiKeyCountTagColor(site: Pick<Site, 'credentials' | 'plugin_key'>): string {
  if (siteApiKeyCount(site) > 0) {
    return 'green'
  }
  return siteApiKeyCountNeedsEndpoint(site) ? 'warning' : 'default'
}

function requestApiUrlText(site: Pick<Site, 'plugin_config'>): string {
  const raw = (site.plugin_config as Record<string, unknown>)?.api_request_urls
  return normalizeStringList(raw).join('\n')
}

function defaultRequestApiUrl(site: Pick<Site, 'base_url' | 'plugin_config'>): string {
  const pluginConfig = site.plugin_config as Record<string, unknown>
  return normalizeStringList([
    ...normalizeStringList(pluginConfig?.gateway_request_urls),
    String(pluginConfig?.gateway_request_url ?? '').trim(),
    String(pluginConfig?.endpoint_url ?? '').trim(),
    String(site.base_url ?? '').trim(),
  ])[0] ?? ''
}

const apiKeyDialogSite = computed(() =>
  apiKeyDialogSiteId.value !== null ? sites.value.find((site) => site.id === apiKeyDialogSiteId.value) ?? null : null,
)

const apiKeyDialogPreviewUrls = computed(() => {
  const edited = normalizeStringList(apiKeyDialogForm.request_api_urls)
  if (edited.length) {
    return edited
  }
  const fallback = apiKeyDialogForm.endpoint_hint.trim()
  return fallback ? [fallback] : []
})

const apiKeyDialogEntries = computed(() => {
  const site = apiKeyDialogSite.value
  return site ? siteApiKeyEntries(site) : []
})

const manualApiKeyEntries = computed(() =>
  apiKeyDialogEntries.value.filter((entry) => entry.isManual),
)

const apiKeyRouteTypeOptions = [
  { label: '通用', value: 'general' },
  { label: 'GptChat', value: 'gpt' },
  { label: 'Codex', value: 'codex' },
  { label: 'Claude', value: 'claude' },
  { label: 'Gemini', value: 'gemini' },
]

const apiKeyRoutePathOptions = [
  { label: '跟随客户端', value: '' },
  { label: '/v1/chat/completions', value: 'chat/completions' },
  { label: '/v1/responses', value: 'responses' },
]

function siteCheckinActionLabel(site: Site) {
  return checkinMetaFor(site)?.checkin_label || '签到'
}

function balanceClass(balance: number | null | undefined) {
  const tone = balanceTone(balance)
  return tone === 'empty' ? '' : `balance-value balance-value--${tone}`
}

function applySiteSummary(summary: SiteSummary) {
  const target = sites.value.find((site) => site.id === summary.site_id)
  if (!target) {
    return
  }
  target.last_status = summary.last_status
  target.connection_status = summary.connection_status
  target.last_message = summary.last_message
  target.last_balance = summary.last_balance
  target.balance_display = summary.balance_display
  applyPackageQuota(target, summary)
  target.package_display = summary.package_display
  if (summary.invite_link) {
    target.plugin_config = {
      ...target.plugin_config,
      invite_link: summary.invite_link,
    }
  }
  if (summary.invite_code) {
    target.plugin_config = {
      ...target.plugin_config,
      invite_code: summary.invite_code,
    }
  }
  target.checkin_status = summary.checkin_status
  target.last_run_at = summary.last_run_at
}

function applyBalanceProbeResult(result: { site_id: number; last_balance: number | null; remaining: number | null; unit?: string }) {
  const target = sites.value.find((site) => site.id === result.site_id)
  if (!target) {
    return
  }
  const balance = result.last_balance ?? result.remaining
  target.last_balance = balance
  target.balance_unit = normalizeBalanceUnit(result.unit)
  target.balance_display = formatBalance(balance, result.unit)
}

function isInviteLoading(siteId: number) {
  return inviteLoadingSiteIds.value.includes(siteId)
}

function isApiKeyRefreshing(siteId: number) {
  return apiKeyRefreshingSiteIds.value.includes(siteId)
}

function applyInvitePluginConfig(targetSite: Site, updates?: Record<string, any>) {
  if (!updates) {
    return
  }
  targetSite.plugin_config = {
    ...targetSite.plugin_config,
    ...updates,
  }
}

function applyInviteRefreshResult(result: SiteInviteRefreshResult) {
  const target = sites.value.find((site) => site.id === result.site_id)
  if (!target) {
    return
  }
  applyInvitePluginConfig(target, result.updated_plugin_config)
  applyPackageQuota(target, result)
  if (result.invite_link) {
    target.plugin_config = {
      ...target.plugin_config,
      invite_link: result.invite_link,
    }
  }
  if (result.invite_code) {
    target.plugin_config = {
      ...target.plugin_config,
      invite_code: result.invite_code,
    }
  }
  if (result.package_display) {
    target.package_display = result.package_display
    target.plugin_config = {
      ...target.plugin_config,
      package_display: result.package_display,
    }
  }
}

function applyPackageQuota(target: Site, source: {
  package_remaining?: number | null
  package_total?: number | null
  package_used?: number | null
  package_unit?: string | null
}) {
  if (source.package_remaining !== undefined) {
    target.package_remaining = source.package_remaining
  }
  if (source.package_total !== undefined) {
    target.package_total = source.package_total
  }
  if (source.package_used !== undefined) {
    target.package_used = source.package_used
  }
  if (source.package_unit !== undefined) {
    target.package_unit = normalizeBalanceUnit(source.package_unit, '')
  }
}

function applyApiKeyRefreshResult(result: SiteApiKeyRefreshResult) {
  const target = sites.value.find((site) => site.id === result.site_id)
  if (!target || !result.updated_credentials) {
    return
  }
  target.credentials = {
    ...(target.credentials as Record<string, unknown>),
    ...result.updated_credentials,
  } as Site['credentials']
}

async function refreshAllInvites() {
  const targets = sites.value.filter((site) => site.is_enabled)
  if (!targets.length) {
    toast.error('当前没有启用站点可刷新邀请。')
    return
  }

  inviteRefreshAllLoading.value = true
  inviteRefreshProgress.value = { total: targets.length, done: 0, success: 0, failed: 0 }
  inviteLoadingSiteIds.value = Array.from(new Set([...inviteLoadingSiteIds.value, ...targets.map((site) => site.id)]))
  try {
    await runSiteBatch(targets, async (site) => {
      try {
        const results = await refreshSiteInvites({ site_ids: [site.id], only_enabled: true })
        const result = results[0]
        if (result) {
          applyInviteRefreshResult(result)
        }
        if (result?.ok) {
          inviteRefreshProgress.value!.success += 1
        } else {
          inviteRefreshProgress.value!.failed += 1
        }
      } catch {
        inviteRefreshProgress.value!.failed += 1
      } finally {
        inviteRefreshProgress.value!.done += 1
        inviteLoadingSiteIds.value = inviteLoadingSiteIds.value.filter((siteId) => siteId !== site.id)
      }
    })
    if (inviteRefreshProgress.value.success > 0) {
      toast.success(`邀请刷新完成：成功 ${inviteRefreshProgress.value.success}，失败 ${inviteRefreshProgress.value.failed}。`)
    } else {
      toast.error('未刷新到可用邀请信息。')
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '批量刷新邀请失败')
  } finally {
    const refreshedIds = new Set(targets.map((site) => site.id))
    inviteLoadingSiteIds.value = inviteLoadingSiteIds.value.filter((siteId) => !refreshedIds.has(siteId))
    inviteRefreshProgress.value = null
    inviteRefreshAllLoading.value = false
  }
}

async function refreshAllApiKeys() {
  const targets = sites.value.filter((site) => site.is_enabled && siteSupportsApiKeySync(site))
  if (!targets.length) {
    toast.error('当前没有可更新 API Key 的启用站点。')
    return
  }

  apiKeyRefreshAllLoading.value = true
  apiKeyRefreshProgress.value = { total: targets.length, done: 0, success: 0, failed: 0 }
  apiKeyRefreshingSiteIds.value = Array.from(new Set([...apiKeyRefreshingSiteIds.value, ...targets.map((site) => site.id)]))
  try {
    await runSiteBatch(targets, async (site) => {
      try {
        const result = await refreshOneSiteApiKeys(site.id)
        applyApiKeyRefreshResult(result)
        if (result.ok) {
          apiKeyRefreshProgress.value!.success += 1
        } else {
          apiKeyRefreshProgress.value!.failed += 1
        }
      } catch {
        apiKeyRefreshProgress.value!.failed += 1
      } finally {
        apiKeyRefreshProgress.value!.done += 1
        apiKeyRefreshingSiteIds.value = apiKeyRefreshingSiteIds.value.filter((siteId) => siteId !== site.id)
      }
    })
    if (apiKeyRefreshProgress.value.success > 0) {
      toast.success(`API Key 更新完成：成功 ${apiKeyRefreshProgress.value.success}，失败 ${apiKeyRefreshProgress.value.failed}。`)
      await syncRoutesAfterApiKeyUpdate(apiKeyRefreshProgress.value.success)
    } else {
      toast.error('未更新到可用 API Key。')
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '批量更新 API Key 失败')
  } finally {
    const refreshedIds = new Set(targets.map((site) => site.id))
    apiKeyRefreshingSiteIds.value = apiKeyRefreshingSiteIds.value.filter((siteId) => !refreshedIds.has(siteId))
    apiKeyRefreshProgress.value = null
    apiKeyRefreshAllLoading.value = false
  }
}

async function loadInviteInfo(targetSite: Site, options: { openWhenReady?: boolean; force?: boolean } = {}) {
  const { openWhenReady = true, force = false } = options
  inviteDialogSiteId.value = targetSite.id
  inviteDialogSiteName.value = targetSite.name

  if (!force) {
    const cached = readSiteInviteInfo(targetSite)
    if (cached.link || cached.code) {
      inviteDialogLink.value = cached.link
      inviteDialogCode.value = cached.code
      if (openWhenReady) {
        inviteDialogOpen.value = true
      }
      return
    }
  }

  inviteLoadingSiteIds.value = [...inviteLoadingSiteIds.value, targetSite.id]
  inviteDialogLoading.value = openWhenReady
  try {
    const result = await testSite(targetSite.id)
    applyInvitePluginConfig(targetSite, result.updated_plugin_config)
    if (result.invite_link) {
      targetSite.plugin_config = {
        ...targetSite.plugin_config,
        invite_link: result.invite_link,
      }
    }
    if (result.invite_code) {
      targetSite.plugin_config = {
        ...targetSite.plugin_config,
        invite_code: result.invite_code,
      }
    }
    inviteDialogLink.value = String(result.invite_link ?? targetSite.plugin_config?.invite_link ?? '').trim()
    inviteDialogCode.value = String(result.invite_code ?? targetSite.plugin_config?.invite_code ?? '').trim()
    if (!inviteDialogLink.value && !inviteDialogCode.value) {
      throw new Error('未从站点账号读取到邀请链接或邀请码。')
    }
    if (openWhenReady) {
      inviteDialogOpen.value = true
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '邀请信息读取失败')
  } finally {
    inviteLoadingSiteIds.value = inviteLoadingSiteIds.value.filter((item) => item !== targetSite.id)
    inviteDialogLoading.value = false
  }
}

async function copyInviteLink() {
  const value = inviteDialogLink.value.trim()
  if (!value) {
    toast.error('当前站点未返回邀请链接。')
    return
  }
  try {
    await navigator.clipboard.writeText(value)
    toast.success('邀请链接已复制。')
  } catch {
    toast.error('复制失败，请手动复制。')
  }
}

async function copyInviteCode() {
  const value = inviteDialogCode.value.trim()
  if (!value) {
    toast.error('当前站点未返回邀请码。')
    return
  }
  try {
    await navigator.clipboard.writeText(value)
    toast.success('邀请码已复制。')
  } catch {
    toast.error('复制失败，请手动复制。')
  }
}

async function copyInviteBundle() {
  const parts = [
    inviteDialogLink.value.trim() ? `邀请链接：${inviteDialogLink.value.trim()}` : '',
    inviteDialogCode.value.trim() ? `邀请码：${inviteDialogCode.value.trim()}` : '',
  ].filter(Boolean)
  if (!parts.length) {
    toast.error('当前站点没有可复制的邀请信息。')
    return
  }
  try {
    await navigator.clipboard.writeText(parts.join('\n'))
    toast.success('邀请信息已复制。')
  } catch {
    toast.error('复制失败，请手动复制。')
  }
}

function refreshInviteDialog() {
  if (inviteDialogSiteId.value === null) {
    return
  }
  const site = sites.value.find((item) => item.id === inviteDialogSiteId.value)
  if (!site) {
    toast.error('当前站点不存在。')
    return
  }
  void loadInviteInfo(site, { force: true })
}

async function handleRefreshSiteApiKeys(site: Site) {
  if (!siteSupportsApiKeySync(site)) {
    toast.error('当前插件不支持 API Key 同步。')
    return
  }
  apiKeyRefreshingSiteIds.value = Array.from(new Set([...apiKeyRefreshingSiteIds.value, site.id]))
  try {
    const result = await refreshSiteApiKeys({ site_ids: [site.id], only_enabled: false }).then((items) => items[0])
    if (!result) {
      throw new Error('站点未返回 API Key 更新结果。')
    }
    applyApiKeyRefreshResult(result)
    if (result.ok) {
      toast.success(`${site.name} ${result.message || `已更新 ${result.api_key_count} 个 API Key。`}`)
      await syncRoutesAfterApiKeyUpdate(1)
    } else {
      toast.error(`${site.name} ${result.message || '未更新到可用 API Key。'}`)
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'API Key 更新失败')
  } finally {
    apiKeyRefreshingSiteIds.value = apiKeyRefreshingSiteIds.value.filter((item) => item !== site.id)
  }
}

function applyCheckinResultForSite(
  siteId: number,
  result: {
    status: string
    message: string
    balance: number | null
    balance_unit?: string | null
    balance_display?: string | null
    package_remaining?: number | null
    package_total?: number | null
    package_used?: number | null
    package_unit?: string | null
    package_display?: string | null
    checkin_status?: string | null
    connection_status?: string | null
  },
) {
  const target = sites.value.find((site) => site.id === siteId)
  if (!target) {
    return
  }
  const now = new Date().toISOString()
  target.last_status = result.status
  target.connection_status = result.connection_status ?? result.status
  target.checkin_status = result.checkin_status ?? result.status
  target.last_message = result.message
  if (result.balance !== null && result.balance !== undefined && !Number.isNaN(result.balance)) {
    target.last_balance = result.balance
    target.balance_unit = normalizeBalanceUnit(result.balance_unit)
    target.balance_display = result.balance_display || formatBalance(result.balance, result.balance_unit)
  }
  applyPackageQuota(target, result)
  if (result.package_display) {
    target.package_display = result.package_display
  }
  target.last_run_at = now
}

async function openApiKeyDialog(site: Site) {
  apiKeyDialogSaving.value = true
  try {
    const fullSite = await getSite(site.id)
    const index = sites.value.findIndex((item) => item.id === fullSite.id)
    if (index >= 0) {
      sites.value[index] = fullSite
    }
    apiKeyDialogSiteId.value = fullSite.id
    apiKeyDialogForm.site_name = fullSite.name
    apiKeyDialogForm.request_api_urls = requestApiUrlText(fullSite)
    apiKeyDialogForm.endpoint_hint = defaultRequestApiUrl(fullSite)
    apiKeyDialogForm.image_generation_path = String(fullSite.plugin_config?.image_generation_path ?? '').trim()
    apiKeyDialogForm.image_edit_path = String(fullSite.plugin_config?.image_edit_path ?? '').trim()
    manualApiKeyForm.name = ''
    manualApiKeyForm.key = ''
    manualApiKeyForm.route_type = defaultApiKeyRouteType(fullSite)
    manualApiKeyForm.route_path = defaultApiKeyRoutePath(manualApiKeyForm.route_type)
    manualApiKeyForm.request_base_urls = ''
    manualApiKeyForm.image_generation_path = ''
    manualApiKeyForm.image_edit_path = ''
    resetApiKeyRequestUrlDrafts(siteApiKeyEntries(fullSite))
    apiKeyDialogOpen.value = true
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'API Key 配置加载失败')
  } finally {
    apiKeyDialogSaving.value = false
  }
}

async function copyApiKeyFromDialog(value: string) {
  const normalized = value.trim()
  if (!normalized) {
    toast.error('当前 API Key 为空。')
    return
  }
  try {
    await navigator.clipboard.writeText(normalized)
    toast.success('API Key 已复制。')
  } catch {
    toast.error('复制失败，请手动复制。')
  }
}

async function copyPrimaryApiKeyFromDialog() {
  const primary = apiKeyDialogEntries.value.find((item) => item.isPrimary) ?? apiKeyDialogEntries.value[0]
  const value = primary?.key ?? ''
  if (!value) {
    toast.error('当前站点未配置 API Key。')
    return
  }
  await copyApiKeyFromDialog(value)
}

function apiKeyRouteTypeLabel(value: string) {
  return apiKeyRouteTypeOptions.find((option) => option.value === value)?.label ?? '默认类型'
}

function apiKeyRoutePathLabel(value: string) {
  return apiKeyRoutePathOptions.find((option) => option.value === value)?.label ?? '跟随客户端'
}

function apiKeySourceLabel(entry: SiteApiKeyEntry) {
  if (entry.isManual) {
    return '自定义'
  }
  return entry.source ? '接口' : '默认'
}

function upsertApiKeyDialogSiteCredentials(updater: (site: Site, credentials: Record<string, unknown>) => Record<string, unknown>) {
  const site = apiKeyDialogSite.value
  if (!site) {
    return
  }
  const credentials = updater(site, { ...(site.credentials as Record<string, unknown>) })
  site.credentials = credentials as Site['credentials']
  const index = sites.value.findIndex((item) => item.id === site.id)
  if (index >= 0) {
    sites.value[index] = {
      ...sites.value[index],
      credentials: credentials as Site['credentials'],
    }
  }
}

function addManualApiKey() {
  const key = manualApiKeyForm.key.trim()
  if (!key) {
    toast.error('请先填写自定义 API Key。')
    return
  }
  const site = apiKeyDialogSite.value
  if (!site) {
    return
  }
  const routeType = normalizeApiKeyRouteType(manualApiKeyForm.route_type) || defaultApiKeyRouteType(site)
  const routePath = apiKeyRoutePath({ route_path: manualApiKeyForm.route_path })
  const name = manualApiKeyForm.name.trim() || `自定义 Key ${manualApiKeyEntries.value.length + 1}`
  const entry = {
    id: `manual-${Date.now()}`,
    name,
    key,
    status: 'active',
    source: 'manual',
    route_type: routeType,
    api_type: routeType,
    request_base_urls: normalizeStringList(manualApiKeyForm.request_base_urls),
    image_generation_path: manualApiKeyForm.image_generation_path.trim(),
    image_edit_path: manualApiKeyForm.image_edit_path.trim(),
  }
  if (routePath) {
    Object.assign(entry, { route_path: routePath })
  }
  upsertApiKeyDialogSiteCredentials((currentSite, credentials) => {
    const entries = storedApiKeyEntriesForEdit({ ...currentSite, credentials })
    if (equivalentApiKeyEntryExists(entries, entry)) {
      toast.info('已存在相同 API Key 配置。')
      return credentials
    }
    const next = mergeApiKeyEntries([...entries, entry])
    return {
      ...credentials,
      api_keys: next,
      api_key: String(credentials.api_key ?? '').trim() || key,
    }
  })
  manualApiKeyForm.name = ''
  manualApiKeyForm.key = ''
  manualApiKeyForm.route_type = defaultApiKeyRouteType(site)
  manualApiKeyForm.route_path = defaultApiKeyRoutePath(manualApiKeyForm.route_type)
  manualApiKeyForm.request_base_urls = ''
  manualApiKeyForm.image_generation_path = ''
  manualApiKeyForm.image_edit_path = ''
  resetApiKeyRequestUrlDrafts(siteApiKeyEntries(site))
}

function removeApiKey(entry: SiteApiKeyEntry) {
  upsertApiKeyDialogSiteCredentials((_site, credentials) => removeSiteApiKeyCredential(credentials, entry.key, entry.entryIndex))
  const key = apiKeyDraftKey(entry)
  delete apiKeyRequestUrlDrafts[key]
  delete apiKeyRoutePathDrafts[key]
  delete apiKeyImageGenerationPathDrafts[key]
  delete apiKeyImageEditPathDrafts[key]
  toast.success('API Key 已从本地配置移除，保存后生效。')
}

async function saveApiKeyDialog() {
  const site = apiKeyDialogSite.value
  if (!site) {
    return
  }
  apiKeyDialogSaving.value = true
  try {
    const payload: SitePayload = {
      name: site.name,
      base_url: site.base_url,
      plugin_key: site.plugin_key,
      group_name: site.group_name,
      supported_models: normalizeSupportedModels(site.supported_models),
      is_enabled: site.is_enabled,
      notes: site.notes,
      credentials: JSON.parse(JSON.stringify(site.credentials)),
      plugin_config: JSON.parse(JSON.stringify(site.plugin_config)),
    }
    payload.plugin_config.api_request_urls = normalizeStringList(apiKeyDialogForm.request_api_urls).join('\n')
    payload.plugin_config.image_generation_path = apiKeyDialogForm.image_generation_path.trim()
    payload.plugin_config.image_edit_path = apiKeyDialogForm.image_edit_path.trim()
    const credentials = payload.credentials as Record<string, unknown>
    credentials.api_keys = mergeApiKeyEntries(storedApiKeyEntriesForEdit({
      credentials,
      plugin_config: payload.plugin_config,
    }))
    await updateSite(site.id, payload)
    apiKeyDialogOpen.value = false
    toast.success(`${site.name} API Key 配置已保存。`)
    await syncRoutesAfterApiKeyUpdate(1)
    await loadData(site.id)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    apiKeyDialogSaving.value = false
  }
}

async function refreshTableSummaries() {
  if (!sites.value.length) {
    return
  }
  try {
    const summaries = await refreshSiteSummaries()
    summaries.forEach(applySiteSummary)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '站点摘要刷新失败')
  }
}

const { schedule: scheduleSummaryRefresh } = useDebouncedTask(refreshTableSummaries)

async function handleConnectivitySweep() {
  const targets = sites.value.filter((site) => site.is_enabled)
  if (!targets.length) {
    toast.error('当前没有启用站点可测试。')
    return
  }

  busy.value = true
  connectivitySweepProgress.value = { total: targets.length, done: 0, success: 0, failed: 0 }
  try {
    await runSiteBatch(targets, async (site) => {
      try {
        const summaries = await refreshSiteSummaries({ site_ids: [site.id] })
        const summary = summaries[0]
        if (summary) {
          applySiteSummary(summary)
          if (summary.connection_status === 'success') {
            connectivitySweepProgress.value!.success += 1
          } else {
            connectivitySweepProgress.value!.failed += 1
          }
        } else {
          connectivitySweepProgress.value!.failed += 1
        }
      } catch (err) {
        connectivitySweepProgress.value!.failed += 1
        const target = sites.value.find((item) => item.id === site.id)
        if (target) {
          target.last_status = 'failed'
          target.connection_status = 'failed'
          target.last_message = err instanceof Error ? err.message : '连通测试失败'
          target.last_run_at = new Date().toISOString()
        }
      } finally {
        connectivitySweepProgress.value!.done += 1
      }
    })
    toast.success(
      `连通测试完成：成功 ${connectivitySweepProgress.value!.success}，失败 ${connectivitySweepProgress.value!.failed}。`,
    )
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '连通测试失败')
  } finally {
    connectivitySweepProgress.value = null
    busy.value = false
  }
}

function isBalanceProbing(siteId: number) {
  return balanceProbeIds.value.includes(siteId)
}

async function handleProbeSiteBalance(site: Site) {
  balanceProbeIds.value = [...balanceProbeIds.value, site.id]
  try {
    const result = await probeSiteBalance(site.id)
    applyBalanceProbeResult(result)
    if (result.ok) {
      toast.success(`${site.name} 余额读取成功：${formatBalance(result.remaining, result.unit)}（${result.base_url}）`)
    } else {
      toast.error(`${site.name} 余额读取失败：${result.message}`)
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '余额读取失败')
  } finally {
    balanceProbeIds.value = balanceProbeIds.value.filter((item) => item !== site.id)
  }
}

async function handleRefresh(preferredId: number | null = selectedId.value) {
  await loadData(preferredId)
  await refreshTableSummaries()
}

function openCCSwitchFilePicker() {
  ccSwitchFileInput.value?.click()
}

function resetCCSwitchSqlPreview() {
  ccSwitchResolvedPayload.value = null
  ccSwitchResolveError.value = ''
}

function openCCSwitchConfig(tab: 'import' | 'export' = 'import') {
  ccSwitchConfigTab.value = tab
  ccSwitchConfigOpen.value = true
  if (tab === 'export' && !ccSwitchExportText.value.trim() && !ccSwitchExportLoading.value) {
    void handleCCSwitchExport()
  }
}

async function handleCCSwitchFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) {
    return
  }
  ccSwitchImportMode.value = file.name.toLowerCase().endsWith('.sql') ? 'sql' : 'json'
  ccSwitchImportText.value = await file.text()
  openCCSwitchConfig('import')
  input.value = ''
  if (ccSwitchImportMode.value === 'sql') {
    await resolveCCSwitchSqlPreview()
  }
}

async function resolveCCSwitchSqlPreview() {
  const sqlText = ccSwitchImportText.value.trim()
  if (!sqlText) {
    resetCCSwitchSqlPreview()
    toast.error('请先提供 SQL 内容。')
    return false
  }

  ccSwitchSqlPreviewLoading.value = true
  ccSwitchResolveError.value = ''
  try {
    const result = await convertCCSwitchSql(sqlText)
    ccSwitchResolvedPayload.value = result.payload
    toast.success(`SQL 解析完成：识别 ${result.provider_count} 条供应商。`)
    return true
  } catch (err) {
    resetCCSwitchSqlPreview()
    ccSwitchResolveError.value = err instanceof Error ? err.message : 'SQL 解析失败'
    toast.error(ccSwitchResolveError.value)
    return false
  } finally {
    ccSwitchSqlPreviewLoading.value = false
  }
}

async function submitCCSwitchImport() {
  let payload: Record<string, unknown> | null = null
  if (ccSwitchImportMode.value === 'json') {
    payload = ccSwitchPreviewPayload.value
    if (!payload) {
      toast.error('导入内容不是有效 JSON。')
      return
    }
  } else {
    const resolved = ccSwitchPreviewPayload.value || (await resolveCCSwitchSqlPreview() ? ccSwitchResolvedPayload.value : null)
    if (!resolved) {
      return
    }
  }
  if (!ccSwitchSelectedSections.value.length) {
    toast.error('请至少选择一个供应商分类。')
    return
  }

  ccSwitchImportLoading.value = true
  try {
    const result = ccSwitchImportMode.value === 'sql'
      ? await importCCSwitchSql(ccSwitchImportText.value, {
        sectionKeys: ccSwitchSelectedSections.value,
      })
      : await importCCSwitchConfig(payload as Record<string, unknown>, {
        sectionKeys: ccSwitchSelectedSections.value,
      })
    ccSwitchConfigOpen.value = false
    await loadData(result.imported_site_ids[0] ?? selectedId.value)
    scheduleSummaryRefresh()
    toast.success(`导入完成：新增 ${result.created}，更新 ${result.updated}，删除 ${result.deleted}，跳过 ${result.skipped}。`)
    if (result.messages.length) {
      testFeedback.value = {
        type: 'success',
        title: '供应商导入结果',
        message: result.messages.join('\n'),
      }
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '供应商导入失败')
  } finally {
    ccSwitchImportLoading.value = false
  }
}

function buildExportFilename() {
  const now = new Date()
  const pad = (value: number) => String(value).padStart(2, '0')
  const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`
  return `cc-switch-export-${stamp}.web.json`
}

function downloadCCSwitchExport() {
  if (!ccSwitchExportText.value.trim()) {
    return
  }
  const blob = new Blob([ccSwitchExportText.value], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = buildExportFilename()
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

async function handleCCSwitchExport() {
  ccSwitchExportLoading.value = true
  try {
    const result = await exportCCSwitchConfig()
    ccSwitchExportText.value = JSON.stringify(result.payload, null, 2)
    toast.success(`已生成 ${result.site_count} 条供应商配置。`)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '供应商导出失败')
  } finally {
    ccSwitchExportLoading.value = false
  }
}

async function handleDuplicateCheck() {
  duplicateCheckLoading.value = true
  duplicateCheckOpen.value = true
  duplicateChecked.value = true
  try {
    duplicateGroups.value = await getDuplicateSites()
    toast.success(`检测完成：发现 ${duplicateGroups.value.length} 组重复站点。`)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '清理检测失败')
  } finally {
    duplicateCheckLoading.value = false
  }
}

async function handleSuggestedDuplicateMerge() {
  if (duplicateMergeLoading.value) {
    return
  }

  let groups = duplicateGroups.value
  if (!groups.length) {
    duplicateCheckLoading.value = true
    try {
      groups = await getDuplicateSites()
      duplicateGroups.value = groups
      duplicateChecked.value = true
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '清理检测失败')
      duplicateCheckLoading.value = false
      return
    }
    duplicateCheckLoading.value = false
  }

  if (!groups.length) {
    duplicateCheckOpen.value = true
    toast.success('未发现需要合并的重复站点。')
    return
  }

  const duplicateSiteCount = groups.reduce((total, group) => total + Math.max(group.site_ids.length - 1, 0), 0)

  Modal.confirm({
    title: '按建议合并重复站点',
    content: `将合并 ${groups.length} 组重复站点，删除 ${duplicateSiteCount} 条重复记录，并保留每组建议站点。`,
    okText: '开始合并',
    cancelText: '取消',
    async onOk() {
      duplicateMergeLoading.value = true
      try {
        const result = await mergeDuplicateSites()
        await loadData(selectedId.value)
        duplicateGroups.value = await getDuplicateSites()
        duplicateChecked.value = true
        duplicateCheckOpen.value = true
        toast.success(
          result.merged_group_count
            ? `已合并 ${result.merged_group_count} 组，删除 ${result.deleted_site_count} 条重复记录。`
            : '未发现需要合并的重复站点。',
        )
      } catch (err) {
        toast.error(err instanceof Error ? err.message : '按建议合并失败')
        throw err
      } finally {
        duplicateMergeLoading.value = false
      }
    },
  })
}

function applyRecommendedPlugin() {
  if (recommendedPluginKey.value) {
    editor.plugin_key = recommendedPluginKey.value
    applyPluginConfigDefaults()
    mismatchAcknowledged.value = false
    toast.info(`已切换为推荐插件：${recommendedPlugin.value?.name ?? recommendedPluginKey.value}`)
  }
}

function buildExternalUrl(baseUrl: string, path = ''): string | null {
  const normalized = baseUrl.trim()
  if (!normalized) {
    return null
  }
  try {
    const target = new URL(normalized)
    if (!path.trim()) {
      return target.toString()
    }
    return new URL(path, target.toString()).toString()
  } catch {
    return null
  }
}

async function openExternalUrl(url: string | null, fallbackLabel: string) {
  if (!url) {
    toast.error('请先填写有效的基础 URL。')
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
  toast.info(`已打开${fallbackLabel}。`)
}

function isRelayOnlySitePayload(payload: { plugin_key: string }) {
  return Boolean(pluginForKey(payload.plugin_key)?.capabilities.includes('relay_only'))
}

function handleOpenOfficialSite() {
  openExternalUrl(officialSiteUrl.value, '官网')
}

function handleOpenSiteInNewTab(site: Pick<Site, 'base_url' | 'name'>) {
  openExternalUrl(buildExternalUrl(site.base_url), site.name || '站点')
}

function handleOpenAuthSite() {
  openExternalUrl(authEntryUrl.value, currentPlugin.value?.auth_entry_label || '授权站点')
}

function editableCredentialKeys(pluginKey = editor.plugin_key) {
  const plugin = pluginForKey(pluginKey) ?? currentPlugin.value
  const keys = new Set((plugin?.credential_fields ?? []).map((field) => field.name))
  if (keys.size === 0) {
    for (const key of ['account', 'email', 'username', 'user_id', 'access_token', 'refresh_token', 'api_key', 'cookie', 'user_agent']) {
      keys.add(key)
    }
  }
  return keys
}

function credentialAutocomplete(fieldName: string, fieldType: string) {
  if (fieldType === 'password') {
    return 'new-password'
  }
  const normalized = fieldName.toLowerCase()
  if (normalized === 'email' || normalized === 'username' || normalized === 'account' || normalized.endsWith('_account')) {
    return 'off'
  }
  if (normalized.includes('token') || normalized.includes('key') || normalized.includes('cookie')) {
    return 'off'
  }
  return 'off'
}

function credentialInputName(fieldName: string) {
  return `site-credential-${fieldName}`
}

function buildCredentialSuggestions(suggested: Record<string, string>) {
  const entries = new Map<string, string>()
  const put = (key: string, value: unknown) => {
    const text = String(value ?? '').trim()
    if (key && text && !entries.has(key)) {
      entries.set(key, text)
    }
  }

  for (const [key, value] of Object.entries(suggested)) {
    put(key, value)
  }
  const account = suggested.account || suggested.email || suggested.username || suggested.user_id
  put('account', account)
  put('email', suggested.email || suggested.username || suggested.account)
  put('username', suggested.username || suggested.email || suggested.account)
  put('user_id', suggested.user_id)
  put('access_token', suggested.access_token || suggested.auth_token || suggested.token)
  put('refresh_token', suggested.refresh_token)
  put('cookie', suggested.cookie)
  put('user_agent', suggested.user_agent)
  return entries
}

function summarizeStorageKeys(payload: LocalStorageAnalyzeResult) {
  const localKeys = Object.keys(payload.local_storage)
  const sessionKeys = Object.keys(payload.session_storage)
  const localPreview = localKeys.length ? localKeys.slice(0, 8).join('，') : '无'
  return [
    `页面：${payload.page_title || payload.page_url || '未知页面'}`,
    `已解析：${payload.parsed_items} 项`,
    `Cookie：${payload.cookie_header ? '已包含可读 Cookie' : '无可读 Cookie'}`,
    `localStorage Key：${localPreview}${localKeys.length > 8 ? ' ...' : ''}`,
    `sessionStorage Key：${sessionKeys.length ? sessionKeys.slice(0, 8).join('，') : '无'}${sessionKeys.length > 8 ? ' ...' : ''}`,
  ]
}

const consoleCollectorScript = `(() => {
  const pick = (storage) => Object.fromEntries(
    Array.from({ length: storage.length }, (_, index) => {
      const key = storage.key(index) || ''
      return [key, storage.getItem(key) || '']
    }),
  )
  const decodeJWT = (token) => {
    try {
      const raw = String(token || '').replace(/^Bearer\\s+/i, '').split('.')[1]
      if (!raw) return null
      return JSON.parse(decodeURIComponent(escape(atob(raw.replace(/-/g, '+').replace(/_/g, '/')))))
    } catch {
      return null
    }
  }
  const sanitizeConfig = (value) => {
    if (typeof value === 'string') {
      if (/^data:image\\//i.test(value)) return value.slice(0, 96) + '...[omitted]'
      if (value.length > 2000) return value.slice(0, 2000) + '...[truncated]'
      return value
    }
    if (Array.isArray(value)) return value.map(sanitizeConfig)
    if (value && typeof value === 'object') {
      return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, sanitizeConfig(item)]))
    }
    return value
  }
  const local = pick(window.localStorage)
  const session = pick(window.sessionStorage)
  const tokenPayloads = {}
  Object.entries({ ...local, ...session }).forEach(([key, value]) => {
    if (/token|jwt/i.test(key)) {
      const decoded = decodeJWT(value)
      if (decoded) tokenPayloads[key] = decoded
    }
  })
  const appConfig = sanitizeConfig(window.__APP_CONFIG__ || window.APP_CONFIG || window.appConfig || null)
  const textForGuess = [location.href, document.title, JSON.stringify(appConfig || {}), Object.keys(local).join(' ')].join(' ').toLowerCase()
  const pluginKey = textForGuess.includes('sub2api') || textForGuess.includes('耀闪') || local.auth_token
    ? 'sub2api-platform'
    : (textForGuess.includes('newapi') || textForGuess.includes('oneapi') ? 'yellowpeach-newapi' : '')
  const payload = {
    url: location.href,
    title: document.title || '',
    pluginKey,
    appConfig,
    tokenPayloads,
    userAgent: navigator.userAgent,
    cookie: document.cookie || '',
    localStorage: local,
    sessionStorage: session,
    capturedAt: new Date().toISOString(),
  }
  const text = JSON.stringify(payload, null, 2)
  try {
    if (typeof copy === 'function') {
      copy(text)
    } else if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text)
    }
  } catch {}
  console.log(text)
  return text
})()`

async function handleCopyConsoleScript() {
  try {
    await navigator.clipboard.writeText(consoleCollectorScript)
    toast.success('控制台脚本已复制。')
  } catch {
    toast.error('复制失败，请手动复制脚本内容。')
  }
}

async function handleAnalyzeLocalStorage() {
  if (localStorageAnalyzeLoading.value) {
    return
  }
  const rawText = localStorageRawText.value.trim()
  if (!rawText) {
    toast.error('请先粘贴 localStorage 内容。')
    return
  }

  localStorageAnalyzeLoading.value = true
  try {
    const result = await analyzeLocalStorage(rawText)
    lastAutoAnalyzedStorageRaw.value = rawText
    if (result.suggested_plugin_key && pluginForKey(result.suggested_plugin_key)) {
      editor.plugin_key = result.suggested_plugin_key
      applyPluginConfigDefaults()
      await nextTick()
    }
    if (result.suggested_site_name && !editingId.value) {
      editor.name = result.suggested_site_name
    }
    if (result.suggested_base_url) {
      editor.base_url = result.suggested_base_url
    }
    if (result.suggested_plugin_config) {
      editor.plugin_config = {
        ...editor.plugin_config,
        ...result.suggested_plugin_config,
      }
    }
    const editableKeys = editableCredentialKeys(result.suggested_plugin_key || editor.plugin_key)
    const suggestions = buildCredentialSuggestions(result.suggested_credentials)
    const appliedEntries = [...suggestions.entries()].filter(([key]) => editableKeys.has(key))

    for (const [key, value] of appliedEntries) {
      editor.credentials[key] = value
    }

    if (appliedEntries.length) {
      saveFeedback.value = '已从 localStorage 分析结果回填凭证，请确认内容后保存站点。'
    }

    const appliedLabels = appliedEntries.length
      ? appliedEntries.map(([key]) => key).join('、')
      : '无可直接写入当前插件字段的凭证'
    const matchedPreview = result.matched_keys.length ? result.matched_keys.slice(0, 8).join('，') : '无'
    testFeedback.value = {
      type: appliedEntries.length ? 'success' : 'error',
      title: appliedEntries.length ? 'localStorage 分析完成' : 'localStorage 已解析，但未自动回填',
      message: [
        result.message,
        ...summarizeStorageKeys(result),
        `回填字段：${appliedLabels}`,
        `命中线索：${matchedPreview}${result.matched_keys.length > 8 ? ' ...' : ''}`,
      ].join('\n'),
    }
    toast.success(appliedEntries.length ? `已回填 ${appliedEntries.length} 个字段。` : '分析完成，请查看结果。')
  } catch (err) {
    const message = err instanceof Error ? err.message : 'localStorage 分析失败'
    testFeedback.value = {
      type: 'error',
      title: 'localStorage 分析失败',
      message,
    }
    toast.error(message)
  } finally {
    localStorageAnalyzeLoading.value = false
  }
}

function handleStoragePayloadPaste() {
  scheduleManagedTimeout(() => {
    if (localStorageRawText.value.trim()) {
      void handleAnalyzeLocalStorage()
    }
  }, 0)
}

async function ensureStorageAnalysisFinished() {
  const rawText = localStorageRawText.value.trim()
  if (storageAnalyzeTimer.value) {
    clearManagedTimeout(storageAnalyzeTimer.value)
    storageAnalyzeTimer.value = null
  }
  if (drawerOpen.value && rawText && rawText !== lastAutoAnalyzedStorageRaw.value && isStorageJsonCandidate(rawText)) {
    await handleAnalyzeLocalStorage()
  }
  while (localStorageAnalyzeLoading.value) {
    await waitStorageDelay(50)
    if (!mounted) {
      return
    }
  }
}

function editorPayload(): SitePayload {
  return {
    ...JSON.parse(JSON.stringify(editor)),
    supported_models: null,
  }
}

function upsertSite(saved: Site, options: { edit?: boolean } = {}) {
  saved = normalizeSite(saved)
  const index = sites.value.findIndex((site) => site.id === saved.id)
  if (index >= 0) {
    sites.value[index] = saved
  } else {
    sites.value = [saved, ...sites.value]
  }
  selectedId.value = saved.id
  editingId.value = options.edit === false ? null : saved.id
}

async function persistEditor(options: { keepDrawerOpen?: boolean; showToast?: boolean } = {}) {
  if (pluginMismatch.value && !mismatchAcknowledged.value) {
    mismatchAcknowledged.value = true
    toast.error(
      `检测到当前站点更适合使用“${recommendedPlugin.value?.name ?? recommendedPluginKey.value}”。如仍要继续当前插件，请再次点击保存。`,
    )
    return null
  }

  const activeEditingId = editingId.value
  const payload = editorPayload()
  const isUpdate = activeEditingId !== null
  const saved = activeEditingId !== null
    ? await updateSite(activeEditingId, payload)
    : await createSite(payload)
  upsertSite(saved)
  assignEditor(saved)
  if (options.keepDrawerOpen !== undefined) {
    drawerOpen.value = options.keepDrawerOpen
  } else {
    drawerOpen.value = isUpdate
  }
  saveFeedback.value = isUpdate ? '更改已保存，可继续编辑当前站点。' : null
  lastSavedEditorSnapshot.value = JSON.stringify(editor)
  if (options.showToast !== false) {
    toast.success(isUpdate ? '站点信息已更新。' : '站点已创建。')
  }
  return saved
}

async function saveSite() {
  busy.value = true
  try {
    if (!editingId.value && batchRegisterEnabled.value) {
      await saveBatchRegisteredSites()
      return
    }
    await ensureStorageAnalysisFinished()
    const saved = await persistEditor()
    if (saved) {
      await syncRoutesAfterSiteChange()
      await reloadDataWithCheckinExtras(saved.id)
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    busy.value = false
  }
}

async function saveBatchRegisteredSites() {
  if (!canBatchRegisterEditor.value) {
    toast.error('当前插件不支持批量注册账号。')
    return
  }
  const emailPattern = batchRegisterForm.email_pattern.trim()
  if (!isValidEmailPattern(emailPattern)) {
    toast.error('邮箱规则必须包含 {n}、{n:03} 或 {rand:[字符集]{位数}}。')
    return
  }
  if (!batchRegisterForm.password.trim()) {
    toast.error('请填写注册密码。')
    return
  }
  await ensureStorageAnalysisFinished()
  const payload = {
    ...editorPayload(),
    email_pattern: emailPattern,
    password: batchRegisterForm.password,
    count: Number(batchRegisterForm.count) || 1,
    start_index: Number(batchRegisterForm.start_index) || 1,
  }
  const result = await createRegistrationBatchSites(payload)
  batchRegisterResult.value = result
  const firstCreated = result.items.find((item) => item.ok && item.site)?.site
  if (firstCreated) {
    upsertSite(firstCreated, { edit: false })
  }
  await syncRoutesAfterSiteChange()
  await reloadDataWithCheckinExtras(firstCreated?.id ?? selectedId.value)
  drawerOpen.value = true
  const failedText = result.failed_count ? `，失败 ${result.failed_count}` : ''
  toast.success(`批量注册完成：创建 ${result.created_count}${failedText}。`)
}

function isValidEmailPattern(value: string) {
  return /\{n(?::0?\d+)?\}/.test(value) || /\{rand:\[[^\]]+\]\{\d+\}\}/.test(value)
}

function applySiteHealthResultToEditor(result: Awaited<ReturnType<typeof testSite>>) {
  if (result.updated_credentials) {
    editor.credentials = {
      ...editor.credentials,
      ...result.updated_credentials,
    }
  }
  if (result.updated_plugin_config) {
    editor.plugin_config = {
      ...editor.plugin_config,
      ...result.updated_plugin_config,
    }
  }
  if (result.package_display) {
    editor.plugin_config = {
      ...editor.plugin_config,
      package_display: result.package_display,
    }
  }
  if (result.invite_link) {
    editor.plugin_config = {
      ...editor.plugin_config,
      invite_link: result.invite_link,
    }
  }
  if (result.invite_code) {
    editor.plugin_config = {
      ...editor.plugin_config,
      invite_code: result.invite_code,
    }
  }
}

async function handleTest(targetSite = selectedSite.value) {
  if (!targetSite) {
    return
  }
  busy.value = true
  let activeSite = targetSite
  const drawerTest = drawerOpen.value && editingId.value === targetSite.id
  try {
    if (drawerTest) {
      await ensureStorageAnalysisFinished()
      const saved = await persistEditor({ keepDrawerOpen: true, showToast: false })
      if (!saved) {
        return
      }
      activeSite = saved
      saveFeedback.value = '测试前已保存当前表单。'
    }

    const relayOnlyTarget =
      isRelayOnlySitePayload(activeSite) || (drawerTest && isRelayOnlyEditor.value)
    if (relayOnlyTarget) {
      const result = await probeSiteBalance(activeSite.id)
      applyBalanceProbeResult(result)
      const balanceText = formatBalance(result.remaining, result.unit)
      testFeedback.value = {
        type: result.ok ? 'success' : 'error',
        title: result.ok ? '模型出口验证成功' : '模型出口验证失败',
        message: `${result.message}${balanceText ? `\n当前余额：${balanceText}` : ''}${result.base_url ? `\n当前出口：${result.base_url}` : ''}${result.latency_ms !== null ? `\n延迟：${Math.round(result.latency_ms)} ms` : ''}`,
      }
      if (!result.ok) {
        throw new Error(result.message)
      }
      toast.success(`${activeSite.name} 模型出口验证成功：${balanceText || result.base_url}`)
      await reloadDataWithCheckinExtras(activeSite.id, { preserveEditor: drawerTest })
      return
    }

    if (drawerTest) {
      const result = await testSite(activeSite.id)
      applySiteHealthResultToEditor(result)
      const balanceText = formatBalance(result.balance, result.balance_unit)
      const packageText = result.package_display ? `\n当前套餐：${result.package_display}` : ''
      testFeedback.value = {
        type: result.logged_in ? 'success' : 'error',
        title: result.logged_in ? '站内授权测试成功' : '站内授权测试失败',
        message: `${result.message}${balanceText ? `\n当前余额：${balanceText}` : ''}${packageText}${result.account_name ? `\n当前账号：${result.account_name}` : ''}`,
      }
      const finalSaved = await updateSite(activeSite.id, editorPayload())
      upsertSite(finalSaved)
      assignEditor(finalSaved)
      lastSavedEditorSnapshot.value = JSON.stringify(editor)
      saveFeedback.value = result.logged_in ? '测试通过，最新站点信息已自动保存。' : '测试完成，当前表单和回填信息已保存。'
      await reloadDataWithCheckinExtras(finalSaved.id, { preserveEditor: true })
      if (!result.logged_in) {
        toast.error(result.message)
        return
      }
      toast.success(`${result.message}${balanceText ? ` 当前余额 ${balanceText}` : ''}`)
      return
    }

    const result = await testSite(activeSite.id)
    const balanceText = formatBalance(result.balance, result.balance_unit)
    const packageText = result.package_display ? `\n当前套餐：${result.package_display}` : ''
    testFeedback.value = {
      type: result.logged_in ? 'success' : 'error',
      title: result.logged_in ? '站内授权测试成功' : '站内授权测试失败',
      message: `${result.message}${balanceText ? `\n当前余额：${balanceText}` : ''}${packageText}${result.account_name ? `\n当前账号：${result.account_name}` : ''}`,
    }
    if (!result.logged_in) {
      throw new Error(result.message)
    }
    toast.success(`${result.message}${balanceText ? ` 当前余额 ${balanceText}` : ''}`)
    await reloadDataWithCheckinExtras(activeSite.id)
  } catch (err) {
    const message = err instanceof Error ? err.message : '测试失败'
    testFeedback.value = {
      type: 'error',
      title:
        isRelayOnlySitePayload(activeSite) || (drawerTest && isRelayOnlyEditor.value)
          ? '模型出口验证失败'
          : '站内授权测试失败',
      message,
    }
    toast.error(message)
    if (drawerTest) {
      await reloadDataWithCheckinExtras(activeSite.id, { preserveEditor: true })
    } else if (!isRelayOnlySitePayload(activeSite)) {
      await reloadDataWithCheckinExtras(activeSite.id)
    }
  } finally {
    busy.value = false
  }
}

async function handleCheckin(targetSite = selectedSite.value) {
  if (!targetSite) {
    return
  }
  busy.value = true
  try {
    const result = await runSiteCheckin(targetSite.id)
    const balanceText = formatBalance(result.balance, result.balance_unit)
    toast.success(`${result.message}${balanceText ? ` 当前余额 ${balanceText}` : ''}`)
    await reloadDataWithCheckinExtras(targetSite.id)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '执行失败')
    await reloadDataWithCheckinExtras(targetSite.id)
  } finally {
    busy.value = false
  }
}

async function handleToggle(site: Site) {
  busy.value = true
  try {
    await toggleSite(site.id)
    await syncRoutesAfterSiteChange()
    await loadData(selectedSite.value?.id ?? site.id)
    toast.success(site.is_enabled ? '站点已停用。' : '站点已启用。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '切换失败')
  } finally {
    busy.value = false
  }
}

async function handleDelete(targetSite = selectedSite.value) {
  if (!targetSite) {
    return
  }
  const confirmed = window.confirm(`确认删除站点“${targetSite.name}”吗？此操作不可恢复。`)
  if (!confirmed) {
    return
  }

  busy.value = true
  try {
    await deleteSite(targetSite.id)
    await syncRoutesAfterSiteChange()
    drawerOpen.value = false
    editingId.value = null
    await loadData(null)
    toast.success('站点已删除。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '删除失败')
  } finally {
    busy.value = false
  }
}

async function handlePreviewTotp() {
  if (!editingId.value) {
    return
  }
  totpPreviewLoading.value = true
  try {
    totpPreview.value = await previewSiteTotp(editingId.value)
    totpPreviewOpen.value = true
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '验证码生成失败')
  } finally {
    totpPreviewLoading.value = false
  }
}

function siteRowClassName(record: Site) {
  return record.id === selectedId.value ? 'management-row management-row--active' : 'management-row'
}

function siteCustomRow(record: Site) {
  return {
    onClick: () => selectSite(record),
  }
}

watch(
  () => ccSwitchSectionOptions.value.map((item) => item.value).join('|'),
  () => {
    const available = new Set(ccSwitchSectionOptions.value.map((item) => item.value))
    const filtered = ccSwitchSelectedSections.value.filter((item) => available.has(item))
    ccSwitchSelectedSections.value = filtered.length ? filtered : [...available]
  },
  { immediate: true },
)

watch(
  () => [ccSwitchImportMode.value, ccSwitchImportText.value],
  ([mode]) => {
    if (mode === 'sql') {
      resetCCSwitchSqlPreview()
      return
    }
    ccSwitchResolveError.value = ''
  },
)

watch(
  () => [editor.base_url, editor.plugin_key],
  () => {
    applyPluginConfigDefaults()
    mismatchAcknowledged.value = false
    if (!canBatchRegisterEditor.value) {
      batchRegisterEnabled.value = false
    }
  },
)

watch(
  () => editor.group_name,
  (value) => {
    const nextGroupNames = parseGroupNames(value)
    if (JSON.stringify(nextGroupNames) !== JSON.stringify(editorGroupNames.value)) {
      editorGroupNames.value = nextGroupNames
    }
  },
  { immediate: true },
)

watch(
  editorGroupNames,
  (value) => {
    const normalized = normalizeGroupNames(value)
    if (normalized !== editor.group_name) {
      editor.group_name = normalized
    }
  },
  { deep: true },
)

watch(
  () => JSON.stringify(editor),
  (value) => {
    if (saveFeedback.value && value !== lastSavedEditorSnapshot.value) {
      saveFeedback.value = null
    }
  },
)

watch(
  () => currentPlugin.value,
  (plugin) => {
    if (!plugin) return
    plugin.credential_fields.forEach((field) => ensureField(editor.credentials, field.name, field.type))
    plugin.config_fields.forEach((field) => ensureField(editor.plugin_config, field.name, field.type))
  },
  { immediate: true },
)

watch(
  localStorageRawText,
  (value) => {
    if (storageAnalyzeTimer.value) {
      clearManagedTimeout(storageAnalyzeTimer.value)
      storageAnalyzeTimer.value = null
    }
    const rawText = value.trim()
    if (!drawerOpen.value || !rawText || rawText === lastAutoAnalyzedStorageRaw.value || !isStorageJsonCandidate(rawText)) {
      return
    }
    storageAnalyzeTimer.value = scheduleManagedTimeout(() => {
      storageAnalyzeTimer.value = null
      const latest = localStorageRawText.value.trim()
      if (latest && latest !== lastAutoAnalyzedStorageRaw.value && !localStorageAnalyzeLoading.value) {
        void handleAnalyzeLocalStorage()
      }
    }, 350)
  },
)

async function handleSiteGroupsChanged() {
  await loadData(editingId.value ?? selectedId.value, { preserveEditor: true })
}

onMounted(async () => {
  mounted = true
  window.addEventListener('site-groups:changed', handleSiteGroupsChanged)
  await loadData(null)
  await loadCheckinExtras()
  scheduleSummaryRefresh()
})

onBeforeUnmount(() => {
  mounted = false
  window.removeEventListener('site-groups:changed', handleSiteGroupsChanged)
  if (storageAnalyzeTimer.value) {
    clearManagedTimeout(storageAnalyzeTimer.value)
    storageAnalyzeTimer.value = null
  }
  storageManagedTimers.forEach((timer) => window.clearTimeout(timer))
  storageManagedTimers.clear()
  storageDelayResolvers.forEach((resolve) => resolve())
  storageDelayResolvers.clear()
})
</script>

<template>
  <ShellLayout>
    <div class="sites-page page-stack page-stack--dashboard">
      <div class="sites-toolbar">
        <div class="sites-toolbar__segment">
          <a-button class="sites-toolbar__seg-btn" @click="checkinConfigOpen = true">签到配置</a-button>
          <a-button class="sites-toolbar__seg-btn" @click="checkinLogsOpen = true">最近执行</a-button>
          <a-button
            type="primary"
            class="sites-toolbar__seg-btn sites-toolbar__seg-btn--primary"
            :loading="busy"
            :disabled="!includedCheckinCount"
            @click="handleCheckinAllIncluded"
          >
            {{ checkinAllIncludedLabel }}
          </a-button>
        </div>

        <div class="sites-toolbar__actions">
          <a-button class="sites-toolbar__ghost-btn" :loading="busy" @click="handleRefresh(selectedId)">
            <template #icon>
              <ReloadOutlined />
            </template>
            刷新
          </a-button>
          <a-button class="sites-toolbar__ghost-btn" :loading="busy" @click="handleConnectivitySweep">
            {{ connectivitySweepLabel }}
          </a-button>
          <a-button class="sites-toolbar__ghost-btn" :loading="duplicateCheckLoading" @click="handleDuplicateCheck">
            清理检测
          </a-button>
          <a-button class="sites-toolbar__ghost-btn" :loading="inviteRefreshAllLoading" @click="refreshAllInvites">
            <template #icon>
              <ShareAltOutlined />
            </template>
            {{ inviteRefreshAllLabel }}
          </a-button>
          <a-button
            type="primary"
            class="sites-toolbar__ghost-btn sites-toolbar__ghost-btn--strong"
            :loading="apiKeyRefreshAllLoading"
            @click="refreshAllApiKeys"
          >
            <template #icon>
              <KeyOutlined />
            </template>
            {{ apiKeyRefreshAllLabel }}
          </a-button>
          <a-button class="sites-toolbar__ghost-btn" :loading="ccSwitchExportLoading" @click="openCCSwitchConfig('export')">
            导出供应商
          </a-button>
          <a-button class="sites-toolbar__ghost-btn" @click="openCCSwitchConfig('import')">导入供应商</a-button>
          <a-button type="primary" class="sites-toolbar__create-btn" @click="openCreateDrawer">
            <template #icon>
              <PlusOutlined />
            </template>
            新建站点
          </a-button>
        </div>
      </div>

      <input
        id="cc-switch-import-file"
        ref="ccSwitchFileInput"
        type="file"
        name="cc_switch_import_file"
        accept=".json,.sql,application/json,text/plain"
        style="display: none"
        @change="handleCCSwitchFileChange"
      >

      <a-row :gutter="[16, 16]" class="sites-metric-grid">
        <a-col :xs="24" :sm="12" :xl="8" :xxl="4">
          <a-card :bordered="false" class="admin-card stat-card stat-card--signal sites-stat-card">
            <div class="sites-stat-card__meta">
              <div class="sites-stat-card__eyebrow">站点总数</div>
              <div class="sites-stat-card__value">{{ sites.length }}</div>
              <div class="sites-stat-card__desc">所有已配置站点</div>
            </div>
            <div class="sites-stat-card__icon sites-stat-card__icon--sheet" />
          </a-card>
        </a-col>
        <a-col :xs="24" :sm="12" :xl="8" :xxl="4">
          <a-card :bordered="false" class="admin-card stat-card stat-card--signal sites-stat-card">
            <div class="sites-stat-card__meta">
              <div class="sites-stat-card__eyebrow">已启用</div>
              <div class="sites-stat-card__value">{{ enabledSiteCount }}</div>
              <div class="sites-stat-card__desc">当前启用站点</div>
            </div>
            <div class="sites-stat-card__icon sites-stat-card__icon--stack" />
          </a-card>
        </a-col>
        <a-col :xs="24" :sm="12" :xl="8" :xxl="4">
          <a-card :bordered="false" class="admin-card stat-card stat-card--signal sites-stat-card">
            <div class="sites-stat-card__meta">
              <div class="sites-stat-card__eyebrow">网关就绪</div>
              <div class="sites-stat-card__value">{{ readyGatewayCount }}</div>
              <div class="sites-stat-card__desc">可稳定转发站点</div>
            </div>
            <div class="sites-stat-card__icon sites-stat-card__icon--gateway" />
          </a-card>
        </a-col>
        <a-col :xs="24" :sm="12" :xl="8" :xxl="6">
          <a-card :bordered="false" class="admin-card stat-card stat-card--signal sites-stat-card sites-stat-card--balance">
            <div class="sites-stat-card__meta">
              <div class="sites-stat-card__eyebrow">余额合计</div>
              <div
                class="sites-stat-card__value sites-stat-card__value--balance"
                :class="`sites-stat-card__value--${totalBalanceTone}`"
                :title="totalBalanceSummary"
              >
                {{ totalBalanceSummary }}
              </div>
              <div class="sites-stat-card__desc">{{ quantifiedBalanceSiteCount }} 个站点</div>
            </div>
            <div class="sites-stat-card__icon sites-stat-card__icon--wallet" />
          </a-card>
        </a-col>
        <a-col :xs="24" :sm="12" :xl="8" :xxl="3">
          <a-card :bordered="false" class="admin-card stat-card stat-card--signal sites-stat-card">
            <div class="sites-stat-card__meta">
              <div class="sites-stat-card__eyebrow">状态成功</div>
              <div class="sites-stat-card__value">{{ successSiteCount }}</div>
              <div class="sites-stat-card__desc">连通检测成功</div>
            </div>
            <div class="sites-stat-card__icon sites-stat-card__icon--check" />
          </a-card>
        </a-col>
        <a-col :xs="24" :sm="12" :xl="8" :xxl="3">
          <a-card :bordered="false" class="admin-card stat-card stat-card--signal sites-stat-card">
            <div class="sites-stat-card__meta">
              <div class="sites-stat-card__eyebrow">失败 / 未测</div>
              <div class="sites-stat-card__value">{{ `${failedSiteCount} / ${pendingSiteCount}` }}</div>
              <div class="sites-stat-card__desc">失败 / 未检测站点</div>
            </div>
            <div class="sites-stat-card__icon sites-stat-card__icon--users" />
          </a-card>
        </a-col>
      </a-row>

      <a-card :bordered="false" class="admin-card admin-card--fill site-list-card sites-list-card">
        <template #title>站点列表</template>
        <template #extra>
          <div class="sites-list-toolbar">
            <a-input
              id="sites-list-search"
              v-model:value="siteSearch"
              allow-clear
              class="sites-list-toolbar__search"
              name="sites_list_search"
              placeholder="搜索站点 / 标签 / 分组"
            />
            <span class="sites-list-toolbar__meta">{{ groupedSiteCount }} 个已分组</span>
            <span class="sites-list-toolbar__meta">已选 {{ selectedCheckinIds.length }} 个</span>
            <a-button
              type="primary"
              class="sites-list-toolbar__btn"
              :disabled="!selectedCheckinIds.length || busy"
              @click="handleCheckinSelected"
            >
              {{ checkinBatchProgress ? `签到中 ${checkinBatchProgress.done}/${checkinBatchProgress.total}` : '签到选中' }}
            </a-button>
            <a-button class="sites-list-toolbar__btn" :disabled="!selectedCheckinIds.length" @click="selectedCheckinIds = []">清空选择</a-button>
          </div>
        </template>

        <div class="card-shell sites-list-shell">
          <div :ref="bindPageTableContainer" class="table-fill table-fill--management">
            <a-table
              :columns="siteColumns"
              :data-source="filteredSites"
              :pagination="{ pageSize: tablePageSize }"
              :row-key="rowKey"
              :row-selection="checkinRowSelection"
              size="small"
              :custom-row="siteCustomRow"
              :row-class-name="siteRowClassName"
              :scroll="{ x: 1480, y: pageTableY }"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'site'">
                  <div class="site-table-cell">
                    <div class="site-name-cell">
                      <strong>{{ record.name }}</strong>
                      <a-tooltip title="新标签页打开站点">
                        <a-button
                          type="text"
                          size="small"
                          class="site-name-open-btn"
                          @click.stop="handleOpenSiteInNewTab(asSite(record))"
                        >
                          <template #icon>
                            <ExportOutlined />
                          </template>
                        </a-button>
                      </a-tooltip>
                    </div>
                    <div class="site-subline">
                      <span class="site-subline__label">{{ record.base_url }}</span>
                    </div>
                    <div class="site-subline site-subline--secondary">
                      <span>{{ supportedModelsPreview(asSite(record).supported_models) }}</span>
                      <a-tag class="site-inline-badge" :color="siteApiKeyCountTagColor(asSite(record))">
                        {{ siteApiKeyCountLabel(asSite(record)) }}
                      </a-tag>
                    </div>
                  </div>
                </template>
                <template v-else-if="column.key === 'plugin'">
                  <PluginTag class="site-platform-tag" :plugin-key="record.plugin_key" :label="displayPluginLabel(asSite(record))" />
                </template>
                <template v-else-if="column.key === 'balance'">
                  <div class="site-balance-cell">
                    <span :class="balanceClass(record.last_balance)">
                      {{ record.balance_display || '暂无' }}
                    </span>
                    <span class="site-balance-cell__meta">{{ siteApiKeyCount(asSite(record)) ? `${siteApiKeyCount(asSite(record))} 个 Key` : '无 Key' }}</span>
                  </div>
                </template>
                <template v-else-if="column.key === 'package'">
                  <a-tooltip v-if="record.package_display" :title="record.package_display">
                    <span class="site-package-cell">{{ record.package_display }}</span>
                  </a-tooltip>
                  <span v-else class="site-package-cell site-package-cell--empty">暂无</span>
                </template>
                <template v-else-if="column.key === 'checkin_status'">
                  <StatusPill v-if="visibleCheckinStatus(asSite(record))" :value="visibleCheckinStatus(asSite(record))" />
                  <span v-else class="site-empty-pill">未加入</span>
                </template>
                <template v-else-if="column.key === 'group'">
                  <span class="site-group-text">{{ displayGroupName(asSite(record)) }}</span>
                </template>
                <template v-else-if="column.key === 'status'">
                  <StatusPill :value="record.connection_status" />
                </template>
                <template v-else-if="column.key === 'enabled'">
                  <a-switch
                    :checked="asSite(record).is_enabled"
                    checked-children="开"
                    un-checked-children="关"
                    @click.stop
                    @change="() => handleToggle(asSite(record))"
                  />
                </template>
                <template v-else-if="column.key === 'participation'">
                  <a-switch
                    :checked="siteIncludedInCheckin(asSite(record))"
                    :disabled="!siteCanCheckin(asSite(record))"
                    checked-children="可以"
                    un-checked-children="禁止"
                    @click.stop
                    @change="(checked) => handleParticipationToggle(asSite(record), checked)"
                  />
                </template>
                <template v-else-if="column.key === 'actions'">
                  <div class="site-actions-cell">
                    <a-button size="small" class="site-action-btn site-action-btn--edit" @click.stop="openEditDrawer(asSite(record))">
                      编辑
                    </a-button>
                    <a-dropdown :trigger="['click']">
                      <a-button
                        size="small"
                        class="site-actions-menu-button"
                        :loading="
                          isInviteLoading(asSite(record).id) ||
                          isBalanceProbing(asSite(record).id) ||
                          isApiKeyRefreshing(asSite(record).id)
                        "
                        @click.stop
                      >
                        <template #icon><MoreOutlined /></template>
                      </a-button>
                      <template #overlay>
                        <a-menu @click.stop class="site-actions-menu">
                          <a-menu-item key="test" @click="handleTest(asSite(record))">
                            <ExperimentOutlined />
                            <span>{{ isRelayOnlySitePayload(asSite(record)) ? '验证出口' : '测试连接' }}</span>
                          </a-menu-item>
                          <a-menu-item
                            v-if="siteCanCheckin(asSite(record)) && !isRelayOnlySitePayload(asSite(record))"
                            key="checkin"
                            @click="handleCheckin(asSite(record))"
                          >
                            <ReloadOutlined />
                            <span>{{ siteCheckinActionLabel(asSite(record)) }}</span>
                          </a-menu-item>
                          <a-menu-item key="api-key" @click="openApiKeyDialog(asSite(record))">
                            <KeyOutlined />
                            <span>API Key</span>
                          </a-menu-item>
                          <a-menu-item
                            key="balance"
                            :disabled="isBalanceProbing(asSite(record).id)"
                            @click="handleProbeSiteBalance(asSite(record))"
                          >
                            <DollarCircleOutlined />
                            <span>{{ isBalanceProbing(asSite(record).id) ? '余额读取中' : '读取余额' }}</span>
                          </a-menu-item>
                          <a-menu-item
                            v-if="siteSupportsApiKeySync(asSite(record))"
                            key="api-key-refresh"
                            :disabled="isApiKeyRefreshing(asSite(record).id)"
                            @click="handleRefreshSiteApiKeys(asSite(record))"
                          >
                            <ReloadOutlined />
                            <span>{{ isApiKeyRefreshing(asSite(record).id) ? '更新中' : '更新 API Key' }}</span>
                          </a-menu-item>
                          <a-menu-item
                            v-if="siteSupportsInvite(asSite(record))"
                            key="invite"
                            :disabled="isInviteLoading(asSite(record).id)"
                            @click="loadInviteInfo(asSite(record))"
                          >
                            <ShareAltOutlined />
                            <span>{{ isInviteLoading(asSite(record).id) ? '邀请读取中' : '邀请信息' }}</span>
                          </a-menu-item>
                          <a-menu-item key="delete" danger @click="handleDelete(asSite(record))">
                            <DeleteOutlined />
                            <span>删除站点</span>
                          </a-menu-item>
                        </a-menu>
                      </template>
                    </a-dropdown>
                  </div>
                </template>
              </template>
            </a-table>
          </div>
        </div>
      </a-card>

      <a-modal
        v-model:open="drawerOpen"
        :title="null"
        width="1180px"
        centered
        :mask-closable="false"
        :destroy-on-close="false"
        wrap-class-name="site-editor-modal-wrap"
        class="site-editor-modal"
      >
        <template #closeIcon>
          <span class="site-editor-modal__close">×</span>
        </template>
        <div class="site-editor-modal__frame">
          <div class="site-editor-modal__header">
            <div class="site-editor-modal__title">
              {{ editingId ? `编辑站点 · ${editingSite?.name ?? ''}` : '新建站点' }}
            </div>
          </div>

          <div class="site-editor-modal__body">
            <div class="drawer-form-shell site-editor-shell">
              <a-alert
                v-if="saveFeedback"
                type="success"
                show-icon
                :message="saveFeedback"
                class="site-editor-alert"
              />

              <a-alert
                v-if="pluginMismatch && recommendedPlugin"
                type="warning"
                show-icon
                class="site-editor-alert"
                :message="`当前域名更适合使用 ${recommendedPlugin.name}`"
              >
                <template #description>
                  <a-button type="link" style="padding: 0" @click="applyRecommendedPlugin">
                    切换到推荐插件
                  </a-button>
                </template>
              </a-alert>

              <a-alert
                v-if="testFeedback"
                :type="testFeedback.type"
                show-icon
                :message="testFeedback.title"
                class="site-editor-alert"
              >
                <template #description>
                  <pre class="feedback-pre">{{ testFeedback.message }}</pre>
                </template>
              </a-alert>

              <a-form layout="vertical">
                <div class="site-editor-grid">
                  <div class="site-editor-column site-editor-column--primary">
                    <section class="site-editor-card site-editor-card--info site-editor-card--gateway">
                      <div class="site-editor-card__art site-editor-card__art--gateway" aria-hidden="true">
                        <img :src="siteEditorGatewayArtwork" alt="" />
                      </div>
                      <div class="site-editor-card__content">
                        <div class="site-editor-section-head">
                          <div class="site-editor-section-head__badge">◫</div>
                          <h3>基础信息</h3>
                        </div>

                        <div v-if="!editingId && canBatchRegisterEditor" class="nested-form-block site-editor-subblock">
                          <div class="site-editor-subblock__head site-editor-subblock__head--between">
                            <h4>批量注册生成账号</h4>
                            <a-switch
                              v-model:checked="batchRegisterEnabled"
                              checked-children="启用"
                              un-checked-children="关闭"
                            />
                          </div>
                          <a-alert
                            type="warning"
                            show-icon
                            message="仅用于对注册邮箱验证不敏感、且你确认允许批量注册的站点。"
                            description="保存时会按邮箱规则循环请求注册接口，登录新账号，同步 API Key，并把每个账号创建为一个站点。"
                            class="site-editor-config-alert"
                          />
                          <a-row v-if="batchRegisterEnabled" :gutter="[18, 4]">
                            <a-col :xs="24" :md="12">
                              <a-form-item>
                                <template #label>
                                  <span class="field-label-help">
                                    邮箱规则
                                    <a-tooltip placement="right">
                                      <template #title>
                                        <div class="email-pattern-tooltip">
                                          <div v-for="example in emailPatternExamples" :key="example">{{ example }}</div>
                                        </div>
                                      </template>
                                      <QuestionCircleOutlined class="field-help-icon" />
                                    </a-tooltip>
                                  </span>
                                </template>
                                <a-input
                                  v-model:value="batchRegisterForm.email_pattern"
                                  placeholder="user+{n}@example.com"
                                />
                                <small class="field-help">支持序号和随机字符，例如 user+{n:03}@example.com。</small>
                              </a-form-item>
                            </a-col>
                            <a-col :xs="24" :md="12">
                              <a-form-item label="注册密码">
                                <a-input-password v-model:value="batchRegisterForm.password" placeholder="所有账号使用同一密码" />
                              </a-form-item>
                            </a-col>
                            <a-col :xs="12" :md="6">
                              <a-form-item label="请求次数">
                                <a-input-number v-model:value="batchRegisterForm.count" :min="1" :max="100" style="width: 100%" />
                              </a-form-item>
                            </a-col>
                            <a-col :xs="12" :md="6">
                              <a-form-item label="起始序号">
                                <a-input-number v-model:value="batchRegisterForm.start_index" :min="1" style="width: 100%" />
                              </a-form-item>
                            </a-col>
                          </a-row>
                          <div v-if="batchRegisterResult" class="result-block">
                            <p>创建 {{ batchRegisterResult.created_count }} 个，失败 {{ batchRegisterResult.failed_count }} 个。</p>
                            <p v-for="item in batchRegisterResult.items.slice(0, 5)" :key="`${item.index}-${item.email}`">
                              {{ item.ok ? '成功' : '失败' }} #{{ item.index }} {{ item.email }}：{{ item.message }}
                            </p>
                          </div>
                        </div>

                        <a-row :gutter="[18, 4]">
                          <a-col :xs="24" :md="12">
                            <a-form-item label="站点名称">
                              <a-input v-model:value="editor.name" />
                            </a-form-item>
                          </a-col>
                          <a-col :xs="24" :md="12">
                            <a-form-item label="基础 URL">
                              <a-input v-model:value="editor.base_url" />
                            </a-form-item>
                          </a-col>
                          <a-col :xs="24" :md="12">
                            <a-form-item label="插件类型">
                              <a-select v-model:value="editor.plugin_key" :options="pluginOptions" />
                            </a-form-item>
                          </a-col>
                          <a-col :xs="24" :md="12">
                            <a-form-item label="分组标签">
                              <a-select
                                v-model:value="editorGroupNames"
                                mode="multiple"
                                :options="groupOptions"
                                :max-tag-count="4"
                                placeholder="选择分组"
                              />
                              <small class="field-help">分组请在站点中心顶部“分组管理”里单独维护；这里仅负责选择。</small>
                            </a-form-item>
                          </a-col>
                        </a-row>

                        <a-form-item label="站点备注">
                          <a-textarea v-model:value="editor.notes" :rows="3" placeholder="请输入站点备注（选填）" />
                        </a-form-item>

                        <a-form-item label="启用状态" class="site-editor-switch-item">
                          <a-switch v-model:checked="editor.is_enabled" checked-children="启用" un-checked-children="停用" />
                        </a-form-item>
                      </div>
                    </section>

                    <section class="site-editor-card site-editor-card--wide">
                      <div class="site-editor-card__content">
                        <div class="site-editor-section-head site-editor-section-head--between">
                          <div>
                            <div class="site-editor-section-head">
                              <div class="site-editor-section-head__badge">⌘</div>
                              <h3>浏览器存储导入</h3>
                            </div>
                            <p>在目标站点控制台运行采集脚本，粘贴输出后自动识别插件类型并回填账号凭证。</p>
                          </div>
                          <a-space wrap align="center">
                            <a-button @click="handleCopyConsoleScript">复制脚本</a-button>
                            <a-button
                              type="primary"
                              :loading="localStorageAnalyzeLoading"
                              @click="handleAnalyzeLocalStorage"
                            >
                              分析并回填
                            </a-button>
                          </a-space>
                        </div>

                        <div class="site-editor-storage-grid">
                          <a-form-item label="控制台函数">
                            <a-textarea
                              :value="consoleCollectorScript"
                              :rows="8"
                              readonly
                            />
                            <small class="field-help">复制后到目标站点控制台执行；脚本会尝试把结果写入剪贴板，并输出 URL、可读 Cookie、localStorage、sessionStorage 和可识别 token payload。</small>
                          </a-form-item>

                          <a-form-item label="脚本输出 JSON">
                            <a-textarea
                              v-model:value="localStorageRawText"
                              :rows="8"
                              placeholder="粘贴控制台输出的 JSON；粘贴后会自动分析。"
                              @paste="handleStoragePayloadPaste"
                            />
                            <small class="field-help">系统会自动切换插件类型，再把 access_token、refresh_token、邮箱、用户 ID、User-Agent 等写入当前插件支持的字段。</small>
                          </a-form-item>
                        </div>
                      </div>
                    </section>

                    <section v-if="currentPlugin" class="site-editor-card site-editor-card--wide site-editor-card--account">
                      <div class="site-editor-card__art site-editor-card__art--account" aria-hidden="true">
                        <img :src="siteEditorAccountArtwork" alt="" />
                      </div>
                      <div class="site-editor-card__content">
                        <div class="site-editor-section-head site-editor-section-head--between">
                          <div>
                            <div class="site-editor-section-head">
                              <div class="site-editor-section-head__badge">◎</div>
                              <h3>账号凭证</h3>
                            </div>
                            <p>{{ currentPlugin.auth_hint || '可先在站点侧完成登录，再回到后台回填最终凭证。' }}</p>
                          </div>
                          <a-space wrap align="center">
                            <a-button :disabled="!officialSiteUrl" @click="handleOpenOfficialSite">打开官网</a-button>
                            <a-button
                              v-if="showAuthEntryButton"
                              type="primary"
                              ghost
                              :disabled="!authEntryUrl"
                              @click="handleOpenAuthSite"
                            >
                              {{ authEntryLabel }}
                            </a-button>
                          </a-space>
                        </div>

                        <a-row :gutter="[18, 4]">
                          <a-col
                            v-for="field in primaryCredentialFields"
                            :key="field.name"
                            :xs="24"
                            :md="field.type === 'textarea' ? 24 : 12"
                          >
                            <a-form-item :label="field.label">
                              <a-textarea
                                v-if="field.type === 'textarea'"
                                v-model:value="editor.credentials[field.name]"
                                :rows="3"
                                :placeholder="field.placeholder"
                                :name="credentialInputName(field.name)"
                                :autocomplete="credentialAutocomplete(field.name, field.type)"
                              />
                              <a-input-password
                                v-else-if="field.type === 'password'"
                                v-model:value="editor.credentials[field.name]"
                                :placeholder="field.placeholder"
                                :name="credentialInputName(field.name)"
                                :autocomplete="credentialAutocomplete(field.name, field.type)"
                              />
                              <a-input
                                v-else
                                v-model:value="editor.credentials[field.name]"
                                :placeholder="field.placeholder"
                                :name="credentialInputName(field.name)"
                                :autocomplete="credentialAutocomplete(field.name, field.type)"
                              />
                              <small v-if="field.help_text" class="field-help">{{ field.help_text }}</small>
                            </a-form-item>
                          </a-col>
                        </a-row>

                        <div v-if="manualLoginFields.length" class="nested-form-block site-editor-subblock">
                          <div class="site-editor-subblock__head">
                            <h4>账号密码</h4>
                          </div>
                          <a-row :gutter="[18, 4]">
                            <a-col
                              v-for="field in manualLoginFields"
                              :key="field.name"
                              :xs="24"
                              :md="12"
                            >
                              <a-form-item :label="field.label">
                                <a-input-password
                                  v-if="field.type === 'password'"
                                  v-model:value="editor.credentials[field.name]"
                                  :placeholder="field.placeholder"
                                  :name="credentialInputName(field.name)"
                                  :autocomplete="credentialAutocomplete(field.name, field.type)"
                                />
                                <a-input
                                  v-else
                                  v-model:value="editor.credentials[field.name]"
                                  :placeholder="field.placeholder"
                                  :name="credentialInputName(field.name)"
                                  :autocomplete="credentialAutocomplete(field.name, field.type)"
                                />
                                <small v-if="field.help_text" class="field-help">{{ field.help_text }}</small>
                              </a-form-item>
                            </a-col>
                          </a-row>
                        </div>

                        <div v-if="totpCredentialFields.length" class="nested-form-block site-editor-subblock">
                          <div class="site-editor-subblock__head site-editor-subblock__head--between">
                            <h4>双重验证</h4>
                            <a-button
                              v-if="editingId"
                              :loading="totpPreviewLoading"
                              @click="handlePreviewTotp"
                            >
                              查看当前验证码
                            </a-button>
                          </div>
                          <a-row :gutter="[18, 4]">
                            <a-col
                              v-for="field in totpCredentialFields"
                              :key="field.name"
                              :xs="24"
                            >
                              <a-form-item :label="field.label">
                                <a-textarea
                                  v-model:value="editor.credentials[field.name]"
                                  :rows="3"
                                  :placeholder="field.placeholder"
                                  :name="credentialInputName(field.name)"
                                  autocomplete="off"
                                />
                                <small v-if="field.help_text" class="field-help">{{ field.help_text }}</small>
                              </a-form-item>
                            </a-col>
                          </a-row>
	                        </div>
	                      </div>
	                    </section>
                  </div>

                  <div class="site-editor-column site-editor-column--secondary">
                    <section v-if="currentPlugin" class="site-editor-card site-editor-card--config site-editor-card--cloud">
                      <div class="site-editor-card__art site-editor-card__art--cloud" aria-hidden="true">
                        <img :src="siteEditorCloudArtwork" alt="" />
                      </div>
                      <div class="site-editor-card__content">
                        <div class="site-editor-section-head">
                          <div class="site-editor-section-head__badge">▣</div>
                          <h3>插件配置</h3>
                        </div>
                        <a-alert
                          v-if="editor.plugin_key === 'api-supplier'"
                          type="info"
                          show-icon
                          message="此站点只用于网关转发，不参与签到 / 资料同步。"
                          description="api_format 推荐填 codex / openai / anthropic / gemini / general（写错只会影响路由分类，不会影响转发）。Base URL 与 API Key 是必填项。"
                          class="site-editor-config-alert"
                        />
                        <a-row :gutter="[0, 4]">
                          <a-col
                            v-for="field in currentPlugin.config_fields"
                            :key="field.name"
                            :xs="24"
                          >
                            <a-form-item :label="field.label">
                              <a-textarea
                                v-if="field.type === 'textarea'"
                                :value="configTextValue(field.name)"
                                :rows="3"
                                :placeholder="field.placeholder"
                                @update:value="(value) => updateConfigField(field.name, value)"
                              />
                              <a-input-number
                                v-else-if="field.type === 'number'"
                                :value="configNumberValue(field.name)"
                                style="width: 100%"
                                :placeholder="field.placeholder"
                                @update:value="(value) => updateConfigField(field.name, value)"
                              />
                              <a-input
                                v-else
                                :value="configTextValue(field.name)"
                                :placeholder="field.placeholder"
                                @update:value="(value) => updateConfigField(field.name, value)"
                              />
                              <small v-if="field.help_text" class="field-help">{{ field.help_text }}</small>
                            </a-form-item>
                          </a-col>
                        </a-row>
                      </div>
                    </section>
                  </div>
                </div>
              </a-form>
            </div>
          </div>
        </div>

        <template #footer>
          <div class="drawer-footer site-editor-modal__footer">
            <a-space wrap>
              <a-button @click="closeDrawer">取消</a-button>
              <a-button v-if="editingSite" :loading="busy" @click="handleTest(editingSite)">{{ testActionLabel }}</a-button>
              <a-button
                v-if="editingSite && !isRelayOnlyEditor"
                :loading="busy"
                @click="handleCheckin(editingSite)"
              >
                {{ primaryActionLabel }}
              </a-button>
              <a-button type="primary" :loading="busy" @click="saveSite">
                {{ editingSite ? '保存修改' : (batchRegisterEnabled ? '批量注册并创建' : '创建站点') }}
              </a-button>
              <a-button v-if="editingSite" danger :loading="busy" @click="handleDelete(editingSite)">删除站点</a-button>
            </a-space>
          </div>
        </template>
      </a-modal>

      <a-modal
        v-model:open="ccSwitchConfigOpen"
        title="供应商配置"
        width="820px"
        :confirm-loading="ccSwitchImportLoading"
        :ok-text="ccSwitchImportOkText"
        :footer="ccSwitchConfigTab === 'import' ? undefined : null"
        @ok="submitCCSwitchImport"
      >
        <a-tabs v-model:active-key="ccSwitchConfigTab" :animated="false">
          <a-tab-pane key="import" tab="导入">
            <a-space style="margin-bottom: 12px">
              <a-radio-group v-model:value="ccSwitchImportMode" button-style="solid">
                <a-radio-button value="json">JSON</a-radio-button>
                <a-radio-button value="sql">SQL</a-radio-button>
              </a-radio-group>
              <a-button @click="openCCSwitchFilePicker">{{ ccSwitchFileButtonLabel }}</a-button>
              <a-button
                v-if="ccSwitchImportMode === 'sql'"
                :loading="ccSwitchSqlPreviewLoading"
                @click="resolveCCSwitchSqlPreview"
              >
                解析 SQL
              </a-button>
            </a-space>
            <a-textarea
              v-model:value="ccSwitchImportText"
              :rows="12"
              :placeholder="ccSwitchImportPlaceholder"
            />
            <a-alert
              v-if="ccSwitchPreviewError"
              type="error"
              show-icon
              style="margin-top: 12px"
              :message="ccSwitchPreviewError"
            />
            <div v-else-if="ccSwitchPreviewRows.length" class="result-block" style="margin-top: 12px">
              <a-space>
                <a-select
                  v-model:value="ccSwitchSelectedSections"
                  mode="multiple"
                  :options="ccSwitchSectionOptions"
                  placeholder="选择要导入的供应商分类"
                  style="min-width: 260px"
                />
                <a-input
                  v-model:value="ccSwitchPreviewSearch"
                  allow-clear
                  placeholder="搜索名称 / 站点 / 备注"
                  style="width: 240px"
                />
              </a-space>
              <a-space wrap>
                <a-tag
                  v-for="item in ccSwitchSectionOptions"
                  :key="item.value"
                  :color="ccSwitchSelectedSections.includes(String(item.value)) ? 'processing' : 'default'"
                >
                  {{ item.label }}
                </a-tag>
                <a-tag color="red">
                  缺认证 {{ ccSwitchFilteredPreviewRows.filter((item) => !item.hasAuth).length }}
                </a-tag>
                <a-tag>合计 {{ ccSwitchFilteredPreviewRows.length }}</a-tag>
              </a-space>
              <div class="table-fill table-fill--management table-fill--modal">
                <a-table
                  :columns="ccSwitchPreviewColumns"
                  :data-source="ccSwitchFilteredPreviewRows"
                  :loading="ccSwitchSqlPreviewLoading"
                  :pagination="{ pageSize: tablePageSize }"
                  :row-key="ccSwitchPreviewRowKey"
                  size="small"
                  :scroll="{ x: 860, y: modalTableY }"
                >
                  <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'current'">
                      <a-tag v-if="record.isCurrent" color="processing">默认</a-tag>
                      <span v-else>-</span>
                    </template>
                    <template v-else-if="column.key === 'name'">
                      <a-space size="small">
                        <a-tag v-if="!record.hasAuth" color="error">缺认证</a-tag>
                        <span>{{ record.name }}</span>
                      </a-space>
                    </template>
                    <template v-else-if="column.key === 'website'">
                      <span>{{ record.website || '未填写' }}</span>
                    </template>
                    <template v-else-if="column.key === 'apiKeyStatus'">
                      <a-tag :color="record.hasAuth ? 'green' : 'red'">
                        {{ record.apiKeyStatus }}
                      </a-tag>
                    </template>
                  </template>
                </a-table>
              </div>
            </div>
          </a-tab-pane>
          <a-tab-pane key="export" tab="导出">
            <a-space style="margin-bottom: 12px">
              <a-button :loading="ccSwitchExportLoading" @click="handleCCSwitchExport">重新生成</a-button>
              <a-button type="primary" :disabled="!ccSwitchExportText.trim()" @click="downloadCCSwitchExport">下载 JSON</a-button>
            </a-space>
            <a-textarea
              :value="ccSwitchExportText"
              :rows="20"
              readonly
              placeholder="点击重新生成后显示导出内容"
            />
          </a-tab-pane>
        </a-tabs>
      </a-modal>

      <a-modal
        v-model:open="duplicateCheckOpen"
        title="清理检测"
        width="1080px"
      >
        <a-alert
          v-if="duplicateChecked && !duplicateGroups.length"
          type="success"
          show-icon
          message="未发现需要合并的重复站点。"
          style="margin-bottom: 12px"
        />
        <a-input
          v-model:value="duplicateSearch"
          allow-clear
          placeholder="搜索基础 URL / 账号 / 站点名"
          style="margin-bottom: 12px"
        />
        <div class="table-fill table-fill--management table-fill--modal">
          <a-table
            :columns="duplicateColumns"
            :data-source="filteredDuplicateGroups"
            :loading="duplicateCheckLoading"
            :pagination="{ pageSize: tablePageSize }"
            :row-key="duplicateGroupRowKey"
            size="small"
            :scroll="{ x: 1040, y: modalTableY }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'password'">
                <a-tag :color="record.password_present ? 'processing' : 'default'">
                  {{ record.password_present ? '已填写' : '留空' }}
                </a-tag>
              </template>
              <template v-else-if="column.key === 'suggested'">
                <div class="result-block">
                  <span>ID {{ record.suggested_keep_id }}</span>
                  <span>{{ duplicateSuggestedSiteName(record) }}</span>
                </div>
              </template>
              <template v-else-if="column.key === 'sites'">
                <div class="tag-list">
                  <a-tag
                    v-for="site in record.sites"
                    :key="site.id"
                    :color="site.suggested_keep ? 'processing' : 'default'"
                  >
                    {{ site.suggested_keep ? `保留 ${site.name}#${site.id}` : `${site.name}#${site.id}` }}
                  </a-tag>
                </div>
              </template>
            </template>
          </a-table>
        </div>
        <template #footer>
          <a-space>
            <a-button @click="duplicateCheckOpen = false">关闭</a-button>
            <a-button type="primary" :loading="duplicateMergeLoading" @click="handleSuggestedDuplicateMerge">
              按建议合并
            </a-button>
          </a-space>
        </template>
      </a-modal>

      <a-modal
        v-model:open="totpPreviewOpen"
        title="当前 TOTP 验证码"
        :footer="null"
      >
        <div v-if="totpPreview" class="result-block">
          <p><strong>验证码：</strong><span class="totp-code">{{ totpPreview.code }}</span></p>
          <p><strong>剩余有效期：</strong>{{ totpPreview.expires_in }} 秒</p>
        </div>
      </a-modal>

      <a-modal
        v-model:open="inviteDialogOpen"
        :title="`邀请信息 · ${inviteDialogSiteName}`"
        width="720px"
        :footer="null"
        destroy-on-close
      >
        <div class="invite-dialog">
          <a-alert
            type="info"
            show-icon
            message="邀请信息来自当前站点账号资料读取结果。"
          />
          <a-spin :spinning="inviteDialogLoading">
            <div class="invite-dialog__grid">
              <div class="invite-dialog__field">
                <div class="invite-dialog__label">邀请链接</div>
                <div class="invite-dialog__control">
                  <a-input :value="inviteDialogLink" readonly placeholder="未读取到邀请链接" />
                  <a-button :disabled="!inviteDialogLink" @click="copyInviteLink">复制链接</a-button>
                </div>
              </div>
              <div class="invite-dialog__field">
                <div class="invite-dialog__label">邀请码</div>
                <div class="invite-dialog__control invite-dialog__control--code">
                  <a-input :value="inviteDialogCode" readonly placeholder="未读取到邀请码" />
                  <a-button :disabled="!inviteDialogCode" @click="copyInviteCode">复制邀请码</a-button>
                </div>
              </div>
            </div>
          </a-spin>
          <div class="invite-dialog__actions">
            <a-space wrap>
              <a-button :loading="inviteDialogLoading" @click="refreshInviteDialog">
                刷新读取
              </a-button>
              <a-button type="primary" :disabled="!inviteDialogLink && !inviteDialogCode" @click="copyInviteBundle">
                复制全部
              </a-button>
              <a-button @click="inviteDialogOpen = false">关闭</a-button>
            </a-space>
          </div>
        </div>
      </a-modal>

      <a-modal
        v-model:open="apiKeyDialogOpen"
        title="查看 API Key 与请求 URL"
        width="820px"
        :confirm-loading="apiKeyDialogSaving"
        ok-text="保存配置"
        @ok="saveApiKeyDialog"
      >
        <a-form layout="vertical">
          <a-form-item label="站点">
            <a-input :value="apiKeyDialogForm.site_name" readonly />
          </a-form-item>
          <a-form-item label="API Key">
            <a-space direction="vertical" style="width: 100%">
              <a-space>
                <a-button
                  type="primary"
                  ghost
                  :disabled="!apiKeyDialogEntries.length"
                  @click="copyPrimaryApiKeyFromDialog"
                >
                  复制主 Key
                </a-button>
                <span class="table-subtitle">可删除接口同步或自定义 Key，保存后同步到本地配置。</span>
              </a-space>
              <div v-if="apiKeyDialogEntries.length" class="api-key-dialog-list">
                <div
                  v-for="entry in apiKeyDialogEntries"
                  :key="entry.id"
                  class="api-key-dialog-item"
                >
                  <div class="api-key-dialog-item__head">
                    <a-space>
                      <strong>{{ entry.name }}</strong>
                      <a-tag v-if="entry.isPrimary" color="processing">主 Key</a-tag>
                      <a-tag :color="entry.isManual ? 'blue' : 'purple'">{{ apiKeySourceLabel(entry) }}</a-tag>
                      <a-tag v-if="entry.routeType">{{ apiKeyRouteTypeLabel(entry.routeType) }}</a-tag>
                      <a-tag>{{ apiKeyRoutePathLabel(entry.routePath) }}</a-tag>
                      <a-tag :color="entry.status === 'active' ? 'green' : 'default'">{{ entry.status }}</a-tag>
                    </a-space>
                    <a-space size="small">
                      <a-button size="small" @click="copyApiKeyFromDialog(entry.key)">复制</a-button>
                      <a-button size="small" danger @click="removeApiKey(entry)">删除</a-button>
                    </a-space>
                  </div>
                  <div class="api-key-dialog-field">
                    <div class="api-key-dialog-field__label">API Key</div>
                    <a-input-password
                      :value="entry.key"
                      readonly
                      visibility-toggle
                      placeholder="当前 API Key 为空"
                    />
                  </div>
                  <div class="api-key-dialog-field">
                    <div class="api-key-dialog-field__label">专用请求 URL</div>
                    <a-textarea
                      class="api-key-dialog-item__urls"
                      :value="apiKeyRequestUrlDraft(entry)"
                      :rows="2"
                      placeholder="当前 Key 专用请求 URL，每行一个；留空使用下方站点级 URL"
                      @update:value="(value) => updateApiKeyRequestUrlDraft(entry, value)"
                    />
                  </div>
                  <div class="api-key-dialog-field">
                    <div class="api-key-dialog-field__label">请求路径</div>
                    <a-select
                      :value="apiKeyRoutePathDraft(entry)"
                      :options="apiKeyRoutePathOptions"
                      @update:value="(value) => updateApiKeyRoutePathDraft(entry, value)"
                    />
                  </div>
                  <div class="api-key-dialog-item__paths">
                    <div class="api-key-dialog-field">
                      <div class="api-key-dialog-field__label">图片生成 Path</div>
                      <a-input
                        :value="apiKeyImageGenerationPathDraft(entry)"
                        placeholder="当前 Key 生图 Path，可留空"
                        @update:value="(value) => updateApiKeyImagePathDraft(entry, 'generation', value)"
                      />
                    </div>
                    <div class="api-key-dialog-field">
                      <div class="api-key-dialog-field__label">图片编辑 Path</div>
                      <a-input
                        :value="apiKeyImageEditPathDraft(entry)"
                        placeholder="当前 Key 编辑 Path，可留空"
                        @update:value="(value) => updateApiKeyImagePathDraft(entry, 'edit', value)"
                      />
                    </div>
                  </div>
                </div>
              </div>
              <a-empty v-else description="当前站点未配置 API Key" />
            </a-space>
          </a-form-item>
          <a-form-item label="添加自定义 API Key">
            <div class="manual-api-key-editor">
              <div class="api-key-dialog-field manual-api-key-editor__name">
                <div class="api-key-dialog-field__label">名称</div>
                <a-input
                  v-model:value="manualApiKeyForm.name"
                  placeholder="名称，例如 Claude 备用"
                />
              </div>
              <div class="api-key-dialog-field manual-api-key-editor__type">
                <div class="api-key-dialog-field__label">路由类型</div>
                <a-select
                  v-model:value="manualApiKeyForm.route_type"
                  :options="apiKeyRouteTypeOptions"
                />
              </div>
              <div class="api-key-dialog-field manual-api-key-editor__path">
                <div class="api-key-dialog-field__label">请求路径</div>
                <a-select
                  v-model:value="manualApiKeyForm.route_path"
                  :options="apiKeyRoutePathOptions"
                />
              </div>
              <div class="api-key-dialog-field manual-api-key-editor__key">
                <div class="api-key-dialog-field__label">API Key</div>
                <a-input-password
                  v-model:value="manualApiKeyForm.key"
                  placeholder="sk-..."
                  autocomplete="new-password"
                  @press-enter="addManualApiKey"
                />
              </div>
              <div class="api-key-dialog-field manual-api-key-editor__urls">
                <div class="api-key-dialog-field__label">专用请求 URL</div>
                <a-textarea
                  v-model:value="manualApiKeyForm.request_base_urls"
                  :rows="2"
                  placeholder="当前 Key 专用请求 URL，可留空"
                />
              </div>
              <div class="api-key-dialog-field manual-api-key-editor__generation">
                <div class="api-key-dialog-field__label">图片生成 Path</div>
                <a-input
                  v-model:value="manualApiKeyForm.image_generation_path"
                  placeholder="生图 Path，可留空"
                />
              </div>
              <div class="api-key-dialog-field manual-api-key-editor__edit">
                <div class="api-key-dialog-field__label">图片编辑 Path</div>
                <a-input
                  v-model:value="manualApiKeyForm.image_edit_path"
                  placeholder="编辑 Path，可留空"
                />
              </div>
              <div class="api-key-dialog-field manual-api-key-editor__action">
                <div class="api-key-dialog-field__label api-key-dialog-field__label--spacer" aria-hidden="true">操作</div>
                <a-button type="primary" @click="addManualApiKey">添加</a-button>
              </div>
            </div>
            <small class="field-help">保存后自定义 Key 会与接口同步 Key 同时存在；专用 URL 优先于站点级 URL，适合同一站点 Claude/GPT 走不同 API URL。</small>
          </a-form-item>
          <a-form-item label="请求 API URL 列表">
            <a-textarea
              v-model:value="apiKeyDialogForm.request_api_urls"
              :rows="8"
              placeholder="每行一个 URL。网关会按顺序回退，全部失败后再轮转下一个路由。"
            />
            <small class="field-help">留空时会继续使用当前默认出口或站点 Base URL。</small>
          </a-form-item>
          <a-row :gutter="16">
            <a-col :xs="24" :md="12">
              <a-form-item label="图片生成 Path">
                <a-input
                  v-model:value="apiKeyDialogForm.image_generation_path"
                  placeholder="/images/generations"
                />
                <small class="field-help">纯文本生图接口路径，留空使用默认值。</small>
              </a-form-item>
            </a-col>
            <a-col :xs="24" :md="12">
              <a-form-item label="图片编辑 Path">
                <a-input
                  v-model:value="apiKeyDialogForm.image_edit_path"
                  placeholder="/images/edits"
                />
                <small class="field-help">参考图编辑/融合接口路径，留空使用默认值。</small>
              </a-form-item>
            </a-col>
          </a-row>
          <a-form-item label="当前生效顺序">
            <div class="tag-list">
              <a-tag v-for="url in apiKeyDialogPreviewUrls" :key="url" color="processing">
                {{ url }}
              </a-tag>
              <a-tag v-if="!apiKeyDialogPreviewUrls.length">暂无可用请求 URL</a-tag>
            </div>
          </a-form-item>
        </a-form>
      </a-modal>

      <a-modal
        v-model:open="checkinConfigOpen"
        title="签到配置"
        width="760px"
        :confirm-loading="checkinSettingsBusy"
        @ok="saveCheckinConfig"
      >
        <a-form layout="vertical">
          <a-row :gutter="16">
            <a-col :xs="24" :md="12">
              <a-form-item label="时区">
                <a-input v-model:value="checkinConfigForm.timezone" />
              </a-form-item>
            </a-col>
            <a-col :xs="24" :md="12">
              <a-form-item label="签到时间">
                <a-input v-model:value="checkinConfigForm.daily_run_time" type="time" />
              </a-form-item>
            </a-col>
          </a-row>

          <a-row :gutter="16">
            <a-col :xs="24" :md="12">
              <a-form-item label="同站点 URL 并发数">
                <a-input-number v-model:value="checkinConfigForm.checkin_concurrency" style="width: 100%" :min="1" :max="20" />
                <small class="field-help">同一 base_url 下多账号最多同时执行数，默认 1。</small>
              </a-form-item>
            </a-col>
            <a-col :xs="24" :md="12">
              <a-form-item label="不同站点总并发数">
                <a-input-number v-model:value="checkinConfigForm.checkin_global_concurrency" style="width: 100%" :min="1" :max="50" />
                <small class="field-help">不同 base_url 之间可同时执行的总任务数。</small>
              </a-form-item>
            </a-col>
          </a-row>

          <a-row :gutter="16">
            <a-col :xs="24" :md="12">
              <a-form-item label="站点间隔（秒）">
                <a-input-number v-model:value="checkinConfigForm.checkin_interval_seconds" style="width: 100%" :min="0" :max="60" />
              </a-form-item>
            </a-col>
            <a-col :xs="24" :md="12">
              <a-form-item label="失败重试次数">
                <a-input-number v-model:value="checkinConfigForm.retry_count" style="width: 100%" :min="0" :max="5" />
              </a-form-item>
            </a-col>
          </a-row>

          <a-row :gutter="16">
            <a-col :xs="24" :md="12">
              <a-form-item label="请求超时（秒）">
                <a-input-number v-model:value="checkinConfigForm.request_timeout" style="width: 100%" :min="5" :max="120" />
              </a-form-item>
            </a-col>
            <a-col :xs="24" :md="12">
              <a-form-item label="定时签到">
                <a-switch v-model:checked="checkinConfigForm.schedule_enabled" checked-children="启用" un-checked-children="关闭" />
              </a-form-item>
            </a-col>
          </a-row>

          <a-row :gutter="16">
            <a-col :xs="24" :md="12">
              <a-form-item label="仅执行启用站点">
                <a-switch v-model:checked="checkinConfigForm.only_enabled_sites" checked-children="是" un-checked-children="否" />
              </a-form-item>
            </a-col>
          </a-row>

          <a-alert
            type="info"
            show-icon
            style="margin-bottom: 0"
            message="默认并发数为 1"
            description="同站点 URL 并发默认 1，避免同站多账号同时触发风控；不同站点总并发默认 4。默认站点间隔为 1 秒。"
          />
        </a-form>

        <template #footer>
          <a-space>
            <a-button :loading="busy" @click="handleRunSchedulerNow">立即执行计划任务</a-button>
            <a-button @click="checkinConfigOpen = false">取消</a-button>
            <a-button type="primary" :loading="checkinSettingsBusy" @click="saveCheckinConfig">保存签到配置</a-button>
          </a-space>
        </template>
      </a-modal>

      <a-drawer
        v-model:open="checkinLogsOpen"
        title="最近执行"
        width="min(1040px, 100vw)"
        placement="right"
      >
        <a-input
          v-model:value="checkinRunSearch"
          allow-clear
          placeholder="搜索站点 / 结果 / 触发方式 / 消息"
          style="margin-bottom: 12px"
        />
        <div class="table-fill table-fill--management table-fill--drawer">
          <a-table
            :columns="checkinRunColumns"
            :data-source="filteredCheckinRuns"
            :pagination="{ pageSize: tablePageSize }"
            :row-key="(record) => record.id"
            size="middle"
            :scroll="{ x: 880, y: drawerTableY }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'site'">
                {{ record.site_name ?? '未知站点' }}
              </template>
              <template v-else-if="column.key === 'status'">
                <StatusPill :value="record.status" />
              </template>
              <template v-else-if="column.key === 'trigger_type'">
                {{ record.trigger_type }}
              </template>
              <template v-else-if="column.key === 'message'">
                {{ record.message }}
              </template>
              <template v-else-if="column.key === 'started_at'">
                {{ formatCheckinRunTime(record.started_at) }}
              </template>
            </template>
          </a-table>
        </div>
      </a-drawer>
    </div>
  </ShellLayout>
</template>

<style scoped>
.site-editor-shell {
  display: grid;
  gap: 16px;
}

.site-editor-alert {
  margin: 0;
  border-radius: 16px;
  border: 1px solid rgba(126, 167, 255, 0.22);
  background: rgba(255, 255, 255, 0.72);
  backdrop-filter: blur(18px);
}

.site-editor-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.35fr) minmax(360px, 1fr);
  gap: 18px;
  align-items: start;
}

.site-editor-column {
  display: grid;
  gap: 18px;
  min-width: 0;
}

.site-editor-card {
  position: relative;
  overflow: hidden;
  border-radius: 20px;
  border: 1px solid rgba(176, 207, 255, 0.42);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.92), rgba(241, 248, 255, 0.86)),
    rgba(232, 241, 255, 0.74);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 18px 42px rgba(107, 154, 235, 0.18);
  backdrop-filter: blur(22px);
}

.site-editor-card--info {
  min-height: 348px;
}

.site-editor-card--config {
  min-height: 348px;
}

.site-editor-card__content {
  position: relative;
  z-index: 1;
  padding: 18px 20px 16px;
}

.site-editor-card__art {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 12px 16px;
  pointer-events: none;
  user-select: none;
  z-index: 0;
  opacity: 0.18;
  filter: saturate(0.92) contrast(1.04);
}

.site-editor-card__art img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  object-position: center;
}

.site-editor-card__art--gateway {
  opacity: 0.2;
}

.site-editor-card__art--cloud {
  opacity: 0.18;
}

.site-editor-card__art--account {
  opacity: 0.16;
}

.site-editor-section-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}

.site-editor-section-head--between {
  align-items: flex-start;
  justify-content: space-between;
}

.site-editor-section-head--between > div:first-child {
  min-width: 0;
}

.site-editor-section-head h3 {
  margin: 0;
  color: #1c2f57;
  font-size: 20px;
  font-weight: 700;
  line-height: 1.2;
}

.site-editor-section-head p {
  margin: 10px 0 0;
  color: #7083a7;
  font-size: 13px;
  line-height: 1.6;
}

.site-editor-section-head__badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 10px;
  background: linear-gradient(180deg, rgba(69, 134, 255, 0.18), rgba(95, 159, 255, 0.1));
  color: #4282ff;
  font-size: 15px;
  font-weight: 700;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.9);
}

.site-editor-storage-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.site-editor-subblock {
  margin-top: 8px;
  border-radius: 16px;
  border: 1px solid rgba(178, 204, 245, 0.42);
  background: rgba(255, 255, 255, 0.5);
  padding: 14px 16px 4px;
}

.site-editor-subblock__head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}

.site-editor-subblock__head--between {
  justify-content: space-between;
}

.site-editor-subblock__head h4 {
  margin: 0;
  color: #29416b;
  font-size: 15px;
  font-weight: 700;
}

.site-editor-switch-item :deep(.ant-form-item-control-input) {
  min-height: auto;
}

.site-editor-config-alert {
  margin: 0 0 14px;
  border-radius: 14px;
}

.site-editor-modal__frame {
  position: relative;
  overflow: hidden;
  border-radius: 26px;
  background:
    radial-gradient(circle at top left, rgba(104, 174, 255, 0.2), rgba(104, 174, 255, 0) 28%),
    linear-gradient(180deg, rgba(251, 253, 255, 0.98), rgba(232, 240, 251, 0.95));
}

.site-editor-modal__frame::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.62), rgba(255, 255, 255, 0) 36%),
    repeating-linear-gradient(120deg, rgba(112, 164, 255, 0.06) 0 1px, transparent 1px 18px);
  opacity: 0.8;
  pointer-events: none;
}

.site-editor-modal__header {
  position: relative;
  z-index: 1;
  padding: 18px 30px 8px;
}

.site-editor-modal__title {
  color: #11295a;
  font-size: 18px;
  font-weight: 700;
  line-height: 1.2;
}

.site-editor-modal__body {
  position: relative;
  z-index: 1;
  max-height: calc(100vh - 220px);
  overflow: auto;
  padding: 10px 24px 0;
}

.site-editor-modal__footer {
  width: 100%;
  padding: 14px 24px 18px;
  border-top: 1px solid rgba(171, 200, 241, 0.42);
  background: linear-gradient(180deg, rgba(246, 250, 255, 0.56), rgba(233, 241, 252, 0.94));
  backdrop-filter: blur(16px);
}

.site-editor-modal__footer :deep(.ant-space) {
  width: 100%;
  justify-content: flex-end;
}

.site-editor-modal__close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  color: #7d8eb2;
  font-size: 26px;
  line-height: 1;
}

.field-help {
  color: #7b8aa9;
}

.field-label-help {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.field-help-icon {
  color: #7b8aa9;
  cursor: help;
  font-size: 13px;
}

.api-key-dialog-list {
  display: grid;
  gap: 10px;
}

.api-key-dialog-item {
  display: grid;
  gap: 8px;
  padding: 10px;
  border: 1px solid rgba(130, 156, 232, 0.24);
  border-radius: 8px;
  background: rgba(248, 251, 255, 0.72);
}

.api-key-dialog-item__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.api-key-dialog-field {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.api-key-dialog-field__label {
  color: #18315e;
  font-size: 12px;
  font-weight: 700;
  line-height: 1.4;
}

.api-key-dialog-field__label--spacer {
  visibility: hidden;
}

.api-key-dialog-item__urls {
  font-size: 12px;
}

.api-key-dialog-item__paths {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.manual-api-key-editor {
  display: grid;
  grid-template-columns: minmax(140px, 1fr) 150px 200px;
  gap: 8px;
  align-items: start;
}

.manual-api-key-editor__key,
.manual-api-key-editor__urls {
  grid-column: 1 / -1;
}

.manual-api-key-editor__generation {
  grid-column: 1 / 2;
}

.manual-api-key-editor__edit {
  grid-column: 2 / 3;
}

.manual-api-key-editor__action {
  grid-column: 3 / 4;
  justify-self: start;
}

.email-pattern-tooltip {
  display: grid;
  gap: 4px;
  font-family: 'IBM Plex Mono', monospace;
  font-size: 12px;
  line-height: 1.45;
}

.site-editor-shell :deep(.ant-form-item) {
  margin-bottom: 14px;
}

.site-editor-shell :deep(.ant-form-item-label > label) {
  color: #18315e;
  font-size: 14px;
  font-weight: 700;
}

.site-editor-shell :deep(.ant-input),
.site-editor-shell :deep(.ant-input-affix-wrapper),
.site-editor-shell :deep(.ant-input-number),
.site-editor-shell :deep(.ant-select-selector),
.site-editor-shell :deep(.ant-input-number-affix-wrapper),
.site-editor-shell :deep(textarea.ant-input) {
  border-radius: 12px;
  border-color: rgba(188, 211, 245, 0.88);
  background: rgba(255, 255, 255, 0.92);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.74);
}

.site-editor-shell :deep(.ant-input),
.site-editor-shell :deep(.ant-input-affix-wrapper),
.site-editor-shell :deep(.ant-input-number),
.site-editor-shell :deep(.ant-input-number-affix-wrapper),
.site-editor-shell :deep(.ant-select-selector) {
  min-height: 42px;
}

.site-editor-shell :deep(textarea.ant-input) {
  min-height: 94px;
}

.site-editor-shell :deep(.ant-input:focus),
.site-editor-shell :deep(.ant-input-affix-wrapper-focused),
.site-editor-shell :deep(.ant-input-number-focused),
.site-editor-shell :deep(.ant-select-focused .ant-select-selector) {
  border-color: #7aaaff;
  box-shadow: 0 0 0 3px rgba(73, 133, 255, 0.12);
}

.site-editor-shell :deep(.ant-btn) {
  border-radius: 12px;
  height: 40px;
  padding-inline: 18px;
  font-weight: 600;
}

.site-editor-shell :deep(.ant-btn-primary) {
  border-color: #4b82ff;
  background: linear-gradient(180deg, #62a0ff, #4b7dff);
  box-shadow: 0 10px 24px rgba(86, 132, 255, 0.24);
}

.site-editor-shell :deep(.ant-switch) {
  background: rgba(65, 119, 217, 0.32);
}

.site-editor-shell :deep(.ant-switch.ant-switch-checked) {
  background: linear-gradient(90deg, #2f7bff, #65a9ff);
}

.site-editor-shell :deep(.ant-select-multiple .ant-select-selection-item) {
  border-radius: 10px;
  background: rgba(89, 138, 255, 0.14);
}

@media (max-width: 1280px) {
  .site-editor-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 900px) {
  .site-editor-storage-grid {
    grid-template-columns: 1fr;
  }

  .site-editor-card__art {
    opacity: 0.14;
    padding: 10px 12px;
  }

  .site-editor-modal__body {
    max-height: calc(100vh - 188px);
    padding-inline: 14px;
  }

  .site-editor-modal__header {
    padding-inline: 18px;
  }

  .site-editor-modal__footer {
    padding-inline: 14px;
  }
}
</style>

<style scoped>
.sites-page {
  --sites-accent: #4f86ff;
  --sites-accent-strong: #2f6fff;
  --sites-text: #213a7a;
  --sites-text-soft: #7f93c7;
  --sites-border: rgba(110, 149, 255, 0.2);
  --sites-panel: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(245, 249, 255, 0.96));
  --sites-shadow: 0 18px 40px rgba(66, 113, 230, 0.09);
  gap: 18px;
}

.sites-toolbar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 14px 18px;
  align-items: center;
  padding: 6px 0 2px;
}

.sites-toolbar__segment,
.sites-toolbar__actions {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex-wrap: wrap;
}

.sites-toolbar__seg-btn.ant-btn,
.sites-toolbar__ghost-btn.ant-btn,
.sites-toolbar__create-btn.ant-btn,
.sites-list-toolbar__btn.ant-btn {
  height: 38px;
  padding: 0 18px;
  border-radius: 13px !important;
  border-color: rgba(125, 154, 233, 0.28) !important;
  background: rgba(255, 255, 255, 0.8) !important;
  color: #4261b3 !important;
  box-shadow: 0 10px 20px rgba(110, 140, 214, 0.08);
}

.sites-toolbar__seg-btn--primary.ant-btn,
.sites-toolbar__ghost-btn--strong.ant-btn,
.sites-toolbar__create-btn.ant-btn {
  background: linear-gradient(135deg, #5a8dff, #3f72ff) !important;
  color: #fff !important;
  border-color: transparent !important;
  box-shadow: 0 14px 24px rgba(79, 124, 255, 0.26);
}

.sites-toolbar__segment :deep(.ant-btn-icon),
.sites-toolbar__actions :deep(.ant-btn-icon) {
  font-size: 14px;
}

.sites-metric-grid {
  margin-top: 2px;
}

.sites-stat-card.ant-card {
  position: relative;
  overflow: hidden;
  min-height: 128px;
  border-radius: 24px !important;
  border: 1px solid var(--sites-border) !important;
  background: var(--sites-panel) !important;
  box-shadow: var(--sites-shadow) !important;
}

.sites-stat-card::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    radial-gradient(circle at 15% 16%, rgba(113, 179, 255, 0.18), transparent 32%),
    radial-gradient(circle at 92% 12%, rgba(125, 122, 255, 0.08), transparent 34%);
  pointer-events: none;
}

.sites-stat-card :deep(.ant-card-body) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 18px;
  position: relative;
  z-index: 1;
  min-width: 0;
}

.sites-stat-card__meta {
  display: grid;
  gap: 6px;
  min-width: 0;
  flex: 1 1 auto;
}

.sites-stat-card__eyebrow {
  font-size: 13px;
  font-weight: 700;
  color: #4d76d3;
}

.sites-stat-card__value {
  max-width: 100%;
  min-width: 0;
  overflow-wrap: anywhere;
  font-size: clamp(1.55rem, 1.65vw, 2.05rem);
  line-height: 1.05;
  font-weight: 700;
  letter-spacing: 0;
  color: #1f397a;
  font-variant-numeric: tabular-nums;
}

.sites-stat-card__value--balance {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: clamp(1.1rem, 1.18vw, 1.45rem);
  line-height: 1.12;
}

.sites-stat-card__value--positive {
  color: var(--sites-accent-strong);
}

.sites-stat-card__value--negative {
  color: #ff5d5d;
}

.sites-stat-card__value--zero,
.sites-stat-card__value--empty {
  color: #5f77b6;
}

.sites-stat-card__desc {
  font-size: 12px;
  color: var(--sites-text-soft);
}

.sites-stat-card__icon {
  width: 48px;
  height: 48px;
  border-radius: 16px;
  flex: 0 0 auto;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.92), rgba(233, 241, 255, 0.95)),
    linear-gradient(135deg, rgba(122, 163, 255, 0.22), rgba(92, 122, 255, 0.18));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.86),
    0 10px 24px rgba(93, 122, 205, 0.18);
  position: relative;
  overflow: hidden;
}

.sites-stat-card__icon::before,
.sites-stat-card__icon::after {
  content: '';
  position: absolute;
}

.sites-stat-card__icon--sheet::before {
  left: 50%;
  top: 50%;
  width: 22px;
  height: 28px;
  border-radius: 8px;
  background: linear-gradient(180deg, #6aa5ff, #4d82ff);
  transform: translate(-50%, -50%);
}

.sites-stat-card__icon--sheet::after {
  left: 50%;
  top: 44%;
  width: 14px;
  height: 2px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: 0 7px 0 rgba(255, 255, 255, 0.92), 0 14px 0 rgba(255, 255, 255, 0.92);
  transform: translate(-38%, -50%);
}

.sites-stat-card__icon--stack::before,
.sites-stat-card__icon--stack::after {
  left: 50%;
  width: 24px;
  height: 8px;
  border-radius: 999px;
  background: linear-gradient(180deg, #5e95ff, #4480ff);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.28) inset;
  transform: translateX(-50%);
}

.sites-stat-card__icon--stack::before {
  top: 14px;
  box-shadow:
    0 10px 0 0 #4f84ff,
    0 20px 0 0 #3f73ff;
}

.sites-stat-card__icon--stack::after {
  display: none;
}

.sites-stat-card__icon--gateway::before {
  left: 50%;
  top: 58%;
  width: 25px;
  height: 16px;
  border-radius: 6px;
  background: linear-gradient(180deg, #6aa0ff, #4f82ff);
  transform: translate(-50%, -50%);
}

.sites-stat-card__icon--gateway::after {
  left: 50%;
  top: 28%;
  width: 12px;
  height: 18px;
  border: 4px solid #6ea3ff;
  border-bottom: 0;
  border-radius: 8px 8px 0 0;
  transform: translate(-50%, 0);
}

.sites-stat-card__icon--wallet::before {
  left: 50%;
  top: 50%;
  width: 28px;
  height: 20px;
  border-radius: 10px;
  background: linear-gradient(180deg, #6ca0ff, #4d80ff);
  transform: translate(-50%, -50%);
}

.sites-stat-card__icon--wallet::after {
  left: 58%;
  top: 50%;
  width: 12px;
  height: 9px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.92);
  transform: translate(-50%, -50%);
}

.sites-stat-card__icon--check::before {
  left: 50%;
  top: 50%;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: linear-gradient(180deg, #6da6ff, #4c80ff);
  transform: translate(-50%, -50%);
}

.sites-stat-card__icon--check::after {
  left: 51%;
  top: 50%;
  width: 12px;
  height: 7px;
  border-left: 3px solid #fff;
  border-bottom: 3px solid #fff;
  transform: translate(-50%, -50%) rotate(-45deg);
}

.sites-stat-card__icon--users::before {
  left: 41%;
  top: 36%;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: linear-gradient(180deg, #75abff, #4b7fff);
  box-shadow: 15px 0 0 #6ca2ff;
  transform: translate(-50%, -50%);
}

.sites-stat-card__icon--users::after {
  left: 50%;
  top: 65%;
  width: 34px;
  height: 16px;
  border-radius: 16px 16px 10px 10px;
  background: linear-gradient(180deg, #7fb4ff, #5185ff);
  transform: translate(-50%, -50%);
}

.sites-list-card.ant-card {
  border-radius: 26px !important;
  overflow: hidden;
  border: 1px solid rgba(109, 145, 247, 0.18) !important;
}

.sites-list-card :deep(.ant-card-head) {
  min-height: 72px;
  padding: 0 20px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(246, 249, 255, 0.98));
}

.sites-list-card :deep(.ant-card-head-title) {
  padding: 20px 0;
  font-size: 22px;
  font-weight: 700;
  color: #274284;
}

.sites-list-card :deep(.ant-card-extra) {
  padding: 14px 0;
}

.sites-list-shell {
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.76), rgba(247, 250, 255, 0.96));
}

.sites-list-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.sites-list-toolbar__search {
  width: 260px;
}

.sites-list-toolbar__search :deep(.ant-input-affix-wrapper),
.sites-list-toolbar__search :deep(.ant-input) {
  border-radius: 12px !important;
  background: rgba(255, 255, 255, 0.85) !important;
}

.sites-list-toolbar__meta {
  color: #5a6fa9;
  font-size: 13px;
  font-weight: 600;
}

.table-fill--management {
  background: transparent;
}

.sites-page .table-fill--management :deep(.ant-table) {
  background: transparent !important;
}

.sites-page .table-fill--management :deep(.ant-table-container) {
  border-radius: 18px;
}

.sites-page .table-fill--management :deep(.ant-table-thead > tr > th) {
  height: 40px;
  padding: 0 14px !important;
  background: linear-gradient(180deg, #f1f5ff, #edf3ff) !important;
  color: #516cae !important;
  font-size: 13px;
}

.sites-page .table-fill--management :deep(.ant-table-selection-column) {
  width: 48px !important;
  min-width: 48px !important;
}

.sites-page .table-fill--management :deep(.ant-table-tbody > tr > td) {
  padding: 8px 14px !important;
  font-size: 13px;
  color: #2e4381;
  border-bottom: 1px solid rgba(104, 141, 241, 0.08);
  background: rgba(255, 255, 255, 0.88);
}

.sites-page .table-fill--management :deep(.ant-table-tbody > tr:hover > td) {
  background: linear-gradient(180deg, rgba(246, 249, 255, 0.96), rgba(241, 246, 255, 0.92)) !important;
}

.sites-page .table-fill--management :deep(.ant-pagination) {
  background: rgba(255, 255, 255, 0.9);
}

.site-table-cell {
  gap: 6px;
}

.site-name-cell strong {
  font-size: 14px;
  color: #253c79;
}

.site-name-open-btn.ant-btn {
  width: 24px;
  min-width: 24px;
  height: 24px;
  padding: 0;
  border-radius: 8px !important;
  color: var(--sites-accent) !important;
}

.site-subline {
  gap: 8px;
  min-width: 0;
  font-size: 12px;
  color: #8a9bc4;
}

.site-subline__label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.site-subline--secondary {
  color: #6e82b8;
}

.site-inline-badge.ant-tag {
  margin-inline-end: 0;
  padding: 0 8px;
}

.site-platform-tag {
  max-width: 100%;
}

.site-balance-cell {
  display: grid;
  gap: 4px;
}

.site-balance-cell__meta {
  font-size: 11px;
  color: #8ea0cb;
}

.site-package-cell {
  display: inline-flex;
  align-items: center;
  min-height: 26px;
  padding: 0 10px;
  border-radius: 999px;
  color: #4e648f;
  background: rgba(95, 136, 255, 0.08);
}

.site-package-cell--empty {
  color: #9cadcf;
  background: rgba(139, 156, 197, 0.12);
}

.site-empty-pill {
  display: inline-flex;
  align-items: center;
  height: 24px;
  padding: 0 10px;
  border-radius: 999px;
  color: #96a5c9;
  background: rgba(145, 159, 194, 0.14);
  font-size: 12px;
}

.site-group-text {
  color: #4a6296;
}

.site-actions-cell {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: nowrap;
  justify-content: flex-end;
}

.site-action-btn.ant-btn,
.site-actions-menu-button.ant-btn {
  height: 28px;
  padding: 0 10px;
  border-radius: 999px !important;
  background: rgba(255, 255, 255, 0.92) !important;
  border-color: rgba(130, 156, 232, 0.3) !important;
  color: #5e73a6 !important;
  box-shadow: none;
}

.site-action-btn--edit.ant-btn {
  color: #4a7dff !important;
  border-color: rgba(89, 134, 255, 0.34) !important;
  background: rgba(79, 124, 255, 0.08) !important;
}

.site-action-btn--danger.ant-btn {
  color: #ff6a6a !important;
  border-color: rgba(255, 126, 126, 0.28) !important;
  background: rgba(255, 108, 108, 0.07) !important;
}

.site-actions-menu-button.ant-btn {
  width: 28px;
  min-width: 28px;
  padding: 0;
}

.site-actions-menu :deep(.ant-dropdown-menu-item) {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sites-page :deep(.ant-switch) {
  min-width: 42px;
  height: 22px;
  line-height: 22px;
}

.sites-page :deep(.ant-switch.ant-switch-checked) {
  background: linear-gradient(135deg, #5a8dff, #3f72ff);
}

.sites-page :deep(.plugin-tag.ant-tag),
.sites-page :deep(.ant-tag) {
  font-size: 12px;
}

.sites-page :deep(.management-row--active > td:first-child::before) {
  background: linear-gradient(180deg, #7ab2ff, #3f72ff);
}

@media (max-width: 1400px) {
  .sites-toolbar {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 960px) {
  .sites-list-toolbar {
    align-items: stretch;
  }

  .sites-list-toolbar__search {
    width: min(100%, 360px);
  }

  .api-key-dialog-item__head,
  .api-key-dialog-item__paths,
  .manual-api-key-editor {
    grid-template-columns: 1fr;
  }

  .manual-api-key-editor__key,
  .manual-api-key-editor__urls,
  .manual-api-key-editor__generation,
  .manual-api-key-editor__edit,
  .manual-api-key-editor__action {
    grid-column: 1 / -1;
  }

  .manual-api-key-editor__action .api-key-dialog-field__label--spacer {
    display: none;
  }

  .api-key-dialog-item__head {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
