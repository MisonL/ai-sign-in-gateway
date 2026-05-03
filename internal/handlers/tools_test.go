package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-sign-in-gateway/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestModelListLoadsModelsFromSiteRequestURL(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer model-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("x-api-key"); got != "model-key" {
			t.Fatalf("x-api-key = %q", got)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "model-key" {
			t.Fatalf("x-goog-api-key = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o-mini"},{"id":"gpt-image-2"}]}`))
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file:tools-model-list?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	site := models.Site{
		Name:      "models",
		BaseURL:   "https://panel.example",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "model-key",
		},
		PluginConfig: models.JSONMap{
			"api_request_urls": []any{upstream.URL},
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"site_id": site.ID})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tools/models", bytes.NewReader(body))
	(&App{DB: db}).ModelList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("ok = %v, body = %s", payload["ok"], rec.Body.String())
	}
	items, _ := payload["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items len = %d, body = %s", len(items), rec.Body.String())
	}
	second, _ := items[1].(map[string]any)
	if second["id"] != "gpt-image-2" || second["mode"] != "image" {
		t.Fatalf("unexpected image model item: %#v", second)
	}
}

func TestModelListExplainsUpstream404(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file:tools-model-list-404?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	site := models.Site{
		Name:      "models-404",
		BaseURL:   upstream.URL,
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "model-key",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"site_id": site.ID})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tools/models", bytes.NewReader(body))
	(&App{DB: db}).ModelList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["ok"] == true {
		t.Fatalf("ok = true, body = %s", rec.Body.String())
	}
	if !strings.Contains(strings.TrimSpace(payload["message"].(string)), "API 请求 URL") {
		t.Fatalf("message = %v", payload["message"])
	}
}

func TestModelListExplainsMissingUpstreamAPIKey(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"API_KEY_REQUIRED","message":"API key is required"}`))
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer model-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("x-api-key"); got != "model-key" {
			t.Fatalf("x-api-key = %q", got)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "model-key" {
			t.Fatalf("x-goog-api-key = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"INVALID_API_KEY","message":"Invalid API key"}`))
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file:tools-model-list-auth-missing-key?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	site := models.Site{
		Name:      "otokapi-like",
		BaseURL:   "https://panel.example",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "model-key",
		},
		PluginConfig: models.JSONMap{
			"api_request_urls": []any{upstream.URL + "/v1"},
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"site_id": site.ID})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tools/models", bytes.NewReader(body))
	(&App{DB: db}).ModelList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["ok"] == true {
		t.Fatalf("ok = true, body = %s", rec.Body.String())
	}
	message := strings.TrimSpace(payload["message"].(string))
	if strings.Contains(message, "API_KEY_REQUIRED") {
		t.Fatalf("request did not send Authorization header: %s", message)
	}
	if !strings.Contains(message, "INVALID_API_KEY") {
		t.Fatalf("message = %v", payload["message"])
	}
}

func TestModelListKeepsAuthFailureBeforeFallbackURLFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"INVALID_API_KEY","message":"Invalid API key"}`))
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file:tools-model-list-auth-fallback?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	site := models.Site{
		Name:      "auth-fallback",
		BaseURL:   "https://panel.invalid",
		PluginKey: "api-supplier",
		IsEnabled: true,
		Credentials: models.JSONMap{
			"api_key": "model-key",
		},
		PluginConfig: models.JSONMap{
			"api_request_urls": upstream.URL + "/v1",
		},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"site_id": site.ID})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tools/models", bytes.NewReader(body))
	(&App{DB: db}).ModelList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	message := strings.TrimSpace(payload["message"].(string))
	if !strings.Contains(message, "INVALID_API_KEY") {
		t.Fatalf("message = %v", payload["message"])
	}
	if strings.Contains(message, "panel.invalid") {
		t.Fatalf("auth failure was overwritten by fallback URL failure: %s", message)
	}
}
