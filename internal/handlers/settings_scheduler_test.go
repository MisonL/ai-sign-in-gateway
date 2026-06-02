package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/plugins"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRunSchedulerNowExecutesCheckinBatch(t *testing.T) {
	requestCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/checkin":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "signed",
				"data":    map[string]any{"balance": 18.5, "currency": "USD"},
			})
		case "/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "ok",
				"data":    map[string]any{"logged_in": true, "balance": 20.5, "currency": "USD"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file:settings-run-scheduler-now?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	if err := db.Create(&models.SystemSetting{ID: 1, RequestTimeout: 5, OnlyEnabledSites: true}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	enabled := schedulerNowTestSite(upstream.URL, true)
	disabled := schedulerNowTestSite(upstream.URL, false)
	disabled.Name = "disabled"
	if err := db.Create(&enabled).Error; err != nil {
		t.Fatalf("create enabled site: %v", err)
	}
	if err := db.Create(&disabled).Error; err != nil {
		t.Fatalf("create disabled site: %v", err)
	}

	app := &App{DB: db, PluginManager: plugins.NewManager()}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/settings/scheduler/run-now", nil)
	app.RunSchedulerNow(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "ok" || payload["message"] != "已执行一次签到：成功 1，失败 0。" {
		t.Fatalf("payload = %#v", payload)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want checkin and status requests", requestCount)
	}
	var runs []models.CheckinRun
	if err := db.Order("id asc").Find(&runs).Error; err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].SiteID == nil || *runs[0].SiteID != enabled.ID || runs[0].Status != "success" {
		t.Fatalf("runs = %+v", runs)
	}
	var stored models.Site
	if err := db.First(&stored, enabled.ID).Error; err != nil {
		t.Fatalf("reload site: %v", err)
	}
	if stored.LastBalance == nil || *stored.LastBalance != 20.5 {
		t.Fatalf("last balance = %v", stored.LastBalance)
	}
}

func TestRunSchedulerNowAppliesRetrySetting(t *testing.T) {
	checkinCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/checkin":
			checkinCount++
			if checkinCount == 1 {
				http.Error(w, `{"message":"temporary"}`, http.StatusBadGateway)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "signed after retry",
				"data":    map[string]any{"balance": 18.5, "currency": "USD"},
			})
		case "/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "ok",
				"data":    map[string]any{"logged_in": true, "balance": 20.5, "currency": "USD"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file:settings-run-scheduler-now-retry?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	if err := db.Create(&models.SystemSetting{
		ID:                       1,
		RequestTimeout:           5,
		OnlyEnabledSites:         true,
		CheckinConcurrency:       1,
		CheckinGlobalConcurrency: 1,
		CheckinIntervalSeconds:   0,
		RetryCount:               1,
	}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	site := schedulerNowTestSite(upstream.URL, true)
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &App{DB: db, PluginManager: plugins.NewManager()}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/settings/scheduler/run-now", nil)
	app.RunSchedulerNow(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if checkinCount != 2 {
		t.Fatalf("checkin attempts = %d", checkinCount)
	}
	var runs []models.CheckinRun
	if err := db.Order("id asc").Find(&runs).Error; err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 || runs[0].Status != "failed" || runs[1].Status != "success" {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestCheckinSchedulerRunsDueBatchOncePerLocalDay(t *testing.T) {
	requestCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/checkin":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "scheduled",
				"data":    map[string]any{"balance": 18.5, "currency": "USD"},
			})
		case "/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "ok",
				"data":    map[string]any{"logged_in": true, "balance": 20.5, "currency": "USD"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file:settings-scheduler-loop?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	if err := db.Create(&models.SystemSetting{
		ID:               1,
		Timezone:         "Asia/Shanghai",
		ScheduleEnabled:  true,
		DailyRunTime:     "09:00",
		RequestTimeout:   5,
		OnlyEnabledSites: true,
	}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	site := schedulerNowTestSite(upstream.URL, true)
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	lastRunDate := ""
	runner := CheckinSchedulerRunner{
		App: &App{DB: db, PluginManager: plugins.NewManager()},
		Now: func() time.Time {
			return time.Date(2026, 6, 2, 9, 1, 0, 0, time.FixedZone("CST", 8*60*60))
		},
	}
	if !runner.RunDue(context.Background(), &lastRunDate) {
		t.Fatalf("scheduler did not run")
	}
	if runner.RunDue(context.Background(), &lastRunDate) {
		t.Fatalf("scheduler ran twice in one local day")
	}
	lastRunDate = ""
	if runner.RunDue(context.Background(), &lastRunDate) {
		t.Fatalf("scheduler ignored existing scheduled run after restart")
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want checkin and status requests once", requestCount)
	}
	var runs []models.CheckinRun
	if err := db.Order("id asc").Find(&runs).Error; err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].TriggerType != "scheduled" || runs[0].Status != "success" {
		t.Fatalf("runs = %+v", runs)
	}
}

func schedulerNowTestSite(baseURL string, enabled bool) models.Site {
	return models.Site{
		Name:      "enabled",
		BaseURL:   baseURL,
		PluginKey: "http-relay-station",
		IsEnabled: enabled,
		Credentials: models.JSONMap{
			"account": "demo",
		},
		PluginConfig: models.JSONMap{
			"auth_mode":                 "none",
			"status_path":               "/status",
			"status_method":             "GET",
			"status_login_path":         "data.logged_in",
			"status_balance_path":       "data.balance",
			"status_balance_unit_path":  "data.currency",
			"status_message_path":       "message",
			"checkin_path":              "/checkin",
			"checkin_method":            "POST",
			"checkin_success_path":      "success",
			"checkin_message_path":      "message",
			"checkin_balance_path":      "data.balance",
			"checkin_balance_unit_path": "data.currency",
			"default_balance_unit":      "USD",
		},
	}
}
