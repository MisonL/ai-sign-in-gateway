import { clearToken, getToken, setToken } from './session'
import type {
  AdminUser,
  BalanceProbeResult,
  CCSwitchExportResult,
  CCSwitchImportResult,
  CCSwitchSqlConvertResult,
  CheckinSite,
  ChatResult,
  CheckinRun,
  ConnectivityResult,
  DuplicateSiteGroup,
  DuplicateSiteMergeResult,
  GatewayLog,
  GatewayOverview,
  GatewayRouteProbeResult,
  GatewayRoute,
  GatewaySettingsData,
  LocalStorageAnalyzeResult,
  McpTestResult,
  OverviewData,
  PluginMeta,
  PublicInvite,
  QueueTask,
  RuntimeConfigDirResult,
  RuntimeDatabaseBackupNowResult,
  RuntimeDatabaseBackupsResult,
  RuntimeDatabaseImportResult,
  RuntimeStopStalePortsResult,
  SettingsData,
  Site,
  SiteGroup,
  SiteHealth,
  SiteInviteRefreshResult,
  SitePayload,
  SiteSummary,
  TotpPreview,
} from './types'

const API_BASE = import.meta.env.VITE_API_BASE || '/api'

export class ApiError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
}

function extractErrorMessage(data: unknown): string {
  if (typeof data !== 'object' || data === null) {
    return '请求失败'
  }

  const candidate = data as { detail?: unknown; message?: unknown }
  const value = candidate.detail ?? candidate.message
  if (typeof value === 'string') {
    return value
  }
  if (value && typeof value === 'object') {
    try {
      return JSON.stringify(value, null, 2)
    } catch {
      return '请求失败'
    }
  }
  return '请求失败'
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Content-Type', 'application/json')

  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  const response = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers,
  })

  if (response.status === 401) {
    clearToken()
    throw new ApiError('登录状态失效，请重新登录。', 401)
  }

  const text = await response.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = { message: text }
    }
  }

  if (!response.ok) {
    throw new ApiError(extractErrorMessage(data), response.status)
  }

  return data as T
}

async function requestForm<T>(path: string, body: FormData): Promise<T> {
  const headers = new Headers()

  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  const response = await fetch(`${API_BASE}${path}`, {
    method: 'POST',
    headers,
    body,
  })

  if (response.status === 401) {
    clearToken()
    throw new ApiError('登录状态失效，请重新登录。', 401)
  }

  const text = await response.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = { message: text }
    }
  }

  if (!response.ok) {
    throw new ApiError(extractErrorMessage(data), response.status)
  }

  return data as T
}

export async function login(username: string, password: string): Promise<void> {
  const data = await request<{ access_token: string }>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
  setToken(data.access_token)
}

export function logout(): void {
  clearToken()
}

export function getMe(): Promise<AdminUser> {
  return request('/auth/me')
}

export function getPublicInvites(): Promise<PublicInvite[]> {
  return request('/public/invites')
}

export interface AdminAccountUpdatePayload {
  current_password: string
  new_username?: string
  new_password?: string
}

export interface AdminAccountUpdateResult {
  user: AdminUser
  access_token: string
  token_type: string
}

export async function updateAdminAccount(
  payload: AdminAccountUpdatePayload,
): Promise<AdminAccountUpdateResult> {
  const result = await request<AdminAccountUpdateResult>('/auth/account', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
  if (result?.access_token) {
    setToken(result.access_token)
  }
  return result
}

export function getOverview(): Promise<OverviewData> {
  return request('/overview')
}

export function getPlugins(): Promise<PluginMeta[]> {
  return request('/plugins')
}

export function getSites(): Promise<Site[]> {
  return request('/sites')
}

export function getDuplicateSites(): Promise<DuplicateSiteGroup[]> {
  return request('/sites/cleanup-duplicates')
}

export function mergeDuplicateSites(): Promise<DuplicateSiteMergeResult> {
  return request('/sites/cleanup-duplicates/merge', {
    method: 'POST',
  })
}

export function refreshSiteSummaries(payload: { site_ids?: number[]; only_enabled?: boolean } = {}): Promise<SiteSummary[]> {
  return request('/sites/refresh-summaries', {
    method: 'POST',
    body: JSON.stringify({
      site_ids: payload.site_ids ?? [],
      only_enabled: payload.only_enabled ?? false,
    }),
  })
}

export function refreshSiteInvites(payload: { site_ids?: number[]; only_enabled?: boolean } = {}): Promise<SiteInviteRefreshResult[]> {
  return request('/sites/invites/refresh', {
    method: 'POST',
    body: JSON.stringify({
      site_ids: payload.site_ids ?? [],
      only_enabled: payload.only_enabled ?? false,
    }),
  })
}

export function importCCSwitchConfig(
  payload: Record<string, unknown>,
  options: { sectionKeys?: string[] } = {},
): Promise<CCSwitchImportResult> {
  return request('/sites/cc-switch/import', {
    method: 'POST',
    body: JSON.stringify({
      payload,
      section_keys: options.sectionKeys ?? [],
    }),
  })
}

export function convertCCSwitchSql(sqlText: string): Promise<CCSwitchSqlConvertResult> {
  return request('/sites/cc-switch/sql/convert', {
    method: 'POST',
    body: JSON.stringify({
      sql_text: sqlText,
    }),
  })
}

export function importCCSwitchSql(
  sqlText: string,
  options: { sectionKeys?: string[] } = {},
): Promise<CCSwitchImportResult> {
  return request('/sites/cc-switch/sql/import', {
    method: 'POST',
    body: JSON.stringify({
      sql_text: sqlText,
      section_keys: options.sectionKeys ?? [],
    }),
  })
}

export function exportCCSwitchConfig(options: { site_ids?: number[]; only_enabled?: boolean } = {}): Promise<CCSwitchExportResult> {
  return request('/sites/cc-switch/export', {
    method: 'POST',
    body: JSON.stringify({
      site_ids: options.site_ids ?? [],
      only_enabled: options.only_enabled ?? false,
    }),
  })
}

export function createSite(payload: SitePayload): Promise<Site> {
  return request('/sites', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateSite(id: number, payload: SitePayload): Promise<Site> {
  return request(`/sites/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export function toggleSite(id: number): Promise<Site> {
  return request(`/sites/${id}/toggle`, {
    method: 'POST',
  })
}

export function deleteSite(id: number): Promise<void> {
  return request(`/sites/${id}`, {
    method: 'DELETE',
  })
}

export function testSite(id: number): Promise<SiteHealth> {
  return request(`/sites/${id}/test`, {
    method: 'POST',
  })
}

export function probeSiteBalance(id: number): Promise<BalanceProbeResult> {
  return request(`/sites/${id}/balance-probe`, {
    method: 'POST',
  })
}

export function testSiteDraft(payload: SitePayload & { site_id?: number | null }): Promise<SiteHealth> {
  return request('/sites/test-draft', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function analyzeLocalStorage(raw_text: string): Promise<LocalStorageAnalyzeResult> {
  return request('/sites/storage/analyze', {
    method: 'POST',
    body: JSON.stringify({ raw_text }),
  })
}

export function getSiteGroups(): Promise<SiteGroup[]> {
  return request('/sites/groups')
}

export function createSiteGroup(name: string): Promise<SiteGroup> {
  return request('/sites/groups', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}

export function renameSiteGroup(old_name: string, new_name: string): Promise<SiteGroup> {
  return request('/sites/groups', {
    method: 'PUT',
    body: JSON.stringify({ old_name, new_name }),
  })
}

export function deleteSiteGroup(name: string): Promise<{ status: string; message: string }> {
  return request('/sites/groups', {
    method: 'DELETE',
    body: JSON.stringify({ name }),
  })
}

export function getSiteQueue(id: number): Promise<QueueTask[]> {
  return request(`/sites/${id}/queue`)
}

export function activateSiteQueueTask(id: number, taskKey: string, message = ''): Promise<QueueTask> {
  return request(`/sites/${id}/queue/${taskKey}/activate`, {
    method: 'POST',
    body: JSON.stringify({ message }),
  })
}

export function previewSiteTotp(id: number): Promise<TotpPreview> {
  return request(`/sites/${id}/totp-preview`)
}

export function runSiteCheckin(
  id: number,
): Promise<{ run: number; status: string; message: string; balance: number | null; balance_unit?: string | null }> {
  return request(`/sites/${id}/checkin`, {
    method: 'POST',
  })
}

export function getRuns(limit = 50): Promise<CheckinRun[]> {
  return request(`/checkins/runs?limit=${limit}`)
}

export function getCheckinSites(): Promise<CheckinSite[]> {
  return request('/checkins/sites')
}

export function updateCheckinParticipation(
  id: number,
  includeInCheckin: boolean,
): Promise<CheckinSite> {
  return request(`/checkins/sites/${id}/participation`, {
    method: 'POST',
    body: JSON.stringify({ include_in_checkin: includeInCheckin }),
  })
}

export function runBatch(siteIds: number[] = [], onlyEnabled = true): Promise<CheckinRun[]> {
  return request('/checkins/batch', {
    method: 'POST',
    body: JSON.stringify({ site_ids: siteIds, only_enabled: onlyEnabled }),
  })
}

export function getSettings(): Promise<SettingsData> {
  return request('/settings')
}

export function updateSettings(payload: SettingsData): Promise<SettingsData> {
  return request('/settings', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export function runSchedulerNow(): Promise<{ status: string; message: string }> {
  return request('/settings/scheduler/run-now', {
    method: 'POST',
  })
}

export function openRuntimeUrl(url: string): Promise<{ status: string; message: string }> {
  return request('/settings/runtime/open-url', {
    method: 'POST',
    body: JSON.stringify({ url }),
  })
}

export function stopStaleRuntimePorts(): Promise<RuntimeStopStalePortsResult> {
  return request('/settings/runtime/stop-stale-ports', {
    method: 'POST',
  })
}

export function setRuntimeConfigDir(configDir: string): Promise<RuntimeConfigDirResult> {
  return request('/settings/runtime/config-dir', {
    method: 'POST',
    body: JSON.stringify({ config_dir: configDir }),
  })
}

export function importRuntimeDatabase(databasePath: string): Promise<RuntimeDatabaseImportResult> {
  return request('/settings/runtime/database', {
    method: 'POST',
    body: JSON.stringify({ database_path: databasePath }),
  })
}

export function uploadRuntimeDatabase(file: File): Promise<RuntimeDatabaseImportResult> {
  const form = new FormData()
  form.append('database', file)
  return requestForm('/settings/runtime/database', form)
}

export function getRuntimeDatabaseBackups(): Promise<RuntimeDatabaseBackupsResult> {
  return request('/settings/runtime/database/backups')
}

export function backupRuntimeDatabaseNow(): Promise<RuntimeDatabaseBackupNowResult> {
  return request('/settings/runtime/database/backups', {
    method: 'POST',
  })
}

export function deleteRuntimeDatabaseBackup(name: string): Promise<RuntimeDatabaseBackupsResult> {
  return request(`/settings/runtime/database/backups/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  })
}

export function testConnectivity(payload: {
  base_url: string
  api_key: string
  model: string
}): Promise<ConnectivityResult> {
  return request('/tools/connectivity-test', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function testChat(payload: {
  base_url: string
  api_key: string
  model: string
  prompt: string
}): Promise<ChatResult> {
  return request('/tools/chat-test', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function testMcp(payload: {
  base_url: string
  api_key: string
  model: string
  prompt: string
  server_label: string
  server_url: string
  allowed_tools: string[]
  require_approval: 'never' | 'always'
}): Promise<McpTestResult> {
  return request('/tools/mcp-test', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function getGatewayOverview(): Promise<GatewayOverview> {
  return request('/gateway-admin/overview')
}

export function getGatewaySettings(): Promise<GatewaySettingsData> {
  return request('/gateway-admin/settings')
}

export function updateGatewaySettings(payload: GatewaySettingsData): Promise<GatewaySettingsData> {
  return request('/gateway-admin/settings', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export function syncGatewayRoutes(): Promise<{ status: string; route_count: number }> {
  return request('/gateway-admin/sync', {
    method: 'POST',
  })
}

export function getGatewayRoutes(options?: { group?: string; includeDisabled?: boolean }): Promise<GatewayRoute[]> {
  const params = new URLSearchParams()
  if (options?.group) {
    params.set('group', options.group)
  }
  if (options?.includeDisabled !== undefined) {
    params.set('include_disabled', String(options.includeDisabled))
  }
  const suffix = params.toString() ? `?${params.toString()}` : ''
  return request(`/gateway-admin/routes${suffix}`)
}

export function toggleGatewayRoute(id: number): Promise<{ id: number; is_enabled: boolean; circuit_state: string }> {
  return request(`/gateway-admin/routes/${id}/toggle`, {
    method: 'POST',
  })
}

export function resetGatewayRouteCircuit(id: number): Promise<{ id: number; is_enabled: boolean; circuit_state: string }> {
  return request(`/gateway-admin/routes/${id}/reset-circuit`, {
    method: 'POST',
  })
}

export function updateGatewayRouteType(id: number, routeType: 'claude' | 'codex' | 'gemini'): Promise<GatewayRoute> {
  return request(`/gateway-admin/routes/${id}/type`, {
    method: 'PATCH',
    body: JSON.stringify({ route_type: routeType }),
  })
}

export function probeGatewayRoute(id: number): Promise<GatewayRouteProbeResult> {
  return request(`/gateway-admin/routes/${id}/probe`, {
    method: 'POST',
  })
}

export function probeGatewayRouteBalance(id: number): Promise<BalanceProbeResult> {
  return request(`/gateway-admin/routes/${id}/balance-probe`, {
    method: 'POST',
  })
}

export function probeGatewayRoutes(routeIds: number[]): Promise<GatewayRouteProbeResult[]> {
  return request('/gateway-admin/routes/probe', {
    method: 'POST',
    body: JSON.stringify({ route_ids: routeIds }),
  })
}

export function getGatewayLogs(limit = 80): Promise<GatewayLog[]> {
  return request(`/gateway-admin/logs?limit=${limit}`)
}

export function getGatewayRouteLogs(routeId: number, limit = 80): Promise<GatewayLog[]> {
  return request(`/gateway-admin/routes/${routeId}/logs?limit=${limit}`)
}
