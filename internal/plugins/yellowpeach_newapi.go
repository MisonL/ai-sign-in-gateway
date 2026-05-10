package plugins

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/schemas"
	"ai-sign-in-gateway/internal/services"
)

var ypSignablePaths = map[string]struct{}{
	"/api/verification":   {},
	"/api/user/register":  {},
	"/api/user/login":     {},
	"/api/user/login/2fa": {},
}

var ypSigningKeyCache = struct {
	sync.Mutex
	m map[string]string
}{m: map[string]string{}}

// YellowPeach 实现 NewAPI 系站点（黄桃 / yellowpeach）的核心子集：
//   - 已知 session Cookie 或 access_token 时：拉取 /api/user/self 与签到。
//   - 没有 cookie/access_token 时：暂不在 Go 端自动登录（依赖 HMAC 签名 + 可选 TOTP），
//     返回明确错误指引用户填入 cookie / access_token。
//
// 后续会补齐自动登录与签到日历摘要等附加能力。
type YellowPeach struct {
	client *http.Client
}

func NewYellowPeach() *YellowPeach { return &YellowPeach{client: &http.Client{}} }

func (p *YellowPeach) Meta() schemas.PluginMetaResponse {
	return schemas.PluginMetaResponse{
		Key:            "yellowpeach-newapi",
		Name:           "黄桃 New API 站点",
		Description:    "适配 NewAPI 面板的登录、资料读取、API Key 同步与签到。",
		Capabilities:   []string{"checkin", "account_status", "gateway", "api_key_sync", "account_registration"},
		AuthEntryLabel: "打开站点登录页",
		AuthHint:       "推荐填入浏览器中的 session Cookie，其次是后台系统访问令牌；自动登录将在后续版本启用。",
		CredentialFields: []schemas.FieldDescriptor{
			Field("account", "账号备注", "text", "主账号 / 个人号", false, ""),
			Field("username", "登录用户名", "text", "用于自动登录（暂未启用）", false, ""),
			Field("password", "登录密码", "password", "用于自动登录（暂未启用）", false, ""),
			Field("cookie", "Session Cookie", "textarea", "session=...", false, ""),
			Field("access_token", "系统访问令牌", "textarea", "Bearer ... 或 token", false, ""),
			Field("api_key", "主 API Key", "password", "同步出的首选 API Key", false, ""),
			Field("user_id", "用户 ID", "text", "例如 4217", false, ""),
			Field("user_agent", "User-Agent", "text", services.DefaultBrowserUserAgent, false, ""),
		},
		ConfigFields: []schemas.FieldDescriptor{
			Field("turnstile_token", "Turnstile Token", "text", "可留空", false, "注册 / 登录接口需要 Turnstile 时填写。"),
			Field("referer_path", "Referer 路径", "text", "/console/personal", false, ""),
			Field("quota_per_unit", "额度换算基数", "number", "500000", false, ""),
			Field("checkin_mode", "签到模式", "text", "default 或 reward_center", false, ""),
			Field("api_format", "API 格式", "text", "openai / anthropic / gemini", false, ""),
			Field("api_keys_url", "API Key 列表 URL", "url", "/api/token/?p=0&size=100", false, "NewAPI 默认令牌列表接口；部分自定义站点可在这里改路径。"),
			Field("token_keys_url", "批量获取 Key URL", "url", "/api/token/batch/keys", false, "当列表接口只返回脱敏 Key 时，使用该接口按 token id 获取完整 Key。"),
			Field("create_api_key_url", "API Key 创建 URL", "url", "/api/token/", false, "注册后没有可用 Key 时自动创建。"),
			Field("default_api_key_name", "默认 API Key 名称", "text", "default", false, ""),
			Field("preferred_api_key_name", "首选 API Key 名称", "text", "default", false, ""),
			Field("preferred_api_key_id", "首选 API Key ID", "text", "例如 12", false, ""),
			Field("invite_path", "邀请接口路径", "url", "/api/user/aff", false, "填写后会额外请求该接口解析邀请链接；留空则直接复用资料接口响应。"),
			Field("invite_method", "邀请接口方法", "text", "GET", false, ""),
			Field("invite_body_json", "邀请接口 JSON Body", "textarea", `{"refresh": false}`, false, ""),
			Field("status_invite_link_path", "邀请链接字段路径", "text", "data.invite_link", false, ""),
			Field("status_invite_code_path", "邀请码字段路径", "text", "data.aff_code", false, ""),
			Field("invite_link_path", "邀请接口链接字段路径", "text", "data.invite_link", false, ""),
			Field("invite_code_path", "邀请接口邀请码字段路径", "text", "data.aff_code", false, ""),
			Field("invite_link_template", "邀请链接模板", "text", "/register?aff={code}", false, "支持相对路径或完整 URL，使用 {code} 作为邀请码占位符。"),
		},
	}
}

func (p *YellowPeach) RegisterAccount(ctx context.Context, site models.Site, request AccountRegistrationRequest, timeoutSeconds int) (AccountRegistrationResult, error) {
	email := strings.TrimSpace(request.Email)
	password := strings.TrimSpace(request.Password)
	if email == "" || password == "" {
		return AccountRegistrationResult{}, errors.New("注册邮箱和密码不能为空")
	}
	if strings.TrimSpace(site.BaseURL) == "" {
		return AccountRegistrationResult{}, errors.New("Base URL 不能为空")
	}

	turnstile := strings.TrimSpace(stringValue(site.PluginConfig, "turnstile_token", ""))
	registerURL := strings.TrimRight(site.BaseURL, "/") + "/api/user/register"
	if turnstile != "" {
		registerURL += "?turnstile=" + url.QueryEscape(turnstile)
	}
	body := map[string]any{"username": email, "password": password}
	if inviteCode := strings.TrimSpace(stringValue(site.PluginConfig, "invite_code", "")); inviteCode != "" {
		body["aff_code"] = inviteCode
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	payload, _, err := p.postSigned(ctx, client, site, registerURL, body, timeoutSeconds)
	if err != nil {
		return AccountRegistrationResult{}, err
	}
	if !pathBool(payload, "success") {
		return AccountRegistrationResult{}, errors.New(pathString(payload, "message", "注册失败"))
	}

	loginSite := site
	loginSite.Credentials = clonePluginJSON(site.Credentials)
	loginSite.Credentials["account"] = firstNonEmptyPlugin(strings.TrimSpace(request.AccountName), email)
	loginSite.Credentials["username"] = email
	loginSite.Credentials["email"] = email
	loginSite.Credentials["password"] = password
	auth, err := p.autoLogin(ctx, loginSite, timeoutSeconds)
	if err != nil {
		return AccountRegistrationResult{}, err
	}
	updates, primaryKey := p.syncAPIKeys(ctx, loginSite, auth, timeoutSeconds, true)
	credentials := clonePluginJSON(updates)
	credentials["account"] = firstNonEmptyPlugin(strings.TrimSpace(request.AccountName), email)
	credentials["username"] = email
	credentials["email"] = email
	credentials["password"] = password
	credentials["cookie"] = auth.SessionCookie
	if auth.UserID != "" {
		credentials["user_id"] = auth.UserID
	}
	if primaryKey != "" {
		credentials["api_key"] = primaryKey
	}
	apiCount := apiKeyUpdateCount(credentials)
	message := "注册并登录成功。"
	if apiCount > 0 {
		message = fmt.Sprintf("%s 已同步 %d 个 API Key。", message, apiCount)
	}
	return AccountRegistrationResult{
		Message:      message,
		Credentials:  credentials,
		PluginConfig: clonePluginJSON(site.PluginConfig),
		PrimaryKey:   primaryKey,
		APIKeyCount:  apiCount,
		AccountName:  email,
	}, nil
}

func (p *YellowPeach) Validate(site models.Site) error {
	if strings.TrimSpace(site.BaseURL) == "" {
		return errors.New("Base URL 不能为空")
	}
	cookie := strings.TrimSpace(stringValue(site.Credentials, "cookie", ""))
	access := strings.TrimSpace(stringValue(site.Credentials, "access_token", ""))
	username := strings.TrimSpace(stringValue(site.Credentials, "username", ""))
	if username == "" {
		username = strings.TrimSpace(stringValue(site.Credentials, "account", ""))
	}
	if username == "" {
		username = strings.TrimSpace(stringValue(site.Credentials, "email", ""))
	}
	password := strings.TrimSpace(stringValue(site.Credentials, "password", ""))
	if cookie == "" && access == "" && (username == "" || password == "") {
		return errors.New("请填写有效 session Cookie、系统访问令牌 access_token，或同时填写用户名/邮箱和密码")
	}
	return nil
}

type yellowpeachAuth struct {
	SessionCookie string
	Authorization string
	UserID        string
}

func (p *YellowPeach) FetchAccountStatus(ctx context.Context, site models.Site, timeoutSeconds int) (AccountStatus, error) {
	if err := p.Validate(site); err != nil {
		return AccountStatus{}, err
	}
	auth, err := p.authContext(ctx, site, timeoutSeconds)
	if err != nil {
		return AccountStatus{}, err
	}
	payload, _, err := p.requestJSON(ctx, site, http.MethodGet, "/api/user/self", auth, nil, timeoutSeconds)
	if err != nil {
		if hasYellowPeachLoginCredentials(site) {
			fallbackAuth, loginErr := p.autoLogin(ctx, site, timeoutSeconds)
			if loginErr == nil {
				auth = fallbackAuth
				payload, _, err = p.requestJSON(ctx, site, http.MethodGet, "/api/user/self", auth, nil, timeoutSeconds)
			}
		}
		if err != nil {
			return AccountStatus{}, err
		}
	}
	loggedIn := pathBool(payload, "success")
	message := pathString(payload, "message", "用户信息读取成功。")
	balance := p.extractBalance(site, payload)
	balanceUnit := normalizeBalanceUnit(p.extractBalanceUnit(payload))
	packageQuota := p.fetchPackageQuota(ctx, site, auth, payload, timeoutSeconds)
	accountName := p.extractAccountName(site, payload)
	updates := models.JSONMap{}
	if auth.UserID != "" {
		updates["user_id"] = auth.UserID
	}
	if auth.SessionCookie != "" {
		updates["cookie"] = auth.SessionCookie
	}
	keyUpdates, primaryKey := p.listAPIKeys(ctx, site, auth, timeoutSeconds)
	for key, value := range keyUpdates {
		updates[key] = value
	}
	if primaryKey != "" {
		updates["api_key"] = primaryKey
		message = fmt.Sprintf("%s 已识别 API Key %s。", message, maskKey(primaryKey))
	}
	inviteLink, inviteCode := extractInviteInfo(payload, site)
	if fetchedLink, fetchedCode, err := fetchInviteInfo(ctx, site, func(ctx context.Context, spec inviteRequestSpec) (map[string]any, error) {
		invitePayload, _, err := p.requestJSON(ctx, site, spec.Method, spec.Target, auth, spec.Body, timeoutSeconds)
		return invitePayload, err
	}); err == nil {
		inviteLink, inviteCode = mergeInviteInfo(site, fetchedLink, fetchedCode, inviteLink, inviteCode)
	}
	return AccountStatus{
		LoggedIn:           loggedIn,
		Message:            message,
		Balance:            balance,
		BalanceUnit:        ptrIfNonEmpty(balanceUnit),
		PackageRemaining:   packageQuota.Remaining,
		PackageTotal:       packageQuota.Total,
		PackageUsed:        packageQuota.Used,
		PackageUnit:        ptrIfNonEmpty(firstNonEmptyPlugin(packageQuota.Unit, balanceUnit)),
		PackageDisplay:     ptrIfNonEmpty(packageQuota.Display),
		AccountName:        ptrIfNonEmpty(accountName),
		InviteLink:         inviteLink,
		InviteCode:         inviteCode,
		UpdatedCredentials: updates,
	}, nil
}

func (p *YellowPeach) SyncAPIKeys(ctx context.Context, site models.Site, timeoutSeconds int) (APIKeySyncResult, error) {
	if err := p.Validate(site); err != nil {
		return APIKeySyncResult{}, err
	}
	auth, err := p.apiKeyAuthContext(ctx, site, timeoutSeconds)
	if err != nil {
		return APIKeySyncResult{}, err
	}
	updates, primaryKey := p.syncAPIKeys(ctx, site, auth, timeoutSeconds, false)
	if updates == nil {
		updates = models.JSONMap{}
	}
	if primaryKey != "" {
		updates["api_key"] = primaryKey
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

func (p *YellowPeach) Checkin(ctx context.Context, site models.Site, timeoutSeconds int) (CheckinResult, error) {
	if err := p.Validate(site); err != nil {
		return CheckinResult{}, err
	}
	auth, err := p.authContext(ctx, site, timeoutSeconds)
	if err != nil {
		return CheckinResult{}, err
	}
	mode := strings.ToLower(strings.TrimSpace(stringValue(site.PluginConfig, "checkin_mode", "default")))
	target := "/api/user/checkin"
	var body any
	if mode == "reward_center" {
		// 简化版：直接调用 claim 接口；若需要 calendar/claim_meta 可在后续补齐
		target = "/api/user/reward_center/claim"
		body = map[string]any{
			"action_code": stringValue(site.PluginConfig, "reward_claim_action_code", "daily_gift_claim_v2"),
		}
	}
	payload, raw, err := p.requestJSON(ctx, site, http.MethodPost, target, auth, body, timeoutSeconds)
	if err != nil {
		if hasYellowPeachLoginCredentials(site) {
			fallbackAuth, loginErr := p.autoLogin(ctx, site, timeoutSeconds)
			if loginErr == nil {
				auth = fallbackAuth
				payload, raw, err = p.requestJSON(ctx, site, http.MethodPost, target, auth, body, timeoutSeconds)
			}
		}
		if err != nil {
			return CheckinResult{}, err
		}
	}
	message := pathString(payload, "message", "签到请求已提交。")
	success := pathBool(payload, "success") || strings.Contains(message, "已签到")
	excerpt := shorten(raw, 500)
	// 顺便刷新一次余额
	var balance *float64
	if selfPayload, _, err := p.requestJSON(ctx, site, http.MethodGet, "/api/user/self", auth, nil, timeoutSeconds); err == nil {
		balance = p.extractBalance(site, selfPayload)
	}
	return CheckinResult{
		Success:         success,
		Message:         message,
		Balance:         balance,
		ResponseExcerpt: &excerpt,
	}, nil
}

// --------------------------- helpers ---------------------------

func (p *YellowPeach) authContext(ctx context.Context, site models.Site, timeoutSeconds int) (yellowpeachAuth, error) {
	cookie := strings.TrimSpace(stringValue(site.Credentials, "cookie", ""))
	access := strings.TrimSpace(stringValue(site.Credentials, "access_token", ""))
	userID := strings.TrimSpace(stringValue(site.Credentials, "user_id", ""))

	if cookie == "" && access == "" {
		return p.autoLogin(ctx, site, timeoutSeconds)
	}

	auth := yellowpeachAuth{UserID: userID}
	if cookie != "" {
		auth.SessionCookie = normalizeYPCookie(cookie)
	} else if access != "" {
		auth.Authorization = normalizeYPAuthorization(access)
	}
	if userID == "" {
		resolved := p.resolveUserID(ctx, site, auth, timeoutSeconds)
		if resolved == "" {
			if hasYellowPeachLoginCredentials(site) {
				return p.autoLogin(ctx, site, timeoutSeconds)
			}
			return auth, errors.New("已提供凭证但未能自动获取 user_id，请手动填写后重试。")
		}
		auth.UserID = resolved
	}
	return auth, nil
}

func hasYellowPeachLoginCredentials(site models.Site) bool {
	username := strings.TrimSpace(stringValue(site.Credentials, "username", ""))
	if username == "" {
		username = strings.TrimSpace(stringValue(site.Credentials, "account", ""))
	}
	if username == "" {
		username = strings.TrimSpace(stringValue(site.Credentials, "email", ""))
	}
	password := strings.TrimSpace(stringValue(site.Credentials, "password", ""))
	return username != "" && password != ""
}

func (p *YellowPeach) apiKeyAuthContext(ctx context.Context, site models.Site, timeoutSeconds int) (yellowpeachAuth, error) {
	cookie := strings.TrimSpace(stringValue(site.Credentials, "cookie", ""))
	access := strings.TrimSpace(stringValue(site.Credentials, "access_token", ""))
	userID := strings.TrimSpace(stringValue(site.Credentials, "user_id", ""))
	auth := yellowpeachAuth{UserID: userID}
	if cookie != "" {
		auth.SessionCookie = normalizeYPCookie(cookie)
		return auth, nil
	}
	if access != "" {
		auth.Authorization = normalizeYPAuthorization(access)
		return auth, nil
	}
	return p.autoLogin(ctx, site, timeoutSeconds)
}

func (p *YellowPeach) autoLogin(ctx context.Context, site models.Site, timeoutSeconds int) (yellowpeachAuth, error) {
	username := strings.TrimSpace(stringValue(site.Credentials, "username", ""))
	if username == "" {
		username = strings.TrimSpace(stringValue(site.Credentials, "account", ""))
	}
	if username == "" {
		username = strings.TrimSpace(stringValue(site.Credentials, "email", ""))
	}
	password := strings.TrimSpace(stringValue(site.Credentials, "password", ""))
	if username == "" || password == "" {
		return yellowpeachAuth{}, errors.New("缺少用户名或密码，无法执行自动登录")
	}

	turnstile := strings.TrimSpace(stringValue(site.PluginConfig, "turnstile_token", ""))
	loginURL := strings.TrimRight(site.BaseURL, "/") + "/api/user/login"
	if turnstile != "" {
		loginURL = loginURL + "?turnstile=" + url.QueryEscape(turnstile)
	}
	body := map[string]any{"username": username, "password": password}
	if field := strings.TrimSpace(stringValue(site.PluginConfig, "totp_field_name", "")); field != "" {
		if code, _, _ := services.ResolveTOTPCode(
			strings.TrimSpace(stringValue(site.Credentials, "totp_secret", "")),
			strings.TrimSpace(stringValue(site.Credentials, "totp_otpauth_url", "")),
		); code != "" {
			body[field] = code
		}
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	payload, sessionCookie, err := p.postSigned(ctx, client, site, loginURL, body, timeoutSeconds)
	if err != nil {
		return yellowpeachAuth{}, err
	}
	if !pathBool(payload, "success") {
		return yellowpeachAuth{}, errors.New(pathString(payload, "message", "登录失败"))
	}

	if dataMap, ok := payload["data"].(map[string]any); ok {
		if require, ok := dataMap["require_2fa"].(bool); ok && require {
			code, _, _ := services.ResolveTOTPCode(
				strings.TrimSpace(stringValue(site.Credentials, "totp_secret", "")),
				strings.TrimSpace(stringValue(site.Credentials, "totp_otpauth_url", "")),
			)
			if code == "" {
				return yellowpeachAuth{}, errors.New("站点要求 Authenticator 验证码，但当前未配置 TOTP secret")
			}
			twoURL := strings.TrimRight(site.BaseURL, "/") + "/api/user/login/2fa"
			twoBody := map[string]any{"code": code}
			payload2, refreshed, err := p.postSigned(ctx, client, site, twoURL, twoBody, timeoutSeconds)
			if err != nil {
				return yellowpeachAuth{}, err
			}
			if !pathBool(payload2, "success") {
				return yellowpeachAuth{}, errors.New(pathString(payload2, "message", "二次验证失败"))
			}
			if refreshed != "" {
				sessionCookie = refreshed
			}
			payload = payload2
		}
	}

	auth := yellowpeachAuth{
		SessionCookie: sessionCookie,
		Authorization: normalizeYPAuthorization(firstNonEmptyPlugin(
			pathString(payload, "data.access_token", ""),
			pathString(payload, "data.token", ""),
			pathString(payload, "data.auth_token", ""),
			pathString(payload, "access_token", ""),
			pathString(payload, "token", ""),
		)),
		UserID: extractYPUserID(payload),
	}
	if auth.SessionCookie == "" && auth.Authorization == "" {
		return yellowpeachAuth{}, errors.New("登录成功但未获得 session Cookie 或 access_token")
	}
	if auth.UserID == "" {
		auth.UserID = strings.TrimSpace(stringValue(site.Credentials, "user_id", ""))
	}
	if auth.UserID == "" {
		auth.UserID = p.resolveUserID(ctx, site, auth, timeoutSeconds)
	}
	if auth.UserID == "" {
		return auth, errors.New("登录成功，但未能自动获取 user_id")
	}
	return auth, nil
}

func (p *YellowPeach) postSigned(ctx context.Context, client *http.Client, site models.Site, target string, body map[string]any, timeoutSeconds int) (map[string]any, string, error) {
	payloadBytes, err := json.Marshal(body)
	if err != nil {
		return nil, "", err
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, target, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, "", err
	}
	headers := services.BuildBrowserHeaders(site.BaseURL, true, "", "", nil)
	headers["Origin"] = strings.TrimRight(site.BaseURL, "/")
	headers["Referer"] = strings.TrimRight(site.BaseURL, "/") + "/login"
	headers["Cache-Control"] = "no-store"
	headers["New-API-User"] = "-1"
	if ua := strings.TrimSpace(stringValue(site.Credentials, "user_agent", "")); ua != "" {
		headers["User-Agent"] = ua
	}
	for k, v := range p.signHeaders(ctx, site, http.MethodPost, target, payloadBytes, timeoutSeconds) {
		headers[k] = v
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	contentType := strings.ToLower(resp.Header.Get("content-type"))
	var parsed map[string]any
	if strings.Contains(contentType, "json") {
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, "", fmt.Errorf("登录响应 JSON 解析失败：%w", err)
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parsed, "", fmt.Errorf("yellowpeach 登录返回 %d: %s", resp.StatusCode, shorten(string(data), 200))
	}
	if parsed == nil {
		return nil, "", fmt.Errorf("yellowpeach 登录未返回 JSON：%s", contentType)
	}
	sessionCookie := ""
	for _, c := range resp.Cookies() {
		if c.Name == "session" {
			sessionCookie = "session=" + c.Value
			break
		}
	}
	return parsed, sessionCookie, nil
}

func (p *YellowPeach) signHeaders(ctx context.Context, site models.Site, method, target string, body []byte, timeoutSeconds int) map[string]string {
	parsed, err := url.Parse(target)
	if err != nil {
		return nil
	}
	path := parsed.Path
	if path == "" {
		path = "/"
	}
	if _, ok := ypSignablePaths[path]; !ok {
		return nil
	}
	signingKey := p.fetchSigningKey(ctx, site, timeoutSeconds)
	if signingKey == "" {
		return nil
	}
	signedPath := path
	if parsed.RawQuery != "" {
		signedPath = path + "?" + parsed.RawQuery
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	canonical := strings.ToUpper(method) + "\n" + signedPath + "\n" + timestamp + "\n" + string(body)
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(canonical))
	signature := hex.EncodeToString(mac.Sum(nil))
	return map[string]string{
		"X-NewAPI-Timestamp": timestamp,
		"X-NewAPI-Signature": signature,
	}
}

func (p *YellowPeach) fetchSigningKey(ctx context.Context, site models.Site, timeoutSeconds int) string {
	origin := strings.TrimRight(site.BaseURL, "/")
	ypSigningKeyCache.Lock()
	cached, ok := ypSigningKeyCache.m[origin]
	ypSigningKeyCache.Unlock()
	if ok {
		return cached
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, origin+"/api/status", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Cache-Control", "no-store")
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/login")
	req.Header.Set("New-API-User", "-1")
	if ua := strings.TrimSpace(stringValue(site.Credentials, "user_agent", "")); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	key := strings.TrimSpace(pathString(payload, "data.public_request_signing_key", ""))
	if key != "" {
		ypSigningKeyCache.Lock()
		ypSigningKeyCache.m[origin] = key
		ypSigningKeyCache.Unlock()
	}
	return key
}

func (p *YellowPeach) resolveUserID(ctx context.Context, site models.Site, auth yellowpeachAuth, timeoutSeconds int) string {
	for _, path := range []string{"/api/user/self", "/api/user/checkin"} {
		payload, _, err := p.requestJSON(ctx, site, http.MethodGet, path, auth, nil, timeoutSeconds)
		if err != nil {
			continue
		}
		if id := extractYPUserID(payload); id != "" {
			return id
		}
	}
	return ""
}

func (p *YellowPeach) requestJSON(ctx context.Context, site models.Site, method, target string, auth yellowpeachAuth, body any, timeoutSeconds int) (map[string]any, string, error) {
	url, err := services.JoinURL(site.BaseURL, target)
	if err != nil {
		return nil, "", err
	}
	includeContentType := body != nil
	var payload io.Reader
	if includeContentType {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, "", err
		}
		payload = bytes.NewReader(buf)
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, strings.ToUpper(method), url, payload)
	if err != nil {
		return nil, "", err
	}
	headers := services.BuildBrowserHeaders(site.BaseURL, includeContentType, auth.Authorization, auth.SessionCookie, nil)
	if auth.UserID != "" {
		headers["New-API-User"] = auth.UserID
	}
	if ua := strings.TrimSpace(stringValue(site.Credentials, "user_agent", "")); ua != "" {
		headers["User-Agent"] = ua
	}
	if refPath := strings.TrimSpace(stringValue(site.PluginConfig, "referer_path", "")); refPath != "" {
		if strings.HasPrefix(refPath, "http") {
			headers["Referer"] = refPath
		} else {
			headers["Referer"] = strings.TrimRight(site.BaseURL, "/") + "/" + strings.TrimLeft(refPath, "/")
		}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	raw := string(data)
	if !strings.Contains(strings.ToLower(resp.Header.Get("content-type")), "json") {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, raw, fmt.Errorf("yellowpeach 接口返回 %d: %s", resp.StatusCode, shorten(raw, 200))
		}
		return nil, raw, fmt.Errorf("yellowpeach 接口未返回 JSON：%s", resp.Header.Get("content-type"))
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, raw, fmt.Errorf("yellowpeach 接口 JSON 解析失败：%w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := pathString(parsed, "message", "")
		if message == "" {
			message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return parsed, raw, fmt.Errorf("yellowpeach 接口返回 %d: %s", resp.StatusCode, shorten(message, 200))
	}
	return parsed, raw, nil
}

func (p *YellowPeach) listAPIKeys(ctx context.Context, site models.Site, auth yellowpeachAuth, timeoutSeconds int) (models.JSONMap, string) {
	return p.syncAPIKeys(ctx, site, auth, timeoutSeconds, false)
}

func (p *YellowPeach) syncAPIKeys(ctx context.Context, site models.Site, auth yellowpeachAuth, timeoutSeconds int, allowCreate bool) (models.JSONMap, string) {
	apiKeysURL := strings.TrimSpace(stringValue(site.PluginConfig, "api_keys_url", ""))
	if apiKeysURL == "" {
		apiKeysURL = "/api/token/?p=0&size=100"
	}
	payload, _, err := p.requestJSON(ctx, site, http.MethodGet, apiKeysURL, auth, nil, timeoutSeconds)
	if err != nil {
		return nil, ""
	}
	items := extractTokenItems(payload)
	if len(items) == 0 {
		if !allowCreate {
			return nil, ""
		}
		p.createAPIKey(ctx, site, auth, timeoutSeconds)
		payload, _, err = p.requestJSON(ctx, site, http.MethodGet, apiKeysURL, auth, nil, timeoutSeconds)
		if err != nil {
			return nil, ""
		}
		items = extractTokenItems(payload)
		if len(items) == 0 || !newAPITokenItemsHaveUsableKey(items) {
			return nil, ""
		}
	}

	keyByID := p.fetchTokenKeys(ctx, site, auth, tokenItemIDsNeedingKeys(items), timeoutSeconds)
	if len(keyByID) == 0 && !newAPITokenItemsHaveUsableKey(items) {
		if !allowCreate {
			return nil, ""
		}
		p.createAPIKey(ctx, site, auth, timeoutSeconds)
		payload, _, err = p.requestJSON(ctx, site, http.MethodGet, apiKeysURL, auth, nil, timeoutSeconds)
		if err != nil {
			return nil, ""
		}
		items = extractTokenItems(payload)
		keyByID = p.fetchTokenKeys(ctx, site, auth, tokenItemIDsNeedingKeys(items), timeoutSeconds)
	}
	preferredName := strings.TrimSpace(stringValue(site.PluginConfig, "preferred_api_key_name", ""))
	preferredID := strings.TrimSpace(stringValue(site.PluginConfig, "preferred_api_key_id", ""))
	credentialsUpdate := models.JSONMap{}
	apiKeys := []map[string]any{}
	var primary map[string]any
	for _, item := range items {
		id := tokenIDValue(firstExistingValue(item, "id", "token_id"))
		key := strings.TrimSpace(fmt.Sprint(firstExistingValue(item, "key", "token", "api_key", "value")))
		if !usableAPIKey(key) && id != "" {
			key = keyByID[id]
		}
		if !usableAPIKey(key) {
			continue
		}
		name := strings.TrimSpace(fmt.Sprint(firstExistingValue(item, "name", "token_name")))
		if name == "" || name == "<nil>" {
			name = id
		}
		status := tokenItemStatus(item)
		routeType := routeTypeFromAPIKeyItem(map[string]any{
			"name":       name,
			"status":     status,
			"api_format": firstExistingValue(item, "api_format", "type", "model_type", "provider"),
		}, site)
		entry := map[string]any{
			"id":         id,
			"name":       name,
			"key":        key,
			"status":     status,
			"route_type": routeType,
			"api_type":   routeType,
		}
		if supportedModels := apiKeySupportedModelsFromItem(item); len(supportedModels) > 0 {
			entry["supported_models"] = supportedModels
		}
		apiKeys = append(apiKeys, entry)
		if primary == nil && tokenItemMatchesPreference(entry, preferredName, preferredID) {
			primary = entry
		}
	}
	if len(apiKeys) == 0 {
		return nil, ""
	}
	if primary == nil {
		primary = apiKeys[0]
	}
	credentialsUpdate["api_keys"] = apiKeys
	return credentialsUpdate, strings.TrimSpace(fmt.Sprint(primary["key"]))
}

func newAPITokenItemsHaveUsableKey(items []map[string]any) bool {
	for _, item := range items {
		key := strings.TrimSpace(fmt.Sprint(firstExistingValue(item, "key", "token", "api_key", "value")))
		if usableAPIKey(key) {
			return true
		}
	}
	return false
}

func (p *YellowPeach) createAPIKey(ctx context.Context, site models.Site, auth yellowpeachAuth, timeoutSeconds int) {
	target := strings.TrimSpace(stringValue(site.PluginConfig, "create_api_key_url", ""))
	if target == "" {
		target = "/api/token/"
	}
	name := strings.TrimSpace(stringValue(site.PluginConfig, "default_api_key_name", ""))
	if name == "" {
		name = strings.TrimSpace(stringValue(site.Credentials, "account", ""))
	}
	if name == "" {
		name = "default"
	}
	body := map[string]any{
		"name":            name,
		"expired_time":    -1,
		"remain_quota":    0,
		"unlimited_quota": true,
	}
	_, _, _ = p.requestJSON(ctx, site, http.MethodPost, target, auth, body, timeoutSeconds)
}

func (p *YellowPeach) fetchPackageQuota(ctx context.Context, site models.Site, auth yellowpeachAuth, selfPayload map[string]any, timeoutSeconds int) packageQuotaSnapshot {
	if payload, _, err := p.requestJSON(ctx, site, http.MethodGet, "/api/subscription/self", auth, nil, timeoutSeconds); err == nil {
		if quota := newAPIQuotaFromSubscriptionPayload(site, payload); quota.hasQuota() {
			return quota
		}
	}
	if apiKey := strings.TrimSpace(stringValue(site.Credentials, "api_key", "")); apiKey != "" {
		tokenAuth := yellowpeachAuth{Authorization: normalizeYPAuthorization(apiKey), UserID: auth.UserID}
		if payload, _, err := p.requestJSON(ctx, site, http.MethodGet, "/api/usage/token/", tokenAuth, nil, timeoutSeconds); err == nil {
			if quota := newAPIQuotaFromTokenUsagePayload(site, payload); quota.hasQuota() {
				return quota
			}
		}
	}
	return packageQuotaFromPayload(site, selfPayload, "")
}

func newAPIQuotaFromSubscriptionPayload(site models.Site, payload map[string]any) packageQuotaSnapshot {
	items := newAPISubscriptionItems(payload)
	var best packageQuotaSnapshot
	for _, item := range items {
		subscription := item
		if nested, ok := item["subscription"].(map[string]any); ok {
			subscription = nested
		}
		status := strings.ToLower(strings.TrimSpace(fmt.Sprint(firstExistingValue(subscription, "status", "state"))))
		if status != "" && status != "<nil>" && status != "active" {
			continue
		}
		totalRaw := numberPtr(firstExistingValue(subscription, "amount_total", "total", "quota", "total_quota"))
		usedRaw := numberPtr(firstExistingValue(subscription, "amount_used", "used", "used_quota"))
		if totalRaw == nil && usedRaw == nil {
			continue
		}
		if usedRaw == nil {
			zero := 0.0
			usedRaw = &zero
		}
		var remainingRaw *float64
		if totalRaw != nil && *totalRaw > 0 {
			value := *totalRaw - *usedRaw
			remainingRaw = &value
		}
		if totalRaw != nil && *totalRaw == 0 {
			remainingRaw = nil
			usedRaw = nil
		}
		remaining := normalizeQuotaAmount(site, remainingRaw)
		total := normalizeQuotaAmount(site, totalRaw)
		used := normalizeQuotaAmount(site, usedRaw)
		label := newAPISubscriptionLabel(item, subscription)
		if totalRaw != nil && *totalRaw == 0 {
			label = strings.TrimSpace(label + " 无限套餐")
		}
		quota := packageQuotaSnapshot{
			Display:   formatPackageQuotaDisplay(label, remaining, total, used, "$"),
			Remaining: remaining,
			Total:     total,
			Used:      used,
			Unit:      "$",
		}
		if !best.hasQuota() || quotaRemainingValue(quota) > quotaRemainingValue(best) {
			best = quota
		}
	}
	return best
}

func newAPIQuotaFromTokenUsagePayload(site models.Site, payload map[string]any) packageQuotaSnapshot {
	data := payload
	if nested, ok := payload["data"].(map[string]any); ok {
		data = nested
	}
	if unlimited, ok := data["unlimited_quota"].(bool); ok && unlimited {
		name := strings.TrimSpace(fmt.Sprint(firstExistingValue(data, "name", "token_name")))
		if name == "" || name == "<nil>" {
			name = "Token"
		}
		return packageQuotaSnapshot{Display: name + " 无限额度", Unit: "$"}
	}
	totalRaw := numberPtr(firstExistingValue(data, "total_granted", "total", "remain_quota"))
	usedRaw := numberPtr(firstExistingValue(data, "total_used", "used", "used_quota"))
	remainingRaw := numberPtr(firstExistingValue(data, "total_available", "remaining", "remain_quota"))
	if remainingRaw == nil && totalRaw != nil && usedRaw != nil {
		value := *totalRaw - *usedRaw
		remainingRaw = &value
	}
	if remainingRaw == nil && totalRaw == nil && usedRaw == nil {
		return packageQuotaSnapshot{}
	}
	name := strings.TrimSpace(fmt.Sprint(firstExistingValue(data, "name", "token_name")))
	if name == "" || name == "<nil>" {
		name = "Token"
	}
	remaining := normalizeQuotaAmount(site, remainingRaw)
	total := normalizeQuotaAmount(site, totalRaw)
	used := normalizeQuotaAmount(site, usedRaw)
	return packageQuotaSnapshot{
		Display:   formatPackageQuotaDisplay(name+" Token 额度", remaining, total, used, "$"),
		Remaining: remaining,
		Total:     total,
		Used:      used,
		Unit:      "$",
	}
}

func newAPISubscriptionItems(payload map[string]any) []map[string]any {
	if payload == nil {
		return nil
	}
	for _, container := range []any{payload["data"], payload} {
		obj, ok := container.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"subscriptions", "all_subscriptions", "items", "list"} {
			if values, ok := obj[key].([]any); ok {
				return mapDictArray(values)
			}
		}
	}
	return nil
}

func newAPISubscriptionLabel(item, subscription map[string]any) string {
	for _, source := range []map[string]any{item, subscription} {
		if source == nil {
			continue
		}
		if plan, ok := source["plan"].(map[string]any); ok {
			if value := strings.TrimSpace(fmt.Sprint(firstExistingValue(plan, "title", "name"))); value != "" && value != "<nil>" {
				return value
			}
		}
		if value := strings.TrimSpace(fmt.Sprint(firstExistingValue(source, "plan_title", "plan_name", "name", "title"))); value != "" && value != "<nil>" {
			return value
		}
	}
	return "New API 套餐"
}

func quotaRemainingValue(quota packageQuotaSnapshot) float64 {
	if quota.Remaining != nil {
		return *quota.Remaining
	}
	if quota.Total != nil && quota.Used != nil {
		return *quota.Total - *quota.Used
	}
	if quota.Total != nil {
		return *quota.Total
	}
	return 0
}

func extractTokenItems(payload map[string]any) []map[string]any {
	items := extractItems(payload)
	if len(items) > 0 {
		return items
	}
	if payload == nil {
		return nil
	}
	for _, container := range []any{payload["data"], payload} {
		obj, ok := container.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"tokens", "list", "records", "rows"} {
			if values, ok := obj[key].([]any); ok {
				return mapDictArray(values)
			}
		}
	}
	return nil
}

func tokenItemIDsNeedingKeys(items []map[string]any) []string {
	ids := []string{}
	for _, item := range items {
		key := strings.TrimSpace(fmt.Sprint(firstExistingValue(item, "key", "token", "api_key", "value")))
		if usableAPIKey(key) {
			continue
		}
		id := tokenIDValue(firstExistingValue(item, "id", "token_id"))
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func (p *YellowPeach) fetchTokenKeys(ctx context.Context, site models.Site, auth yellowpeachAuth, ids []string, timeoutSeconds int) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	target := strings.TrimSpace(stringValue(site.PluginConfig, "token_keys_url", ""))
	if target == "" {
		target = "/api/token/batch/keys"
	}
	payload, _, err := p.requestJSON(ctx, site, http.MethodPost, target, auth, map[string]any{"ids": ids}, timeoutSeconds)
	if err == nil {
		for id, key := range tokenKeysFromPayload(payload) {
			out[id] = key
		}
	}
	for _, id := range ids {
		if out[id] != "" {
			continue
		}
		for _, endpoint := range tokenDetailEndpoints(site, id) {
			payload, _, err := p.requestJSON(ctx, site, endpoint.method, endpoint.target, auth, nil, timeoutSeconds)
			if err != nil {
				continue
			}
			keys := tokenKeysFromPayload(payload)
			if out[id] == "" && len(keys) == 1 {
				for _, key := range keys {
					out[id] = key
				}
			}
			for gotID, key := range tokenKeysFromPayload(payload) {
				if gotID == "" || gotID == id {
					out[id] = key
					break
				}
			}
			if out[id] != "" {
				break
			}
		}
	}
	return out
}

type tokenDetailEndpoint struct {
	method string
	target string
}

func tokenDetailEndpoints(site models.Site, id string) []tokenDetailEndpoint {
	escapedID := url.PathEscape(id)
	endpoints := []tokenDetailEndpoint{}
	custom := strings.TrimSpace(stringValue(site.PluginConfig, "token_keys_url", ""))
	if custom != "" && strings.Contains(custom, "{id}") {
		target := strings.ReplaceAll(custom, "{id}", escapedID)
		endpoints = append(endpoints,
			tokenDetailEndpoint{method: http.MethodPost, target: target},
			tokenDetailEndpoint{method: http.MethodGet, target: target},
		)
	}
	endpoints = append(endpoints,
		tokenDetailEndpoint{method: http.MethodPost, target: fmt.Sprintf("/api/token/%s/key", escapedID)},
		tokenDetailEndpoint{method: http.MethodGet, target: fmt.Sprintf("/api/token/%s/key", escapedID)},
		tokenDetailEndpoint{method: http.MethodGet, target: fmt.Sprintf("/api/token/%s", escapedID)},
		tokenDetailEndpoint{method: http.MethodPost, target: fmt.Sprintf("/api/token/key/%s", escapedID)},
		tokenDetailEndpoint{method: http.MethodGet, target: fmt.Sprintf("/api/token/key/%s", escapedID)},
	)
	return endpoints
}

func tokenKeysFromPayload(payload map[string]any) map[string]string {
	out := map[string]string{}
	for _, container := range []any{payload["data"], payload} {
		switch typed := container.(type) {
		case map[string]any:
			id := tokenIDValue(firstExistingValue(typed, "id", "token_id"))
			key := strings.TrimSpace(fmt.Sprint(firstExistingValue(typed, "key", "token", "api_key", "value")))
			if usableAPIKey(key) {
				out[id] = key
			}
			for rawID, rawKey := range typed {
				if key := strings.TrimSpace(fmt.Sprint(rawKey)); usableAPIKey(key) {
					out[rawID] = key
				}
			}
			for _, listKey := range []string{"items", "tokens", "list", "records", "rows"} {
				if values, ok := typed[listKey].([]any); ok {
					for _, item := range mapDictArray(values) {
						id := tokenIDValue(firstExistingValue(item, "id", "token_id"))
						key := strings.TrimSpace(fmt.Sprint(firstExistingValue(item, "key", "token", "api_key", "value")))
						if usableAPIKey(key) {
							out[id] = key
						}
					}
				}
			}
		case []any:
			for _, item := range mapDictArray(typed) {
				id := tokenIDValue(firstExistingValue(item, "id", "token_id"))
				key := strings.TrimSpace(fmt.Sprint(firstExistingValue(item, "key", "token", "api_key", "value")))
				if usableAPIKey(key) {
					out[id] = key
				}
			}
		}
	}
	return out
}

func tokenIDValue(value any) string {
	id := strings.TrimSpace(fmt.Sprint(value))
	if id == "" || id == "<nil>" {
		return ""
	}
	return id
}

func tokenItemStatus(item map[string]any) string {
	if enabled, ok := item["enabled"].(bool); ok {
		if enabled {
			return "active"
		}
		return "disabled"
	}
	status := strings.TrimSpace(fmt.Sprint(firstExistingValue(item, "status", "state")))
	if status == "" || status == "<nil>" {
		return "active"
	}
	return status
}

func tokenItemMatchesPreference(item map[string]any, preferredName, preferredID string) bool {
	if preferredID != "" && strings.TrimSpace(fmt.Sprint(item["id"])) == preferredID {
		return true
	}
	return preferredName != "" && strings.EqualFold(strings.TrimSpace(fmt.Sprint(item["name"])), preferredName)
}

func usableAPIKey(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "<nil>" {
		return false
	}
	return !strings.Contains(value, "***") && !strings.Contains(value, "****")
}

func firstExistingValue(item map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			return value
		}
	}
	return nil
}

func (p *YellowPeach) quotaPerUnit(site models.Site) float64 {
	return quotaPerUnitFromConfig(site)
}

func (p *YellowPeach) extractBalance(site models.Site, payload map[string]any) *float64 {
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil
	}
	quota := p.quotaPerUnit(site)
	for _, key := range []string{"quota", "remain_quota", "balance", "quota_remain"} {
		raw, ok := data[key]
		if !ok || raw == nil {
			continue
		}
		var numeric float64
		switch typed := raw.(type) {
		case float64:
			numeric = typed
		case string:
			if _, err := fmt.Sscan(strings.TrimSpace(typed), &numeric); err != nil {
				continue
			}
		default:
			continue
		}
		if (key == "quota" || key == "remain_quota" || key == "quota_remain") && (numeric >= 1000 || numeric <= -1000) {
			result := numeric / quota
			return &result
		}
		if key == "balance" && (numeric >= 1000 || numeric <= -1000) {
			result := numeric / quota
			return &result
		}
		return &numeric
	}
	return nil
}

func (p *YellowPeach) extractBalanceUnit(payload map[string]any) string {
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"quota_unit", "currency", "unit", "balance_unit"} {
		if v, ok := data[key]; ok && v != nil && fmt.Sprint(v) != "" {
			return fmt.Sprint(v)
		}
	}
	return "$"
}

func (p *YellowPeach) extractAccountName(site models.Site, payload map[string]any) string {
	data, ok := payload["data"].(map[string]any)
	if ok {
		for _, key := range []string{"username", "display_name", "nickname", "email"} {
			if v, ok := data[key]; ok && v != nil && fmt.Sprint(v) != "" {
				return fmt.Sprint(v)
			}
		}
	}
	if v := strings.TrimSpace(stringValue(site.Credentials, "account", "")); v != "" {
		return v
	}
	return strings.TrimSpace(stringValue(site.Credentials, "username", ""))
}

func extractYPUserID(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if data, ok := payload["data"].(map[string]any); ok {
		for _, key := range []string{"id", "user_id", "uid"} {
			if v, ok := data[key]; ok && v != nil && fmt.Sprint(v) != "" {
				return fmt.Sprint(v)
			}
		}
	}
	for _, key := range []string{"id", "user_id", "uid"} {
		if v, ok := payload[key]; ok && v != nil && fmt.Sprint(v) != "" {
			return fmt.Sprint(v)
		}
	}
	return ""
}

func normalizeYPCookie(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "session=") {
		return v
	}
	return "session=" + v
}

func normalizeYPAuthorization(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(v), "bearer ") {
		return v
	}
	return "Bearer " + v
}
