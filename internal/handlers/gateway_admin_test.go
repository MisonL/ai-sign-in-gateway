package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

func TestGatewayRouteGroupManagementAssignsRouteToMultipleGroups(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:gateway-route-groups-admin?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	site := models.Site{
		Name:      "group-admin-site",
		BaseURL:   "https://group-admin.example",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "route-secret",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	state := models.GatewayRouteState{
		SiteID:         site.ID,
		KeyFingerprint: testGatewayRouteFingerprint("route-secret"),
		RouteType:      "codex",
		IsEnabled:      true,
		CircuitState:   "closed",
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}

	app := &App{DB: db}
	router := chi.NewRouter()
	router.Post("/route-groups", app.CreateGatewayRouteGroup)
	router.Put("/routes/{routeID}/groups", app.UpdateGatewayRouteGroups)
	router.Get("/routes", app.GatewayRoutes)

	firstBody, _ := json.Marshal(map[string]any{"name": "fast", "api_key": "fast-key"})
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, httptest.NewRequest(http.MethodPost, "/route-groups", bytes.NewReader(firstBody)))
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("create first status = %d body = %s", firstRec.Code, firstRec.Body.String())
	}
	var firstGroup map[string]any
	if err := json.Unmarshal(firstRec.Body.Bytes(), &firstGroup); err != nil {
		t.Fatalf("decode first group: %v", err)
	}

	secondBody, _ := json.Marshal(map[string]any{"name": "cheap"})
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, httptest.NewRequest(http.MethodPost, "/route-groups", bytes.NewReader(secondBody)))
	if secondRec.Code != http.StatusCreated {
		t.Fatalf("create second status = %d body = %s", secondRec.Code, secondRec.Body.String())
	}
	var secondGroup map[string]any
	if err := json.Unmarshal(secondRec.Body.Bytes(), &secondGroup); err != nil {
		t.Fatalf("decode second group: %v", err)
	}

	assignBody, _ := json.Marshal(map[string]any{
		"group_ids": []uint{uint(firstGroup["id"].(float64)), uint(secondGroup["id"].(float64))},
	})
	assignRec := httptest.NewRecorder()
	router.ServeHTTP(assignRec, httptest.NewRequest(http.MethodPut, "/routes/1/groups", bytes.NewReader(assignBody)))
	if assignRec.Code != http.StatusOK {
		t.Fatalf("assign status = %d body = %s", assignRec.Code, assignRec.Body.String())
	}

	routesRec := httptest.NewRecorder()
	router.ServeHTTP(routesRec, httptest.NewRequest(http.MethodGet, "/routes", nil))
	if routesRec.Code != http.StatusOK {
		t.Fatalf("routes status = %d body = %s", routesRec.Code, routesRec.Body.String())
	}
	var routes []map[string]any
	if err := json.Unmarshal(routesRec.Body.Bytes(), &routes); err != nil {
		t.Fatalf("decode routes: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("routes len = %d", len(routes))
	}
	groups, ok := routes[0]["groups"].([]any)
	if !ok || len(groups) != 2 {
		t.Fatalf("route groups = %#v", routes[0]["groups"])
	}
	if routes[0]["group_name"] != "cheap, fast" && routes[0]["group_name"] != "fast, cheap" {
		t.Fatalf("group_name = %v", routes[0]["group_name"])
	}
}

func TestDeleteGatewayRouteRemovesMatchingSiteAPIKeyAndGroupMembership(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:gateway-route-delete-api-key?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	site := models.Site{
		Name:      "delete-route-site",
		BaseURL:   "https://delete-route.example",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{"name": "keep", "key": "keep-secret", "status": "active", "route_type": "codex"},
				map[string]any{"name": "delete", "key": "delete-secret", "status": "active", "route_type": "claude"},
			},
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	route := models.GatewayRouteState{
		SiteID:              site.ID,
		KeyFingerprint:      testGatewayRouteFingerprint("delete-secret"),
		KeyName:             "delete",
		KeySource:           "site.credentials.api_keys",
		SiteNameSnapshot:    site.Name,
		SiteBaseURLSnapshot: site.BaseURL,
		RouteType:           "claude",
		IsEnabled:           true,
		CircuitState:        "closed",
	}
	if err := db.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	keepRoute := models.GatewayRouteState{
		SiteID:              site.ID,
		KeyFingerprint:      testGatewayRouteFingerprint("keep-secret"),
		KeyName:             "keep",
		KeySource:           "site.credentials.api_keys",
		SiteNameSnapshot:    site.Name,
		SiteBaseURLSnapshot: site.BaseURL,
		RouteType:           "codex",
		IsEnabled:           true,
		CircuitState:        "closed",
	}
	if err := db.Create(&keepRoute).Error; err != nil {
		t.Fatalf("create keep route: %v", err)
	}
	group := models.GatewayRouteGroup{Name: "fast", APIKey: "group-key"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := db.Create(&models.GatewayRouteGroupMember{GroupID: group.ID, RouteStateID: route.ID}).Error; err != nil {
		t.Fatalf("create route group member: %v", err)
	}

	app := &App{DB: db}
	router := chi.NewRouter()
	router.Delete("/routes/{routeID}", app.DeleteGatewayRoute)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/routes/1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", rec.Code, rec.Body.String())
	}

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["removed_api_key"] != true {
		t.Fatalf("removed_api_key = %v", response["removed_api_key"])
	}

	var deleted models.GatewayRouteState
	if err := db.First(&deleted, route.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("deleted route lookup err = %v", err)
	}
	var remaining models.GatewayRouteState
	if err := db.First(&remaining, keepRoute.ID).Error; err != nil {
		t.Fatalf("remaining route missing: %v", err)
	}
	var memberCount int64
	if err := db.Model(&models.GatewayRouteGroupMember{}).Where("route_state_id = ?", route.ID).Count(&memberCount).Error; err != nil {
		t.Fatalf("count route members: %v", err)
	}
	if memberCount != 0 {
		t.Fatalf("route group member count = %d", memberCount)
	}

	var storedSite models.Site
	if err := db.First(&storedSite, site.ID).Error; err != nil {
		t.Fatalf("reload site: %v", err)
	}
	apiKeys, ok := storedSite.Credentials["api_keys"].([]any)
	if !ok {
		t.Fatalf("api_keys type = %T", storedSite.Credentials["api_keys"])
	}
	if len(apiKeys) != 1 {
		t.Fatalf("api_keys len = %d values = %#v", len(apiKeys), apiKeys)
	}
	kept, ok := apiKeys[0].(map[string]any)
	if !ok {
		t.Fatalf("api key type = %T", apiKeys[0])
	}
	if kept["key"] != "keep-secret" {
		t.Fatalf("remaining api key = %#v", kept)
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

	summary := gatewayUsageCostSummary(logs, services.OfficialGatewayPricingScheme())
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

func TestGatewayUsageCostSummaryUsesCustomPricingScheme(t *testing.T) {
	prompt, cached, completion, total := 1000, 100, 500, 1500
	logs := []models.GatewayRequestLog{
		{
			Model:             "claude-custom",
			RouteType:         "claude",
			PromptTokens:      &prompt,
			CachedInputTokens: &cached,
			CompletionTokens:  &completion,
			TotalTokens:       &total,
			Success:           true,
			CreatedAt:         time.Now().UTC(),
		},
	}
	pricing := models.GatewayPricingScheme{
		ID:       "custom",
		Name:     "custom",
		Currency: "USD",
		Prices: []models.GatewayModelPrice{
			{Provider: "claude", ModelPrefix: "claude-custom", InputPerMTok: 10, CachedInputPerMTok: 1, OutputPerMTok: 20},
		},
	}

	summary := gatewayUsageCostSummary(logs, pricing)
	if got := summary["total_cost"]; got != 0.0201 {
		t.Fatalf("total_cost = %v", got)
	}
	if got := summary["known_requests"]; got != 1 {
		t.Fatalf("known_requests = %v", got)
	}
}

func TestGatewayUsageStreamsAndPreservesRouteGrouping(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:gateway-usage-stream?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	if err := db.Create(&models.SystemSetting{ID: 1, GatewayPricingActiveSchemeID: services.OfficialGatewayPricingSchemeID}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	site := models.Site{
		Name:      "usage-site",
		BaseURL:   "https://usage.example",
		PluginKey: "api-supplier",
		GroupName: "usage-group",
		IsEnabled: true,
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	state := models.GatewayRouteState{
		SiteID:         site.ID,
		KeyFingerprint: "fingerprint-a",
		KeyName:        "usage-key",
		GroupName:      "usage-group",
		RouteType:      "codex",
		IsEnabled:      true,
		CircuitState:   "closed",
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatalf("create route state: %v", err)
	}

	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	end := start.Add(2 * time.Hour)
	latency := 120.0
	prompt, completion, totalTokens := 1000, 500, 1500
	logs := []models.GatewayRequestLog{
		{
			RequestID:        "usage-a",
			RouteStateID:     &state.ID,
			SiteID:           &site.ID,
			KeyFingerprint:   state.KeyFingerprint,
			KeyName:          state.KeyName,
			GroupName:        state.GroupName,
			RouteType:        "codex",
			RequestedModel:   "gpt-5.1",
			Success:          true,
			LatencyMS:        &latency,
			PromptTokens:     &prompt,
			CompletionTokens: &completion,
			TotalTokens:      &totalTokens,
			IsStream:         true,
			CreatedAt:        start.Add(time.Minute),
		},
		{
			RequestID:      "usage-b",
			SiteID:         &site.ID,
			KeyFingerprint: state.KeyFingerprint,
			KeyName:        state.KeyName,
			GroupName:      state.GroupName,
			RouteType:      "codex",
			RequestedModel: "gpt-5.1",
			Success:        false,
			CreatedAt:      start.Add(2 * time.Minute),
		},
		{
			RequestID:      "usage-old",
			SiteID:         &site.ID,
			KeyFingerprint: state.KeyFingerprint,
			Success:        true,
			CreatedAt:      start.Add(-time.Second),
		},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatalf("create logs: %v", err)
	}

	app := &App{DB: db}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/gateway-admin/usage?start="+start.Format(time.RFC3339)+"&end="+end.Format(time.RFC3339), nil)
	app.GatewayUsage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("usage status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["request_count"] != float64(2) || response["success_count"] != float64(1) || response["failure_count"] != float64(1) {
		t.Fatalf("usage totals = %#v", response)
	}
	if response["stream_request_count"] != float64(1) || response["total_tokens"] != float64(1500) {
		t.Fatalf("usage token totals = %#v", response)
	}
	routes, ok := response["routes"].([]any)
	if !ok || len(routes) != 1 {
		t.Fatalf("routes = %#v", response["routes"])
	}
	route, ok := routes[0].(map[string]any)
	if !ok {
		t.Fatalf("route type = %T", routes[0])
	}
	if route["route_id"] != float64(state.ID) || route["site_name"] != site.Name || route["request_count"] != float64(2) {
		t.Fatalf("route usage = %#v", route)
	}
	if route["avg_latency_ms"] != float64(120) || route["model"] != "gpt-5.1" || route["route_type"] != "codex" {
		t.Fatalf("route details = %#v", route)
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

func TestGatewayLogsFilterByStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:gateway-logs-status-filter?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	site := models.Site{Name: "status-filter-site", BaseURL: "https://status-filter.example", PluginKey: "api-supplier", IsEnabled: true}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	route := models.GatewayRouteState{
		SiteID:         site.ID,
		KeyFingerprint: "status-filter-key",
		KeyName:        "status-filter",
		RouteType:      "codex",
		IsEnabled:      true,
		CircuitState:   "closed",
	}
	if err := db.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	failureReason := "upstream status filter failed"
	now := time.Now().UTC().Truncate(time.Second)
	logs := []models.GatewayRequestLog{
		{
			RequestID:      "status-filter-success",
			RouteStateID:   &route.ID,
			SiteID:         &site.ID,
			KeyFingerprint: route.KeyFingerprint,
			Success:        true,
			CreatedAt:      now,
		},
		{
			RequestID:      "status-filter-error",
			RouteStateID:   &route.ID,
			SiteID:         &site.ID,
			KeyFingerprint: route.KeyFingerprint,
			Success:        false,
			FailureReason:  &failureReason,
			CreatedAt:      now.Add(time.Second),
		},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatalf("create logs: %v", err)
	}

	app := &App{DB: db}
	rec := httptest.NewRecorder()
	app.GatewayLogs(rec, httptest.NewRequest(http.MethodGet, "/gateway-admin/logs?status=error", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) != 1 || response[0]["request_id"] != "status-filter-error" {
		t.Fatalf("error filtered logs = %#v", response)
	}

	successRec := httptest.NewRecorder()
	app.GatewayLogs(successRec, httptest.NewRequest(http.MethodGet, "/gateway-admin/logs?status=success", nil))
	if successRec.Code != http.StatusOK {
		t.Fatalf("success status = %d body = %s", successRec.Code, successRec.Body.String())
	}
	response = nil
	if err := json.Unmarshal(successRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode success response: %v", err)
	}
	if len(response) != 1 || response[0]["request_id"] != "status-filter-success" {
		t.Fatalf("success filtered logs = %#v", response)
	}

	routeRec := httptest.NewRecorder()
	routeReq := gatewayAdminRequestWithRouteParam(httptest.NewRequest(http.MethodGet, "/gateway-admin/routes/1/logs?status=error", nil), "routeID", strconv.FormatUint(uint64(route.ID), 10))
	app.GatewayRouteLogs(routeRec, routeReq)
	if routeRec.Code != http.StatusOK {
		t.Fatalf("route status = %d body = %s", routeRec.Code, routeRec.Body.String())
	}
	response = nil
	if err := json.Unmarshal(routeRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode route response: %v", err)
	}
	if len(response) != 1 || response[0]["request_id"] != "status-filter-error" {
		t.Fatalf("route error filtered logs = %#v", response)
	}
}

func TestGatewayLogResponseIncludesFailureTransferChain(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:gateway-log-transfer-chain?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	sites := []models.Site{
		{Name: "failed-route", BaseURL: "https://failed-route.example", PluginKey: "api-supplier", IsEnabled: true},
		{Name: "fallback-route", BaseURL: "https://fallback-route.example", PluginKey: "api-supplier", IsEnabled: true},
	}
	if err := db.Create(&sites).Error; err != nil {
		t.Fatalf("create sites: %v", err)
	}
	routes := []models.GatewayRouteState{
		{SiteID: sites[0].ID, KeyFingerprint: "failed-fingerprint", KeyName: "failed-key", RouteType: "codex", IsEnabled: true, CircuitState: "closed"},
		{SiteID: sites[1].ID, KeyFingerprint: "fallback-fingerprint", KeyName: "fallback-key", RouteType: "codex", IsEnabled: true, CircuitState: "closed"},
	}
	if err := db.Create(&routes).Error; err != nil {
		t.Fatalf("create routes: %v", err)
	}
	status := http.StatusInternalServerError
	reason := "upstream failed token=raw-secret"
	now := time.Now().UTC().Truncate(time.Second)
	logs := []models.GatewayRequestLog{
		{
			RequestID:      "transfer-chain",
			RouteStateID:   &routes[0].ID,
			SiteID:         &sites[0].ID,
			KeyFingerprint: routes[0].KeyFingerprint,
			KeyName:        routes[0].KeyName,
			Method:         http.MethodPost,
			AttemptIndex:   1,
			StatusCode:     &status,
			Success:        false,
			FailureReason:  &reason,
			CreatedAt:      now,
		},
		{
			RequestID:      "transfer-chain",
			RouteStateID:   &routes[1].ID,
			SiteID:         &sites[1].ID,
			KeyFingerprint: routes[1].KeyFingerprint,
			KeyName:        routes[1].KeyName,
			Method:         http.MethodPost,
			AttemptIndex:   2,
			Success:        true,
			CreatedAt:      now.Add(time.Millisecond),
		},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatalf("create logs: %v", err)
	}

	app := &App{DB: db}
	response := app.gatewayLogResponse([]models.GatewayRequestLog{logs[0]})
	if len(response) != 1 {
		t.Fatalf("response length = %d", len(response))
	}
	if got := response[0]["related_attempt_count"]; got != 2 {
		t.Fatalf("related_attempt_count = %#v", got)
	}
	transfer, ok := response[0]["transfer_to"].(map[string]any)
	if !ok {
		t.Fatalf("missing transfer_to: %#v", response[0]["transfer_to"])
	}
	if got := fmt.Sprint(transfer["route_label"]); !strings.Contains(got, "fallback-route") || !strings.Contains(got, "fallback-key") {
		t.Fatalf("transfer route label = %s", got)
	}
	if got := transfer["attempt_index"]; got != 2 {
		t.Fatalf("transfer attempt_index = %#v", got)
	}
	if got := gatewayAdminTestString(response[0]["failure_reason"]); strings.Contains(got, "raw-secret") {
		t.Fatalf("failure_reason redaction = %s", got)
	}
	final, ok := response[0]["final_attempt"].(map[string]any)
	if !ok {
		t.Fatalf("missing final_attempt: %#v", response[0]["final_attempt"])
	}
	if got := final["success"]; got != true {
		t.Fatalf("final success = %#v", got)
	}
}

func TestGatewayActiveRequestsIncludesPreviousFailureChain(t *testing.T) {
	services.ResetGatewayCountersForTest()
	t.Cleanup(services.ResetGatewayCountersForTest)
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failed token=active-secret", http.StatusInternalServerError)
	}))
	defer failed.Close()

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer fallback.Close()

	db, err := gorm.Open(sqlite.Open("file:gateway-active-failure-chain?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	if err := db.Create(&models.SystemSetting{ID: 1}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	sites := []models.Site{
		{Name: "failed-active", BaseURL: failed.URL, PluginKey: "api-supplier", IsEnabled: true, Credentials: models.JSONMap{"api_key": "failed-key"}, PluginConfig: models.JSONMap{"api_format": "openai"}},
		{Name: "fallback-active", BaseURL: fallback.URL, PluginKey: "api-supplier", IsEnabled: true, Credentials: models.JSONMap{"api_key": "fallback-key"}, PluginConfig: models.JSONMap{"api_format": "openai"}},
	}
	if err := db.Create(&sites).Error; err != nil {
		t.Fatalf("create sites: %v", err)
	}
	if _, err := services.SyncGatewayRoutes(db); err != nil {
		t.Fatalf("sync routes: %v", err)
	}
	var states []models.GatewayRouteState
	if err := db.Order("site_id asc").Find(&states).Error; err != nil {
		t.Fatalf("load routes: %v", err)
	}
	for idx := range states {
		states[idx].SupportedModels = services.EncodeGatewaySupportedModels([]string{"gpt-4o"})
		states[idx].RoutePriority = idx + 1
		if err := db.Save(&states[idx]).Error; err != nil {
			t.Fatalf("save route %d: %v", idx, err)
		}
	}

	done := make(chan error, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "active-chain")
		result, err := services.ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", services.GatewayPolicy{
			RouteStrategy:    "priority",
			RequestTimeout:   5,
			FailureRetryMode: "all",
		})
		if err == nil && !result.Success {
			err = errors.New("proxy request failed")
		}
		done <- err
	}()

	select {
	case <-entered:
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("proxy returned before fallback request was observed")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for fallback request")
	}

	app := &App{DB: db}
	rec := httptest.NewRecorder()
	app.GatewayActiveRequests(rec, httptest.NewRequest(http.MethodGet, "/gateway-admin/active-requests?include_recent=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) < 1 {
		t.Fatalf("active response = %#v", response)
	}
	var active map[string]any
	for _, item := range response {
		if item["recent"] != true {
			active = item
			break
		}
	}
	if active == nil {
		t.Fatalf("missing active request: %#v", response)
	}
	previous, ok := active["previous_error"].(map[string]any)
	if !ok {
		t.Fatalf("missing previous_error: %#v", active)
	}
	if got := fmt.Sprint(previous["route_label"]); !strings.Contains(got, "failed-active") {
		t.Fatalf("previous route label = %s", got)
	}
	if got := gatewayAdminTestString(previous["failure_reason"]); strings.Contains(got, "active-secret") || !strings.Contains(got, "upstream failed") {
		t.Fatalf("previous failure reason = %s", got)
	}
	if got := active["related_attempt_count"]; got != float64(2) {
		t.Fatalf("related_attempt_count = %#v", got)
	}
	if got := active["final_attempt"]; got != nil {
		t.Fatalf("running request final_attempt = %#v", got)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestGatewayAdminResponsesRedactHistoricalFailureSecrets(t *testing.T) {
	secretReason := "failed token=history-secret cookie=session=history-secret"
	status := http.StatusUnauthorized
	app := &App{}
	logResponse := app.gatewayLogResponse([]models.GatewayRequestLog{
		{
			ID:            1,
			RequestURL:    "https://upstream.example/v1/chat/completions?debug=1&api_key=history-secret&token=history-token",
			StatusCode:    &status,
			FailureReason: &secretReason,
			CreatedAt:     time.Now().UTC(),
		},
	})
	if len(logResponse) != 1 {
		t.Fatalf("log response length = %d", len(logResponse))
	}
	if got := gatewayAdminTestString(logResponse[0]["failure_reason"]); strings.Contains(got, "history-secret") {
		t.Fatalf("failure_reason leaked secret: %s", got)
	}
	if got := fmt.Sprint(logResponse[0]["request_url"]); strings.Contains(got, "history-secret") || strings.Contains(got, "history-token") || !strings.Contains(got, "debug=1") {
		t.Fatalf("request_url redaction = %s", got)
	}

	lastError := `{"error":{"message":"bad token=route-history-secret","api_key":"raw-history-secret"}}`
	route := services.GatewayRoute{
		State: models.GatewayRouteState{
			ID:                1,
			LastError:         &lastError,
			ModelProbeMessage: lastError,
		},
	}
	routeResponse := gatewayRouteResponse(route)
	if got := gatewayAdminTestString(routeResponse["last_error"]); strings.Contains(got, "route-history-secret") || strings.Contains(got, "raw-history-secret") {
		t.Fatalf("last_error leaked secret: %s", got)
	}
	if got := fmt.Sprint(routeResponse["model_probe_message"]); strings.Contains(got, "route-history-secret") || strings.Contains(got, "raw-history-secret") {
		t.Fatalf("model_probe_message leaked secret: %s", got)
	}

	probeResponse := gatewayProbeResponse(services.GatewayProbeResult{Route: route, Message: lastError})
	if got := gatewayAdminTestString(probeResponse["last_error"]); strings.Contains(got, "route-history-secret") || strings.Contains(got, "raw-history-secret") {
		t.Fatalf("probe last_error leaked secret: %s", got)
	}
	if got := fmt.Sprint(probeResponse["message"]); strings.Contains(got, "route-history-secret") || strings.Contains(got, "raw-history-secret") {
		t.Fatalf("probe message leaked secret: %s", got)
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

func testIntPtr(value int) *int {
	return &value
}

func TestGatewayOverviewStatsAggregateFromDatabase(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:gateway-overview-db-agg?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	if err := db.Create(&models.SystemSetting{ID: 1, GatewayRouteConcurrencyLimit: 5}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	now := time.Now().UTC()
	latencyFast := 100.0
	latencySlow := 500.0
	logs := []models.GatewayRequestLog{
		{
			RequestID:        "req-a",
			Success:          true,
			LatencyMS:        &latencyFast,
			RouteStrategy:    "smart",
			IsStream:         true,
			RouteType:        "codex",
			RequestedModel:   "gpt-5.1",
			PromptTokens:     testIntPtr(1000),
			CompletionTokens: testIntPtr(500),
			TotalTokens:      testIntPtr(1500),
			CreatedAt:        now.Add(-time.Hour),
		},
		{
			RequestID:      "req-a",
			Success:        false,
			RouteStrategy:  "smart",
			IsStream:       true,
			RouteType:      "codex",
			RequestedModel: "gpt-5.1",
			CreatedAt:      now.Add(-50 * time.Minute),
		},
		{
			RequestID:      "req-b",
			Success:        true,
			LatencyMS:      &latencySlow,
			RouteStrategy:  "priority",
			RouteType:      "codex",
			RequestedModel: "gpt-5.1",
			CreatedAt:      now.Add(-30 * time.Minute),
		},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatalf("create logs: %v", err)
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
	if response["request_count_24h"] != float64(2) {
		t.Fatalf("request_count_24h = %v", response["request_count_24h"])
	}
	if response["success_rate_24h"] != float64(66.67) {
		t.Fatalf("success_rate_24h = %v", response["success_rate_24h"])
	}
	if response["avg_latency_ms_24h"] != float64(300) {
		t.Fatalf("avg_latency_ms_24h = %v", response["avg_latency_ms_24h"])
	}
	strategies, ok := response["strategy_breakdown_24h"].([]any)
	if !ok || len(strategies) != 2 {
		t.Fatalf("strategy_breakdown_24h = %#v", response["strategy_breakdown_24h"])
	}
	first, ok := strategies[0].(map[string]any)
	if !ok || first["route_strategy"] != "smart" || first["request_count"] != float64(2) || first["stream_request_count"] != float64(2) {
		t.Fatalf("smart strategy stats = %#v", first)
	}
	usageCost, ok := response["usage_cost_24h"].(map[string]any)
	if !ok || usageCost["known_requests"] != float64(3) || usageCost["total_tokens"] != float64(1500) {
		t.Fatalf("usage_cost_24h = %#v", response["usage_cost_24h"])
	}
}

func gatewayAdminTestString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func gatewayAdminRequestWithRouteParam(req *http.Request, key, value string) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}
