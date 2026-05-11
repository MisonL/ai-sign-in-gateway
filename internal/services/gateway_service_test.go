package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"ai-sign-in-gateway/internal/config"
	"ai-sign-in-gateway/internal/database"
	"ai-sign-in-gateway/internal/migrations"
	"ai-sign-in-gateway/internal/models"
	"gorm.io/gorm"
)

func expectedDefaultOpenAISupportedModelsCSV() string {
	return strings.Join([]string{
		"gpt-5.3-codex",
		"gpt-5.3-codex-spark",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.4-nano",
		"gpt-5.4-pro",
		"gpt-5.5",
		"gpt-5.5-pro",
	}, ",")
}

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

func TestSyncGatewayRoutesDeletesDisabledSiteRoutes(t *testing.T) {
	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "enabled", "https://enabled.example", "enabled-key")
	createGatewaySite(t, db, "disabled", "https://disabled.example", "disabled-key")

	if count, err := SyncGatewayRoutes(db); err != nil || count != 2 {
		t.Fatalf("initial SyncGatewayRoutes count=%d err=%v", count, err)
	}

	if err := db.Model(&models.Site{}).Where("name = ?", "disabled").Update("is_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := SyncGatewayRoutes(db); err != nil || count != 1 {
		t.Fatalf("second SyncGatewayRoutes count=%d err=%v", count, err)
	}

	var states []models.GatewayRouteState
	if err := db.Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 {
		t.Fatalf("route states len = %d", len(states))
	}
	var site models.Site
	if err := db.First(&site, states[0].SiteID).Error; err != nil {
		t.Fatal(err)
	}
	if site.Name != "enabled" {
		t.Fatalf("remaining route site = %q", site.Name)
	}
}

func TestSyncGatewayRoutesPreservesManualDisabledRouteAfterSiteReenable(t *testing.T) {
	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "manual-disabled", "https://manual-disabled.example", "manual-key")
	if count, err := SyncGatewayRoutes(db); err != nil || count != 1 {
		t.Fatalf("initial SyncGatewayRoutes count=%d err=%v", count, err)
	}
	var site models.Site
	if err := db.Where("name = ?", "manual-disabled").First(&site).Error; err != nil {
		t.Fatal(err)
	}
	var state models.GatewayRouteState
	if err := db.Where("site_id = ?", site.ID).First(&state).Error; err != nil {
		t.Fatal(err)
	}

	state.IsEnabled = false
	state.IsEnabledManual = true
	if err := db.Save(&state).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.Site{}).Where("id = ?", site.ID).Update("is_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := SyncGatewayRoutes(db); err != nil || count != 0 {
		t.Fatalf("disabled site SyncGatewayRoutes count=%d err=%v", count, err)
	}
	if err := db.Model(&models.Site{}).Where("id = ?", site.ID).Updates(map[string]any{
		"is_enabled": true,
		"name":       "manual-disabled-edited",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := SyncGatewayRoutes(db); err != nil || count != 1 {
		t.Fatalf("reenabled site SyncGatewayRoutes count=%d err=%v", count, err)
	}

	var recreated models.GatewayRouteState
	if err := db.Where("site_id = ?", site.ID).First(&recreated).Error; err != nil {
		t.Fatal(err)
	}
	if recreated.IsEnabled {
		t.Fatalf("manual disabled route was re-enabled after site edit: %+v", recreated)
	}
	if !recreated.IsEnabledManual {
		t.Fatalf("manual disabled marker was not preserved: %+v", recreated)
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

func TestUpdateRouteFailureOpensImmediatelyForUpstream429Quota(t *testing.T) {
	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "limited", "https://limited.example", "limited-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	status := http.StatusTooManyRequests
	before := time.Now().UTC()
	UpdateRouteFailure(db, &state, `{"error":{"code":"DAILY_LIMIT_EXCEEDED","message":"daily limit exceeded"}}`, 42, &status, GatewayPolicy{
		FailureThreshold: 9,
		CooldownSeconds:  10,
	})

	if state.CircuitState != "open" {
		t.Fatalf("circuit state = %q", state.CircuitState)
	}
	if state.ConsecutiveFailures != 1 {
		t.Fatalf("consecutive failures = %d", state.ConsecutiveFailures)
	}
	if state.CircuitOpenUntil == nil || state.CircuitOpenUntil.Sub(before) < 23*time.Hour {
		t.Fatalf("circuit open until = %v", state.CircuitOpenUntil)
	}
}

func TestUpdateRouteFailureOpensImmediatelyForUpstreamConcurrencyLimit(t *testing.T) {
	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "busy", "https://busy.example", "busy-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	status := http.StatusTooManyRequests
	before := time.Now().UTC()
	UpdateRouteFailure(db, &state, `token text request concurrency limit reached (8)`, 42, &status, GatewayPolicy{
		FailureThreshold: 9,
		CooldownSeconds:  10,
	})

	if state.CircuitState != "open" {
		t.Fatalf("circuit state = %q", state.CircuitState)
	}
	if state.CircuitOpenUntil == nil || state.CircuitOpenUntil.Sub(before) < 50*time.Second {
		t.Fatalf("circuit open until = %v", state.CircuitOpenUntil)
	}
}

func TestGatewaySkipsRouteAfterUpstream429Limit(t *testing.T) {
	ResetGatewayCountersForTest()

	limitedCalls := 0
	limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limitedCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"DAILY_LIMIT_EXCEEDED","message":"daily limit exceeded"}}`))
	}))
	defer limited.Close()

	healthyCalls := 0
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		healthyCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"route":"healthy"}`))
	}))
	defer healthy.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "aaa-limited", limited.URL, "limited-key")
	createGatewaySite(t, db, "zzz-healthy", healthy.URL, "healthy-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	policy := GatewayPolicy{
		RouteStrategy:    "priority",
		RequestTimeout:   5,
		FailureThreshold: 9,
		CooldownSeconds:  10,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	result, err := ProxyGatewayRequest(req.Context(), db, req, "models", "", "", policy)
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), `"route":"healthy"`) {
		t.Fatalf("unexpected proxy result: status=%d body=%s", result.StatusCode, result.Body)
	}
	if limitedCalls != 1 || healthyCalls != 1 {
		t.Fatalf("expected one limited hit and one fallback, limited=%d healthy=%d", limitedCalls, healthyCalls)
	}

	var limitedState models.GatewayRouteState
	if err := db.Joins("JOIN sites ON sites.id = gateway_route_states.site_id").
		Where("sites.name = ?", "aaa-limited").
		First(&limitedState).Error; err != nil {
		t.Fatal(err)
	}
	if limitedState.CircuitState != "open" {
		t.Fatalf("limited route circuit = %q", limitedState.CircuitState)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	secondResult, err := ProxyGatewayRequest(secondReq.Context(), db, secondReq, "models", "", "", policy)
	if err != nil {
		t.Fatal(err)
	}
	if secondResult.StatusCode != http.StatusOK || !strings.Contains(string(secondResult.Body), `"route":"healthy"`) {
		t.Fatalf("unexpected second result: status=%d body=%s", secondResult.StatusCode, secondResult.Body)
	}
	if limitedCalls != 1 || healthyCalls != 2 {
		t.Fatalf("limited route should be skipped after circuit opens, limited=%d healthy=%d", limitedCalls, healthyCalls)
	}
}

func TestGatewayFallsBackForQuotaLimitWithNon429Status(t *testing.T) {
	ResetGatewayCountersForTest()

	limitedCalls := 0
	limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limitedCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"insufficient_user_quota","message":"insufficient user quota"}}`))
	}))
	defer limited.Close()

	healthyCalls := 0
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		healthyCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"route":"healthy"}`))
	}))
	defer healthy.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "aaa-limited", limited.URL, "limited-key")
	createGatewaySite(t, db, "zzz-healthy", healthy.URL, "healthy-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	result, err := ProxyGatewayRequest(req.Context(), db, req, "models", "", "", GatewayPolicy{
		RouteStrategy:    "priority",
		RequestTimeout:   5,
		FailureThreshold: 9,
		CooldownSeconds:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), `"route":"healthy"`) {
		t.Fatalf("unexpected proxy result: status=%d body=%s", result.StatusCode, result.Body)
	}
	if limitedCalls != 1 || healthyCalls != 1 {
		t.Fatalf("expected fallback for route-specific quota limit, limited=%d healthy=%d", limitedCalls, healthyCalls)
	}

	var limitedState models.GatewayRouteState
	if err := db.Joins("JOIN sites ON sites.id = gateway_route_states.site_id").
		Where("sites.name = ?", "aaa-limited").
		First(&limitedState).Error; err != nil {
		t.Fatal(err)
	}
	if limitedState.CircuitState != "open" {
		t.Fatalf("limited route circuit = %q", limitedState.CircuitState)
	}
}

func TestGatewayHonorsMaxAttempts(t *testing.T) {
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
	var maxAttempts GatewayMaxAttemptsExceededError
	if !errors.As(err, &maxAttempts) {
		t.Fatalf("expected max attempts error, got %T %v", err, err)
	}
	if maxAttempts.Attempts != 1 || result.StatusCode != http.StatusBadGateway || firstCalls != 1 || secondCalls != 0 {
		t.Fatalf("expected first route only, attempts=%d status=%d first=%d second=%d", maxAttempts.Attempts, result.StatusCode, firstCalls, secondCalls)
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
		RouteStrategy:               "round_robin",
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

func TestGatewayPriorityStrategyIgnoresBalanceTransferBeforeLimit(t *testing.T) {
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
	var idleState models.GatewayRouteState
	if err := db.Joins("JOIN sites ON sites.id = gateway_route_states.site_id").
		Where("sites.name = ?", "zzz-idle").
		First(&idleState).Error; err != nil {
		t.Fatal(err)
	}
	busyState.RoutePriority = 1
	if err := db.Save(&busyState).Error; err != nil {
		t.Fatal(err)
	}
	idleState.RoutePriority = 22
	if err := db.Save(&idleState).Error; err != nil {
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
		ConcurrencyOverflowStrategy: "latency_first",
		FailureThreshold:            5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), `"route":"busy"`) {
		t.Fatalf("unexpected proxy result: status=%d body=%s", result.StatusCode, result.Body)
	}
	if busyCalls != 1 || idleCalls != 0 {
		t.Fatalf("expected priority strategy to keep filling priority route, busy=%d idle=%d", busyCalls, idleCalls)
	}
}

func TestGatewayConcurrencyTransferLimitOnlyFillsActiveRouteUntilLimit(t *testing.T) {
	ResetGatewayCountersForTest()
	firstCalls := 0
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalls++
		_, _ = w.Write([]byte(`{"route":"first"}`))
	}))
	defer first.Close()
	secondCalls := 0
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCalls++
		_, _ = w.Write([]byte(`{"route":"second"}`))
	}))
	defer second.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "aaa-first", first.URL, "first-key")
	createGatewaySite(t, db, "zzz-second", second.URL, "second-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var firstState models.GatewayRouteState
	if err := db.Joins("JOIN sites ON sites.id = gateway_route_states.site_id").
		Where("sites.name = ?", "aaa-first").
		First(&firstState).Error; err != nil {
		t.Fatal(err)
	}
	firstActive := acquireRoute(firstState.ID)
	if firstActive != 1 {
		t.Fatalf("first active = %d", firstActive)
	}
	defer releaseRoute(firstState.ID)

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	result, err := ProxyGatewayRequest(req.Context(), db, req, "models", "", "", GatewayPolicy{
		RouteStrategy:               "round_robin",
		RequestTimeout:              5,
		RouteConcurrencyLimit:       5,
		ConcurrencyTransferStrategy: "limit_only",
		ConcurrencyOverflowStrategy: "sequential",
		FailureThreshold:            5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), `"route":"first"`) {
		t.Fatalf("unexpected proxy result: status=%d body=%s", result.StatusCode, result.Body)
	}
	if firstCalls != 1 || secondCalls != 0 {
		t.Fatalf("expected limit-only to keep filling first route, first=%d second=%d", firstCalls, secondCalls)
	}
}

func TestGatewayConcurrencyTransferLimitOnlyMovesAfterRouteLimit(t *testing.T) {
	ResetGatewayCountersForTest()
	firstCalls := 0
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalls++
		_, _ = w.Write([]byte(`{"route":"first"}`))
	}))
	defer first.Close()
	secondCalls := 0
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCalls++
		_, _ = w.Write([]byte(`{"route":"second"}`))
	}))
	defer second.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "aaa-first", first.URL, "first-key")
	createGatewaySite(t, db, "zzz-second", second.URL, "second-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var firstState models.GatewayRouteState
	if err := db.Joins("JOIN sites ON sites.id = gateway_route_states.site_id").
		Where("sites.name = ?", "aaa-first").
		First(&firstState).Error; err != nil {
		t.Fatal(err)
	}
	for range 5 {
		acquireRoute(firstState.ID)
		defer releaseRoute(firstState.ID)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	result, err := ProxyGatewayRequest(req.Context(), db, req, "models", "", "", GatewayPolicy{
		RouteStrategy:               "round_robin",
		RequestTimeout:              5,
		RouteConcurrencyLimit:       5,
		ConcurrencyTransferStrategy: "limit_only",
		ConcurrencyOverflowStrategy: "sequential",
		FailureThreshold:            5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), `"route":"second"`) {
		t.Fatalf("unexpected proxy result: status=%d body=%s", result.StatusCode, result.Body)
	}
	if firstCalls != 0 || secondCalls != 1 {
		t.Fatalf("expected limit-only to move after first route is full, first=%d second=%d", firstCalls, secondCalls)
	}
}

func TestGatewayConcurrencyOverflowUsesPriorityWhenEveryRouteIsFull(t *testing.T) {
	ResetGatewayCountersForTest()
	firstCalls := 0
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalls++
		_, _ = w.Write([]byte(`{"route":"first"}`))
	}))
	defer first.Close()
	secondCalls := 0
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCalls++
		_, _ = w.Write([]byte(`{"route":"second"}`))
	}))
	defer second.Close()
	thirdCalls := 0
	third := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		thirdCalls++
		_, _ = w.Write([]byte(`{"route":"third"}`))
	}))
	defer third.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "aaa-first", first.URL, "first-key")
	createGatewaySite(t, db, "mmm-second", second.URL, "second-key")
	createGatewaySite(t, db, "zzz-third", third.URL, "third-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var states []models.GatewayRouteState
	if err := db.Order("route_priority asc, id asc").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	if len(states) != 3 {
		t.Fatalf("route count = %d", len(states))
	}
	latencies := []float64{300, 200, 10}
	for idx := range states {
		states[idx].RoutePriority = idx * 100
		states[idx].EWMALatencyMS = &latencies[idx]
		if err := db.Save(&states[idx]).Error; err != nil {
			t.Fatal(err)
		}
		for range 2 {
			acquireRoute(states[idx].ID)
			defer releaseRoute(states[idx].ID)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	result, err := ProxyGatewayRequest(req.Context(), db, req, "models", "", "", GatewayPolicy{
		RouteStrategy:               "round_robin",
		RequestTimeout:              5,
		RouteConcurrencyLimit:       2,
		ConcurrencyTransferStrategy: "limit_only",
		ConcurrencyOverflowStrategy: "latency_first",
		FailureThreshold:            5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), `"route":"first"`) {
		t.Fatalf("unexpected proxy result: status=%d body=%s", result.StatusCode, result.Body)
	}
	if firstCalls != 1 || secondCalls != 0 || thirdCalls != 0 {
		t.Fatalf("expected overflow to continue from priority route when all are full, first=%d second=%d third=%d", firstCalls, secondCalls, thirdCalls)
	}
}

func TestGatewayPriorityStrategyHonorsPriorityBeforeLoad(t *testing.T) {
	ResetGatewayCountersForTest()
	highCalls := 0
	high := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		highCalls++
		_, _ = w.Write([]byte(`{"route":"high"}`))
	}))
	defer high.Close()
	lowCalls := 0
	low := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lowCalls++
		_, _ = w.Write([]byte(`{"route":"low"}`))
	}))
	defer low.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "aaa-high", high.URL, "high-key")
	createGatewaySite(t, db, "zzz-low", low.URL, "low-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var highState models.GatewayRouteState
	if err := db.Joins("JOIN sites ON sites.id = gateway_route_states.site_id").
		Where("sites.name = ?", "aaa-high").
		First(&highState).Error; err != nil {
		t.Fatal(err)
	}
	var lowState models.GatewayRouteState
	if err := db.Joins("JOIN sites ON sites.id = gateway_route_states.site_id").
		Where("sites.name = ?", "zzz-low").
		First(&lowState).Error; err != nil {
		t.Fatal(err)
	}
	highState.RoutePriority = 0
	if err := db.Save(&highState).Error; err != nil {
		t.Fatal(err)
	}
	lowState.RoutePriority = 100
	if err := db.Save(&lowState).Error; err != nil {
		t.Fatal(err)
	}
	acquireRoute(highState.ID)
	defer releaseRoute(highState.ID)

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	result, err := ProxyGatewayRequest(req.Context(), db, req, "models", "", "", GatewayPolicy{
		RouteStrategy:               "priority",
		RequestTimeout:              5,
		RouteConcurrencyLimit:       5,
		ConcurrencyTransferStrategy: "limit_only",
		ConcurrencyOverflowStrategy: "sequential",
		FailureThreshold:            5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), `"route":"high"`) {
		t.Fatalf("unexpected proxy result: status=%d body=%s", result.StatusCode, result.Body)
	}
	if highCalls != 1 || lowCalls != 0 {
		t.Fatalf("expected priority strategy to keep high-priority route, high=%d low=%d", highCalls, lowCalls)
	}
}

func TestGatewayPriorityStrategyKeepsRecentlyFailedRouteBeforeCircuitOpen(t *testing.T) {
	ResetGatewayCountersForTest()
	failedCalls := 0
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failedCalls++
		_, _ = w.Write([]byte(`{"route":"failed"}`))
	}))
	defer failed.Close()
	healthyCalls := 0
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		healthyCalls++
		_, _ = w.Write([]byte(`{"route":"healthy"}`))
	}))
	defer healthy.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "aaa-failed", failed.URL, "failed-key")
	createGatewaySite(t, db, "zzz-healthy", healthy.URL, "healthy-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var failedState models.GatewayRouteState
	if err := db.Joins("JOIN sites ON sites.id = gateway_route_states.site_id").
		Where("sites.name = ?", "aaa-failed").
		First(&failedState).Error; err != nil {
		t.Fatal(err)
	}
	var healthyState models.GatewayRouteState
	if err := db.Joins("JOIN sites ON sites.id = gateway_route_states.site_id").
		Where("sites.name = ?", "zzz-healthy").
		First(&healthyState).Error; err != nil {
		t.Fatal(err)
	}
	failedState.RoutePriority = 0
	UpdateRouteFailure(db, &failedState, "temporary upstream failure", 42, nil, GatewayPolicy{FailureThreshold: 5})
	healthyState.RoutePriority = 100
	if err := db.Save(&healthyState).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/v1/models", nil)
	result, err := ProxyGatewayRequest(req.Context(), db, req, "models", "", "", GatewayPolicy{
		RouteStrategy:               "priority",
		RequestTimeout:              5,
		RouteConcurrencyLimit:       5,
		ConcurrencyTransferStrategy: "limit_only",
		ConcurrencyOverflowStrategy: "sequential",
		FailureThreshold:            5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), `"route":"failed"`) {
		t.Fatalf("unexpected proxy result: status=%d body=%s", result.StatusCode, result.Body)
	}
	if failedCalls != 1 || healthyCalls != 0 {
		t.Fatalf("expected strict priority to keep recently failed route until circuit opens, failed=%d healthy=%d", failedCalls, healthyCalls)
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

func TestGatewayRouteManualRequestBaseOverridesSiteCandidatesAndSurvivesSync(t *testing.T) {
	ResetGatewayCountersForTest()

	siteServerCalls := 0
	siteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siteServerCalls++
		http.Error(w, "site should not be used", http.StatusBadGateway)
	}))
	defer siteServer.Close()

	routeServerCalls := 0
	routeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routeServerCalls++
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true,"source":"route"}`))
	}))
	defer routeServer.Close()

	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "manual-route-url",
		BaseURL:   siteServer.URL,
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "route-key",
		},
		PluginConfig: models.JSONMap{
			"api_request_urls": []any{siteServer.URL},
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var state models.GatewayRouteState
	if err := db.Where("site_id = ?", site.ID).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	state.ManualRequestBaseURLs = marshalStringSlice([]string{routeServer.URL})
	if err := db.Save(&state).Error; err != nil {
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
	if result.StatusCode != http.StatusOK || !strings.Contains(string(result.Body), `"source":"route"`) {
		t.Fatalf("unexpected proxy result: status=%d body=%s", result.StatusCode, result.Body)
	}
	if siteServerCalls != 0 || routeServerCalls != 1 {
		t.Fatalf("expected only manual route URL, site=%d route=%d", siteServerCalls, routeServerCalls)
	}

	var refreshed models.GatewayRouteState
	if err := db.First(&refreshed, state.ID).Error; err != nil {
		t.Fatal(err)
	}
	candidates := GatewayRouteRequestBaseCandidates(refreshed, site)
	if len(candidates) != 1 || candidates[0] != routeServer.URL {
		t.Fatalf("route candidates = %v", candidates)
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
	if byName["gpt-plus"] != "gpt" {
		t.Fatalf("gpt-plus route type = %q", byName["gpt-plus"])
	}
	if byName["claude-plus"] != "claude" {
		t.Fatalf("claude-plus route type = %q", byName["claude-plus"])
	}
}

func TestGatewayGPTChatAndCodexResponsesUseDifferentAPIKeyRoutes(t *testing.T) {
	gptHits := 0
	gpt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected gpt path "+r.URL.Path, http.StatusBadGateway)
			return
		}
		gptHits++
		if got := r.Header.Get("Authorization"); got != "Bearer shared-key" {
			t.Fatalf("gpt Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"provider":"gpt-chat"}`))
	}))
	defer gpt.Close()

	codexHits := 0
	codex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.Error(w, "unexpected codex path "+r.URL.Path, http.StatusBadGateway)
			return
		}
		codexHits++
		if got := r.Header.Get("Authorization"); got != "Bearer shared-key" {
			t.Fatalf("codex Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"provider":"codex-response"}`))
	}))
	defer codex.Close()

	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "gpt-codex-request-modes",
		BaseURL:   "https://panel.example",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{
					"name":              "shared-gpt-chat",
					"key":               "shared-key",
					"status":            "active",
					"route_type":        "gpt",
					"supported_models":  []any{"gpt-5.5"},
					"request_base_urls": []any{gpt.URL},
				},
				map[string]any{
					"name":              "shared-codex-response",
					"key":               "shared-key",
					"status":            "active",
					"route_type":        "codex",
					"supported_models":  []any{"gpt-5.5"},
					"request_base_urls": []any{codex.URL},
				},
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

	chatBody := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"ping"}]}`)
	chatReq := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", bytes.NewReader(chatBody))
	chatReq.Header.Set("Content-Type", "application/json")
	chatResult, err := ProxyGatewayRequest(chatReq.Context(), db, chatReq, "chat/completions", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(chatResult.Body), `"provider":"gpt-chat"`) {
		t.Fatalf("chat body = %s", chatResult.Body)
	}

	responseBody := []byte(`{"model":"gpt-5.5","input":"ping"}`)
	responseReq := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/responses", bytes.NewReader(responseBody))
	responseReq.Header.Set("Content-Type", "application/json")
	responseResult, err := ProxyGatewayRequest(responseReq.Context(), db, responseReq, "responses", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(responseResult.Body), `"provider":"codex-response"`) {
		t.Fatalf("response body = %s", responseResult.Body)
	}
	if gptHits != 1 || codexHits != 1 {
		t.Fatalf("hits gpt=%d codex=%d", gptHits, codexHits)
	}
}

func TestGatewayRequestBaseUsesPerAPIKeyMetadata(t *testing.T) {
	gptHits := 0
	gpt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gptHits++
		if got := r.Header.Get("Authorization"); got != "Bearer gpt-key" {
			t.Fatalf("gpt Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"provider":"gpt"}`))
	}))
	defer gpt.Close()

	claudeHits := 0
	claude := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claudeHits++
		if got := r.Header.Get("Authorization"); got != "Bearer claude-key" {
			t.Fatalf("claude Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"provider":"claude"}`))
	}))
	defer claude.Close()

	defaultHits := 0
	defaultUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultHits++
		http.Error(w, "default upstream should not be used", http.StatusBadGateway)
	}))
	defer defaultUpstream.Close()

	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "mixed-key-urls",
		BaseURL:   defaultUpstream.URL,
		PluginKey: "sub2api-platform",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{
					"name":              "gpt-plus",
					"key":               "gpt-key",
					"status":            "active",
					"route_type":        "gpt",
					"supported_models":  []any{"gpt-5.5"},
					"request_base_urls": []any{gpt.URL},
				},
				map[string]any{
					"name":              "claude-plus",
					"key":               "claude-key",
					"status":            "active",
					"api_format":        "anthropic",
					"supported_models":  []any{"claude-3-7-sonnet"},
					"request_base_urls": []any{claude.URL},
				},
			},
		},
		PluginConfig: models.JSONMap{
			"api_format":       "openai",
			"api_request_urls": []any{defaultUpstream.URL},
		},
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
	candidatesByName := map[string][]string{}
	for _, state := range states {
		candidatesByName[state.KeyName] = GatewayRouteRequestBaseCandidates(state, site)
	}
	if got := strings.Join(candidatesByName["gpt-plus"], ","); got != gpt.URL {
		t.Fatalf("gpt-plus request bases = %q", got)
	}
	if got := strings.Join(candidatesByName["claude-plus"], ","); got != claude.URL {
		t.Fatalf("claude-plus request bases = %q", got)
	}

	gptBody := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"ping"}]}`)
	gptReq := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", bytes.NewReader(gptBody))
	gptReq.Header.Set("Content-Type", "application/json")
	gptResult, err := ProxyGatewayRequest(gptReq.Context(), db, gptReq, "chat/completions", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gptResult.Body), `"provider":"gpt"`) {
		t.Fatalf("gpt body = %s", gptResult.Body)
	}

	claudeBody := []byte(`{"model":"claude-3-7-sonnet","messages":[{"role":"user","content":"ping"}]}`)
	claudeReq := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/messages", bytes.NewReader(claudeBody))
	claudeReq.Header.Set("Content-Type", "application/json")
	claudeResult, err := ProxyGatewayRequest(claudeReq.Context(), db, claudeReq, "messages", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claudeResult.Body), `"provider":"claude"`) {
		t.Fatalf("claude body = %s", claudeResult.Body)
	}
	if gptHits != 1 || claudeHits != 1 || defaultHits != 0 {
		t.Fatalf("hits gpt=%d claude=%d default=%d", gptHits, claudeHits, defaultHits)
	}
}

func TestGatewaySameAPIKeyCanUseDifferentURLsForDifferentModelTypes(t *testing.T) {
	gptHits := 0
	gpt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gptHits++
		if got := r.Header.Get("Authorization"); got != "Bearer shared-key" {
			t.Fatalf("gpt Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"provider":"gpt"}`))
	}))
	defer gpt.Close()

	claudeHits := 0
	claude := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claudeHits++
		if got := r.Header.Get("Authorization"); got != "Bearer shared-key" {
			t.Fatalf("claude Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"provider":"claude"}`))
	}))
	defer claude.Close()

	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "same-key-different-urls",
		BaseURL:   "https://panel.example",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{
					"name":              "shared-gpt",
					"key":               "shared-key",
					"status":            "active",
					"route_type":        "gpt",
					"supported_models":  []any{"gpt-5.5"},
					"request_base_urls": []any{gpt.URL},
				},
				map[string]any{
					"name":              "shared-claude",
					"key":               "shared-key",
					"status":            "active",
					"route_type":        "claude",
					"supported_models":  []any{"claude-3-7-sonnet"},
					"request_base_urls": []any{claude.URL},
				},
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
	seenFingerprints := map[string]bool{}
	for _, state := range states {
		seenFingerprints[state.KeyFingerprint] = true
		if state.KeyFingerprint == fingerprint("shared-key") {
			t.Fatalf("same-key split route kept legacy fingerprint: %+v", state)
		}
	}
	if len(seenFingerprints) != 2 {
		t.Fatalf("fingerprints = %#v", seenFingerprints)
	}

	gptBody := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"ping"}]}`)
	gptReq := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", bytes.NewReader(gptBody))
	gptReq.Header.Set("Content-Type", "application/json")
	gptResult, err := ProxyGatewayRequest(gptReq.Context(), db, gptReq, "chat/completions", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gptResult.Body), `"provider":"gpt"`) {
		t.Fatalf("gpt body = %s", gptResult.Body)
	}

	claudeBody := []byte(`{"model":"claude-3-7-sonnet","messages":[{"role":"user","content":"ping"}]}`)
	claudeReq := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/messages", bytes.NewReader(claudeBody))
	claudeReq.Header.Set("Content-Type", "application/json")
	claudeResult, err := ProxyGatewayRequest(claudeReq.Context(), db, claudeReq, "messages", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claudeResult.Body), `"provider":"claude"`) {
		t.Fatalf("claude body = %s", claudeResult.Body)
	}
	if gptHits != 1 || claudeHits != 1 {
		t.Fatalf("hits gpt=%d claude=%d", gptHits, claudeHits)
	}
}

func TestGatewayDifferentAPIKeysCanShareSameURL(t *testing.T) {
	hitsByKey := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsByKey[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]++
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "different-keys-same-url",
		BaseURL:   "https://panel.example",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{"name": "gpt-a", "key": "gpt-key-a", "status": "active", "route_type": "gpt", "supported_models": []any{"gpt-5.5"}, "request_base_urls": []any{upstream.URL}},
				map[string]any{"name": "gpt-b", "key": "gpt-key-b", "status": "active", "route_type": "gpt", "supported_models": []any{"gpt-5.4"}, "request_base_urls": []any{upstream.URL}},
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
	for _, state := range states {
		if got := strings.Join(GatewayRouteRequestBaseCandidates(state, site), ","); got != upstream.URL {
			t.Fatalf("%s request bases = %q", state.KeyName, got)
		}
	}

	for _, model := range []string{"gpt-5.5", "gpt-5.4"} {
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}]}`, model))
		req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if _, err := ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if hitsByKey["gpt-key-a"] != 1 || hitsByKey["gpt-key-b"] != 1 {
		t.Fatalf("hitsByKey = %#v", hitsByKey)
	}
}

func TestGatewayAPIKeyURLPairsRouteByModel(t *testing.T) {
	type fixture struct {
		name  string
		key   string
		model string
	}
	fixtures := []fixture{
		{name: "gpt-54", key: "gpt-54-key", model: "gpt-5.4"},
		{name: "gpt-55", key: "gpt-55-key", model: "gpt-5.5"},
	}
	hits := map[string]int{}
	servers := map[string]*httptest.Server{}
	for _, item := range fixtures {
		item := item
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits[item.name]++
			if got := r.Header.Get("Authorization"); got != "Bearer "+item.key {
				t.Fatalf("%s Authorization = %q", item.name, got)
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"provider":%q}`, item.name)))
		}))
		defer server.Close()
		servers[item.name] = server
	}

	db := newGatewayTestDB(t)
	apiKeys := []any{}
	for _, item := range fixtures {
		apiKeys = append(apiKeys, map[string]any{
			"name":              item.name,
			"key":               item.key,
			"status":            "active",
			"route_type":        "gpt",
			"supported_models":  []any{item.model},
			"request_base_urls": []any{servers[item.name].URL},
		})
	}
	site := models.Site{
		Name:         "key-url-pairs",
		BaseURL:      "https://panel.example",
		PluginKey:    "api-supplier",
		IsEnabled:    true,
		Credentials:  models.JSONMap{"api_keys": apiKeys},
		PluginConfig: models.JSONMap{"api_format": "openai"},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := SyncGatewayRoutes(db); err != nil || count != 2 {
		t.Fatalf("SyncGatewayRoutes count=%d err=%v", count, err)
	}

	for _, item := range fixtures {
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}]}`, item.model))
		req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		result, err := ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(result.Body), fmt.Sprintf(`"provider":"%s"`, item.name)) {
			t.Fatalf("%s body = %s", item.name, result.Body)
		}
	}
	if hits["gpt-54"] != 1 || hits["gpt-55"] != 1 {
		t.Fatalf("hits = %#v", hits)
	}
}

func TestGatewayImageModelUsesPerAPIKeyURLAndImagePaths(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer image-key" {
			t.Fatalf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/v1/custom/images/create":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode generation body: %v", err)
			}
			if payload["model"] != "gpt-image-2" {
				t.Fatalf("generation body = %#v", payload)
			}
			_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/generated.png"}]}`))
		case "/v1/custom/images/edit":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode edit body: %v", err)
			}
			if payload["model"] != "gpt-image-2" {
				t.Fatalf("edit body = %#v", payload)
			}
			_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/edited.png"}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "image-key-url-paths",
		BaseURL:   "https://panel.example",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{
					"name":                  "image",
					"key":                   "image-key",
					"status":                "active",
					"route_type":            "gpt",
					"supported_models":      []any{"gpt-image-2"},
					"request_base_urls":     []any{upstream.URL},
					"image_generation_path": "/custom/images/create",
					"image_edit_path":       "/custom/images/edit",
				},
			},
		},
		PluginConfig: models.JSONMap{
			"api_format":            "openai",
			"image_generation_path": "/site/images/create",
			"image_edit_path":       "/site/images/edit",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := SyncGatewayRoutes(db); err != nil || count != 1 {
		t.Fatalf("SyncGatewayRoutes count=%d err=%v", count, err)
	}

	generationBody := []byte(`{"model":"gpt-image-2","prompt":"paint"}`)
	generationReq := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/images/generations", bytes.NewReader(generationBody))
	generationReq.Header.Set("Content-Type", "application/json")
	generationResult, err := ProxyGatewayRequest(generationReq.Context(), db, generationReq, "images/generations", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !generationResult.Success || !strings.Contains(string(generationResult.Body), "generated.png") {
		t.Fatalf("generation result success=%v body=%s err=%s", generationResult.Success, generationResult.Body, generationResult.Error)
	}

	editBody := []byte(`{"model":"gpt-image-2","prompt":"edit"}`)
	editReq := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/images/edits", bytes.NewReader(editBody))
	editReq.Header.Set("Content-Type", "application/json")
	editResult, err := ProxyGatewayRequest(editReq.Context(), db, editReq, "images/edits", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !editResult.Success || !strings.Contains(string(editResult.Body), "edited.png") {
		t.Fatalf("edit result success=%v body=%s err=%s", editResult.Success, editResult.Body, editResult.Error)
	}
}

func TestGatewaySupportedModelsUsePerAPIKeyMetadata(t *testing.T) {
	gpt55Hits := 0
	gpt55 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer gpt-55-key" {
			t.Fatalf("Authorization = %q", got)
		}
		gpt55Hits++
		_, _ = w.Write([]byte(`{"provider":"gpt-5.5"}`))
	}))
	defer gpt55.Close()

	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "model-keyed",
		BaseURL:   gpt55.URL,
		PluginKey: "sub2api-platform",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{"name": "gpt-54", "key": "gpt-54-key", "status": "active", "route_type": "gpt", "supported_models": []any{"gpt-5.4"}},
				map[string]any{"name": "gpt-55", "key": "gpt-55-key", "status": "active", "route_type": "gpt", "supported_models": "gpt-5.5"},
			},
		},
		PluginConfig: models.JSONMap{
			"api_format":       "openai",
			"api_request_urls": []any{gpt55.URL},
		},
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
		byName[state.KeyName] = strings.Join(GatewayRouteSupportedModels(state), ",")
	}
	if byName["gpt-54"] != "gpt-5.4" {
		t.Fatalf("gpt-54 supported models = %q", byName["gpt-54"])
	}
	if byName["gpt-55"] != "gpt-5.5" {
		t.Fatalf("gpt-55 supported models = %q", byName["gpt-55"])
	}

	reqBody := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"ping"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	result, err := ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Body), `"provider":"gpt-5.5"`) {
		t.Fatalf("body = %s", result.Body)
	}
	if gpt55Hits != 1 {
		t.Fatalf("gpt55Hits=%d", gpt55Hits)
	}
}

func TestGatewaySupportedModelsIgnoreSiteLevelManualConfig(t *testing.T) {
	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "site-level-models",
		BaseURL:   "https://models.example",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "route-key",
		},
		PluginConfig: models.JSONMap{
			"api_format":       "openai",
			"supported_models": []any{"gpt-4o"},
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := SyncGatewayRoutes(db); err != nil || count != 1 {
		t.Fatalf("SyncGatewayRoutes count=%d err=%v", count, err)
	}

	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(GatewayRouteSupportedModels(state), ","); got != expectedDefaultOpenAISupportedModelsCSV() {
		t.Fatalf("supported models = %q", got)
	}
}

func TestGatewayProxyRewritesImageGenerationPathFromSiteConfig(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/custom/images/create" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer image-key" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/custom.png"}]}`))
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "image-generation-path",
		BaseURL:   upstream.URL,
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "image-key",
		},
		PluginConfig: models.JSONMap{
			"api_format":            "openai",
			"image_generation_path": "/custom/images/create",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-image-2"})
	if err := db.Save(&state).Error; err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"model":"gpt-image-2","prompt":"paint"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	result, err := ProxyGatewayRequest(req.Context(), db, req, "images/generations", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.StatusCode != http.StatusOK {
		t.Fatalf("result success=%v status=%d body=%s err=%s", result.Success, result.StatusCode, result.Body, result.Error)
	}
}

func TestGatewayProxyRewritesImageEditPathFromSiteConfig(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/custom/images/edit" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer image-key" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/custom-edit.png"}]}`))
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "image-edit-path",
		BaseURL:   upstream.URL,
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "image-key",
		},
		PluginConfig: models.JSONMap{
			"api_format":      "openai",
			"image_edit_path": "/custom/images/edit",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-image-2"})
	if err := db.Save(&state).Error; err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"model":"gpt-image-2","prompt":"edit"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/images/edits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	result, err := ProxyGatewayRequest(req.Context(), db, req, "images/edits", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.StatusCode != http.StatusOK {
		t.Fatalf("result success=%v status=%d body=%s err=%s", result.Success, result.StatusCode, result.Body, result.Error)
	}
}

func TestGatewaySupportedModelsDefaultForOpenAIRoutes(t *testing.T) {
	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "gpt-default", "https://gpt.example", "gpt-key", "openai")
	if count, err := SyncGatewayRoutes(db); err != nil || count != 1 {
		t.Fatalf("SyncGatewayRoutes count=%d err=%v", count, err)
	}

	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(GatewayRouteSupportedModels(state), ","); got != expectedDefaultOpenAISupportedModelsCSV() {
		t.Fatalf("default OpenAI supported models = %q", got)
	}
}

func TestGatewayProbeFailureKeepsPreviousSupportedModels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary", http.StatusBadGateway)
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "unstable", upstream.URL, "unstable-key", "openai")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-5.5"})
	state.ModelProbeStatus = "success"
	if err := db.Save(&state).Error; err != nil {
		t.Fatal(err)
	}

	result, err := ProbeGatewayRoute(context.Background(), db, strconv.FormatUint(uint64(state.ID), 10), 5)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("expected failed probe")
	}

	var updated models.GatewayRouteState
	if err := db.First(&updated, state.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(GatewayRouteSupportedModels(updated), ","); got != "gpt-5.5" {
		t.Fatalf("supported models changed after failed probe: %q", got)
	}
	if updated.ModelProbeStatus != "failed" {
		t.Fatalf("model probe status = %q", updated.ModelProbeStatus)
	}
}

func TestGatewayProbeEmptyModelsDefaultsOpenAISupportedModels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "empty-models", upstream.URL, "empty-key", "openai")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	state.SupportedModels = "[]"
	state.ModelProbeStatus = ""
	if err := db.Save(&state).Error; err != nil {
		t.Fatal(err)
	}

	result, err := ProbeGatewayRoute(context.Background(), db, strconv.FormatUint(uint64(state.ID), 10), 5)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("expected empty model probe to use defaults: %s", result.Message)
	}
	if got := strings.Join(result.Models, ","); got != expectedDefaultOpenAISupportedModelsCSV() {
		t.Fatalf("result models = %q", got)
	}

	var updated models.GatewayRouteState
	if err := db.First(&updated, state.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(GatewayRouteSupportedModels(updated), ","); got != expectedDefaultOpenAISupportedModelsCSV() {
		t.Fatalf("stored supported models = %q", got)
	}
	if updated.ModelProbeStatus != "success" {
		t.Fatalf("model probe status = %q", updated.ModelProbeStatus)
	}
}

func TestGatewaySupportedModelsDefaultDoesNotOverwriteExistingState(t *testing.T) {
	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "codex-existing", "https://codex.example", "codex-key", "openai")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-4o"})
	if err := db.Save(&state).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var updated models.GatewayRouteState
	if err := db.First(&updated, state.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(GatewayRouteSupportedModels(updated), ","); got != "gpt-4o" {
		t.Fatalf("existing supported models overwritten: %q", got)
	}
}

func TestGatewaySupportedModelsDefaultDoesNotRefillEditedEmptyState(t *testing.T) {
	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "codex-empty-existing", "https://codex.example", "codex-key", "openai")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	state.SupportedModels = EncodeGatewaySupportedModels(nil)
	state.ModelProbeStatus = ""
	if err := db.Save(&state).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var updated models.GatewayRouteState
	if err := db.First(&updated, state.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got := GatewayRouteSupportedModels(updated); len(got) != 0 {
		t.Fatalf("edited empty supported models were refilled: %#v", got)
	}
}

func TestGatewaySupportedModelsDefaultSkipsNonOpenAIRoutes(t *testing.T) {
	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "claude-default", "https://claude.example", "claude-key", "anthropic")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if got := GatewayRouteSupportedModels(state); len(got) != 0 {
		t.Fatalf("non-codex default supported models = %#v", got)
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

func TestGatewayRejectsOversizedDeclaredRequestBody(t *testing.T) {
	db := newGatewayTestDB(t)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", strings.NewReader(`{}`))
	req.ContentLength = 1 << 62

	_, err := ProxyGatewayRequestWithOptions(req.Context(), db, req, "chat/completions", ProxyGatewayOptions{}, GatewayPolicy{
		RequestTimeout: 5,
		MaxAttempts:    1,
	})
	if err == nil || !strings.Contains(err.Error(), "网关请求体过大") {
		t.Fatalf("expected oversized body error, got %T %v", err, err)
	}
}

func TestGatewayDoesNotTreatTruncatedSuccessBodyAsSuccess(t *testing.T) {
	ResetGatewayCountersForTest()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "64")
		_, _ = w.Write([]byte(`{"partial":true}`))
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "truncated", upstream.URL, "k")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", strings.NewReader(`{"messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	result, err := ProxyGatewayRequestWithOptions(req.Context(), db, req, "chat/completions", ProxyGatewayOptions{}, GatewayPolicy{
		RouteStrategy:  "round_robin",
		RequestTimeout: 5,
		MaxAttempts:    1,
	})
	if err == nil {
		t.Fatalf("expected truncated upstream body error, got success=%v body=%q", result.Success, string(result.Body))
	}
	if result.Success {
		t.Fatalf("truncated upstream body was marked successful: %#v", result)
	}
}

func TestGatewayUsageLoggedFromOpenAIResponse(t *testing.T) {
	ResetGatewayCountersForTest()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-demo","model":"gpt-5.4","usage":{"prompt_tokens":12,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens":8,"total_tokens":20,"total_cost":0.0042}}`))
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	createGatewaySite(t, db, "usage-up", upstream.URL, "k")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-5.5"})
	if err := db.Save(&state).Error; err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader(`{"model":"gpt-5.5","messages":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", body)
	req.Header.Set("Content-Type", "application/json")
	res, err := ProxyGatewayRequestWithOptions(req.Context(), db, req, "chat/completions", ProxyGatewayOptions{}, GatewayPolicy{
		RouteStrategy:  "round_robin",
		RequestTimeout: 5,
		MaxAttempts:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.TotalTokens == nil || *res.TotalTokens != 20 {
		t.Fatalf("expected usage in proxy result, success=%v total=%v", res.Success, res.TotalTokens)
	}
	if res.CachedInputTokens == nil || *res.CachedInputTokens != 4 {
		t.Fatalf("cached input tokens = %v", res.CachedInputTokens)
	}

	var log models.GatewayRequestLog
	if err := db.Order("created_at desc").First(&log).Error; err != nil {
		t.Fatal(err)
	}
	if log.PromptTokens == nil || *log.PromptTokens != 12 {
		t.Fatalf("prompt tokens = %v", log.PromptTokens)
	}
	if log.CachedInputTokens == nil || *log.CachedInputTokens != 4 {
		t.Fatalf("cached input tokens = %v", log.CachedInputTokens)
	}
	if log.CompletionTokens == nil || *log.CompletionTokens != 8 {
		t.Fatalf("completion tokens = %v", log.CompletionTokens)
	}
	if log.TotalTokens == nil || *log.TotalTokens != 20 {
		t.Fatalf("total tokens = %v", log.TotalTokens)
	}
	if log.UsageCost == nil || *log.UsageCost != 0.0042 {
		t.Fatalf("usage cost = %v", log.UsageCost)
	}
	if log.Model != "gpt-5.5" {
		t.Fatalf("model = %q", log.Model)
	}
	if log.RequestedModel != "gpt-5.5" || log.ActualModel != "gpt-5.4" {
		t.Fatalf("requested=%q actual=%q", log.RequestedModel, log.ActualModel)
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
		{name: "gpt", body: `{"model":"gpt-4o-mini"}`, want: "gpt"},
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
	var states []models.GatewayRouteState
	if err := db.Order("id asc").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		switch state.KeyFingerprint {
		case fingerprint("claude-key"):
			state.SupportedModels = EncodeGatewaySupportedModels([]string{"claude-3-7-sonnet"})
		case fingerprint("gpt-key"):
			state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-4o-mini"})
		default:
			t.Fatalf("unexpected key fingerprint: %s", state.KeyFingerprint)
		}
		if err := db.Save(&state).Error; err != nil {
			t.Fatal(err)
		}
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

func TestGatewayModelPreciseMatchDoesNotRouteToWrongModel(t *testing.T) {
	ResetGatewayCountersForTest()

	gpt54Hits := 0
	gpt54 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gpt54Hits++
		_, _ = w.Write([]byte(`{"provider":"gpt-5.4"}`))
	}))
	defer gpt54.Close()

	gpt55Hits := 0
	gpt55 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gpt55Hits++
		_, _ = w.Write([]byte(`{"provider":"gpt-5.5"}`))
	}))
	defer gpt55.Close()

	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "gpt-54", gpt54.URL, "gpt-54-key", "openai")
	createTypedGatewaySite(t, db, "gpt-55", gpt55.URL, "gpt-55-key", "openai")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var states []models.GatewayRouteState
	if err := db.Order("id asc").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 {
		t.Fatalf("route count = %d", len(states))
	}
	for _, state := range states {
		switch state.KeyFingerprint {
		case fingerprint("gpt-54-key"):
			state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-5.4"})
		case fingerprint("gpt-55-key"):
			state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-5.5"})
		default:
			t.Fatalf("unexpected key fingerprint: %s", state.KeyFingerprint)
		}
		if err := db.Save(&state).Error; err != nil {
			t.Fatal(err)
		}
	}

	reqBody := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"ping"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	result, err := ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Body), `"provider":"gpt-5.5"`) {
		t.Fatalf("body = %s", result.Body)
	}
	if gpt55Hits != 1 || gpt54Hits != 0 {
		t.Fatalf("gpt55Hits=%d gpt54Hits=%d", gpt55Hits, gpt54Hits)
	}
}

func TestGatewayModelFilteringMatchesDatedCodexModelVersions(t *testing.T) {
	ResetGatewayCountersForTest()

	type routeFixture struct {
		key      string
		model    string
		server   *httptest.Server
		hits     int
		provider string
	}

	fixtures := []*routeFixture{
		{key: "gpt-54-key", model: "gpt-5.4", provider: "gpt-5.4"},
		{key: "gpt-54-pro-key", model: "gpt-5.4-pro", provider: "gpt-5.4-pro"},
		{key: "gpt-55-key", model: "gpt-5.5", provider: "gpt-5.5"},
		{key: "gpt-55-pro-key", model: "gpt-5.5-pro", provider: "gpt-5.5-pro"},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fixture.hits++
			_, _ = w.Write([]byte(`{"provider":"` + fixture.provider + `"}`))
		}))
		defer fixture.server.Close()
	}

	db := newGatewayTestDB(t)
	for _, fixture := range fixtures {
		createTypedGatewaySite(t, db, fixture.provider, fixture.server.URL, fixture.key, "openai")
	}
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var states []models.GatewayRouteState
	if err := db.Order("id asc").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	byFingerprint := map[string]*routeFixture{}
	for _, fixture := range fixtures {
		byFingerprint[fingerprint(fixture.key)] = fixture
	}
	for _, state := range states {
		fixture := byFingerprint[state.KeyFingerprint]
		if fixture == nil {
			t.Fatalf("unexpected key fingerprint: %s", state.KeyFingerprint)
		}
		state.SupportedModels = EncodeGatewaySupportedModels([]string{fixture.model})
		if err := db.Save(&state).Error; err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		model        string
		wantProvider string
	}{
		{model: "gpt-5.4-2026-05-09", wantProvider: "gpt-5.4"},
		{model: "gpt-5.4-pro-2026-05-09", wantProvider: "gpt-5.4-pro"},
		{model: "gpt-5.5-2026-05-09", wantProvider: "gpt-5.5"},
		{model: "gpt-5.5-pro-2026-05-09", wantProvider: "gpt-5.5-pro"},
	}
	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			for _, fixture := range fixtures {
				fixture.hits = 0
			}
			reqBody := []byte(`{"model":"` + tc.model + `","messages":[{"role":"user","content":"ping"}]}`)
			req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			result, err := ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(result.Body), `"provider":"`+tc.wantProvider+`"`) {
				t.Fatalf("body = %s", result.Body)
			}
			for _, fixture := range fixtures {
				wantHits := 0
				if fixture.provider == tc.wantProvider {
					wantHits = 1
				}
				if fixture.hits != wantHits {
					t.Fatalf("%s hits=%d want %d", fixture.provider, fixture.hits, wantHits)
				}
			}
		})
	}
}

func TestGatewayModelFilteringUsesDefaultOpenAIModelsWhenUnspecified(t *testing.T) {
	ResetGatewayCountersForTest()

	declaredHits := 0
	declared := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		declaredHits++
		_, _ = w.Write([]byte(`{"provider":"declared"}`))
	}))
	defer declared.Close()

	unspecifiedHits := 0
	unspecified := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unspecifiedHits++
		_, _ = w.Write([]byte(`{"provider":"default"}`))
	}))
	defer unspecified.Close()

	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "declared", declared.URL, "declared-key", "openai")
	createTypedGatewaySite(t, db, "unspecified", unspecified.URL, "unspecified-key", "openai")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var states []models.GatewayRouteState
	if err := db.Order("id asc").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.KeyFingerprint == fingerprint("declared-key") {
			state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-5.4"})
			if err := db.Save(&state).Error; err != nil {
				t.Fatal(err)
			}
		}
	}

	reqBody := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"ping"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	result, err := ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Body), `"provider":"default"`) {
		t.Fatalf("body = %s", result.Body)
	}
	if unspecifiedHits != 1 || declaredHits != 0 {
		t.Fatalf("unspecifiedHits=%d declaredHits=%d", unspecifiedHits, declaredHits)
	}
}

func TestGatewayRetryKeepsPreciseModelFilter(t *testing.T) {
	ResetGatewayCountersForTest()

	gpt54Hits := 0
	gpt54 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gpt54Hits++
		_, _ = w.Write([]byte(`{"provider":"gpt-5.4"}`))
	}))
	defer gpt54.Close()

	gpt55Hits := 0
	gpt55 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gpt55Hits++
		http.Error(w, "temporary failure", http.StatusBadGateway)
	}))
	defer gpt55.Close()

	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "gpt-54", gpt54.URL, "gpt-54-key", "openai")
	createTypedGatewaySite(t, db, "gpt-55", gpt55.URL, "gpt-55-key", "openai")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var states []models.GatewayRouteState
	if err := db.Order("id asc").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		switch state.KeyFingerprint {
		case fingerprint("gpt-54-key"):
			state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-5.4"})
		case fingerprint("gpt-55-key"):
			state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-5.5"})
		default:
			t.Fatalf("unexpected key fingerprint: %s", state.KeyFingerprint)
		}
		if err := db.Save(&state).Error; err != nil {
			t.Fatal(err)
		}
	}

	reqBody := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"ping"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	_, err := ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", GatewayPolicy{
		RequestTimeout:   5,
		MaxAttempts:      0,
		FailureThreshold: 5,
	})
	var allFailed GatewayAllRoutesFailedError
	if !errors.As(err, &allFailed) {
		t.Fatalf("expected all routes failed, got %T %v", err, err)
	}
	if allFailed.Attempts != 1 || gpt55Hits != 1 || gpt54Hits != 0 {
		t.Fatalf("attempts=%d gpt55Hits=%d gpt54Hits=%d", allFailed.Attempts, gpt55Hits, gpt54Hits)
	}
}

func TestGatewayModelFilteringKeepsOldBehaviorWhenRequestHasNoModel(t *testing.T) {
	ResetGatewayCountersForTest()

	firstHits := 0
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits++
		_, _ = w.Write([]byte(`{"provider":"first"}`))
	}))
	defer first.Close()

	secondHits := 0
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits++
		_, _ = w.Write([]byte(`{"provider":"second"}`))
	}))
	defer second.Close()

	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "first", first.URL, "first-key", "openai")
	createTypedGatewaySite(t, db, "second", second.URL, "second-key", "openai")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	reqBody := []byte(`{"messages":[{"role":"user","content":"ping"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	result, err := ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", GatewayPolicy{
		RouteStrategy:  "priority",
		RequestTimeout: 5,
		MaxAttempts:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Body), `"provider":"first"`) {
		t.Fatalf("body = %s", result.Body)
	}
	if firstHits != 1 || secondHits != 0 {
		t.Fatalf("firstHits=%d secondHits=%d", firstHits, secondHits)
	}
}

func TestGatewayModelFilteringReturnsClearErrorWhenAllDeclaredButNoMatch(t *testing.T) {
	ResetGatewayCountersForTest()

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unexpected request to first route")
	}))
	defer first.Close()

	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unexpected request to second route")
	}))
	defer second.Close()

	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "first", first.URL, "first-key", "openai")
	createTypedGatewaySite(t, db, "second", second.URL, "second-key", "openai")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var states []models.GatewayRouteState
	if err := db.Order("id asc").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-5.4"})
		if err := db.Save(&state).Error; err != nil {
			t.Fatal(err)
		}
	}

	reqBody := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"ping"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	_, err := ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	var notSupported GatewayModelNotSupportedError
	if !errors.As(err, &notSupported) {
		t.Fatalf("expected GatewayModelNotSupportedError, got %T %v", err, err)
	}
	if !strings.Contains(err.Error(), `gpt-5.5`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGatewaySub2APIModelProbeListsOnlyHealthySupportedModels(t *testing.T) {
	ResetGatewayCountersForTest()

	upstreamHits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
		_, _ = w.Write([]byte(`{"unexpected":true}`))
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "gpt-54", upstream.URL, "gpt-54-key", "openai")
	createTypedGatewaySite(t, db, "gpt-55", upstream.URL, "gpt-55-key", "openai")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var states []models.GatewayRouteState
	if err := db.Order("id asc").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	until := time.Now().UTC().Add(time.Hour)
	for _, state := range states {
		switch state.KeyFingerprint {
		case fingerprint("gpt-54-key"):
			state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-5.4"})
			state.CircuitState = "open"
			state.CircuitOpenUntil = &until
		case fingerprint("gpt-55-key"):
			state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-5.5"})
		default:
			t.Fatalf("unexpected key fingerprint: %s", state.KeyFingerprint)
		}
		if err := db.Save(&state).Error; err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/sub2api/v1/models", nil)
	result, err := ProxyGatewayRequestWithOptions(req.Context(), db, req, "models", ProxyGatewayOptions{ModelProbeStrategy: "sub2api"}, GatewayPolicy{RequestTimeout: 5})
	if err != nil {
		t.Fatal(err)
	}
	body := string(result.Body)
	if result.StatusCode != http.StatusOK || !result.Success {
		t.Fatalf("status=%d success=%v body=%s", result.StatusCode, result.Success, body)
	}
	if !strings.Contains(body, `"id":"gpt-5.5"`) {
		t.Fatalf("body missing healthy model: %s", body)
	}
	if strings.Contains(body, `"id":"gpt-5.4"`) {
		t.Fatalf("body contains open-circuit model: %s", body)
	}
	if upstreamHits != 0 {
		t.Fatalf("sub2api model probe should not call upstream, hits=%d", upstreamHits)
	}
}

func TestGatewaySub2APIModelProbeListsAllHealthySupportedModels(t *testing.T) {
	ResetGatewayCountersForTest()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("sub2api model probe should not call upstream")
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "codex", upstream.URL, "codex-key", "openai")
	createTypedGatewaySite(t, db, "claude", upstream.URL, "claude-key", "anthropic")
	createTypedGatewaySite(t, db, "gemini", upstream.URL, "gemini-key", "google")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var states []models.GatewayRouteState
	if err := db.Order("id asc").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		switch state.KeyFingerprint {
		case fingerprint("codex-key"):
			state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-4o-mini", "gpt-5.5"})
		case fingerprint("claude-key"):
			state.SupportedModels = EncodeGatewaySupportedModels([]string{"claude-3-7-sonnet"})
		case fingerprint("gemini-key"):
			state.SupportedModels = EncodeGatewaySupportedModels([]string{"gemini-2.5-pro"})
		default:
			t.Fatalf("unexpected key fingerprint: %s", state.KeyFingerprint)
		}
		if err := db.Save(&state).Error; err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	result, err := ProxyGatewayRequestWithOptions(req.Context(), db, req, "models", ProxyGatewayOptions{ModelProbeStrategy: "sub2api"}, GatewayPolicy{RequestTimeout: 5})
	if err != nil {
		t.Fatal(err)
	}
	body := string(result.Body)
	for _, model := range []string{"gpt-4o-mini", "gpt-5.5", "claude-3-7-sonnet", "gemini-2.5-pro"} {
		if !strings.Contains(body, fmt.Sprintf(`"id":"%s"`, model)) {
			t.Fatalf("body missing %s: %s", model, body)
		}
	}
}

func TestGatewaySub2APIModelProbeExcludesRoutesWithoutUsableKey(t *testing.T) {
	ResetGatewayCountersForTest()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("sub2api model probe should not call upstream")
	}))
	defer upstream.Close()

	db := newGatewayTestDB(t)
	site := models.Site{
		Name:      "disabled-key-site",
		BaseURL:   upstream.URL,
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{"name": "disabled", "key": "disabled-key", "status": "disabled", "route_type": "gpt", "supported_models": []any{"gpt-5.5"}},
			},
		},
		PluginConfig: models.JSONMap{"api_format": "openai"},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	state := models.GatewayRouteState{
		SiteID:              site.ID,
		KeyFingerprint:      fingerprint("disabled-key"),
		KeyName:             "disabled",
		SiteNameSnapshot:    site.Name,
		SiteBaseURLSnapshot: site.BaseURL,
		SiteAPIURLSnapshot:  marshalStringSlice(GatewayRequestBaseCandidates(site)),
		RouteType:           "codex",
		SupportedModels:     EncodeGatewaySupportedModels([]string{"gpt-5.5"}),
		IsEnabled:           true,
		CircuitState:        "closed",
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/sub2api/v1/models", nil)
	result, err := ProxyGatewayRequestWithOptions(req.Context(), db, req, "models", ProxyGatewayOptions{ModelProbeStrategy: "sub2api"}, GatewayPolicy{RequestTimeout: 5})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusServiceUnavailable || result.Success {
		t.Fatalf("status=%d success=%v body=%s", result.StatusCode, result.Success, result.Body)
	}
	if strings.Contains(string(result.Body), `"id":"gpt-5.5"`) {
		t.Fatalf("body contains disabled-key model: %s", result.Body)
	}
}

func TestGatewaySub2APIModelProbeReturnsUnavailableWithoutHealthyModels(t *testing.T) {
	ResetGatewayCountersForTest()

	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "gpt-54", "https://upstream.example", "gpt-54-key", "openai")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-5.4"})
	state.CircuitState = "open"
	until := time.Now().UTC().Add(time.Hour)
	state.CircuitOpenUntil = &until
	if err := db.Save(&state).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gateway/sub2api/v1/models", nil)
	result, err := ProxyGatewayRequestWithOptions(req.Context(), db, req, "models", ProxyGatewayOptions{ModelProbeStrategy: "sub2api"}, GatewayPolicy{RequestTimeout: 5})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusServiceUnavailable || result.Success {
		t.Fatalf("status=%d success=%v body=%s", result.StatusCode, result.Success, result.Body)
	}
	if !strings.Contains(string(result.Body), "gateway_model_health_unavailable") {
		t.Fatalf("body = %s", result.Body)
	}
}

func TestGatewayProbePersistsSupportedModelsForPreciseRouting(t *testing.T) {
	ResetGatewayCountersForTest()

	gpt54Hits := 0
	gpt54 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.4"}]}`))
		case "/v1/chat/completions":
			gpt54Hits++
			t.Fatal("unexpected request to gpt-5.4 route")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer gpt54.Close()

	gpt55Hits := 0
	gpt55 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.5"}]}`))
		case "/v1/chat/completions":
			gpt55Hits++
			_, _ = w.Write([]byte(`{"provider":"gpt-5.5"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer gpt55.Close()

	db := newGatewayTestDB(t)
	createTypedGatewaySite(t, db, "gpt-54", gpt54.URL, "gpt-54-key", "openai")
	createTypedGatewaySite(t, db, "gpt-55", gpt55.URL, "gpt-55-key", "openai")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var states []models.GatewayRouteState
	if err := db.Order("id asc").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		result, err := ProbeGatewayRoute(context.Background(), db, strconv.FormatUint(uint64(state.ID), 10), 5)
		if err != nil {
			t.Fatal(err)
		}
		if !result.OK {
			t.Fatalf("probe failed for route %d: %s", state.ID, result.Message)
		}
	}

	var gpt54State models.GatewayRouteState
	if err := db.Where("key_fingerprint = ?", fingerprint("gpt-54-key")).First(&gpt54State).Error; err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(GatewayRouteSupportedModels(gpt54State), ","); got != "gpt-5.4" {
		t.Fatalf("gpt-5.4 route supported models = %q", got)
	}

	var gpt55State models.GatewayRouteState
	if err := db.Where("key_fingerprint = ?", fingerprint("gpt-55-key")).First(&gpt55State).Error; err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(GatewayRouteSupportedModels(gpt55State), ","); got != "gpt-5.5" {
		t.Fatalf("gpt-5.5 route supported models = %q", got)
	}

	reqBody := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"ping"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	result, err := ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", GatewayPolicy{RequestTimeout: 5, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Body), `"provider":"gpt-5.5"`) {
		t.Fatalf("body = %s", result.Body)
	}
	if gpt55Hits != 1 || gpt54Hits != 0 {
		t.Fatalf("gpt55Hits=%d gpt54Hits=%d", gpt55Hits, gpt54Hits)
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
	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-4o"})
	if err := db.Save(&state).Error; err != nil {
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

func TestGatewayConcurrencyLimitIsTransferThresholdNotHardLimit(t *testing.T) {
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
	createGatewaySite(t, db, "threshold-up", upstream.URL, "threshold-key")
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}
	var state models.GatewayRouteState
	if err := db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	state.SupportedModels = EncodeGatewaySupportedModels([]string{"gpt-4o"})
	if err := db.Save(&state).Error; err != nil {
		t.Fatal(err)
	}
	acquireRoute(state.ID)
	defer releaseRoute(state.ID)

	done := make(chan error, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/api/gateway/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
		req.Header.Set("Content-Type", "application/json")
		result, err := ProxyGatewayRequest(req.Context(), db, req, "chat/completions", "", "", GatewayPolicy{
			RouteStrategy:               "round_robin",
			RequestTimeout:              5,
			MaxAttempts:                 1,
			RouteConcurrencyLimit:       1,
			ConcurrencyTransferStrategy: "limit_only",
			ConcurrencyOverflowStrategy: "sequential",
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
	if active := RouteActiveCount(state.ID); active != 2 {
		close(release)
		t.Fatalf("active count = %d, want 2 when threshold is 1", active)
	}
	activeRequests := ListGatewayActiveRequests()
	if len(activeRequests) != 1 {
		close(release)
		t.Fatalf("active request count = %d", len(activeRequests))
	}
	if activeRequests[0].ActiveConcurrency != 2 {
		close(release)
		t.Fatalf("active request concurrency = %d, want 2", activeRequests[0].ActiveConcurrency)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
