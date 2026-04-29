package handlers

import (
	"net/http"
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
	r.Get("/settings", a.GetGatewaySettings)
	r.Put("/settings", a.UpdateGatewaySettings)
	r.Post("/sync", a.SyncGatewayRoutes)
	r.Get("/routes", a.GatewayRoutes)
	r.Post("/routes/probe", a.ProbeGatewayRoutes)
	r.Post("/routes/{routeID}/toggle", a.ToggleGatewayRoute)
	r.Post("/routes/{routeID}/reset-circuit", a.ResetGatewayCircuit)
	r.Patch("/routes/{routeID}/type", a.UpdateGatewayRouteType)
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
		"recent_trend_5m":               recentTrend5m(logs, now),
		"route_strategy":                settings.GatewayRouteStrategy,
		"failure_threshold":             settings.GatewayFailureThreshold,
		"cooldown_seconds":              settings.GatewayCooldownSeconds,
		"request_timeout":               settings.GatewayRequestTimeout,
		"max_attempts":                  settings.GatewayMaxAttempts,
		"route_concurrency_limit":       settings.GatewayRouteConcurrencyLimit,
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

const (
	trendBucketSeconds = 1
	trendBucketCount   = 60
)

func recentTrend5m(logs []models.GatewayRequestLog, now time.Time) []map[string]any {
	endEpoch := now.Unix()
	endAligned := (endEpoch/int64(trendBucketSeconds) + 1) * int64(trendBucketSeconds)
	starts := make([]int64, trendBucketCount)
	for i := 0; i < trendBucketCount; i++ {
		starts[trendBucketCount-1-i] = endAligned - int64((i+1)*trendBucketSeconds)
	}
	earliest := starts[0]
	latest := starts[trendBucketCount-1] + int64(trendBucketSeconds)

	type bucketAgg struct {
		total, success, failure, stream int
		latencies                       []float64
	}
	buckets := make(map[int64]*bucketAgg, trendBucketCount)
	for _, s := range starts {
		buckets[s] = &bucketAgg{}
	}
	for _, log := range logs {
		ts := log.CreatedAt.Unix()
		if ts < earliest || ts >= latest {
			continue
		}
		bucketStart := (ts / int64(trendBucketSeconds)) * int64(trendBucketSeconds)
		agg, ok := buckets[bucketStart]
		if !ok {
			continue
		}
		agg.total++
		if log.Success {
			agg.success++
			if log.LatencyMS != nil {
				agg.latencies = append(agg.latencies, *log.LatencyMS)
			}
		} else {
			agg.failure++
		}
		if log.IsStream {
			agg.stream++
		}
	}
	out := make([]map[string]any, 0, trendBucketCount)
	for _, s := range starts {
		agg := buckets[s]
		var avgLatency any
		if len(agg.latencies) > 0 {
			avgLatency = round2(sumFloat(agg.latencies) / float64(len(agg.latencies)))
		}
		out = append(out, map[string]any{
			"bucket_start":         time.Unix(s, 0).UTC().Format("2006-01-02T15:04:05Z"),
			"request_count":        agg.total,
			"success_count":        agg.success,
			"failure_count":        agg.failure,
			"stream_request_count": agg.stream,
			"avg_latency_ms":       avgLatency,
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
		"route_concurrency_limit":       settings.GatewayRouteConcurrencyLimit,
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
	_ = a.DB.Save(&state).Error
	writeJSON(w, http.StatusOK, map[string]any{"id": state.ID, "is_enabled": state.IsEnabled, "circuit_state": state.CircuitState})
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
	if strings.TrimSpace(settings.GatewayAPIKey) != "" {
		header := r.Header.Get("Authorization")
		expected := "Bearer " + strings.TrimSpace(settings.GatewayAPIKey)
		if header != expected {
			writeError(w, http.StatusUnauthorized, "网关 API Key 无效")
			return
		}
	}
	targetPath := gatewayProxyTargetPath(r.URL.Path)
	policy := services.GatewayPolicy{
		RouteStrategy:               settings.GatewayRouteStrategy,
		FailureThreshold:            settings.GatewayFailureThreshold,
		CooldownSeconds:             settings.GatewayCooldownSeconds,
		RequestTimeout:              settings.GatewayRequestTimeout,
		MaxAttempts:                 settings.GatewayMaxAttempts,
		RouteConcurrencyLimit:       settings.GatewayRouteConcurrencyLimit,
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
	result, err := services.ProxyGatewayRequestWithOptions(r.Context(), a.DB, r, targetPath, opts, policy)
	if err != nil {
		// service did not write anything yet (no candidate)
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "没有可用") {
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, err.Error())
		return
	}
	if !result.Success && len(result.Body) > 0 && result.Header != nil {
		// retryable failures exhausted: write last upstream body
		if w.Header().Get("Content-Type") == "" {
			for k, vs := range result.Header {
				if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") {
					continue
				}
				for _, v := range vs {
					w.Header().Add(k, v)
				}
			}
			if result.StatusCode > 0 {
				w.WriteHeader(result.StatusCode)
			}
			_, _ = w.Write(result.Body)
		}
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

func gatewayRouteResponse(route services.GatewayRoute) map[string]any {
	state := route.State
	successRate := 0.0
	if state.RequestCount > 0 {
		successRate = round2(float64(state.SuccessCount) / float64(state.RequestCount) * 100)
	}
	return map[string]any{
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
		"balance_display":        balanceDisplay(route.Site.LastBalance),
		"package_display":        packageDisplay(route.Site),
		"checkin_status":         route.Site.LastStatus,
		"key_name":               state.KeyName,
		"key_fingerprint":        state.KeyFingerprint,
		"key_source":             state.KeySource,
		"route_type":             state.RouteType,
		"route_type_manual":      state.RouteTypeManual,
		"route_priority":         state.RoutePriority,
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
