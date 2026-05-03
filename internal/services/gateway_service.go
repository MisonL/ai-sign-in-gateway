package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-sign-in-gateway/internal/models"
	"gorm.io/gorm"
)

// ----------------------------- in-memory counters -----------------------------

var gatewayRoundRobin = struct {
	sync.Mutex
	offsets map[string]int
}{offsets: map[string]int{}}

var gatewayActive = struct {
	sync.Mutex
	counts map[uint]int
}{counts: map[uint]int{}}

var gatewayActiveRequests = struct {
	sync.Mutex
	items map[string]GatewayActiveRequest
}{items: map[string]GatewayActiveRequest{}}

func acquireRoute(stateID uint) int {
	gatewayActive.Lock()
	defer gatewayActive.Unlock()
	gatewayActive.counts[stateID]++
	return gatewayActive.counts[stateID]
}

func releaseRoute(stateID uint) {
	gatewayActive.Lock()
	defer gatewayActive.Unlock()
	if gatewayActive.counts[stateID] <= 1 {
		delete(gatewayActive.counts, stateID)
	} else {
		gatewayActive.counts[stateID]--
	}
}

func RouteActiveCount(stateID uint) int {
	gatewayActive.Lock()
	defer gatewayActive.Unlock()
	return gatewayActive.counts[stateID]
}

func RouteTotalActive() int {
	gatewayActive.Lock()
	defer gatewayActive.Unlock()
	total := 0
	for _, n := range gatewayActive.counts {
		total += n
	}
	return total
}

type GatewayActiveRequest struct {
	ID                string    `json:"id"`
	RequestID         string    `json:"request_id"`
	RouteID           uint      `json:"route_id"`
	SiteID            uint      `json:"site_id"`
	RouteLabel        string    `json:"route_label"`
	SiteName          string    `json:"site_name"`
	KeyName           string    `json:"key_name"`
	KeyFingerprint    string    `json:"key_fingerprint"`
	GroupName         string    `json:"group_name"`
	TargetPath        string    `json:"target_path"`
	Method            string    `json:"method"`
	RouteStrategy     string    `json:"route_strategy"`
	AttemptIndex      int       `json:"attempt_index"`
	IsStream          bool      `json:"is_stream"`
	RouteType         string    `json:"route_type"`
	RequestBaseURL    string    `json:"request_base_url"`
	ActiveConcurrency int       `json:"active_concurrency"`
	StartedAt         time.Time `json:"started_at"`
	ElapsedMS         int64     `json:"elapsed_ms"`
}

func ListGatewayActiveRequests() []GatewayActiveRequest {
	gatewayActiveRequests.Lock()
	out := make([]GatewayActiveRequest, 0, len(gatewayActiveRequests.items))
	for _, item := range gatewayActiveRequests.items {
		out = append(out, item)
	}
	gatewayActiveRequests.Unlock()

	now := time.Now().UTC()
	for idx := range out {
		out[idx].ElapsedMS = now.Sub(out[idx].StartedAt).Milliseconds()
		out[idx].ActiveConcurrency = RouteActiveCount(out[idx].RouteID)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out
}

func beginGatewayActiveRequest(route GatewayRoute, targetPath, method, strategy string, attemptIndex int, isStream bool, requestID string, activeConcurrency int) string {
	if requestID == "" {
		requestID = newRequestID()
	}
	if attemptIndex <= 0 {
		attemptIndex = 1
	}
	token := requestID + ":" + newRequestID()
	siteName := GatewayRouteSiteLabel(route)
	routeLabel := siteName
	if keyName := strings.TrimSpace(route.State.KeyName); keyName != "" {
		routeLabel += " · " + keyName
	}
	siteID := route.State.SiteID
	if route.Site.ID != 0 {
		siteID = route.Site.ID
	}
	gatewayActiveRequests.Lock()
	gatewayActiveRequests.items[token] = GatewayActiveRequest{
		ID:                token,
		RequestID:         requestID,
		RouteID:           route.State.ID,
		SiteID:            siteID,
		RouteLabel:        routeLabel,
		SiteName:          siteName,
		KeyName:           route.State.KeyName,
		KeyFingerprint:    route.State.KeyFingerprint,
		GroupName:         route.State.GroupName,
		TargetPath:        targetPath,
		Method:            method,
		RouteStrategy:     normalizeStrategy(strategy),
		AttemptIndex:      attemptIndex,
		IsStream:          isStream,
		RouteType:         route.State.RouteType,
		RequestBaseURL:    route.RequestBaseURL,
		ActiveConcurrency: activeConcurrency,
		StartedAt:         time.Now().UTC(),
	}
	gatewayActiveRequests.Unlock()
	return token
}

func finishGatewayActiveRequest(token string) {
	if token == "" {
		return
	}
	gatewayActiveRequests.Lock()
	delete(gatewayActiveRequests.items, token)
	gatewayActiveRequests.Unlock()
}

// ResetGatewayCountersForTest clears in-memory gateway counters/offsets.
// Intended for tests; safe to call from any goroutine.
func ResetGatewayCountersForTest() {
	gatewayActive.Lock()
	gatewayActive.counts = map[uint]int{}
	gatewayActive.Unlock()
	gatewayActiveRequests.Lock()
	gatewayActiveRequests.items = map[string]GatewayActiveRequest{}
	gatewayActiveRequests.Unlock()
	gatewayRoundRobin.Lock()
	gatewayRoundRobin.offsets = map[string]int{}
	gatewayRoundRobin.Unlock()
}

// ----------------------------- types -----------------------------

const (
	smartFreshFailureWindow = 15.0
	smartDefaultLatency     = 1000.0
	ewmaAlpha               = 0.3
)

type GatewayPolicy struct {
	RouteStrategy               string
	FailureThreshold            int
	CooldownSeconds             int
	RequestTimeout              int
	MaxAttempts                 int
	FailureRetryMode            string
	RouteConcurrencyLimit       int
	ConcurrencyTransferStrategy string
	ConcurrencyOverflowStrategy string
	SmartLatencyBias            float64
	SmartConcurrencyBias        float64
	SmartFailureBias            float64
	SmartPriorityBias           float64
}

type GatewayRoutePriorityMode string

const (
	GatewayRoutePriorityMove    GatewayRoutePriorityMode = "move"
	GatewayRoutePriorityPackage GatewayRoutePriorityMode = "package"
	GatewayRoutePriorityBalance GatewayRoutePriorityMode = "balance"
)

type GatewayRoutePriorityReorderOptions struct {
	RouteID uint
	Mode    GatewayRoutePriorityMode
	Index   int
}

type GatewayRoute struct {
	State          models.GatewayRouteState
	Site           models.Site
	APIKey         string
	RequestBaseURL string
}

type GatewayProxyResult struct {
	Route      GatewayRoute
	StatusCode int
	Header     http.Header
	Body       []byte
	LatencyMS  float64
	Success    bool
	Error      string
	Attempts   int
	IsStream   bool
}

type GatewayAllRoutesFailedError struct {
	Attempts int
	Last     string
}

func (e GatewayAllRoutesFailedError) Error() string {
	return fmt.Sprintf("网关路由池全部失败，已尝试 %d 个候选", e.Attempts)
}

type GatewayNonRetryableUpstreamError struct {
	Attempts   int
	StatusCode int
}

func (e GatewayNonRetryableUpstreamError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("上游返回不可重试错误，网关已停止切换路由，已尝试 %d 个候选，状态码 %d", e.Attempts, e.StatusCode)
	}
	return fmt.Sprintf("上游返回不可重试错误，网关已停止切换路由，已尝试 %d 个候选", e.Attempts)
}

type GatewayProbeResult struct {
	Route      GatewayRoute
	OK         bool
	StatusCode *int
	LatencyMS  *float64
	Message    string
	Models     []string
	CheckedAt  time.Time
}

type ProxyResponseHook func(statusCode int, header http.Header)

// ProxyGatewayOptions controls request-level proxy behavior.
// When ResponseWriter is set, successful upstream responses are streamed
// directly into it; the returned Body will be empty.
type ProxyGatewayOptions struct {
	ResponseWriter http.ResponseWriter
	BeforeWrite    ProxyResponseHook
	RequestID      string
	Group          string
	RouteType      string
}

// ----------------------------- discovery / sync -----------------------------

func SyncGatewayRoutes(db *gorm.DB) (int, error) {
	if err := cleanupOrphanGatewayRoutes(db); err != nil {
		return 0, err
	}
	var sites []models.Site
	if err := db.Where("is_enabled = ?", true).Order("name asc").Find(&sites).Error; err != nil {
		return 0, err
	}
	count := 0
	for _, site := range sites {
		activeFingerprints := map[string]bool{}
		for _, key := range siteAPIKeys(site) {
			fp := fingerprint(key.Value)
			activeFingerprints[fp] = true
			routeType := firstNonEmpty(key.RouteType, inferRouteType(site))
			var state models.GatewayRouteState
			err := db.Where("site_id = ? AND key_fingerprint = ?", site.ID, fp).First(&state).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				state = models.GatewayRouteState{
					SiteID:              site.ID,
					KeyFingerprint:      fp,
					KeyName:             key.Name,
					KeySource:           key.Source,
					SiteNameSnapshot:    site.Name,
					SiteBaseURLSnapshot: site.BaseURL,
					SiteAPIURLSnapshot:  marshalStringSlice(GatewayRequestBaseCandidates(site)),
					RouteType:           routeType,
					GroupName:           site.GroupName,
					RoutePriority:       intValue(site.PluginConfig, "gateway_priority", 100),
					Weight:              intValue(site.PluginConfig, "gateway_weight", 1),
					IsEnabled:           true,
					CircuitState:        "closed",
				}
				if err := db.Create(&state).Error; err != nil {
					return count, err
				}
			} else if err != nil {
				return count, err
			} else {
				state.KeyName = key.Name
				state.KeySource = key.Source
				state.SiteNameSnapshot = site.Name
				state.SiteBaseURLSnapshot = site.BaseURL
				state.SiteAPIURLSnapshot = marshalStringSlice(GatewayRequestBaseCandidates(site))
				state.GroupName = site.GroupName
				if !state.RoutePriorityManual {
					state.RoutePriority = intValue(site.PluginConfig, "gateway_priority", state.RoutePriority)
				}
				state.Weight = intValue(site.PluginConfig, "gateway_weight", state.Weight)
				if !state.RouteTypeManual {
					state.RouteType = routeType
				}
				if err := db.Save(&state).Error; err != nil {
					return count, err
				}
			}
			count++
		}
		var staleStates []models.GatewayRouteState
		if err := db.Where("site_id = ?", site.ID).Find(&staleStates).Error; err != nil {
			return count, err
		}
		for _, state := range staleStates {
			if activeFingerprints[state.KeyFingerprint] {
				continue
			}
			if err := db.Delete(&state).Error; err != nil {
				return count, err
			}
		}
	}
	return count, nil
}

func cleanupOrphanGatewayRoutes(db *gorm.DB) error {
	var siteIDs []uint
	if err := db.Model(&models.Site{}).Pluck("id", &siteIDs).Error; err != nil {
		return err
	}
	if len(siteIDs) == 0 {
		return db.Where("1 = 1").Delete(&models.GatewayRouteState{}).Error
	}
	return db.Where("site_id NOT IN ?", siteIDs).Delete(&models.GatewayRouteState{}).Error
}

func ListGatewayRoutes(db *gorm.DB, group string, includeDisabled bool) ([]GatewayRoute, error) {
	if _, err := SyncGatewayRoutes(db); err != nil {
		return nil, err
	}
	return listGatewayRoutes(db, group, includeDisabled)
}

func listGatewayRoutes(db *gorm.DB, group string, includeDisabled bool) ([]GatewayRoute, error) {
	query := db.Preload("Site")
	if strings.TrimSpace(group) != "" {
		query = query.Where("group_name = ?", strings.TrimSpace(group))
	}
	if !includeDisabled {
		query = query.Where("is_enabled = ?", true)
	}
	var states []models.GatewayRouteState
	if err := query.Order("route_priority asc, id asc").Find(&states).Error; err != nil {
		return nil, err
	}
	routes := make([]GatewayRoute, 0, len(states))
	for _, state := range states {
		key := apiKeyForFingerprint(state.Site, state.KeyFingerprint)
		routes = append(routes, GatewayRoute{State: state, Site: state.Site, APIKey: key, RequestBaseURL: GatewayRouteRequestBase(state, state.Site)})
	}
	return routes, nil
}

func ReorderGatewayRoutePriorities(db *gorm.DB, opts GatewayRoutePriorityReorderOptions) ([]GatewayRoute, error) {
	if _, err := SyncGatewayRoutes(db); err != nil {
		return nil, err
	}

	var out []GatewayRoute
	err := db.Transaction(func(tx *gorm.DB) error {
		routes, err := listGatewayRoutes(tx, "", true)
		if err != nil {
			return err
		}
		if len(routes) == 0 {
			out = routes
			return nil
		}

		ordered := append([]GatewayRoute{}, routes...)
		switch opts.Mode {
		case GatewayRoutePriorityMove:
			next, err := moveGatewayRoutePriority(ordered, opts.RouteID, opts.Index)
			if err != nil {
				return err
			}
			ordered = next
		case GatewayRoutePriorityPackage:
			sort.SliceStable(ordered, func(i, j int) bool {
				return gatewayRoutePackageRank(ordered[i]) < gatewayRoutePackageRank(ordered[j])
			})
		case GatewayRoutePriorityBalance:
			sort.SliceStable(ordered, func(i, j int) bool {
				left, right := ordered[i].Site.LastBalance, ordered[j].Site.LastBalance
				if (left != nil) != (right != nil) {
					return left != nil
				}
				if left != nil && right != nil && *left != *right {
					return *left > *right
				}
				return false
			})
		default:
			return fmt.Errorf("unsupported priority reorder mode %q", opts.Mode)
		}

		for idx, route := range ordered {
			if err := tx.Model(&models.GatewayRouteState{}).
				Where("id = ?", route.State.ID).
				Updates(map[string]any{
					"route_priority":        idx,
					"route_priority_manual": true,
				}).Error; err != nil {
				return err
			}
		}
		out, err = listGatewayRoutes(tx, "", true)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func moveGatewayRoutePriority(routes []GatewayRoute, routeID uint, index int) ([]GatewayRoute, error) {
	if routeID == 0 {
		return nil, errors.New("route id is required")
	}
	source := -1
	for idx, route := range routes {
		if route.State.ID == routeID {
			source = idx
			break
		}
	}
	if source < 0 {
		return nil, errors.New("gateway route not found")
	}
	target := routes[source]
	ordered := append([]GatewayRoute{}, routes[:source]...)
	ordered = append(ordered, routes[source+1:]...)
	if index < 0 {
		index = 0
	}
	if index > len(ordered) {
		index = len(ordered)
	}
	ordered = append(ordered, GatewayRoute{})
	copy(ordered[index+1:], ordered[index:])
	ordered[index] = target
	return ordered, nil
}

func gatewayRoutePackageRank(route GatewayRoute) int {
	if gatewayRouteHasPriorityPackageGroup(route.State.GroupName) || gatewayRoutePackageLooksSubscribed(GatewayRoutePackageDisplay(route.Site)) {
		return 0
	}
	if strings.TrimSpace(GatewayRoutePackageDisplay(route.Site)) != "" {
		return 1
	}
	return 2
}

func gatewayRouteHasPriorityPackageGroup(value string) bool {
	for _, group := range parseGatewayRouteGroupNames(value) {
		if group == "订阅" || group == "套餐" {
			return true
		}
	}
	return false
}

func parseGatewayRouteGroupNames(value string) []string {
	return normalizeStringList(strings.FieldsFunc(value, func(r rune) bool {
		return strings.ContainsRune(",，;/|、\n\r\t", r)
	}))
}

func GatewayRoutePackageDisplay(site models.Site) string {
	value := strings.TrimSpace(stringMapValue(site.PluginConfig, "package_display", ""))
	if value == "" {
		value = strings.TrimSpace(stringMapValue(site.PluginConfig, "package_name", ""))
	}
	if value == "" {
		value = strings.TrimSpace(stringMapValue(site.PluginConfig, "plan_name", ""))
	}
	remaining, hasRemaining := numericMapValue(site.PluginConfig, "package_remaining")
	total, hasTotal := numericMapValue(site.PluginConfig, "package_total")
	used, hasUsed := numericMapValue(site.PluginConfig, "package_used")
	unit := strings.TrimSpace(stringMapValue(site.PluginConfig, "package_unit", ""))
	quota := ""
	if hasRemaining && hasTotal {
		quota = fmt.Sprintf("余量 %s / %s", formatCompactNumber(remaining), formatCompactNumber(total))
	} else if hasRemaining {
		quota = fmt.Sprintf("余量 %s", formatCompactNumber(remaining))
	} else if hasUsed && hasTotal {
		quota = fmt.Sprintf("已用 %s / %s", formatCompactNumber(used), formatCompactNumber(total))
	}
	if quota != "" && unit != "" {
		quota += " " + unit
	}
	if value != "" && quota != "" && !strings.Contains(value, quota) {
		return value + " · " + quota
	}
	if value == "" {
		return quota
	}
	return value
}

func numericMapValue(m models.JSONMap, key string) (float64, bool) {
	if m == nil || m[key] == nil {
		return 0, false
	}
	switch typed := m[key].(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		value, err := typed.Float64()
		return value, err == nil
	case string:
		value, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return value, err == nil
	default:
		return 0, false
	}
}

func formatCompactNumber(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".")
}

func gatewayRoutePackageLooksSubscribed(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, "订阅") ||
		strings.Contains(normalized, "subscription") ||
		strings.Contains(normalized, "subscribe")
}

func GetGatewayRoute(db *gorm.DB, routeID string) (GatewayRoute, error) {
	var state models.GatewayRouteState
	if err := db.Preload("Site").First(&state, routeID).Error; err != nil {
		return GatewayRoute{}, err
	}
	return GatewayRoute{
		State:          state,
		Site:           state.Site,
		APIKey:         apiKeyForFingerprint(state.Site, state.KeyFingerprint),
		RequestBaseURL: GatewayRouteRequestBase(state, state.Site),
	}, nil
}

// ----------------------------- candidate ordering -----------------------------

func filterAndOrderCandidates(routes []GatewayRoute, group, routeType string, policy GatewayPolicy) []GatewayRoute {
	now := time.Now().UTC()
	rt := normalizeRouteType(routeType)
	candidates := make([]GatewayRoute, 0, len(routes))
	for _, route := range routes {
		if route.APIKey == "" || !route.Site.IsEnabled || !route.State.IsEnabled {
			continue
		}
		if rt != "" && route.State.RouteType != rt {
			continue
		}
		// refresh half-open after cooldown
		if route.State.CircuitState == "open" {
			if route.State.CircuitOpenUntil != nil && route.State.CircuitOpenUntil.Before(now) {
				route.State.CircuitState = "half_open"
				route.State.CircuitOpenUntil = nil
			} else {
				continue
			}
		}
		candidates = append(candidates, route)
	}
	if len(candidates) == 0 {
		return candidates
	}
	closed := make([]GatewayRoute, 0)
	halfOpenAll := make([]GatewayRoute, 0)
	for _, c := range candidates {
		if c.State.CircuitState == "half_open" {
			halfOpenAll = append(halfOpenAll, c)
		} else {
			closed = append(closed, c)
		}
	}
	withinClosed, overflowClosed := splitByConcurrency(closed, policy)
	halfOpen, overflowHalf := splitByConcurrency(halfOpenAll, policy)

	bucket := normalizeStrategy(policy.RouteStrategy) + ":" + strings.ToLower(strings.TrimSpace(group))

	var ordered []GatewayRoute
	switch normalizeStrategy(policy.RouteStrategy) {
	case "smart":
		ref := referenceLatency(append(append([]GatewayRoute{}, withinClosed...), halfOpen...))
		ordered = append(sortBySmart(withinClosed, ref, policy, now), sortBySmart(halfOpen, ref, policy, now)...)
	case "latency_first":
		ordered = append(sortByLatency(withinClosed), sortByLoadAndPriority(halfOpen)...)
	case "priority":
		merged := append([]GatewayRoute{}, withinClosed...)
		merged = append(merged, halfOpen...)
		sort.SliceStable(merged, func(i, j int) bool {
			ai, bi := candidateHealthRank(merged[i]), candidateHealthRank(merged[j])
			if ai != bi {
				return ai < bi
			}
			al, bl := candidateLoadRank(merged[i]), candidateLoadRank(merged[j])
			if al != bl {
				return al < bl
			}
			if merged[i].State.RoutePriority != merged[j].State.RoutePriority {
				return merged[i].State.RoutePriority < merged[j].State.RoutePriority
			}
			if merged[i].State.Weight != merged[j].State.Weight {
				return merged[i].State.Weight > merged[j].State.Weight
			}
			return merged[i].Site.Name < merged[j].Site.Name
		})
		ordered = merged
	default: // round_robin
		base := append([]GatewayRoute{}, withinClosed...)
		sort.SliceStable(base, func(i, j int) bool {
			if base[i].State.RoutePriority != base[j].State.RoutePriority {
				return base[i].State.RoutePriority < base[j].State.RoutePriority
			}
			if base[i].State.ConsecutiveFailures != base[j].State.ConsecutiveFailures {
				return base[i].State.ConsecutiveFailures < base[j].State.ConsecutiveFailures
			}
			return base[i].Site.Name < base[j].Site.Name
		})
		var leastBusy []GatewayRoute
		var busier []GatewayRoute
		if len(base) > 0 {
			lowest := RouteActiveCount(base[0].State.ID)
			for _, r := range base[1:] {
				if c := RouteActiveCount(r.State.ID); c < lowest {
					lowest = c
				}
			}
			for _, r := range base {
				if RouteActiveCount(r.State.ID) == lowest {
					leastBusy = append(leastBusy, r)
				} else {
					busier = append(busier, r)
				}
			}
		}
		ordered = append(rotateUnique(leastBusy, bucket, true), sortByLoadAndPriority(busier)...)
		ordered = append(ordered, sortByLoadAndPriority(halfOpen)...)
	}

	if policy.ConcurrencyTransferStrategy == "balance" {
		ordered = sortByLoadThenExistingOrder(ordered)
	}

	if len(overflowClosed) > 0 {
		ordered = append(ordered, sortConcurrencyOverflow(overflowClosed, policy)...)
	}
	if len(overflowHalf) > 0 {
		ordered = append(ordered, sortByLoadAndPriority(overflowHalf)...)
	}
	return ordered
}

func splitByConcurrency(in []GatewayRoute, policy GatewayPolicy) (within, overflow []GatewayRoute) {
	for _, r := range in {
		if candidateBelowConcurrencyLimit(r, policy) {
			within = append(within, r)
		} else {
			overflow = append(overflow, r)
		}
	}
	return
}

func candidateBelowConcurrencyLimit(r GatewayRoute, policy GatewayPolicy) bool {
	active := RouteActiveCount(r.State.ID)
	if r.State.CircuitState == "half_open" {
		return active < 1
	}
	if policy.RouteConcurrencyLimit <= 0 {
		return true
	}
	return active < policy.RouteConcurrencyLimit
}

func sortByLatency(in []GatewayRoute) []GatewayRoute {
	out := append([]GatewayRoute{}, in...)
	sort.SliceStable(out, func(i, j int) bool {
		ai := candidateEffectiveLatency(out[i])
		aj := candidateEffectiveLatency(out[j])
		if ai != aj {
			return ai < aj
		}
		return candidateLoadRank(out[i]) < candidateLoadRank(out[j])
	})
	return out
}

func sortByLoadAndPriority(in []GatewayRoute) []GatewayRoute {
	out := append([]GatewayRoute{}, in...)
	sort.SliceStable(out, func(i, j int) bool {
		ai, aj := candidateLoadRank(out[i]), candidateLoadRank(out[j])
		if ai != aj {
			return ai < aj
		}
		if out[i].State.RoutePriority != out[j].State.RoutePriority {
			return out[i].State.RoutePriority < out[j].State.RoutePriority
		}
		return out[i].Site.Name < out[j].Site.Name
	})
	return out
}

func sortBySmart(in []GatewayRoute, ref float64, policy GatewayPolicy, now time.Time) []GatewayRoute {
	out := append([]GatewayRoute{}, in...)
	sort.SliceStable(out, func(i, j int) bool {
		si := smartScore(out[i], ref, policy, now)
		sj := smartScore(out[j], ref, policy, now)
		if si != sj {
			return si < sj
		}
		return out[i].Site.Name < out[j].Site.Name
	})
	return out
}

func sortByLoadThenExistingOrder(in []GatewayRoute) []GatewayRoute {
	out := append([]GatewayRoute{}, in...)
	sort.SliceStable(out, func(i, j int) bool {
		return RouteActiveCount(out[i].State.ID) < RouteActiveCount(out[j].State.ID)
	})
	return out
}

func sortConcurrencyOverflow(in []GatewayRoute, policy GatewayPolicy) []GatewayRoute {
	out := append([]GatewayRoute{}, in...)
	if strings.ToLower(strings.TrimSpace(policy.ConcurrencyOverflowStrategy)) == "sequential" {
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].State.RoutePriority != out[j].State.RoutePriority {
				return out[i].State.RoutePriority < out[j].State.RoutePriority
			}
			return candidateLoadRank(out[i]) < candidateLoadRank(out[j])
		})
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		ai := candidateEffectiveLatency(out[i])
		aj := candidateEffectiveLatency(out[j])
		if ai != aj {
			return ai < aj
		}
		return candidateLoadRank(out[i]) < candidateLoadRank(out[j])
	})
	return out
}

func candidateHealthRank(r GatewayRoute) int {
	if r.State.CircuitState == "half_open" {
		return 1
	}
	return 0
}

func candidateLoadRank(r GatewayRoute) int {
	return RouteActiveCount(r.State.ID)*1000 - r.State.Weight
}

func candidateEffectiveLatency(r GatewayRoute) float64 {
	if r.State.EWMALatencyMS != nil {
		return *r.State.EWMALatencyMS
	}
	if r.State.AvgLatencyMS != nil {
		return *r.State.AvgLatencyMS
	}
	return float64(1 << 30)
}

func referenceLatency(routes []GatewayRoute) float64 {
	samples := make([]float64, 0, len(routes))
	for _, r := range routes {
		if r.State.EWMALatencyMS != nil && *r.State.EWMALatencyMS > 0 {
			samples = append(samples, *r.State.EWMALatencyMS)
		} else if r.State.AvgLatencyMS != nil && *r.State.AvgLatencyMS > 0 {
			samples = append(samples, *r.State.AvgLatencyMS)
		}
	}
	if len(samples) == 0 {
		return smartDefaultLatency
	}
	sort.Float64s(samples)
	return samples[len(samples)/2]
}

func smartScore(r GatewayRoute, ref float64, policy GatewayPolicy, now time.Time) float64 {
	state := r.State
	health := 0.0
	if state.CircuitState == "half_open" {
		health = 0.5
	}
	active := RouteActiveCount(state.ID)
	concurrencyFactor := 0.0
	if policy.RouteConcurrencyLimit > 0 {
		concurrencyFactor = float64(active) / float64(policy.RouteConcurrencyLimit)
	} else {
		concurrencyFactor = float64(active) * 0.05
	}
	latencyFactor := 0.5
	if l := candidateEffectiveLatency(r); l < float64(1<<29) {
		ref = max(ref, 1.0)
		latencyFactor = l / ref
	}
	consecutiveFactor := float64(state.ConsecutiveFailures) * 0.1
	freshFailure := 0.0
	if state.LastFailureAt != nil {
		elapsed := now.Sub(*state.LastFailureAt).Seconds()
		if elapsed >= 0 && elapsed < smartFreshFailureWindow {
			freshFailure = (smartFreshFailureWindow - elapsed) / smartFreshFailureWindow
		}
	}
	weightFactor := 1.0 / float64(maxInt(state.Weight, 1))
	priorityFactor := float64(state.RoutePriority) / 1000.0
	failureRate := 0.0
	if state.RequestCount >= 5 {
		failureRate = float64(state.FailureCount) / float64(maxInt(state.RequestCount, 1))
	}
	return health +
		concurrencyFactor*orDefault(policy.SmartConcurrencyBias, 1.5) +
		latencyFactor*orDefault(policy.SmartLatencyBias, 1.0) +
		consecutiveFactor*orDefault(policy.SmartFailureBias, 1.0)*0.5 +
		freshFailure*orDefault(policy.SmartFailureBias, 1.0) +
		failureRate*orDefault(policy.SmartFailureBias, 1.0) +
		weightFactor*orDefault(policy.SmartPriorityBias, 0.5) +
		priorityFactor*orDefault(policy.SmartPriorityBias, 0.5)
}

func rotateUnique(in []GatewayRoute, bucket string, weighted bool) []GatewayRoute {
	if len(in) == 0 {
		return in
	}
	if !weighted {
		gatewayRoundRobin.Lock()
		start := gatewayRoundRobin.offsets[bucket] % len(in)
		gatewayRoundRobin.offsets[bucket] = start + 1
		gatewayRoundRobin.Unlock()
		out := append([]GatewayRoute{}, in[start:]...)
		out = append(out, in[:start]...)
		return out
	}
	expanded := make([]int, 0, len(in))
	for idx, r := range in {
		w := r.State.Weight
		if w <= 0 {
			w = 1
		}
		for i := 0; i < w; i++ {
			expanded = append(expanded, idx)
		}
	}
	gatewayRoundRobin.Lock()
	start := gatewayRoundRobin.offsets[bucket] % len(expanded)
	gatewayRoundRobin.offsets[bucket] = start + 1
	gatewayRoundRobin.Unlock()
	primary := expanded[start]
	out := []GatewayRoute{in[primary]}
	for idx, r := range in {
		if idx == primary {
			continue
		}
		out = append(out, r)
	}
	return out
}

// SelectGatewayRoute is preserved for back-compat (ignores excluded set + concurrency).
func SelectGatewayRoute(db *gorm.DB, group, routeType string, policy GatewayPolicy, excluded map[uint]bool) (GatewayRoute, error) {
	routes, err := ListGatewayRoutes(db, group, false)
	if err != nil {
		return GatewayRoute{}, err
	}
	policy = normalizePolicy(policy)
	ordered := filterAndOrderCandidates(routes, group, routeType, policy)
	for _, r := range ordered {
		if excluded == nil || !excluded[r.State.ID] {
			return r, nil
		}
	}
	return GatewayRoute{}, errors.New("没有可用的网关路由")
}

// ----------------------------- proxy core -----------------------------

func ProxyGatewayRequest(ctx context.Context, db *gorm.DB, r *http.Request, targetPath, group, routeType string, policy GatewayPolicy) (GatewayProxyResult, error) {
	return ProxyGatewayRequestWithOptions(ctx, db, r, targetPath, ProxyGatewayOptions{Group: group, RouteType: routeType}, policy)
}

func ProxyGatewayRequestWithOptions(ctx context.Context, db *gorm.DB, r *http.Request, targetPath string, opts ProxyGatewayOptions, policy GatewayPolicy) (GatewayProxyResult, error) {
	policy = normalizePolicy(policy)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return GatewayProxyResult{}, err
	}
	if strings.TrimSpace(opts.RouteType) == "" {
		opts.RouteType = InferGatewayRouteTypeFromRequestBody(body)
	}
	streaming := isStreamingRequest(r, body)
	if opts.RequestID == "" {
		opts.RequestID = newRequestID()
	}

	routes, err := ListGatewayRoutes(db, opts.Group, false)
	if err != nil {
		return GatewayProxyResult{}, err
	}
	ordered := filterAndOrderCandidates(routes, opts.Group, opts.RouteType, policy)
	if len(ordered) == 0 {
		return GatewayProxyResult{}, errors.New("没有可用的网关路由")
	}

	var lastResult GatewayProxyResult
	var lastErr error
	attemptCount := 0
	for index, route := range ordered {
		for candidateIndex, baseURL := range gatewayRouteBasesInOrder(route) {
			attempt := index + 1
			if candidateIndex > 0 {
				attempt = index + 1
			}
			attemptCount++
			candidateRoute := route
			candidateRoute.RequestBaseURL = baseURL
			result, shouldFallback, err := proxyGatewayAttempt(ctx, db, r, body, candidateRoute, targetPath, opts, policy, streaming, attempt)
			result.Attempts = attempt
			result.IsStream = streaming
			LogGatewayRequest(db, candidateRoute, targetPath, r.Method, statusCodePtrOrNil(result.StatusCode), result.Success, result.LatencyMS, result.Error, policy.RouteStrategy, attempt, streaming, opts.RequestID)
			lastResult = result
			lastErr = err
			if err == nil && result.Success {
				return result, nil
			}
			if !shouldFallback {
				lastResult.Success = false
				lastResult.Body = nil
				lastResult.Header = nil
				return lastResult, GatewayNonRetryableUpstreamError{Attempts: attemptCount, StatusCode: lastResult.StatusCode}
			}
		}
	}
	lastMessage := strings.TrimSpace(lastResult.Error)
	if lastMessage == "" && lastErr != nil {
		lastMessage = lastErr.Error()
	}
	if lastMessage == "" && lastResult.StatusCode > 0 {
		lastMessage = fmt.Sprintf("status=%d", lastResult.StatusCode)
	}
	lastResult.Success = false
	lastResult.Body = nil
	lastResult.Header = nil
	lastResult.Error = lastMessage
	return lastResult, GatewayAllRoutesFailedError{Attempts: attemptCount, Last: lastMessage}
}

func proxyGatewayAttempt(ctx context.Context, db *gorm.DB, r *http.Request, body []byte, route GatewayRoute, targetPath string, opts ProxyGatewayOptions, policy GatewayPolicy, streaming bool, attempt int) (GatewayProxyResult, bool, error) {
	upstreamURL, err := targetURL(route.RequestBaseURL, targetPath, r.URL.RawQuery, route.State.RouteType)
	if err != nil {
		return GatewayProxyResult{Route: route, Error: err.Error()}, true, err
	}

	circuitBefore := route.State.CircuitState
	_ = circuitBefore // captured by caller through LogGatewayRequest via fresh route.State
	activeConcurrency := acquireRoute(route.State.ID)
	activeToken := beginGatewayActiveRequest(route, targetPath, r.Method, policy.RouteStrategy, attempt, streaming, opts.RequestID, activeConcurrency)
	defer func() {
		finishGatewayActiveRequest(activeToken)
		releaseRoute(route.State.ID)
	}()

	timeout := time.Duration(policy.RequestTimeout) * time.Second
	reqCtx := ctx
	var cancel context.CancelFunc
	if !streaming {
		reqCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(reqCtx, r.Method, upstreamURL, bodyReader)
	if err != nil {
		return GatewayProxyResult{Route: route, Error: err.Error()}, true, err
	}
	copyGatewayHeaders(req.Header, r.Header)
	req.Header.Set("Authorization", "Bearer "+route.APIKey)

	client := &http.Client{}
	if !streaming {
		client.Timeout = timeout
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		UpdateRouteFailure(db, &route.State, err.Error(), latency, nil, policy)
		return GatewayProxyResult{Route: route, LatencyMS: latency, Success: false, Error: err.Error()}, true, nil
	}

	statusCode := resp.StatusCode
	is2xx := statusCode >= 200 && statusCode < 300

	if is2xx && streaming && opts.ResponseWriter != nil {
		if opts.BeforeWrite != nil {
			opts.BeforeWrite(statusCode, resp.Header.Clone())
		}
		writeStreamHeaders(opts.ResponseWriter, resp.Header, statusCode)
		written, copyErr := streamBody(opts.ResponseWriter, resp.Body)
		_ = resp.Body.Close()
		end := time.Now()
		actualLatency := float64(end.Sub(start).Microseconds()) / 1000.0
		if copyErr != nil && written == 0 {
			// nothing flushed yet — count as failure, allow retry
			UpdateRouteFailure(db, &route.State, copyErr.Error(), actualLatency, &statusCode, policy)
			return GatewayProxyResult{Route: route, StatusCode: statusCode, Header: resp.Header.Clone(), LatencyMS: actualLatency, Success: false, Error: copyErr.Error()}, true, nil
		}
		// once data is on the wire, treat as success even if upstream cuts off mid-stream
		route.State.LastRequestBaseURL = route.RequestBaseURL
		UpdateRouteSuccess(db, &route.State, statusCode, latency)
		return GatewayProxyResult{Route: route, StatusCode: statusCode, Header: resp.Header.Clone(), LatencyMS: actualLatency, Success: true}, false, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if is2xx {
		route.State.LastRequestBaseURL = route.RequestBaseURL
		UpdateRouteSuccess(db, &route.State, statusCode, latency)
		if opts.ResponseWriter != nil {
			if opts.BeforeWrite != nil {
				opts.BeforeWrite(statusCode, resp.Header.Clone())
			}
			writeStreamHeaders(opts.ResponseWriter, resp.Header, statusCode)
			_, _ = opts.ResponseWriter.Write(respBody)
		}
		return GatewayProxyResult{Route: route, StatusCode: statusCode, Header: resp.Header.Clone(), Body: respBody, LatencyMS: latency, Success: true}, false, nil
	}

	reason := strings.TrimSpace(string(respBody))
	if reason == "" {
		reason = fmt.Sprintf("status=%d", statusCode)
	}
	UpdateRouteFailure(db, &route.State, reason, latency, &statusCode, policy)
	return GatewayProxyResult{Route: route, StatusCode: statusCode, Header: resp.Header.Clone(), Body: respBody, LatencyMS: latency, Success: false, Error: reason}, shouldFallbackStatus(statusCode, policy.FailureRetryMode), nil
}

func writeStreamHeaders(w http.ResponseWriter, src http.Header, status int) {
	dst := w.Header()
	for k, vs := range src {
		lk := strings.ToLower(k)
		if lk == "content-length" || lk == "transfer-encoding" || lk == "connection" {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
	w.WriteHeader(status)
}

func streamBody(w http.ResponseWriter, src io.Reader) (int, error) {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	written := 0
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return written, werr
			}
			written += n
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return written, nil
			}
			return written, err
		}
	}
}

func isStreamingRequest(r *http.Request, body []byte) bool {
	if accept := strings.ToLower(r.Header.Get("Accept")); strings.Contains(accept, "text/event-stream") {
		return true
	}
	if len(body) == 0 {
		return false
	}
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "json") {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	stream, _ := payload["stream"].(bool)
	return stream
}

// ----------------------------- route state updates -----------------------------

func UpdateRouteSuccess(db *gorm.DB, state *models.GatewayRouteState, statusCode int, latency float64) {
	now := time.Now().UTC()
	previous := state.RequestCount
	state.RequestCount++
	state.SuccessCount++
	state.ConsecutiveFailures = 0
	state.LastStatusCode = &statusCode
	l := latency
	state.LastLatencyMS = &l
	state.LastError = nil
	state.LastUsedAt = &now
	state.LastSuccessAt = &now
	state.CircuitState = "closed"
	state.CircuitOpenedAt = nil
	state.CircuitOpenUntil = nil
	state.AvgLatencyMS = updateAvgLatency(state.AvgLatencyMS, previous, latency)
	state.EWMALatencyMS = updateEWMA(state.EWMALatencyMS, latency)
	_ = db.Save(state).Error
}

func UpdateRouteFailure(db *gorm.DB, state *models.GatewayRouteState, message string, latency float64, statusCode *int, policy GatewayPolicy) {
	policy = normalizePolicy(policy)
	now := time.Now().UTC()
	state.RequestCount++
	state.FailureCount++
	state.ConsecutiveFailures++
	state.LastStatusCode = statusCode
	l := latency
	state.LastLatencyMS = &l
	reason := shorten(message, 500)
	state.LastError = &reason
	state.LastUsedAt = &now
	state.LastFailureAt = &now
	if state.ConsecutiveFailures >= policy.FailureThreshold {
		until := now.Add(time.Duration(policy.CooldownSeconds) * time.Second)
		state.CircuitState = "open"
		state.CircuitOpenedAt = &now
		state.CircuitOpenUntil = &until
	} else if state.CircuitState == "half_open" {
		until := now.Add(time.Duration(policy.CooldownSeconds) * time.Second)
		state.CircuitState = "open"
		state.CircuitOpenedAt = &now
		state.CircuitOpenUntil = &until
	}
	_ = db.Save(state).Error
}

func updateAvgLatency(current *float64, previousCount int, sample float64) *float64 {
	if current == nil || previousCount <= 0 {
		v := sample
		return &v
	}
	avg := ((*current)*float64(previousCount) + sample) / float64(previousCount+1)
	avg = roundTo(avg, 2)
	return &avg
}

func updateEWMA(current *float64, sample float64) *float64 {
	if sample <= 0 {
		return current
	}
	if current == nil {
		v := roundTo(sample, 2)
		return &v
	}
	v := roundTo(ewmaAlpha*sample+(1-ewmaAlpha)*(*current), 2)
	return &v
}

func roundTo(value float64, digits int) float64 {
	pow := 1.0
	for i := 0; i < digits; i++ {
		pow *= 10
	}
	return float64(int64(value*pow+0.5)) / pow
}

// ----------------------------- logging -----------------------------

func LogGatewayRequest(db *gorm.DB, route GatewayRoute, targetPath, method string, statusCode *int, success bool, latency float64, reason string, strategy string, attemptIndex int, isStream bool, requestID string) {
	siteID := route.State.SiteID
	if route.Site.ID != 0 {
		siteID = route.Site.ID
	}
	routeStateID := route.State.ID
	if attemptIndex <= 0 {
		attemptIndex = 1
	}
	if requestID == "" {
		requestID = newRequestID()
	}
	log := models.GatewayRequestLog{
		RequestID:          requestID,
		RouteStateID:       &routeStateID,
		SiteID:             &siteID,
		KeyFingerprint:     route.State.KeyFingerprint,
		KeyName:            route.State.KeyName,
		GroupName:          route.State.GroupName,
		TargetPath:         targetPath,
		Method:             method,
		RouteStrategy:      normalizeStrategy(strategy),
		AttemptIndex:       attemptIndex,
		StatusCode:         statusCode,
		Success:            success,
		LatencyMS:          &latency,
		CircuitStateBefore: route.State.CircuitState,
		IsStream:           isStream,
	}
	if reason != "" {
		log.FailureReason = stringPtr(reason)
	}
	_ = db.Create(&log).Error
}

// ----------------------------- probe -----------------------------

func ProbeGatewayRoute(ctx context.Context, db *gorm.DB, routeID string, timeoutSeconds int) (GatewayProbeResult, error) {
	route, err := GetGatewayRoute(db, routeID)
	if err != nil {
		return GatewayProbeResult{}, err
	}
	if route.APIKey == "" {
		message := fmt.Sprintf(
			"路由缺少 API Key：%s，站点 ID %d，Key 指纹 %s",
			GatewayRouteSiteLabel(route),
			route.State.SiteID,
			route.State.KeyFingerprint,
		)
		return GatewayProbeResult{Route: route, OK: false, Message: message, CheckedAt: time.Now().UTC()}, nil
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 20
	}
	var last GatewayProbeResult
	for _, baseURL := range gatewayRouteBasesInOrder(route) {
		candidateRoute := route
		candidateRoute.RequestBaseURL = baseURL
		upstreamURL, err := targetURL(candidateRoute.RequestBaseURL, "models", "", candidateRoute.State.RouteType)
		if err != nil {
			return GatewayProbeResult{}, err
		}
		reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, upstreamURL, nil)
		if err != nil {
			cancel()
			return GatewayProbeResult{}, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+candidateRoute.APIKey)
		start := time.Now()
		resp, err := (&http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}).Do(req)
		latency := float64(time.Since(start).Microseconds()) / 1000.0
		checkedAt := time.Now().UTC()
		cancel()
		if err != nil {
			UpdateRouteFailure(db, &candidateRoute.State, err.Error(), latency, nil, GatewayPolicy{RequestTimeout: timeoutSeconds})
			last = GatewayProbeResult{Route: candidateRoute, OK: false, LatencyMS: &latency, Message: err.Error(), CheckedAt: checkedAt}
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		statusCode := resp.StatusCode
		ok := statusCode >= 200 && statusCode < 300
		models := extractModelIDs(body)
		message := "探测成功。"
		if !ok {
			message = fmt.Sprintf("接口返回 %d", statusCode)
			UpdateRouteFailure(db, &candidateRoute.State, string(body), latency, &statusCode, GatewayPolicy{RequestTimeout: timeoutSeconds})
		} else {
			candidateRoute.State.LastRequestBaseURL = candidateRoute.RequestBaseURL
			UpdateRouteSuccess(db, &candidateRoute.State, statusCode, latency)
		}
		last = GatewayProbeResult{Route: candidateRoute, OK: ok, StatusCode: &statusCode, LatencyMS: &latency, Message: message, Models: models, CheckedAt: checkedAt}
		if ok {
			return last, nil
		}
	}
	return last, nil
}

// ----------------------------- helpers -----------------------------

func GatewayRequestBase(site models.Site) string {
	candidates := GatewayRequestBaseCandidates(site)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func GatewayRequestBaseCandidates(site models.Site) []string {
	raw := []string{}
	raw = append(raw, stringListMapValue(site.PluginConfig, "api_request_urls")...)
	raw = append(raw, stringListMapValue(site.PluginConfig, "gateway_request_urls")...)
	raw = append(raw, stringMapValue(site.PluginConfig, "gateway_request_url", ""))
	raw = append(raw, stringMapValue(site.PluginConfig, "endpoint_url", ""))
	raw = append(raw, site.BaseURL)
	out := []string{}
	for _, target := range raw {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		joined := target
		if value, err := JoinURL(site.BaseURL, target); err == nil && value != "" {
			joined = value
		}
		joined = NormalizeBaseURL(joined)
		if joined == "" || containsString(out, joined) {
			continue
		}
		out = append(out, joined)
	}
	return out
}

func GatewayRouteRequestBase(state models.GatewayRouteState, site models.Site) string {
	if strings.TrimSpace(state.LastRequestBaseURL) != "" {
		return NormalizeBaseURL(state.LastRequestBaseURL)
	}
	candidates := GatewayRouteRequestBaseCandidates(state, site)
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

func GatewayRouteRequestBaseCandidates(state models.GatewayRouteState, site models.Site) []string {
	if site.ID != 0 {
		return GatewayRequestBaseCandidates(site)
	}
	var snapshot []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(state.SiteAPIURLSnapshot)), &snapshot); err == nil {
		snapshot = normalizeStringList(snapshot)
		if len(snapshot) > 0 {
			return snapshot
		}
	}
	base := NormalizeBaseURL(state.SiteBaseURLSnapshot)
	if base == "" {
		return nil
	}
	return []string{base}
}

func GatewayRouteSiteLabel(route GatewayRoute) string {
	name := strings.TrimSpace(route.Site.Name)
	if name == "" {
		name = strings.TrimSpace(route.State.SiteNameSnapshot)
	}
	if name == "" {
		name = strings.TrimSpace(route.Site.BaseURL)
	}
	if name == "" {
		name = strings.TrimSpace(route.State.SiteBaseURLSnapshot)
	}
	if name == "" {
		name = fmt.Sprintf("站点 #%d", route.State.SiteID)
	}
	return name
}

func gatewayRouteBasesInOrder(route GatewayRoute) []string {
	candidates := GatewayRouteRequestBaseCandidates(route.State, route.Site)
	if len(candidates) == 0 && strings.TrimSpace(route.RequestBaseURL) != "" {
		candidates = []string{route.RequestBaseURL}
	}
	preferred := NormalizeBaseURL(route.RequestBaseURL)
	if preferred == "" {
		return candidates
	}
	out := []string{preferred}
	for _, candidate := range candidates {
		candidate = NormalizeBaseURL(candidate)
		if candidate == "" || containsString(out, candidate) {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

type siteKey struct {
	Value     string
	Name      string
	Source    string
	RouteType string
}

func siteAPIKeys(site models.Site) []siteKey {
	rawKeys, ok := site.Credentials["api_keys"].([]any)
	keys := []siteKey{}
	seen := map[string]bool{}
	if ok {
		for _, item := range rawKeys {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			value := strings.TrimSpace(fmt.Sprint(obj["key"]))
			status := strings.ToLower(strings.TrimSpace(fmt.Sprint(obj["status"])))
			if value == "" || seen[value] || status == "disabled" || status == "inactive" || status == "revoked" {
				continue
			}
			seen[value] = true
			keys = append(keys, siteKey{
				Value:     value,
				Name:      strings.TrimSpace(fmt.Sprint(obj["name"])),
				Source:    "site.credentials.api_keys",
				RouteType: routeTypeFromAny(obj["route_type"], obj["api_type"], obj["api_format"], obj["type"]),
			})
		}
	}
	if len(keys) == 0 {
		value := strings.TrimSpace(stringMapValue(site.Credentials, "api_key", ""))
		if value != "" {
			keys = append(keys, siteKey{Value: value, Source: "site.credentials.api_key", RouteType: inferRouteType(site)})
		}
	}
	return keys
}

func stringListMapValue(m models.JSONMap, key string) []string {
	if m == nil || m[key] == nil {
		return nil
	}
	switch typed := m[key].(type) {
	case []string:
		return normalizeStringList(typed)
	case []any:
		out := []string{}
		for _, item := range typed {
			out = append(out, fmt.Sprint(item))
		}
		return normalizeStringList(out)
	case string:
		return normalizeStringList(strings.FieldsFunc(typed, func(r rune) bool {
			return strings.ContainsRune(",，\n\r\t", r)
		}))
	default:
		return normalizeStringList([]string{fmt.Sprint(typed)})
	}
}

func normalizeStringList(values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || containsString(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func marshalStringSlice(values []string) string {
	data, err := json.Marshal(normalizeStringList(values))
	if err != nil {
		return "[]"
	}
	return string(data)
}

func apiKeyForFingerprint(site models.Site, fp string) string {
	for _, key := range siteAPIKeys(site) {
		if fingerprint(key.Value) == fp {
			return key.Value
		}
	}
	return ""
}

func GatewayRouteAPIKeyForState(state models.GatewayRouteState) string {
	return apiKeyForFingerprint(state.Site, state.KeyFingerprint)
}

func fingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func inferRouteType(site models.Site) string {
	routeType := normalizeRouteType(stringMapValue(site.PluginConfig, "gateway_route_type", ""))
	if routeType != "" {
		return routeType
	}
	if routeType := routeTypeFromAny(stringMapValue(site.PluginConfig, "api_format", "")); routeType != "" {
		return routeType
	}
	return "codex"
}

func normalizeRouteType(value string) string {
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

func routeTypeFromAny(values ...any) string {
	for _, value := range values {
		candidate := normalizeRouteType(fmt.Sprint(value))
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func InferGatewayRouteTypeFromRequestBody(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	model := strings.ToLower(strings.TrimSpace(fmt.Sprint(payload["model"])))
	if model == "" || model == "<nil>" {
		return ""
	}
	switch {
	case strings.Contains(model, "claude") || strings.Contains(model, "anthropic"):
		return "claude"
	case strings.Contains(model, "gemini"):
		return "gemini"
	case strings.Contains(model, "gpt") ||
		strings.Contains(model, "openai") ||
		strings.Contains(model, "codex") ||
		strings.HasPrefix(model, "o1") ||
		strings.HasPrefix(model, "o3") ||
		strings.HasPrefix(model, "o4"):
		return "codex"
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func targetURL(baseURL, targetPath, rawQuery, routeType string) (string, error) {
	base := NormalizeBaseURL(baseURL)
	path := strings.TrimLeft(targetPath, "/")
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if parsed.Path == "" || parsed.Path == "/" {
		if strings.HasPrefix(path, "v1/") || path == "v1" || strings.HasPrefix(path, "v1beta/") || path == "v1beta" {
			// keep path
		} else if normalizeRouteType(routeType) == "gemini" {
			path = "v1beta/openai/" + path
		} else {
			path = "v1/" + path
		}
	}
	joined, err := JoinURL(base, path)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(joined)
	if err != nil {
		return "", err
	}
	if rawQuery != "" {
		values, _ := url.ParseQuery(rawQuery)
		values.Del("group")
		values.Del("type")
		values.Del("route_type")
		u.RawQuery = values.Encode()
	}
	return u.String(), nil
}

func GatewayTargetURL(baseURL, targetPath, rawQuery, routeType string) (string, error) {
	return targetURL(baseURL, targetPath, rawQuery, routeType)
}

func copyGatewayHeaders(dst, src http.Header) {
	allowed := map[string]bool{"content-type": true, "accept": true, "openai-organization": true, "openai-project": true}
	for key, values := range src {
		if allowed[strings.ToLower(key)] {
			for _, value := range values {
				dst.Add(key, value)
			}
		}
	}
}

func extractModelIDs(body []byte) []string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return []string{}
	}
	items, ok := payload["data"].([]any)
	if !ok {
		return []string{}
	}
	ids := make([]string, 0, 8)
	for _, item := range items {
		if len(ids) >= 8 {
			break
		}
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, ok := obj["id"].(string)
		if ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func normalizePolicy(policy GatewayPolicy) GatewayPolicy {
	policy.RouteStrategy = normalizeStrategy(policy.RouteStrategy)
	if policy.RouteStrategy == "" {
		policy.RouteStrategy = "round_robin"
	}
	policy.FailureRetryMode = NormalizeGatewayFailureRetryMode(policy.FailureRetryMode)
	if policy.FailureThreshold <= 0 {
		policy.FailureThreshold = 3
	}
	if policy.CooldownSeconds <= 0 {
		policy.CooldownSeconds = 180
	}
	if policy.RequestTimeout <= 0 {
		policy.RequestTimeout = 60
	}
	if policy.MaxAttempts < 0 {
		policy.MaxAttempts = 0
	}
	switch strings.ToLower(strings.TrimSpace(policy.ConcurrencyOverflowStrategy)) {
	case "latency_first", "sequential":
		policy.ConcurrencyOverflowStrategy = strings.ToLower(strings.TrimSpace(policy.ConcurrencyOverflowStrategy))
	default:
		policy.ConcurrencyOverflowStrategy = "latency_first"
	}
	switch strings.ToLower(strings.TrimSpace(policy.ConcurrencyTransferStrategy)) {
	case "limit_only", "balance":
		policy.ConcurrencyTransferStrategy = strings.ToLower(strings.TrimSpace(policy.ConcurrencyTransferStrategy))
	default:
		policy.ConcurrencyTransferStrategy = "limit_only"
	}
	policy.SmartLatencyBias = clampBias(policy.SmartLatencyBias, 1.0)
	policy.SmartConcurrencyBias = clampBias(policy.SmartConcurrencyBias, 1.5)
	policy.SmartFailureBias = clampBias(policy.SmartFailureBias, 1.0)
	policy.SmartPriorityBias = clampBias(policy.SmartPriorityBias, 0.5)
	return policy
}

func NormalizeGatewayFailureRetryMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "all":
		return "all"
	default:
		return "retryable"
	}
}

func shouldFallbackStatus(status int, mode string) bool {
	if status >= 200 && status < 300 {
		return false
	}
	if NormalizeGatewayFailureRetryMode(mode) == "all" {
		return true
	}
	return status == http.StatusTooManyRequests || status >= 500
}

func clampBias(value, fallback float64) float64 {
	if value < 0 {
		return 0
	}
	if value == 0 && fallback > 0 {
		return fallback
	}
	if value > 5 {
		return 5
	}
	return value
}

func orDefault(value, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}

func normalizeStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "priority", "round_robin", "latency_first", "smart":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "round_robin"
	}
}

func stringMapValue(m models.JSONMap, key, fallback string) string {
	if m == nil || m[key] == nil {
		return fallback
	}
	switch typed := m[key].(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return fallback
		}
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func intValue(m models.JSONMap, key string, fallback int) int {
	raw := stringMapValue(m, key, "")
	if raw == "" {
		return fallback
	}
	var parsed int
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed
	}
	if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil {
		return parsed
	}
	return fallback
}

func stringPtr(value string) *string { return &value }

func shorten(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func newRequestID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func statusCodePtrOrNil(value int) *int {
	if value == 0 {
		return nil
	}
	v := value
	return &v
}
