<script setup lang="ts">
import {
  PlusOutlined,
  ReloadOutlined,
  ExportOutlined,
  EditOutlined,
  ExperimentOutlined,
  CheckCircleOutlined,
  KeyOutlined,
  ShareAltOutlined,
  DollarCircleOutlined,
  DeleteOutlined,
  MoreOutlined,
} from '@ant-design/icons-vue'
import { Modal } from 'ant-design-vue'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch, type ComponentPublicInstance } from 'vue'
import {
  analyzeLocalStorage,
  convertCCSwitchSql,
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
import { balanceTone, formatBalance, formatGroupNames, normalizeGroupNames, parseGroupNames } from '../format'
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
})

const manualApiKeyForm = reactive({
  name: '',
  key: '',
  route_type: 'codex',
})

type SiteApiKeyEntry = {
  id: string
  name: string
  key: string
  status: string
  isPrimary: boolean
  source: string
  routeType: string
  isManual: boolean
}

function bindPageTableContainer(element: Element | ComponentPublicInstance | null) {
  pageTableContainer.value = element instanceof HTMLElement ? element : null
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
        unit = match[1]
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
  { title: '站点', key: 'site', width: 320, sorter: (a: Site, b: Site) => a.name.localeCompare(b.name, 'zh-CN') },
  { title: '插件', key: 'plugin', width: 150, sorter: (a: Site, b: Site) => pluginNameFor(a.plugin_key).localeCompare(pluginNameFor(b.plugin_key), 'zh-CN') },
  { title: 'API Key 数量', key: 'api_key_count', width: 170, sorter: (a: Site, b: Site) => siteApiKeyCount(a) - siteApiKeyCount(b) },
  { title: '余额', key: 'balance', width: 150, sorter: (a: Site, b: Site) => (a.last_balance ?? -Infinity) - (b.last_balance ?? -Infinity) },
  { title: '套餐', key: 'package', width: 160, sorter: (a: Site, b: Site) => String(a.package_display ?? '').localeCompare(String(b.package_display ?? ''), 'zh-CN') },
  { title: '签到状态', key: 'checkin_status', width: 120, sorter: (a: Site, b: Site) => String(visibleCheckinStatus(a) ?? '').localeCompare(String(visibleCheckinStatus(b) ?? ''), 'zh-CN') },
  { title: '分组', key: 'group', width: 120, sorter: (a: Site, b: Site) => String(a.group_name ?? '').localeCompare(String(b.group_name ?? ''), 'zh-CN') },
  { title: '连通状态', key: 'status', width: 120, sorter: (a: Site, b: Site) => String(a.connection_status ?? '').localeCompare(String(b.connection_status ?? ''), 'zh-CN') },
  { title: '启用', key: 'enabled', width: 90, sorter: (a: Site, b: Site) => Number(a.is_enabled) - Number(b.is_enabled) },
  { title: '可签到', key: 'participation', width: 100, sorter: (a: Site, b: Site) => Number(checkinMeta.value.get(b.id)?.include_in_checkin ?? false) - Number(checkinMeta.value.get(a.id)?.include_in_checkin ?? false) },
  { title: '操作', key: 'actions', width: 136, fixed: 'right' as const },
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
    sites.value = siteData
    siteGroups.value = groupData

    const nextSelected =
      preferredId !== null ? siteData.find((item) => item.id === preferredId) ?? siteData[0] ?? null : siteData[0] ?? null

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
  drawerOpen.value = true
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

function normalizeApiKeyRouteType(value: unknown): string {
  const normalized = String(value ?? '').trim().toLowerCase()
  if (normalized === 'claude' || normalized === 'anthropic') {
    return 'claude'
  }
  if (normalized === 'gemini' || normalized === 'google') {
    return 'gemini'
  }
  if (['codex', 'gpt', 'openai', 'chatgpt'].includes(normalized)) {
    return 'codex'
  }
  return ''
}

function defaultApiKeyRouteType(site: Pick<Site, 'plugin_config'>): string {
  const config = site.plugin_config as Record<string, unknown>
  return normalizeApiKeyRouteType(config?.gateway_route_type) || normalizeApiKeyRouteType(config?.api_format) || 'codex'
}

function apiKeyEntryValue(item: Record<string, unknown>, key: string): string {
  const value = String(item?.[key] ?? '').trim()
  return value === '<nil>' ? '' : value
}

function isManualApiKeyEntry(item: Record<string, unknown>): boolean {
  const source = apiKeyEntryValue(item, 'source').toLowerCase()
  return source === 'manual' || source === 'custom' || source === 'user' || item?.is_custom === true || item?.manual === true
}

function storedApiKeyEntries(credentials: Record<string, unknown> | undefined): Record<string, unknown>[] {
  const raw = credentials?.api_keys
  if (!Array.isArray(raw)) {
    return []
  }
  return raw
    .map((item) => (item && typeof item === 'object' ? { ...(item as Record<string, unknown>) } : null))
    .filter((item): item is Record<string, unknown> => Boolean(item && apiKeyEntryValue(item, 'key')))
}

function mergeApiKeyEntries(entries: Record<string, unknown>[]): Record<string, unknown>[] {
  const seen = new Set<string>()
  const out: Record<string, unknown>[] = []
  for (const entry of entries) {
    const key = apiKeyEntryValue(entry, 'key')
    if (!key || seen.has(key)) {
      continue
    }
    seen.add(key)
    out.push({ ...entry, key })
  }
  return out
}

function storedApiKeyEntriesForEdit(site: Pick<Site, 'credentials' | 'plugin_config'>): Record<string, unknown>[] {
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

function apiKeyValue(site: Pick<Site, 'credentials'>): string {
  const value = (site.credentials as Record<string, unknown>)?.api_key
  return typeof value === 'string' ? value.trim() : String(value ?? '').trim()
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
          name: apiKeyEntryValue(entry, 'name') || `Key ${index + 1}`,
          key,
          status: apiKeyEntryValue(entry, 'status') || 'unknown',
          isPrimary: Boolean(entry?.is_primary) || key === apiKeyValue(site),
          source,
          routeType,
          isManual: isManualApiKeyEntry(entry),
        }
      })
      .filter((item): item is SiteApiKeyEntry => Boolean(item))
  }

  const fallback = apiKeyValue(site)
  if (!fallback) {
    return []
  }
  return [
    {
      id: 'primary',
      name: '默认 Key',
      key: fallback,
      status: 'active',
      isPrimary: true,
      source: '',
      routeType: '',
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
  { label: 'GPT / Codex', value: 'codex' },
  { label: 'Claude', value: 'claude' },
  { label: 'Gemini', value: 'gemini' },
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
    target.package_unit = source.package_unit
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
    manualApiKeyForm.name = ''
    manualApiKeyForm.key = ''
    manualApiKeyForm.route_type = defaultApiKeyRouteType(fullSite)
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
  const name = manualApiKeyForm.name.trim() || `自定义 Key ${manualApiKeyEntries.value.length + 1}`
  const entry = {
    id: `manual-${Date.now()}`,
    name,
    key,
    status: 'active',
    source: 'manual',
    route_type: routeType,
    api_type: routeType,
  }
  upsertApiKeyDialogSiteCredentials((currentSite, credentials) => {
    const entries = storedApiKeyEntriesForEdit({ ...currentSite, credentials })
    const exists = entries.some((item) => apiKeyEntryValue(item, 'key') === key)
    if (exists) {
      toast.info('已存在相同 API Key，保存后同步接口条目会优先生效。')
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
}

function removeManualApiKey(entry: SiteApiKeyEntry) {
  if (!entry.isManual) {
    return
  }
  upsertApiKeyDialogSiteCredentials((_site, credentials) => {
    const next = storedApiKeyEntries(credentials).filter((item) => apiKeyEntryValue(item, 'key') !== entry.key)
    const credentialUpdates: Record<string, unknown> = {
      ...credentials,
      api_keys: next,
    }
    if (apiKeyValue({ credentials }) === entry.key) {
      credentialUpdates.api_key = apiKeyEntryValue(next[0] ?? {}, 'key')
    }
    return credentialUpdates
  })
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
      is_enabled: site.is_enabled,
      notes: site.notes,
      credentials: JSON.parse(JSON.stringify(site.credentials)),
      plugin_config: JSON.parse(JSON.stringify(site.plugin_config)),
    }
    payload.plugin_config.api_request_urls = normalizeStringList(apiKeyDialogForm.request_api_urls).join('\n')
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
  window.setTimeout(() => {
    if (localStorageRawText.value.trim()) {
      void handleAnalyzeLocalStorage()
    }
  }, 0)
}

async function ensureStorageAnalysisFinished() {
  const rawText = localStorageRawText.value.trim()
  if (storageAnalyzeTimer.value) {
    window.clearTimeout(storageAnalyzeTimer.value)
    storageAnalyzeTimer.value = null
  }
  if (drawerOpen.value && rawText && rawText !== lastAutoAnalyzedStorageRaw.value && isStorageJsonCandidate(rawText)) {
    await handleAnalyzeLocalStorage()
  }
  while (localStorageAnalyzeLoading.value) {
    await new Promise<void>((resolve) => window.setTimeout(() => resolve(), 50))
  }
}

function editorPayload(): SitePayload {
  return JSON.parse(JSON.stringify(editor))
}

function upsertSite(saved: Site) {
  const index = sites.value.findIndex((site) => site.id === saved.id)
  if (index >= 0) {
    sites.value[index] = saved
  } else {
    sites.value = [saved, ...sites.value]
  }
  selectedId.value = saved.id
  editingId.value = saved.id
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
      window.clearTimeout(storageAnalyzeTimer.value)
      storageAnalyzeTimer.value = null
    }
    const rawText = value.trim()
    if (!drawerOpen.value || !rawText || rawText === lastAutoAnalyzedStorageRaw.value || !isStorageJsonCandidate(rawText)) {
      return
    }
    storageAnalyzeTimer.value = window.setTimeout(() => {
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
  window.addEventListener('site-groups:changed', handleSiteGroupsChanged)
  await loadData(null)
  await loadCheckinExtras()
  scheduleSummaryRefresh()
})

onBeforeUnmount(() => {
  window.removeEventListener('site-groups:changed', handleSiteGroupsChanged)
  if (storageAnalyzeTimer.value) {
    window.clearTimeout(storageAnalyzeTimer.value)
  }
})
</script>

<template>
  <ShellLayout>
    <div class="page-stack page-stack--dashboard">
      <div class="page-toolbar page-toolbar--actions">
        <div>
          <a-space>
            <a-button @click="checkinConfigOpen = true">签到配置</a-button>
            <a-button @click="checkinLogsOpen = true">最近执行</a-button>
            <a-button type="primary" :loading="busy" :disabled="!includedCheckinCount" @click="handleCheckinAllIncluded">
              {{ checkinAllIncludedLabel }}
            </a-button>
          </a-space>
        </div>
        <a-space>
          <a-button :loading="busy" @click="handleRefresh(selectedId)">
            <template #icon>
              <ReloadOutlined />
            </template>
            刷新
          </a-button>
          <a-button :loading="busy" @click="handleConnectivitySweep">{{ connectivitySweepLabel }}</a-button>
          <a-button :loading="inviteRefreshAllLoading" @click="refreshAllInvites">
            <template #icon>
              <ShareAltOutlined />
            </template>
            {{ inviteRefreshAllLabel }}
          </a-button>
          <a-button :loading="apiKeyRefreshAllLoading" @click="refreshAllApiKeys">
            <template #icon>
              <KeyOutlined />
            </template>
            {{ apiKeyRefreshAllLabel }}
          </a-button>
          <a-button :loading="duplicateCheckLoading" @click="handleDuplicateCheck">清理检测</a-button>
          <a-button :loading="ccSwitchExportLoading" @click="openCCSwitchConfig('import')">供应商配置</a-button>
          <a-button type="primary" @click="openCreateDrawer">
            <template #icon>
              <PlusOutlined />
            </template>
            新建站点
          </a-button>
        </a-space>
      </div>

      <input
        ref="ccSwitchFileInput"
        type="file"
        accept=".json,.sql,application/json,text/plain"
        style="display: none"
        @change="handleCCSwitchFileChange"
      >

      <a-row :gutter="[16, 16]">
        <a-col :xs="24" :sm="12" :xl="8" :xxl="4">
          <a-card :bordered="false" class="admin-card stat-card stat-card--signal">
            <a-statistic title="站点总数" :value="sites.length" />
          </a-card>
        </a-col>
        <a-col :xs="24" :sm="12" :xl="8" :xxl="4">
          <a-card :bordered="false" class="admin-card stat-card stat-card--signal">
            <a-statistic title="已启用" :value="enabledSiteCount" />
          </a-card>
        </a-col>
        <a-col :xs="24" :sm="12" :xl="8" :xxl="4">
          <a-card :bordered="false" class="admin-card stat-card stat-card--signal">
            <a-statistic title="网关就绪" :value="readyGatewayCount" />
          </a-card>
        </a-col>
        <a-col :xs="24" :sm="12" :xl="8" :xxl="4">
          <a-card :bordered="false" class="admin-card stat-card stat-card--signal">
            <div class="stat-balance-total">
              <div class="ant-statistic-title">余额合计</div>
              <div class="stat-balance-line">
                <span :class="['stat-balance-amount', `stat-balance-amount--${totalBalanceTone}`]">
                  {{ totalBalanceSummary }}
                </span>
                <span class="stat-balance-count">{{ quantifiedBalanceSiteCount }} 个站点</span>
              </div>
            </div>
          </a-card>
        </a-col>
        <a-col :xs="24" :sm="12" :xl="8" :xxl="4">
          <a-card :bordered="false" class="admin-card stat-card stat-card--signal">
            <a-statistic title="状态成功" :value="successSiteCount" />
          </a-card>
        </a-col>
        <a-col :xs="24" :sm="12" :xl="8" :xxl="4">
          <a-card :bordered="false" class="admin-card stat-card stat-card--signal">
            <a-statistic title="失败 / 未测" :value="`${failedSiteCount} / ${pendingSiteCount}`" />
          </a-card>
        </a-col>
      </a-row>

      <a-card :bordered="false" class="admin-card admin-card--fill site-list-card">
        <template #title>站点列表</template>
        <template #extra>
          <a-space>
            <a-input
              v-model:value="siteSearch"
              allow-clear
              placeholder="搜索站点 / 域名 / 分组"
              style="width: 240px"
            />
            <span>{{ groupedSiteCount }} 个已分组</span>
            <span>已选签到 {{ selectedCheckinIds.length }} 个</span>
            <a-button
              type="primary"
              :disabled="!selectedCheckinIds.length || busy"
              @click="handleCheckinSelected"
            >
              {{ checkinBatchProgress ? `签到中 ${checkinBatchProgress.done}/${checkinBatchProgress.total}` : '签到选中' }}
            </a-button>
            <a-button :disabled="!selectedCheckinIds.length" @click="selectedCheckinIds = []">清空选择</a-button>
          </a-space>
        </template>

        <div class="card-shell">
          <div :ref="bindPageTableContainer" class="table-fill table-fill--management">
            <a-table
              :columns="siteColumns"
              :data-source="filteredSites"
              :pagination="{ pageSize: tablePageSize }"
              :row-key="rowKey"
              :row-selection="checkinRowSelection"
              size="middle"
              :custom-row="siteCustomRow"
              :row-class-name="siteRowClassName"
              :scroll="{ x: 1530, y: pageTableY }"
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
                  </div>
                </template>
                <template v-else-if="column.key === 'plugin'">
                  <PluginTag :plugin-key="record.plugin_key" :label="displayPluginLabel(asSite(record))" />
                </template>
                <template v-else-if="column.key === 'api_key_count'">
                  <a-tag :color="siteApiKeyCountTagColor(asSite(record))">
                    {{ siteApiKeyCountLabel(asSite(record)) }}
                  </a-tag>
                </template>
                <template v-else-if="column.key === 'balance'">
                  <span :class="balanceClass(record.last_balance)">
                    {{ record.balance_display || '暂无' }}
                  </span>
                </template>
                <template v-else-if="column.key === 'package'">
                  <a-tooltip v-if="record.package_display" :title="record.package_display">
                    <span class="site-package-cell">{{ record.package_display }}</span>
                  </a-tooltip>
                  <span v-else class="site-package-cell site-package-cell--empty">暂无</span>
                </template>
                <template v-else-if="column.key === 'checkin_status'">
                  <StatusPill v-if="visibleCheckinStatus(asSite(record))" :value="visibleCheckinStatus(asSite(record))" />
                </template>
                <template v-else-if="column.key === 'group'">
                  <span>{{ displayGroupName(asSite(record)) }}</span>
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
                  <a-space size="small" class="site-actions-cell">
                    <a-button size="small" type="primary" ghost @click.stop="openEditDrawer(asSite(record))">
                      <template #icon><EditOutlined /></template>
                      编辑
                    </a-button>
                    <a-dropdown :trigger="['click']">
                      <a-tooltip title="更多操作">
                        <a-button
                          size="small"
                          class="site-actions-menu-button"
                          :loading="isInviteLoading(asSite(record).id) || isBalanceProbing(asSite(record).id) || isApiKeyRefreshing(asSite(record).id)"
                          @click.stop
                        >
                          <template #icon><MoreOutlined /></template>
                        </a-button>
                      </a-tooltip>
                      <template #overlay>
                        <a-menu @click.stop>
                          <a-menu-item key="test" @click="handleTest(asSite(record))">
                            <ExperimentOutlined />
                            <span>{{ isRelayOnlySitePayload(asSite(record)) ? '验证出口' : '测试连接' }}</span>
                          </a-menu-item>
                          <a-menu-item
                            v-if="siteCanCheckin(asSite(record)) && !isRelayOnlySitePayload(asSite(record))"
                            key="checkin"
                            @click="handleCheckin(asSite(record))"
                          >
                            <CheckCircleOutlined />
                            <span>{{ siteCheckinActionLabel(asSite(record)) }}</span>
                          </a-menu-item>
                          <a-menu-item key="api-key" @click="openApiKeyDialog(asSite(record))">
                            <KeyOutlined />
                            <span>API Key</span>
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
                          <a-menu-item
                            key="balance"
                            :disabled="isBalanceProbing(asSite(record).id)"
                            @click="handleProbeSiteBalance(asSite(record))"
                          >
                            <DollarCircleOutlined />
                            <span>{{ isBalanceProbing(asSite(record).id) ? '余额读取中' : '读取余额' }}</span>
                          </a-menu-item>
                          <a-menu-divider />
                          <a-menu-item key="delete" danger @click="handleDelete(asSite(record))">
                            <DeleteOutlined />
                            <span>删除站点</span>
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

      <a-drawer
        v-model:open="drawerOpen"
        :title="editingId ? `编辑站点 · ${editingSite?.name ?? ''}` : '新建站点'"
        width="min(1180px, 100vw)"
        placement="right"
        :destroy-on-close="false"
      >
        <div class="drawer-form-shell">
          <a-alert
            v-if="saveFeedback"
            type="success"
            show-icon
            :message="saveFeedback"
            style="margin-bottom: 16px"
          />

          <a-alert
            v-if="pluginMismatch && recommendedPlugin"
            type="warning"
            show-icon
            style="margin-bottom: 16px"
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
            style="margin-bottom: 16px"
          >
            <template #description>
              <pre class="feedback-pre">{{ testFeedback.message }}</pre>
            </template>
          </a-alert>

          <a-form layout="vertical">
            <a-row :gutter="[18, 18]" class="drawer-editor-grid">
              <a-col :xs="24" :xl="13">
                <div class="drawer-editor-column">
                  <div class="form-block form-block--elevated">
                    <div class="form-block__head compact">
                      <div>
                        <h3>基础信息</h3>
                      </div>
                    </div>

                    <a-row :gutter="16">
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
                    </a-row>

                    <a-row :gutter="16">
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
                      <a-textarea v-model:value="editor.notes" :rows="3" />
                    </a-form-item>

                    <a-form-item label="启用状态">
                      <a-switch v-model:checked="editor.is_enabled" checked-children="启用" un-checked-children="停用" />
                    </a-form-item>
                  </div>

                  <div class="form-block form-block--elevated">
                    <div class="form-block__head">
                      <div>
                        <h3>浏览器存储导入</h3>
                        <p>在目标站点控制台运行采集脚本，粘贴输出后自动识别插件类型并回填账号凭证。</p>
                      </div>
                      <a-space wrap align="start">
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

                    <a-row :gutter="16">
                      <a-col :xs="24" :lg="12">
                        <a-form-item label="控制台函数">
                          <a-textarea
                            :value="consoleCollectorScript"
                            :rows="8"
                            readonly
                          />
                          <small class="field-help">复制后到目标站点控制台执行；脚本会尝试把结果写入剪贴板，并输出 URL、可读 Cookie、localStorage、sessionStorage 和可识别 token payload。</small>
                        </a-form-item>
                      </a-col>
                      <a-col :xs="24" :lg="12">
                        <a-form-item label="脚本输出 JSON">
                          <a-textarea
                            v-model:value="localStorageRawText"
                            :rows="8"
                            placeholder="粘贴控制台输出的 JSON；粘贴后会自动分析。"
                            @paste="handleStoragePayloadPaste"
                          />
                          <small class="field-help">系统会自动切换插件类型，再把 access_token、refresh_token、邮箱、用户 ID、User-Agent 等写入当前插件支持的字段。</small>
                        </a-form-item>
                      </a-col>
                    </a-row>
                  </div>

                  <div v-if="currentPlugin" class="form-block form-block--elevated">
                    <div class="form-block__head">
                      <div>
                        <h3>账号凭证</h3>
                        <p>{{ currentPlugin.auth_hint || '可先在站点侧完成登录，再回到后台回填最终凭证。' }}</p>
                      </div>
                      <a-space wrap align="start">
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

                    <a-row :gutter="16">
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

                    <div v-if="manualLoginFields.length" class="nested-form-block">
                      <div class="form-block__head compact">
                        <div>
                          <h3>账号密码</h3>
                        </div>
                      </div>
                      <a-row :gutter="16">
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

                    <div v-if="totpCredentialFields.length" class="nested-form-block">
                      <div class="form-block__head compact">
                        <div>
                          <h3>双重验证</h3>
                        </div>
                        <a-button
                          v-if="editingId"
                          :loading="totpPreviewLoading"
                          @click="handlePreviewTotp"
                        >
                          查看当前验证码
                        </a-button>
                      </div>
                      <a-row :gutter="16">
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
                </div>
              </a-col>

              <a-col :xs="24" :xl="11">
                <div class="drawer-editor-column">
                  <div v-if="currentPlugin" class="form-block form-block--elevated form-block--sticky">
                    <div class="form-block__head compact">
                      <div>
                        <h3>插件配置</h3>
                      </div>
                    </div>
                    <a-alert
                      v-if="editor.plugin_key === 'api-supplier'"
                      type="info"
                      show-icon
                      message="此站点只用于网关转发，不参与签到 / 资料同步。"
                      description="api_format 推荐填 openai / anthropic / gemini / general（写错只会影响路由分类，不会影响转发）。Base URL 与 API Key 是必填项。"
                      style="margin: 0 8px 12px;"
                    />
                    <a-row :gutter="16">
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
                </div>
              </a-col>
            </a-row>
          </a-form>
        </div>

        <template #footer>
          <div class="drawer-footer">
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
                {{ editingSite ? '保存修改' : '创建站点' }}
              </a-button>
              <a-button v-if="editingSite" danger :loading="busy" @click="handleDelete(editingSite)">删除站点</a-button>
            </a-space>
          </div>
        </template>
      </a-drawer>

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
                <span class="table-subtitle">接口同步结果会保留，自定义 Key 可在这里添加或删除。</span>
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
                      <a-tag :color="entry.status === 'active' ? 'green' : 'default'">{{ entry.status }}</a-tag>
                    </a-space>
                    <a-space size="small">
                      <a-button size="small" @click="copyApiKeyFromDialog(entry.key)">复制</a-button>
                      <a-button
                        v-if="entry.isManual"
                        size="small"
                        danger
                        @click="removeManualApiKey(entry)"
                      >
                        删除
                      </a-button>
                    </a-space>
                  </div>
                  <a-input-password
                    :value="entry.key"
                    readonly
                    visibility-toggle
                    placeholder="当前 API Key 为空"
                  />
                </div>
              </div>
              <a-empty v-else description="当前站点未配置 API Key" />
            </a-space>
          </a-form-item>
          <a-form-item label="添加自定义 API Key">
            <div class="manual-api-key-editor">
              <a-input
                v-model:value="manualApiKeyForm.name"
                placeholder="名称，例如 Claude 备用"
              />
              <a-select
                v-model:value="manualApiKeyForm.route_type"
                :options="apiKeyRouteTypeOptions"
              />
              <a-input-password
                v-model:value="manualApiKeyForm.key"
                placeholder="sk-..."
                autocomplete="new-password"
                @press-enter="addManualApiKey"
              />
              <a-button type="primary" @click="addManualApiKey">添加</a-button>
            </div>
            <small class="field-help">保存后自定义 Key 会与接口同步 Key 同时存在；如果值相同，下次同步时保留接口返回条目。</small>
          </a-form-item>
          <a-form-item label="请求 API URL 列表">
            <a-textarea
              v-model:value="apiKeyDialogForm.request_api_urls"
              :rows="8"
              placeholder="每行一个 URL。网关会按顺序回退，全部失败后再轮转下一个路由。"
            />
            <small class="field-help">留空时会继续使用当前默认出口或站点 Base URL。</small>
          </a-form-item>
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
