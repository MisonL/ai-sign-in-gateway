package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ai-sign-in-gateway/internal/config"
	"ai-sign-in-gateway/internal/database"
	"ai-sign-in-gateway/internal/models"
	"github.com/glebarez/sqlite"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func TestGetSettingsIncludesRuntimeInfo(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	settings := models.SystemSetting{
		ID:                       1,
		Timezone:                 "Asia/Shanghai",
		ScheduleEnabled:          true,
		DailyRunTime:             "09:00",
		CheckinConcurrency:       1,
		CheckinGlobalConcurrency: 4,
		CheckinIntervalSeconds:   1,
		RetryCount:               1,
		RequestTimeout:           20,
		OnlyEnabledSites:         true,
		DesktopKeepRunning:       true,
	}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}

	SetRuntimeInfo(RuntimeInfo{
		FrontendURL:                 "http://127.0.0.1:3722",
		FrontendDefaultPort:         3721,
		FrontendPort:                3722,
		FrontendDefaultPortOccupant: "python3(pid:4321)",
		BackendURL:                  "http://127.0.0.1:8973",
		BackendDefaultPort:          8972,
		BackendPort:                 8973,
		BackendDefaultPortOccupant:  "ai-sign-in-gateway(pid:1234)",
		GatewayURL:                  "http://127.0.0.1:8973/api/gateway",
	})
	t.Cleanup(func() {
		SetRuntimeInfo(RuntimeInfo{})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	recorder := httptest.NewRecorder()
	app := &App{DB: db}

	app.GetSettings(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response["desktop_frontend_url"] != "http://127.0.0.1:3722" {
		t.Fatalf("desktop_frontend_url = %v", response["desktop_frontend_url"])
	}
	if response["desktop_backend_url"] != "http://127.0.0.1:8973" {
		t.Fatalf("desktop_backend_url = %v", response["desktop_backend_url"])
	}
	if response["desktop_gateway_url"] != "http://127.0.0.1:8973/api/gateway" {
		t.Fatalf("desktop_gateway_url = %v", response["desktop_gateway_url"])
	}
	if response["desktop_frontend_default_port_occupant"] != "python3(pid:4321)" {
		t.Fatalf("desktop_frontend_default_port_occupant = %v", response["desktop_frontend_default_port_occupant"])
	}
	if response["desktop_backend_default_port_occupant"] != "ai-sign-in-gateway(pid:1234)" {
		t.Fatalf("desktop_backend_default_port_occupant = %v", response["desktop_backend_default_port_occupant"])
	}
}

func TestImportRuntimeDatabaseCopiesAndReopensDB(t *testing.T) {
	tempDir := t.TempDir()
	currentPath := filepath.Join(tempDir, "current", "ai-sign-in-gateway.db")
	sourcePath := filepath.Join(tempDir, "source.db")

	currentDB := openTestSQLite(t, currentPath)
	if err := currentDB.Create(&models.AdminUser{Username: "old", PasswordHash: "old-hash"}).Error; err != nil {
		t.Fatalf("create current admin: %v", err)
	}
	sourceDB := openTestSQLite(t, sourcePath)
	if err := sourceDB.Create(&models.AdminUser{Username: "imported", PasswordHash: "imported-hash"}).Error; err != nil {
		t.Fatalf("create source admin: %v", err)
	}
	if err := database.Close(sourceDB); err != nil {
		t.Fatalf("close source db: %v", err)
	}

	SetRuntimeInfo(RuntimeInfo{
		ConfigDir:    filepath.Dir(currentPath),
		DatabasePath: currentPath,
	})
	t.Cleanup(func() {
		SetRuntimeInfo(RuntimeInfo{})
	})

	body := []byte(`{"database_path":` + strconvQuote(sourcePath) + `}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/runtime/database", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	app := &App{
		DB:  currentDB,
		Cfg: config.Config{DatabaseURL: "sqlite:///" + filepath.ToSlash(currentPath)},
	}

	app.ImportRuntimeDatabase(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var imported models.AdminUser
	if err := app.DB.Where("username = ?", "imported").First(&imported).Error; err != nil {
		t.Fatalf("imported admin not found after reopen: %v", err)
	}
	var oldCount int64
	if err := app.DB.Model(&models.AdminUser{}).Where("username = ?", "old").Count(&oldCount).Error; err != nil {
		t.Fatalf("count old admin: %v", err)
	}
	if oldCount != 0 {
		t.Fatalf("old admin count = %d", oldCount)
	}

	matches, err := filepath.Glob(currentPath + ".backup-*")
	if err != nil {
		t.Fatalf("glob backup: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("backup count = %d, want 1", len(matches))
	}
}

func TestImportRuntimeDatabaseUploadCopiesAndReopensDB(t *testing.T) {
	tempDir := t.TempDir()
	currentPath := filepath.Join(tempDir, "current", "ai-sign-in-gateway.db")
	sourcePath := filepath.Join(tempDir, "source.db")

	currentDB := openTestSQLite(t, currentPath)
	if err := currentDB.Create(&models.AdminUser{Username: "old", PasswordHash: "old-hash"}).Error; err != nil {
		t.Fatalf("create current admin: %v", err)
	}
	sourceDB := openTestSQLite(t, sourcePath)
	if err := sourceDB.Create(&models.AdminUser{Username: "uploaded", PasswordHash: "uploaded-hash"}).Error; err != nil {
		t.Fatalf("create source admin: %v", err)
	}
	if err := database.Close(sourceDB); err != nil {
		t.Fatalf("close source db: %v", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("database", "source.db")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	if _, err := io.Copy(part, sourceFile); err != nil {
		_ = sourceFile.Close()
		t.Fatalf("copy source to multipart: %v", err)
	}
	if err := sourceFile.Close(); err != nil {
		t.Fatalf("close source file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	SetRuntimeInfo(RuntimeInfo{
		ConfigDir:    filepath.Dir(currentPath),
		DatabasePath: currentPath,
	})
	t.Cleanup(func() {
		SetRuntimeInfo(RuntimeInfo{})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/settings/runtime/database", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	app := &App{
		DB:  currentDB,
		Cfg: config.Config{DatabaseURL: "sqlite:///" + filepath.ToSlash(currentPath)},
	}

	app.ImportRuntimeDatabase(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var uploaded models.AdminUser
	if err := app.DB.Where("username = ?", "uploaded").First(&uploaded).Error; err != nil {
		t.Fatalf("uploaded admin not found after reopen: %v", err)
	}
}

func TestRuntimeDatabaseBackupLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "data", "ai-sign-in-gateway.db")
	backupDir := filepath.Join(tempDir, "backups")
	db := openTestSQLite(t, dbPath)
	if err := db.Create(&models.AdminUser{Username: "admin", PasswordHash: "hash"}).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	settings := models.SystemSetting{
		ID:                            1,
		Timezone:                      "Asia/Shanghai",
		DatabaseBackupEnabled:         true,
		DatabaseBackupDir:             backupDir,
		DatabaseBackupIntervalMinutes: 1440,
		DatabaseBackupRetention:       7,
	}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	SetRuntimeInfo(RuntimeInfo{
		ConfigDir:    filepath.Dir(dbPath),
		DatabasePath: dbPath,
	})
	t.Cleanup(func() {
		SetRuntimeInfo(RuntimeInfo{})
	})

	app := &App{DB: db}
	createRecorder := httptest.NewRecorder()
	app.BackupRuntimeDatabaseNow(createRecorder, httptest.NewRequest(http.MethodPost, "/api/settings/runtime/database/backups", nil))
	if createRecorder.Code != http.StatusOK {
		t.Fatalf("create status = %d body = %s", createRecorder.Code, createRecorder.Body.String())
	}
	var createResponse struct {
		Backup struct {
			Name string `json:"name"`
		} `json:"backup"`
		Backups []any `json:"backups"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &createResponse); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResponse.Backup.Name == "" || len(createResponse.Backups) != 1 {
		t.Fatalf("unexpected create response: %+v", createResponse)
	}

	listRecorder := httptest.NewRecorder()
	app.ListRuntimeDatabaseBackups(listRecorder, httptest.NewRequest(http.MethodGet, "/api/settings/runtime/database/backups", nil))
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status = %d body = %s", listRecorder.Code, listRecorder.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/settings/runtime/database/backups/"+createResponse.Backup.Name, nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("name", createResponse.Backup.Name)
	deleteReq = deleteReq.WithContext(context.WithValue(deleteReq.Context(), chi.RouteCtxKey, routeCtx))
	deleteRecorder := httptest.NewRecorder()
	app.DeleteRuntimeDatabaseBackup(deleteRecorder, deleteReq)
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}
	if _, err := os.Stat(filepath.Join(backupDir, createResponse.Backup.Name)); !os.IsNotExist(err) {
		t.Fatalf("backup file still exists or stat failed unexpectedly: %v", err)
	}
}

func openTestSQLite(t *testing.T, path string) *gorm.DB {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir sqlite dir %s: %v", path, err)
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite %s: %v", path, err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("auto migrate %s: %v", path, err)
	}
	return db
}

func strconvQuote(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
