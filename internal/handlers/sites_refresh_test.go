package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/plugins"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
)

func TestRefreshOneSiteUsesBalanceProbeForRelayOnlySites(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/usage" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer probe-key" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"remaining":12.34,"unit":"USD"}`))
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	site := models.Site{
		Name:      "su8",
		BaseURL:   upstream.URL,
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"account": "公司",
			"api_key": "probe-key",
		},
		PluginConfig: models.JSONMap{
			"endpoint_url": upstream.URL + "/v1",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &App{DB: db, PluginManager: plugins.NewManager()}
	summary := app.refreshOneSite(context.Background(), site, 5)

	connectionStatus, _ := summary["connection_status"].(*string)
	if connectionStatus == nil || *connectionStatus != "success" {
		t.Fatalf("connection_status = %v", summary["connection_status"])
	}
	message, _ := summary["last_message"].(*string)
	if message == nil || !strings.Contains(*message, "模型出口验证成功") {
		t.Fatalf("last_message = %v", summary["last_message"])
	}
	balance, _ := summary["last_balance"].(*float64)
	if balance == nil || *balance != 12.34 {
		t.Fatalf("last_balance = %v", summary["last_balance"])
	}

	var stored models.Site
	if err := db.First(&stored, site.ID).Error; err != nil {
		t.Fatalf("reload site: %v", err)
	}
	if stored.LastStatus == nil || *stored.LastStatus != "success" {
		t.Fatalf("stored.LastStatus = %v", stored.LastStatus)
	}
	if stored.LastBalance == nil || *stored.LastBalance != 12.34 {
		t.Fatalf("stored.LastBalance = %v", stored.LastBalance)
	}
	if got := strings.TrimSpace(jsonMapString(stored.PluginConfig, "balance_unit")); got != "USD" {
		t.Fatalf("balance_unit = %q", got)
	}
}

func TestRefreshOneSiteInvitePersistsInviteInfo(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "ok",
				"data": map[string]any{
					"logged_in": true,
				},
			})
		case "/invite":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"invite": map[string]any{
						"code": "BATCH-INVITE",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	site := models.Site{
		Name:      "invite-site",
		BaseURL:   upstream.URL,
		PluginKey: "http-relay-station",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "token-123",
		},
		PluginConfig: models.JSONMap{
			"auth_mode":            "bearer",
			"status_path":          "/status",
			"status_method":        "GET",
			"status_login_path":    "data.logged_in",
			"invite_path":          "/invite",
			"invite_code_path":     "data.invite.code",
			"invite_link_template": "/register?code={code}",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &App{DB: db, PluginManager: plugins.NewManager()}
	result := app.refreshOneSiteInvite(context.Background(), site, 5)
	if !result.OK {
		t.Fatalf("result.OK = false, message = %q", result.Message)
	}
	if result.InviteCode == nil || *result.InviteCode != "BATCH-INVITE" {
		t.Fatalf("InviteCode = %v", result.InviteCode)
	}
	if result.InviteLink == nil || *result.InviteLink != upstream.URL+"/register?code=BATCH-INVITE" {
		t.Fatalf("InviteLink = %v", result.InviteLink)
	}

	var stored models.Site
	if err := db.First(&stored, site.ID).Error; err != nil {
		t.Fatalf("reload site: %v", err)
	}
	if got := strings.TrimSpace(jsonMapString(stored.PluginConfig, "invite_code")); got != "BATCH-INVITE" {
		t.Fatalf("stored invite_code = %q", got)
	}
	if got := strings.TrimSpace(jsonMapString(stored.PluginConfig, "invite_link")); got != upstream.URL+"/register?code=BATCH-INVITE" {
		t.Fatalf("stored invite_link = %q", got)
	}
}
