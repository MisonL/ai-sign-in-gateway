package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-sign-in-gateway/internal/models"
)

func TestYellowPeachStatusFallsBackToPasswordWhenAccessTokenUnauthorized(t *testing.T) {
	loginCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			if r.Header.Get("Authorization") == "Bearer stale-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "unauthorized"})
				return
			}
			if got := r.Header.Get("Cookie"); got != "session=valid-session" {
				t.Fatalf("Cookie after fallback = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "ok",
				"data": map[string]any{
					"id":       42,
					"email":    "xem8k5@example.com",
					"balance":  12.5,
					"aff_code": "YELLOW42",
				},
			})
		case "/api/user/login":
			loginCount++
			if r.Method != http.MethodPost {
				t.Fatalf("login method = %s", r.Method)
			}
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "valid-session", Path: "/"})
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id": 42,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewYellowPeach()
	status, err := plugin.FetchAccountStatus(context.Background(), models.Site{
		BaseURL:   server.URL,
		PluginKey: "yellowpeach-newapi",
		Credentials: models.JSONMap{
			"username":     "xem8k5",
			"password":     "password",
			"access_token": "stale-token",
			"user_id":      "42",
		},
	}, 5)
	if err != nil {
		t.Fatalf("FetchAccountStatus returned error: %v", err)
	}
	if loginCount != 1 {
		t.Fatalf("loginCount = %d", loginCount)
	}
	if !status.LoggedIn {
		t.Fatalf("LoggedIn = false: %+v", status)
	}
	if got := status.UpdatedCredentials["cookie"]; got != "session=valid-session" {
		t.Fatalf("updated cookie = %v", got)
	}
	if status.InviteLink == nil || *status.InviteLink != server.URL+"/register?aff=YELLOW42" {
		t.Fatalf("InviteLink = %v", status.InviteLink)
	}
	if status.InviteCode == nil || *status.InviteCode != "YELLOW42" {
		t.Fatalf("InviteCode = %v", status.InviteCode)
	}
}

func TestYellowPeachCheckinFallsBackToPasswordWhenAccessTokenUnauthorized(t *testing.T) {
	loginCount := 0
	checkinCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/checkin":
			checkinCount++
			if r.Header.Get("Authorization") == "Bearer stale-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "unauthorized"})
				return
			}
			if got := r.Header.Get("Cookie"); got != "session=valid-session" {
				t.Fatalf("Cookie after fallback = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "signed"})
		case "/api/user/self":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    map[string]any{"id": 42, "balance": 9.5},
			})
		case "/api/user/login":
			loginCount++
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "valid-session", Path: "/"})
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    map[string]any{"id": 42},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewYellowPeach()
	result, err := plugin.Checkin(context.Background(), models.Site{
		BaseURL: server.URL,
		Credentials: models.JSONMap{
			"username":     "xem8k5",
			"password":     "password",
			"access_token": "stale-token",
			"user_id":      "42",
		},
	}, 5)
	if err != nil {
		t.Fatalf("Checkin returned error: %v", err)
	}
	if loginCount != 1 {
		t.Fatalf("loginCount = %d", loginCount)
	}
	if checkinCount != 2 {
		t.Fatalf("checkinCount = %d", checkinCount)
	}
	if !result.Success {
		t.Fatalf("Success = false: %+v", result)
	}
}

func TestYellowPeachInviteAPIUsesAuthenticatedSession(t *testing.T) {
	loginCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			if r.Header.Get("Authorization") == "Bearer stale-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "unauthorized"})
				return
			}
			if got := r.Header.Get("Cookie"); got != "session=valid-session" {
				t.Fatalf("self Cookie = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "ok",
				"data": map[string]any{
					"id":      42,
					"email":   "xem8k5@example.com",
					"balance": 12.5,
				},
			})
		case "/api/user/aff":
			if got := r.Header.Get("Cookie"); got != "session=valid-session" {
				t.Fatalf("invite Cookie = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"invite": map[string]any{
						"code": "YP-INVITE",
					},
				},
			})
		case "/api/user/login":
			loginCount++
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "valid-session", Path: "/"})
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id": 42,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewYellowPeach()
	status, err := plugin.FetchAccountStatus(context.Background(), models.Site{
		BaseURL:   server.URL,
		PluginKey: "yellowpeach-newapi",
		Credentials: models.JSONMap{
			"username":     "xem8k5",
			"password":     "password",
			"access_token": "stale-token",
			"user_id":      "42",
		},
		PluginConfig: models.JSONMap{
			"invite_path":      "/api/user/aff",
			"invite_code_path": "data.invite.code",
		},
	}, 5)
	if err != nil {
		t.Fatalf("FetchAccountStatus returned error: %v", err)
	}
	if loginCount != 1 {
		t.Fatalf("loginCount = %d", loginCount)
	}
	if status.InviteCode == nil || *status.InviteCode != "YP-INVITE" {
		t.Fatalf("InviteCode = %v", status.InviteCode)
	}
	if status.InviteLink == nil || *status.InviteLink != server.URL+"/register?aff=YP-INVITE" {
		t.Fatalf("InviteLink = %v", status.InviteLink)
	}
}

func TestYellowPeachSyncsAPIKeysFromTokenEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if got := r.Header.Get("Cookie"); got != "session=valid-session" {
			t.Fatalf("Cookie = %q", got)
		}
		switch r.URL.Path {
		case "/api/user/self":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "ok",
				"data": map[string]any{
					"id":      42,
					"email":   "xem8k5@example.com",
					"balance": 12.5,
				},
			})
		case "/api/token/":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"items": []map[string]any{
						{"id": 7, "name": "claude-main", "key": "sk-****", "enabled": true, "api_format": "anthropic"},
						{"id": 9, "name": "gpt-main", "key": "sk-live-gpt", "enabled": true, "api_format": "openai"},
					},
				},
			})
		case "/api/token/batch/keys":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode batch body: %v", err)
			}
			ids, _ := payload["ids"].([]any)
			if len(ids) != 1 || strings.TrimSpace(ids[0].(string)) != "7" {
				t.Fatalf("unexpected ids payload: %#v", payload["ids"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"7": "sk-live-claude",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewYellowPeach()
	status, err := plugin.FetchAccountStatus(context.Background(), models.Site{
		BaseURL:   server.URL,
		PluginKey: "yellowpeach-newapi",
		Credentials: models.JSONMap{
			"cookie":  "session=valid-session",
			"user_id": "42",
		},
		PluginConfig: models.JSONMap{
			"preferred_api_key_name": "claude-main",
		},
	}, 5)
	if err != nil {
		t.Fatalf("FetchAccountStatus returned error: %v", err)
	}
	if got := status.UpdatedCredentials["api_key"]; got != "sk-live-claude" {
		t.Fatalf("updated api_key = %v", got)
	}
	apiKeys, ok := status.UpdatedCredentials["api_keys"].([]map[string]any)
	if !ok {
		raw, ok := status.UpdatedCredentials["api_keys"].([]any)
		if !ok {
			t.Fatalf("api_keys type = %T", status.UpdatedCredentials["api_keys"])
		}
		apiKeys = make([]map[string]any, 0, len(raw))
		for _, item := range raw {
			obj, ok := item.(map[string]any)
			if !ok {
				t.Fatalf("api_keys item type = %T", item)
			}
			apiKeys = append(apiKeys, obj)
		}
	}
	if len(apiKeys) != 2 {
		t.Fatalf("api_keys len = %d", len(apiKeys))
	}
	if got := strings.TrimSpace(status.Message); !strings.Contains(got, "已识别 API Key") {
		t.Fatalf("message = %q", got)
	}
	if apiKeys[0]["route_type"] != "claude" {
		t.Fatalf("first route_type = %v", apiKeys[0]["route_type"])
	}
}

func TestYellowPeachSyncsMaskedAPIKeyFromTokenDetailEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if got := r.Header.Get("Cookie"); got != "session=valid-session" {
			t.Fatalf("Cookie = %q", got)
		}
		switch r.URL.Path {
		case "/api/user/self":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id":      42,
					"email":   "xem8k5@example.com",
					"balance": 12.5,
				},
			})
		case "/api/token/":
			if got := r.URL.Query().Get("p"); got != "1" {
				t.Fatalf("page query = %q", got)
			}
			if got := r.URL.Query().Get("size"); got != "10" {
				t.Fatalf("size query = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"items": []map[string]any{
						{"id": 25, "name": "欢喜自用", "key": "W4DiyFnN2smON5wIi91Jx9fx****", "enabled": true},
					},
				},
			})
		case "/api/token/batch/keys":
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "not found"})
		case "/api/token/25/key":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"key": "W4DiyFnN2smON5wIi91Jx9fxBJV3Sw4VHszjdpkC1JoWNHI4",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewYellowPeach()
	status, err := plugin.FetchAccountStatus(context.Background(), models.Site{
		BaseURL:   server.URL,
		PluginKey: "yellowpeach-newapi",
		Credentials: models.JSONMap{
			"cookie":  "session=valid-session",
			"user_id": "42",
		},
		PluginConfig: models.JSONMap{
			"api_keys_url":            "/api/token/?p=1&size=10",
			"preferred_api_key_id":    "25",
			"preferred_api_key_name":  "欢喜自用",
			"token_keys_url":          "/api/token/batch/keys",
			"status_invite_link_path": "",
			"status_invite_code_path": "",
			"invite_link_template":    "",
		},
	}, 5)
	if err != nil {
		t.Fatalf("FetchAccountStatus returned error: %v", err)
	}
	const wantKey = "W4DiyFnN2smON5wIi91Jx9fxBJV3Sw4VHszjdpkC1JoWNHI4"
	if got := status.UpdatedCredentials["api_key"]; got != wantKey {
		t.Fatalf("updated api_key = %v, want %s", got, wantKey)
	}
	apiKeys, ok := status.UpdatedCredentials["api_keys"].([]map[string]any)
	if !ok {
		t.Fatalf("api_keys type = %T", status.UpdatedCredentials["api_keys"])
	}
	if len(apiKeys) != 1 || apiKeys[0]["id"] != "25" || apiKeys[0]["key"] != wantKey {
		t.Fatalf("api_keys = %#v", apiKeys)
	}
}

func TestYellowPeachStatusReadsSubscriptionSelf(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if got := r.Header.Get("Cookie"); got != "session=valid-session" {
			t.Fatalf("Cookie = %q", got)
		}
		switch r.URL.Path {
		case "/api/user/self":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "ok",
				"data": map[string]any{
					"id":         42,
					"email":      "xem8k5@example.com",
					"quota":      1000000,
					"used_quota": 250000,
					"group":      "vip",
				},
			})
		case "/api/subscription/self":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"subscriptions": []map[string]any{
						{
							"subscription": map[string]any{
								"amount_total": 2500000,
								"amount_used":  1000000,
								"status":       "active",
							},
						},
					},
				},
			})
		case "/api/token/":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": []map[string]any{}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewYellowPeach()
	status, err := plugin.FetchAccountStatus(context.Background(), models.Site{
		BaseURL:   server.URL,
		PluginKey: "yellowpeach-newapi",
		Credentials: models.JSONMap{
			"cookie":  "session=valid-session",
			"user_id": "42",
		},
		PluginConfig: models.JSONMap{
			"quota_per_unit": "500000",
		},
	}, 5)
	if err != nil {
		t.Fatalf("FetchAccountStatus returned error: %v", err)
	}
	if status.PackageRemaining == nil || *status.PackageRemaining != 3 {
		t.Fatalf("PackageRemaining = %v", status.PackageRemaining)
	}
	if status.PackageTotal == nil || *status.PackageTotal != 5 {
		t.Fatalf("PackageTotal = %v", status.PackageTotal)
	}
	if status.PackageUsed == nil || *status.PackageUsed != 2 {
		t.Fatalf("PackageUsed = %v", status.PackageUsed)
	}
	if status.PackageDisplay == nil || !strings.Contains(*status.PackageDisplay, "New API 套餐") {
		t.Fatalf("PackageDisplay = %v", status.PackageDisplay)
	}
}

func TestYellowPeachStatusFallsBackToTokenUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			if got := r.Header.Get("Cookie"); got != "session=valid-session" {
				t.Fatalf("self Cookie = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "ok",
				"data": map[string]any{
					"id":    42,
					"email": "xem8k5@example.com",
				},
			})
		case "/api/subscription/self":
			http.NotFound(w, r)
		case "/api/usage/token/":
			if got := r.Header.Get("Authorization"); got != "Bearer sk-token" {
				t.Fatalf("usage Authorization = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"name":            "default",
					"total_granted":   2000000,
					"total_used":      500000,
					"total_available": 1500000,
				},
			})
		case "/api/token/":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": []map[string]any{}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewYellowPeach()
	status, err := plugin.FetchAccountStatus(context.Background(), models.Site{
		BaseURL:   server.URL,
		PluginKey: "yellowpeach-newapi",
		Credentials: models.JSONMap{
			"cookie":  "session=valid-session",
			"user_id": "42",
			"api_key": "sk-token",
		},
		PluginConfig: models.JSONMap{
			"quota_per_unit": "500000",
		},
	}, 5)
	if err != nil {
		t.Fatalf("FetchAccountStatus returned error: %v", err)
	}
	if status.PackageRemaining == nil || *status.PackageRemaining != 3 {
		t.Fatalf("PackageRemaining = %v", status.PackageRemaining)
	}
	if status.PackageTotal == nil || *status.PackageTotal != 4 {
		t.Fatalf("PackageTotal = %v", status.PackageTotal)
	}
	if status.PackageUsed == nil || *status.PackageUsed != 1 {
		t.Fatalf("PackageUsed = %v", status.PackageUsed)
	}
	if status.PackageDisplay == nil || !strings.Contains(*status.PackageDisplay, "default Token 额度") {
		t.Fatalf("PackageDisplay = %v", status.PackageDisplay)
	}
}
