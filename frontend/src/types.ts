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

export interface FeatureMeta {
  key: string
  name: string
  description: string
  frontend_path: string
  default_enabled: boolean
  enabled: boolean
}

export interface SitePayload {
  name: string
  base_url: string
  plugin_key: string
  group_name: string
  supported_models: string[] | null
  is_enabled: boolean
  notes: string
  credentials: Record<string, any>
  plugin_config: Record<string, any>
}

export interface Site extends SitePayload {
  id: number
  last_status: string | null
  connection_status?: string | null
  last_message: string | null
  last_balance: number | null
  balance_display?: string | null
  balance_unit?: string | null
  package_remaining?: number | null
  package_total?: number | null
  package_used?: number | null
  package_unit?: string | null
  package_display?: string | null
  checkin_status?: string | null
  last_run_at: string | null
  created_at: string
  updated_at: string | null
}

export interface SiteRegistrationBatchPayload extends SitePayload {
  email_pattern: string
  password: string
  count: number
  start_index: number
}

export interface SiteRegistrationBatchItem {
  index: number
  email: string
  ok: boolean
  message: string
  site?: Site
  api_key_count: number
}

export interface SiteRegistrationBatchResult {
  created_count: number
  failed_count: number
  items: SiteRegistrationBatchItem[]
}

export interface SiteSummary {
  site_id: number
  last_status: string | null
  connection_status?: string | null
  last_message: string | null
  last_balance: number | null
  balance_display?: string | null
  balance_unit?: string | null
  package_remaining?: number | null
  package_total?: number | null
  package_used?: number | null
  package_unit?: string | null
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
  package_remaining?: number | null
  package_total?: number | null
  package_used?: number | null
  package_unit?: string | null
  package_display?: string | null
  updated_credentials: Record<string, any>
  updated_plugin_config: Record<string, any>
}

export interface SiteApiKeyRefreshResult {
  site_id: number
  site_name: string
  ok: boolean
  message: string
  api_key_count: number
  primary_key_updated: boolean
  updated_credentials: Record<string, unknown>
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
  package_remaining?: number | null
  package_total?: number | null
  package_used?: number | null
  package_unit?: string | null
  package_display?: string | null
  account_name: string | null
  invite_link?: string | null
  invite_code?: string | null
  updated_credentials?: Record<string, any>
  updated_plugin_config?: Record<string, any>
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
  balance_probe_url?: string | null
  message: string
  checked_at: string
  last_balance: number | null
  balance_display?: string | null
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
  suggested_plugin_config?: Record<string, any>
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
  package_remaining?: number | null
  package_total?: number | null
  package_used?: number | null
  package_unit?: string | null
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
  feature_flags: Record<string, boolean>
  features: FeatureMeta[]
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
  security_warnings: string[]
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

export interface ModelListItem {
  id: string
  route_type: 'claude' | 'codex' | 'gemini' | string
  mode: 'chat' | 'image' | string
  base_url: string
  key_fingerprint: string
  key_name: string
  image_generation_path?: string
  image_edit_path?: string
}

export interface ModelListResult {
  ok: boolean
  status_code: number | null
  latency_ms: number | null
  message: string
  models: string[]
  items: ModelListItem[]
  base_url: string
  route_type: string
  key_fingerprint: string
  key_name: string
}

export interface ChatResult {
  ok: boolean
  status_code: number | null
  latency_ms: number | null
  message: string
  output: string
  images?: ChatImageOutput[]
  revised_prompt?: string
}

export interface ChatImageReference {
  name: string
  url: string
}

export interface ChatRequestMessage {
  role: 'system' | 'user' | 'assistant'
  content: string
  reference_images?: ChatImageReference[]
}

export interface ChatImageOutput {
  url: string
  b64_json: string
  revised_prompt: string
}

export interface ChatSession {
  id: number
  title: string
  site_id?: number | null
  site_name: string
  model: string
  mode: 'chat' | 'image' | string
  route_type: string
  key_fingerprint: string
  key_name: string
  image_size: string
  image_width: number
  image_height: number
  message_count: number
  last_message_text: string
  created_at: string
  updated_at: string
}

export interface ChatSessionMessage {
  id: number
  session_id: number
  seq: number
  role: 'system' | 'user' | 'assistant' | string
  content: string
  status: 'idle' | 'sending' | 'done' | 'error' | string
  mode: 'chat' | 'image' | string
  latency_ms: number | null
  status_code: number | null
  error: string
  reference_images: ChatImageReference[]
  images: ChatImageReference[]
  created_at: string
  updated_at: string
}

export interface ChatSessionDetail extends ChatSession {
  messages: ChatSessionMessage[]
}

export interface ChatSessionListResult {
  items: ChatSession[]
  count: number
}

export interface ChatSessionCreatePayload {
  title?: string
  site_id?: number | null
  site_name?: string
  model?: string
  mode?: 'chat' | 'image' | string
  route_type?: string
  key_fingerprint?: string
  key_name?: string
  image_size?: string
  image_width?: number
  image_height?: number
}

export type ChatSessionUpdatePayload = Partial<ChatSessionCreatePayload>

export interface ChatSessionMessagePayload {
  role: 'system' | 'user' | 'assistant'
  content: string
  status: 'idle' | 'sending' | 'done' | 'error'
  mode?: 'chat' | 'image' | ''
  latency_ms?: number | null
  status_code?: number | null
  error?: string
  reference_images?: ChatImageReference[]
  images?: ChatImageReference[]
  created_at?: string
}

export interface ChatSessionAppendPayload {
  messages: ChatSessionMessagePayload[]
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
  usage_cost_24h: GatewayUsageCostSummary
  strategy_breakdown_24h: GatewayStrategyStat[]
  route_strategy: 'round_robin' | 'latency_first' | 'priority' | 'smart'
  failure_threshold: number
  cooldown_seconds: number
  request_timeout: number
  max_attempts: number
  failure_retry_mode: 'retryable' | 'all'
  route_concurrency_limit: number
  concurrency_transfer_strategy: 'limit_only' | 'balance'
  concurrency_overflow_strategy: 'latency_first' | 'sequential'
}

export interface GatewayUsageCostSummary {
  input_cost: number
  cached_cost: number
  output_cost: number
  total_cost: number
  upstream_cost: number
  prompt_tokens: number
  cached_tokens: number
  output_tokens: number
  total_tokens: number
  known_requests: number
  unknown_requests: number
  upstream_requests: number
  currency: string
  window_seconds: number
  top_models: Array<{
    model: string
    requests: number
    total_cost: number
    known_price: boolean
  }>
}

export interface GatewayUsageRoute {
  route_id: number | null
  route_label: string
  site_id: number | null
  site_name: string | null
  key_name: string
  key_fingerprint: string
  group_name: string
  route_type: string
  model: string
  request_count: number
  success_count: number
  failure_count: number
  success_rate: number
  stream_request_count: number
  prompt_tokens: number
  cached_input_tokens: number
  completion_tokens: number
  total_tokens: number
  usage_cost: number | null
  computed_input_cost: number
  computed_cached_cost: number
  computed_output_cost: number
  computed_total_cost: number
  computed_cost_known: boolean
  computed_cost_mixed: boolean
  avg_latency_ms: number | null
  last_used_at: string | null
}

export interface GatewayUsage extends GatewayUsageRoute {
  start: string
  end: string
  routes: GatewayUsageRoute[]
}

export interface GatewaySettingsData {
  route_strategy: 'round_robin' | 'latency_first' | 'priority' | 'smart'
  failure_threshold: number
  cooldown_seconds: number
  request_timeout: number
  max_attempts: number
  failure_retry_mode: 'retryable' | 'all'
  route_concurrency_limit: number
  concurrency_transfer_strategy: 'limit_only' | 'balance'
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
  manual_request_base_urls?: string[]
  last_request_base_url?: string
  site_name_snapshot?: string
  site_base_url_snapshot?: string
  site_missing?: boolean
  has_api_key?: boolean
  group_name: string
  supported_models: string[]
  last_balance?: number | null
  balance_display?: string | null
  balance_unit?: string | null
  balance_probe_url?: string | null
  package_remaining?: number | null
  package_total?: number | null
  package_used?: number | null
  package_unit?: string | null
  package_display?: string | null
  checkin_status?: string | null
  key_name: string
  key_fingerprint: string
  key_source: string
  route_type: 'claude' | 'codex' | 'gemini'
  route_type_manual?: boolean
  model_probe_status?: 'default' | 'key_metadata' | 'success' | 'failed' | ''
  model_probe_message?: string
  model_probe_updated_at?: string | null
  route_priority: number
  route_priority_manual?: boolean
  weight: number
  is_enabled: boolean
  is_enabled_manual?: boolean
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
  supported_models?: string[]
  model_probe_status?: 'default' | 'key_metadata' | 'success' | 'failed' | ''
  model_probe_message?: string
  model_probe_updated_at?: string | null
  last_status_code: number | null
  last_error: string | null
  last_latency_ms: number | null
  last_success_at: string | null
  last_failure_at: string | null
  checked_at: string
}

export interface GatewayRouteUpdatePayload {
  route_type: 'claude' | 'codex' | 'gemini'
  supported_models?: string[]
  manual_request_base_urls?: string[]
}

export interface GatewayRouteDiagnosticItem {
  label: string
  ok: boolean
  severity: 'ok' | 'warning' | 'error'
  message: string
  detail: string
}

export interface GatewayRouteDiagnosis {
  id: number
  healthy: boolean
  route_label: string
  route: GatewayRoute
  diagnostics: GatewayRouteDiagnosticItem[]
  checked_at: string
  active_count: number
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
  prompt_tokens: number | null
  cached_input_tokens: number | null
  completion_tokens: number | null
  total_tokens: number | null
  usage_cost: number | null
  model: string
  requested_model?: string
  actual_model?: string
  circuit_state_before: string
  failure_reason: string | null
  is_stream: boolean
  created_at: string
}

export interface GatewayActiveRequest {
  id: string
  request_id: string
  route_id: number
  site_id: number
  route_label: string
  site_name: string
  key_name: string
  key_fingerprint: string
  group_name: string
  target_path: string
  method: string
  route_strategy: string
  attempt_index: number
  is_stream: boolean
  route_type: 'claude' | 'codex' | 'gemini' | string
  requested_model?: string
  actual_model?: string
  request_base_url: string
  active_concurrency: number
  started_at: string
  elapsed_ms: number
}
