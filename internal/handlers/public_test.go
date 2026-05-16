package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"ai-sign-in-gateway/internal/config"
	"ai-sign-in-gateway/internal/database"
	"ai-sign-in-gateway/internal/migrations"
	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/services"
	"github.com/go-chi/chi/v5"
)

func TestPublicInvitesListsEnabledSitesWithStoredInvite(t *testing.T) {
	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/public_invites.db"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	sites := []models.Site{
		{
			Name:      "with invite",
			BaseURL:   "https://invite.example",
			PluginKey: "sub2api-platform",
			IsEnabled: true,
			PluginConfig: models.JSONMap{
				"invite_link":     "https://invite.example/register?aff=abc",
				"invite_code":     "abc",
				"package_display": "free",
			},
		},
		{
			Name:         "disabled",
			BaseURL:      "https://disabled.example",
			PluginKey:    "sub2api-platform",
			IsEnabled:    false,
			PluginConfig: models.JSONMap{"invite_link": "https://disabled.example/register"},
		},
		{
			Name:         "without invite",
			BaseURL:      "https://empty.example",
			PluginKey:    "sub2api-platform",
			IsEnabled:    true,
			PluginConfig: models.JSONMap{},
		},
	}
	for _, site := range sites {
		if err := db.Create(&site).Error; err != nil {
			t.Fatalf("create site: %v", err)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/public/invites", nil)
	(&App{DB: db}).PublicInvites(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("payload len = %d: %#v", len(payload), payload)
	}
	if got := payload[0]["site_name"]; got != "with invite" {
		t.Fatalf("site_name = %v", got)
	}
	if got := payload[0]["invite_code"]; got != "abc" {
		t.Fatalf("invite_code = %v", got)
	}
}

func TestGatewayAliasRouteWithoutV1(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer route-key" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"demo"}]}`))
	}))
	defer upstream.Close()

	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/gateway_alias.db"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	site := models.Site{
		Name:      "upstream",
		BaseURL:   upstream.URL,
		PluginKey: "http-relay-station",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "route-key",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	if err := db.Create(&models.SystemSetting{ID: 1, GatewayAPIKey: "gateway-key"}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}

	router := NewRouter(db, config.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/gateway/models", nil)
	req.Header.Set("Authorization", "Bearer gateway-key")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body == "" || body == "{}" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestGatewaySub2APIProxyPathUsesModelProbeStrategy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/gateway/sub2api/v1/models", nil)
	if got := gatewayProxyTargetPath(req.URL.Path); got != "models" {
		t.Fatalf("target path = %q", got)
	}
	if got := gatewayModelProbeStrategy(req); got != "sub2api" {
		t.Fatalf("model probe strategy = %q", got)
	}
}

func TestGatewayRootV1PathUsesSub2APIModelProbeStrategy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	if got := gatewayProxyTargetPath(req.URL.Path); got != "models" {
		t.Fatalf("target path = %q", got)
	}
	if got := gatewayModelProbeStrategy(req); got != "sub2api" {
		t.Fatalf("model probe strategy = %q", got)
	}

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if got := gatewayProxyTargetPath(chatReq.URL.Path); got != "chat/completions" {
		t.Fatalf("chat target path = %q", got)
	}

	responsesReq := httptest.NewRequest(http.MethodPost, "/responses", nil)
	if got := gatewayProxyTargetPath(responsesReq.URL.Path); got != "responses" {
		t.Fatalf("responses target path = %q", got)
	}

	responsesSubpathReq := httptest.NewRequest(http.MethodPost, "/responses/compact", nil)
	if got := gatewayProxyTargetPath(responsesSubpathReq.URL.Path); got != "responses/compact" {
		t.Fatalf("responses subpath target path = %q", got)
	}
}

func TestGatewayBareResponsesAliasRoutesToUpstreamV1Responses(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer route-key" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","model":"gpt-5.5","output":[]}`))
	}))
	defer upstream.Close()

	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/gateway_bare_responses.db"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	site := models.Site{
		Name:      "responses-upstream",
		BaseURL:   upstream.URL,
		PluginKey: "http-relay-station",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "route-key",
		},
		PluginConfig: models.JSONMap{
			"api_format": "codex",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := db.Create(&models.SystemSetting{ID: 1, GatewayAPIKey: "gateway-key"}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	if _, err := services.SyncGatewayRoutes(db); err != nil {
		t.Fatalf("sync routes: %v", err)
	}

	router := NewRouter(db, config.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{"model":"gpt-5.5","input":"ping"}`))
	req.Header.Set("Authorization", "Bearer gateway-key")
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `"id":"resp_1"`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestGatewayRootV1ModelsServesSub2APIHealthProbe(t *testing.T) {
	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/gateway_root_v1.db"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	site := models.Site{
		Name:      "root-v1",
		BaseURL:   "https://upstream.example",
		PluginKey: "http-relay-station",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "route-key",
		},
		PluginConfig: models.JSONMap{
			"supported_models": []any{"gpt-5.5"},
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := db.Create(&models.SystemSetting{ID: 1, GatewayAPIKey: "gateway-key"}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}

	router := NewRouter(db, config.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer gateway-key")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `"id":"gpt-5.5"`) || !strings.Contains(body, `"object":"list"`) {
		t.Fatalf("unexpected body: %s", body)
	}
	if got := rec.Header().Get("X-Gateway-Model-Probe"); got != "sub2api" {
		t.Fatalf("X-Gateway-Model-Probe = %q", got)
	}
}

func TestGatewayProxyRejectsMissingGatewayAPIKey(t *testing.T) {
	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/gateway_missing_key.db"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if err := db.Create(&models.SystemSetting{ID: 1}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}

	router := NewRouter(db, config.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/gateway/models", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestGatewayProxyRejectsOversizedRequestWith413(t *testing.T) {
	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/gateway_oversized_request.db"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if err := db.Create(&models.SystemSetting{ID: 1, GatewayAPIKey: "gateway-key"}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}

	router := NewRouter(db, config.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/chat/completions", strings.NewReader(`{}`))
	req.ContentLength = 1 << 62
	req.Header.Set("Authorization", "Bearer gateway-key")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestGatewayProxyReturnsUnifiedErrorWhenAllRoutesFail(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream secret failure", http.StatusBadRequest)
	}))
	defer upstream.Close()

	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/gateway_all_failed.db"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if err := db.Create(&models.SystemSetting{ID: 1, GatewayAPIKey: "gateway-key", GatewayFailureRetryMode: "all"}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	for _, name := range []string{"a", "b"} {
		site := models.Site{
			Name:      name,
			BaseURL:   upstream.URL,
			PluginKey: "http-relay-station",
			IsEnabled: true,
			Credentials: models.JSONMap{
				"api_key": name + "-key",
			},
		}
		if err := db.Create(&site).Error; err != nil {
			t.Fatalf("create site: %v", err)
		}
	}

	router := NewRouter(db, config.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/gateway/models", nil)
	req.Header.Set("Authorization", "Bearer gateway-key")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "upstream secret failure") {
		t.Fatalf("upstream body leaked to client: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "网关路由池全部失败") {
		t.Fatalf("missing gateway failure message: %s", rec.Body.String())
	}
}

func TestDiagnoseGatewayRouteReportsBlockingItems(t *testing.T) {
	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/gateway_diagnose.db"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if err := db.Create(&models.SystemSetting{ID: 1, GatewayRouteConcurrencyLimit: 5}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	site := models.Site{
		Name:        "no-key",
		BaseURL:     "https://example.test",
		PluginKey:   "http-relay-station",
		IsEnabled:   true,
		Credentials: models.JSONMap{},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	state := models.GatewayRouteState{
		SiteID:              site.ID,
		KeyFingerprint:      "missing-key",
		KeyName:             "missing",
		SiteNameSnapshot:    site.Name,
		SiteBaseURLSnapshot: site.BaseURL,
		RouteType:           "codex",
		IsEnabled:           true,
		CircuitState:        "closed",
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatalf("create route state: %v", err)
	}

	app := &App{DB: db}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/gateway-admin/routes/1/diagnose", nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("routeID", strconv.Itoa(int(state.ID)))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
	app.DiagnoseGatewayRoute(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["healthy"] != false {
		t.Fatalf("healthy = %v", payload["healthy"])
	}
	items := payload["diagnostics"].([]any)
	foundMissingKey := false
	for _, item := range items {
		entry := item.(map[string]any)
		if entry["label"] == "API Key" && entry["severity"] == "error" {
			foundMissingKey = true
		}
	}
	if !foundMissingKey {
		t.Fatalf("missing API Key diagnostic not found: %#v", items)
	}
}

func TestGatewayRouteBulkEnableDisable(t *testing.T) {
	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/gateway_bulk.db"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	site := models.Site{
		Name:      "bulk",
		BaseURL:   "https://bulk.example",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{"name": "first", "key": "bulk-first", "status": "active"},
				map[string]any{"name": "second", "key": "bulk-second", "status": "active"},
				map[string]any{"name": "third", "key": "bulk-third", "status": "active"},
			},
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &App{DB: db}
	router := chi.NewRouter()
	router.Post("/routes/disable-all", app.DisableAllGatewayRoutes)
	router.Post("/routes/{routeID}/enable-only", app.EnableOnlyGatewayRoute)

	disableRec := httptest.NewRecorder()
	router.ServeHTTP(disableRec, httptest.NewRequest(http.MethodPost, "/routes/disable-all", nil))
	if disableRec.Code != http.StatusOK {
		t.Fatalf("disable all status = %d body = %s", disableRec.Code, disableRec.Body.String())
	}
	var enabledCount int64
	if err := db.Model(&models.GatewayRouteState{}).Where("is_enabled = ?", true).Count(&enabledCount).Error; err != nil {
		t.Fatal(err)
	}
	if enabledCount != 0 {
		t.Fatalf("enabled routes after disable all = %d", enabledCount)
	}

	var target models.GatewayRouteState
	if err := db.Where("key_name = ?", "second").First(&target).Error; err != nil {
		t.Fatal(err)
	}
	enableOnlyRec := httptest.NewRecorder()
	router.ServeHTTP(enableOnlyRec, httptest.NewRequest(http.MethodPost, "/routes/"+strconv.FormatUint(uint64(target.ID), 10)+"/enable-only", nil))
	if enableOnlyRec.Code != http.StatusOK {
		t.Fatalf("enable only status = %d body = %s", enableOnlyRec.Code, enableOnlyRec.Body.String())
	}

	var states []models.GatewayRouteState
	if err := db.Order("key_name asc").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	if len(states) != 3 {
		t.Fatalf("route state count = %d", len(states))
	}
	for _, state := range states {
		if state.ID == target.ID && !state.IsEnabled {
			t.Fatalf("target route %d was not enabled", target.ID)
		}
		if state.ID != target.ID && state.IsEnabled {
			t.Fatalf("route %d unexpectedly enabled", state.ID)
		}
	}
}
