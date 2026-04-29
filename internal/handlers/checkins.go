package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"ai-sign-in-gateway/internal/httpx"
	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/schemas"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func (a *App) CheckinRoutes(r chi.Router) {
	r.Post("/batch", a.RunBatchCheckin)
	r.Get("/runs", a.CheckinRuns)
	r.Get("/sites", a.CheckinSites)
	r.Post("/sites/{siteID}/participation", a.UpdateCheckinParticipation)
}

func (a *App) CheckinRuns(w http.ResponseWriter, r *http.Request) {
	var runs []models.CheckinRun
	a.DB.Preload("Site").Order("started_at desc").Limit(80).Find(&runs)
	out := make([]schemas.CheckinRunResponse, 0, len(runs))
	for _, run := range runs {
		out = append(out, checkinRunResponse(run))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) CheckinSites(w http.ResponseWriter, r *http.Request) {
	var sites []models.Site
	a.DB.Order("name asc").Find(&sites)
	out := make([]map[string]any, 0, len(sites))
	for _, site := range sites {
		out = append(out, map[string]any{
			"id": site.ID, "name": site.Name, "plugin_key": site.PluginKey, "group_name": site.GroupName,
			"base_url": site.BaseURL, "is_enabled": site.IsEnabled, "can_checkin": true,
			"include_in_checkin": includeInCheckin(site), "checkin_label": "签到", "reason": "",
			"last_status": site.LastStatus, "connection_status": site.LastStatus, "last_message": site.LastMessage,
			"last_balance": site.LastBalance, "balance_display": balanceDisplay(site.LastBalance),
			"package_display": packageDisplay(site), "checkin_status": site.LastStatus, "last_run_at": site.LastRunAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) RunBatchCheckin(w http.ResponseWriter, r *http.Request) {
	var payload schemas.BatchCheckinRequest
	_ = httpx.Decode(r, &payload)
	query := a.DB.Model(&models.Site{})
	if len(payload.SiteIDs) > 0 {
		query = query.Where("id IN ?", payload.SiteIDs)
	}
	if payload.OnlyEnabled {
		query = query.Where("is_enabled = ?", true)
	}
	var sites []models.Site
	if err := query.Order("name asc").Find(&sites).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]schemas.CheckinRunResponse, 0, len(sites))
	for _, site := range sites {
		if payload.OnlyEnabled && !includeInCheckin(site) {
			continue
		}
		run := a.executeSiteCheckin(r, site, "manual")
		out = append(out, checkinRunResponse(run))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) UpdateCheckinParticipation(w http.ResponseWriter, r *http.Request) {
	site, ok := a.getSite(w, chi.URLParam(r, "siteID"))
	if !ok {
		return
	}
	var payload struct {
		IncludeInCheckin bool `json:"include_in_checkin"`
	}
	if err := httpx.Decode(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	site.PluginConfig = mergeJSON(site.PluginConfig, models.JSONMap{
		"include_in_checkin": payload.IncludeInCheckin,
	})
	if err := a.DB.Model(&site).Update("plugin_config", site.PluginConfig).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": site.ID, "name": site.Name, "plugin_key": site.PluginKey, "group_name": site.GroupName,
		"base_url": site.BaseURL, "is_enabled": site.IsEnabled, "can_checkin": true,
		"include_in_checkin": includeInCheckin(site), "checkin_label": "签到", "reason": "",
		"last_status": site.LastStatus, "connection_status": site.LastStatus, "last_message": site.LastMessage,
		"last_balance": site.LastBalance, "balance_display": balanceDisplay(site.LastBalance),
		"package_display": packageDisplay(site), "checkin_status": site.LastStatus, "last_run_at": site.LastRunAt,
	})
}

func (a *App) SiteCheckin(w http.ResponseWriter, r *http.Request) {
	site, ok := a.getSite(w, chi.URLParam(r, "siteID"))
	if !ok {
		return
	}
	run := a.executeSiteCheckin(r, site, "manual")
	writeJSON(w, http.StatusOK, map[string]any{"run": run.ID, "status": run.Status, "message": run.Message, "balance": run.Balance})
}

func (a *App) executeSiteCheckin(r *http.Request, site models.Site, triggerType string) models.CheckinRun {
	now := time.Now().UTC()
	run := models.CheckinRun{SiteID: &site.ID, TriggerType: triggerType, Status: "running", Message: "签到执行中。", StartedAt: now}
	_ = a.DB.Create(&run).Error
	plugin, err := a.PluginManager.Get(site.PluginKey)
	if err != nil {
		finishRun(a.DB, &run, "failed", err.Error(), nil, nil)
		return run
	}
	settings, _ := a.systemSettings()
	result, err := plugin.Checkin(r.Context(), site, settings.RequestTimeout)
	if err != nil {
		finishRun(a.DB, &run, "failed", err.Error(), nil, nil)
		updateSiteAfterCheckin(a.DB, &site, "failed", err.Error(), nil, now)
		return run
	}
	status := "failed"
	if result.Success {
		status = "success"
	}
	finishRun(a.DB, &run, status, result.Message, result.Balance, result.ResponseExcerpt)
	updateSiteAfterCheckin(a.DB, &site, status, result.Message, result.Balance, now)
	return run
}

func finishRun(db *gorm.DB, run *models.CheckinRun, status, message string, balance *float64, excerpt *string) {
	finished := time.Now().UTC()
	run.Status = status
	run.Message = message
	run.Balance = balance
	run.ResponseExcerpt = excerpt
	run.FinishedAt = &finished
	_ = db.Save(run).Error
}

func updateSiteAfterCheckin(db *gorm.DB, site *models.Site, status, message string, balance *float64, runAt time.Time) {
	site.LastStatus = &status
	site.LastMessage = &message
	site.LastBalance = balance
	site.LastRunAt = &runAt
	_ = db.Save(site).Error
}

func includeInCheckin(site models.Site) bool {
	if site.PluginConfig == nil || site.PluginConfig["include_in_checkin"] == nil {
		return site.IsEnabled
	}
	switch typed := site.PluginConfig["include_in_checkin"].(type) {
	case bool:
		return typed
	case string:
		value := strings.TrimSpace(strings.ToLower(typed))
		if value == "" {
			return site.IsEnabled
		}
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
		return site.IsEnabled
	case float64:
		return typed != 0
	case int:
		return typed != 0
	default:
		return site.IsEnabled
	}
}

func (a *App) SiteQueue(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (a *App) ActivateQueueTask(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"id": 0, "task_key": chi.URLParam(r, "taskKey"), "title": "", "detail": "", "status": "done", "sort_order": 0, "action_key": "", "action_label": "", "last_message": nil, "last_error": nil, "completed_at": time.Now().UTC(), "updated_at": time.Now().UTC()})
}

func (a *App) TotpPreview(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"code": "", "expires_in": 0})
}
