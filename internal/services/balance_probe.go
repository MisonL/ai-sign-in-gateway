package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ai-sign-in-gateway/internal/models"
	"gorm.io/gorm"
)

type BalanceProbeResult struct {
	SiteID     uint
	RouteID    uint
	OK         bool
	StatusCode *int
	LatencyMS  *float64
	Remaining  *float64
	Unit       string
	BaseURL    string
	Message    string
	CheckedAt  time.Time
}

func ProbeSiteBalance(ctx context.Context, db *gorm.DB, siteID uint, timeoutSeconds int) (BalanceProbeResult, error) {
	var site models.Site
	if err := db.First(&site, siteID).Error; err != nil {
		return BalanceProbeResult{}, err
	}
	return probeBalanceForSite(ctx, db, site, 0, timeoutSeconds)
}

func ProbeGatewayRouteBalance(ctx context.Context, db *gorm.DB, routeID uint, timeoutSeconds int) (BalanceProbeResult, error) {
	route, err := GetGatewayRoute(db, fmt.Sprint(routeID))
	if err != nil {
		return BalanceProbeResult{}, err
	}
	if route.Site.ID == 0 {
		return BalanceProbeResult{}, errors.New("路由对应站点不存在，无法读取余额")
	}
	result, err := probeBalanceForSite(ctx, db, route.Site, route.State.ID, timeoutSeconds)
	if err != nil {
		return result, err
	}
	return result, nil
}

func probeBalanceForSite(ctx context.Context, db *gorm.DB, site models.Site, routeID uint, timeoutSeconds int) (BalanceProbeResult, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 20
	}
	if result, ok := packageBalanceProbeResult(site, routeID); ok {
		updates := map[string]any{"last_balance": result.Remaining}
		site.LastBalance = result.Remaining
		if result.Unit != "" {
			if site.PluginConfig == nil {
				site.PluginConfig = models.JSONMap{}
			}
			site.PluginConfig["balance_unit"] = result.Unit
			updates["plugin_config"] = site.PluginConfig
		}
		_ = db.Model(&site).Updates(updates).Error
		if routeID != 0 {
			_ = db.Model(&models.GatewayRouteState{}).Where("id = ?", routeID).Update("last_request_base_url", result.BaseURL).Error
		}
		return result, nil
	}
	key := firstSiteAPIKey(site)
	if key == "" {
		return BalanceProbeResult{SiteID: site.ID, RouteID: routeID, OK: false, Message: "站点缺少 API Key", CheckedAt: time.Now().UTC()}, nil
	}
	candidates := GatewayRequestBaseCandidates(site)
	if len(candidates) == 0 {
		return BalanceProbeResult{SiteID: site.ID, RouteID: routeID, OK: false, Message: "站点缺少 API 请求 URL", CheckedAt: time.Now().UTC()}, nil
	}

	var last BalanceProbeResult
	for _, baseURL := range candidates {
		result := requestUsageBalance(ctx, site, baseURL, key, timeoutSeconds)
		result.SiteID = site.ID
		result.RouteID = routeID
		last = result
		if !result.OK {
			continue
		}
		updates := map[string]any{"last_balance": result.Remaining}
		site.LastBalance = result.Remaining
		if result.Unit != "" {
			if site.PluginConfig == nil {
				site.PluginConfig = models.JSONMap{}
			}
			site.PluginConfig["balance_unit"] = result.Unit
			site.PluginConfig["package_remaining"] = result.Remaining
			site.PluginConfig["package_unit"] = result.Unit
			updates["plugin_config"] = site.PluginConfig
		}
		_ = db.Model(&site).Updates(updates).Error
		if routeID != 0 {
			_ = db.Model(&models.GatewayRouteState{}).Where("id = ?", routeID).Update("last_request_base_url", result.BaseURL).Error
		}
		return result, nil
	}
	if last.Message == "" {
		last.Message = "所有 API 请求 URL 均未返回可用余额"
	}
	return last, nil
}

func packageBalanceProbeResult(site models.Site, routeID uint) (BalanceProbeResult, bool) {
	if !strings.EqualFold(strings.TrimSpace(site.PluginKey), "sub2api-platform") {
		return BalanceProbeResult{}, false
	}
	remaining, ok := numericMapValue(site.PluginConfig, "package_remaining")
	if !ok {
		return BalanceProbeResult{}, false
	}
	unit := strings.TrimSpace(stringMapValue(site.PluginConfig, "package_unit", ""))
	if unit == "" {
		unit = strings.TrimSpace(stringMapValue(site.PluginConfig, "balance_unit", ""))
	}
	if unit == "" {
		unit = "USD"
	}
	return BalanceProbeResult{
		SiteID:    site.ID,
		RouteID:   routeID,
		OK:        true,
		Remaining: &remaining,
		Unit:      unit,
		BaseURL:   firstNonEmpty(site.BaseURL, GatewayRequestBase(site)),
		Message:   "已使用当前套餐余量作为余额",
		CheckedAt: time.Now().UTC(),
	}, true
}

func requestUsageBalance(ctx context.Context, site models.Site, baseURL, apiKey string, timeoutSeconds int) BalanceProbeResult {
	checkedAt := time.Now().UTC()
	candidates := usageURLCandidates(site, baseURL)
	var last BalanceProbeResult
	for _, candidate := range candidates {
		result := requestUsageBalanceEndpoint(ctx, baseURL, candidate, apiKey, timeoutSeconds, checkedAt)
		if candidate.Scale != 0 && result.Remaining != nil {
			value := *result.Remaining / candidate.Scale
			result.Remaining = &value
		}
		if candidate.Unit != "" {
			result.Unit = candidate.Unit
		}
		last = result
		if result.OK {
			return result
		}
	}
	if last.CheckedAt.IsZero() {
		return BalanceProbeResult{OK: false, BaseURL: baseURL, Message: "未找到可用余额接口", CheckedAt: checkedAt}
	}
	return last
}

type usageEndpointCandidate struct {
	URL   string
	Unit  string
	Scale float64
}

func requestUsageBalanceEndpoint(ctx context.Context, baseURL string, candidate usageEndpointCandidate, apiKey string, timeoutSeconds int, checkedAt time.Time) BalanceProbeResult {
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, candidate.URL, nil)
	if err != nil {
		return BalanceProbeResult{OK: false, BaseURL: baseURL, Message: err.Error(), CheckedAt: checkedAt}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))

	start := time.Now()
	resp, err := (&http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}).Do(req)
	latency := float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		return BalanceProbeResult{OK: false, BaseURL: baseURL, LatencyMS: &latency, Message: err.Error(), CheckedAt: checkedAt}
	}
	defer resp.Body.Close()
	statusCode := resp.StatusCode
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if statusCode < 200 || statusCode >= 300 {
		return BalanceProbeResult{OK: false, StatusCode: &statusCode, BaseURL: baseURL, LatencyMS: &latency, Message: fmt.Sprintf("余额接口 %s 返回 %d: %s", candidate.URL, statusCode, shorten(string(body), 200)), CheckedAt: checkedAt}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return BalanceProbeResult{OK: false, StatusCode: &statusCode, BaseURL: baseURL, LatencyMS: &latency, Message: "余额接口 JSON 解析失败：" + err.Error(), CheckedAt: checkedAt}
	}
	remaining, ok := usageRemaining(payload)
	if !ok {
		return BalanceProbeResult{OK: false, StatusCode: &statusCode, BaseURL: baseURL, LatencyMS: &latency, Message: "余额接口未返回 remaining / quota.remaining / balance", CheckedAt: checkedAt}
	}
	unit := strings.TrimSpace(stringPath(payload, "unit", stringPath(payload, "quota.unit", stringPath(payload, "data.unit", ""))))
	if unit == "" {
		unit = candidate.Unit
	}
	if unit == "" {
		unit = "USD"
	}
	return BalanceProbeResult{OK: true, StatusCode: &statusCode, LatencyMS: &latency, Remaining: &remaining, Unit: unit, BaseURL: baseURL, Message: "余额读取成功", CheckedAt: checkedAt}
}

func usageURLCandidates(site models.Site, baseURL string) []usageEndpointCandidate {
	base := strings.ToLower(NormalizeBaseURL(baseURL))
	if strings.Contains(base, "api.deepseek.com") {
		return []usageEndpointCandidate{{URL: "https://api.deepseek.com/user/balance", Unit: "CNY"}}
	}
	if strings.Contains(base, "api.stepfun.ai") || strings.Contains(base, "api.stepfun.com") {
		return []usageEndpointCandidate{{URL: "https://api.stepfun.com/v1/accounts", Unit: "CNY"}}
	}
	if strings.Contains(base, "api.siliconflow.cn") {
		return []usageEndpointCandidate{{URL: "https://api.siliconflow.cn/v1/user/info", Unit: "CNY"}}
	}
	if strings.Contains(base, "api.siliconflow.com") {
		return []usageEndpointCandidate{{URL: "https://api.siliconflow.com/v1/user/info", Unit: "USD"}}
	}
	if strings.Contains(base, "openrouter.ai") {
		return []usageEndpointCandidate{{URL: "https://openrouter.ai/api/v1/credits", Unit: "USD"}}
	}
	if strings.Contains(base, "api.novita.ai") {
		return []usageEndpointCandidate{{URL: "https://api.novita.ai/v3/user/balance", Unit: "USD", Scale: 10000}}
	}
	out := []usageEndpointCandidate{}
	if shouldProbeNewAPIUsage(site, baseURL) {
		if target, err := newAPIUsageURL(baseURL); err == nil && target != "" {
			out = append(out, usageEndpointCandidate{URL: target, Unit: "$"})
		}
	}
	if target, err := usageURL(baseURL); err == nil && target != "" {
		out = append(out, usageEndpointCandidate{URL: target, Unit: "USD"})
	}
	return out
}

func shouldProbeNewAPIUsage(site models.Site, baseURL string) bool {
	if strings.EqualFold(strings.TrimSpace(site.PluginKey), "yellowpeach-newapi") {
		return true
	}
	if value := strings.TrimSpace(stringMapValue(site.PluginConfig, "usage_balance_url", "")); value != "" && strings.Contains(value, "api/usage/token") {
		return true
	}
	text := strings.ToLower(strings.Join([]string{
		site.Name,
		site.BaseURL,
		baseURL,
		stringMapValue(site.PluginConfig, "api_platform", ""),
		stringMapValue(site.PluginConfig, "platform", ""),
	}, " "))
	return strings.Contains(text, "newapi") || strings.Contains(text, "new-api") || strings.Contains(text, "yellowpeach")
}

func usageURL(baseURL string) (string, error) {
	base := NormalizeBaseURL(baseURL)
	if strings.HasSuffix(strings.TrimRight(base, "/"), "/v1") {
		return JoinURL(base, "usage")
	}
	return JoinURL(base, "/v1/usage")
}

func newAPIUsageURL(baseURL string) (string, error) {
	parsed, err := url.Parse(NormalizeBaseURL(baseURL))
	if err != nil {
		return "", err
	}
	parsed.Path = strings.TrimRight(strings.TrimSuffix(parsed.Path, "/v1"), "/") + "/api/usage/token/"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func usageRemaining(payload map[string]any) (float64, bool) {
	for _, path := range []string{
		"remaining",
		"quota.remaining",
		"balance",
		"total_balance",
		"availableBalance",
		"data.total_available",
		"data.remaining",
		"data.balance",
		"data.totalBalance",
		"data.chargeBalance",
		"data.quota",
		"data.remain_quota",
		"data.quota_remain",
		"data.quota_remaining",
		"data.available_quota",
	} {
		if value, ok := numericPath(payload, path); ok {
			return value, true
		}
	}
	if remaining, ok := deepSeekBalanceRemaining(payload); ok {
		return remaining, true
	}
	total, hasTotal := firstNumericPath(payload, "total", "total_quota", "total_credits", "quota.total", "data.total", "data.total_quota", "data.quota_total", "data.amount_total", "data.total_granted", "data.total_credits")
	used, hasUsed := firstNumericPath(payload, "used", "used_quota", "total_usage", "quota.used", "data.used", "data.used_quota", "data.quota_used", "data.amount_used", "data.total_used", "data.total_usage")
	if hasTotal && hasUsed {
		return total - used, true
	}
	return 0, false
}

func deepSeekBalanceRemaining(payload map[string]any) (float64, bool) {
	items, ok := payload["balance_infos"].([]any)
	if !ok {
		return 0, false
	}
	total := 0.0
	found := false
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		value, ok := firstNumericPath(obj, "total_balance", "granted_balance", "topped_up_balance")
		if !ok {
			continue
		}
		total += value
		found = true
	}
	return total, found
}

func firstNumericPath(payload map[string]any, paths ...string) (float64, bool) {
	for _, path := range paths {
		if value, ok := numericPath(payload, path); ok {
			return value, true
		}
	}
	return 0, false
}

func numericPath(payload map[string]any, path string) (float64, bool) {
	parts := strings.Split(path, ".")
	var current any = payload
	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return 0, false
		}
		current, ok = obj[part]
		if !ok || current == nil {
			return 0, false
		}
	}
	switch typed := current.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case json.Number:
		value, err := typed.Float64()
		return value, err == nil
	case string:
		var value float64
		_, err := fmt.Sscan(strings.TrimSpace(typed), &value)
		return value, err == nil
	default:
		return 0, false
	}
}

func stringPath(payload map[string]any, path string, fallback string) string {
	parts := strings.Split(path, ".")
	var current any = payload
	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return fallback
		}
		current, ok = obj[part]
		if !ok || current == nil {
			return fallback
		}
	}
	value := strings.TrimSpace(fmt.Sprint(current))
	if value == "" {
		return fallback
	}
	return value
}

func firstSiteAPIKey(site models.Site) string {
	keys := siteAPIKeys(site)
	if len(keys) == 0 {
		return ""
	}
	return keys[0].Value
}
