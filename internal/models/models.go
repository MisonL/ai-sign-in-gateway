package models

import "time"

type AdminUser struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:50;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Site struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Name         string     `gorm:"size:100;index;not null" json:"name"`
	BaseURL      string     `gorm:"size:255;not null" json:"base_url"`
	PluginKey    string     `gorm:"size:100;index;not null" json:"plugin_key"`
	GroupName    string     `gorm:"size:100;default:''" json:"group_name"`
	IsEnabled    bool       `gorm:"not null" json:"is_enabled"`
	Notes        string     `gorm:"type:text;default:''" json:"notes"`
	Credentials  JSONMap    `gorm:"type:json" json:"credentials"`
	PluginConfig JSONMap    `gorm:"type:json" json:"plugin_config"`
	LastStatus   *string    `gorm:"size:30" json:"last_status,omitempty"`
	LastMessage  *string    `gorm:"type:text" json:"last_message,omitempty"`
	LastBalance  *float64   `json:"last_balance,omitempty"`
	LastRunAt    *time.Time `json:"last_run_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Runs         []CheckinRun
	QueueTasks   []SiteQueueTask `gorm:"constraint:OnDelete:CASCADE"`
}

type CheckinRun struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	SiteID          *uint      `gorm:"index" json:"site_id,omitempty"`
	TriggerType     string     `gorm:"size:30;default:manual" json:"trigger_type"`
	Status          string     `gorm:"size:30;default:pending;index" json:"status"`
	Message         string     `gorm:"type:text;default:''" json:"message"`
	ResponseExcerpt *string    `gorm:"type:text" json:"response_excerpt,omitempty"`
	Balance         *float64   `json:"balance,omitempty"`
	AttemptCount    int        `gorm:"default:1" json:"attempt_count"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	Site            *Site      `json:"-"`
}

type SystemSetting struct {
	ID                                 uint      `gorm:"primaryKey;default:1" json:"id"`
	Timezone                           string    `gorm:"size:80;default:Asia/Shanghai" json:"timezone"`
	ScheduleEnabled                    bool      `gorm:"default:true" json:"schedule_enabled"`
	DailyRunTime                       string    `gorm:"size:10;default:09:00" json:"daily_run_time"`
	CheckinConcurrency                 int       `gorm:"default:1" json:"checkin_concurrency"`
	CheckinGlobalConcurrency           int       `gorm:"default:4" json:"checkin_global_concurrency"`
	CheckinIntervalSeconds             int       `gorm:"default:1" json:"checkin_interval_seconds"`
	RetryCount                         int       `gorm:"default:1" json:"retry_count"`
	RequestTimeout                     int       `gorm:"default:20" json:"request_timeout"`
	OnlyEnabledSites                   bool      `gorm:"default:true" json:"only_enabled_sites"`
	DesktopKeepRunning                 bool      `gorm:"default:false" json:"desktop_keep_running"`
	DatabaseBackupEnabled              bool      `gorm:"default:false" json:"database_backup_enabled"`
	DatabaseBackupDir                  string    `gorm:"type:text;default:''" json:"database_backup_dir"`
	DatabaseBackupIntervalMinutes      int       `gorm:"default:1440" json:"database_backup_interval_minutes"`
	DatabaseBackupRetention            int       `gorm:"default:7" json:"database_backup_retention"`
	GatewayRouteStrategy               string    `gorm:"size:30;default:round_robin" json:"gateway_route_strategy"`
	GatewayFailureThreshold            int       `gorm:"default:3" json:"gateway_failure_threshold"`
	GatewayCooldownSeconds             int       `gorm:"default:180" json:"gateway_cooldown_seconds"`
	GatewayRequestTimeout              int       `gorm:"default:60" json:"gateway_request_timeout"`
	GatewayMaxAttempts                 int       `gorm:"default:0" json:"gateway_max_attempts"`
	GatewayFailureRetryMode            string    `gorm:"size:30;default:retryable" json:"gateway_failure_retry_mode"`
	GatewayRouteConcurrencyLimit       int       `gorm:"default:5" json:"gateway_route_concurrency_limit"`
	GatewayConcurrencyTransferStrategy string    `gorm:"size:30;default:limit_only" json:"gateway_concurrency_transfer_strategy"`
	GatewayConcurrencyOverflowStrategy string    `gorm:"size:30;default:latency_first" json:"gateway_concurrency_overflow_strategy"`
	GatewaySmartLatencyBias            float64   `gorm:"default:1" json:"gateway_smart_latency_bias"`
	GatewaySmartConcurrencyBias        float64   `gorm:"default:1.5" json:"gateway_smart_concurrency_bias"`
	GatewaySmartFailureBias            float64   `gorm:"default:1" json:"gateway_smart_failure_bias"`
	GatewaySmartPriorityBias           float64   `gorm:"default:0.5" json:"gateway_smart_priority_bias"`
	GatewayAPIKey                      string    `gorm:"size:255;default:''" json:"gateway_api_key"`
	SiteGroupCatalog                   string    `gorm:"type:text;default:'[]'" json:"site_group_catalog"`
	UpdatedAt                          time.Time `json:"updated_at"`
}

type GatewayRouteState struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	SiteID              uint       `gorm:"uniqueIndex:uq_gateway_route_site_key;index" json:"site_id"`
	KeyFingerprint      string     `gorm:"size:64;uniqueIndex:uq_gateway_route_site_key;index" json:"key_fingerprint"`
	KeyName             string     `gorm:"size:120;default:''" json:"key_name"`
	KeySource           string     `gorm:"size:80;default:site" json:"key_source"`
	SiteNameSnapshot    string     `gorm:"size:120;default:''" json:"site_name_snapshot"`
	SiteBaseURLSnapshot string     `gorm:"size:255;default:''" json:"site_base_url_snapshot"`
	SiteAPIURLSnapshot  string     `gorm:"type:text;default:'[]'" json:"site_api_url_snapshot"`
	LastRequestBaseURL  string     `gorm:"size:255;default:''" json:"last_request_base_url"`
	RouteType           string     `gorm:"size:20;default:codex" json:"route_type"`
	RouteTypeManual     bool       `gorm:"default:false" json:"route_type_manual"`
	GroupName           string     `gorm:"size:100;default:''" json:"group_name"`
	RoutePriority       int        `gorm:"default:100;index" json:"route_priority"`
	RoutePriorityManual bool       `gorm:"default:false" json:"route_priority_manual"`
	Weight              int        `gorm:"default:1" json:"weight"`
	IsEnabled           bool       `gorm:"default:true" json:"is_enabled"`
	CircuitState        string     `gorm:"size:20;default:closed;index" json:"circuit_state"`
	ConsecutiveFailures int        `gorm:"default:0" json:"consecutive_failures"`
	RequestCount        int        `gorm:"default:0" json:"request_count"`
	SuccessCount        int        `gorm:"default:0" json:"success_count"`
	FailureCount        int        `gorm:"default:0" json:"failure_count"`
	AvgLatencyMS        *float64   `json:"avg_latency_ms,omitempty"`
	EWMALatencyMS       *float64   `gorm:"column:ewma_latency_ms" json:"ewma_latency_ms,omitempty"`
	LastLatencyMS       *float64   `json:"last_latency_ms,omitempty"`
	LastStatusCode      *int       `json:"last_status_code,omitempty"`
	LastError           *string    `gorm:"type:text" json:"last_error,omitempty"`
	LastUsedAt          *time.Time `json:"last_used_at,omitempty"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt       *time.Time `json:"last_failure_at,omitempty"`
	CircuitOpenedAt     *time.Time `json:"circuit_opened_at,omitempty"`
	CircuitOpenUntil    *time.Time `json:"circuit_open_until,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	Site                Site       `json:"-"`
}

type GatewayRequestLog struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	RequestID          string    `gorm:"size:40;index" json:"request_id"`
	RouteStateID       *uint     `gorm:"index" json:"route_state_id,omitempty"`
	SiteID             *uint     `gorm:"index" json:"site_id,omitempty"`
	KeyFingerprint     string    `gorm:"size:64;index" json:"key_fingerprint"`
	KeyName            string    `gorm:"size:120;default:''" json:"key_name"`
	GroupName          string    `gorm:"size:100;default:''" json:"group_name"`
	TargetPath         string    `gorm:"size:255;default:''" json:"target_path"`
	Method             string    `gorm:"size:10;default:GET" json:"method"`
	RouteStrategy      string    `gorm:"size:30;default:round_robin" json:"route_strategy"`
	AttemptIndex       int       `gorm:"default:1" json:"attempt_index"`
	StatusCode         *int      `json:"status_code,omitempty"`
	Success            bool      `gorm:"default:false;index" json:"success"`
	LatencyMS          *float64  `json:"latency_ms,omitempty"`
	CircuitStateBefore string    `gorm:"size:20;default:closed" json:"circuit_state_before"`
	FailureReason      *string   `gorm:"type:text" json:"failure_reason,omitempty"`
	IsStream           bool      `gorm:"default:false;index" json:"is_stream"`
	CreatedAt          time.Time `gorm:"index" json:"created_at"`
	Site               *Site     `json:"-"`
}

type SiteQueueTask struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	SiteID      uint       `gorm:"uniqueIndex:uq_site_queue_task_site_key;index" json:"site_id"`
	TaskKey     string     `gorm:"size:50;uniqueIndex:uq_site_queue_task_site_key;index" json:"task_key"`
	Title       string     `gorm:"size:120;not null" json:"title"`
	Detail      string     `gorm:"type:text;default:''" json:"detail"`
	Status      string     `gorm:"size:20;default:pending;index" json:"status"`
	SortOrder   int        `gorm:"default:0" json:"sort_order"`
	ActionKey   string     `gorm:"size:40;default:''" json:"action_key"`
	ActionLabel string     `gorm:"size:80;default:''" json:"action_label"`
	LastMessage *string    `gorm:"type:text" json:"last_message,omitempty"`
	LastError   *string    `gorm:"type:text" json:"last_error,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Site        Site       `json:"-"`
}

func All() []any {
	return []any{
		&AdminUser{},
		&Site{},
		&CheckinRun{},
		&SystemSetting{},
		&GatewayRouteState{},
		&GatewayRequestLog{},
		&SiteQueueTask{},
	}
}
