package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/schemas"
	"ai-sign-in-gateway/internal/services"
)

// Sub2API 实现 sub2api 平台的登录 / 状态 / 签到 / API key 同步流程。
type Sub2API struct {
	client *http.Client
}

func NewSub2API() *Sub2API {
	return &Sub2API{client: &http.Client{}}
}

func (p *Sub2API) Meta() schemas.PluginMetaResponse {
	return schemas.PluginMetaResponse{
		Key:            "sub2api-platform",
		Name:           "sub2api 平台",
		Description:    "适配 Wei-Shaw/sub2api 平台的登录、资料读取、API Key 同步与可选签到。",
		Capabilities:   []string{"login", "checkin", "account_status", "api_key_sync", "token_refresh"},
		AuthEntryLabel: "打开 sub2api 登录页",
		AuthHint:       "支持 access_token 直填或 email/password 自动登录；api_key 仅用于模型调用。",
		CredentialFields: []schemas.FieldDescriptor{
			Field("account", "账号备注", "text", "sub2api-main", false, ""),
			Field("email", "登录邮箱", "text", "name@example.com", false, ""),
			Field("password", "登录密码", "password", "用于自动登录", false, ""),
			Field("access_token", "Access Token", "textarea", "Bearer access token", false, ""),
			Field("refresh_token", "Refresh Token", "textarea", "refresh token", false, ""),
			Field("api_key", "主 API Key", "password", "同步出的首选 API Key", false, ""),
			Field("user_agent", "User-Agent", "text", services.DefaultBrowserUserAgent, false, ""),
			Field("totp_secret", "TOTP Secret", "textarea", "JBSWY3DPEHPK3PXP", false, ""),
			Field("totp_otpauth_url", "otpauth 链接", "textarea", "otpauth://totp/...", false, ""),
		},
		ConfigFields: []schemas.FieldDescriptor{
			Field("turnstile_token", "Turnstile Token", "text", "可留空", false, ""),
			Field("totp_field_name", "TOTP 字段名", "text", "code", false, ""),
			Field("preferred_api_key_name", "首选 API Key 名称", "text", "default", false, ""),
			Field("preferred_api_key_id", "首选 API Key ID", "text", "例如 12", false, ""),
			Field("api_keys_url", "API Key 列表 URL", "url", "/api/v1/keys?page=1&page_size=100", false, ""),
			Field("invite_path", "邀请接口路径", "url", "/api/v1/user/referral?timezone=Asia%2FShanghai", false, "填写后会额外请求该接口解析邀请链接；留空时默认尝试 sub2api 常见 referral 接口。"),
			Field("invite_method", "邀请接口方法", "text", "GET", false, ""),
			Field("invite_body_json", "邀请接口 JSON Body", "textarea", `{"refresh": false}`, false, ""),
			Field("checkin_url", "签到 URL（可选）", "url", "/api/v1/user/checkin", false, ""),
			Field("disable_checkin", "关闭签到", "text", "填 1 关闭", false, ""),
			Field("status_invite_link_path", "邀请链接字段路径", "text", "data.invite_link", false, ""),
			Field("status_invite_code_path", "邀请码字段路径", "text", "data.aff_code", false, ""),
			Field("invite_link_path", "邀请接口链接字段路径", "text", "data.referral.link", false, ""),
			Field("invite_code_path", "邀请接口邀请码字段路径", "text", "data.referral.code", false, ""),
			Field("invite_link_template", "邀请链接模板", "text", "/register?aff={code}", false, "支持相对路径或完整 URL，使用 {code} 作为邀请码占位符；部分 sub2api 站点会使用 /register?ref={code}。"),
		},
	}
}

func (p *Sub2API) Validate(site models.Site) error {
	access := strings.TrimSpace(stringValue(site.Credentials, "access_token", ""))
	email := strings.TrimSpace(stringValue(site.Credentials, "email", ""))
	password := strings.TrimSpace(stringValue(site.Credentials, "password", ""))
	if access == "" && (email == "" || password == "") {
		return errors.New("请填写 access_token，或同时填写 email 和 password。")
	}
	if strings.TrimSpace(site.BaseURL) == "" {
		return errors.New("Base URL 不能为空")
	}
	return nil
}

type sub2apiAuth struct {
	AccessToken  string
	RefreshToken string
}

func (p *Sub2API) FetchAccountStatus(ctx context.Context, site models.Site, timeoutSeconds int) (AccountStatus, error) {
	if err := p.Validate(site); err != nil {
		return AccountStatus{}, err
	}
	auth, err := p.authContext(ctx, site, timeoutSeconds)
	if err != nil {
		return AccountStatus{}, err
	}

	profile, _, nextAuth, err := p.requestJSONWithAuth(ctx, site, http.MethodGet, "/api/v1/auth/me", auth, nil, timeoutSeconds)
	if err == nil {
		auth = nextAuth
	}
	if err != nil {
		if hasSub2APILoginCredentials(site) {
			fallbackAuth, loginErr := p.login(ctx, site, timeoutSeconds)
			if loginErr == nil {
				auth = fallbackAuth
				profile, _, nextAuth, err = p.requestJSONWithAuth(ctx, site, http.MethodGet, "/api/v1/auth/me", auth, nil, timeoutSeconds)
				if err == nil {
					auth = nextAuth
				}
			}
		}
		if err != nil {
			return AccountStatus{}, err
		}
	}
	data, err := unwrapData(profile)
	if err != nil {
		return AccountStatus{}, err
	}

	updatedCredentials, primaryKey := p.syncAPIKeys(ctx, site, &auth, timeoutSeconds)
	if updatedCredentials == nil {
		updatedCredentials = models.JSONMap{}
	}

	email := strings.TrimSpace(pathString(profile, "data.email", pathString(map[string]any{"data": data}, "data.email", "")))
	if email == "" {
		email = stringValue(site.Credentials, "email", "")
	}
	balance := pathFloat(profile, "data.balance")
	currency := pathString(profile, "data.currency", "$")
	balanceUnit := strings.TrimSpace(currency)
	packageQuota := p.fetchPackageQuota(ctx, site, &auth, profile, timeoutSeconds)
	if packageQuota.Remaining != nil {
		balance = packageQuota.Remaining
		if strings.TrimSpace(packageQuota.Unit) != "" {
			balanceUnit = strings.TrimSpace(packageQuota.Unit)
		}
	}
	if primaryKey != "" {
		updatedCredentials["api_key"] = primaryKey
	}

	message := "sub2api 资料同步完成。"
	if primaryKey != "" {
		message = fmt.Sprintf("%s 已识别 API Key %s。", message, maskKey(primaryKey))
	}
	inviteLink, inviteCode := extractInviteInfo(profile, site)
	if fetchedLink, fetchedCode, err := fetchInviteInfo(ctx, site, func(ctx context.Context, spec inviteRequestSpec) (map[string]any, error) {
		invitePayload, _, nextAuth, err := p.requestJSONWithAuth(ctx, site, spec.Method, spec.Target, auth, spec.Body, timeoutSeconds)
		if err == nil {
			auth = nextAuth
		}
		return invitePayload, err
	}); err == nil {
		inviteLink, inviteCode = mergeInviteInfo(site, fetchedLink, fetchedCode, inviteLink, inviteCode)
	}
	updatedCredentials["access_token"] = auth.AccessToken
	if auth.RefreshToken != "" {
		updatedCredentials["refresh_token"] = auth.RefreshToken
	}
	return AccountStatus{
		LoggedIn:           true,
		Message:            message,
		Balance:            balance,
		BalanceUnit:        ptrIfNonEmpty(balanceUnit),
		PackageRemaining:   packageQuota.Remaining,
		PackageTotal:       packageQuota.Total,
		PackageUsed:        packageQuota.Used,
		PackageUnit:        ptrIfNonEmpty(packageQuota.Unit),
		PackageDisplay:     ptrIfNonEmpty(packageQuota.Display),
		AccountName:        ptrIfNonEmpty(email),
		InviteLink:         inviteLink,
		InviteCode:         inviteCode,
		UpdatedCredentials: updatedCredentials,
	}, nil
}

func (p *Sub2API) SyncAPIKeys(ctx context.Context, site models.Site, timeoutSeconds int) (APIKeySyncResult, error) {
	if err := p.Validate(site); err != nil {
		return APIKeySyncResult{}, err
	}
	auth, err := p.authContext(ctx, site, timeoutSeconds)
	if err != nil {
		return APIKeySyncResult{}, err
	}
	updates, primaryKey := p.syncAPIKeys(ctx, site, &auth, timeoutSeconds)
	if updates == nil {
		updates = models.JSONMap{}
	}
	if primaryKey != "" {
		updates["api_key"] = primaryKey
	}
	if auth.AccessToken != "" {
		updates["access_token"] = auth.AccessToken
	}
	if auth.RefreshToken != "" {
		updates["refresh_token"] = auth.RefreshToken
	}
	count := apiKeyUpdateCount(updates)
	message := "未读取到可用 API Key。"
	if count > 0 {
		message = fmt.Sprintf("已更新 %d 个 API Key。", count)
		if primaryKey != "" {
			message = fmt.Sprintf("%s 首选 Key %s。", message, maskKey(primaryKey))
		}
	}
	return APIKeySyncResult{
		UpdatedCredentials: updates,
		PrimaryKey:         primaryKey,
		Message:            message,
	}, nil
}

func (p *Sub2API) Checkin(ctx context.Context, site models.Site, timeoutSeconds int) (CheckinResult, error) {
	if err := p.Validate(site); err != nil {
		return CheckinResult{}, err
	}
	if disabled := strings.ToLower(strings.TrimSpace(stringValue(site.PluginConfig, "disable_checkin", ""))); disabled == "1" || disabled == "true" || disabled == "yes" || disabled == "on" {
		return CheckinResult{Success: false, Message: "当前部署已关闭签到接口。"}, nil
	}
	auth, err := p.authContext(ctx, site, timeoutSeconds)
	if err != nil {
		return CheckinResult{}, err
	}
	checkinURL := strings.TrimSpace(stringValue(site.PluginConfig, "checkin_url", ""))
	if checkinURL == "" {
		checkinURL = "/api/v1/user/checkin"
	}
	payload, raw, err := p.requestJSON(ctx, site, http.MethodPost, checkinURL, auth, map[string]any{}, timeoutSeconds)
	if err != nil {
		return CheckinResult{}, err
	}
	data, err := unwrapData(payload)
	if err != nil {
		return CheckinResult{}, err
	}
	balance := pathFloat(payload, "data.balance")
	excerpt := shorten(raw, 500)
	message := pathString(payload, "message", "签到完成。")
	success := pathBool(payload, "success") || data != nil
	return CheckinResult{
		Success:         success,
		Message:         message,
		Balance:         balance,
		BalanceUnit:     ptrIfNonEmpty(pathString(payload, "data.currency", "")),
		ResponseExcerpt: &excerpt,
	}, nil
}

// ---------------------------- internals ----------------------------

func (p *Sub2API) authContext(ctx context.Context, site models.Site, timeoutSeconds int) (sub2apiAuth, error) {
	access := strings.TrimSpace(stringValue(site.Credentials, "access_token", ""))
	refresh := strings.TrimSpace(stringValue(site.Credentials, "refresh_token", ""))
	if access != "" {
		return sub2apiAuth{AccessToken: access, RefreshToken: refresh}, nil
	}
	if refresh != "" {
		refreshed, err := p.refresh(ctx, site, refresh, timeoutSeconds)
		if err == nil && refreshed.AccessToken != "" {
			return refreshed, nil
		}
		if hasSub2APILoginCredentials(site) {
			return p.login(ctx, site, timeoutSeconds)
		}
	}
	return p.login(ctx, site, timeoutSeconds)
}

func hasSub2APILoginCredentials(site models.Site) bool {
	email := strings.TrimSpace(stringValue(site.Credentials, "email", ""))
	password := strings.TrimSpace(stringValue(site.Credentials, "password", ""))
	return email != "" && password != ""
}

func (p *Sub2API) login(ctx context.Context, site models.Site, timeoutSeconds int) (sub2apiAuth, error) {
	body := map[string]any{
		"email":    strings.TrimSpace(stringValue(site.Credentials, "email", "")),
		"password": strings.TrimSpace(stringValue(site.Credentials, "password", "")),
	}
	if t := strings.TrimSpace(stringValue(site.PluginConfig, "turnstile_token", "")); t != "" {
		body["turnstile_token"] = t
	}
	if field := strings.TrimSpace(stringValue(site.PluginConfig, "totp_field_name", "")); field != "" {
		code, _, _ := services.ResolveTOTPCode(
			strings.TrimSpace(stringValue(site.Credentials, "totp_secret", "")),
			strings.TrimSpace(stringValue(site.Credentials, "totp_otpauth_url", "")),
		)
		if code != "" {
			body[field] = code
		}
	}
	payload, _, err := p.requestJSON(ctx, site, http.MethodPost, "/api/v1/auth/login", sub2apiAuth{}, body, timeoutSeconds)
	if err != nil {
		return sub2apiAuth{}, err
	}
	data, err := unwrapData(payload)
	if err != nil {
		return sub2apiAuth{}, err
	}
	access := strings.TrimSpace(pathString(map[string]any{"data": data}, "data.access_token", ""))
	if access == "" {
		return sub2apiAuth{}, errors.New("sub2api 登录成功但未返回 access_token")
	}
	refresh := strings.TrimSpace(pathString(map[string]any{"data": data}, "data.refresh_token", ""))
	return sub2apiAuth{AccessToken: access, RefreshToken: refresh}, nil
}

func (p *Sub2API) syncAPIKeys(ctx context.Context, site models.Site, auth *sub2apiAuth, timeoutSeconds int) (models.JSONMap, string) {
	apiKeysURL := strings.TrimSpace(stringValue(site.PluginConfig, "api_keys_url", ""))
	if apiKeysURL == "" {
		apiKeysURL = "/api/v1/keys?page=1&page_size=100&sort_by=created_at&sort_order=desc"
	}
	payload, _, nextAuth, err := p.requestJSONWithAuth(ctx, site, http.MethodGet, apiKeysURL, *auth, nil, timeoutSeconds)
	if err != nil {
		return nil, ""
	}
	*auth = nextAuth
	items := extractItems(payload)
	if len(items) == 0 {
		return nil, ""
	}
	preferredName := strings.TrimSpace(stringValue(site.PluginConfig, "preferred_api_key_name", ""))
	preferredID := strings.TrimSpace(stringValue(site.PluginConfig, "preferred_api_key_id", ""))
	primary := pickAPIKey(items, preferredName, preferredID)
	credentialsUpdate := models.JSONMap{}
	apiKeys := []map[string]any{}
	for _, item := range items {
		routeType := routeTypeFromAPIKeyItem(item, site)
		entry := map[string]any{
			"id":         fmt.Sprint(item["id"]),
			"name":       fmt.Sprint(item["name"]),
			"key":        fmt.Sprint(item["key"]),
			"status":     fmt.Sprint(item["status"]),
			"route_type": routeType,
			"api_type":   routeType,
		}
		apiKeys = append(apiKeys, entry)
	}
	credentialsUpdate["api_keys"] = apiKeys
	primaryKey := ""
	if primary != nil {
		primaryKey = strings.TrimSpace(fmt.Sprint(primary["key"]))
	}
	return credentialsUpdate, primaryKey
}

func (p *Sub2API) fetchPackageQuota(ctx context.Context, site models.Site, auth *sub2apiAuth, profile map[string]any, timeoutSeconds int) packageQuotaSnapshot {
	if payload, _, nextAuth, err := p.requestJSONWithAuth(ctx, site, http.MethodGet, "/api/v1/subscriptions/progress", *auth, nil, timeoutSeconds); err == nil {
		*auth = nextAuth
		if quota := sub2apiQuotaFromProgressPayload(payload); quota.hasQuota() {
			return quota
		}
	}
	if payload, _, nextAuth, err := p.requestJSONWithAuth(ctx, site, http.MethodGet, "/api/v1/subscriptions/summary", *auth, nil, timeoutSeconds); err == nil {
		*auth = nextAuth
		if quota := sub2apiQuotaFromSummaryPayload(payload); quota.hasQuota() {
			return quota
		}
	}
	if payload, _, nextAuth, err := p.requestJSONWithAuth(ctx, site, http.MethodGet, strings.TrimSpace(stringValue(site.PluginConfig, "api_keys_url", "/api/v1/keys?page=1&page_size=100&sort_by=created_at&sort_order=desc")), *auth, nil, timeoutSeconds); err == nil {
		*auth = nextAuth
		if quota := sub2apiQuotaFromKeysPayload(payload); quota.hasQuota() {
			return quota
		}
	}
	return packageQuotaFromPayload(site, profile, "")
}

func (p *Sub2API) requestJSON(ctx context.Context, site models.Site, method, target string, auth sub2apiAuth, body any, timeoutSeconds int) (map[string]any, string, error) {
	payload, raw, _, err := p.requestJSONWithAuth(ctx, site, method, target, auth, body, timeoutSeconds)
	return payload, raw, err
}

func sub2apiQuotaFromProgressPayload(payload map[string]any) packageQuotaSnapshot {
	items := extractItems(payload)
	var best packageQuotaSnapshot
	bestRank := -1
	displays := []string{}
	for _, item := range items {
		progress, ok := item["progress"].(map[string]any)
		if !ok {
			continue
		}
		label := sub2apiSubscriptionLabel(item)
		for _, window := range []struct {
			key   string
			label string
			rank  int
		}{
			{key: "monthly", label: "月度套餐", rank: 1},
			{key: "weekly", label: "周度套餐", rank: 2},
			{key: "daily", label: "日度套餐", rank: 3},
		} {
			raw, ok := progress[window.key].(map[string]any)
			if !ok {
				continue
			}
			limit := numberPtr(firstExistingValue(raw, "limit_usd", "limit", "total"))
			used := numberPtr(firstExistingValue(raw, "used_usd", "used"))
			remaining := numberPtr(firstExistingValue(raw, "remaining_usd", "remaining"))
			if remaining == nil && limit != nil && used != nil {
				value := *limit - *used
				remaining = &value
			}
			if limit == nil && used == nil && remaining == nil {
				continue
			}
			fullLabel := strings.TrimSpace(label + " " + window.label)
			display := formatPackageQuotaDisplay(fullLabel, remaining, limit, used, "USD")
			displays = appendUniqueNonEmpty(displays, display)
			quota := packageQuotaSnapshot{
				Display:   display,
				Remaining: remaining,
				Total:     limit,
				Used:      used,
				Unit:      "USD",
			}
			if window.rank > bestRank {
				best = quota
				bestRank = window.rank
			}
		}
	}
	if best.hasQuota() && len(displays) > 0 {
		best.Display = strings.Join(displays, "；")
	}
	return best
}

func sub2apiQuotaFromSummaryPayload(payload map[string]any) packageQuotaSnapshot {
	var items []map[string]any
	if data, ok := payload["data"].(map[string]any); ok {
		if values, ok := data["subscriptions"].([]any); ok {
			items = mapDictArray(values)
		}
	}
	if len(items) == 0 {
		if values, ok := payload["subscriptions"].([]any); ok {
			items = mapDictArray(values)
		}
	}
	var best packageQuotaSnapshot
	bestRank := -1
	displays := []string{}
	for _, item := range items {
		label := strings.TrimSpace(fmt.Sprint(firstExistingValue(item, "group_name", "name")))
		if label == "" || label == "<nil>" {
			label = "sub2api"
		}
		for _, window := range []struct {
			usedKey  string
			limitKey string
			label    string
			rank     int
		}{
			{usedKey: "monthly_used_usd", limitKey: "monthly_limit_usd", label: "月度套餐", rank: 1},
			{usedKey: "weekly_used_usd", limitKey: "weekly_limit_usd", label: "周度套餐", rank: 2},
			{usedKey: "daily_used_usd", limitKey: "daily_limit_usd", label: "日度套餐", rank: 3},
		} {
			total := numberPtr(item[window.limitKey])
			used := numberPtr(item[window.usedKey])
			if total == nil || used == nil || *total <= 0 {
				continue
			}
			remaining := *total - *used
			display := formatPackageQuotaDisplay(label+" "+window.label, &remaining, total, used, "USD")
			displays = appendUniqueNonEmpty(displays, display)
			quota := packageQuotaSnapshot{
				Display:   display,
				Remaining: &remaining,
				Total:     total,
				Used:      used,
				Unit:      "USD",
			}
			if window.rank > bestRank {
				best = quota
				bestRank = window.rank
			}
		}
	}
	if best.hasQuota() && len(displays) > 0 {
		best.Display = strings.Join(displays, "；")
	}
	return best
}

func appendUniqueNonEmpty(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func sub2apiQuotaFromKeysPayload(payload map[string]any) packageQuotaSnapshot {
	items := extractItems(payload)
	var best packageQuotaSnapshot
	for _, item := range items {
		total := numberPtr(firstExistingValue(item, "quota", "rate_limit_7d", "rate_limit_1d", "rate_limit_5h"))
		used := numberPtr(firstExistingValue(item, "quota_used", "usage_7d", "usage_1d", "usage_5h"))
		if total == nil || *total <= 0 {
			continue
		}
		if used == nil {
			zero := 0.0
			used = &zero
		}
		remaining := *total - *used
		name := strings.TrimSpace(fmt.Sprint(firstExistingValue(item, "name", "id")))
		if name == "" || name == "<nil>" {
			name = "API Key"
		}
		quota := packageQuotaSnapshot{
			Display:   formatPackageQuotaDisplay(name+" Key 额度", &remaining, total, used, "USD"),
			Remaining: &remaining,
			Total:     total,
			Used:      used,
			Unit:      "USD",
		}
		if !best.hasQuota() || best.Remaining == nil || remaining > *best.Remaining {
			best = quota
		}
	}
	return best
}

func sub2apiSubscriptionLabel(item map[string]any) string {
	subscription, _ := item["subscription"].(map[string]any)
	for _, source := range []map[string]any{subscription, item} {
		if source == nil {
			continue
		}
		if group, ok := source["group"].(map[string]any); ok {
			if value := strings.TrimSpace(fmt.Sprint(firstExistingValue(group, "name", "group_name"))); value != "" && value != "<nil>" {
				return value
			}
		}
		if value := strings.TrimSpace(fmt.Sprint(firstExistingValue(source, "group_name", "name", "plan_name"))); value != "" && value != "<nil>" {
			return value
		}
	}
	return "sub2api"
}

func (p *Sub2API) requestJSONWithAuth(ctx context.Context, site models.Site, method, target string, auth sub2apiAuth, body any, timeoutSeconds int) (map[string]any, string, sub2apiAuth, error) {
	return p.requestJSONWithAuthRefresh(ctx, site, method, target, auth, body, timeoutSeconds, true)
}

func (p *Sub2API) requestJSONWithAuthRefresh(ctx context.Context, site models.Site, method, target string, auth sub2apiAuth, body any, timeoutSeconds int, allowRefresh bool) (map[string]any, string, sub2apiAuth, error) {
	if target == "" {
		return nil, "", auth, errors.New("请求路径不能为空")
	}
	url, err := services.JoinURL(site.BaseURL, target)
	if err != nil {
		return nil, "", auth, err
	}
	includeContentType := body != nil
	var payload io.Reader
	if includeContentType {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, "", auth, err
		}
		payload = bytes.NewReader(buf)
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, strings.ToUpper(method), url, payload)
	if err != nil {
		return nil, "", auth, err
	}
	headers := services.BuildBrowserHeaders(site.BaseURL, includeContentType, "", "", nil)
	if auth.AccessToken != "" {
		headers["Authorization"] = "Bearer " + auth.AccessToken
	}
	if ua := strings.TrimSpace(stringValue(site.Credentials, "user_agent", "")); ua != "" {
		headers["User-Agent"] = ua
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", auth, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	raw := string(data)
	if allowRefresh && resp.StatusCode == http.StatusUnauthorized && auth.RefreshToken != "" {
		refreshed, refreshErr := p.refresh(ctx, site, auth.RefreshToken, timeoutSeconds)
		if refreshErr == nil {
			return p.requestJSONWithAuthRefresh(ctx, site, method, target, refreshed, body, timeoutSeconds, false)
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, raw, auth, fmt.Errorf("sub2api 接口返回 %d: %s", resp.StatusCode, shorten(raw, 300))
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("content-type")), "json") {
		return nil, raw, auth, fmt.Errorf("sub2api 接口未返回 JSON：%s", resp.Header.Get("content-type"))
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, raw, auth, fmt.Errorf("sub2api 接口 JSON 解析失败：%w", err)
	}
	return parsed, raw, auth, nil
}

func (p *Sub2API) refresh(ctx context.Context, site models.Site, refreshToken string, timeoutSeconds int) (sub2apiAuth, error) {
	payload, _, err := p.requestJSON(ctx, site, http.MethodPost, "/api/v1/auth/refresh", sub2apiAuth{}, map[string]any{"refresh_token": refreshToken}, timeoutSeconds)
	if err != nil {
		return sub2apiAuth{}, err
	}
	data, err := unwrapData(payload)
	if err != nil {
		return sub2apiAuth{}, err
	}
	access := strings.TrimSpace(pathString(map[string]any{"data": data}, "data.access_token", ""))
	if access == "" {
		return sub2apiAuth{}, errors.New("sub2api 刷新 token 成功但未返回 access_token")
	}
	next := strings.TrimSpace(pathString(map[string]any{"data": data}, "data.refresh_token", ""))
	if next == "" {
		next = refreshToken
	}
	return sub2apiAuth{AccessToken: access, RefreshToken: next}, nil
}

// ---------------------------- helpers ----------------------------

func unwrapData(payload map[string]any) (map[string]any, error) {
	if payload == nil {
		return nil, errors.New("sub2api 接口未返回 JSON")
	}
	if v, ok := payload["success"].(bool); ok && !v {
		message := strings.TrimSpace(pathString(payload, "message", ""))
		if message == "" {
			message = "sub2api 接口返回失败。"
		}
		return nil, errors.New(message)
	}
	if data, ok := payload["data"].(map[string]any); ok {
		return data, nil
	}
	return nil, nil
}

func extractItems(payload map[string]any) []map[string]any {
	if payload == nil {
		return nil
	}
	data := payload["data"]
	switch typed := data.(type) {
	case []any:
		return mapDictArray(typed)
	case map[string]any:
		if items, ok := typed["items"].([]any); ok {
			return mapDictArray(items)
		}
	}
	if items, ok := payload["items"].([]any); ok {
		return mapDictArray(items)
	}
	return nil
}

func mapDictArray(in []any) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, item := range in {
		if obj, ok := item.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	return out
}

func pickAPIKey(items []map[string]any, preferredName, preferredID string) map[string]any {
	if preferredID != "" {
		for _, item := range items {
			if fmt.Sprint(item["id"]) == preferredID {
				return item
			}
		}
	}
	if preferredName != "" {
		for _, item := range items {
			if strings.EqualFold(fmt.Sprint(item["name"]), preferredName) {
				return item
			}
		}
	}
	for _, item := range items {
		if status := strings.ToLower(fmt.Sprint(item["status"])); status == "active" || status == "" {
			if isPrimary, ok := item["is_primary"].(bool); ok && isPrimary {
				return item
			}
		}
	}
	for _, item := range items {
		status := strings.ToLower(fmt.Sprint(item["status"]))
		if status == "active" || status == "" {
			return item
		}
	}
	if len(items) > 0 {
		return items[0]
	}
	return nil
}

func ptrIfNonEmpty(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func routeTypeFromAPIKeyItem(item map[string]any, site models.Site) string {
	for _, key := range []string{"route_type", "api_type", "api_format", "type", "format", "model_type", "provider"} {
		if routeType := normalizeAPIKeyRouteType(fmt.Sprint(item[key])); routeType != "" {
			return routeType
		}
	}
	nameText := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["name"])))
	if routeType := normalizeAPIKeyRouteType(nameText); routeType != "" {
		return routeType
	}
	for _, key := range []string{"gateway_route_type", "api_format"} {
		if routeType := normalizeAPIKeyRouteType(stringValue(site.PluginConfig, key, "")); routeType != "" {
			return routeType
		}
	}
	return "codex"
}

func normalizeAPIKeyRouteType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "<nil>" {
		return ""
	}
	if strings.Contains(value, "claude") || strings.Contains(value, "anthropic") {
		return "claude"
	}
	if strings.Contains(value, "gemini") || strings.Contains(value, "google") {
		return "gemini"
	}
	if strings.Contains(value, "codex") || strings.Contains(value, "openai") || strings.Contains(value, "gpt") || strings.Contains(value, "chatgpt") {
		return "codex"
	}
	return ""
}

func maskKey(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return value
	}
	return value[:4] + "..." + value[len(value)-4:]
}

// keep compiler-happy in case strconv is unused after refactor
var _ = strconv.Itoa
