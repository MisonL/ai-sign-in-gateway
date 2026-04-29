package handlers

import (
	"context"
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
	return schemas.SiteResponse{
		SiteBase: schemas.SiteBase{
			Name:         site.Name,
			BaseURL:      site.BaseURL,
			PluginKey:    site.PluginKey,
			GroupName:    site.GroupName,
			IsEnabled:    site.IsEnabled,
			Notes:        site.Notes,
			Credentials:  nonNilJSON(site.Credentials),
			PluginConfig: nonNilJSON(site.PluginConfig),
		},
		ID:               site.ID,
		LastStatus:       site.LastStatus,
		ConnectionStatus: site.LastStatus,
		LastMessage:      site.LastMessage,
		LastBalance:      site.LastBalance,
		BalanceDisplay:   balanceDisplay(site.LastBalance),
		PackageDisplay:   packageDisplay(site),
		CheckinStatus:    site.LastStatus,
		LastRunAt:        site.LastRunAt,
		CreatedAt:        site.CreatedAt,
		UpdatedAt:        &site.UpdatedAt,
	}
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

func mergeJSON(base models.JSONMap, updates models.JSONMap) models.JSONMap {
	merged := nonNilJSON(base)
	for key, value := range updates {
		merged[key] = value
	}
	return merged
}

func balanceDisplay(value *float64) *string {
	if value == nil {
		return nil
	}
	text := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", *value), "0"), ".")
	if text == "" {
		text = "0"
	}
	return &text
}

func packageDisplay(site models.Site) *string {
	value := strings.TrimSpace(jsonMapString(site.PluginConfig, "package_display"))
	if value == "" {
		value = strings.TrimSpace(jsonMapString(site.PluginConfig, "package_name"))
	}
	if value == "" {
		value = strings.TrimSpace(jsonMapString(site.PluginConfig, "plan_name"))
	}
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
		"site_id":      result.SiteID,
		"route_id":     result.RouteID,
		"ok":           result.OK,
		"status_code":  result.StatusCode,
		"latency_ms":   result.LatencyMS,
		"remaining":    result.Remaining,
		"unit":         result.Unit,
		"base_url":     result.BaseURL,
		"message":      result.Message,
		"checked_at":   result.CheckedAt,
		"last_balance": result.Remaining,
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
