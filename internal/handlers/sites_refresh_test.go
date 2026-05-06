package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/plugins"
	"ai-sign-in-gateway/internal/services"
	"github.com/glebarez/sqlite"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func TestRefreshOneSiteUsesBalanceProbeForRelayOnlySites(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/usage" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer probe-key" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"remaining":12.34,"unit":"USD"}`))
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file:sites-refresh-relay?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	site := models.Site{
		Name:      "su8",
		BaseURL:   upstream.URL,
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"account": "公司",
			"api_key": "probe-key",
		},
		PluginConfig: models.JSONMap{
			"endpoint_url": upstream.URL + "/v1",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &App{DB: db, PluginManager: plugins.NewManager()}
	summary := app.refreshOneSite(context.Background(), site, 5)

	connectionStatus, _ := summary["connection_status"].(*string)
	if connectionStatus == nil || *connectionStatus != "success" {
		t.Fatalf("connection_status = %v", summary["connection_status"])
	}
	message, _ := summary["last_message"].(*string)
	if message == nil || !strings.Contains(*message, "模型出口验证成功") {
		t.Fatalf("last_message = %v", summary["last_message"])
	}
	balance, _ := summary["last_balance"].(*float64)
	if balance == nil || *balance != 12.34 {
		t.Fatalf("last_balance = %v", summary["last_balance"])
	}

	var stored models.Site
	if err := db.First(&stored, site.ID).Error; err != nil {
		t.Fatalf("reload site: %v", err)
	}
	if stored.LastStatus == nil || *stored.LastStatus != "success" {
		t.Fatalf("stored.LastStatus = %v", stored.LastStatus)
	}
	if stored.LastBalance == nil || *stored.LastBalance != 12.34 {
		t.Fatalf("stored.LastBalance = %v", stored.LastBalance)
	}
	if got := strings.TrimSpace(jsonMapString(stored.PluginConfig, "balance_unit")); got != "$" {
		t.Fatalf("balance_unit = %q", got)
	}
}

func TestRefreshOneSiteInvitePersistsInviteInfo(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "ok",
				"data": map[string]any{
					"logged_in": true,
				},
			})
		case "/invite":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"invite": map[string]any{
						"code": "BATCH-INVITE",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file:sites-refresh-invite?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	site := models.Site{
		Name:      "invite-site",
		BaseURL:   upstream.URL,
		PluginKey: "http-relay-station",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "token-123",
		},
		PluginConfig: models.JSONMap{
			"auth_mode":            "bearer",
			"status_path":          "/status",
			"status_method":        "GET",
			"status_login_path":    "data.logged_in",
			"invite_path":          "/invite",
			"invite_code_path":     "data.invite.code",
			"invite_link_template": "/register?code={code}",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &App{DB: db, PluginManager: plugins.NewManager()}
	result := app.refreshOneSiteInvite(context.Background(), site, 5)
	if !result.OK {
		t.Fatalf("result.OK = false, message = %q", result.Message)
	}
	if result.InviteCode == nil || *result.InviteCode != "BATCH-INVITE" {
		t.Fatalf("InviteCode = %v", result.InviteCode)
	}
	if result.InviteLink == nil || *result.InviteLink != upstream.URL+"/register?code=BATCH-INVITE" {
		t.Fatalf("InviteLink = %v", result.InviteLink)
	}

	var stored models.Site
	if err := db.First(&stored, site.ID).Error; err != nil {
		t.Fatalf("reload site: %v", err)
	}
	if got := strings.TrimSpace(jsonMapString(stored.PluginConfig, "invite_code")); got != "BATCH-INVITE" {
		t.Fatalf("stored invite_code = %q", got)
	}
	if got := strings.TrimSpace(jsonMapString(stored.PluginConfig, "invite_link")); got != upstream.URL+"/register?code=BATCH-INVITE" {
		t.Fatalf("stored invite_link = %q", got)
	}
}

func TestRefreshOneSiteAPIKeysPersistsUnmaskedYellowPeachKey(t *testing.T) {
	const wantKey = "W4DiyFnN2smON5wIi91Jx9fxBJV3Sw4VHszjdpkC1JoWNHI4"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer panel-token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/token/":
			if got := r.URL.Query().Get("p"); got != "1" {
				t.Fatalf("page query = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"items": []map[string]any{
						{
							"id":      25,
							"name":    "default",
							"key":     "sk-****",
							"enabled": true,
						},
					},
				},
			})
		case "/api/token/batch/keys":
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		case "/api/token/25/key":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"key": wantKey,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file:sites-refresh-api-keys?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	site := models.Site{
		Name:      "欢喜自用API",
		BaseURL:   upstream.URL,
		PluginKey: "yellowpeach-newapi",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"access_token": "panel-token",
		},
		PluginConfig: models.JSONMap{
			"api_keys_url": "/api/token/?p=1&size=10",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &App{DB: db, PluginManager: plugins.NewManager()}
	result := app.refreshOneSiteAPIKeys(context.Background(), site, 5)
	if !result.OK {
		t.Fatalf("result.OK = false, message = %q", result.Message)
	}
	if result.APIKeyCount != 1 {
		t.Fatalf("APIKeyCount = %d", result.APIKeyCount)
	}
	if !result.PrimaryKeyUpdated {
		t.Fatalf("PrimaryKeyUpdated = false")
	}

	var stored models.Site
	if err := db.First(&stored, site.ID).Error; err != nil {
		t.Fatalf("reload site: %v", err)
	}
	if got := strings.TrimSpace(jsonMapString(stored.Credentials, "api_key")); got != wantKey {
		t.Fatalf("stored api_key = %q", got)
	}
	apiKeys, ok := stored.Credentials["api_keys"].([]any)
	if !ok {
		t.Fatalf("stored api_keys type = %T", stored.Credentials["api_keys"])
	}
	if len(apiKeys) != 1 {
		t.Fatalf("stored api_keys len = %d", len(apiKeys))
	}
	item, ok := apiKeys[0].(map[string]any)
	if !ok {
		t.Fatalf("stored api_keys[0] type = %T", apiKeys[0])
	}
	if got := strings.TrimSpace(fmt.Sprint(item["key"])); got != wantKey {
		t.Fatalf("stored api_keys[0].key = %q", got)
	}
}

func TestRefreshOneSiteAPIKeysPreservesDistinctManualKeys(t *testing.T) {
	const syncedKey = "sk-synced"
	const manualKey = "sk-manual"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer panel-token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/token/":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"items": []map[string]any{
						{"id": 25, "name": "synced", "key": syncedKey, "enabled": true},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file:sites-refresh-api-keys-manual?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	site := models.Site{
		Name:      "manual-keys",
		BaseURL:   upstream.URL,
		PluginKey: "yellowpeach-newapi",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"access_token": "panel-token",
			"api_keys": []any{
				map[string]any{"id": "manual-1", "name": "manual", "key": manualKey, "status": "active", "source": "manual", "route_type": "claude"},
			},
		},
		PluginConfig: models.JSONMap{
			"api_keys_url": "/api/token/?p=1&size=10",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &App{DB: db, PluginManager: plugins.NewManager()}
	result := app.refreshOneSiteAPIKeys(context.Background(), site, 5)
	if !result.OK {
		t.Fatalf("result.OK = false, message = %q", result.Message)
	}
	if result.APIKeyCount != 2 {
		t.Fatalf("APIKeyCount = %d", result.APIKeyCount)
	}

	var stored models.Site
	if err := db.First(&stored, site.ID).Error; err != nil {
		t.Fatalf("reload site: %v", err)
	}
	apiKeys, ok := stored.Credentials["api_keys"].([]any)
	if !ok {
		t.Fatalf("stored api_keys type = %T", stored.Credentials["api_keys"])
	}
	if len(apiKeys) != 2 {
		t.Fatalf("stored api_keys len = %d", len(apiKeys))
	}
	first := apiKeys[0].(map[string]any)
	second := apiKeys[1].(map[string]any)
	if got := strings.TrimSpace(fmt.Sprint(first["key"])); got != syncedKey {
		t.Fatalf("first key = %q", got)
	}
	if got := strings.TrimSpace(fmt.Sprint(second["key"])); got != manualKey {
		t.Fatalf("second key = %q", got)
	}
	if got := strings.TrimSpace(fmt.Sprint(second["source"])); got != "manual" {
		t.Fatalf("manual source = %q", got)
	}
}

func TestMergeAPIKeyListsUsesSyncedEntryWhenManualKeyMatches(t *testing.T) {
	merged := mergeAPIKeyLists(
		[]any{
			map[string]any{"id": "manual-1", "name": "manual", "key": "sk-same", "source": "manual", "status": "active"},
			map[string]any{"id": "manual-2", "name": "other", "key": "sk-other", "source": "manual", "status": "active"},
		},
		[]map[string]any{
			{"id": "remote-1", "name": "remote", "key": "sk-same", "status": "active"},
		},
	)
	if len(merged) != 2 {
		t.Fatalf("merged len = %d", len(merged))
	}
	if got := strings.TrimSpace(fmt.Sprint(merged[0]["name"])); got != "remote" {
		t.Fatalf("merged[0].name = %q", got)
	}
	if got := strings.TrimSpace(fmt.Sprint(merged[0]["source"])); got != "" && got != "<nil>" {
		t.Fatalf("merged[0].source = %q", got)
	}
	if got := strings.TrimSpace(fmt.Sprint(merged[1]["key"])); got != "sk-other" {
		t.Fatalf("merged[1].key = %q", got)
	}
}

func TestListSitesRedactsCredentialsAndGetSiteReturnsFullCredentials(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:sites-redaction?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	site := models.Site{
		Name:      "secret-site",
		BaseURL:   "https://example.test",
		PluginKey: "yellowpeach-newapi",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key":      "sk-live-secret",
			"access_token": "access-secret",
			"api_keys": []any{
				map[string]any{"id": "1", "name": "primary", "key": "sk-child-secret", "status": "active"},
			},
		},
		PluginConfig: models.JSONMap{
			"api_keys_url":       "/api/token/?p=1&size=10",
			"client_secret":      "client-secret",
			"supported_models":   []any{"manual-should-not-leak"},
			"gateway_route_type": "codex",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	route := models.GatewayRouteState{
		SiteID:          site.ID,
		KeyFingerprint:  testGatewayRouteFingerprint("sk-live-secret"),
		KeyName:         "primary",
		KeySource:       "test",
		RouteType:       "codex",
		SupportedModels: services.EncodeGatewaySupportedModels([]string{"gpt-5.5"}),
		IsEnabled:       true,
		CircuitState:    "closed",
	}
	if err := db.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}

	app := &App{DB: db, PluginManager: plugins.NewManager()}
	listRec := httptest.NewRecorder()
	app.ListSites(listRec, httptest.NewRequest(http.MethodGet, "/api/sites", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}
	var listPayload []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	credentials := listPayload[0]["credentials"].(map[string]any)
	if got := credentials["api_key"]; got != "********" {
		t.Fatalf("list api_key = %v", got)
	}
	apiKeys := credentials["api_keys"].([]any)
	firstKey := apiKeys[0].(map[string]any)["key"]
	if firstKey != "********" {
		t.Fatalf("list api_keys[0].key = %v", firstKey)
	}
	listConfig := listPayload[0]["plugin_config"].(map[string]any)
	if _, ok := listConfig["supported_models"]; ok {
		t.Fatalf("list plugin_config leaked supported_models: %v", listConfig["supported_models"])
	}
	if got := fmt.Sprint(listPayload[0]["supported_models"]); !strings.Contains(got, "gpt-5.5") || strings.Contains(got, "manual-should-not-leak") {
		t.Fatalf("list supported_models = %v", listPayload[0]["supported_models"])
	}

	detailRec := httptest.NewRecorder()
	app.GetSite(detailRec, siteRequestWithID(site.ID))
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", detailRec.Code, detailRec.Body.String())
	}
	var detailPayload map[string]any
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detailPayload); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	detailCredentials := detailPayload["credentials"].(map[string]any)
	if got := detailCredentials["api_key"]; got != "sk-live-secret" {
		t.Fatalf("detail api_key = %v", got)
	}
	detailConfig := detailPayload["plugin_config"].(map[string]any)
	if _, ok := detailConfig["supported_models"]; ok {
		t.Fatalf("detail plugin_config leaked supported_models: %v", detailConfig["supported_models"])
	}
	if got := fmt.Sprint(detailPayload["supported_models"]); !strings.Contains(got, "gpt-5.5") || strings.Contains(got, "manual-should-not-leak") {
		t.Fatalf("detail supported_models = %v", detailPayload["supported_models"])
	}
}

func TestToggleSiteOffDeletesGatewayRoutes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:sites-toggle-route-cleanup?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	site := models.Site{
		Name:         "route-site",
		BaseURL:      "https://example.test",
		PluginKey:    "http-relay-station",
		IsEnabled:    true,
		Credentials:  models.JSONMap{"api_key": "route-key"},
		PluginConfig: models.JSONMap{},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := db.Create(&models.GatewayRouteState{SiteID: site.ID, KeyFingerprint: testGatewayRouteFingerprint("route-key"), IsEnabled: true, CircuitState: "closed"}).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}

	app := &App{DB: db, PluginManager: plugins.NewManager()}
	rec := httptest.NewRecorder()
	app.ToggleSite(rec, siteRequestWithID(site.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var routeCount int64
	if err := db.Model(&models.GatewayRouteState{}).Where("site_id = ?", site.ID).Count(&routeCount).Error; err != nil {
		t.Fatal(err)
	}
	if routeCount != 0 {
		t.Fatalf("route count after disable = %d", routeCount)
	}
}

func testGatewayRouteFingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func siteRequestWithID(siteID uint) *http.Request {
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/sites/%d", siteID), nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("siteID", fmt.Sprint(siteID))
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
}
