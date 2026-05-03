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

func TestSub2APIStatusPrefersAccessTokenBeforeRefreshOrPassword(t *testing.T) {
	refreshCalls := 0
	loginCalls := 0
	meCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/refresh":
			refreshCalls++
			http.Error(w, `{"code":401,"message":"refresh expired"}`, http.StatusUnauthorized)
		case "/api/v1/auth/login":
			loginCalls++
			http.Error(w, `{"code":401,"message":"invalid email or password"}`, http.StatusUnauthorized)
		case "/api/v1/auth/me":
			meCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer valid-access-token" {
				t.Fatalf("Authorization = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"email":       "user@example.com",
					"balance":     8.5,
					"currency":    "USD",
					"invite_link": "/register?aff=SUB2API88",
					"aff_code":    "SUB2API88",
				},
			})
		case "/api/v1/keys":
			if got := r.Header.Get("Authorization"); got != "Bearer valid-access-token" {
				t.Fatalf("keys Authorization = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"id": "1", "name": "gpt", "key": "sk-demo", "status": "active"},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewSub2API()
	status, err := plugin.FetchAccountStatus(context.Background(), models.Site{
		BaseURL:   server.URL,
		PluginKey: "sub2api-platform",
		Credentials: models.JSONMap{
			"access_token":  "valid-access-token",
			"refresh_token": "stale-refresh-token",
			"email":         "user@example.com",
			"password":      "wrong-password",
		},
		PluginConfig: models.JSONMap{
			"api_keys_url": "/api/v1/keys",
		},
	}, 5)
	if err != nil {
		t.Fatalf("FetchAccountStatus returned error: %v", err)
	}
	if !status.LoggedIn {
		t.Fatalf("LoggedIn = false: %+v", status)
	}
	if meCalls != 1 {
		t.Fatalf("meCalls = %d", meCalls)
	}
	if refreshCalls != 0 {
		t.Fatalf("refreshCalls = %d", refreshCalls)
	}
	if loginCalls != 0 {
		t.Fatalf("loginCalls = %d", loginCalls)
	}
	if got := status.UpdatedCredentials["access_token"]; got != "valid-access-token" {
		t.Fatalf("updated access_token = %v", got)
	}
	if status.InviteLink == nil || *status.InviteLink != server.URL+"/register?aff=SUB2API88" {
		t.Fatalf("InviteLink = %v", status.InviteLink)
	}
	if status.InviteCode == nil || *status.InviteCode != "SUB2API88" {
		t.Fatalf("InviteCode = %v", status.InviteCode)
	}
}

func TestSub2APIStatusPersistsRefreshedTokens(t *testing.T) {
	refreshCalls := 0
	loginCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/refresh":
			refreshCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"access_token":  "new-access-token",
					"refresh_token": "new-refresh-token",
				},
			})
		case "/api/v1/auth/login":
			loginCalls++
			http.Error(w, `{"code":401,"message":"invalid email or password"}`, http.StatusUnauthorized)
		case "/api/v1/auth/me":
			switch r.Header.Get("Authorization") {
			case "Bearer stale-access-token":
				http.Error(w, `{"code":401,"message":"token expired"}`, http.StatusUnauthorized)
			case "Bearer new-access-token":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{
						"email":    "user@example.com",
						"balance":  8.5,
						"currency": "USD",
					},
				})
			default:
				t.Fatalf("unexpected me Authorization = %q", r.Header.Get("Authorization"))
			}
		case "/api/v1/keys":
			if got := r.Header.Get("Authorization"); got != "Bearer new-access-token" {
				t.Fatalf("keys Authorization = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"id": "1", "name": "gpt", "key": "sk-demo", "status": "active"},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewSub2API()
	status, err := plugin.FetchAccountStatus(context.Background(), models.Site{
		BaseURL:   server.URL,
		PluginKey: "sub2api-platform",
		Credentials: models.JSONMap{
			"access_token":  "stale-access-token",
			"refresh_token": "old-refresh-token",
		},
		PluginConfig: models.JSONMap{
			"api_keys_url": "/api/v1/keys",
		},
	}, 5)
	if err != nil {
		t.Fatalf("FetchAccountStatus returned error: %v", err)
	}
	if !status.LoggedIn {
		t.Fatalf("LoggedIn = false: %+v", status)
	}
	if refreshCalls != 1 {
		t.Fatalf("refreshCalls = %d", refreshCalls)
	}
	if loginCalls != 0 {
		t.Fatalf("loginCalls = %d", loginCalls)
	}
	if got := status.UpdatedCredentials["access_token"]; got != "new-access-token" {
		t.Fatalf("updated access_token = %v", got)
	}
	if got := status.UpdatedCredentials["refresh_token"]; got != "new-refresh-token" {
		t.Fatalf("updated refresh_token = %v", got)
	}
}

func TestSub2APIInviteAPIOverridesStatusInvitePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/me":
			if got := r.Header.Get("Authorization"); got != "Bearer valid-access-token" {
				t.Fatalf("me Authorization = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"email":   "user@example.com",
					"balance": 8.5,
				},
			})
		case "/api/v1/user/aff":
			if got := r.Header.Get("Authorization"); got != "Bearer valid-access-token" {
				t.Fatalf("invite Authorization = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"invite": map[string]any{
						"code": "SUB2-INVITE",
					},
				},
			})
		case "/api/v1/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"id": "1", "name": "gpt", "key": "sk-demo", "status": "active"},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewSub2API()
	status, err := plugin.FetchAccountStatus(context.Background(), models.Site{
		BaseURL:   server.URL,
		PluginKey: "sub2api-platform",
		Credentials: models.JSONMap{
			"access_token": "valid-access-token",
		},
		PluginConfig: models.JSONMap{
			"api_keys_url":         "/api/v1/keys",
			"invite_path":          "/api/v1/user/aff",
			"invite_code_path":     "data.invite.code",
			"invite_link_path":     "data.invite.link",
			"invite_link_template": "/register?aff={code}",
		},
	}, 5)
	if err != nil {
		t.Fatalf("FetchAccountStatus returned error: %v", err)
	}
	if status.InviteCode == nil || *status.InviteCode != "SUB2-INVITE" {
		t.Fatalf("InviteCode = %v", status.InviteCode)
	}
	if status.InviteLink == nil || *status.InviteLink != server.URL+"/register?aff=SUB2-INVITE" {
		t.Fatalf("InviteLink = %v", status.InviteLink)
	}
}

func TestSub2APIStatusReadsSubscriptionProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"email":    "user@example.com",
					"balance":  8.5,
					"currency": "USD",
				},
			})
		case "/api/v1/subscriptions/progress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"subscription": map[string]any{
							"group": map[string]any{"name": "Pro"},
						},
						"progress": map[string]any{
							"monthly": map[string]any{
								"limit_usd":     100,
								"used_usd":      25,
								"remaining_usd": 75,
							},
							"weekly": map[string]any{
								"limit_usd":     20,
								"used_usd":      3,
								"remaining_usd": 17,
							},
							"daily": map[string]any{
								"limit_usd":     5,
								"used_usd":      1,
								"remaining_usd": 4,
							},
						},
					},
				},
			})
		case "/api/v1/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []map[string]any{}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewSub2API()
	status, err := plugin.FetchAccountStatus(context.Background(), models.Site{
		BaseURL:   server.URL,
		PluginKey: "sub2api-platform",
		Credentials: models.JSONMap{
			"access_token": "valid-access-token",
		},
		PluginConfig: models.JSONMap{
			"api_keys_url": "/api/v1/keys",
		},
	}, 5)
	if err != nil {
		t.Fatalf("FetchAccountStatus returned error: %v", err)
	}
	if status.PackageRemaining == nil || *status.PackageRemaining != 4 {
		t.Fatalf("PackageRemaining = %v", status.PackageRemaining)
	}
	if status.PackageTotal == nil || *status.PackageTotal != 5 {
		t.Fatalf("PackageTotal = %v", status.PackageTotal)
	}
	if status.PackageUsed == nil || *status.PackageUsed != 1 {
		t.Fatalf("PackageUsed = %v", status.PackageUsed)
	}
	if status.PackageUnit == nil || *status.PackageUnit != "USD" {
		t.Fatalf("PackageUnit = %v", status.PackageUnit)
	}
	if status.PackageDisplay == nil || !strings.Contains(*status.PackageDisplay, "Pro 月度套餐") {
		t.Fatalf("PackageDisplay = %v", status.PackageDisplay)
	}
	if !strings.Contains(*status.PackageDisplay, "Pro 周度套餐") || !strings.Contains(*status.PackageDisplay, "Pro 日度套餐") {
		t.Fatalf("PackageDisplay missing weekly/daily quota = %v", *status.PackageDisplay)
	}
}

func TestSub2APIStatusFallsBackToSubscriptionSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"email":   "user@example.com",
					"balance": 8.5,
				},
			})
		case "/api/v1/subscriptions/progress":
			http.NotFound(w, r)
		case "/api/v1/subscriptions/summary":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"subscriptions": []map[string]any{
						{
							"group_name":        "Team",
							"monthly_used_usd":  12.5,
							"monthly_limit_usd": 50,
							"weekly_used_usd":   2,
							"weekly_limit_usd":  10,
							"daily_used_usd":    0.5,
							"daily_limit_usd":   1.5,
						},
					},
				},
			})
		case "/api/v1/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []map[string]any{}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewSub2API()
	status, err := plugin.FetchAccountStatus(context.Background(), models.Site{
		BaseURL:   server.URL,
		PluginKey: "sub2api-platform",
		Credentials: models.JSONMap{
			"access_token": "valid-access-token",
		},
		PluginConfig: models.JSONMap{
			"api_keys_url": "/api/v1/keys",
		},
	}, 5)
	if err != nil {
		t.Fatalf("FetchAccountStatus returned error: %v", err)
	}
	if status.PackageRemaining == nil || *status.PackageRemaining != 1 {
		t.Fatalf("PackageRemaining = %v", status.PackageRemaining)
	}
	if status.PackageDisplay == nil || !strings.Contains(*status.PackageDisplay, "Team 月度套餐") {
		t.Fatalf("PackageDisplay = %v", status.PackageDisplay)
	}
	if !strings.Contains(*status.PackageDisplay, "Team 周度套餐") || !strings.Contains(*status.PackageDisplay, "Team 日度套餐") {
		t.Fatalf("PackageDisplay missing weekly/daily quota = %v", *status.PackageDisplay)
	}
}

func TestSub2APIStatusUsesPackageRemainingAsBalanceWhenProfileBalanceMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"email":    "user@example.com",
					"currency": "USD",
				},
			})
		case "/api/v1/subscriptions/progress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"subscription": map[string]any{
							"group": map[string]any{"name": "OpenToken-1.5R日卡"},
						},
						"progress": map[string]any{
							"daily": map[string]any{
								"limit_usd": 1.5,
								"used_usd":  0.4,
							},
						},
					},
				},
			})
		case "/api/v1/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []map[string]any{}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewSub2API()
	status, err := plugin.FetchAccountStatus(context.Background(), models.Site{
		BaseURL:   server.URL,
		PluginKey: "sub2api-platform",
		Credentials: models.JSONMap{
			"access_token": "valid-access-token",
		},
		PluginConfig: models.JSONMap{
			"api_keys_url": "/api/v1/keys",
		},
	}, 5)
	if err != nil {
		t.Fatalf("FetchAccountStatus returned error: %v", err)
	}
	if status.Balance == nil || *status.Balance != 1.1 {
		t.Fatalf("Balance = %v", status.Balance)
	}
	if status.BalanceUnit == nil || *status.BalanceUnit != "USD" {
		t.Fatalf("BalanceUnit = %v", status.BalanceUnit)
	}
	if status.PackageDisplay == nil || !strings.Contains(*status.PackageDisplay, "OpenToken-1.5R日卡 日度套餐") {
		t.Fatalf("PackageDisplay = %v", status.PackageDisplay)
	}
}
