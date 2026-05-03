package migrations

import (
	"strings"

	"ai-sign-in-gateway/internal/database"
	"gorm.io/gorm"
)

func Apply(db *gorm.DB) error {
	// Detect whether this DB was previously bootstrapped by the Python backend.
	// In that case GORM's AutoMigrate would try to rebuild a few tables
	// (e.g. admin_users) and silently drop NOT NULL columns. Skip AutoMigrate
	// when an existing schema is detected and only run idempotent ALTER/CREATE
	// statements via ensureIndexes / addMissingColumns.
	if isFreshDatabase(db) {
		if err := database.AutoMigrate(db); err != nil {
			return err
		}
	} else if err := addMissingColumns(db); err != nil {
		return err
	}
	return ensureIndexes(db)
}

func isFreshDatabase(db *gorm.DB) bool {
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'admin_users'").Scan(&count).Error; err != nil {
		return true
	}
	return count == 0
}

func addMissingColumns(db *gorm.DB) error {
	type columnPatch struct {
		table     string
		column    string
		statement string
	}
	patches := []columnPatch{
		{table: "gateway_request_logs", column: "is_stream", statement: "ALTER TABLE gateway_request_logs ADD COLUMN is_stream BOOLEAN NOT NULL DEFAULT 0"},
		{table: "gateway_request_logs", column: "route_state_id", statement: "ALTER TABLE gateway_request_logs ADD COLUMN route_state_id INTEGER"},
		{table: "gateway_route_states", column: "ewma_latency_ms", statement: "ALTER TABLE gateway_route_states ADD COLUMN ewma_latency_ms FLOAT"},
		{table: "system_settings", column: "desktop_keep_running", statement: "ALTER TABLE system_settings ADD COLUMN desktop_keep_running BOOLEAN NOT NULL DEFAULT 0"},
		{table: "system_settings", column: "database_backup_enabled", statement: "ALTER TABLE system_settings ADD COLUMN database_backup_enabled BOOLEAN NOT NULL DEFAULT 0"},
		{table: "system_settings", column: "database_backup_dir", statement: "ALTER TABLE system_settings ADD COLUMN database_backup_dir TEXT NOT NULL DEFAULT ''"},
		{table: "system_settings", column: "database_backup_interval_minutes", statement: "ALTER TABLE system_settings ADD COLUMN database_backup_interval_minutes INTEGER NOT NULL DEFAULT 1440"},
		{table: "system_settings", column: "database_backup_retention", statement: "ALTER TABLE system_settings ADD COLUMN database_backup_retention INTEGER NOT NULL DEFAULT 7"},
		{table: "system_settings", column: "gateway_smart_latency_bias", statement: "ALTER TABLE system_settings ADD COLUMN gateway_smart_latency_bias FLOAT NOT NULL DEFAULT 1"},
		{table: "system_settings", column: "gateway_smart_concurrency_bias", statement: "ALTER TABLE system_settings ADD COLUMN gateway_smart_concurrency_bias FLOAT NOT NULL DEFAULT 1.5"},
		{table: "system_settings", column: "gateway_smart_failure_bias", statement: "ALTER TABLE system_settings ADD COLUMN gateway_smart_failure_bias FLOAT NOT NULL DEFAULT 1"},
		{table: "system_settings", column: "gateway_smart_priority_bias", statement: "ALTER TABLE system_settings ADD COLUMN gateway_smart_priority_bias FLOAT NOT NULL DEFAULT 0.5"},
		{table: "system_settings", column: "gateway_failure_retry_mode", statement: "ALTER TABLE system_settings ADD COLUMN gateway_failure_retry_mode TEXT NOT NULL DEFAULT 'retryable'"},
		{table: "system_settings", column: "gateway_concurrency_transfer_strategy", statement: "ALTER TABLE system_settings ADD COLUMN gateway_concurrency_transfer_strategy TEXT NOT NULL DEFAULT 'limit_only'"},
		{table: "system_settings", column: "site_group_catalog", statement: "ALTER TABLE system_settings ADD COLUMN site_group_catalog TEXT NOT NULL DEFAULT '[]'"},
		{table: "gateway_route_states", column: "site_name_snapshot", statement: "ALTER TABLE gateway_route_states ADD COLUMN site_name_snapshot TEXT NOT NULL DEFAULT ''"},
		{table: "gateway_route_states", column: "site_base_url_snapshot", statement: "ALTER TABLE gateway_route_states ADD COLUMN site_base_url_snapshot TEXT NOT NULL DEFAULT ''"},
		{table: "gateway_route_states", column: "site_api_url_snapshot", statement: "ALTER TABLE gateway_route_states ADD COLUMN site_api_url_snapshot TEXT NOT NULL DEFAULT '[]'"},
		{table: "gateway_route_states", column: "last_request_base_url", statement: "ALTER TABLE gateway_route_states ADD COLUMN last_request_base_url TEXT NOT NULL DEFAULT ''"},
		{table: "gateway_route_states", column: "route_type", statement: "ALTER TABLE gateway_route_states ADD COLUMN route_type TEXT NOT NULL DEFAULT 'codex'"},
		{table: "gateway_route_states", column: "route_type_manual", statement: "ALTER TABLE gateway_route_states ADD COLUMN route_type_manual BOOLEAN NOT NULL DEFAULT 0"},
		{table: "gateway_route_states", column: "route_priority_manual", statement: "ALTER TABLE gateway_route_states ADD COLUMN route_priority_manual BOOLEAN NOT NULL DEFAULT 0"},
	}
	for _, patch := range patches {
		if db.Migrator().HasColumn(patch.table, patch.column) {
			continue
		}
		if err := db.Exec(patch.statement).Error; err != nil {
			if isDuplicateColumnErr(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate column name") || strings.Contains(msg, "already exists")
}

func ensureIndexes(db *gorm.DB) error {
	statements := []string{
		"CREATE INDEX IF NOT EXISTS ix_gateway_route_states_route_type ON gateway_route_states (route_type)",
		"CREATE INDEX IF NOT EXISTS ix_gateway_route_states_route_priority ON gateway_route_states (route_priority)",
		"CREATE INDEX IF NOT EXISTS ix_gateway_route_states_circuit_state ON gateway_route_states (circuit_state)",
		"CREATE INDEX IF NOT EXISTS ix_gateway_request_logs_created_at ON gateway_request_logs (created_at)",
		"CREATE INDEX IF NOT EXISTS ix_gateway_request_logs_success ON gateway_request_logs (success)",
		"CREATE INDEX IF NOT EXISTS ix_gateway_request_logs_is_stream ON gateway_request_logs (is_stream)",
		"CREATE INDEX IF NOT EXISTS ix_gateway_request_logs_route_state_id ON gateway_request_logs (route_state_id)",
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
