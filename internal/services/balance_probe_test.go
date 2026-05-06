package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-sign-in-gateway/internal/config"
	"ai-sign-in-gateway/internal/database"
	"ai-sign-in-gateway/internal/migrations"
	"ai-sign-in-gateway/internal/models"
)

func TestUsageRemainingSupportsNewAPIUserQuota(t *testing.T) {
	remaining, ok := usageRemaining(map[string]any{
		"data": map[string]any{
			"quota":      float64(1200),
			"used_quota": float64(200),
		},
	})
	if !ok {
		t.Fatal("usageRemaining returned ok=false")
	}
	if remaining != 1200 {
		t.Fatalf("remaining = %v", remaining)
	}
}

func TestNewAPIUsageRemainingComputesTotalMinusUsedBeforeQuota(t *testing.T) {
	remaining, ok := newAPIUsageRemaining(map[string]any{
		"success": true,
		"data": map[string]any{
			"quota":         float64(-96000000000000),
			"total_granted": float64(2500000),
			"total_used":    float64(1000000),
		},
	})
	if !ok {
		t.Fatal("newAPIUsageRemaining returned ok=false")
	}
	if remaining != 1500000 {
		t.Fatalf("remaining = %v", remaining)
	}
}

func TestNewAPIUsageRemainingRejectsNegativeQuotaOnly(t *testing.T) {
	remaining, ok := newAPIUsageRemaining(map[string]any{
		"success": true,
		"data": map[string]any{
			"quota": float64(-96000000000000),
		},
	})
	if ok {
		t.Fatalf("expected negative quota-only payload to be rejected, remaining=%v", remaining)
	}
}

func TestUsageRemainingSupportsTotalMinusUsed(t *testing.T) {
	remaining, ok := usageRemaining(map[string]any{
		"data": map[string]any{
			"amount_total": float64(5000),
			"amount_used":  float64(1250),
		},
	})
	if !ok {
		t.Fatal("usageRemaining returned ok=false")
	}
	if remaining != 3750 {
		t.Fatalf("remaining = %v", remaining)
	}
}

func TestUsageRemainingSupportsCCSwitchShape(t *testing.T) {
	remaining, ok := usageRemaining(map[string]any{
		"total": float64(100),
		"used":  float64(62.5),
	})
	if !ok {
		t.Fatal("usageRemaining returned ok=false")
	}
	if remaining != 37.5 {
		t.Fatalf("remaining = %v", remaining)
	}
}

func TestUsageRemainingSupportsOpenRouterCredits(t *testing.T) {
	remaining, ok := usageRemaining(map[string]any{
		"data": map[string]any{
			"total_credits": float64(15),
			"total_usage":   float64(4.25),
		},
	})
	if !ok {
		t.Fatal("usageRemaining returned ok=false")
	}
	if remaining != 10.75 {
		t.Fatalf("remaining = %v", remaining)
	}
}

func TestUsageRemainingSupportsDeepSeekBalanceInfos(t *testing.T) {
	remaining, ok := usageRemaining(map[string]any{
		"balance_infos": []any{
			map[string]any{"currency": "CNY", "total_balance": "11.5"},
			map[string]any{"currency": "USD", "total_balance": float64(2)},
		},
	})
	if !ok {
		t.Fatal("usageRemaining returned ok=false")
	}
	if remaining != 13.5 {
		t.Fatalf("remaining = %v", remaining)
	}
}

func TestProbeGatewayRouteBalanceUsesRouteAPIKey(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/usage" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.Header.Get("Authorization") {
		case "Bearer first-key":
			_, _ = w.Write([]byte(`{"remaining":11,"unit":"USD"}`))
		case "Bearer second-key":
			_, _ = w.Write([]byte(`{"remaining":22,"unit":"USD"}`))
		default:
			http.Error(w, "bad key", http.StatusUnauthorized)
		}
	}))
	defer upstream.Close()

	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/route-balance.db"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatal(err)
	}
	site := models.Site{
		Name:      "multi-key",
		BaseURL:   upstream.URL,
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{"name": "first", "key": "first-key", "status": "active"},
				map[string]any{"name": "second", "key": "second-key", "status": "active"},
			},
		},
		PluginConfig: models.JSONMap{},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var second models.GatewayRouteState
	if err := db.Where("key_fingerprint = ?", fingerprint("second-key")).First(&second).Error; err != nil {
		t.Fatal(err)
	}
	result, err := ProbeGatewayRouteBalance(context.Background(), db, second.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Remaining == nil || *result.Remaining != 22 {
		t.Fatalf("unexpected balance result: ok=%v remaining=%v message=%s", result.OK, result.Remaining, result.Message)
	}

	var first models.GatewayRouteState
	if err := db.Where("key_fingerprint = ?", fingerprint("first-key")).First(&first).Error; err != nil {
		t.Fatal(err)
	}
	if first.LastBalance != nil {
		t.Fatalf("first route balance should remain nil, got %v", *first.LastBalance)
	}
	if err := db.First(&second, second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if second.LastBalance == nil || *second.LastBalance != 22 {
		t.Fatalf("second route balance = %v", second.LastBalance)
	}
}

func TestProbeGatewayRouteBalanceFallsBackToSiteConfigAndUpdatesRoute(t *testing.T) {
	routeCalls := 0
	fallbackCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/usage":
			routeCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer route-key" {
				t.Fatalf("route Authorization = %q", got)
			}
			http.Error(w, "route key blocked", http.StatusUnauthorized)
		case "/site/balance":
			fallbackCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer site-key" {
				t.Fatalf("fallback Authorization = %q", got)
			}
			_, _ = w.Write([]byte(`{"remaining":44,"unit":"USD"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/route-balance-fallback.db"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatal(err)
	}
	site := models.Site{
		Name:      "fallback",
		BaseURL:   upstream.URL,
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_keys": []any{
				map[string]any{"name": "site", "key": "site-key", "status": "disabled"},
				map[string]any{"name": "route", "key": "route-key", "status": "active"},
			},
			"api_key": "site-key",
		},
		PluginConfig: models.JSONMap{
			"balance_probe_url": "/site/balance",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var route models.GatewayRouteState
	if err := db.Where("key_fingerprint = ?", fingerprint("route-key")).First(&route).Error; err != nil {
		t.Fatal(err)
	}
	result, err := ProbeGatewayRouteBalance(context.Background(), db, route.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Remaining == nil || *result.Remaining != 44 {
		t.Fatalf("unexpected fallback result: ok=%v remaining=%v message=%s", result.OK, result.Remaining, result.Message)
	}
	if routeCalls != 1 || fallbackCalls != 1 {
		t.Fatalf("calls route=%d fallback=%d", routeCalls, fallbackCalls)
	}
	if err := db.First(&route, route.ID).Error; err != nil {
		t.Fatal(err)
	}
	if route.LastBalance == nil || *route.LastBalance != 44 {
		t.Fatalf("stored route LastBalance = %v", route.LastBalance)
	}
	if route.BalanceUnit != "$" {
		t.Fatalf("stored route BalanceUnit = %q", route.BalanceUnit)
	}
}

func TestProbeGatewayRouteBalanceUsesManualURLAndPersists(t *testing.T) {
	hits := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		if r.URL.Path != "/custom/balance" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer route-key" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"remaining":33,"unit":"USD"}`))
	}))
	defer upstream.Close()

	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/manual-route-balance.db"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatal(err)
	}
	site := models.Site{
		Name:      "manual-balance",
		BaseURL:   upstream.URL,
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "route-key",
		},
		PluginConfig: models.JSONMap{},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var route models.GatewayRouteState
	if err := db.First(&route).Error; err != nil {
		t.Fatal(err)
	}
	result, err := ProbeGatewayRouteBalance(context.Background(), db, route.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("expected automatic probe to fail before manual URL, got remaining=%v", result.Remaining)
	}

	manualURL := upstream.URL + "/custom/balance"
	result, err = ProbeGatewayRouteBalanceWithOptions(context.Background(), db, route.ID, 5, BalanceProbeOptions{BalanceURL: manualURL})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Remaining == nil || *result.Remaining != 33 {
		t.Fatalf("unexpected manual balance result: ok=%v remaining=%v message=%s", result.OK, result.Remaining, result.Message)
	}
	if result.BaseURL != upstream.URL {
		t.Fatalf("BaseURL = %q", result.BaseURL)
	}
	if result.BalanceProbeURL != manualURL {
		t.Fatalf("BalanceProbeURL = %q", result.BalanceProbeURL)
	}
	if err := db.First(&route, route.ID).Error; err != nil {
		t.Fatal(err)
	}
	if route.LastBalance == nil || *route.LastBalance != 33 {
		t.Fatalf("stored LastBalance = %v", route.LastBalance)
	}
	if route.BalanceProbeURL != manualURL {
		t.Fatalf("stored BalanceProbeURL = %q", route.BalanceProbeURL)
	}
	if route.LastRequestBaseURL != upstream.URL {
		t.Fatalf("stored LastRequestBaseURL = %q", route.LastRequestBaseURL)
	}

	result, err = ProbeGatewayRouteBalance(context.Background(), db, route.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Remaining == nil || *result.Remaining != 33 {
		t.Fatalf("stored manual URL result: ok=%v remaining=%v message=%s", result.OK, result.Remaining, result.Message)
	}
	if hits["/custom/balance"] != 2 {
		t.Fatalf("manual balance hits = %d", hits["/custom/balance"])
	}
}

func TestProbeGatewayRouteBalanceConvertsNewAPIQuotaToDollars(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/usage/token/" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer route-key" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"total_available":1500000,"total_used":500000}}`))
	}))
	defer upstream.Close()

	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/newapi-route-balance.db"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatal(err)
	}
	site := models.Site{
		Name:      "xem8k5",
		BaseURL:   upstream.URL,
		PluginKey: "yellowpeach-newapi",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "route-key",
		},
		PluginConfig: models.JSONMap{
			"quota_per_unit": "500000",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var route models.GatewayRouteState
	if err := db.First(&route).Error; err != nil {
		t.Fatal(err)
	}
	result, err := ProbeGatewayRouteBalance(context.Background(), db, route.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Remaining == nil || *result.Remaining != 3 {
		t.Fatalf("unexpected balance result: ok=%v remaining=%v message=%s", result.OK, result.Remaining, result.Message)
	}
	if result.Unit != "$" {
		t.Fatalf("result.Unit = %q", result.Unit)
	}
	if err := db.First(&route, route.ID).Error; err != nil {
		t.Fatal(err)
	}
	if route.LastBalance == nil || *route.LastBalance != 3 {
		t.Fatalf("stored LastBalance = %v", route.LastBalance)
	}
	if route.BalanceUnit != "$" {
		t.Fatalf("stored BalanceUnit = %q", route.BalanceUnit)
	}
}

func TestProbeGatewayRouteBalanceNewAPIRejectsBogusNegativeQuota(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer route-key" {
			t.Fatalf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/api/usage/token/":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota":-96000000000000}}`))
		case "/v1/usage":
			http.NotFound(w, r)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/newapi-negative-route-balance.db"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatal(err)
	}
	site := models.Site{
		Name:      "xem8k5",
		BaseURL:   upstream.URL,
		PluginKey: "yellowpeach-newapi",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "route-key",
		},
		PluginConfig: models.JSONMap{
			"quota_per_unit": "500000",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SyncGatewayRoutes(db); err != nil {
		t.Fatal(err)
	}

	var route models.GatewayRouteState
	if err := db.First(&route).Error; err != nil {
		t.Fatal(err)
	}
	result, err := ProbeGatewayRouteBalance(context.Background(), db, route.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("expected probe to fail instead of storing bogus balance, got remaining=%v", result.Remaining)
	}
	if err := db.First(&route, route.ID).Error; err != nil {
		t.Fatal(err)
	}
	if route.LastBalance != nil {
		t.Fatalf("stored LastBalance should remain nil, got %v", *route.LastBalance)
	}
}

func TestUsageURLCandidatesPreferKnownProvider(t *testing.T) {
	candidates := usageURLCandidates(models.Site{}, "https://openrouter.ai/api/v1")
	if len(candidates) != 1 {
		t.Fatalf("candidate len = %d", len(candidates))
	}
	if candidates[0].URL != "https://openrouter.ai/api/v1/credits" {
		t.Fatalf("candidate URL = %s", candidates[0].URL)
	}
}

func TestUsageURLCandidatesCoverGatewayPluginVariants(t *testing.T) {
	cases := []struct {
		name     string
		site     models.Site
		baseURL  string
		firstURL string
	}{
		{
			name:     "api supplier configured usage URL",
			site:     models.Site{PluginKey: "api-supplier", PluginConfig: models.JSONMap{"usage_balance_url": "/provider/credits"}},
			baseURL:  "https://relay.example/v1",
			firstURL: "https://relay.example/provider/credits",
		},
		{
			name:     "http relay station status path",
			site:     models.Site{PluginKey: "http-relay-station", PluginConfig: models.JSONMap{"status_path": "/api/user/profile"}},
			baseURL:  "https://relay.example/v1",
			firstURL: "https://relay.example/api/user/profile",
		},
		{
			name:     "sub2api configured quota URL",
			site:     models.Site{PluginKey: "sub2api-platform", PluginConfig: models.JSONMap{"quota_url": "/api/v1/keys/usage"}},
			baseURL:  "https://sub2api.example/v1",
			firstURL: "https://sub2api.example/api/v1/keys/usage",
		},
		{
			name:     "yellowpeach newapi token usage",
			site:     models.Site{PluginKey: "yellowpeach-newapi", PluginConfig: models.JSONMap{}},
			baseURL:  "https://newapi.example/v1",
			firstURL: "https://newapi.example/api/usage/token/",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidates := usageURLCandidates(tc.site, tc.baseURL)
			if len(candidates) == 0 {
				t.Fatal("candidate len = 0")
			}
			if candidates[0].URL != tc.firstURL {
				t.Fatalf("first candidate = %s", candidates[0].URL)
			}
			if candidates[0].Manual {
				t.Fatal("configured candidates should not be persisted as manual route URLs")
			}
		})
	}
}

func TestUsageURLCandidatesWithManualAcceptsAbsoluteAndRelative(t *testing.T) {
	candidates := usageURLCandidatesWithManual(models.Site{}, "https://relay.example/v1", "/custom/balance", true)
	if len(candidates) == 0 {
		t.Fatal("candidate len = 0")
	}
	if candidates[0].URL != "https://relay.example/custom/balance" || !candidates[0].Manual {
		t.Fatalf("first candidate = %+v", candidates[0])
	}

	candidates = usageURLCandidatesWithManual(models.Site{}, "https://relay.example/v1", "https://balance.example/usage", true)
	if candidates[0].URL != "https://balance.example/usage" || !candidates[0].Manual {
		t.Fatalf("absolute candidate = %+v", candidates[0])
	}
}

func TestUsageURLCandidatesOnlyProbeNewAPIForKnownNewAPI(t *testing.T) {
	candidates := usageURLCandidates(models.Site{}, "https://relay.example/v1")
	if len(candidates) != 1 {
		t.Fatalf("candidate len = %d", len(candidates))
	}
	if candidates[0].URL != "https://relay.example/v1/usage" {
		t.Fatalf("candidate URL = %s", candidates[0].URL)
	}

	candidates = usageURLCandidates(models.Site{PluginKey: "yellowpeach-newapi"}, "https://relay.example/v1")
	if len(candidates) != 2 {
		t.Fatalf("newapi candidate len = %d", len(candidates))
	}
	if candidates[0].URL != "https://relay.example/api/usage/token/" {
		t.Fatalf("newapi candidate URL = %s", candidates[0].URL)
	}
}

func TestProbeSiteBalanceUsesSub2APIPackageRemaining(t *testing.T) {
	db, err := database.Open(config.Config{DatabaseURL: "sqlite:///" + t.TempDir() + "/balance.db"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err := migrations.Apply(db); err != nil {
		t.Fatal(err)
	}
	site := models.Site{
		Name:      "panglong",
		BaseURL:   "https://panglong.example",
		PluginKey: "sub2api-platform",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "sk-demo",
		},
		PluginConfig: models.JSONMap{
			"package_remaining": 82.82,
			"package_unit":      "USD",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}

	result, err := ProbeSiteBalance(context.Background(), db, site.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Remaining == nil || *result.Remaining != 82.82 {
		t.Fatalf("unexpected balance result: ok=%v remaining=%v", result.OK, result.Remaining)
	}
	if result.Unit != "$" {
		t.Fatalf("result.Unit = %q", result.Unit)
	}

	var stored models.Site
	if err := db.First(&stored, site.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.LastBalance == nil || *stored.LastBalance != 82.82 {
		t.Fatalf("stored.LastBalance = %v", stored.LastBalance)
	}
	if got := stringMapValue(stored.PluginConfig, "balance_unit", ""); got != "$" {
		t.Fatalf("stored balance_unit = %q", got)
	}
}
