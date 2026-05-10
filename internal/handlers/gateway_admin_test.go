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

func TestGatewayTotalBalanceUsesRouteUnits(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:gateway-total-balance?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	usdPrimary := 10.5
	usdDisabled := 1.25
	cny := 20.0
	routes := []models.GatewayRouteState{
		{SiteID: 1, KeyFingerprint: "key-a", LastBalance: &usdPrimary, BalanceUnit: "USD", IsEnabled: true},
		{SiteID: 2, KeyFingerprint: "key-b", LastBalance: &cny, BalanceUnit: "CNY", IsEnabled: true},
		{SiteID: 3, KeyFingerprint: "key-c", LastBalance: &usdDisabled, BalanceUnit: "$", IsEnabled: false},
	}
	if err := db.Create(&routes).Error; err != nil {
		t.Fatalf("create routes: %v", err)
	}

	display, count := totalBalanceForRoutes(db)
	if count != 3 {
		t.Fatalf("count = %d", count)
	}
	if display != "$11.75 / ¥20" {
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
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	state := models.GatewayRouteState{
		SiteID:              site.ID,
		KeyFingerprint:      "key-a",
		RouteType:           "codex",
		SupportedModels:     services.EncodeGatewaySupportedModels([]string{"gpt-5.5"}),
		SiteNameSnapshot:    site.Name,
		SiteBaseURLSnapshot: site.BaseURL,
		SiteAPIURLSnapshot:  "[]",
		LastRequestBaseURL:  "https://last.example",
		IsEnabled:           true,
		CircuitState:        "closed",
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"route_type":               "claude",
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
