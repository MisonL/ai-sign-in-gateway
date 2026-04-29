package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnalyzeBrowserStorageSub2APIPayload(t *testing.T) {
	raw := `{
		"url": "https://alexai.work/keys",
		"title": "API 密钥 - Alex AI",
		"pluginKey": "sub2api-platform",
		"appConfig": {
			"site_name": "Alex AI",
			"site_logo": "data:image/png;base64,` + string(bytes.Repeat([]byte("A"), 8192)) + `"
		},
		"tokenPayloads": {
			"auth_token": {
				"user_id": 71,
				"email": "user@example.com",
				"role": "user"
			}
		},
		"userAgent": "Mozilla/5.0",
		"localStorage": {
			"auth_token": "header.eyJ1c2VyX2lkIjo3MSwiZW1haWwiOiJ1c2VyQGV4YW1wbGUuY29tIiwicm9sZSI6InVzZXIifQ.signature",
			"refresh_token": "rt_test_refresh",
			"auth_user": "{\"id\":71,\"email\":\"user@example.com\",\"role\":\"user\"}"
		},
		"sessionStorage": {
			"chunk_reload_attempted": "1777333664501"
		}
	}`

	result, err := analyzeBrowserStorage(raw)
	if err != nil {
		t.Fatalf("analyzeBrowserStorage returned error: %v", err)
	}

	credentials := result["suggested_credentials"].(map[string]string)
	if got := result["suggested_plugin_key"]; got != "sub2api-platform" {
		t.Fatalf("suggested_plugin_key = %v", got)
	}
	if got := result["suggested_base_url"]; got != "https://alexai.work" {
		t.Fatalf("suggested_base_url = %v", got)
	}
	if got := credentials["access_token"]; got == "" {
		t.Fatal("access_token was not suggested")
	}
	if got := credentials["refresh_token"]; got != "rt_test_refresh" {
		t.Fatalf("refresh_token = %q", got)
	}
	if got := credentials["email"]; got != "user@example.com" {
		t.Fatalf("email = %q", got)
	}
	if got := credentials["user_id"]; got != "71" {
		t.Fatalf("user_id = %q", got)
	}
}

func TestAnalyzeLocalStorageAcceptsDirectPayload(t *testing.T) {
	body := map[string]any{
		"url":       "https://alexai.work/keys",
		"pluginKey": "sub2api-platform",
		"localStorage": map[string]any{
			"auth_token":    "header.eyJ1c2VyX2lkIjo3MSwiZW1haWwiOiJ1c2VyQGV4YW1wbGUuY29tIn0.signature",
			"refresh_token": "rt_direct",
		},
	}
	data, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/storage/analyze", bytes.NewReader(data))
	rec := httptest.NewRecorder()
	(&App{}).AnalyzeLocalStorage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("response JSON error: %v", err)
	}
	credentials := result["suggested_credentials"].(map[string]any)
	if got := credentials["refresh_token"]; got != "rt_direct" {
		t.Fatalf("refresh_token = %v", got)
	}
}

func TestAnalyzeBrowserStorageAcceptsStorageOnlyPayload(t *testing.T) {
	raw := `{
		"auth_token": "header.eyJ1c2VyX2lkIjo3MSwiZW1haWwiOiJ1c2VyQGV4YW1wbGUuY29tIn0.signature",
		"refresh_token": "rt_storage_only"
	}`
	result, err := analyzeBrowserStorage(raw)
	if err != nil {
		t.Fatalf("analyzeBrowserStorage returned error: %v", err)
	}
	credentials := result["suggested_credentials"].(map[string]string)
	if got := result["suggested_plugin_key"]; got != "sub2api-platform" {
		t.Fatalf("suggested_plugin_key = %v", got)
	}
	if got := credentials["refresh_token"]; got != "rt_storage_only" {
		t.Fatalf("refresh_token = %q", got)
	}
}

func TestAnalyzeBrowserStorageAcceptsQuotedConsoleReturnValue(t *testing.T) {
	captured := `{
		"url": "https://alexai.work/keys",
		"localStorage": {
			"auth_token": "header.eyJ1c2VyX2lkIjo3MSwiZW1haWwiOiJ1c2VyQGV4YW1wbGUuY29tIn0.signature",
			"refresh_token": "rt_quoted"
		}
	}`
	data, _ := json.Marshal(captured)

	result, err := analyzeBrowserStorage(string(data))
	if err != nil {
		t.Fatalf("analyzeBrowserStorage returned error: %v", err)
	}
	credentials := result["suggested_credentials"].(map[string]string)
	if got := credentials["refresh_token"]; got != "rt_quoted" {
		t.Fatalf("refresh_token = %q", got)
	}
}

func TestAnalyzeBrowserStorageAcceptsEscapedJSONStringWithoutOuterQuotes(t *testing.T) {
	raw := `{\"url\":\"https://alexai.work/keys\",\"localStorage\":{\"auth_token\":\"header.eyJ1c2VyX2lkIjo3MSwiZW1haWwiOiJ1c2VyQGV4YW1wbGUuY29tIn0.signature\",\"refresh_token\":\"rt_escaped\"}}`

	result, err := analyzeBrowserStorage(raw)
	if err != nil {
		t.Fatalf("analyzeBrowserStorage returned error: %v", err)
	}
	credentials := result["suggested_credentials"].(map[string]string)
	if got := credentials["refresh_token"]; got != "rt_escaped" {
		t.Fatalf("refresh_token = %q", got)
	}
}

func TestAnalyzeBrowserStorageAcceptsSingleQuotedJSONString(t *testing.T) {
	raw := `'{\"url\":\"https://alexai.work/keys\",\"localStorage\":{\"auth_token\":\"header.eyJ1c2VyX2lkIjo3MSwiZW1haWwiOiJ1c2VyQGV4YW1wbGUuY29tIn0.signature\",\"refresh_token\":\"rt_single_quoted\"}}'`

	result, err := analyzeBrowserStorage(raw)
	if err != nil {
		t.Fatalf("analyzeBrowserStorage returned error: %v", err)
	}
	credentials := result["suggested_credentials"].(map[string]string)
	if got := credentials["refresh_token"]; got != "rt_single_quoted" {
		t.Fatalf("refresh_token = %q", got)
	}
}

func TestAnalyzeLocalStorageAcceptsJSONBodyAsString(t *testing.T) {
	captured := `{"url":"https://alexai.work/keys","localStorage":{"auth_token":"header.eyJ1c2VyX2lkIjo3MSwiZW1haWwiOiJ1c2VyQGV4YW1wbGUuY29tIn0.signature","refresh_token":"rt_body_string"}}`
	data, _ := json.Marshal(captured)

	req := httptest.NewRequest(http.MethodPost, "/storage/analyze", bytes.NewReader(data))
	rec := httptest.NewRecorder()
	(&App{}).AnalyzeLocalStorage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("response JSON error: %v", err)
	}
	credentials := result["suggested_credentials"].(map[string]any)
	if got := credentials["refresh_token"]; got != "rt_body_string" {
		t.Fatalf("refresh_token = %v", got)
	}
}
