package handlers

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-sign-in-gateway/internal/config"
	"ai-sign-in-gateway/internal/database"
	"ai-sign-in-gateway/internal/httpx"
	"ai-sign-in-gateway/internal/migrations"
	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/runtimecontrol"
	"ai-sign-in-gateway/internal/schemas"
	"ai-sign-in-gateway/internal/services"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func (a *App) SettingsRoutes(r chi.Router) {
	r.Get("/", a.GetSettings)
	r.Put("/", a.UpdateSettings)
	r.Post("/scheduler/run-now", a.RunSchedulerNow)
	r.Post("/runtime/open-url", a.OpenRuntimeURL)
	r.Post("/runtime/stop-stale-ports", a.StopStaleRuntimePorts)
	r.Post("/runtime/config-dir", a.SetRuntimeConfigDir)
	r.Post("/runtime/database", a.ImportRuntimeDatabase)
	r.Get("/runtime/database/backups", a.ListRuntimeDatabaseBackups)
	r.Post("/runtime/database/backups", a.BackupRuntimeDatabaseNow)
	r.Get("/runtime/database/backups/{name}/download", a.DownloadRuntimeDatabaseBackup)
	r.Delete("/runtime/database/backups/{name}", a.DeleteRuntimeDatabaseBackup)
	r.Get("/runtime/config-dir/archive", a.DownloadRuntimeConfigArchive)
}

func (a *App) systemSettings() (models.SystemSetting, error) {
	var settings models.SystemSetting
	err := a.DB.First(&settings, 1).Error
	return settings, err
}

func (a *App) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := a.systemSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse(settings))
}

func (a *App) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var payload schemas.SettingsUpdate
	if err := httpx.Decode(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	settings, err := a.systemSettings()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		settings.ID = 1
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	settings.Timezone = payload.Timezone
	settings.ScheduleEnabled = payload.ScheduleEnabled
	settings.DailyRunTime = payload.DailyRunTime
	settings.CheckinConcurrency = payload.CheckinConcurrency
	settings.CheckinGlobalConcurrency = payload.CheckinGlobalConcurrency
	settings.CheckinIntervalSeconds = payload.CheckinIntervalSeconds
	settings.RetryCount = payload.RetryCount
	settings.RequestTimeout = clampInt(payload.RequestTimeout, 5, 120, 20)
	settings.OnlyEnabledSites = payload.OnlyEnabledSites
	settings.DesktopKeepRunning = payload.DesktopKeepRunning
	settings.DatabaseBackupEnabled = payload.DatabaseBackupEnabled
	databaseBackupDir, err := normalizeRuntimeBackupDirInput(payload.DatabaseBackupDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, "数据库备份目录无效: "+err.Error())
		return
	}
	settings.DatabaseBackupDir = databaseBackupDir
	settings.DatabaseBackupIntervalMinutes = clampInt(payload.DatabaseBackupIntervalMinutes, 5, 10080, 1440)
	settings.DatabaseBackupRetention = clampInt(payload.DatabaseBackupRetention, 1, 365, 7)
	settings.LogRetentionDays = clampInt(payload.LogRetentionDays, 1, 365, services.DefaultLogRetentionDays)
	activePricingSchemeID, customPricingSchemes := services.NormalizeGatewayPricingSettings(payload.GatewayPricingActiveSchemeID, payload.GatewayPricingSchemes)
	settings.GatewayPricingActiveSchemeID = activePricingSchemeID
	settings.GatewayPricingSchemes = services.EncodeGatewayPricingSchemes(customPricingSchemes)
	settings.FeatureFlags = normalizeFeatureFlags(payload.FeatureFlags)
	if err := a.DB.Save(&settings).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result, err := services.CleanupOldLogs(a.DB, settings.LogRetentionDays, time.Now().UTC()); err != nil {
		log.Printf("日志清理失败: %v", err)
	} else if result.TotalDeleted() > 0 {
		log.Printf("日志清理: 已删除 %d 条旧日志，保留 %d 天", result.TotalDeleted(), result.RetentionDays)
	}
	writeJSON(w, http.StatusOK, settingsResponse(settings))
}

func (a *App) RunSchedulerNow(w http.ResponseWriter, r *http.Request) {
	settings, err := a.checkinBatchSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	runs, err := a.runCheckinBatch(manualCheckinContext(r.Context()), nil, settings.OnlyEnabledSites, "manual", settings)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	successCount, failedCount := checkinRunStatusCounts(runs)
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": fmt.Sprintf("已执行一次签到：成功 %d，失败 %d。", successCount, failedCount),
	})
}

func (a *App) OpenRuntimeURL(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		URL string `json:"url"`
	}
	if err := httpx.Decode(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	target := strings.TrimSpace(payload.URL)
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		writeError(w, http.StatusBadRequest, "URL 无效")
		return
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		writeError(w, http.StatusBadRequest, "只支持打开 http/https 地址")
		return
	}
	if err := runtimecontrol.OpenURL(target); err != nil {
		writeError(w, http.StatusInternalServerError, "打开系统浏览器失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "已交给系统浏览器打开。"})
}

func (a *App) StopStaleRuntimePorts(w http.ResponseWriter, r *http.Request) {
	info := GetRuntimeInfo()
	currentPorts := map[int]bool{}
	if info.FrontendPort > 0 {
		currentPorts[info.FrontendPort] = true
	}
	if info.BackendPort > 0 {
		currentPorts[info.BackendPort] = true
	}
	rawResults := runtimecontrol.StopAppProcessesOnPorts([]int{
		info.FrontendDefaultPort,
		info.BackendDefaultPort,
	}, currentPorts, "ai-sign-in-gateway")

	results := make([]schemas.RuntimeStopPortResult, 0, len(rawResults))
	for _, result := range rawResults {
		results = append(results, schemas.RuntimeStopPortResult{
			Port:    result.Port,
			PID:     result.PID,
			Command: result.Command,
			Stopped: result.Stopped,
			Skipped: result.Skipped,
			Message: result.Message,
		})
	}
	refreshRuntimePortOccupants()
	writeJSON(w, http.StatusOK, schemas.RuntimeStopStalePortsResponse{Results: results})
}

func (a *App) SetRuntimeConfigDir(w http.ResponseWriter, r *http.Request) {
	var payload schemas.RuntimeConfigDirRequest
	if err := httpx.Decode(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	configDir := strings.TrimSpace(payload.ConfigDir)
	if configDir == "" {
		writeError(w, http.StatusBadRequest, "配置目录不能为空")
		return
	}
	activeDir, err := config.SetActiveConfigDir(configDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, "保存配置目录失败: "+err.Error())
		return
	}
	info := GetRuntimeInfo()
	info.PendingConfigDir = activeDir
	SetRuntimeInfo(info)
	writeJSON(w, http.StatusOK, schemas.RuntimeConfigDirResponse{
		ConfigDir:       activeDir,
		DatabasePath:    filepath.Join(activeDir, "ai-sign-in-gateway.db"),
		RestartRequired: true,
		Message:         "已保存新的配置目录。重启程序后将只加载该目录中的数据库与配置，不会复制或覆盖原数据。",
	})
}

func (a *App) ImportRuntimeDatabase(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		a.importRuntimeDatabaseUpload(w, r)
		return
	}

	var payload schemas.RuntimeDatabaseImportRequest
	if err := httpx.Decode(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	sourcePath, err := normalizeLocalFilePath(payload.DatabasePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "数据库文件路径无效: "+err.Error())
		return
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "数据库文件不存在: "+err.Error())
		return
	}
	if sourceInfo.IsDir() || !sourceInfo.Mode().IsRegular() {
		writeError(w, http.StatusBadRequest, "请选择一个 SQLite 数据库文件")
		return
	}
	if err := validateSQLiteDatabaseFile(sourcePath); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.importRuntimeDatabaseFromFile(w, sourcePath, true)
}

func (a *App) ListRuntimeDatabaseBackups(w http.ResponseWriter, r *http.Request) {
	backupDir, err := a.runtimeBackupDir()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	backups, err := services.ListDatabaseBackups(backupDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取备份列表失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, schemas.RuntimeDatabaseBackupsResponse{
		BackupDir: backupDir,
		Backups:   databaseBackupFilesResponse(backups),
	})
}

func (a *App) BackupRuntimeDatabaseNow(w http.ResponseWriter, r *http.Request) {
	backupDir, err := a.runtimeBackupDir()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	databasePath, err := runtimeDatabasePath()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	settings, err := a.systemSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	backup, err := (services.DatabaseBackupRunner{DatabasePath: databasePath}).CreateBackupTo(backupDir, nonZeroInt(settings.DatabaseBackupRetention, 7))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "创建数据库备份失败: "+err.Error())
		return
	}
	backups, err := services.ListDatabaseBackups(backupDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取备份列表失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, schemas.RuntimeDatabaseBackupNowResponse{
		Backup:    databaseBackupFileResponse(backup),
		BackupDir: backupDir,
		Backups:   databaseBackupFilesResponse(backups),
		Message:   "数据库备份已创建。",
	})
}

func (a *App) DeleteRuntimeDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	backupDir, err := a.runtimeBackupDir()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := chi.URLParam(r, "name")
	if err := services.DeleteDatabaseBackup(backupDir, name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "备份文件不存在")
			return
		}
		writeError(w, http.StatusBadRequest, "删除备份失败: "+err.Error())
		return
	}
	backups, err := services.ListDatabaseBackups(backupDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取备份列表失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, schemas.RuntimeDatabaseBackupsResponse{
		BackupDir: backupDir,
		Backups:   databaseBackupFilesResponse(backups),
	})
}

func (a *App) DownloadRuntimeDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	backupDir, err := a.runtimeBackupDir()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	backup, err := services.DatabaseBackupFileByName(backupDir, chi.URLParam(r, "name"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "备份文件不存在")
			return
		}
		writeError(w, http.StatusBadRequest, "备份文件无效: "+err.Error())
		return
	}
	w.Header().Set("Content-Disposition", contentDispositionAttachment(backup.Name))
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	http.ServeFile(w, r, backup.Path)
}

func (a *App) DownloadRuntimeConfigArchive(w http.ResponseWriter, r *http.Request) {
	configDir, err := runtimeConfigDir()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	info, err := os.Stat(configDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取配置目录失败: "+err.Error())
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "当前配置路径不是目录")
		return
	}

	filename := fmt.Sprintf("ai-sign-in-gateway-config-%s.zip", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Disposition", contentDispositionAttachment(filename))
	w.Header().Set("Content-Type", "application/zip")
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	if err := writeConfigArchive(zipWriter, configDir); err != nil {
		// zip 流一旦开始写入就不能再可靠地改写 HTTP 状态，记录到归档注释中供客户端排查。
		zipWriter.SetComment("配置文件打包未完整完成: " + err.Error())
	}
}

func (a *App) importRuntimeDatabaseUpload(w http.ResponseWriter, r *http.Request) {
	const maxUploadSize = 512 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "读取上传数据库失败: "+err.Error())
		return
	}
	file, header, err := r.FormFile("database")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择数据库文件")
		return
	}
	defer file.Close()
	if header == nil || header.Size <= 0 {
		writeError(w, http.StatusBadRequest, "上传的数据库文件为空")
		return
	}

	tmp, err := os.CreateTemp("", "ai-sign-in-gateway-db-upload-*.db")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "创建临时文件失败: "+err.Error())
		return
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := io.Copy(tmp, file); err != nil {
		_ = tmp.Close()
		writeError(w, http.StatusInternalServerError, "保存上传数据库失败: "+err.Error())
		return
	}
	if err := tmp.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "保存上传数据库失败: "+err.Error())
		return
	}
	if err := validateSQLiteDatabaseFile(tmpPath); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.importRuntimeDatabaseFromFile(w, tmpPath, false)
}

func (a *App) importRuntimeDatabaseFromFile(w http.ResponseWriter, sourcePath string, rejectSameFile bool) {
	targetPath, err := runtimeDatabasePath()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rejectSameFile && sameFile(sourcePath, targetPath) {
		writeError(w, http.StatusBadRequest, "源数据库已经是当前配置目录中的数据库")
		return
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "创建配置目录失败: "+err.Error())
		return
	}

	backupPath := ""
	if _, err := os.Stat(targetPath); err == nil {
		backupPath = backupDatabasePath(targetPath)
		if err := copyFile(targetPath, backupPath, 0o600); err != nil {
			writeError(w, http.StatusInternalServerError, "备份当前数据库失败: "+err.Error())
			return
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, "读取当前数据库失败: "+err.Error())
		return
	}

	tmpPath := fmt.Sprintf("%s.importing-%d", targetPath, os.Getpid())
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if err := copyFile(sourcePath, tmpPath, 0o600); err != nil {
		writeError(w, http.StatusInternalServerError, "复制数据库失败: "+err.Error())
		return
	}
	if err := validateImportedDatabase(tmpPath); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		writeError(w, http.StatusInternalServerError, "替换配置目录数据库失败: "+err.Error())
		return
	}

	if err := a.reopenDatabase(targetPath); err != nil {
		writeError(w, http.StatusInternalServerError, "数据库已复制，但切换连接失败，请重启程序后重新登录: "+err.Error())
		return
	}

	info := GetRuntimeInfo()
	info.DatabasePath = targetPath
	SetRuntimeInfo(info)
	writeJSON(w, http.StatusOK, schemas.RuntimeDatabaseImportResponse{
		DatabasePath:    targetPath,
		BackupPath:      backupPath,
		ReloginRequired: true,
		RestartRequired: false,
		Message:         "数据库已复制到当前配置目录，请重新登录后生效。",
	})
}

func settingsResponse(settings models.SystemSetting) schemas.SettingsResponse {
	info := GetRuntimeInfo()
	if strings.TrimSpace(info.ConfigDir) == "" {
		if configDir, err := config.UserConfigDir(); err == nil {
			info.ConfigDir = configDir
		}
	}
	if strings.TrimSpace(info.DefaultConfigDir) == "" {
		if defaultConfigDir, err := config.DefaultConfigDir(); err == nil {
			info.DefaultConfigDir = defaultConfigDir
		}
	}
	if strings.TrimSpace(info.DatabasePath) == "" && strings.TrimSpace(info.ConfigDir) != "" {
		info.DatabasePath = filepath.Join(info.ConfigDir, "ai-sign-in-gateway.db")
	}
	databaseBackupDir := settings.DatabaseBackupDir
	if strings.TrimSpace(info.DatabasePath) != "" {
		if resolvedDir, err := (services.DatabaseBackupRunner{DatabasePath: info.DatabasePath}).ResolveBackupDir(settings.DatabaseBackupDir); err == nil {
			databaseBackupDir = resolvedDir
		}
	}
	return schemas.SettingsResponse{
		Timezone:                           settings.Timezone,
		ScheduleEnabled:                    settings.ScheduleEnabled,
		DailyRunTime:                       settings.DailyRunTime,
		CheckinConcurrency:                 settings.CheckinConcurrency,
		CheckinGlobalConcurrency:           settings.CheckinGlobalConcurrency,
		CheckinIntervalSeconds:             settings.CheckinIntervalSeconds,
		RetryCount:                         settings.RetryCount,
		RequestTimeout:                     settings.RequestTimeout,
		OnlyEnabledSites:                   settings.OnlyEnabledSites,
		DesktopKeepRunning:                 settings.DesktopKeepRunning,
		DatabaseBackupEnabled:              settings.DatabaseBackupEnabled,
		DatabaseBackupDir:                  databaseBackupDir,
		DatabaseBackupIntervalMinutes:      nonZeroInt(settings.DatabaseBackupIntervalMinutes, 1440),
		DatabaseBackupRetention:            nonZeroInt(settings.DatabaseBackupRetention, 7),
		LogRetentionDays:                   nonZeroInt(settings.LogRetentionDays, services.DefaultLogRetentionDays),
		GatewayPricingActiveSchemeID:       services.ResolveGatewayPricingScheme(settings.GatewayPricingActiveSchemeID, settings.GatewayPricingSchemes).ID,
		GatewayPricingSchemes:              services.GatewayPricingSchemesForResponse(settings.GatewayPricingSchemes),
		FeatureFlags:                       normalizeFeatureFlags(settings.FeatureFlags),
		Features:                           featureResponses(settings),
		DesktopFrontendDefaultPort:         info.FrontendDefaultPort,
		DesktopFrontendPort:                info.FrontendPort,
		DesktopFrontendURL:                 info.FrontendURL,
		DesktopFrontendDefaultPortOccupant: info.FrontendDefaultPortOccupant,
		DesktopBackendDefaultPort:          info.BackendDefaultPort,
		DesktopBackendPort:                 info.BackendPort,
		DesktopBackendURL:                  info.BackendURL,
		DesktopBackendDefaultPortOccupant:  info.BackendDefaultPortOccupant,
		DesktopGatewayURL:                  info.GatewayURL,
		RuntimeConfigDir:                   info.ConfigDir,
		RuntimeDefaultConfigDir:            info.DefaultConfigDir,
		RuntimeDatabasePath:                info.DatabasePath,
		RuntimePendingConfigDir:            info.PendingConfigDir,
		SecurityWarnings:                   securityWarnings(settings),
	}
}

func securityWarnings(settings models.SystemSetting) []string {
	warnings := []string{}
	if strings.TrimSpace(settings.GatewayAPIKey) == "" {
		warnings = append(warnings, "网关 API Key 未配置，公开网关当前已禁用。")
	}
	if strings.TrimSpace(settings.DatabaseBackupDir) == "" {
		warnings = append(warnings, "数据库备份目录未配置，建议启用本地备份以便回滚。")
	}
	return warnings
}

func (a *App) runtimeBackupDir() (string, error) {
	settings, err := a.systemSettings()
	if err != nil {
		return "", err
	}
	databasePath, err := runtimeDatabasePath()
	if err != nil {
		return "", err
	}
	return (services.DatabaseBackupRunner{DatabasePath: databasePath}).ResolveBackupDir(settings.DatabaseBackupDir)
}

func normalizeRuntimeBackupDirInput(path string) (string, error) {
	value := strings.TrimSpace(path)
	if value == "" {
		return "", nil
	}
	databasePath, err := runtimeDatabasePath()
	if err == nil {
		return (services.DatabaseBackupRunner{DatabasePath: databasePath}).ResolveBackupDir(value)
	}
	return services.NormalizeDatabaseBackupDir(value)
}

func databaseBackupFilesResponse(files []services.DatabaseBackupFile) []schemas.RuntimeDatabaseBackupFile {
	items := make([]schemas.RuntimeDatabaseBackupFile, 0, len(files))
	for _, file := range files {
		items = append(items, databaseBackupFileResponse(file))
	}
	return items
}

func databaseBackupFileResponse(file services.DatabaseBackupFile) schemas.RuntimeDatabaseBackupFile {
	return schemas.RuntimeDatabaseBackupFile{
		Name:      file.Name,
		Path:      file.Path,
		Size:      file.Size,
		CreatedAt: file.CreatedAt,
	}
}

func (a *App) reopenDatabase(databasePath string) error {
	cfg := a.Cfg
	cfg.DatabaseURL = "sqlite:///" + filepath.ToSlash(databasePath)
	newDB, err := database.Open(cfg)
	if err != nil {
		return err
	}
	if err := migrations.Apply(newDB); err != nil {
		_ = database.Close(newDB)
		return err
	}
	oldDB := a.DB
	a.DB = newDB
	if oldDB != nil {
		_ = database.Close(oldDB)
	}
	return nil
}

func runtimeDatabasePath() (string, error) {
	info := GetRuntimeInfo()
	if path := strings.TrimSpace(info.DatabasePath); path != "" {
		return filepath.Abs(filepath.Clean(path))
	}
	if path := strings.TrimSpace(info.ConfigDir); path != "" {
		return filepath.Abs(filepath.Join(path, "ai-sign-in-gateway.db"))
	}
	configDir, err := config.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(configDir, "ai-sign-in-gateway.db"))
}

func runtimeConfigDir() (string, error) {
	info := GetRuntimeInfo()
	if path := strings.TrimSpace(info.ConfigDir); path != "" {
		return filepath.Abs(filepath.Clean(path))
	}
	return config.UserConfigDir()
}

func normalizeLocalFilePath(path string) (string, error) {
	value := strings.TrimSpace(path)
	if value == "" {
		return "", os.ErrInvalid
	}
	if strings.HasPrefix(value, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		switch {
		case value == "~":
			value = home
		case strings.HasPrefix(value, "~/"):
			value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
		}
	}
	return filepath.Abs(filepath.Clean(value))
}

func validateSQLiteDatabaseFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	header := make([]byte, 16)
	if _, err := io.ReadFull(file, header); err != nil {
		return fmt.Errorf("无法读取数据库文件头: %w", err)
	}
	if string(header) != "SQLite format 3\x00" {
		return fmt.Errorf("不是有效的 SQLite 数据库文件")
	}
	return nil
}

func validateImportedDatabase(path string) error {
	cfg := config.Config{DatabaseURL: "sqlite:///" + filepath.ToSlash(path)}
	db, err := database.Open(cfg)
	if err != nil {
		return fmt.Errorf("无法打开导入数据库: %w", err)
	}
	defer database.Close(db)
	if err := migrations.Apply(db); err != nil {
		return fmt.Errorf("导入数据库迁移失败: %w", err)
	}
	var adminCount int64
	if err := db.Model(&models.AdminUser{}).Count(&adminCount).Error; err != nil {
		return fmt.Errorf("导入数据库缺少管理员表: %w", err)
	}
	if adminCount == 0 {
		return fmt.Errorf("导入数据库没有管理员账号，无法重新登录")
	}
	return nil
}

func sameFile(left string, right string) bool {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	if leftErr == nil && rightErr == nil {
		return os.SameFile(leftInfo, rightInfo)
	}
	leftAbs, leftAbsErr := filepath.Abs(filepath.Clean(left))
	rightAbs, rightAbsErr := filepath.Abs(filepath.Clean(right))
	return leftAbsErr == nil && rightAbsErr == nil && leftAbs == rightAbs
}

func backupDatabasePath(databasePath string) string {
	timestamp := time.Now().Format("20060102-150405")
	return fmt.Sprintf("%s.backup-%s", databasePath, timestamp)
}

func copyFile(source string, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func writeConfigArchive(zipWriter *zip.Writer, configDir string) error {
	root, err := filepath.Abs(filepath.Clean(configDir))
	if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
			return nil
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = rel
		header.Method = zip.Deflate
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func contentDispositionAttachment(filename string) string {
	cleanName := strings.ReplaceAll(filepath.Base(filename), `"`, "")
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, cleanName, url.PathEscape(cleanName))
}

func refreshRuntimePortOccupants() {
	info := GetRuntimeInfo()
	if info.FrontendDefaultPort > 0 {
		info.FrontendDefaultPortOccupant = runtimePortOwner(info.FrontendDefaultPort, info.FrontendPort)
	}
	if info.BackendDefaultPort > 0 {
		info.BackendDefaultPortOccupant = runtimePortOwner(info.BackendDefaultPort, info.BackendPort)
	}
	SetRuntimeInfo(info)
}

func runtimePortOwner(defaultPort int, currentPort int) string {
	if defaultPort == currentPort {
		return "当前程序"
	}
	if !runtimecontrol.IsPortOccupied("127.0.0.1", defaultPort) {
		return "未占用"
	}
	return runtimecontrol.DescribePortOccupant(defaultPort)
}

func clampInt(value int, min int, max int, fallback int) int {
	if value == 0 {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func nonZeroInt(value int, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
