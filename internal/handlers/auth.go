package handlers

import (
	"errors"
	"net/http"
	"strings"

	"ai-sign-in-gateway/internal/httpx"
	"ai-sign-in-gateway/internal/middleware"
	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/schemas"
	"ai-sign-in-gateway/internal/security"
	"gorm.io/gorm"
)

func (a *App) Login(w http.ResponseWriter, r *http.Request) {
	var payload schemas.LoginRequest
	if err := httpx.Decode(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	var admin models.AdminUser
	err := a.DB.Where("username = ?", payload.Username).First(&admin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || !security.VerifyPassword(payload.Password, admin.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	token, err := security.CreateAccessToken(a.Cfg, admin.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, schemas.TokenResponse{AccessToken: token, TokenType: "bearer"})
}

func (a *App) Me(w http.ResponseWriter, r *http.Request) {
	admin := middleware.CurrentAdmin(r)
	if admin == nil {
		writeError(w, http.StatusUnauthorized, "登录状态失效，请重新登录。")
		return
	}
	writeJSON(w, http.StatusOK, schemas.AdminUserResponse{ID: admin.ID, Username: admin.Username})
}

func (a *App) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	admin := middleware.CurrentAdmin(r)
	if admin == nil {
		writeError(w, http.StatusUnauthorized, "登录状态失效，请重新登录。")
		return
	}

	var payload schemas.AdminAccountUpdateRequest
	if err := httpx.Decode(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if !security.VerifyPassword(payload.CurrentPassword, admin.PasswordHash) {
		writeError(w, http.StatusBadRequest, "当前密码不正确")
		return
	}

	newUsername := strings.TrimSpace(payload.NewUsername)
	newPassword := payload.NewPassword
	if newUsername == "" && newPassword == "" {
		writeError(w, http.StatusBadRequest, "请至少修改用户名或密码中的一项")
		return
	}
	if newUsername != "" && len(newUsername) > 50 {
		writeError(w, http.StatusBadRequest, "用户名长度不能超过 50 个字符")
		return
	}
	if newPassword != "" && (len(newPassword) < 6 || len(newPassword) > 128) {
		writeError(w, http.StatusBadRequest, "新密码长度需在 6-128 之间")
		return
	}

	var fresh models.AdminUser
	if err := a.DB.First(&fresh, admin.ID).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if newUsername != "" && newUsername != fresh.Username {
		var existing models.AdminUser
		err := a.DB.Where("username = ? AND id <> ?", newUsername, fresh.ID).First(&existing).Error
		if err == nil {
			writeError(w, http.StatusBadRequest, "该用户名已被占用")
			return
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		fresh.Username = newUsername
	}

	if newPassword != "" {
		if security.VerifyPassword(newPassword, fresh.PasswordHash) {
			writeError(w, http.StatusBadRequest, "新密码不能与当前密码相同")
			return
		}
		hashed, err := security.HashPassword(newPassword)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		fresh.PasswordHash = hashed
	}

	if err := a.DB.Save(&fresh).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	token, err := security.CreateAccessToken(a.Cfg, fresh.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, schemas.AdminAccountUpdateResponse{
		User:        schemas.AdminUserResponse{ID: fresh.ID, Username: fresh.Username},
		AccessToken: token,
		TokenType:   "bearer",
	})
}
