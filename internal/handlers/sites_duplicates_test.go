package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ai-sign-in-gateway/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDuplicateSitesDetectsStoredGroups(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:sites-duplicates-detect?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	sites := []models.Site{
		{
			Name: "keep", BaseURL: "HTTPS://Example.COM/", PluginKey: "http-relay-station",
			IsEnabled: true, CreatedAt: older, Credentials: models.JSONMap{"email": "User@Example.com", "password": "pw"},
			PluginConfig: models.JSONMap{"auth_mode": "none"},
		},
		{
			Name: "duplicate", BaseURL: "https://example.com", PluginKey: "http-relay-station",
			IsEnabled: false, CreatedAt: newer, Credentials: models.JSONMap{"email": "user@example.com", "password": "pw"},
			PluginConfig: models.JSONMap{},
		},
		{
			Name: "different-key", BaseURL: "https://example.com", PluginKey: "api-supplier",
			IsEnabled: true, Credentials: models.JSONMap{"api_key": "other-key"},
		},
	}
	if err := db.Create(&sites).Error; err != nil {
		t.Fatalf("create sites: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sites/cleanup-duplicates", nil)
	(&App{DB: db}).ListDuplicateSites(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var groups []duplicateSiteGroup
	if err := json.Unmarshal(rec.Body.Bytes(), &groups); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups len = %d, body = %s", len(groups), rec.Body.String())
	}
	group := groups[0]
	if group.PluginKey != "http-relay-station" || group.BaseURL != "https://example.com" || group.Account != "user@example.com" || !group.PasswordPresent {
		t.Fatalf("group = %+v", group)
	}
	if group.SuggestedKeepID != sites[0].ID || len(group.SiteIDs) != 2 || len(group.Sites) != 2 {
		t.Fatalf("unexpected duplicate group = %+v", group)
	}
}

func TestDuplicateSitesKeepsDifferentPluginsSeparate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:sites-duplicates-plugin-boundary?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	sites := []models.Site{
		{
			Name: "station", BaseURL: "https://example.com", PluginKey: "http-relay-station",
			IsEnabled: true, Credentials: models.JSONMap{"email": "user@example.com", "password": "pw"},
		},
		{
			Name: "sub2api", BaseURL: "https://example.com/", PluginKey: "sub2api-platform",
			IsEnabled: true, Credentials: models.JSONMap{"email": "USER@example.com", "password": "pw"},
		},
	}
	if err := db.Create(&sites).Error; err != nil {
		t.Fatalf("create sites: %v", err)
	}

	groups, err := duplicateSiteGroups(db)
	if err != nil {
		t.Fatalf("duplicate groups: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected no cross-plugin duplicate groups, got %+v", groups)
	}

	result, err := mergeDuplicateSites(db)
	if err != nil {
		t.Fatalf("merge duplicates: %v", err)
	}
	if result.DeletedSiteCount != 0 || result.MergedGroupCount != 0 {
		t.Fatalf("cross-plugin merge result = %+v", result)
	}
	var siteCount int64
	if err := db.Model(&models.Site{}).Count(&siteCount).Error; err != nil {
		t.Fatalf("count sites: %v", err)
	}
	if siteCount != 2 {
		t.Fatalf("site count after merge = %d", siteCount)
	}
}

func TestMergeDuplicateSitesKeepsSuggestedAndDeletesDuplicates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:sites-duplicates-merge?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	keep := models.Site{
		Name: "keep", BaseURL: "https://example.com", PluginKey: "http-relay-station", GroupName: "alpha",
		IsEnabled: true, Notes: "keep note", Credentials: models.JSONMap{"email": "user@example.com", "password": "pw"},
		PluginConfig: models.JSONMap{"auth_mode": "none"},
	}
	duplicate := models.Site{
		Name: "duplicate", BaseURL: "https://example.com/", PluginKey: "http-relay-station", GroupName: "beta",
		IsEnabled: false, Notes: "duplicate note", Credentials: models.JSONMap{"email": "USER@example.com", "password": "pw", "cookie": "sid=1"},
		PluginConfig: models.JSONMap{"status_path": "/status"},
	}
	if err := db.Create(&keep).Error; err != nil {
		t.Fatalf("create keep site: %v", err)
	}
	if err := db.Create(&duplicate).Error; err != nil {
		t.Fatalf("create duplicate site: %v", err)
	}
	keepRoute := models.GatewayRouteState{SiteID: keep.ID, KeyFingerprint: "fp-dup", KeyName: "keep", RouteType: "codex", RoutePath: "responses"}
	if err := db.Create(&keepRoute).Error; err != nil {
		t.Fatalf("create keep route: %v", err)
	}
	keepMismatchedRoute := models.GatewayRouteState{SiteID: keep.ID, KeyFingerprint: "fp-mismatched", KeyName: "keep-mismatched", RouteType: "codex", RoutePath: "responses"}
	if err := db.Create(&keepMismatchedRoute).Error; err != nil {
		t.Fatalf("create keep mismatched route: %v", err)
	}
	route := models.GatewayRouteState{SiteID: duplicate.ID, KeyFingerprint: "fp-dup", KeyName: "duplicate", RouteType: "codex", RoutePath: "responses"}
	if err := db.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	mismatchedRoute := models.GatewayRouteState{SiteID: duplicate.ID, KeyFingerprint: "fp-mismatched", KeyName: "mismatched", RouteType: "gpt", RoutePath: "chat/completions"}
	if err := db.Create(&mismatchedRoute).Error; err != nil {
		t.Fatalf("create mismatched route: %v", err)
	}
	unmappedRoute := models.GatewayRouteState{SiteID: duplicate.ID, KeyFingerprint: "fp-unmapped", KeyName: "unmapped"}
	if err := db.Create(&unmappedRoute).Error; err != nil {
		t.Fatalf("create unmapped route: %v", err)
	}
	group := models.GatewayRouteGroup{Name: "routes"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create route group: %v", err)
	}
	member := models.GatewayRouteGroupMember{GroupID: group.ID, RouteStateID: route.ID}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("create route group member: %v", err)
	}
	if err := db.Create(&models.SiteQueueTask{SiteID: duplicate.ID, TaskKey: "todo", Title: "Todo"}).Error; err != nil {
		t.Fatalf("create queue task: %v", err)
	}
	if err := db.Create(&models.CheckinRun{
		SiteID:    &duplicate.ID,
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create checkin run: %v", err)
	}
	if err := db.Create(&models.GatewayRequestLog{
		RouteStateID: &route.ID,
		SiteID:       &duplicate.ID,
		RequestID:    "req-duplicate",
		CreatedAt:    time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create request log: %v", err)
	}
	if err := db.Create(&models.GatewayRequestLog{
		RouteStateID: &mismatchedRoute.ID,
		SiteID:       &duplicate.ID,
		RequestID:    "req-mismatched",
		CreatedAt:    time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create mismatched request log: %v", err)
	}
	if err := db.Create(&models.GatewayRequestLog{
		RouteStateID: &unmappedRoute.ID,
		SiteID:       &duplicate.ID,
		RequestID:    "req-unmapped",
		CreatedAt:    time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create unmapped request log: %v", err)
	}
	if err := db.Create(&models.ChatSession{
		SiteID: &duplicate.ID,
		Title:  "duplicate session",
	}).Error; err != nil {
		t.Fatalf("create chat session: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/sites/cleanup-duplicates/merge", nil)
	(&App{DB: db}).MergeDuplicateSites(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result duplicateSiteMergeResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.MergedGroupCount != 1 || result.DeletedSiteCount != 1 || result.RemainingGroupCount != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.KeptSiteIDs) != 1 || result.KeptSiteIDs[0] != keep.ID || len(result.DeletedSiteIDs) != 1 || result.DeletedSiteIDs[0] != duplicate.ID {
		t.Fatalf("ids = %+v", result)
	}

	var stored models.Site
	if err := db.First(&stored, keep.ID).Error; err != nil {
		t.Fatalf("reload keep site: %v", err)
	}
	if got := jsonMapString(stored.Credentials, "cookie"); got != "sid=1" {
		t.Fatalf("merged cookie = %q", got)
	}
	if got := jsonMapString(stored.PluginConfig, "status_path"); got != "/status" {
		t.Fatalf("merged status_path = %q", got)
	}
	if !strings.Contains(stored.GroupName, "alpha") || !strings.Contains(stored.GroupName, "beta") {
		t.Fatalf("group name = %q", stored.GroupName)
	}
	if !strings.Contains(stored.Notes, "keep note") || !strings.Contains(stored.Notes, "duplicate note") {
		t.Fatalf("notes = %q", stored.Notes)
	}

	var duplicateCount int64
	if err := db.Model(&models.Site{}).Where("id = ?", duplicate.ID).Count(&duplicateCount).Error; err != nil {
		t.Fatalf("count duplicate site: %v", err)
	}
	if duplicateCount != 0 {
		t.Fatalf("duplicate site count = %d", duplicateCount)
	}
	var leftoverCount int64
	if err := db.Model(&models.GatewayRouteGroupMember{}).Where("route_state_id = ?", route.ID).Count(&leftoverCount).Error; err != nil {
		t.Fatalf("count route group members: %v", err)
	}
	if leftoverCount != 0 {
		t.Fatalf("leftover route group members = %d", leftoverCount)
	}
	if err := db.Model(&models.SiteQueueTask{}).Where("site_id = ?", duplicate.ID).Count(&leftoverCount).Error; err != nil {
		t.Fatalf("count queue tasks: %v", err)
	}
	if leftoverCount != 0 {
		t.Fatalf("leftover queue tasks = %d", leftoverCount)
	}
	if err := db.Model(&models.CheckinRun{}).Where("site_id = ?", duplicate.ID).Count(&leftoverCount).Error; err != nil {
		t.Fatalf("count checkin runs: %v", err)
	}
	if leftoverCount != 0 {
		t.Fatalf("leftover checkin runs = %d", leftoverCount)
	}
	if err := db.Model(&models.CheckinRun{}).Where("site_id = ?", keep.ID).Count(&leftoverCount).Error; err != nil {
		t.Fatalf("count reassigned checkin runs: %v", err)
	}
	if leftoverCount != 1 {
		t.Fatalf("reassigned checkin runs = %d", leftoverCount)
	}
	if err := db.Model(&models.GatewayRequestLog{}).Where("site_id = ?", duplicate.ID).Count(&leftoverCount).Error; err != nil {
		t.Fatalf("count request logs: %v", err)
	}
	if leftoverCount != 0 {
		t.Fatalf("leftover request logs = %d", leftoverCount)
	}
	if err := db.Model(&models.GatewayRequestLog{}).Where("site_id = ?", keep.ID).Count(&leftoverCount).Error; err != nil {
		t.Fatalf("count reassigned request logs: %v", err)
	}
	if leftoverCount != 3 {
		t.Fatalf("reassigned request logs = %d", leftoverCount)
	}
	var storedLog models.GatewayRequestLog
	if err := db.Where("request_id = ?", "req-duplicate").First(&storedLog).Error; err != nil {
		t.Fatalf("reload request log: %v", err)
	}
	if storedLog.RouteStateID == nil || *storedLog.RouteStateID != keepRoute.ID {
		t.Fatalf("reassigned route_state_id = %v, want %d", storedLog.RouteStateID, keepRoute.ID)
	}
	var mismatchedLog models.GatewayRequestLog
	if err := db.Where("request_id = ?", "req-mismatched").First(&mismatchedLog).Error; err != nil {
		t.Fatalf("reload mismatched request log: %v", err)
	}
	if mismatchedLog.RouteStateID != nil {
		t.Fatalf("mismatched route_state_id = %v, want nil", *mismatchedLog.RouteStateID)
	}
	var unmappedLog models.GatewayRequestLog
	if err := db.Where("request_id = ?", "req-unmapped").First(&unmappedLog).Error; err != nil {
		t.Fatalf("reload unmapped request log: %v", err)
	}
	if unmappedLog.RouteStateID != nil {
		t.Fatalf("unmapped route_state_id = %v, want nil", *unmappedLog.RouteStateID)
	}
	var sessionCount int64
	if err := db.Model(&models.ChatSession{}).Where("site_id = ?", duplicate.ID).Count(&sessionCount).Error; err != nil {
		t.Fatalf("count chat sessions: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("leftover chat sessions = %d", sessionCount)
	}
	if err := db.Model(&models.ChatSession{}).Where("site_id = ?", keep.ID).Count(&sessionCount).Error; err != nil {
		t.Fatalf("count reassigned chat sessions: %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("reassigned chat sessions = %d", sessionCount)
	}
}

func TestReassignDuplicateRouteLogReferencesChunksLargeRouteMaps(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:duplicate-route-log-chunks?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	routeIDMap := map[uint]uint{}
	for index := 0; index < duplicateMergeReferenceBatchSize+25; index++ {
		fingerprint := fmt.Sprintf("fp-%03d", index)
		kept := models.GatewayRouteState{SiteID: 1, KeyFingerprint: fingerprint}
		removed := models.GatewayRouteState{SiteID: 2, KeyFingerprint: fingerprint}
		if err := db.Create(&kept).Error; err != nil {
			t.Fatalf("create kept route: %v", err)
		}
		if err := db.Create(&removed).Error; err != nil {
			t.Fatalf("create removed route: %v", err)
		}
		routeIDMap[removed.ID] = kept.ID
		siteID := uint(2)
		if err := db.Create(&models.GatewayRequestLog{
			RouteStateID: &removed.ID,
			SiteID:       &siteID,
			RequestID:    fmt.Sprintf("req-%03d", index),
			CreatedAt:    time.Now().UTC(),
		}).Error; err != nil {
			t.Fatalf("create request log: %v", err)
		}
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return reassignDuplicateRouteLogReferences(tx, routeIDMap)
	}); err != nil {
		t.Fatalf("reassign route references: %v", err)
	}
	var staleCount int64
	if err := db.Model(&models.GatewayRequestLog{}).
		Where("route_state_id IN ?", sortedUintKeys(routeIDMap)).
		Count(&staleCount).Error; err != nil {
		t.Fatalf("count stale references: %v", err)
	}
	if staleCount != 0 {
		t.Fatalf("stale removed route references = %d", staleCount)
	}
	var movedCount int64
	if err := db.Model(&models.GatewayRequestLog{}).
		Where("route_state_id IN ?", sortedUintKeys(invertUintMap(routeIDMap))).
		Count(&movedCount).Error; err != nil {
		t.Fatalf("count moved references: %v", err)
	}
	if movedCount != int64(len(routeIDMap)) {
		t.Fatalf("moved references = %d, want %d", movedCount, len(routeIDMap))
	}
}

func invertUintMap(values map[uint]uint) map[uint]uint {
	out := make(map[uint]uint, len(values))
	for key, value := range values {
		out[value] = key
	}
	return out
}
