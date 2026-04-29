package middleware

import (
	"context"
	"net/http"
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
			username, _ := claims["sub"].(string)
			var admin models.AdminUser
			db := dbProvider()
			if db == nil {
				httpx.Error(w, http.StatusUnauthorized, "登录状态失效，请重新登录。")
				return
			}
			if username == "" || db.Where("username = ?", username).First(&admin).Error != nil {
				httpx.Error(w, http.StatusUnauthorized, "登录状态失效，请重新登录。")
				return
			}
			ctx := context.WithValue(r.Context(), adminContextKey{}, &admin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
