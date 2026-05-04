package services

import (
	"context"
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

func TestUsageURLCandidatesPreferKnownProvider(t *testing.T) {
	candidates := usageURLCandidates(models.Site{}, "https://openrouter.ai/api/v1")
	if len(candidates) != 1 {
		t.Fatalf("candidate len = %d", len(candidates))
	}
	if candidates[0].URL != "https://openrouter.ai/api/v1/credits" {
		t.Fatalf("candidate URL = %s", candidates[0].URL)
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

	var stored models.Site
	if err := db.First(&stored, site.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.LastBalance == nil || *stored.LastBalance != 82.82 {
		t.Fatalf("stored.LastBalance = %v", stored.LastBalance)
	}
}
