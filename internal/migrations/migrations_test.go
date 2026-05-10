package migrations

import (
	"testing"

	"ai-sign-in-gateway/internal/config"
	"ai-sign-in-gateway/internal/database"
)

func TestApplyAddsGatewayModelColumnsToExistingSchema(t *testing.T) {
	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/legacy.db"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	statements := []string{
		`CREATE TABLE admin_users (
			id INTEGER PRIMARY KEY,
			username TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE system_settings (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE gateway_route_states (
			id INTEGER PRIMARY KEY,
			site_id INTEGER NOT NULL,
			key_fingerprint TEXT NOT NULL DEFAULT '',
			key_name TEXT NOT NULL DEFAULT '',
			key_source TEXT NOT NULL DEFAULT '',
			group_name TEXT NOT NULL DEFAULT '',
			route_priority INTEGER NOT NULL DEFAULT 100,
			weight INTEGER NOT NULL DEFAULT 1,
			is_enabled BOOLEAN NOT NULL DEFAULT 1,
			circuit_state TEXT NOT NULL DEFAULT 'closed',
			consecutive_failures INTEGER NOT NULL DEFAULT 0,
			request_count INTEGER NOT NULL DEFAULT 0,
			success_count INTEGER NOT NULL DEFAULT 0,
			failure_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE gateway_request_logs (
			id INTEGER PRIMARY KEY,
			request_id TEXT NOT NULL DEFAULT '',
			site_id INTEGER,
			key_fingerprint TEXT NOT NULL DEFAULT '',
			key_name TEXT NOT NULL DEFAULT '',
			group_name TEXT NOT NULL DEFAULT '',
			target_path TEXT NOT NULL DEFAULT '',
			method TEXT NOT NULL DEFAULT 'GET',
			route_strategy TEXT NOT NULL DEFAULT 'round_robin',
			attempt_index INTEGER NOT NULL DEFAULT 1,
			success BOOLEAN NOT NULL DEFAULT 0,
			circuit_state_before TEXT NOT NULL DEFAULT 'closed',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := Apply(db); err != nil {
		t.Fatal(err)
	}

	for _, item := range []struct {
		table  string
		column string
	}{
		{"gateway_route_states", "model_probe_status"},
		{"gateway_route_states", "model_probe_message"},
		{"gateway_route_states", "model_probe_updated_at"},
		{"gateway_route_states", "manual_request_base_urls"},
		{"gateway_request_logs", "requested_model"},
		{"gateway_request_logs", "actual_model"},
		{"chat_sessions", "last_message_text"},
		{"chat_messages", "reference_images"},
	} {
		if !db.Migrator().HasColumn(item.table, item.column) {
			t.Fatalf("missing column %s.%s", item.table, item.column)
		}
	}
}
