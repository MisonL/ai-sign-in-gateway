export interface AdminUser {
  id: number
  username: string
}

export interface FieldDescriptor {
  name: string
  label: string
  type: 'text' | 'password' | 'textarea' | 'number' | 'url'
  placeholder: string
  required: boolean
  help_text: string
}

export interface PluginMeta {
  key: string
  name: string
  description: string
  credential_fields: FieldDescriptor[]
  config_fields: FieldDescriptor[]
  capabilities: string[]
  auth_entry_path: string
  auth_entry_label: string
  auth_hint: string
}

export interface SitePayload {
  name: string
  base_url: string
  plugin_key: string
  group_name: string
  is_enabled: boolean
  notes: string
  credentials: Record<string, string>
  plugin_config: Record<string, string | number | boolean>
}

export interface Site extends SitePayload {
  id: number
  last_status: string | null
  connection_status?: string | null
  last_message: string | null
  last_balance: number | null
  balance_display?: string | null
  package_display?: string | null
  checkin_status?: string | null
  last_run_at: string | null
  created_at: string
  updated_at: string | null
}

export interface SiteSummary {
  site_id: number
  last_status: string | null
  connection_status?: string | null
  last_message: string | null
  last_balance: number | null
  balance_display?: string | null
  package_display?: string | null
  invite_link?: string | null
  invite_code?: string | null
  checkin_status?: string | null
  last_run_at: string | null
}

export interface SiteInviteRefreshResult {
  site_id: number
  ok: boolean
  message: string
  invite_link?: string | null
  invite_code?: string | null
  package_display?: string | null
  updated_credentials: Record<string, string>
  updated_plugin_config: Record<string, string | number | boolean>
}

export interface DuplicateSiteItem {
  id: number
  name: string
  plugin_key: string
  is_enabled: boolean
  notes: string
  plugin_config_count: number
  credentials_count: number
  suggested_keep: boolean
}

export interface DuplicateSiteGroup {
  base_url: string
  account: string
  password_present: boolean
  suggested_keep_id: number
  site_ids: number[]
  sites: DuplicateSiteItem[]
}

export interface DuplicateSiteMergeResult {
  merged_group_count: number
  deleted_site_count: number
  remaining_group_count: number
  kept_site_ids: number[]
  deleted_site_ids: number[]
}

export interface CCSwitchImportResult {
  created: number
  updated: number
  deleted: number
  skipped: number
  imported_site_ids: number[]
  messages: string[]
}

export interface CCSwitchSqlConvertResult {
  payload: Record<string, unknown>
  provider_count: number
}

export interface CCSwitchExportResult {
  site_count: number
  payload: Record<string, unknown>
}

export interface SiteHealth {
  site_id: number
  logged_in: boolean
  message: string
  balance: number | null
  balance_unit?: string | null
  package_display?: string | null
  account_name: string | null
  invite_link?: string | null
  invite_code?: string | null
  updated_credentials?: Record<string, string>
  updated_plugin_config?: Record<string, string | number | boolean>
}

export interface PublicInvite {
  site_id: number
  site_name: string
  base_url: string
  group_name: string
  plugin_key: string
  invite_link: string
  invite_code: string
  package_name?: string | null
}

export interface BalanceProbeResult {
  site_id: number
  route_id: number
  ok: boolean
  status_code: number | null
  latency_ms: number | null
  remaining: number | null
  unit: string
  base_url: string
  message: string
  checked_at: string
  last_balance: number | null
}

export interface LocalStorageAnalyzeResult {
  parsed_items: number
  page_url: string
  page_title: string
  cookie_header: string
  local_storage: Record<string, string>
  session_storage: Record<string, string>
  suggested_credentials: Record<string, string>
  suggested_plugin_key?: string
  suggested_site_name?: string
  suggested_base_url?: string
  suggested_plugin_config?: Record<string, string | number | boolean>
  matched_keys: string[]
  message: string
}

export interface SiteGroup {
  name: string
  site_count: number
  in_catalog: boolean
  in_use: boolean
}

export interface QueueTask {
  id: number
  task_key: string
  title: string
  detail: string
  status: string
  sort_order: number
  action_key: string
  action_label: string
  last_message: string | null
  last_error: string | null
  completed_at: string | null
  updated_at: string | null
}

export interface TotpPreview {
  code: string
  expires_in: number
}

export interface CheckinSite {
  id: number
  name: string
  plugin_key: string
  group_name: string
  base_url: string
  is_enabled: boolean
  can_checkin: boolean
  include_in_checkin: boolean
  checkin_label: string
  reason: string
  last_status: string | null
  connection_status?: string | null
  last_message: string | null
  last_balance?: number | null
  balance_display?: string | null
  package_display?: string | null
  checkin_status?: string | null
  last_run_at: string | null
}

export interface CheckinRun {
  id: number
  site_id: number | null
  site_name: string | null
  trigger_type: string
  status: string
  message: string
  response_excerpt: string | null
  balance: number | null
  balance_unit?: string | null
  attempt_count: number
  started_at: string
  finished_at: string | null
}

export interface OverviewAttentionSite {
  id: number
  name: string
  last_status: string | null
  last_message: string | null
  last_run_at: string | null
}

export interface OverviewData {
  site_count: number
  enabled_site_count: number
  today_success: number
  today_failed: number
  next_run_at: string | null
  latest_sync: string | null
  recent_runs: CheckinRun[]
  attention_sites: OverviewAttentionSite[]
}

export interface SettingsData {
  timezone: string
  schedule_enabled: boolean
  daily_run_time: string
  checkin_concurrency: number
  checkin_global_concurrency: number
  checkin_interval_seconds: number
  retry_count: number
  request_timeout: number
  only_enabled_sites: boolean
  desktop_keep_running: boolean
  database_backup_enabled: boolean
  database_backup_dir: string
  database_backup_interval_minutes: number
  database_backup_retention: number
  desktop_frontend_default_port: number
  desktop_frontend_port: number
  desktop_frontend_url: string
  desktop_frontend_default_port_occupant: string
  desktop_backend_default_port: number
  desktop_backend_port: number
  desktop_backend_url: string
  desktop_backend_default_port_occupant: string
  desktop_gateway_url: string
  runtime_config_dir: string
  runtime_default_config_dir: string
  runtime_database_path: string
  runtime_pending_config_dir: string
}

export interface RuntimeStopPortResult {
  port: number
  pid?: number
  command?: string
  stopped: boolean
  skipped: boolean
  message: string
}

export interface RuntimeStopStalePortsResult {
  results: RuntimeStopPortResult[]
}

export interface RuntimeConfigDirResult {
  config_dir: string
  database_path: string
  restart_required: boolean
  message: string
}

export interface RuntimeDatabaseImportResult {
  database_path: string
  backup_path: string
  relogin_required: boolean
  restart_required: boolean
  message: string
}

export interface RuntimeDatabaseBackupFile {
  name: string
  path: string
  size: number
  created_at: string
}

export interface RuntimeDatabaseBackupsResult {
  backup_dir: string
  backups: RuntimeDatabaseBackupFile[]
}

export interface RuntimeDatabaseBackupNowResult extends RuntimeDatabaseBackupsResult {
  backup: RuntimeDatabaseBackupFile
  message: string
}

export interface ConnectivityResult {
  ok: boolean
  status_code: number | null
  latency_ms: number | null
  message: string
  models: string[]
}

export interface ChatResult {
  ok: boolean
  status_code: number | null
  latency_ms: number | null
  message: string
  output: string
}

export interface McpTestResult {
  ok: boolean
  status_code: number | null
  latency_ms: number | null
  message: string
  output: string
  raw_excerpt: string
  tool_events: string[]
}

export interface GatewayStrategyStat {
  route_strategy: 'round_robin' | 'latency_first' | 'priority' | 'smart'
  request_count: number
  success_rate: number
  avg_latency_ms: number | null
  stream_request_count: number
  stream_success_rate: number
  avg_stream_ttfb_ms: number | null
}

export interface GatewayTrendBucket {
  bucket_start: string
  request_count: number
  success_count: number
  failure_count: number
  stream_request_count: number
  avg_latency_ms: number | null
}

export interface GatewayOverview {
  total_routes: number
  healthy_routes: number
  open_circuit_routes: number
  half_open_routes: number
  disabled_routes: number
  total_balance_display: string | null
  quantified_balance_site_count: number
  active_concurrency: number
  request_count_24h: number
  success_rate_24h: number
  avg_latency_ms_24h: number | null
  strategy_breakdown_24h: GatewayStrategyStat[]
  recent_trend_5m: GatewayTrendBucket[]
  route_strategy: 'round_robin' | 'latency_first' | 'priority' | 'smart'
  failure_threshold: number
  cooldown_seconds: number
  request_timeout: number
  max_attempts: number
  route_concurrency_limit: number
  concurrency_overflow_strategy: 'latency_first' | 'sequential'
}

export interface GatewaySettingsData {
  route_strategy: 'round_robin' | 'latency_first' | 'priority' | 'smart'
  failure_threshold: number
  cooldown_seconds: number
  request_timeout: number
  max_attempts: number
  route_concurrency_limit: number
  concurrency_overflow_strategy: 'latency_first' | 'sequential'
  smart_latency_bias: number
  smart_concurrency_bias: number
  smart_failure_bias: number
  smart_priority_bias: number
  gateway_api_key: string
}

export interface GatewayRoute {
  id: number
  site_id: number
  site_name: string
  base_url: string
  request_base_url: string
  request_base_urls?: string[]
  last_request_base_url?: string
  site_name_snapshot?: string
  site_base_url_snapshot?: string
  site_missing?: boolean
  has_api_key?: boolean
  group_name: string
  last_balance?: number | null
  balance_display?: string | null
  package_display?: string | null
  checkin_status?: string | null
  key_name: string
  key_fingerprint: string
  key_source: string
  route_type: 'claude' | 'codex' | 'gemini'
  route_type_manual?: boolean
  route_priority: number
  weight: number
  is_enabled: boolean
  circuit_state: string
  consecutive_failures: number
  active_concurrency: number
  request_count: number
  success_count: number
  failure_count: number
  avg_latency_ms: number | null
  ewma_latency_ms: number | null
  last_latency_ms: number | null
  success_rate: number
  last_status_code: number | null
  last_error: string | null
  last_used_at: string | null
  last_success_at: string | null
  last_failure_at: string | null
  circuit_open_until: string | null
}

export interface GatewayRouteProbeResult {
  id: number
  site_id: number
  site_name: string
  request_base_url?: string
  key_name: string
  key_fingerprint?: string
  ok: boolean
  status_code: number | null
  latency_ms: number | null
  message: string
  models: string[]
  last_status_code: number | null
  last_error: string | null
  last_latency_ms: number | null
  last_success_at: string | null
  last_failure_at: string | null
  checked_at: string
}

export interface GatewayLog {
  id: number
  request_id: string
  route_id: number | null
  route_label: string
  site_id: number | null
  site_name: string | null
  key_name: string
  key_fingerprint: string
  group_name: string
  target_path: string
  method: string
  route_strategy: string
  attempt_index: number
  status_code: number | null
  success: boolean
  latency_ms: number | null
  circuit_state_before: string
  failure_reason: string | null
  is_stream: boolean
  created_at: string
}
