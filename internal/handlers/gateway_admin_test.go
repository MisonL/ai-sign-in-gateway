package handlers

import (
	"testing"
	"time"

	"ai-sign-in-gateway/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestGatewayTotalBalanceUsesRouteUnits(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:gateway-total-balance?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	usdPrimary := 10.5
	usdDisabled := 1.25
	cny := 20.0
	routes := []models.GatewayRouteState{
		{SiteID: 1, KeyFingerprint: "key-a", LastBalance: &usdPrimary, BalanceUnit: "USD", IsEnabled: true},
		{SiteID: 2, KeyFingerprint: "key-b", LastBalance: &cny, BalanceUnit: "CNY", IsEnabled: true},
		{SiteID: 3, KeyFingerprint: "key-c", LastBalance: &usdDisabled, BalanceUnit: "$", IsEnabled: false},
	}
	if err := db.Create(&routes).Error; err != nil {
		t.Fatalf("create routes: %v", err)
	}

	display, count := totalBalanceForRoutes(db)
	if count != 3 {
		t.Fatalf("count = %d", count)
	}
	if display != "$11.75 / ¥20" {
		t.Fatalf("display = %v", display)
	}
}

func TestGatewayUsageCostSummaryUsesModelInputCacheOutput(t *testing.T) {
	prompt, cached, completion, total := 1000, 250, 500, 1500
	logs := []models.GatewayRequestLog{
		{
			Model:             "gpt-5.5",
			PromptTokens:      &prompt,
			CachedInputTokens: &cached,
			CompletionTokens:  &completion,
			TotalTokens:       &total,
			Success:           true,
			CreatedAt:         time.Now().UTC(),
		},
	}

	summary := gatewayUsageCostSummary(logs)
	if got := summary["input_cost"]; got != 0.00375 {
		t.Fatalf("input_cost = %v", got)
	}
	if got := summary["cached_cost"]; got != 0.000125 {
		t.Fatalf("cached_cost = %v", got)
	}
	if got := summary["output_cost"]; got != 0.015 {
		t.Fatalf("output_cost = %v", got)
	}
	if got := summary["total_cost"]; got != 0.018875 {
		t.Fatalf("total_cost = %v", got)
	}
	if got := summary["known_requests"]; got != 1 {
		t.Fatalf("known_requests = %v", got)
	}
	if got := summary["unknown_requests"]; got != 0 {
		t.Fatalf("unknown_requests = %v", got)
	}
}
