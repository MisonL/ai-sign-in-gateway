package services

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	site := models.Site{
		Name:      name,
		BaseURL:   baseURL,
		PluginKey: "http-relay-station",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": apiKey,
		},
		PluginConfig: models.JSONMap{},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
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

func TestGatewayDoesNotRetry400(t *testing.T) {
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
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest || hits != 1 {
		t.Fatalf("expected single 400 hit, got status=%d hits=%d", res.StatusCode, hits)
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
