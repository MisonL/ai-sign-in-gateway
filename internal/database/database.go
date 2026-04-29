package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-sign-in-gateway/internal/config"
	"ai-sign-in-gateway/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(cfg config.Config) (*gorm.DB, error) {
	dsn, err := sqliteDSN(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
		}),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(models.All()...)
}

func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func sqliteDSN(databaseURL string) (string, error) {
	const prefix = "sqlite:///"
	if strings.HasPrefix(databaseURL, prefix) {
		path := filepath.FromSlash(strings.TrimPrefix(databaseURL, prefix))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		return path, nil
	}
	if strings.HasPrefix(databaseURL, "sqlite://") {
		return "", fmt.Errorf("unsupported sqlite URL %q; expected sqlite:////absolute/path or sqlite:///relative/path", databaseURL)
	}
	if databaseURL == "" {
		return "", fmt.Errorf("database URL is empty")
	}
	if err := os.MkdirAll(filepath.Dir(databaseURL), 0o755); err != nil {
		return "", err
	}
	return databaseURL, nil
}
