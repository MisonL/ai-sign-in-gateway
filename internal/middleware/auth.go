package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"ai-sign-in-gateway/internal/config"
	"ai-sign-in-gateway/internal/httpx"
	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/security"
	"gorm.io/gorm"
)

type adminContextKey struct{}

func CurrentAdmin(r *http.Request) *models.AdminUser {
	admin, _ := r.Context().Value(adminContextKey{}).(*models.AdminUser)
	return admin
}

func RequireAdmin(db *gorm.DB, cfg config.Config) func(http.Handler) http.Handler {
	return RequireAdminDynamic(func() *gorm.DB {
		return db
	}, cfg)
}

func RequireAdminDynamic(dbProvider func() *gorm.DB, cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				httpx.Error(w, http.StatusUnauthorized, "登录状态失效，请重新登录。")
				return
			}
			claims, err := security.DecodeAccessToken(cfg, strings.TrimPrefix(header, "Bearer "))
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "登录状态失效，请重新登录。")
				return
			}
			subject, _ := claims["sub"].(string)
			uid, _ := claims["uid"].(string)
			var admin models.AdminUser
			db := dbProvider()
			if db == nil {
				httpx.Error(w, http.StatusUnauthorized, "登录状态失效，请重新登录。")
				return
			}
			if findAdminForToken(db, uid, subject, &admin) != nil || !admin.IsEnabled {
				httpx.Error(w, http.StatusUnauthorized, "登录状态失效，请重新登录。")
				return
			}
			ctx := context.WithValue(r.Context(), adminContextKey{}, &admin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func findAdminForToken(db *gorm.DB, uid, subject string, admin *models.AdminUser) error {
	if strings.TrimSpace(uid) != "" {
		id, err := strconv.ParseUint(uid, 10, 64)
		if err != nil || id == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := db.First(admin, uint(id)).Error; err != nil {
			return err
		}
		if subject != "" && admin.Username != subject {
			return gorm.ErrRecordNotFound
		}
		return nil
	}
	if subject == "" {
		return gorm.ErrRecordNotFound
	}
	return db.Where("username = ?", subject).First(admin).Error
}
