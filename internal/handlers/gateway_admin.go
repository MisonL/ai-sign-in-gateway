package handlers

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"ai-sign-in-gateway/internal/httpx"
	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/services"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type gormDB = gorm.DB

func (a *App) GatewayAdminRoutes(r chi.Router) {
	r.Get("/overview", a.GatewayOverview)
	r.Get("/usage", a.GatewayUsage)
	r.Get("/settings", a.GetGatewaySettings)
	r.Put("/settings", a.UpdateGatewaySettings)
	r.Post("/sync", a.SyncGatewayRoutes)
	r.Get("/routes", a.GatewayRoutes)
	r.Get("/active-requests", a.GatewayActiveRequests)
	r.Post("/routes/probe", a.ProbeGatewayRoutes)
	r.Post("/routes/priorities/reorder", a.ReorderGatewayRoutePriorities)
	r.Post("/routes/disable-all", a.DisableAllGatewayRoutes)
	r.Post("/routes/{routeID}/toggle", a.ToggleGatewayRoute)
	r.Post("/routes/{routeID}/enable-only", a.EnableOnlyGatewayRoute)
	r.Post("/routes/{routeID}/reset-circuit", a.ResetGatewayCircuit)
	r.Patch("/routes/{routeID}/type", a.UpdateGatewayRouteType)
	r.Get("/routes/{routeID}/diagnose", a.DiagnoseGatewayRoute)
	r.Post("/routes/{routeID}/probe", a.ProbeGatewayRoute)
	r.Post("/routes/{routeID}/balance-probe", a.ProbeGatewayRouteBalance)
	r.Get("/routes/{routeID}/logs", a.GatewayRouteLogs)
	r.Get("/logs", a.GatewayLogs)
}

func (a *App) GatewayOverview(w http.ResponseWriter, r *http.Request) {
	settings, _ := a.systemSettings()
	var total, healthy, open, halfOpen, disabled int64
	a.DB.Model(&models.GatewayRouteState{}).Count(&total)
	a.DB.Model(&models.GatewayRouteState{}).Where("is_enabled = ? AND circuit_state = ?", true, "closed").Count(&healthy)
	a.DB.Model(&models.GatewayRouteState{}).Where("circuit_state = ?", "open").Count(&open)
	a.DB.Model(&models.GatewayRouteState{}).Where("circuit_state = ?", "half_open").Count(&halfOpen)
	a.DB.Model(&models.GatewayRouteState{}).Where("is_enabled = ?", false).Count(&disabled)

	now := time.Now().UTC()
	since24h := now.Add(-24 * time.Hour)
	var logs []models.GatewayRequestLog
	_ = a.DB.Where("created_at >= ?", since24h).Order("created_at desc").Find(&logs).Error

	requestIDs := map[string]struct{}{}
	successCount := 0
	latencySum := 0.0
	latencySamples := 0
	for _, log := range logs {
		requestIDs[log.RequestID] = struct{}{}
		if log.Success {
			successCount++
			if log.LatencyMS != nil {
				latencySum += *log.LatencyMS
				latencySamples++
			}
		}
	}
	successRate := 0.0
	if len(logs) > 0 {
		successRate = round2(float64(successCount) / float64(len(logs)) * 100)
	}
	var avgLatency any = nil
	if latencySamples > 0 {
		avgLatency = round2(latencySum / float64(latencySamples))
	}

	totalBalance, quantified := totalBalanceForActiveRoutes(a.DB)

	writeJSON(w, http.StatusOK, map[string]any{
		"total_routes":                  total,
		"healthy_routes":                healthy,
		"open_circuit_routes":           open,
		"half_open_routes":              halfOpen,
		"disabled_routes":               disabled,
		"total_balance_display":         totalBalance,
		"quantified_balance_site_count": quantified,
		"active_concurrency":            services.RouteTotalActive(),
		"request_count_24h":             len(requestIDs),
		"success_rate_24h":              successRate,
		"avg_latency_ms_24h":            avgLatency,
		"strategy_breakdown_24h":        strategyBreakdown24h(logs),
		"route_strategy":                settings.GatewayRouteStrategy,
		"failure_threshold":             settings.GatewayFailureThreshold,
		"cooldown_seconds":              settings.GatewayCooldownSeconds,
		"request_timeout":               settings.GatewayRequestTimeout,
		"max_attempts":                  settings.GatewayMaxAttempts,
		"failure_retry_mode":            services.NormalizeGatewayFailureRetryMode(settings.GatewayFailureRetryMode),
		"route_concurrency_limit":       settings.GatewayRouteConcurrencyLimit,
		"concurrency_transfer_strategy": normalizeGatewayConcurrencyTransferStrategy(settings.GatewayConcurrencyTransferStrategy),
		"concurrency_overflow_strategy": settings.GatewayConcurrencyOverflowStrategy,
	})
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

func totalBalanceForActiveRoutes(db *gormDB) (any, int) {
	var siteIDs []uint
	_ = db.Model(&models.GatewayRouteState{}).Where("is_enabled = ?", true).Distinct("site_id").Pluck("site_id", &siteIDs).Error
	if len(siteIDs) == 0 {
		return nil, 0
	}
	var sites []models.Site
	if err := db.Where("id IN ?", siteIDs).Find(&sites).Error; err != nil {
		return nil, 0
	}
	total := 0.0
	quantified := 0
	for _, site := range sites {
		if site.LastBalance == nil {
			continue
		}
		quantified++
		total += *site.LastBalance
	}
	if quantified == 0 {
		return nil, 0
	}
	if total < 0 {
		return "-$" + formatFloat(-total), quantified
	}
	return "$" + formatFloat(total), quantified
}

func formatFloat(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(round2(v), 'f', -1, 64)
}

func (a *App) GatewayUsage(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).UTC()
	end := now.UTC()
	var err error
	if raw := strings.TrimSpace(r.URL.Query().Get("start")); raw != "" {
		start, err = parseGatewayUsageTime(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "开始时间格式无效")
			return
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("end")); raw != "" {
		end, err = parseGatewayUsageTime(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "结束时间格式无效")
			return
		}
	}
	if !end.After(start) {
		writeError(w, http.StatusBadRequest, "结束时间必须晚于开始时间")
		return
	}
	if end.Sub(start) > 366*24*time.Hour {
		writeError(w, http.StatusBadRequest, "查询时间段不能超过 366 天")
		return
	}

	var logs []models.GatewayRequestLog
	if err := a.DB.Preload("Site").Where("created_at >= ? AND created_at < ?", start, end).Order("created_at desc").Find(&logs).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.gatewayUsageResponse(logs, start, end))
}

func parseGatewayUsageTime(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.Local); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", raw, time.Local); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, errors.New("invalid time")
}

type gatewayUsageAgg struct {
	RouteID            *uint
	RouteLabel         string
	SiteID             *uint
	SiteName           *string
	KeyName            string
	KeyFingerprint     string
	GroupName          string
	RouteType          string
	RequestCount       int
	SuccessCount       int
	FailureCount       int
	StreamRequestCount int
	PromptTokens       int
	CachedInputTokens  int
	CompletionTokens   int
	TotalTokens        int
	UsageCost          float64
	hasUsageCost       bool
	latencySum         float64
	latencySamples     int
	lastUsedAt         time.Time
}

func (a *App) gatewayUsageResponse(logs []models.GatewayRequestLog, start, end time.Time) map[string]any {
	routeByID, routeBySiteKey := a.gatewayLogRouteLookup(logs)
	total := gatewayUsageAgg{}
	groups := map[string]*gatewayUsageAgg{}
	for _, log := range logs {
		total.RequestCount++
		if log.Success {
			total.SuccessCount++
		} else {
			total.FailureCount++
		}
		if log.IsStream {
			total.StreamRequestCount++
		}
		addGatewayUsageTokens(&total, log)
		addGatewayUsageLatency(&total, log)

		state, matched := models.GatewayRouteState{}, false
		if log.RouteStateID != nil {
			state, matched = routeByID[*log.RouteStateID]
		}
		if !matched && log.SiteID != nil {
			state, matched = routeBySiteKey[gatewayLogRouteKey(*log.SiteID, log.KeyFingerprint)]
		}

		routeID := log.RouteStateID
		if matched {
			id := state.ID
			routeID = &id
		}
		routeKey := "unknown"
		if routeID != nil && *routeID > 0 {
			routeKey = "route:" + strconv.FormatUint(uint64(*routeID), 10)
		} else if log.SiteID != nil {
			routeKey = gatewayLogRouteKey(*log.SiteID, log.KeyFingerprint)
		}
		agg, ok := groups[routeKey]
		if !ok {
			var siteName *string
			if log.Site != nil {
				siteName = &log.Site.Name
			}
			if siteName == nil && matched {
				name := services.GatewayRouteSiteLabel(services.GatewayRoute{State: state, Site: state.Site})
				siteName = &name
			}
			agg = &gatewayUsageAgg{
				RouteID:        routeID,
				SiteID:         log.SiteID,
				SiteName:       siteName,
				KeyName:        log.KeyName,
				KeyFingerprint: log.KeyFingerprint,
				GroupName:      log.GroupName,
				RouteType:      state.RouteType,
				RouteLabel:     gatewayLogRouteLabel(log, state, matched, siteName),
			}
			if matched {
				if agg.SiteID == nil {
					siteID := state.SiteID
					agg.SiteID = &siteID
				}
				if strings.TrimSpace(agg.KeyName) == "" {
					agg.KeyName = state.KeyName
				}
				if strings.TrimSpace(agg.GroupName) == "" {
					agg.GroupName = state.GroupName
				}
			}
			groups[routeKey] = agg
		}
		agg.RequestCount++
		if log.Success {
			agg.SuccessCount++
		} else {
			agg.FailureCount++
		}
		if log.IsStream {
			agg.StreamRequestCount++
		}
		addGatewayUsageTokens(agg, log)
		addGatewayUsageLatency(agg, log)
		if log.CreatedAt.After(agg.lastUsedAt) {
			agg.lastUsedAt = log.CreatedAt
		}
	}

	routes := make([]map[string]any, 0, len(groups))
	for _, agg := range groups {
		routes = append(routes, gatewayUsageAggResponse(agg))
	}
	sortGatewayUsageRoutes(routes)
	out := gatewayUsageAggResponse(&total)
	out["start"] = start.Format(time.RFC3339)
	out["end"] = end.Format(time.RFC3339)
	out["routes"] = routes
	return out
}

func addGatewayUsageTokens(agg *gatewayUsageAgg, log models.GatewayRequestLog) {
	if log.PromptTokens != nil {
		agg.PromptTokens += *log.PromptTokens
	}
	if log.CachedInputTokens != nil {
		agg.CachedInputTokens += *log.CachedInputTokens
	}
	if log.CompletionTokens != nil {
		agg.CompletionTokens += *log.CompletionTokens
	}
	if log.TotalTokens != nil {
		agg.TotalTokens += *log.TotalTokens
	}
	if log.UsageCost != nil {
		agg.UsageCost += *log.UsageCost
		agg.hasUsageCost = true
	}
}

func addGatewayUsageLatency(agg *gatewayUsageAgg, log models.GatewayRequestLog) {
	if log.Success && log.LatencyMS != nil {
		agg.latencySum += *log.LatencyMS
		agg.latencySamples++
	}
}

func gatewayUsageAggResponse(agg *gatewayUsageAgg) map[string]any {
	var avgLatency any
	if agg.latencySamples > 0 {
		avgLatency = round2(agg.latencySum / float64(agg.latencySamples))
	}
	var usageCost any
	if agg.hasUsageCost {
		usageCost = round2(agg.UsageCost)
	}
	officialCost := gatewayOfficialUsageCost(agg.PromptTokens, agg.CachedInputTokens, agg.CompletionTokens)
	successRate := 0.0
	if agg.RequestCount > 0 {
		successRate = round2(float64(agg.SuccessCount) / float64(agg.RequestCount) * 100)
	}
	return map[string]any{
		"route_id":             agg.RouteID,
		"route_label":          agg.RouteLabel,
		"site_id":              agg.SiteID,
		"site_name":            agg.SiteName,
		"key_name":             agg.KeyName,
		"key_fingerprint":      agg.KeyFingerprint,
		"group_name":           agg.GroupName,
		"route_type":           agg.RouteType,
		"request_count":        agg.RequestCount,
		"success_count":        agg.SuccessCount,
		"failure_count":        agg.FailureCount,
		"success_rate":         successRate,
		"stream_request_count": agg.StreamRequestCount,
		"prompt_tokens":        agg.PromptTokens,
		"cached_input_tokens":  agg.CachedInputTokens,
		"completion_tokens":    agg.CompletionTokens,
		"total_tokens":         agg.TotalTokens,
		"usage_cost":           usageCost,
		"official_input_cost":  roundCost(float64(agg.PromptTokens) / 1_000_000 * 5),
		"official_cached_cost": roundCost(float64(agg.CachedInputTokens) / 1_000_000 * 0.5),
		"official_output_cost": roundCost(float64(agg.CompletionTokens) / 1_000_000 * 30),
		"official_total_cost":  officialCost,
		"avg_latency_ms":       avgLatency,
		"last_used_at":         nullableTime(agg.lastUsedAt),
	}
}

func gatewayOfficialUsageCost(promptTokens, cachedInputTokens, completionTokens int) float64 {
	return roundCost(float64(promptTokens)/1_000_000*5 + float64(cachedInputTokens)/1_000_000*0.5 + float64(completionTokens)/1_000_000*30)
}

func roundCost(value float64) float64 {
	return float64(int64(value*1_000_000+0.5)) / 1_000_000
}

func sortGatewayUsageRoutes(routes []map[string]any) {
	sort.SliceStable(routes, func(i, j int) bool {
		leftTokens, _ := routes[i]["total_tokens"].(int)
		rightTokens, _ := routes[j]["total_tokens"].(int)
		if leftTokens != rightTokens {
			return leftTokens > rightTokens
		}
		leftRequests, _ := routes[i]["request_count"].(int)
		rightRequests, _ := routes[j]["request_count"].(int)
		if leftRequests != rightRequests {
			return leftRequests > rightRequests
		}
		leftLabel, _ := routes[i]["route_label"].(string)
		rightLabel, _ := routes[j]["route_label"].(string)
		return strings.TrimSpace(leftLabel) < strings.TrimSpace(rightLabel)
	})
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}

type strategyAgg struct {
	requests   int
	successes  int
	latencies  []float64
	stream     int
	streamSucc int
	streamLat  []float64
}

func strategyBreakdown24h(logs []models.GatewayRequestLog) []map[string]any {
	order := []string{"smart", "round_robin", "latency_first", "priority"}
	groups := map[string]*strategyAgg{}
	for _, log := range logs {
		strategy := strings.TrimSpace(log.RouteStrategy)
		switch strategy {
		case "smart", "round_robin", "latency_first", "priority":
		default:
			continue
		}
		agg, ok := groups[strategy]
		if !ok {
			agg = &strategyAgg{}
			groups[strategy] = agg
		}
		agg.requests++
		if log.Success {
			agg.successes++
			if log.LatencyMS != nil {
				agg.latencies = append(agg.latencies, *log.LatencyMS)
			}
		}
		if log.IsStream {
			agg.stream++
			if log.Success {
				agg.streamSucc++
				if log.LatencyMS != nil {
					agg.streamLat = append(agg.streamLat, *log.LatencyMS)
				}
			}
		}
	}
	out := []map[string]any{}
	for _, key := range order {
		agg, ok := groups[key]
		if !ok || agg.requests == 0 {
			continue
		}
		var avgLatency any
		if len(agg.latencies) > 0 {
			avgLatency = round2(sumFloat(agg.latencies) / float64(len(agg.latencies)))
		}
		var avgStreamTTFB any
		if len(agg.streamLat) > 0 {
			avgStreamTTFB = round2(sumFloat(agg.streamLat) / float64(len(agg.streamLat)))
		}
		streamSuccessRate := 0.0
		if agg.stream > 0 {
			streamSuccessRate = round2(float64(agg.streamSucc) / float64(agg.stream) * 100)
		}
		out = append(out, map[string]any{
			"route_strategy":       key,
			"request_count":        agg.requests,
			"success_rate":         round2(float64(agg.successes) / float64(agg.requests) * 100),
			"avg_latency_ms":       avgLatency,
			"stream_request_count": agg.stream,
			"stream_success_rate":  streamSuccessRate,
			"avg_stream_ttfb_ms":   avgStreamTTFB,
		})
	}
	return out
}

func sumFloat(in []float64) float64 {
	total := 0.0
	for _, v := range in {
		total += v
	}
	return total
}

func (a *App) GetGatewaySettings(w http.ResponseWriter, r *http.Request) {
	settings, _ := a.systemSettings()
	writeJSON(w, http.StatusOK, gatewaySettings(settings))
}

func (a *App) UpdateGatewaySettings(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	_ = httpx.Decode(r, &payload)
	settings, err := a.systemSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if value, ok := payload["route_strategy"].(string); ok {
		settings.GatewayRouteStrategy = value
	}
	if value, ok := payload["concurrency_overflow_strategy"].(string); ok {
		settings.GatewayConcurrencyOverflowStrategy = value
	}
	if value, ok := payload["concurrency_transfer_strategy"].(string); ok {
		settings.GatewayConcurrencyTransferStrategy = normalizeGatewayConcurrencyTransferStrategy(value)
	}
	if value, ok := payload["failure_retry_mode"].(string); ok {
		settings.GatewayFailureRetryMode = services.NormalizeGatewayFailureRetryMode(value)
	}
	if value, ok := payload["gateway_api_key"].(string); ok {
		settings.GatewayAPIKey = value
	}
	if value, ok := payload["failure_threshold"].(float64); ok {
		settings.GatewayFailureThreshold = int(value)
	}
	if value, ok := payload["cooldown_seconds"].(float64); ok {
		settings.GatewayCooldownSeconds = int(value)
	}
	if value, ok := payload["request_timeout"].(float64); ok {
		settings.GatewayRequestTimeout = int(value)
	}
	if value, ok := payload["max_attempts"].(float64); ok {
		settings.GatewayMaxAttempts = int(value)
	}
	if value, ok := payload["route_concurrency_limit"].(float64); ok {
		settings.GatewayRouteConcurrencyLimit = int(value)
	}
	if value, ok := payload["smart_latency_bias"].(float64); ok {
		settings.GatewaySmartLatencyBias = clampBiasFromAPI(value)
	}
	if value, ok := payload["smart_concurrency_bias"].(float64); ok {
		settings.GatewaySmartConcurrencyBias = clampBiasFromAPI(value)
	}
	if value, ok := payload["smart_failure_bias"].(float64); ok {
		settings.GatewaySmartFailureBias = clampBiasFromAPI(value)
	}
	if value, ok := payload["smart_priority_bias"].(float64); ok {
		settings.GatewaySmartPriorityBias = clampBiasFromAPI(value)
	}
	_ = a.DB.Save(&settings).Error
	writeJSON(w, http.StatusOK, gatewaySettings(settings))
}

func clampBiasFromAPI(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 5 {
		return 5
	}
	return value
}

func gatewaySettings(settings models.SystemSetting) map[string]any {
	return map[string]any{
		"route_strategy":                settings.GatewayRouteStrategy,
		"failure_threshold":             settings.GatewayFailureThreshold,
		"cooldown_seconds":              settings.GatewayCooldownSeconds,
		"request_timeout":               settings.GatewayRequestTimeout,
		"max_attempts":                  settings.GatewayMaxAttempts,
		"failure_retry_mode":            services.NormalizeGatewayFailureRetryMode(settings.GatewayFailureRetryMode),
		"route_concurrency_limit":       settings.GatewayRouteConcurrencyLimit,
		"concurrency_transfer_strategy": normalizeGatewayConcurrencyTransferStrategy(settings.GatewayConcurrencyTransferStrategy),
		"concurrency_overflow_strategy": settings.GatewayConcurrencyOverflowStrategy,
		"smart_latency_bias":            settings.GatewaySmartLatencyBias,
		"smart_concurrency_bias":        settings.GatewaySmartConcurrencyBias,
		"smart_failure_bias":            settings.GatewaySmartFailureBias,
		"smart_priority_bias":           settings.GatewaySmartPriorityBias,
		"gateway_api_key":               settings.GatewayAPIKey,
	}
}

func (a *App) SyncGatewayRoutes(w http.ResponseWriter, r *http.Request) {
	count, err := services.SyncGatewayRoutes(a.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "route_count": count})
}
func (a *App) GatewayRoutes(w http.ResponseWriter, r *http.Request) {
	includeDisabled := r.URL.Query().Get("include_disabled") != "false"
	routes, err := services.ListGatewayRoutes(a.DB, r.URL.Query().Get("group"), includeDisabled)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(routes))
	for _, route := range routes {
		out = append(out, gatewayRouteResponse(route))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) GatewayActiveRequests(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, services.ListGatewayActiveRequests())
}

func (a *App) ReorderGatewayRoutePriorities(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		RouteID uint   `json:"route_id"`
		Mode    string `json:"mode"`
		Index   *int   `json:"index"`
	}
	if err := httpx.Decode(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	mode := services.GatewayRoutePriorityMode(strings.TrimSpace(payload.Mode))
	opts := services.GatewayRoutePriorityReorderOptions{RouteID: payload.RouteID, Mode: mode}
	if payload.Index != nil {
		opts.Index = *payload.Index
	}
	if mode == services.GatewayRoutePriorityMove && payload.Index == nil {
		writeError(w, http.StatusBadRequest, "移动优先级需要提供目标优先级")
		return
	}
	routes, err := services.ReorderGatewayRoutePriorities(a.DB, opts)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(routes))
	for _, route := range routes {
		out = append(out, gatewayRouteResponse(route))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) GatewayLogs(w http.ResponseWriter, r *http.Request) {
	limit := 80
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	var logs []models.GatewayRequestLog
	if err := a.DB.Preload("Site").Order("created_at desc").Limit(limit).Find(&logs).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.gatewayLogResponse(logs))
}

func (a *App) GatewayRouteLogs(w http.ResponseWriter, r *http.Request) {
	routeID, err := strconv.ParseUint(chi.URLParam(r, "routeID"), 10, 64)
	if err != nil || routeID == 0 {
		writeError(w, http.StatusBadRequest, "路由 ID 无效")
		return
	}
	limit := 80
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	var logs []models.GatewayRequestLog
	if err := a.DB.Preload("Site").Where("route_state_id = ?", uint(routeID)).Order("created_at desc").Limit(limit).Find(&logs).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.gatewayLogResponse(logs))
}

func (a *App) gatewayLogResponse(logs []models.GatewayRequestLog) []map[string]any {
	routeByID, routeBySiteKey := a.gatewayLogRouteLookup(logs)
	out := make([]map[string]any, 0, len(logs))
	for _, item := range logs {
		var siteName *string
		if item.Site != nil {
			siteName = &item.Site.Name
		}
		routeState, routeMatched := models.GatewayRouteState{}, false
		if item.RouteStateID != nil {
			routeState, routeMatched = routeByID[*item.RouteStateID]
		}
		if !routeMatched && item.SiteID != nil {
			routeState, routeMatched = routeBySiteKey[gatewayLogRouteKey(*item.SiteID, item.KeyFingerprint)]
		}
		routeID := item.RouteStateID
		if routeMatched {
			id := routeState.ID
			routeID = &id
			if siteName == nil {
				label := services.GatewayRouteSiteLabel(services.GatewayRoute{State: routeState, Site: routeState.Site})
				siteName = &label
			}
		}
		out = append(out, map[string]any{
			"id":                   item.ID,
			"request_id":           item.RequestID,
			"route_id":             routeID,
			"route_label":          gatewayLogRouteLabel(item, routeState, routeMatched, siteName),
			"site_id":              item.SiteID,
			"site_name":            siteName,
			"key_name":             item.KeyName,
			"key_fingerprint":      item.KeyFingerprint,
			"group_name":           item.GroupName,
			"target_path":          item.TargetPath,
			"method":               item.Method,
			"route_strategy":       item.RouteStrategy,
			"attempt_index":        item.AttemptIndex,
			"status_code":          item.StatusCode,
			"success":              item.Success,
			"latency_ms":           item.LatencyMS,
			"prompt_tokens":        item.PromptTokens,
			"cached_input_tokens":  item.CachedInputTokens,
			"completion_tokens":    item.CompletionTokens,
			"total_tokens":         item.TotalTokens,
			"usage_cost":           item.UsageCost,
			"circuit_state_before": item.CircuitStateBefore,
			"failure_reason":       item.FailureReason,
			"is_stream":            item.IsStream,
			"created_at":           item.CreatedAt,
		})
	}
	return out
}

func (a *App) gatewayLogRouteLookup(logs []models.GatewayRequestLog) (map[uint]models.GatewayRouteState, map[string]models.GatewayRouteState) {
	routeIDSet := map[uint]bool{}
	siteIDSet := map[uint]bool{}
	for _, item := range logs {
		if item.RouteStateID != nil && *item.RouteStateID > 0 {
			routeIDSet[*item.RouteStateID] = true
		}
		if item.SiteID != nil && *item.SiteID > 0 {
			siteIDSet[*item.SiteID] = true
		}
	}
	routeIDs := make([]uint, 0, len(routeIDSet))
	for id := range routeIDSet {
		routeIDs = append(routeIDs, id)
	}
	siteIDs := make([]uint, 0, len(siteIDSet))
	for id := range siteIDSet {
		siteIDs = append(siteIDs, id)
	}

	states := make([]models.GatewayRouteState, 0)
	if len(routeIDs) > 0 {
		var byID []models.GatewayRouteState
		_ = a.DB.Preload("Site").Where("id IN ?", routeIDs).Find(&byID).Error
		states = append(states, byID...)
	}
	if len(siteIDs) > 0 {
		var bySite []models.GatewayRouteState
		_ = a.DB.Preload("Site").Where("site_id IN ?", siteIDs).Find(&bySite).Error
		states = append(states, bySite...)
	}

	routeByID := map[uint]models.GatewayRouteState{}
	routeBySiteKey := map[string]models.GatewayRouteState{}
	for _, state := range states {
		routeByID[state.ID] = state
		routeBySiteKey[gatewayLogRouteKey(state.SiteID, state.KeyFingerprint)] = state
	}
	return routeByID, routeBySiteKey
}

func gatewayLogRouteKey(siteID uint, fingerprint string) string {
	return strconv.FormatUint(uint64(siteID), 10) + ":" + strings.TrimSpace(fingerprint)
}

func gatewayLogRouteLabel(log models.GatewayRequestLog, state models.GatewayRouteState, matched bool, siteName *string) string {
	parts := []string{}
	if matched && state.ID > 0 {
		parts = append(parts, "#"+strconv.FormatUint(uint64(state.ID), 10))
	} else if log.RouteStateID != nil && *log.RouteStateID > 0 {
		parts = append(parts, "#"+strconv.FormatUint(uint64(*log.RouteStateID), 10))
	}
	if siteName != nil && strings.TrimSpace(*siteName) != "" {
		parts = append(parts, strings.TrimSpace(*siteName))
	}
	keyName := strings.TrimSpace(log.KeyName)
	if keyName == "" && matched {
		keyName = strings.TrimSpace(state.KeyName)
	}
	if keyName != "" {
		parts = append(parts, keyName)
	}
	if len(parts) == 0 && log.SiteID != nil {
		parts = append(parts, "站点 #"+strconv.FormatUint(uint64(*log.SiteID), 10))
	}
	if len(parts) == 0 {
		parts = append(parts, "未知路由")
	}
	return strings.Join(parts, " · ")
}
func (a *App) ToggleGatewayRoute(w http.ResponseWriter, r *http.Request) {
	var state models.GatewayRouteState
	if err := a.DB.First(&state, chi.URLParam(r, "routeID")).Error; err != nil {
		writeError(w, http.StatusNotFound, "网关路由不存在")
		return
	}
	state.IsEnabled = !state.IsEnabled
	if err := a.DB.Save(&state).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "保存路由状态失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": state.ID, "is_enabled": state.IsEnabled, "circuit_state": state.CircuitState})
}

func (a *App) DisableAllGatewayRoutes(w http.ResponseWriter, r *http.Request) {
	if _, err := services.SyncGatewayRoutes(a.DB); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := a.DB.Model(&models.GatewayRouteState{}).Where("is_enabled = ?", true).Update("is_enabled", false)
	if result.Error != nil {
		writeError(w, http.StatusInternalServerError, "禁用全部路由失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "disabled_count": result.RowsAffected})
}

func (a *App) EnableOnlyGatewayRoute(w http.ResponseWriter, r *http.Request) {
	var state models.GatewayRouteState
	if err := a.DB.First(&state, chi.URLParam(r, "routeID")).Error; err != nil {
		writeError(w, http.StatusNotFound, "网关路由不存在")
		return
	}
	if err := a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.GatewayRouteState{}).Where("id <> ?", state.ID).Update("is_enabled", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.GatewayRouteState{}).Where("id = ?", state.ID).Update("is_enabled", true).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "设置唯一启用路由失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "enabled_route_id": state.ID})
}
func (a *App) ResetGatewayCircuit(w http.ResponseWriter, r *http.Request) {
	var state models.GatewayRouteState
	if err := a.DB.First(&state, chi.URLParam(r, "routeID")).Error; err != nil {
		writeError(w, http.StatusNotFound, "网关路由不存在")
		return
	}
	state.CircuitState = "closed"
	state.ConsecutiveFailures = 0
	state.CircuitOpenedAt = nil
	state.CircuitOpenUntil = nil
	_ = a.DB.Save(&state).Error
	writeJSON(w, http.StatusOK, map[string]any{"id": state.ID, "is_enabled": state.IsEnabled, "circuit_state": state.CircuitState})
}
func (a *App) UpdateGatewayRouteType(w http.ResponseWriter, r *http.Request) {
	var payload map[string]string
	_ = httpx.Decode(r, &payload)
	routeType := normalizeGatewayRouteType(payload["route_type"])
	if routeType == "" {
		writeError(w, http.StatusBadRequest, "route_type 必须是 claude/codex(gpt)/gemini")
		return
	}
	var state models.GatewayRouteState
	if err := a.DB.Preload("Site").First(&state, chi.URLParam(r, "routeID")).Error; err != nil {
		writeError(w, http.StatusNotFound, "网关路由不存在")
		return
	}
	state.RouteType = routeType
	state.RouteTypeManual = true
	if err := a.DB.Save(&state).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, gatewayRouteResponse(services.GatewayRoute{State: state, Site: state.Site, RequestBaseURL: services.GatewayRouteRequestBase(state, state.Site)}))
}

func (a *App) DiagnoseGatewayRoute(w http.ResponseWriter, r *http.Request) {
	routeID, err := strconv.ParseUint(chi.URLParam(r, "routeID"), 10, 64)
	if err != nil || routeID == 0 {
		writeError(w, http.StatusBadRequest, "路由 ID 无效")
		return
	}
	var state models.GatewayRouteState
	if err := a.DB.Preload("Site").First(&state, uint(routeID)).Error; err != nil {
		writeError(w, http.StatusNotFound, "网关路由不存在")
		return
	}
	route := services.GatewayRoute{
		State:          state,
		Site:           state.Site,
		APIKey:         services.GatewayRouteAPIKeyForState(state),
		RequestBaseURL: services.GatewayRouteRequestBase(state, state.Site),
	}
	settings, _ := a.systemSettings()
	items := gatewayRouteDiagnosisItems(route, settings)
	healthy := true
	for _, item := range items {
		if severity, ok := item["severity"].(string); ok && severity == "error" {
			healthy = false
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":           state.ID,
		"healthy":      healthy,
		"route_label":  services.GatewayRouteSiteLabel(route),
		"route":        gatewayRouteResponse(route),
		"diagnostics":  items,
		"checked_at":   time.Now().UTC(),
		"active_count": services.RouteActiveCount(state.ID),
	})
}

func gatewayRouteDiagnosisItems(route services.GatewayRoute, settings models.SystemSetting) []map[string]any {
	state := route.State
	activeCount := services.RouteActiveCount(state.ID)
	limit := settings.GatewayRouteConcurrencyLimit
	items := []map[string]any{
		gatewayRouteDiagnosisItem("站点状态", route.Site.IsEnabled, boolLabel(route.Site.IsEnabled, "站点已启用", "站点已停用"), "站点停用后不会参与网关调度。"),
		gatewayRouteDiagnosisItem("路由状态", state.IsEnabled, boolLabel(state.IsEnabled, "路由已启用", "路由已停用"), "路由停用后不会参与网关调度。"),
		gatewayRouteDiagnosisItem("API Key", strings.TrimSpace(route.APIKey) != "", boolLabel(strings.TrimSpace(route.APIKey) != "", "已匹配可用 Key", "未匹配到可用 Key"), "未匹配到 Key 时请先更新站点 API Key，再同步路由池。"),
		gatewayRouteDiagnosisItem("请求入口", strings.TrimSpace(route.RequestBaseURL) != "", firstNonEmpty(route.RequestBaseURL, "未配置请求入口"), "请求入口来自站点基础 URL 或请求 API URL 配置。"),
		gatewayRouteDiagnosisItem("熔断状态", state.CircuitState == "" || state.CircuitState == "closed", firstNonEmpty(state.CircuitState, "closed"), "open/half_open 状态会影响路由被选择。"),
	}
	concurrencyOK := limit <= 0 || activeCount < limit
	concurrencyMessage := strconv.Itoa(activeCount)
	if limit > 0 {
		concurrencyMessage = strconv.Itoa(activeCount) + "/" + strconv.Itoa(limit)
	}
	items = append(items, gatewayRouteDiagnosisItem("当前并发", concurrencyOK, concurrencyMessage, "达到并发上限时该路由会被降权或进入溢出策略。"))
	if state.LastError != nil && strings.TrimSpace(*state.LastError) != "" {
		items = append(items, map[string]any{
			"label":    "最近异常",
			"ok":       false,
			"severity": "warning",
			"message":  strings.TrimSpace(*state.LastError),
			"detail":   "最近异常不一定代表当前不可用，可点击探测确认。",
		})
	}
	if route.Site.ID == 0 {
		items = append(items, map[string]any{
			"label":    "站点记录",
			"ok":       false,
			"severity": "error",
			"message":  "站点记录缺失",
			"detail":   "请同步路由池清理孤立路由。",
		})
	}
	return items
}

func gatewayRouteDiagnosisItem(label string, ok bool, message, detail string) map[string]any {
	severity := "ok"
	if !ok {
		severity = "error"
	}
	return map[string]any{
		"label":    label,
		"ok":       ok,
		"severity": severity,
		"message":  message,
		"detail":   detail,
	}
}

func boolLabel(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func (a *App) ProbeGatewayRoutes(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		RouteIDs []uint `json:"route_ids"`
	}
	_ = httpx.Decode(r, &payload)
	settings, _ := a.systemSettings()
	out := []map[string]any{}
	if len(payload.RouteIDs) == 0 {
		writeJSON(w, http.StatusOK, out)
		return
	}
	for _, routeID := range payload.RouteIDs {
		result, err := services.ProbeGatewayRoute(r.Context(), a.DB, strconv.FormatUint(uint64(routeID), 10), settings.GatewayRequestTimeout)
		if err != nil {
			continue
		}
		out = append(out, gatewayProbeResponse(result))
	}
	writeJSON(w, http.StatusOK, out)
}
func (a *App) ProbeGatewayRoute(w http.ResponseWriter, r *http.Request) {
	settings, _ := a.systemSettings()
	result, err := services.ProbeGatewayRoute(r.Context(), a.DB, chi.URLParam(r, "routeID"), settings.GatewayRequestTimeout)
	if err != nil {
		writeError(w, http.StatusNotFound, "网关路由不存在")
		return
	}
	writeJSON(w, http.StatusOK, gatewayProbeResponse(result))
}

func (a *App) ProbeGatewayRouteBalance(w http.ResponseWriter, r *http.Request) {
	routeID, err := strconv.ParseUint(chi.URLParam(r, "routeID"), 10, 64)
	if err != nil || routeID == 0 {
		writeError(w, http.StatusBadRequest, "路由 ID 无效")
		return
	}
	settings, _ := a.systemSettings()
	result, err := services.ProbeGatewayRouteBalance(r.Context(), a.DB, uint(routeID), settings.GatewayRequestTimeout)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, balanceProbeResponse(result))
}

func (a *App) GatewayProxy(w http.ResponseWriter, r *http.Request) {
	settings, _ := a.systemSettings()
	gatewayAPIKey := strings.TrimSpace(settings.GatewayAPIKey)
	if gatewayAPIKey == "" {
		writeError(w, http.StatusServiceUnavailable, "网关 API Key 未配置，公开网关已禁用")
		return
	}
	header := r.Header.Get("Authorization")
	expected := "Bearer " + gatewayAPIKey
	if header != expected {
		writeError(w, http.StatusUnauthorized, "网关 API Key 无效")
		return
	}
	targetPath := gatewayProxyTargetPath(r.URL.Path)
	policy := services.GatewayPolicy{
		RouteStrategy:               settings.GatewayRouteStrategy,
		FailureThreshold:            settings.GatewayFailureThreshold,
		CooldownSeconds:             settings.GatewayCooldownSeconds,
		RequestTimeout:              settings.GatewayRequestTimeout,
		MaxAttempts:                 settings.GatewayMaxAttempts,
		FailureRetryMode:            settings.GatewayFailureRetryMode,
		RouteConcurrencyLimit:       settings.GatewayRouteConcurrencyLimit,
		ConcurrencyTransferStrategy: normalizeGatewayConcurrencyTransferStrategy(settings.GatewayConcurrencyTransferStrategy),
		ConcurrencyOverflowStrategy: settings.GatewayConcurrencyOverflowStrategy,
		SmartLatencyBias:            settings.GatewaySmartLatencyBias,
		SmartConcurrencyBias:        settings.GatewaySmartConcurrencyBias,
		SmartFailureBias:            settings.GatewaySmartFailureBias,
		SmartPriorityBias:           settings.GatewaySmartPriorityBias,
	}
	opts := services.ProxyGatewayOptions{
		ResponseWriter: w,
		Group:          r.URL.Query().Get("group"),
		RouteType:      normalizeGatewayRouteType(firstNonEmpty(r.URL.Query().Get("type"), r.URL.Query().Get("route_type"))),
	}
	_, err := services.ProxyGatewayRequestWithOptions(r.Context(), a.DB, r, targetPath, opts, policy)
	if err != nil {
		// service did not write anything yet (no candidate)
		status := http.StatusBadGateway
		var allRoutesFailed services.GatewayAllRoutesFailedError
		var nonRetryableUpstream services.GatewayNonRetryableUpstreamError
		if errors.As(err, &allRoutesFailed) || strings.Contains(err.Error(), "没有可用") {
			status = http.StatusServiceUnavailable
		} else if errors.As(err, &nonRetryableUpstream) {
			status = http.StatusBadGateway
		}
		writeError(w, status, err.Error())
		return
	}
}

func gatewayProxyTargetPath(path string) string {
	switch {
	case path == "/api/gateway/v1" || path == "/api/gateway":
		return ""
	case strings.HasPrefix(path, "/api/gateway/v1/"):
		return strings.TrimPrefix(path, "/api/gateway/v1/")
	case strings.HasPrefix(path, "/api/gateway/"):
		return strings.TrimPrefix(path, "/api/gateway/")
	default:
		return strings.TrimLeft(path, "/")
	}
}

func normalizeGatewayConcurrencyTransferStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "limit_only", "balance":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "limit_only"
	}
}

func gatewayRouteResponse(route services.GatewayRoute) map[string]any {
	state := route.State
	successRate := 0.0
	if state.RequestCount > 0 {
		successRate = round2(float64(state.SuccessCount) / float64(state.RequestCount) * 100)
	}
	out := map[string]any{
		"id":                     state.ID,
		"site_id":                state.SiteID,
		"site_name":              services.GatewayRouteSiteLabel(route),
		"base_url":               firstNonEmpty(route.Site.BaseURL, state.SiteBaseURLSnapshot),
		"request_base_url":       route.RequestBaseURL,
		"request_base_urls":      services.GatewayRouteRequestBaseCandidates(state, route.Site),
		"last_request_base_url":  state.LastRequestBaseURL,
		"site_name_snapshot":     state.SiteNameSnapshot,
		"site_base_url_snapshot": state.SiteBaseURLSnapshot,
		"site_missing":           route.Site.ID == 0,
		"has_api_key":            route.APIKey != "",
		"group_name":             state.GroupName,
		"last_balance":           route.Site.LastBalance,
		"balance_display":        balanceDisplayWithUnit(route.Site.LastBalance, jsonMapString(route.Site.PluginConfig, "balance_unit")),
		"package_display":        packageDisplay(route.Site),
		"checkin_status":         route.Site.LastStatus,
		"key_name":               state.KeyName,
		"key_fingerprint":        state.KeyFingerprint,
		"key_source":             state.KeySource,
		"route_type":             state.RouteType,
		"route_type_manual":      state.RouteTypeManual,
		"route_priority":         state.RoutePriority,
		"route_priority_manual":  state.RoutePriorityManual,
		"weight":                 state.Weight,
		"is_enabled":             state.IsEnabled,
		"circuit_state":          state.CircuitState,
		"consecutive_failures":   state.ConsecutiveFailures,
		"active_concurrency":     services.RouteActiveCount(state.ID),
		"request_count":          state.RequestCount,
		"success_count":          state.SuccessCount,
		"failure_count":          state.FailureCount,
		"avg_latency_ms":         state.AvgLatencyMS,
		"ewma_latency_ms":        state.EWMALatencyMS,
		"last_latency_ms":        state.LastLatencyMS,
		"success_rate":           successRate,
		"last_status_code":       state.LastStatusCode,
		"last_error":             state.LastError,
		"last_used_at":           state.LastUsedAt,
		"last_success_at":        state.LastSuccessAt,
		"last_failure_at":        state.LastFailureAt,
		"circuit_open_until":     state.CircuitOpenUntil,
	}
	for key, value := range packageQuotaMap(route.Site) {
		out[key] = value
	}
	return out
}

func gatewayProbeResponse(result services.GatewayProbeResult) map[string]any {
	state := result.Route.State
	return map[string]any{"id": state.ID, "site_id": state.SiteID, "site_name": services.GatewayRouteSiteLabel(result.Route), "request_base_url": result.Route.RequestBaseURL, "key_name": state.KeyName, "key_fingerprint": state.KeyFingerprint, "ok": result.OK, "status_code": result.StatusCode, "latency_ms": result.LatencyMS, "message": result.Message, "models": result.Models, "last_status_code": state.LastStatusCode, "last_error": state.LastError, "last_latency_ms": state.LastLatencyMS, "last_success_at": state.LastSuccessAt, "last_failure_at": state.LastFailureAt, "checked_at": result.CheckedAt}
}

func normalizeGatewayRouteType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "claude", "anthropic":
		return "claude"
	case "codex", "gpt", "openai", "chatgpt":
		return "codex"
	case "gemini", "google":
		return "gemini"
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
