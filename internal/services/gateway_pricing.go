package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ai-sign-in-gateway/internal/models"
)

const OfficialGatewayPricingSchemeID = "official"

func OfficialGatewayPricingScheme() models.GatewayPricingScheme {
	return models.GatewayPricingScheme{
		ID:       OfficialGatewayPricingSchemeID,
		Name:     "官方价格（默认）",
		Currency: "USD",
		Readonly: true,
		Source:   "OpenAI / Anthropic / Google official standard text pricing, checked 2026-05-13",
		Prices: []models.GatewayModelPrice{
			{Provider: "codex", ModelPrefix: "gpt-5.5-pro", DisplayName: "GPT-5.5 pro", InputPerMTok: 30, OutputPerMTok: 180},
			{Provider: "codex", ModelPrefix: "gpt-5.5", DisplayName: "GPT-5.5", InputPerMTok: 5, CachedInputPerMTok: 0.5, OutputPerMTok: 30},
			{Provider: "codex", ModelPrefix: "gpt-5.4-pro", DisplayName: "GPT-5.4 pro", InputPerMTok: 30, OutputPerMTok: 180},
			{Provider: "codex", ModelPrefix: "gpt-5.4-mini", DisplayName: "GPT-5.4 mini", InputPerMTok: 0.75, CachedInputPerMTok: 0.075, OutputPerMTok: 4.5},
			{Provider: "codex", ModelPrefix: "gpt-5.4-nano", DisplayName: "GPT-5.4 nano", InputPerMTok: 0.20, CachedInputPerMTok: 0.02, OutputPerMTok: 1.25},
			{Provider: "codex", ModelPrefix: "gpt-5.4", DisplayName: "GPT-5.4", InputPerMTok: 2.5, CachedInputPerMTok: 0.25, OutputPerMTok: 15},
			{Provider: "codex", ModelPrefix: "gpt-5.3-codex", DisplayName: "GPT-5.3 Codex", InputPerMTok: 1.75, CachedInputPerMTok: 0.175, OutputPerMTok: 14},
			{Provider: "codex", ModelPrefix: "gpt-5-mini", DisplayName: "GPT-5 mini", InputPerMTok: 0.25, CachedInputPerMTok: 0.025, OutputPerMTok: 2},
			{Provider: "codex", ModelPrefix: "gpt-5-nano", DisplayName: "GPT-5 nano", InputPerMTok: 0.05, CachedInputPerMTok: 0.005, OutputPerMTok: 0.4},
			{Provider: "codex", ModelPrefix: "gpt-5", DisplayName: "GPT-5", InputPerMTok: 1.25, CachedInputPerMTok: 0.125, OutputPerMTok: 10},
			{Provider: "codex", ModelPrefix: "gpt-4.1-mini", DisplayName: "GPT-4.1 mini", InputPerMTok: 0.4, CachedInputPerMTok: 0.1, OutputPerMTok: 1.6},
			{Provider: "codex", ModelPrefix: "gpt-4.1-nano", DisplayName: "GPT-4.1 nano", InputPerMTok: 0.1, CachedInputPerMTok: 0.025, OutputPerMTok: 0.4},
			{Provider: "codex", ModelPrefix: "gpt-4.1", DisplayName: "GPT-4.1", InputPerMTok: 2, CachedInputPerMTok: 0.5, OutputPerMTok: 8},
			{Provider: "codex", ModelPrefix: "gpt-4o", DisplayName: "GPT-4o", InputPerMTok: 2.5, CachedInputPerMTok: 1.25, OutputPerMTok: 10},
			{Provider: "codex", ModelPrefix: "o4-mini", DisplayName: "o4-mini", InputPerMTok: 1.1, CachedInputPerMTok: 0.275, OutputPerMTok: 4.4},
			{Provider: "codex", ModelPrefix: "o3", DisplayName: "o3", InputPerMTok: 2, CachedInputPerMTok: 0.5, OutputPerMTok: 8},

			{Provider: "claude", ModelPrefix: "claude-opus-4.7", DisplayName: "Claude Opus 4.7", InputPerMTok: 5, CachedInputPerMTok: 0.5, CacheWritePerMTok: 6.25, OutputPerMTok: 25},
			{Provider: "claude", ModelPrefix: "claude-opus-4.6", DisplayName: "Claude Opus 4.6", InputPerMTok: 5, CachedInputPerMTok: 0.5, CacheWritePerMTok: 6.25, OutputPerMTok: 25},
			{Provider: "claude", ModelPrefix: "claude-opus-4.5", DisplayName: "Claude Opus 4.5", InputPerMTok: 5, CachedInputPerMTok: 0.5, CacheWritePerMTok: 6.25, OutputPerMTok: 25},
			{Provider: "claude", ModelPrefix: "claude-sonnet-4.6", DisplayName: "Claude Sonnet 4.6", InputPerMTok: 3, CachedInputPerMTok: 0.3, CacheWritePerMTok: 3.75, OutputPerMTok: 15},
			{Provider: "claude", ModelPrefix: "claude-sonnet-4.5", DisplayName: "Claude Sonnet 4.5", InputPerMTok: 3, CachedInputPerMTok: 0.3, CacheWritePerMTok: 3.75, OutputPerMTok: 15},
			{Provider: "claude", ModelPrefix: "claude-haiku-4.5", DisplayName: "Claude Haiku 4.5", InputPerMTok: 1, CachedInputPerMTok: 0.1, CacheWritePerMTok: 1.25, OutputPerMTok: 5},
			{Provider: "claude", ModelPrefix: "claude-sonnet-4", DisplayName: "Claude Sonnet 4", InputPerMTok: 3, CachedInputPerMTok: 0.3, CacheWritePerMTok: 3.75, OutputPerMTok: 15},
			{Provider: "claude", ModelPrefix: "claude-opus-4", DisplayName: "Claude Opus 4", InputPerMTok: 15, CachedInputPerMTok: 1.5, CacheWritePerMTok: 18.75, OutputPerMTok: 75},
			{Provider: "claude", ModelPrefix: "claude-3-7-sonnet", DisplayName: "Claude 3.7 Sonnet", InputPerMTok: 3, CachedInputPerMTok: 0.3, CacheWritePerMTok: 3.75, OutputPerMTok: 15},
			{Provider: "claude", ModelPrefix: "claude-3-5-haiku", DisplayName: "Claude 3.5 Haiku", InputPerMTok: 0.8, CachedInputPerMTok: 0.08, CacheWritePerMTok: 1, OutputPerMTok: 4},

			{Provider: "gemini", ModelPrefix: "gemini-3.1-pro", DisplayName: "Gemini 3.1 Pro", InputPerMTok: 2, CachedInputPerMTok: 0.2, OutputPerMTok: 12},
			{Provider: "gemini", ModelPrefix: "gemini-3.1-flash-lite", DisplayName: "Gemini 3.1 Flash-Lite", InputPerMTok: 0.25, CachedInputPerMTok: 0.025, OutputPerMTok: 1.5},
			{Provider: "gemini", ModelPrefix: "gemini-3.1-flash-image", DisplayName: "Gemini 3.1 Flash Image", InputPerMTok: 0.5, OutputPerMTok: 3},
			{Provider: "gemini", ModelPrefix: "gemini-3.1-flash-live", DisplayName: "Gemini 3.1 Flash Live", InputPerMTok: 0.75, OutputPerMTok: 4.5},
			{Provider: "gemini", ModelPrefix: "gemini-3-flash", DisplayName: "Gemini 3 Flash", InputPerMTok: 0.5, CachedInputPerMTok: 0.05, OutputPerMTok: 3},
			{Provider: "gemini", ModelPrefix: "gemini-3-pro-image", DisplayName: "Gemini 3 Pro Image", InputPerMTok: 2, CachedInputPerMTok: 0.2, OutputPerMTok: 12},
			{Provider: "gemini", ModelPrefix: "gemini-3-pro", DisplayName: "Gemini 3 Pro", InputPerMTok: 2, CachedInputPerMTok: 0.2, OutputPerMTok: 12},
			{Provider: "gemini", ModelPrefix: "gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro", InputPerMTok: 1.25, CachedInputPerMTok: 0.125, OutputPerMTok: 10},
			{Provider: "gemini", ModelPrefix: "gemini-2.5-flash-lite", DisplayName: "Gemini 2.5 Flash-Lite", InputPerMTok: 0.1, CachedInputPerMTok: 0.01, OutputPerMTok: 0.4},
			{Provider: "gemini", ModelPrefix: "gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash", InputPerMTok: 0.3, CachedInputPerMTok: 0.03, OutputPerMTok: 2.5},
			{Provider: "gemini", ModelPrefix: "gemini-2.0-flash", DisplayName: "Gemini 2.0 Flash", InputPerMTok: 0.1, CachedInputPerMTok: 0.025, OutputPerMTok: 0.4},
			{Provider: "gemini", ModelPrefix: "gemini-2.0-flash-lite", DisplayName: "Gemini 2.0 Flash-Lite", InputPerMTok: 0.075, OutputPerMTok: 0.3},
		},
	}
}

func GatewayPricingSchemesForResponse(raw string) []models.GatewayPricingScheme {
	custom := DecodeGatewayPricingSchemes(raw)
	out := []models.GatewayPricingScheme{OfficialGatewayPricingScheme()}
	out = append(out, custom...)
	return out
}

func ResolveGatewayPricingScheme(activeID, raw string) models.GatewayPricingScheme {
	activeID = strings.TrimSpace(activeID)
	if activeID == "" || activeID == OfficialGatewayPricingSchemeID {
		return OfficialGatewayPricingScheme()
	}
	for _, scheme := range DecodeGatewayPricingSchemes(raw) {
		if scheme.ID == activeID {
			return scheme
		}
	}
	return OfficialGatewayPricingScheme()
}

func DecodeGatewayPricingSchemes(raw string) []models.GatewayPricingScheme {
	var schemes []models.GatewayPricingScheme
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &schemes) != nil {
		return nil
	}
	_, normalized := NormalizeGatewayPricingSettings("", schemes)
	return normalized
}

func EncodeGatewayPricingSchemes(schemes []models.GatewayPricingScheme) string {
	_, normalized := NormalizeGatewayPricingSettings("", schemes)
	data, err := json.Marshal(normalized)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func NormalizeGatewayPricingSettings(activeID string, schemes []models.GatewayPricingScheme) (string, []models.GatewayPricingScheme) {
	activeID = strings.TrimSpace(activeID)
	seen := map[string]bool{}
	custom := make([]models.GatewayPricingScheme, 0, len(schemes))
	for idx, scheme := range schemes {
		if scheme.Readonly || scheme.ID == OfficialGatewayPricingSchemeID {
			continue
		}
		scheme.ID = strings.TrimSpace(scheme.ID)
		if scheme.ID == "" {
			scheme.ID = fmt.Sprintf("custom-%d", idx+1)
		}
		if seen[scheme.ID] {
			continue
		}
		seen[scheme.ID] = true
		scheme.Name = strings.TrimSpace(scheme.Name)
		if scheme.Name == "" {
			scheme.Name = "自定义价格方案"
		}
		scheme.Currency = strings.ToUpper(strings.TrimSpace(scheme.Currency))
		if scheme.Currency == "" {
			scheme.Currency = "USD"
		}
		scheme.Readonly = false
		scheme.Source = strings.TrimSpace(scheme.Source)
		if scheme.Source == "" {
			scheme.Source = "custom"
		}
		scheme.Prices = normalizeGatewayModelPrices(scheme.Prices)
		custom = append(custom, scheme)
	}
	if activeID == "" || activeID == OfficialGatewayPricingSchemeID {
		return OfficialGatewayPricingSchemeID, custom
	}
	for _, scheme := range custom {
		if scheme.ID == activeID {
			return activeID, custom
		}
	}
	return OfficialGatewayPricingSchemeID, custom
}

func GatewayPriceForModel(scheme models.GatewayPricingScheme, routeType, model string) (models.GatewayModelPrice, bool) {
	model = strings.ToLower(strings.TrimSpace(model))
	routeType = normalizePricingProvider(routeType)
	if model == "" {
		return models.GatewayModelPrice{}, false
	}
	prices := normalizeGatewayModelPrices(scheme.Prices)
	for _, pass := range []bool{true, false} {
		for _, item := range prices {
			provider := normalizePricingProvider(item.Provider)
			if pass && routeType != "" && provider != "" && provider != routeType {
				continue
			}
			if strings.HasPrefix(model, strings.ToLower(strings.TrimSpace(item.ModelPrefix))) {
				return item, true
			}
		}
	}
	return models.GatewayModelPrice{}, false
}

func normalizeGatewayModelPrices(prices []models.GatewayModelPrice) []models.GatewayModelPrice {
	out := make([]models.GatewayModelPrice, 0, len(prices))
	seen := map[string]bool{}
	for _, price := range prices {
		price.Provider = normalizePricingProvider(price.Provider)
		price.ModelPrefix = strings.ToLower(strings.TrimSpace(price.ModelPrefix))
		if price.ModelPrefix == "" {
			continue
		}
		key := price.Provider + ":" + price.ModelPrefix
		if seen[key] {
			continue
		}
		seen[key] = true
		price.DisplayName = strings.TrimSpace(price.DisplayName)
		if price.DisplayName == "" {
			price.DisplayName = price.ModelPrefix
		}
		price.InputPerMTok = nonNegativeFloat(price.InputPerMTok)
		price.CachedInputPerMTok = nonNegativeFloat(price.CachedInputPerMTok)
		price.CacheWritePerMTok = nonNegativeFloat(price.CacheWritePerMTok)
		price.OutputPerMTok = nonNegativeFloat(price.OutputPerMTok)
		out = append(out, price)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return len(out[i].ModelPrefix) > len(out[j].ModelPrefix)
	})
	return out
}

func normalizePricingProvider(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "openai", "gpt":
		return "codex"
	case "anthropic":
		return "claude"
	case "google":
		return "gemini"
	default:
		return value
	}
}

func nonNegativeFloat(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}
