package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
		result := requestUsageBalance(ctx, baseURL, key, timeoutSeconds)
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

func requestUsageBalance(ctx context.Context, baseURL, apiKey string, timeoutSeconds int) BalanceProbeResult {
	checkedAt := time.Now().UTC()
	target, err := usageURL(baseURL)
	if err != nil {
		return BalanceProbeResult{OK: false, BaseURL: baseURL, Message: err.Error(), CheckedAt: checkedAt}
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
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
		return BalanceProbeResult{OK: false, StatusCode: &statusCode, BaseURL: baseURL, LatencyMS: &latency, Message: fmt.Sprintf("余额接口返回 %d: %s", statusCode, shorten(string(body), 200)), CheckedAt: checkedAt}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return BalanceProbeResult{OK: false, StatusCode: &statusCode, BaseURL: baseURL, LatencyMS: &latency, Message: "余额接口 JSON 解析失败：" + err.Error(), CheckedAt: checkedAt}
	}
	remaining, ok := usageRemaining(payload)
	if !ok {
		return BalanceProbeResult{OK: false, StatusCode: &statusCode, BaseURL: baseURL, LatencyMS: &latency, Message: "余额接口未返回 remaining / quota.remaining / balance", CheckedAt: checkedAt}
	}
	unit := strings.TrimSpace(stringPath(payload, "unit", stringPath(payload, "quota.unit", "USD")))
	if unit == "" {
		unit = "USD"
	}
	return BalanceProbeResult{OK: true, StatusCode: &statusCode, LatencyMS: &latency, Remaining: &remaining, Unit: unit, BaseURL: baseURL, Message: "余额读取成功", CheckedAt: checkedAt}
}

func usageURL(baseURL string) (string, error) {
	base := NormalizeBaseURL(baseURL)
	if strings.HasSuffix(strings.TrimRight(base, "/"), "/v1") {
		return JoinURL(base, "usage")
	}
	return JoinURL(base, "/v1/usage")
}

func usageRemaining(payload map[string]any) (float64, bool) {
	for _, path := range []string{"remaining", "quota.remaining", "balance"} {
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
