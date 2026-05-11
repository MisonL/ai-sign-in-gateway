package plugins

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/schemas"
)

// APISupplier 对应 Python plugin_key=api-supplier。
// 只用于网关转发，不参与签到/资料同步。
type APISupplier struct{}

func NewAPISupplier() *APISupplier { return &APISupplier{} }

func (p *APISupplier) Meta() schemas.PluginMetaResponse {
	return schemas.PluginMetaResponse{
		Key:          "api-supplier",
		Name:         "API 供应商",
		Description:  "仅参与网关转发的供应商记录，不会触发站内登录或签到。",
		Capabilities: []string{"relay_only"},
		CredentialFields: []schemas.FieldDescriptor{
			Field("account", "账号标识", "text", "供应商备注 / 账号备注", false, ""),
			Field("api_key", "接口密钥", "password", "sk-...", true, ""),
			Field("user_agent", "浏览器标识", "text", "Mozilla/5.0 ...", false, ""),
		},
		ConfigFields: []schemas.FieldDescriptor{
			Field("api_format", "API 格式", "text", "codex / openai / anthropic / gemini / general", false, ""),
			Field("endpoint_url", "出口地址", "url", "https://example.com/v1", false, ""),
			Field("api_request_urls", "API 请求候选", "textarea", "https://api-a.example.com\nhttps://api-b.example.com/v1", false, "可填多个候选 URL，每行一个；网关请求会按顺序尝试。"),
			Field("image_generation_path", "图片生成 Path", "text", "/images/generations", false, "可选。纯文本生图接口路径，留空默认 /images/generations。"),
			Field("image_edit_path", "图片编辑 Path", "text", "/images/edits", false, "可选。参考图编辑/融合接口路径，留空默认 /images/edits。"),
			Field("preferred_model", "默认模型", "text", "claude-sonnet-4-6 / gemini-2.5-pro", false, ""),
		},
		AuthEntryLabel: "打开供应商站点",
		AuthHint:       "适合仅需要统一管理 API Key 和出口地址的供应商配置；API Key 仅用于模型请求，不参与站内登录、资料同步或签到。",
	}
}

func (p *APISupplier) Validate(site models.Site) error {
	if strings.TrimSpace(site.BaseURL) == "" {
		return errors.New("Base URL 不能为空")
	}
	if strings.TrimSpace(stringValue(site.Credentials, "api_key", "")) == "" {
		return errors.New("API Key 不能为空")
	}
	return nil
}

func (p *APISupplier) FetchAccountStatus(ctx context.Context, site models.Site, timeoutSeconds int) (AccountStatus, error) {
	endpoint := strings.TrimSpace(stringValue(site.PluginConfig, "endpoint_url", ""))
	if endpoint == "" {
		endpoint = strings.TrimSpace(site.BaseURL)
	}
	account := strings.TrimSpace(stringValue(site.Credentials, "account", ""))
	if account == "" {
		account = strings.TrimSpace(site.Name)
	}
	message := "供应商记录不参与站内登录或资料同步，请在网关中心或模型连通性页面验证。"
	if endpoint != "" {
		message = fmt.Sprintf("%s 当前出口：%s", message, endpoint)
	}
	var accountName *string
	if account != "" {
		accountName = &account
	}
	return AccountStatus{
		LoggedIn:    false,
		Message:     message,
		AccountName: accountName,
	}, nil
}

func (p *APISupplier) Checkin(ctx context.Context, site models.Site, timeoutSeconds int) (CheckinResult, error) {
	return CheckinResult{
		Success: false,
		Message: "供应商记录不支持签到。",
	}, nil
}
