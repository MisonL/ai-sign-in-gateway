package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/plugins"
	"github.com/glebarez/sqlite"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func TestUpdateCheckinParticipationPersistsSetting(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:checkin-participation?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	site := models.Site{
		Name:         "demo",
		BaseURL:      "https://example.com",
		PluginKey:    "sub2api-platform",
		IsEnabled:    true,
		PluginConfig: models.JSONMap{},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &App{DB: db}
	body := bytes.NewReader([]byte(`{"include_in_checkin":false}`))
	req := httptest.NewRequest(http.MethodPost, "/sites/1/participation", body)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("siteID", "1")
	req = contextWithRoute(req, routeCtx)
	rec := httptest.NewRecorder()

	app.UpdateCheckinParticipation(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var stored models.Site
	if err := db.First(&stored, site.ID).Error; err != nil {
		t.Fatalf("reload site: %v", err)
	}
	if got := includeInCheckin(stored); got {
		t.Fatalf("includeInCheckin = %v", got)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/checkins/sites", nil)
	listRec := httptest.NewRecorder()
	app.CheckinSites(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}
	var payload []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("payload len = %d", len(payload))
	}
	if got, ok := payload[0]["include_in_checkin"].(bool); !ok || got {
		t.Fatalf("include_in_checkin response = %v", payload[0]["include_in_checkin"])
	}
}

func TestSiteCheckinRefreshesBalanceAfterCheckin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/checkin":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "signed",
				"data":    map[string]any{},
			})
		case "/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "ok",
				"data": map[string]any{
					"logged_in": true,
					"balance":   42.5,
					"currency":  "USD",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	db, err := gorm.Open(sqlite.Open("file:checkin-refresh-balance?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	settings := models.SystemSetting{ID: 1, RequestTimeout: 5}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	oldBalance := 3.25
	site := models.Site{
		Name:        "demo",
		BaseURL:     server.URL,
		PluginKey:   "http-relay-station",
		IsEnabled:   true,
		LastBalance: &oldBalance,
		Credentials: models.JSONMap{
			"api_key": "token-123",
		},
		PluginConfig: models.JSONMap{
			"auth_mode":                 "bearer",
			"status_path":               "/status",
			"status_method":             "GET",
			"status_login_path":         "data.logged_in",
			"status_balance_path":       "data.balance",
			"status_balance_unit_path":  "data.currency",
			"status_message_path":       "message",
			"checkin_path":              "/checkin",
			"checkin_method":            "POST",
			"checkin_success_path":      "success",
			"checkin_message_path":      "message",
			"checkin_balance_path":      "data.missing_balance",
			"checkin_balance_unit_path": "data.currency",
			"default_balance_unit":      "USD",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &App{DB: db, PluginManager: plugins.NewManager()}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/sites/%d/checkin", site.ID), nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("siteID", fmt.Sprint(site.ID))
	req = contextWithRoute(req, routeCtx)
	rec := httptest.NewRecorder()

	app.SiteCheckin(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, _ := payload["balance"].(float64); got != 42.5 {
		t.Fatalf("response balance = %v", payload["balance"])
	}
	if got, _ := payload["balance_unit"].(string); got != "$" {
		t.Fatalf("response balance_unit = %q", got)
	}

	var stored models.Site
	if err := db.First(&stored, site.ID).Error; err != nil {
		t.Fatalf("reload site: %v", err)
	}
	if stored.LastBalance == nil || *stored.LastBalance != 42.5 {
		t.Fatalf("stored.LastBalance = %v", stored.LastBalance)
	}
	if got := jsonMapString(stored.PluginConfig, "balance_unit"); got != "$" {
		t.Fatalf("balance_unit = %q", got)
	}
}

func contextWithRoute(req *http.Request, routeCtx *chi.Context) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}
