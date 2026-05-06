package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-sign-in-gateway/internal/httpx"
	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/schemas"
	"ai-sign-in-gateway/internal/services"
)

func siteRequestTimeoutSeconds(value int) int {
	return clampInt(value, 5, 120, 20)
}

func siteOperationContext(parent context.Context, timeoutSeconds int) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, time.Duration(siteRequestTimeoutSeconds(timeoutSeconds))*time.Second)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	httpx.JSON(w, status, value)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	httpx.Error(w, status, detail)
}

func siteResponse(site models.Site) schemas.SiteResponse {
	return siteResponseWithSupportedModels(site, true, nil)
}

func siteListResponse(site models.Site) schemas.SiteResponse {
	return siteResponseWithSupportedModels(site, false, nil)
}

func siteResponseWithSupportedModels(site models.Site, includeSecrets bool, supportedModels []string) schemas.SiteResponse {
	credentials := cloneJSONMap(nonNilJSON(site.Credentials))
	pluginConfig := stripSiteSupportedModels(site.PluginConfig)
	supportedModels = services.NormalizeStringList(supportedModels)
	if !includeSecrets {
		credentials = redactJSONMap(credentials)
		pluginConfig = redactJSONMap(pluginConfig)
	}
	return schemas.SiteResponse{
		SiteBase: schemas.SiteBase{
			Name:            site.Name,
			BaseURL:         site.BaseURL,
			PluginKey:       site.PluginKey,
			GroupName:       site.GroupName,
			SupportedModels: supportedModels,
			IsEnabled:       site.IsEnabled,
			Notes:           site.Notes,
			Credentials:     credentials,
			PluginConfig:    pluginConfig,
		},
		ID:               site.ID,
		LastStatus:       site.LastStatus,
		ConnectionStatus: site.LastStatus,
		LastMessage:      site.LastMessage,
		LastBalance:      site.LastBalance,
		BalanceDisplay:   balanceDisplayWithUnit(site.LastBalance, jsonMapString(site.PluginConfig, "balance_unit")),
		BalanceUnit:      stringPtrIfNonEmpty(services.NormalizeBalanceUnit(jsonMapString(site.PluginConfig, "balance_unit"))),
		PackageRemaining: jsonMapNumberPtr(site.PluginConfig, "package_remaining"),
		PackageTotal:     jsonMapNumberPtr(site.PluginConfig, "package_total"),
		PackageUsed:      jsonMapNumberPtr(site.PluginConfig, "package_used"),
		PackageUnit:      stringPtrIfNonEmpty(services.NormalizeBalanceUnit(jsonMapString(site.PluginConfig, "package_unit"))),
		PackageDisplay:   packageDisplay(site),
		CheckinStatus:    site.LastStatus,
		LastRunAt:        site.LastRunAt,
		CreatedAt:        site.CreatedAt,
		UpdatedAt:        &site.UpdatedAt,
	}
}

func stripSiteSupportedModels(config models.JSONMap) models.JSONMap {
	next := cloneJSONMap(nonNilJSON(config))
	delete(next, "supported_models")
	normalizePluginConfigBalanceUnits(next)
	return next
}

func normalizePluginConfigBalanceUnits(config models.JSONMap) {
	if config == nil {
		return
	}
	if value := jsonMapString(config, "balance_unit"); value != "" {
		config["balance_unit"] = services.NormalizeBalanceUnit(value)
	}
	if value := jsonMapString(config, "package_unit"); value != "" {
		config["package_unit"] = services.NormalizeBalanceUnit(value)
	}
	if value := jsonMapString(config, "package_display"); value != "" {
		config["package_display"] = services.NormalizeBalanceUnitText(value)
	}
}

func stringPtrIfNonEmpty(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func firstNonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) == 0 {
			continue
		}
		return append([]string(nil), value...)
	}
	return nil
}

func packageQuotaMap(site models.Site) map[string]any {
	return map[string]any{
		"package_remaining": jsonMapNumberPtr(site.PluginConfig, "package_remaining"),
		"package_total":     jsonMapNumberPtr(site.PluginConfig, "package_total"),
		"package_used":      jsonMapNumberPtr(site.PluginConfig, "package_used"),
		"package_unit":      services.NormalizeBalanceUnit(jsonMapString(site.PluginConfig, "package_unit")),
	}
}

func jsonMapNumberPtr(m models.JSONMap, key string) *float64 {
	if m == nil || m[key] == nil {
		return nil
	}
	switch typed := m[key].(type) {
	case float64:
		return &typed
	case float32:
		value := float64(typed)
		return &value
	case int:
		value := float64(typed)
		return &value
	case int64:
		value := float64(typed)
		return &value
	case json.Number:
		value, err := typed.Float64()
		if err == nil {
			return &value
		}
	case string:
		var value float64
		if _, err := fmt.Sscan(strings.TrimSpace(typed), &value); err == nil {
			return &value
		}
	}
	return nil
}

func checkinRunResponse(run models.CheckinRun) schemas.CheckinRunResponse {
	var siteName *string
	if run.Site != nil {
		siteName = &run.Site.Name
	}
	return schemas.CheckinRunResponse{
		ID:              run.ID,
		SiteID:          run.SiteID,
		SiteName:        siteName,
		TriggerType:     run.TriggerType,
		Status:          run.Status,
		Message:         run.Message,
		ResponseExcerpt: run.ResponseExcerpt,
		Balance:         run.Balance,
		BalanceUnit:     nil,
		AttemptCount:    run.AttemptCount,
		StartedAt:       run.StartedAt,
		FinishedAt:      run.FinishedAt,
	}
}

func nonNilJSON(value models.JSONMap) models.JSONMap {
	if value == nil {
		return models.JSONMap{}
	}
	return value
}

func cloneJSONMap(value models.JSONMap) models.JSONMap {
	if value == nil {
		return models.JSONMap{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		clone := models.JSONMap{}
		for key, item := range value {
			clone[key] = item
		}
		return clone
	}
	var clone models.JSONMap
	if err := json.Unmarshal(data, &clone); err != nil || clone == nil {
		return models.JSONMap{}
	}
	return clone
}

func redactJSONMap(value models.JSONMap) models.JSONMap {
	out := models.JSONMap{}
	for key, item := range value {
		out[key] = redactJSONValue(key, item)
	}
	return out
}

func redactJSONValue(key string, value any) any {
	if isSensitiveJSONKey(key) {
		return redactSensitiveJSONValue(value)
	}
	switch typed := value.(type) {
	case map[string]any:
		return redactJSONMap(models.JSONMap(typed))
	case models.JSONMap:
		return redactJSONMap(typed)
	case []any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, redactJSONValue("", item))
		}
		return items
	case []map[string]any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, redactJSONMap(models.JSONMap(item)))
		}
		return items
	default:
		return value
	}
}

func redactSensitiveJSONValue(value any) any {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return typed
		}
		return "********"
	case []any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, redactSensitiveJSONValue(item))
		}
		return items
	case map[string]any:
		out := models.JSONMap{}
		for key, item := range typed {
			out[key] = redactSensitiveJSONValue(item)
		}
		return out
	case models.JSONMap:
		out := models.JSONMap{}
		for key, item := range typed {
			out[key] = redactSensitiveJSONValue(item)
		}
		return out
	default:
		return value
	}
}

func isSensitiveJSONKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return false
	}
	switch normalized {
	case "api_key", "apikey", "api-key", "key", "token", "access_token", "refresh_token", "auth_token", "authorization", "cookie", "password", "secret", "client_secret", "totp_secret", "totp_otpauth_url":
		return true
	}
	return strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "cookie") ||
		strings.Contains(normalized, "authorization")
}

func mergeJSON(base models.JSONMap, updates models.JSONMap) models.JSONMap {
	merged := nonNilJSON(base)
	for key, value := range updates {
		merged[key] = value
	}
	return merged
}

func mergeCredentials(base models.JSONMap, updates models.JSONMap) models.JSONMap {
	merged := cloneJSONMap(nonNilJSON(base))
	for key, value := range updates {
		merged[key] = value
	}
	if _, ok := updates["api_keys"]; ok {
		merged["api_keys"] = mergeAPIKeyLists(base["api_keys"], updates["api_keys"])
	}
	return merged
}

func mergeCredentialUpdates(site *models.Site, updates models.JSONMap) models.JSONMap {
	responseUpdates := cloneJSONMap(nonNilJSON(updates))
	if len(responseUpdates) == 0 {
		return responseUpdates
	}
	site.Credentials = mergeCredentials(site.Credentials, updates)
	if _, ok := updates["api_keys"]; ok {
		responseUpdates["api_keys"] = site.Credentials["api_keys"]
	}
	return responseUpdates
}

func mergeAPIKeyLists(existingValue any, syncedValue any) []map[string]any {
	synced := apiKeyListFromAny(syncedValue)
	existing := apiKeyListFromAny(existingValue)
	merged := make([]map[string]any, 0, len(synced)+len(existing))
	seen := map[string]bool{}
	for _, item := range synced {
		key := apiKeyEntryKey(item)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, item)
	}
	for _, item := range existing {
		key := apiKeyEntryKey(item)
		if key == "" || seen[key] || !isManualAPIKeyEntry(item) {
			continue
		}
		seen[key] = true
		merged = append(merged, item)
	}
	return merged
}

func apiKeyListFromAny(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if item == nil {
				continue
			}
			out = append(out, cloneStringAnyMap(item))
		}
		return out
	case []models.JSONMap:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if item == nil {
				continue
			}
			out = append(out, cloneStringAnyMap(map[string]any(item)))
		}
		return out
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			switch obj := item.(type) {
			case map[string]any:
				out = append(out, cloneStringAnyMap(obj))
			case models.JSONMap:
				out = append(out, cloneStringAnyMap(map[string]any(obj)))
			}
		}
		return out
	default:
		return nil
	}
}

func cloneStringAnyMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func apiKeyEntryKey(item map[string]any) string {
	value := strings.TrimSpace(fmt.Sprint(item["key"]))
	if value == "<nil>" {
		return ""
	}
	return value
}

func isManualAPIKeyEntry(item map[string]any) bool {
	source := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["source"])))
	if source == "manual" || source == "custom" || source == "user" {
		return true
	}
	if value, ok := item["is_custom"].(bool); ok && value {
		return true
	}
	if value, ok := item["manual"].(bool); ok && value {
		return true
	}
	return false
}

func balanceDisplay(value *float64) *string {
	return balanceDisplayWithUnit(value, "")
}

func balanceDisplayWithUnit(value *float64, unit string) *string {
	if value == nil {
		return nil
	}
	text := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", *value), "0"), ".")
	if text == "" {
		text = "0"
	}
	unit = services.NormalizeBalanceUnit(unit)
	if unit == "" {
		return &text
	}
	if services.BalanceUnitIsSymbol(unit) {
		text = unit + text
	} else {
		text = text + " " + unit
	}
	return &text
}

func packageDisplay(site models.Site) *string {
	value := services.GatewayRoutePackageDisplay(site)
	if value == "" {
		return nil
	}
	return &value
}

func jsonMapString(m models.JSONMap, key string) string {
	if m == nil || m[key] == nil {
		return ""
	}
	switch typed := m[key].(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func balanceProbeResponse(result services.BalanceProbeResult) map[string]any {
	return map[string]any{
		"site_id":           result.SiteID,
		"route_id":          result.RouteID,
		"ok":                result.OK,
		"status_code":       result.StatusCode,
		"latency_ms":        result.LatencyMS,
		"remaining":         result.Remaining,
		"unit":              services.NormalizeBalanceUnit(result.Unit),
		"base_url":          result.BaseURL,
		"balance_probe_url": result.BalanceProbeURL,
		"message":           result.Message,
		"checked_at":        result.CheckedAt,
		"last_balance":      result.Remaining,
		"balance_display":   balanceDisplayWithUnit(result.Remaining, result.Unit),
	}
}

func mergePackageQuotaPluginConfig(target models.JSONMap, remaining, total, used *float64, unit *string) {
	if target == nil {
		return
	}
	if remaining != nil {
		target["package_remaining"] = *remaining
	}
	if total != nil {
		target["package_total"] = *total
	}
	if used != nil {
		target["package_used"] = *used
	}
	if unit != nil && strings.TrimSpace(*unit) != "" {
		target["package_unit"] = services.NormalizeBalanceUnit(*unit)
	}
}

func (a *App) syncSiteInviteInfo(ctx context.Context, site *models.Site) {
	if site == nil || site.ID == 0 || strings.TrimSpace(site.PluginKey) == "" {
		return
	}
	plugin, err := a.PluginManager.Get(site.PluginKey)
	if err != nil {
		return
	}
	settings, _ := a.systemSettings()
	timeout := siteRequestTimeoutSeconds(settings.RequestTimeout)
	opCtx, cancel := siteOperationContext(ctx, timeout)
	defer cancel()
	status, err := plugin.FetchAccountStatus(opCtx, *site, timeout)
	if err != nil {
		return
	}
	updates := models.JSONMap{}
	if status.InviteLink != nil && strings.TrimSpace(*status.InviteLink) != "" {
		updates["invite_link"] = strings.TrimSpace(*status.InviteLink)
	}
	if status.InviteCode != nil && strings.TrimSpace(*status.InviteCode) != "" {
		updates["invite_code"] = strings.TrimSpace(*status.InviteCode)
	}
	if status.PackageDisplay != nil && strings.TrimSpace(*status.PackageDisplay) != "" {
		updates["package_display"] = strings.TrimSpace(*status.PackageDisplay)
	}
	mergePackageQuotaPluginConfig(updates, status.PackageRemaining, status.PackageTotal, status.PackageUsed, status.PackageUnit)
	if len(updates) == 0 {
		return
	}
	site.PluginConfig = mergeJSON(site.PluginConfig, updates)
	_ = a.DB.Model(site).Update("plugin_config", site.PluginConfig).Error
}

func (a *App) syncSiteInviteInfoAsync(site models.Site) {
	if site.ID == 0 {
		return
	}
	go a.syncSiteInviteInfo(context.Background(), &site)
}
