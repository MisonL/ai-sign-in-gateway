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
	SiteID          uint
	RouteID         uint
	OK              bool
	StatusCode      *int
	LatencyMS       *float64
	Remaining       *float64
	Unit            string
	BaseURL         string
	BalanceProbeURL string
	Message         string
	CheckedAt       time.Time
}

type BalanceProbeOptions struct {
	BalanceURL string
}

func ProbeSiteBalance(ctx context.Context, db *gorm.DB, siteID uint, timeoutSeconds int) (BalanceProbeResult, error) {
	var site models.Site
	if err := db.First(&site, siteID).Error; err != nil {
		return BalanceProbeResult{}, err
	}
	return probeBalanceForSite(ctx, db, site, 0, timeoutSeconds)
}

func ProbeGatewayRouteBalance(ctx context.Context, db *gorm.DB, routeID uint, timeoutSeconds int) (BalanceProbeResult, error) {
	return ProbeGatewayRouteBalanceWithOptions(ctx, db, routeID, timeoutSeconds, BalanceProbeOptions{})
}

func ProbeGatewayRouteBalanceWithOptions(ctx context.Context, db *gorm.DB, routeID uint, timeoutSeconds int, opts BalanceProbeOptions) (BalanceProbeResult, error) {
	route, err := GetGatewayRoute(db, fmt.Sprint(routeID))
	if err != nil {
		return BalanceProbeResult{}, err
	}
	if route.Site.ID == 0 {
		return BalanceProbeResult{}, errors.New("路由对应站点不存在，无法读取余额")
	}
	result, err := probeBalanceForRoute(ctx, db, route, timeoutSeconds, opts)
	if err != nil {
		return result, err
	}
	return result, nil
}

func probeBalanceForRoute(ctx context.Context, db *gorm.DB, route GatewayRoute, timeoutSeconds int, opts BalanceProbeOptions) (BalanceProbeResult, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 20
	}
	key := strings.TrimSpace(route.APIKey)
	if key == "" {
		return probeBalanceForRouteSiteFallback(ctx, db, route, timeoutSeconds, BalanceProbeResult{SiteID: route.State.SiteID, RouteID: route.State.ID, OK: false, Message: "路由缺少 API Key", CheckedAt: time.Now().UTC()})
	}
	candidates := gatewayRouteBasesInOrder(route)
	if len(candidates) == 0 {
		return probeBalanceForRouteSiteFallback(ctx, db, route, timeoutSeconds, BalanceProbeResult{SiteID: route.State.SiteID, RouteID: route.State.ID, OK: false, Message: "路由缺少 API 请求 URL", CheckedAt: time.Now().UTC()})
	}

	manualURL := strings.TrimSpace(opts.BalanceURL)
	if manualURL == "" {
		manualURL = strings.TrimSpace(route.State.BalanceProbeURL)
	}
	var last BalanceProbeResult
	for _, baseURL := range candidates {
		result := requestUsageBalance(ctx, route.Site, baseURL, key, timeoutSeconds, manualURL, false)
		result.SiteID = route.State.SiteID
		result.RouteID = route.State.ID
		last = result
		if !result.OK {
			continue
		}
		updateGatewayRouteBalance(db, &route.State, result)
		return result, nil
	}
	if last.Message == "" {
		last.Message = "所有 API 请求 URL 均未返回可用余额"
	}
	return probeBalanceForRouteSiteFallback(ctx, db, route, timeoutSeconds, last)
}

func probeBalanceForRouteSiteFallback(ctx context.Context, db *gorm.DB, route GatewayRoute, timeoutSeconds int, primary BalanceProbeResult) (BalanceProbeResult, error) {
	fallback, err := probeBalanceForSite(ctx, db, route.Site, route.State.ID, timeoutSeconds)
	if err != nil {
		return primary, err
	}
	if fallback.OK {
		fallback.Message = "路由 API Key 余额读取失败，已使用站点配置兜底：" + fallback.Message
		return fallback, nil
	}
	if strings.TrimSpace(primary.Message) == "" {
		primary = fallback
	} else if strings.TrimSpace(fallback.Message) != "" {
		primary.Message = primary.Message + "；站点配置兜底失败：" + fallback.Message
	}
	if primary.SiteID == 0 {
		primary.SiteID = route.State.SiteID
	}
	if primary.RouteID == 0 {
		primary.RouteID = route.State.ID
	}
	if primary.CheckedAt.IsZero() {
		primary.CheckedAt = time.Now().UTC()
	}
	return primary, nil
}

func updateGatewayRouteBalance(db *gorm.DB, state *models.GatewayRouteState, result BalanceProbeResult) {
	unit := NormalizeBalanceUnit(result.Unit)
	updates := map[string]any{
		"last_balance": result.Remaining,
	}
	if strings.TrimSpace(result.BaseURL) != "" {
		updates["last_request_base_url"] = result.BaseURL
		state.LastRequestBaseURL = result.BaseURL
	}
	if unit != "" {
		updates["balance_unit"] = unit
		state.BalanceUnit = unit
	}
	if strings.TrimSpace(result.BalanceProbeURL) != "" {
		updates["balance_probe_url"] = strings.TrimSpace(result.BalanceProbeURL)
		state.BalanceProbeURL = strings.TrimSpace(result.BalanceProbeURL)
	}
	state.LastBalance = result.Remaining
	_ = db.Model(state).Updates(updates).Error
}

func probeBalanceForSite(ctx context.Context, db *gorm.DB, site models.Site, routeID uint, timeoutSeconds int) (BalanceProbeResult, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 20
	}
	if result, ok := packageBalanceProbeResult(site, routeID); ok {
		updates := map[string]any{"last_balance": result.Remaining}
		site.LastBalance = result.Remaining
		unit := NormalizeBalanceUnit(result.Unit)
		result.Unit = unit
		if unit != "" {
			if site.PluginConfig == nil {
				site.PluginConfig = models.JSONMap{}
			}
			site.PluginConfig["balance_unit"] = unit
			updates["plugin_config"] = site.PluginConfig
		}
		_ = db.Model(&site).Updates(updates).Error
		if routeID != 0 {
			updateGatewayRouteBalanceByID(db, routeID, result)
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
		result := requestUsageBalance(ctx, site, baseURL, key, timeoutSeconds, "", true)
		result.SiteID = site.ID
		result.RouteID = routeID
		last = result
		if !result.OK {
			continue
		}
		updates := map[string]any{"last_balance": result.Remaining}
		site.LastBalance = result.Remaining
		unit := NormalizeBalanceUnit(result.Unit)
		result.Unit = unit
		if unit != "" {
			if site.PluginConfig == nil {
				site.PluginConfig = models.JSONMap{}
			}
			site.PluginConfig["balance_unit"] = unit
			site.PluginConfig["package_remaining"] = result.Remaining
			site.PluginConfig["package_unit"] = unit
			updates["plugin_config"] = site.PluginConfig
		}
		_ = db.Model(&site).Updates(updates).Error
		if routeID != 0 {
			updateGatewayRouteBalanceByID(db, routeID, result)
		}
		return result, nil
	}
	if last.Message == "" {
		last.Message = "所有 API 请求 URL 均未返回可用余额"
	}
	return last, nil
}

func updateGatewayRouteBalanceByID(db *gorm.DB, routeID uint, result BalanceProbeResult) {
	updates := map[string]any{"last_balance": result.Remaining}
	if strings.TrimSpace(result.BaseURL) != "" {
		updates["last_request_base_url"] = result.BaseURL
	}
	if unit := NormalizeBalanceUnit(result.Unit); unit != "" {
		updates["balance_unit"] = unit
	}
	if strings.TrimSpace(result.BalanceProbeURL) != "" {
		updates["balance_probe_url"] = strings.TrimSpace(result.BalanceProbeURL)
	}
	_ = db.Model(&models.GatewayRouteState{}).Where("id = ?", routeID).Updates(updates).Error
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
		unit = "$"
	}
	unit = NormalizeBalanceUnit(unit)
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

func requestUsageBalance(ctx context.Context, site models.Site, baseURL, apiKey string, timeoutSeconds int, manualURL string, includeConfigured bool) BalanceProbeResult {
	checkedAt := time.Now().UTC()
	candidates := usageURLCandidatesWithManual(site, baseURL, manualURL, includeConfigured)
	var last BalanceProbeResult
	for _, candidate := range candidates {
		result := requestUsageBalanceEndpoint(ctx, baseURL, candidate, apiKey, timeoutSeconds, checkedAt)
		if candidate.Scale != 0 && result.Remaining != nil {
			raw := *result.Remaining
			if candidate.ScaleMinAbs <= 0 || raw >= candidate.ScaleMinAbs || raw <= -candidate.ScaleMinAbs {
				value := raw / candidate.Scale
				result.Remaining = &value
			}
		}
		if candidate.Unit != "" {
			result.Unit = candidate.Unit
		}
		result.Unit = NormalizeBalanceUnit(result.Unit)
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
	URL         string
	Unit        string
	Scale       float64
	ScaleMinAbs float64
	Manual      bool
	Parser      string
}

func requestUsageBalanceEndpoint(ctx context.Context, baseURL string, candidate usageEndpointCandidate, apiKey string, timeoutSeconds int, checkedAt time.Time) BalanceProbeResult {
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, candidate.URL, nil)
	if err != nil {
		return BalanceProbeResult{OK: false, BaseURL: baseURL, BalanceProbeURL: manualProbeURL(candidate), Message: err.Error(), CheckedAt: checkedAt}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))

	start := time.Now()
	resp, err := (&http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}).Do(req)
	latency := float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		return BalanceProbeResult{OK: false, BaseURL: baseURL, BalanceProbeURL: manualProbeURL(candidate), LatencyMS: &latency, Message: err.Error(), CheckedAt: checkedAt}
	}
	defer resp.Body.Close()
	statusCode := resp.StatusCode
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if statusCode < 200 || statusCode >= 300 {
		return BalanceProbeResult{OK: false, StatusCode: &statusCode, BaseURL: baseURL, BalanceProbeURL: manualProbeURL(candidate), LatencyMS: &latency, Message: fmt.Sprintf("余额接口 %s 返回 %d: %s", candidate.URL, statusCode, shorten(string(body), 200)), CheckedAt: checkedAt}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return BalanceProbeResult{OK: false, StatusCode: &statusCode, BaseURL: baseURL, BalanceProbeURL: manualProbeURL(candidate), LatencyMS: &latency, Message: "余额接口 JSON 解析失败：" + err.Error(), CheckedAt: checkedAt}
	}
	remaining, ok := usageRemainingForCandidate(payload, candidate)
	if !ok {
		return BalanceProbeResult{OK: false, StatusCode: &statusCode, BaseURL: baseURL, BalanceProbeURL: manualProbeURL(candidate), LatencyMS: &latency, Message: "余额接口未返回 remaining / quota.remaining / balance", CheckedAt: checkedAt}
	}
	unit := strings.TrimSpace(firstStringPath(payload, "unit", "currency", "balance_unit", "quota.unit", "quota_unit", "data.unit", "data.currency", "data.balance_unit", "data.quota_unit"))
	if unit == "" {
		unit = candidate.Unit
	}
	if unit == "" {
		unit = "$"
	}
	unit = NormalizeBalanceUnit(unit)
	return BalanceProbeResult{OK: true, StatusCode: &statusCode, LatencyMS: &latency, Remaining: &remaining, Unit: unit, BaseURL: baseURL, BalanceProbeURL: manualProbeURL(candidate), Message: "余额读取成功", CheckedAt: checkedAt}
}

func manualProbeURL(candidate usageEndpointCandidate) string {
	if !candidate.Manual {
		return ""
	}
	return strings.TrimSpace(candidate.URL)
}

func usageURLCandidatesWithManual(site models.Site, baseURL string, manualURL string, includeConfigured bool) []usageEndpointCandidate {
	out := []usageEndpointCandidate{}
	if candidate, ok := manualUsageURLCandidate(baseURL, manualURL); ok {
		out = append(out, enrichUsageEndpointCandidate(site, candidate))
	}
	out = append(out, usageURLCandidatesWithOptions(site, baseURL, includeConfigured)...)
	return dedupeUsageEndpointCandidates(out)
}

func manualUsageURLCandidate(baseURL, manualURL string) (usageEndpointCandidate, bool) {
	target := strings.TrimSpace(manualURL)
	if target == "" {
		return usageEndpointCandidate{}, false
	}
	resolved := target
	if !looksLikeAbsoluteURL(target) {
		joined, err := JoinURL(baseURL, target)
		if err != nil || strings.TrimSpace(joined) == "" {
			return usageEndpointCandidate{}, false
		}
		resolved = joined
	}
	if !looksLikeAbsoluteURL(resolved) {
		return usageEndpointCandidate{}, false
	}
	return usageEndpointCandidate{URL: resolved, Manual: true}, true
}

func looksLikeAbsoluteURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func usageURLCandidates(site models.Site, baseURL string) []usageEndpointCandidate {
	return usageURLCandidatesWithOptions(site, baseURL, true)
}

func usageURLCandidatesWithOptions(site models.Site, baseURL string, includeConfigured bool) []usageEndpointCandidate {
	base := strings.ToLower(NormalizeBaseURL(baseURL))
	out := []usageEndpointCandidate{}
	if includeConfigured {
		out = configuredUsageURLCandidates(site, baseURL)
	}
	withConfigured := func(candidate usageEndpointCandidate) []usageEndpointCandidate {
		return dedupeUsageEndpointCandidates(append(out, candidate))
	}
	if strings.Contains(base, "api.deepseek.com") {
		return withConfigured(usageEndpointCandidate{URL: "https://api.deepseek.com/user/balance", Unit: "¥"})
	}
	if strings.Contains(base, "api.stepfun.ai") || strings.Contains(base, "api.stepfun.com") {
		return withConfigured(usageEndpointCandidate{URL: "https://api.stepfun.com/v1/accounts", Unit: "¥"})
	}
	if strings.Contains(base, "api.siliconflow.cn") {
		return withConfigured(usageEndpointCandidate{URL: "https://api.siliconflow.cn/v1/user/info", Unit: "¥"})
	}
	if strings.Contains(base, "api.siliconflow.com") {
		return withConfigured(usageEndpointCandidate{URL: "https://api.siliconflow.com/v1/user/info", Unit: "$"})
	}
	if strings.Contains(base, "openrouter.ai") {
		return withConfigured(usageEndpointCandidate{URL: "https://openrouter.ai/api/v1/credits", Unit: "$"})
	}
	if strings.Contains(base, "api.novita.ai") {
		return withConfigured(usageEndpointCandidate{URL: "https://api.novita.ai/v3/user/balance", Unit: "$", Scale: 10000})
	}
	if shouldProbeNewAPIUsage(site, baseURL) {
		if target, err := newAPIUsageURL(baseURL); err == nil && target != "" {
			out = append(out, newAPIUsageEndpointCandidate(site, target, false))
		}
	}
	if target, err := usageURL(baseURL); err == nil && target != "" {
		out = append(out, usageEndpointCandidate{URL: target, Unit: "$"})
	}
	return dedupeUsageEndpointCandidates(out)
}

func configuredUsageURLCandidates(site models.Site, baseURL string) []usageEndpointCandidate {
	out := []usageEndpointCandidate{}
	for _, key := range []string{
		"balance_probe_url",
		"usage_balance_url",
		"balance_url",
		"usage_url",
		"quota_url",
	} {
		if candidate, ok := manualUsageURLCandidate(baseURL, stringMapValue(site.PluginConfig, key, "")); ok {
			candidate.Manual = false
			out = append(out, enrichUsageEndpointCandidate(site, candidate))
		}
	}
	if strings.EqualFold(strings.TrimSpace(site.PluginKey), "http-relay-station") {
		if candidate, ok := manualUsageURLCandidate(baseURL, stringMapValue(site.PluginConfig, "status_path", "")); ok {
			candidate.Manual = false
			out = append(out, enrichUsageEndpointCandidate(site, candidate))
		}
	}
	return dedupeUsageEndpointCandidates(out)
}

func enrichUsageEndpointCandidate(site models.Site, candidate usageEndpointCandidate) usageEndpointCandidate {
	candidate.Unit = NormalizeBalanceUnit(candidate.Unit)
	if isNewAPIQuotaUsageCandidate(candidate.URL) {
		return newAPIUsageEndpointCandidate(site, candidate.URL, candidate.Manual)
	}
	return candidate
}

func newAPIUsageEndpointCandidate(site models.Site, target string, manual bool) usageEndpointCandidate {
	return usageEndpointCandidate{
		URL:         target,
		Unit:        "$",
		Scale:       quotaPerUnitFromSiteConfig(site),
		ScaleMinAbs: 1000,
		Manual:      manual,
		Parser:      "newapi-token-usage",
	}
}

func isNewAPIQuotaUsageCandidate(target string) bool {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return strings.Contains(strings.ToLower(target), "/api/usage/token")
	}
	return strings.Contains(strings.ToLower(parsed.Path), "/api/usage/token")
}

func dedupeUsageEndpointCandidates(in []usageEndpointCandidate) []usageEndpointCandidate {
	out := []usageEndpointCandidate{}
	for _, item := range in {
		item.URL = strings.TrimSpace(item.URL)
		if item.URL == "" {
			continue
		}
		duplicate := false
		for _, existing := range out {
			if existing.URL == item.URL {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, item)
		}
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

func quotaPerUnitFromSiteConfig(site models.Site) float64 {
	value, ok := numericMapValue(site.PluginConfig, "quota_per_unit")
	if !ok || value <= 0 {
		return 500000.0
	}
	return value
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

func usageRemainingForCandidate(payload map[string]any, candidate usageEndpointCandidate) (float64, bool) {
	if candidate.Parser == "newapi-token-usage" {
		return newAPIUsageRemaining(payload)
	}
	return usageRemaining(payload)
}

func newAPIUsageRemaining(payload map[string]any) (float64, bool) {
	for _, path := range []string{
		"data.total_available",
		"data.remaining",
		"data.remain_quota",
		"data.quota_remaining",
		"data.quota_remain",
		"data.available_quota",
		"data.available",
		"data.balance",
		"remaining",
		"remain_quota",
		"quota.remaining",
		"quota_remain",
		"available_quota",
		"balance",
	} {
		if value, ok := numericPath(payload, path); ok {
			return value, true
		}
	}

	total, hasTotal := firstNumericPath(payload,
		"data.total_granted",
		"data.total",
		"data.total_quota",
		"data.quota_total",
		"data.amount_total",
		"data.total_credits",
		"total_granted",
		"total",
		"total_quota",
		"quota.total",
		"amount_total",
		"total_credits",
	)
	used, hasUsed := firstNumericPath(payload,
		"data.total_used",
		"data.used",
		"data.used_quota",
		"data.quota_used",
		"data.amount_used",
		"data.total_usage",
		"used",
		"used_quota",
		"quota.used",
		"amount_used",
		"total_usage",
	)
	if hasTotal && hasUsed {
		return total - used, true
	}

	// NewAPI token-usage payloads may include data.quota as a configured quota or
	// ledger counter. A negative quota without explicit remaining semantics is not
	// a reliable balance and caused huge bogus route balances after unit scaling.
	if quota, ok := firstNumericPath(payload, "data.quota", "quota"); ok && quota >= 0 && !newAPIUsagePayloadHasUsageCounters(payload) {
		return quota, true
	}
	return 0, false
}

func newAPIUsagePayloadHasUsageCounters(payload map[string]any) bool {
	_, hasUsed := firstNumericPath(payload,
		"data.total_used",
		"data.used",
		"data.used_quota",
		"data.quota_used",
		"data.amount_used",
		"data.total_usage",
		"used",
		"used_quota",
		"quota.used",
		"amount_used",
		"total_usage",
	)
	if hasUsed {
		return true
	}
	_, hasTotal := firstNumericPath(payload,
		"data.total_granted",
		"data.total",
		"data.total_quota",
		"data.quota_total",
		"data.amount_total",
		"data.total_credits",
		"total_granted",
		"total",
		"total_quota",
		"quota.total",
		"amount_total",
		"total_credits",
	)
	return hasTotal
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

func firstStringPath(payload map[string]any, paths ...string) string {
	for _, path := range paths {
		if value := stringPath(payload, path, ""); value != "" {
			return value
		}
	}
	return ""
}

func firstSiteAPIKey(site models.Site) string {
	if value := strings.TrimSpace(stringMapValue(site.Credentials, "api_key", "")); value != "" {
		return value
	}
	keys := siteAPIKeys(site)
	if len(keys) == 0 {
		return ""
	}
	return keys[0].Value
}
