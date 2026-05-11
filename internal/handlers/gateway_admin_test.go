package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/services"
	"github.com/glebarez/sqlite"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func TestGatewayTotalBalanceUsesEnabledRouteUnits(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:gateway-total-balance?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	sites := []models.Site{
		{ID: 1, Name: "enabled-usd", BaseURL: "https://enabled-usd.example", PluginKey: "http-relay-station", IsEnabled: true},
		{ID: 2, Name: "enabled-cny", BaseURL: "https://enabled-cny.example", PluginKey: "http-relay-station", IsEnabled: true},
		{ID: 3, Name: "enabled-disabled-route", BaseURL: "https://enabled-disabled-route.example", PluginKey: "http-relay-station", IsEnabled: true},
		{ID: 4, Name: "disabled-site", BaseURL: "https://disabled-site.example", PluginKey: "http-relay-station", IsEnabled: false},
	}
	if err := db.Create(&sites).Error; err != nil {
		t.Fatalf("create sites: %v", err)
	}

	usdPrimary := 10.5
	usdDisabled := 1.25
	cny := 20.0
	cnyDisabled := 400.0
	routes := []models.GatewayRouteState{
		{SiteID: 1, KeyFingerprint: "key-a", LastBalance: &usdPrimary, BalanceUnit: "USD", IsEnabled: true},
		{SiteID: 2, KeyFingerprint: "key-b", LastBalance: &cny, BalanceUnit: "CNY", IsEnabled: true},
		{SiteID: 3, KeyFingerprint: "key-c", LastBalance: &usdDisabled, BalanceUnit: "$", IsEnabled: false},
		{SiteID: 4, KeyFingerprint: "key-d", LastBalance: &cnyDisabled, BalanceUnit: "CNY", IsEnabled: true},
	}
	if err := db.Create(&routes).Error; err != nil {
		t.Fatalf("create routes: %v", err)
	}
	if err := db.Model(&models.GatewayRouteState{}).Where("site_id = ?", uint(3)).Update("is_enabled", false).Error; err != nil {
		t.Fatalf("disable routes: %v", err)
	}

	display, count := totalBalanceForRoutes(db)
	if count != 2 {
		t.Fatalf("count = %d", count)
	}
	if display != "$10.5 / ¥20" {
		t.Fatalf("display = %v", display)
	}
}

func TestUpdateGatewayRouteTypePersistsManualRequestBaseURLs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:gateway-route-manual-url?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	site := models.Site{
		Name:      "manual-url-site",
		BaseURL:   "https://site.example",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{
					"name":       "route key",
					"key":        "route-secret",
					"status":     "active",
					"route_type": "codex",
				},
			},
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	state := models.GatewayRouteState{
		SiteID:              site.ID,
		KeyFingerprint:      testGatewayRouteFingerprint("route-secret"),
		RouteType:           "codex",
		SupportedModels:     services.EncodeGatewaySupportedModels([]string{"gpt-5.5"}),
		SiteNameSnapshot:    site.Name,
		SiteBaseURLSnapshot: site.BaseURL,
		SiteAPIURLSnapshot:  `["https://old.example/v1"]`,
		LastRequestBaseURL:  "https://last.example",
		IsEnabled:           true,
		CircuitState:        "closed",
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"route_type":               "claude",
		"route_path":               "chat/completions",
		"supported_models":         []string{"claude-sonnet-4-6"},
		"manual_request_base_urls": []string{"https://claude.example/v1"},
	})
	app := &App{DB: db}
	router := chi.NewRouter()
	router.Patch("/routes/{routeID}/type", app.UpdateGatewayRouteType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/routes/1/type", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["request_base_url"] != "https://claude.example/v1" {
		t.Fatalf("request_base_url = %v", response["request_base_url"])
	}
	if response["last_request_base_url"] != "" {
		t.Fatalf("last_request_base_url = %v", response["last_request_base_url"])
	}
	var stored models.GatewayRouteState
	if err := db.First(&stored, state.ID).Error; err != nil {
		t.Fatalf("reload route: %v", err)
	}
	if got := services.GatewayRouteManualRequestBaseURLs(stored, site); len(got) != 1 || got[0] != "https://claude.example/v1" {
		t.Fatalf("stored manual urls = %v", got)
	}
	if stored.RouteType != "claude" || !stored.RouteTypeManual {
		t.Fatalf("route type manual = %s/%v", stored.RouteType, stored.RouteTypeManual)
	}
	if stored.RoutePath != "chat/completions" || !stored.RoutePathManual {
		t.Fatalf("route path manual = %s/%v", stored.RoutePath, stored.RoutePathManual)
	}
	var storedSite models.Site
	if err := db.First(&storedSite, site.ID).Error; err != nil {
		t.Fatalf("reload site: %v", err)
	}
	apiKeys, ok := storedSite.Credentials["api_keys"].([]any)
	if !ok || len(apiKeys) != 1 {
		t.Fatalf("api_keys = %#v", storedSite.Credentials["api_keys"])
	}
	apiKey, ok := apiKeys[0].(map[string]any)
	if !ok {
		t.Fatalf("api key type = %T", apiKeys[0])
	}
	urls, ok := apiKey["request_base_urls"].([]any)
	if !ok {
		t.Fatalf("api key request_base_urls = %#v", apiKey["request_base_urls"])
	}
	if len(urls) != 1 || urls[0] != "https://claude.example/v1" {
		t.Fatalf("api key request_base_urls = %#v", urls)
	}

	clearBody, _ := json.Marshal(map[string]any{
		"route_type":               "claude",
		"route_path":               "chat/completions",
		"supported_models":         []string{"claude-sonnet-4-6"},
		"manual_request_base_urls": []string{},
	})
	clearRec := httptest.NewRecorder()
	router.ServeHTTP(clearRec, httptest.NewRequest(http.MethodPatch, "/routes/1/type", bytes.NewReader(clearBody)))
	if clearRec.Code != http.StatusOK {
		t.Fatalf("clear status = %d body = %s", clearRec.Code, clearRec.Body.String())
	}
	var clearResponse map[string]any
	if err := json.Unmarshal(clearRec.Body.Bytes(), &clearResponse); err != nil {
		t.Fatalf("decode clear response: %v", err)
	}
	if clearResponse["request_base_url"] != "https://site.example" {
		t.Fatalf("cleared request_base_url = %v", clearResponse["request_base_url"])
	}
	if err := db.First(&storedSite, site.ID).Error; err != nil {
		t.Fatalf("reload cleared site: %v", err)
	}
	apiKeys = storedSite.Credentials["api_keys"].([]any)
	apiKey = apiKeys[0].(map[string]any)
	if _, ok := apiKey["request_base_urls"]; ok {
		t.Fatalf("cleared api key request_base_urls = %#v", apiKey["request_base_urls"])
	}
}

func TestUpdateGatewayRouteTypeResponseKeepsAPIKeyStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:gateway-route-path-keeps-key?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	site := models.Site{
		Name:      "path-format-site",
		BaseURL:   "https://site.example",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{
					"name":       "route key",
					"key":        "route-secret",
					"status":     "active",
					"route_type": "gpt",
				},
			},
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	state := models.GatewayRouteState{
		SiteID:              site.ID,
		KeyFingerprint:      testGatewayRouteFingerprint("route-secret"),
		RouteType:           "gpt",
		SupportedModels:     services.EncodeGatewaySupportedModels([]string{"gpt-5.5"}),
		SiteNameSnapshot:    site.Name,
		SiteBaseURLSnapshot: site.BaseURL,
		IsEnabled:           true,
		CircuitState:        "closed",
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"route_type":       "gpt",
		"route_path":       "chat/completions",
		"supported_models": []string{"gpt-5.5"},
	})
	app := &App{DB: db}
	router := chi.NewRouter()
	router.Patch("/routes/{routeID}/type", app.UpdateGatewayRouteType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/routes/1/type", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["has_api_key"] != true {
		t.Fatalf("has_api_key = %v", response["has_api_key"])
	}
	if response["route_path"] != "chat/completions" {
		t.Fatalf("route_path = %v", response["route_path"])
	}
}

func TestUpdateGatewayRouteTypeAcceptsGptChatAlias(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:gateway-route-gpt-chat-alias?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	state := models.GatewayRouteState{
		SiteID:         1,
		KeyFingerprint: testGatewayRouteFingerprint("route-secret"),
		RouteType:      "codex",
		IsEnabled:      true,
		CircuitState:   "closed",
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"route_type": "gpt_chat"})
	app := &App{DB: db}
	router := chi.NewRouter()
	router.Patch("/routes/{routeID}/type", app.UpdateGatewayRouteType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/routes/1/type", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["route_type"] != "gpt" {
		t.Fatalf("route_type = %v", response["route_type"])
	}
}

func TestGatewayUsageCostSummaryUsesModelInputCacheOutput(t *testing.T) {
	prompt, cached, completion, total := 1000, 250, 500, 1500
	logs := []models.GatewayRequestLog{
		{
			Model:             "gpt-5.5",
			PromptTokens:      &prompt,
			CachedInputTokens: &cached,
			CompletionTokens:  &completion,
			TotalTokens:       &total,
			Success:           true,
			CreatedAt:         time.Now().UTC(),
		},
	}

	summary := gatewayUsageCostSummary(logs)
	if got := summary["input_cost"]; got != 0.00375 {
		t.Fatalf("input_cost = %v", got)
	}
	if got := summary["cached_cost"]; got != 0.000125 {
		t.Fatalf("cached_cost = %v", got)
	}
	if got := summary["output_cost"]; got != 0.015 {
		t.Fatalf("output_cost = %v", got)
	}
	if got := summary["total_cost"]; got != 0.018875 {
		t.Fatalf("total_cost = %v", got)
	}
	if got := summary["known_requests"]; got != 1 {
		t.Fatalf("known_requests = %v", got)
	}
	if got := summary["unknown_requests"]; got != 0 {
		t.Fatalf("unknown_requests = %v", got)
	}
}

func TestGatewayLogResponseIncludesUserAgent(t *testing.T) {
	app := &App{}
	const upstreamUserAgent = "ConfiguredBrowser/1.0"
	logs := []models.GatewayRequestLog{
		{
			ID:        1,
			UserAgent: upstreamUserAgent,
			CreatedAt: time.Now().UTC(),
		},
	}

	response := app.gatewayLogResponse(logs)
	if len(response) != 1 {
		t.Fatalf("response length = %d", len(response))
	}
	if got := response[0]["user_agent"]; got != upstreamUserAgent {
		t.Fatalf("user_agent = %v, want %q", got, upstreamUserAgent)
	}
}

func TestGatewayOverviewIncludesConcurrencyPeaks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:gateway-overview-concurrency-peaks?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	if err := db.Create(&models.SystemSetting{ID: 1, GatewayRouteConcurrencyLimit: 5}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	now := time.Now()
	if err := db.Create(&models.GatewayConcurrencyPeak{Day: "all", MaxConcurrency: 9, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("create all-time peak: %v", err)
	}
	if err := db.Create(&models.GatewayConcurrencyPeak{Day: now.In(time.Local).Format("2006-01-02"), MaxConcurrency: 4, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("create today peak: %v", err)
	}

	app := &App{DB: db}
	rec := httptest.NewRecorder()
	app.GatewayOverview(rec, httptest.NewRequest(http.MethodGet, "/gateway-admin/overview", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("overview status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["max_concurrency_all_time"] != float64(9) {
		t.Fatalf("max_concurrency_all_time = %v", response["max_concurrency_all_time"])
	}
	if response["max_concurrency_today"] != float64(4) {
		t.Fatalf("max_concurrency_today = %v", response["max_concurrency_today"])
	}
}
