package services

import (
	"testing"
	"time"

	"ai-sign-in-gateway/internal/models"
)

func TestCleanupOldLogsDeletesRowsOlderThanRetention(t *testing.T) {
	db := openBackupTestDB(t, t.TempDir()+"/logs.db")
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	oldTime := now.AddDate(0, 0, -6)
	recentTime := now.AddDate(0, 0, -2)

	runs := []models.CheckinRun{
		{Status: "success", Message: "old", StartedAt: oldTime},
		{Status: "success", Message: "recent", StartedAt: recentTime},
	}
	if err := db.Create(&runs).Error; err != nil {
		t.Fatalf("create checkin runs: %v", err)
	}
	logs := []models.GatewayRequestLog{
		{RequestID: "old", Method: "POST", CreatedAt: oldTime},
		{RequestID: "recent", Method: "POST", CreatedAt: recentTime},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatalf("create gateway logs: %v", err)
	}

	result, err := CleanupOldLogs(db, 5, now)
	if err != nil {
		t.Fatalf("cleanup logs: %v", err)
	}
	if result.RetentionDays != 5 || result.TotalDeleted() != 2 {
		t.Fatalf("cleanup result = %+v", result)
	}

	var checkinCount int64
	if err := db.Model(&models.CheckinRun{}).Count(&checkinCount).Error; err != nil {
		t.Fatal(err)
	}
	if checkinCount != 1 {
		t.Fatalf("checkin count = %d, want 1", checkinCount)
	}
	var gatewayCount int64
	if err := db.Model(&models.GatewayRequestLog{}).Count(&gatewayCount).Error; err != nil {
		t.Fatal(err)
	}
	if gatewayCount != 1 {
		t.Fatalf("gateway log count = %d, want 1", gatewayCount)
	}
}

func TestCleanupOldLogsUsesDefaultRetentionForInvalidValue(t *testing.T) {
	db := openBackupTestDB(t, t.TempDir()+"/default-retention.db")
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	if err := db.Create(&models.GatewayRequestLog{RequestID: "old", CreatedAt: now.AddDate(0, 0, -6)}).Error; err != nil {
		t.Fatalf("create gateway log: %v", err)
	}

	result, err := CleanupOldLogs(db, 0, now)
	if err != nil {
		t.Fatalf("cleanup logs: %v", err)
	}
	if result.RetentionDays != DefaultLogRetentionDays || result.GatewayRequestLogsDeleted != 1 {
		t.Fatalf("cleanup result = %+v", result)
	}
}
