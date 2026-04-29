package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-sign-in-gateway/internal/config"
	"ai-sign-in-gateway/internal/database"
	"ai-sign-in-gateway/internal/migrations"
	"ai-sign-in-gateway/internal/models"
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

	router := NewRouter(db, config.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/gateway/models", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body == "" || body == "{}" {
		t.Fatalf("unexpected body: %q", body)
	}
}
