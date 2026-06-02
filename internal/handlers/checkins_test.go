package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/plugins"
	"github.com/glebarez/sqlite"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func TestCheckinRunsHonorsLimitQuery(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:checkin-runs-limit?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	site := models.Site{Name: "demo", BaseURL: "https://example.com", PluginKey: "http-relay-station", IsEnabled: true}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for i := 0; i < 3; i++ {
		run := models.CheckinRun{
			SiteID:      &site.ID,
			TriggerType: "manual",
			Status:      "success",
			Message:     fmt.Sprintf("run-%d", i),
			StartedAt:   base.Add(time.Duration(i) * time.Minute),
		}
		if err := db.Create(&run).Error; err != nil {
			t.Fatalf("create run %d: %v", i, err)
		}
	}

	app := &App{DB: db}
	req := httptest.NewRequest(http.MethodGet, "/checkins/runs?limit=2", nil)
	rec := httptest.NewRecorder()
	app.CheckinRuns(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("payload len = %d, body = %s", len(payload), rec.Body.String())
	}
	if got, _ := payload[0]["message"].(string); got != "run-2" {
		t.Fatalf("first run message = %q", got)
	}
}

func TestTotpPreviewReturnsCurrentCode(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:totp-preview-code?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	site := models.Site{
		Name:      "totp",
		BaseURL:   "https://example.com",
		PluginKey: "sub2api-platform",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"totp_secret": "JBSWY3DPEHPK3PXP",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &App{DB: db}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/sites/%d/totp-preview", site.ID), nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("siteID", fmt.Sprint(site.ID))
	req = contextWithRoute(req, routeCtx)
	rec := httptest.NewRecorder()
	app.TotpPreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	code, _ := payload["code"].(string)
	if !isSixDigitCode(code) {
		t.Fatalf("code = %q", code)
	}
	expiresIn, _ := payload["expires_in"].(float64)
	if expiresIn <= 0 || expiresIn > 30 {
		t.Fatalf("expires_in = %v", payload["expires_in"])
	}
}

func TestTotpPreviewRejectsMissingConfig(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:totp-preview-missing?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	site := models.Site{
		Name:        "totp",
		BaseURL:     "https://example.com",
		PluginKey:   "sub2api-platform",
		IsEnabled:   true,
		Credentials: models.JSONMap{},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &App{DB: db}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/sites/%d/totp-preview", site.ID), nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("siteID", fmt.Sprint(site.ID))
	req = contextWithRoute(req, routeCtx)
	rec := httptest.NewRecorder()
	app.TotpPreview(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if detail, ok := payload["detail"].(string); !ok || detail == "" {
		t.Fatalf("detail is empty: %s", rec.Body.String())
	}
}

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

func TestRelayOnlySiteCannotParticipateInCheckin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "signed",
			"data": map[string]any{
				"balance": 12.5,
			},
		})
	}))
	defer server.Close()

	db, err := gorm.Open(sqlite.Open("file:checkin-relay-only?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	relaySite := models.Site{
		Name:      "relay",
		BaseURL:   "https://relay.example/v1",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "relay-key",
		},
		PluginConfig: models.JSONMap{
			"include_in_checkin": true,
		},
	}
	checkinSite := models.Site{
		Name:      "checkin",
		BaseURL:   server.URL,
		PluginKey: "http-relay-station",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"account": "demo",
		},
		PluginConfig: models.JSONMap{
			"auth_mode":            "none",
			"checkin_path":         "/checkin",
			"checkin_method":       "POST",
			"checkin_success_path": "success",
			"checkin_message_path": "message",
			"checkin_balance_path": "data.balance",
		},
	}
	if err := db.Create(&relaySite).Error; err != nil {
		t.Fatalf("create relay site: %v", err)
	}
	if err := db.Create(&checkinSite).Error; err != nil {
		t.Fatalf("create checkin site: %v", err)
	}

	app := &App{DB: db, PluginManager: plugins.NewManager()}
	listReq := httptest.NewRequest(http.MethodGet, "/checkins/sites", nil)
	listRec := httptest.NewRecorder()
	app.CheckinSites(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}
	var sites []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &sites); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	byName := map[string]map[string]any{}
	for _, item := range sites {
		byName[fmt.Sprint(item["name"])] = item
	}
	if got, _ := byName["relay"]["can_checkin"].(bool); got {
		t.Fatalf("relay can_checkin = %v", got)
	}
	if got, _ := byName["relay"]["include_in_checkin"].(bool); got {
		t.Fatalf("relay include_in_checkin = %v", got)
	}
	if got, _ := byName["checkin"]["can_checkin"].(bool); !got {
		t.Fatalf("checkin can_checkin = %v", got)
	}

	body := bytes.NewReader([]byte(`{"include_in_checkin":true}`))
	req := httptest.NewRequest(http.MethodPost, "/checkins/sites/1/participation", body)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("siteID", fmt.Sprint(relaySite.ID))
	req = contextWithRoute(req, routeCtx)
	rec := httptest.NewRecorder()
	app.UpdateCheckinParticipation(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("participation status = %d, body = %s", rec.Code, rec.Body.String())
	}

	batchReq := httptest.NewRequest(http.MethodPost, "/checkins/batch", bytes.NewReader([]byte(`{"only_enabled":true}`)))
	batchRec := httptest.NewRecorder()
	app.RunBatchCheckin(batchRec, batchReq)
	if batchRec.Code != http.StatusOK {
		t.Fatalf("batch status = %d, body = %s", batchRec.Code, batchRec.Body.String())
	}
	var runs []map[string]any
	if err := json.Unmarshal(batchRec.Body.Bytes(), &runs); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("batch runs len = %d, body = %s", len(runs), batchRec.Body.String())
	}
	if got, _ := runs[0]["site_id"].(float64); uint(got) != checkinSite.ID {
		t.Fatalf("batch site_id = %v", runs[0]["site_id"])
	}
}

func TestRunBatchCheckinRejectsMalformedJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/checkins/batch", bytes.NewReader([]byte(`{"site_ids":[`)))
	rec := httptest.NewRecorder()

	(&App{}).RunBatchCheckin(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望状态码 400，实际 %d，响应体：%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解码错误响应失败：%v", err)
	}
	if detail, ok := payload["detail"].(string); !ok || detail == "" {
		t.Fatalf("错误详情为空：%s", rec.Body.String())
	}
}

func TestRunBatchCheckinCompletesAfterRequestContextCanceled(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/checkin":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "signed",
				"data":    map[string]any{"balance": 9.5, "currency": "USD"},
			})
		case "/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "ok",
				"data":    map[string]any{"logged_in": true, "balance": 10.5, "currency": "USD"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	db, err := gorm.Open(sqlite.Open("file:checkin-request-cancel?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	if err := db.Create(&models.SystemSetting{
		ID:                       1,
		RequestTimeout:           5,
		OnlyEnabledSites:         true,
		CheckinConcurrency:       1,
		CheckinGlobalConcurrency: 1,
		CheckinIntervalSeconds:   0,
		RetryCount:               0,
	}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	site := models.Site{
		Name:      "manual",
		BaseURL:   server.URL,
		PluginKey: "http-relay-station",
		IsEnabled: true,
		PluginConfig: models.JSONMap{
			"auth_mode":                 "none",
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
			"checkin_balance_path":      "data.balance",
			"checkin_balance_unit_path": "data.currency",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/checkins/batch", bytes.NewReader([]byte(`{"only_enabled":true}`))).WithContext(ctx)
	cancel()
	rec := httptest.NewRecorder()

	(&App{DB: db, PluginManager: plugins.NewManager()}).RunBatchCheckin(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 || payload[0]["status"] != "success" {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want checkin and status requests", requestCount)
	}
}

func TestSiteCheckinUpdatesSiteStatusWhenPluginIsMissing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:checkin-missing-plugin-status?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	site := models.Site{Name: "missing", BaseURL: "https://example.com", PluginKey: "missing-plugin", IsEnabled: true}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/sites/%d/checkin", site.ID), nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("siteID", fmt.Sprint(site.ID))
	req = contextWithRoute(req, routeCtx)
	rec := httptest.NewRecorder()
	(&App{DB: db, PluginManager: plugins.NewManager()}).SiteCheckin(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var stored models.Site
	if err := db.First(&stored, site.ID).Error; err != nil {
		t.Fatalf("reload site: %v", err)
	}
	if stored.LastStatus == nil || *stored.LastStatus != "failed" {
		t.Fatalf("last status = %v", stored.LastStatus)
	}
	if stored.LastMessage == nil || !strings.Contains(*stored.LastMessage, "Plugin 'missing-plugin' not found") {
		t.Fatalf("last message = %v", stored.LastMessage)
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

func isSixDigitCode(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
