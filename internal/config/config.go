package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultAppDir = ".ai-sign-in-gateway"
const launcherConfigFile = "launcher.json"
const envConfigDir = "AI_SIGN_IN_GATEWAY_CONFIG_DIR"

type LauncherConfig struct {
	ActiveConfigDir string `json:"active_config_dir"`
}

type Config struct {
	AppName                      string
	SecretKey                    string
	GatewayAPIKey                string
	Algorithm                    string
	AccessTokenExpireMinutes     int
	DatabaseURL                  string
	DefaultAdminUsername         string
	DefaultAdminPassword         string
	SchedulerTimezone            string
	CORSOrigins                  []string
	ManagedBrowserProfileRoot    string
	ManagedBrowserHeadless       bool
	ManagedBrowserTimeoutSeconds int
	ManagedBrowserSettleMS       int
}

func Load() (Config, error) {
	userConfigDir, err := UserConfigDir()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		AppName:                      getEnv("APP_NAME", "爱签网关"),
		SecretKey:                    getEnv("SECRET_KEY", "change-me-in-production-at-least-32-bytes"),
		GatewayAPIKey:                getEnv("GATEWAY_API_KEY", ""),
		Algorithm:                    getEnv("ALGORITHM", "HS256"),
		AccessTokenExpireMinutes:     getEnvInt("ACCESS_TOKEN_EXPIRE_MINUTES", 60*24),
		DatabaseURL:                  strings.TrimSpace(os.Getenv("DATABASE_URL")),
		DefaultAdminUsername:         getEnv("DEFAULT_ADMIN_USERNAME", "admin"),
		DefaultAdminPassword:         getEnv("DEFAULT_ADMIN_PASSWORD", "admin123"),
		SchedulerTimezone:            getEnv("SCHEDULER_TIMEZONE", "Asia/Shanghai"),
		CORSOrigins:                  splitCSV(getEnv("CORS_ORIGINS", "http://localhost:3721,http://127.0.0.1:3721")),
		ManagedBrowserProfileRoot:    strings.TrimSpace(os.Getenv("MANAGED_BROWSER_PROFILE_ROOT")),
		ManagedBrowserHeadless:       getEnvBool("MANAGED_BROWSER_HEADLESS", false),
		ManagedBrowserTimeoutSeconds: getEnvInt("MANAGED_BROWSER_TIMEOUT_SECONDS", 20),
		ManagedBrowserSettleMS:       getEnvInt("MANAGED_BROWSER_SETTLE_MS", 1200),
	}
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = "sqlite:///" + filepath.ToSlash(filepath.Join(userConfigDir, "ai-sign-in-gateway.db"))
	}
	if cfg.ManagedBrowserProfileRoot == "" {
		cfg.ManagedBrowserProfileRoot = filepath.Join(userConfigDir, "browser-profiles")
	}
	return cfg, nil
}

func UserConfigDir() (string, error) {
	if envDir := strings.TrimSpace(os.Getenv(envConfigDir)); envDir != "" {
		path, err := normalizeDir(envDir)
		if err != nil {
			return "", err
		}
		return path, os.MkdirAll(path, 0o755)
	}
	if activeDir, err := ActiveConfigDir(); err == nil && activeDir != "" {
		return activeDir, os.MkdirAll(activeDir, 0o755)
	}
	return DefaultConfigDir()
}

func DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, defaultAppDir)
	return path, os.MkdirAll(path, 0o755)
}

func ActiveConfigDir() (string, error) {
	defaultDir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	cfg, err := ReadLauncherConfig()
	if err != nil {
		return defaultDir, nil
	}
	if strings.TrimSpace(cfg.ActiveConfigDir) == "" {
		return defaultDir, nil
	}
	activeDir, err := normalizeDir(cfg.ActiveConfigDir)
	if err != nil {
		return "", err
	}
	return activeDir, nil
}

func ReadLauncherConfig() (LauncherConfig, error) {
	defaultDir, err := DefaultConfigDir()
	if err != nil {
		return LauncherConfig{}, err
	}
	data, err := os.ReadFile(filepath.Join(defaultDir, launcherConfigFile))
	if err != nil {
		return LauncherConfig{}, err
	}
	var cfg LauncherConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return LauncherConfig{}, err
	}
	return cfg, nil
}

func SetActiveConfigDir(path string) (string, error) {
	activeDir, err := normalizeDir(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(activeDir, 0o755); err != nil {
		return "", err
	}
	defaultDir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(LauncherConfig{ActiveConfigDir: activeDir}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(defaultDir, launcherConfigFile), data, 0o600); err != nil {
		return "", err
	}
	return activeDir, nil
}

func (c Config) SQLitePath() string {
	const prefix = "sqlite:///"
	if strings.HasPrefix(c.DatabaseURL, prefix) {
		return filepath.FromSlash(strings.TrimPrefix(c.DatabaseURL, prefix))
	}
	return c.DatabaseURL
}

func normalizeDir(path string) (string, error) {
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

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			items = append(items, item)
		}
	}
	return items
}
