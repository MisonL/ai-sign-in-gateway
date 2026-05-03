package services

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ai-sign-in-gateway/internal/config"
	"ai-sign-in-gateway/internal/database"
	"ai-sign-in-gateway/internal/migrations"
	"ai-sign-in-gateway/internal/models"
	"gorm.io/gorm"
)

func TestGatewaySyncAndProxy(t *testing.T) {
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

	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/gateway.db"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close(db)
	if err := migrations.Apply(db); err != nil {
		t.Fatal(err)
	}
	site := models.Site{
		Name:      "upstream",
		BaseURL:   upstream.URL,
		PluginKey: "http-relay-station",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "route-key",
		},
		PluginConfig: models.JSONMap{},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	count, err := SyncGatewayRoutes(db)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("route count = %d", count)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	result, err := ProxyGatewayRequest(req.Context(), db, req, "models", "", "", GatewayPolicy{RequestTimeout: 5})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), "demo") {
		t.Fatalf("unexpected proxy result: status=%d body=%s", result.StatusCode, result.Body)
	}
}

func TestGatewayRoundRobinAndRetry(t *testing.T) {
	ResetGatewayCountersForTest()
	firstCalls := 0
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalls++
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer first.Close()
	secondCalls := 0
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCalls++
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer second.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "first", first.URL, "first-key")
	createGatewaySite(t, db, "second", second.URL, "second-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	result, err := ProxyGatewayRequest(req.Context(), db, req, "models", "", "", GatewayPolicy{
		RouteStrategy:    "round_robin",
		RequestTimeout:   5,
		MaxAttempts:      2,
		FailureThreshold: 1,
		CooldownSeconds:  60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected eventual 200, got status=%d", result.StatusCode)
	}
	if firstCalls+secondCalls != 2 || firstCalls == 0 || secondCalls == 0 {
		t.Fatalf("expected each route hit once, first=%d second=%d", firstCalls, secondCalls)
	}

	var firstSite models.Site
	if err := db.Where("name = ?", "first").First(&firstSite).Error; err != nil {
		t.Fatal(err)
	}
	var firstState models.GatewayRouteState
	if err := db.Where("site_id = ?", firstSite.ID).First(&firstState).Error; err != nil {
		t.Fatal(err)
	}
	if firstState.CircuitState != "open" {
		t.Fatalf("first route circuit = %q", firstState.CircuitState)
	}
}

func TestGatewayTriesWholeRoutePoolDespiteMaxAttempts(t *testing.T) {
	ResetGatewayCountersForTest()
	firstCalls := 0
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalls++
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer first.Close()
	secondCalls := 0
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCalls++
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer second.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "first", first.URL, "first-key")
	createGatewaySite(t, db, "second", second.URL, "second-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	result, err := ProxyGatewayRequest(req.Context(), db, req, "models", "", "", GatewayPolicy{
		RouteStrategy:    "priority",
		RequestTimeout:   5,
		MaxAttempts:      1,
		FailureThreshold: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("expected whole route pool fallback, status=%d first=%d second=%d", result.StatusCode, firstCalls, secondCalls)
	}
}

func TestDisabledGatewayRouteStaysDisabledAndTransfersTraffic(t *testing.T) {
	ResetGatewayCountersForTest()
	disabledCalls := 0
	disabled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		disabledCalls++
		_, _ = w.Write([]byte(`{"route":"disabled"}`))
	}))
	defer disabled.Close()
	enabledCalls := 0
	enabled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enabledCalls++
		_, _ = w.Write([]byte(`{"route":"enabled"}`))
	}))
	defer enabled.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "disabled-route", disabled.URL, "disabled-key")
	createGatewaySite(t, db, "enabled-route", enabled.URL, "enabled-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var disabledState models.GatewayRouteState
	if err := db.Joins("JOIN sites ON sites.id = gateway_route_states.site_id").
		Where("sites.name = ?", "disabled-route").
		First(&disabledState).Error; err != nil {
		t.Fatal(err)
	}
	disabledState.IsEnabled = false
	if err := db.Save(&disabledState).Error; err != nil {
		t.Fatal(err)
	}
	acquireRoute(disabledState.ID)
	defer releaseRoute(disabledState.ID)

	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var refreshed models.GatewayRouteState
	if err := db.First(&refreshed, disabledState.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshed.IsEnabled {
		t.Fatal("disabled route was re-enabled after sync")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	result, err := ProxyGatewayRequest(req.Context(), db, req, "models", "", "", GatewayPolicy{
		RouteStrategy:    "priority",
		RequestTimeout:   5,
		FailureThreshold: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), `"route":"enabled"`) {
		t.Fatalf("unexpected proxy result: status=%d body=%s", result.StatusCode, result.Body)
	}
	if disabledCalls != 0 || enabledCalls != 1 {
		t.Fatalf("expected traffic to transfer to enabled route, disabled=%d enabled=%d", disabledCalls, enabledCalls)
	}
}

func TestGatewayConcurrencyTransferBalancePrefersLowerActiveRoute(t *testing.T) {
	ResetGatewayCountersForTest()
	busyCalls := 0
	busy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		busyCalls++
		_, _ = w.Write([]byte(`{"route":"busy"}`))
	}))
	defer busy.Close()
	idleCalls := 0
	idle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idleCalls++
		_, _ = w.Write([]byte(`{"route":"idle"}`))
	}))
	defer idle.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "aaa-busy", busy.URL, "busy-key")
	createGatewaySite(t, db, "zzz-idle", idle.URL, "idle-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var busyState models.GatewayRouteState
	if err := db.Joins("JOIN sites ON sites.id = gateway_route_states.site_id").
		Where("sites.name = ?", "aaa-busy").
		First(&busyState).Error; err != nil {
		t.Fatal(err)
	}
	acquireRoute(busyState.ID)
	defer releaseRoute(busyState.ID)

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	result, err := ProxyGatewayRequest(req.Context(), db, req, "models", "", "", GatewayPolicy{
		RouteStrategy:               "priority",
		RequestTimeout:              5,
		RouteConcurrencyLimit:       5,
		ConcurrencyTransferStrategy: "balance",
		ConcurrencyOverflowStrategy: "sequential",
		FailureThreshold:            5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), `"route":"idle"`) {
		t.Fatalf("unexpected proxy result: status=%d body=%s", result.StatusCode, result.Body)
	}
	if busyCalls != 0 || idleCalls != 1 {
		t.Fatalf("expected balanced transfer to idle route, busy=%d idle=%d", busyCalls, idleCalls)
	}
}

func TestGatewayFallsBackAcrossRequestBaseURLsBeforeNextRoute(t *testing.T) {
	ResetGatewayCountersForTest()

	firstCalls := 0
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalls++
		http.Error(w, "first failed", http.StatusBadGateway)
	}))
	defer first.Close()

	secondCalls := 0
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCalls++
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer second.Close()

	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "fallback-upstream",
		BaseURL:   first.URL,
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "route-key",
		},
		PluginConfig: models.JSONMap{
			"api_request_urls": []any{first.URL, second.URL},
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	result, err := ProxyGatewayRequest(req.Context(), db, req, "models", "", "", GatewayPolicy{
		RouteStrategy:    "round_robin",
		RequestTimeout:   5,
		MaxAttempts:      1,
		FailureThreshold: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), `"ok":true`) {
		t.Fatalf("unexpected proxy result: status=%d body=%s", result.StatusCode, result.Body)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("expected same route to try both request bases once, first=%d second=%d", firstCalls, secondCalls)
	}

	var state models.GatewayRouteState
	if err := db.Where("site_id = ?", site.ID).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.LastRequestBaseURL != second.URL {
		t.Fatalf("LastRequestBaseURL = %q", state.LastRequestBaseURL)
	}
}

func TestGatewayRouteTypeUsesPerAPIKeyMetadata(t *testing.T) {
	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "mixed-keys",
		BaseURL:   "https://example.test",
		PluginKey: "sub2api-platform",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{"name": "gpt-plus", "key": "gpt-key", "status": "active", "route_type": "gpt"},
				map[string]any{"name": "claude-plus", "key": "claude-key", "status": "active", "api_format": "anthropic"},
			},
		},
		PluginConfig: models.JSONMap{"api_format": "openai"},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := SyncGatewayRoutes(db); err != nil || count != 2 {
		t.Fatalf("SyncGatewayRoutes count=%d err=%v", count, err)
	}

	var states []models.GatewayRouteState
	if err := db.Where("site_id = ?", site.ID).Order("key_name asc").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 {
		t.Fatalf("state count=%d", len(states))
	}
	byName := map[string]string{}
	for _, state := range states {
		byName[state.KeyName] = state.RouteType
	}
	if byName["gpt-plus"] != "codex" {
		t.Fatalf("gpt-plus route type = %q", byName["gpt-plus"])
	}
	if byName["claude-plus"] != "claude" {
		t.Fatalf("claude-plus route type = %q", byName["claude-plus"])
	}
}

func TestSyncGatewayRoutesDeletesStaleKeys(t *testing.T) {
	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "rotating-keys",
		BaseURL:   "https://example.test",
		PluginKey: "sub2api-platform",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "old-key",
		},
		PluginConfig: models.JSONMap{"api_format": "openai"},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := SyncGatewayRoutes(db); err != nil || count != 1 {
		t.Fatalf("initial SyncGatewayRoutes count=%d err=%v", count, err)
	}

	site.Credentials = models.JSONMap{
		"api_keys": []any{
			map[string]any{"name": "new", "key": "new-key", "status": "active", "route_type": "gpt"},
		},
	}
	if err := db.Save(&site).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := SyncGatewayRoutes(db); err != nil || count != 1 {
		t.Fatalf("second SyncGatewayRoutes count=%d err=%v", count, err)
	}

	var states []models.GatewayRouteState
	if err := db.Where("site_id = ?", site.ID).Order("id asc").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 {
		t.Fatalf("state count=%d, states=%#v", len(states), states)
	}
	if states[0].KeyName != "new" || states[0].KeySource != "site.credentials.api_keys" || !states[0].IsEnabled {
		t.Fatalf("unexpected remaining route: %#v", states[0])
	}
}

func TestReorderGatewayRoutePrioritiesMoveAndPreserveManualPriority(t *testing.T) {
	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "first", "https://first.example", "first-key")
	createGatewaySite(t, db, "second", "https://second.example", "second-key")
	createGatewaySite(t, db, "third", "https://third.example", "third-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var second models.GatewayRouteState
	if err := db.Joins("JOIN sites ON sites.id = gateway_route_states.site_id").
		Where("sites.name = ?", "second").
		First(&second).Error; err != nil {
		t.Fatal(err)
	}
	routes, err := ReorderGatewayRoutePriorities(db, GatewayRoutePriorityReorderOptions{
		RouteID: second.ID,
		Mode:    GatewayRoutePriorityMove,
		Index:   0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := routePriorityNames(routes); strings.Join(got, ",") != "second,first,third" {
		t.Fatalf("route order = %v", got)
	}
	if routes[0].State.RoutePriority != 0 || routes[1].State.RoutePriority != 1 || routes[2].State.RoutePriority != 2 {
		t.Fatalf("priorities = %d,%d,%d", routes[0].State.RoutePriority, routes[1].State.RoutePriority, routes[2].State.RoutePriority)
	}

	var secondSite models.Site
	if err := db.Where("name = ?", "second").First(&secondSite).Error; err != nil {
		t.Fatal(err)
	}
	secondSite.PluginConfig = models.JSONMap{"gateway_priority": 99}
	if err := db.Save(&secondSite).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var stored models.GatewayRouteState
	if err := db.First(&stored, second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.RoutePriorityManual || stored.RoutePriority != 0 {
		t.Fatalf("stored manual=%v priority=%d", stored.RoutePriorityManual, stored.RoutePriority)
	}
}

func TestReorderGatewayRoutePrioritiesPackagePriority(t *testing.T) {
	db := newGatewayTestDB(t)
	createGatewaySiteWithDetails(t, db, "plain", "https://plain.example", "plain-key", "", "")
	createGatewaySiteWithDetails(t, db, "package", "https://package.example", "package-key", "", "Plus")
	createGatewaySiteWithDetails(t, db, "grouped", "https://grouped.example", "grouped-key", "订阅", "")
	createGatewaySiteWithDetails(t, db, "subscribed", "https://subscribed.example", "subscribed-key", "", "订阅套餐")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	routes, err := ReorderGatewayRoutePriorities(db, GatewayRoutePriorityReorderOptions{Mode: GatewayRoutePriorityPackage})
	if err != nil {
		t.Fatal(err)
	}
	if got := routePriorityNames(routes); strings.Join(got, ",") != "grouped,subscribed,package,plain" {
		t.Fatalf("route order = %v", got)
	}
}

func newGatewayTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/gateway.db"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func createGatewaySite(t *testing.T, db *gorm.DB, name, baseURL, apiKey string) {
	t.Helper()
	createGatewaySiteWithDetails(t, db, name, baseURL, apiKey, "", "")
}

func createGatewaySiteWithDetails(t *testing.T, db *gorm.DB, name, baseURL, apiKey, groupName, packageDisplay string) {
	t.Helper()
	pluginConfig := models.JSONMap{}
	if packageDisplay != "" {
		pluginConfig["package_display"] = packageDisplay
	}
	site := models.Site{
		Name:      name,
		BaseURL:   baseURL,
		PluginKey: "http-relay-station",
		GroupName: groupName,
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": apiKey,
		},
		PluginConfig: pluginConfig,
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
}

func routePriorityNames(routes []GatewayRoute) []string {
	out := make([]string, 0, len(routes))
	for _, route := range routes {
		out = append(out, route.Site.Name)
	}
	return out
}

func createTypedGatewaySite(t *testing.T, db *gorm.DB, name, baseURL, apiKey, apiFormat string) {
	t.Helper()
	site := models.Site{
		Name:      name,
		BaseURL:   baseURL,
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": apiKey,
		},
		PluginConfig: models.JSONMap{
			"api_format": apiFormat,
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
}

func TestGatewayStreamingForwarding(t *testing.T) {
	ResetGatewayCountersForTest()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: hello\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: world\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "stream-up", upstream.URL, "k")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader(`{"stream":true,"messages":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	res, err := ProxyGatewayRequestWithOptions(req.Context(), db, req, "chat/completions", ProxyGatewayOptions{ResponseWriter: rec}, GatewayPolicy{
		RouteStrategy:  "round_robin",
		RequestTimeout: 5,
		MaxAttempts:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsStream || !res.Success {
		t.Fatalf("expected success stream, got success=%v stream=%v", res.Success, res.IsStream)
	}
	if got := rec.Body.String(); !strings.Contains(got, "data: hello") || !strings.Contains(got, "data: world") {
		t.Fatalf("unexpected stream payload: %q", got)
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("missing SSE content-type, got %q", rec.Header().Get("Content-Type"))
	}
}

func TestGatewayStopsOn400ByDefault(t *testing.T) {
	ResetGatewayCountersForTest()

	hits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "a", upstream.URL, "k1")
	createGatewaySite(t, db, "b", upstream.URL, "k2")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	rec := httptest.NewRecorder()
	res, err := ProxyGatewayRequestWithOptions(req.Context(), db, req, "models", ProxyGatewayOptions{ResponseWriter: rec}, GatewayPolicy{
		RouteStrategy:    "round_robin",
		RequestTimeout:   5,
		MaxAttempts:      0,
		FailureThreshold: 5,
	})
	var nonRetryable GatewayNonRetryableUpstreamError
	if !errors.As(err, &nonRetryable) {
		t.Fatalf("expected non-retryable upstream error, got %T %v", err, err)
	}
	if nonRetryable.Attempts != 1 || res.StatusCode != http.StatusBadRequest || hits != 1 {
		t.Fatalf("expected first route only with 400, attempts=%d status=%d hits=%d", nonRetryable.Attempts, res.StatusCode, hits)
	}
	if rec.Body.Len() != 0 || rec.Code != http.StatusOK {
		t.Fatalf("upstream error should not be written to client, status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestGatewayRetries400InAllFailureRetryMode(t *testing.T) {
	ResetGatewayCountersForTest()

	hits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "a", upstream.URL, "k1")
	createGatewaySite(t, db, "b", upstream.URL, "k2")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	rec := httptest.NewRecorder()
	res, err := ProxyGatewayRequestWithOptions(req.Context(), db, req, "models", ProxyGatewayOptions{ResponseWriter: rec}, GatewayPolicy{
		RouteStrategy:    "round_robin",
		RequestTimeout:   5,
		MaxAttempts:      0,
		FailureThreshold: 5,
		FailureRetryMode: "all",
	})
	var allFailed GatewayAllRoutesFailedError
	if !errors.As(err, &allFailed) {
		t.Fatalf("expected all routes failed error, got %T %v", err, err)
	}
	if allFailed.Attempts != 2 || res.StatusCode != http.StatusBadRequest || hits != 2 {
		t.Fatalf("expected both routes tried with last 400, attempts=%d status=%d hits=%d", allFailed.Attempts, res.StatusCode, hits)
	}
	if rec.Body.Len() != 0 || rec.Code != http.StatusOK {
		t.Fatalf("upstream error should not be written to client, status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestInferGatewayRouteTypeFromRequestBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "claude", body: `{"model":"claude-3-7-sonnet"}`, want: "claude"},
		{name: "gpt", body: `{"model":"gpt-4o-mini"}`, want: "codex"},
		{name: "gemini", body: `{"model":"gemini-2.5-pro"}`, want: "gemini"},
		{name: "unknown", body: `{"model":"deepseek-chat"}`, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := InferGatewayRouteTypeFromRequestBody([]byte(tc.body)); got != tc.want {
				t.Fatalf("InferGatewayRouteTypeFromRequestBody() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGatewayProxyAutoSelectsRouteTypeFromModel(t *testing.T) {
	ResetGatewayCountersForTest()

	claudeHits := 0
	claude := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claudeHits++
		if got := r.Header.Get("Authorization"); got != "Bearer claude-key" {
			t.Fatalf("claude Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"provider":"claude"}`))
	}))
	defer claude.Close()

	gptHits := 0
	gpt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gptHits++
		if got := r.Header.Get("Authorization"); got != "Bearer gpt-key" {
			t.Fatalf("gpt Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"provider":"gpt"}`))
	}))
	defer gpt.Close()

	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "claude", claude.URL, "claude-key", "anthropic")
	createTypedGatewaySite(t, db, "gpt", gpt.URL, "gpt-key", "openai")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		model        string
		wantProvider string
	}{
		{name: "claude model", model: "claude-3-7-sonnet", wantProvider: "claude"},
		{name: "gpt model", model: "gpt-4o-mini", wantProvider: "gpt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claudeHits = 0
			gptHits = 0
			body := []byte(`{"model":"` + tc.model + `","messages":[{"role":"user","content":"ping"}]}`)
			req := httptest.NewRequest(http.MethodPost, "/api/gateway/chat/completions", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			result, err := ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(result.Body), tc.wantProvider) {
				t.Fatalf("body = %s, want provider %q", result.Body, tc.wantProvider)
			}
			switch tc.wantProvider {
			case "claude":
				if claudeHits != 1 || gptHits != 0 {
					t.Fatalf("claudeHits=%d gptHits=%d", claudeHits, gptHits)
				}
			case "gpt":
				if claudeHits != 0 || gptHits != 1 {
					t.Fatalf("claudeHits=%d gptHits=%d", claudeHits, gptHits)
				}
			}
		})
	}
}

func TestGatewayActiveRequestsSnapshot(t *testing.T) {
	ResetGatewayCountersForTest()

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "active-up", upstream.URL, "active-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
		req.Header.Set("Content-Type", "application/json")
		result, err := ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", GatewayPolicy{
			RouteStrategy:  "round_robin",
			RequestTimeout: 5,
			MaxAttempts:    1,
		})
		if err == nil && !result.Success {
			err = io.ErrUnexpectedEOF
		}
		done <- err
	}()

	select {
	case <-entered:
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("proxy returned before upstream request was observed")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upstream request")
	}
	active := ListGatewayActiveRequests()
	if len(active) != 1 {
		close(release)
		t.Fatalf("active request count = %d", len(active))
	}
	if active[0].RouteLabel != "active-up" || active[0].TargetPath != "chat/completions" || active[0].ActiveConcurrency != 1 {
		close(release)
		t.Fatalf("unexpected active request: %+v", active[0])
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if active := ListGatewayActiveRequests(); len(active) != 0 {
		t.Fatalf("expected active requests cleared, got %d", len(active))
	}
}
