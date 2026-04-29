package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-sign-in-gateway/internal/models"
	"github.com/glebarez/sqlite"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func TestUpdateCheckinParticipationPersistsSetting(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	site := models.Site{
		Name:         "demo",
		BaseURL:      "https://example.com",
		PluginKey:    "sub2api-platform",
		IsEnabled:    true,
		PluginConfig: models.JSONMap{},
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}

	app := &App{DB: db}
	body := bytes.NewReader([]byte(`{"include_in_checkin":false}`))
	req := httptest.NewRequest(http.MethodPost, "/sites/1/participation", body)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("siteID", "1")
	req = contextWithRoute(req, routeCtx)
	rec := httptest.NewRecorder()

	app.UpdateCheckinParticipation(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var stored models.Site
	if err := db.First(&stored, site.ID).Error; err != nil {
		t.Fatalf("reload site: %v", err)
	}
	if got := includeInCheckin(stored); got {
		t.Fatalf("includeInCheckin = %v", got)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/checkins/sites", nil)
	listRec := httptest.NewRecorder()
	app.CheckinSites(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}
	var payload []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("payload len = %d", len(payload))
	}
	if got, ok := payload[0]["include_in_checkin"].(bool); !ok || got {
		t.Fatalf("include_in_checkin response = %v", payload[0]["include_in_checkin"])
	}
}

func contextWithRoute(req *http.Request, routeCtx *chi.Context) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}
