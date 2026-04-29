package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-sign-in-gateway/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDatabaseBackupRunnerBackupToCreatesSnapshotAndAppliesRetention(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "data", "ai-sign-in-gateway.db")
	backupDir := filepath.Join(tempDir, "backups")
	db := openBackupTestDB(t, dbPath)
	if err := db.Create(&models.AdminUser{Username: "admin", PasswordHash: "hash"}).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	oldBackup := filepath.Join(backupDir, "ai-sign-in-gateway-20000101-000000.db")
	if err := copyFixtureDB(dbPath, oldBackup); err != nil {
		t.Fatalf("copy old backup: %v", err)
	}
	oldTime := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(oldBackup, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old backup: %v", err)
	}

	runner := DatabaseBackupRunner{DatabasePath: dbPath}
	if err := runner.BackupTo(backupDir, 1); err != nil {
		t.Fatalf("backup: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(backupDir, "ai-sign-in-gateway-*.db"))
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("backup count = %d, want 1: %v", len(matches), matches)
	}
	if matches[0] == oldBackup {
		t.Fatalf("old backup was not pruned")
	}
}

func openBackupTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir db dir: %v", err)
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

func copyFixtureDB(source string, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o600)
}
