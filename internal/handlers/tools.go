package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"sort"
	"strings"
	"time"

	"ai-sign-in-gateway/internal/httpx"
	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/schemas"
	"ai-sign-in-gateway/internal/services"
	"github.com/go-chi/chi/v5"
)

func (a *App) ToolRoutes(r chi.Router) {
	r.Post("/models", a.ModelList)
	r.Post("/chat-test", a.ChatTest)
	r.Post("/mcp-test", a.MCPTest)
}

func (a *App) ModelList(w http.ResponseWriter, r *http.Request) {
	var payload schemas.ModelListRequest
	if err := httpx.Decode(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	site, ok := a.toolSite(w, payload.SiteID)
	if !ok {
		return
	}
	candidates := toolModelCandidates(site)
	if len(candidates) == 0 {
		writeJSON(w, http.StatusOK, schemas.ModelListResponse{OK: false, Message: "站点没有可用 API Key 或请求 API URL", Models: []string{}, Items: []schemas.ModelListItem{}})
		return
	}
	var last upstreamResult
	var authFailure *upstreamResult
	items := []schemas.ModelListItem{}
	for _, candidate := range candidates {
		result := openAIGet(r, candidate.BaseURL, candidate.APIKey, modelListPath(candidate.RouteType), 25*time.Second, candidate.RouteType)
		last = result
		if isUpstreamAuthFailure(result) && authFailure == nil {
			copied := result
			authFailure = &copied
		}
		for _, modelID := range extractModelIDs(result.data) {
			items = append(items, schemas.ModelListItem{
				ID:             modelID,
				RouteType:      candidate.RouteType,
				Mode:           inferChatMode(modelID),
				BaseURL:        candidate.BaseURL,
				KeyFingerprint: candidate.KeyFingerprint,
				KeyName:        candidate.KeyName,
			})
		}
	}
	items = normalizeModelItems(items)
	modelIDs := make([]string, 0, len(items))
	for _, item := range items {
		modelIDs = append(modelIDs, item.ID)
	}
	message := "模型列表已加载。"
	if len(items) == 0 {
		if authFailure != nil {
			last = *authFailure
		}
		message = "未获取到模型列表。"
		if !last.ok && strings.TrimSpace(last.message) != "" {
			message = last.message
		}
		if last.statusCode != nil && *last.statusCode == http.StatusNotFound {
			message = fmt.Sprintf("%s 请检查站点的 API 请求 URL 是否是模型请求根地址，或该上游是否支持 /models。", message)
		}
	}
	response := schemas.ModelListResponse{OK: len(items) > 0, Message: message, Models: modelIDs, Items: items}
	if last.statusCode != nil {
		response.StatusCode = last.statusCode
		response.LatencyMS = last.latencyMS
	}
	if len(candidates) > 0 {
		response.BaseURL = candidates[0].BaseURL
		response.RouteType = candidates[0].RouteType
		response.KeyFingerprint = candidates[0].KeyFingerprint
		response.KeyName = candidates[0].KeyName
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) ChatTest(w http.ResponseWriter, r *http.Request) {
	var payload schemas.ChatTestRequest
	if err := httpx.Decode(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if err := a.resolveChatTarget(w, &payload); err != nil {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(payload.Mode))
	if mode == "" || mode == "auto" {
		mode = inferChatMode(payload.Model)
	}
	if mode == "image" {
		a.chatImageTest(w, r, payload)
		return
	}
	messages := buildChatCompletionMessages(payload)
	body := map[string]any{"model": payload.Model, "messages": messages, "temperature": 0.2}
	result := openAIPost(r, payload.BaseURL, payload.APIKey, "/chat/completions", body, 30*time.Second, payload.RouteType)
	output := ""
	if result.data != nil {
		output = extractChatOutput(result.data)
	}
	message := "测试完成。"
	if !result.ok {
		message = result.message
	}
	writeJSON(w, http.StatusOK, schemas.ChatTestResponse{OK: result.ok, StatusCode: result.statusCode, LatencyMS: result.latencyMS, Message: message, Output: output})
}

func (a *App) chatImageTest(w http.ResponseWriter, r *http.Request, payload schemas.ChatTestRequest) {
	prompt := strings.TrimSpace(payload.Prompt)
	if prompt == "" && len(payload.Messages) > 0 {
		prompt = strings.TrimSpace(payload.Messages[len(payload.Messages)-1].Content)
	}
	if prompt == "" {
		writeError(w, http.StatusBadRequest, "请输入图片生成提示词")
		return
	}
	imageSize := strings.TrimSpace(payload.ImageSize)
	if imageSize == "" {
		imageSize = "1024x1024"
	}
	var result upstreamResult
	if len(payload.ReferenceImgs) > 0 {
		result = openAIImageEditPost(r, payload.BaseURL, payload.APIKey, payload.Model, prompt, imageSize, payload.ReferenceImgs, 120*time.Second, payload.RouteType)
	} else {
		body := map[string]any{"model": payload.Model, "prompt": prompt, "size": imageSize}
		result = openAIPost(r, payload.BaseURL, payload.APIKey, "/images/generations", body, 120*time.Second, payload.RouteType)
	}
	images := []schemas.ChatTestImageOutput{}
	revisedPrompt := ""
	if result.data != nil {
		images, revisedPrompt = extractImageOutputs(result.data)
	}
	message := "图片生成完成。"
	if !result.ok {
		message = result.message
	}
	writeJSON(w, http.StatusOK, schemas.ChatTestResponse{OK: result.ok, StatusCode: result.statusCode, LatencyMS: result.latencyMS, Message: message, Images: images, RevisedPrompt: revisedPrompt})
}

func buildChatCompletionMessages(payload schemas.ChatTestRequest) []map[string]any {
	source := payload.Messages
	if len(source) == 0 {
		source = []schemas.ChatTestMessage{{Role: "user", Content: payload.Prompt, ReferenceImages: payload.ReferenceImgs}}
	}
	out := make([]map[string]any, 0, len(source))
	for _, item := range source {
		role := strings.TrimSpace(item.Role)
		if role != "assistant" && role != "system" && role != "user" {
			role = "user"
		}
		text := strings.TrimSpace(item.Content)
		if len(item.ReferenceImages) == 0 {
			out = append(out, map[string]any{"role": role, "content": text})
			continue
		}
		parts := []map[string]any{}
		if text != "" {
			parts = append(parts, map[string]any{"type": "text", "text": text})
		}
		for _, image := range item.ReferenceImages {
			if strings.TrimSpace(image.URL) == "" {
				continue
			}
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": image.URL}})
		}
		if len(parts) == 0 {
			continue
		}
		out = append(out, map[string]any{"role": role, "content": parts})
	}
	if len(out) == 0 {
		out = append(out, map[string]any{"role": "user", "content": payload.Prompt})
	}
	return out
}

func extractChatOutput(data map[string]any) string {
	if choices, ok := data["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if message, ok := choice["message"].(map[string]any); ok {
				switch content := message["content"].(type) {
				case string:
					return content
				case []any:
					parts := []string{}
					for _, part := range content {
						if obj, ok := part.(map[string]any); ok {
							if text, ok := obj["text"].(string); ok && text != "" {
								parts = append(parts, text)
							}
						}
					}
					return strings.Join(parts, "\n")
				}
			}
		}
	}
	return ""
}

func extractImageOutputs(data map[string]any) ([]schemas.ChatTestImageOutput, string) {
	items, _ := data["data"].([]any)
	images := make([]schemas.ChatTestImageOutput, 0, len(items))
	revisedPrompt := ""
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		image := schemas.ChatTestImageOutput{}
		image.URL, _ = obj["url"].(string)
		image.B64JSON, _ = obj["b64_json"].(string)
		image.RevisedPrompt, _ = obj["revised_prompt"].(string)
		if revisedPrompt == "" {
			revisedPrompt = image.RevisedPrompt
		}
		if image.URL != "" || image.B64JSON != "" {
			images = append(images, image)
		}
	}
	return images, revisedPrompt
}

func (a *App) MCPTest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, schemas.McpTestResponse{OK: false, Message: "Go MCP 测试待接入。", ToolEvents: []string{}})
}

type upstreamResult struct {
	ok         bool
	statusCode *int
	latencyMS  *float64
	message    string
	data       map[string]any
}

type toolModelCandidate struct {
	BaseURL        string
	APIKey         string
	RouteType      string
	KeyFingerprint string
	KeyName        string
}

type toolSiteKey struct {
	Value     string
	Name      string
	RouteType string
}

func (a *App) toolSite(w http.ResponseWriter, siteID uint) (models.Site, bool) {
	if siteID == 0 {
		writeError(w, http.StatusBadRequest, "请选择站点")
		return models.Site{}, false
	}
	var site models.Site
	if err := a.DB.First(&site, siteID).Error; err != nil {
		writeError(w, http.StatusNotFound, "站点不存在")
		return models.Site{}, false
	}
	return site, true
}

func (a *App) resolveChatTarget(w http.ResponseWriter, payload *schemas.ChatTestRequest) error {
	if payload.SiteID == 0 {
		if strings.TrimSpace(payload.BaseURL) == "" || strings.TrimSpace(payload.APIKey) == "" {
			writeError(w, http.StatusBadRequest, "请选择站点")
			return fmt.Errorf("site required")
		}
		return nil
	}
	site, ok := a.toolSite(w, payload.SiteID)
	if !ok {
		return fmt.Errorf("site not found")
	}
	candidate, ok := pickToolModelCandidate(site, payload.KeyFingerprint, payload.RouteType)
	if !ok {
		writeError(w, http.StatusBadRequest, "站点没有匹配的 API Key 或请求 API URL")
		return fmt.Errorf("candidate not found")
	}
	payload.BaseURL = candidate.BaseURL
	payload.APIKey = candidate.APIKey
	payload.RouteType = candidate.RouteType
	return nil
}

func openAIGet(r *http.Request, baseURL, apiKey, path string, timeout time.Duration, routeType ...string) upstreamResult {
	return openAIRequest(r, http.MethodGet, baseURL, apiKey, path, nil, timeout, routeType...)
}

func openAIPost(r *http.Request, baseURL, apiKey, path string, body any, timeout time.Duration, routeType ...string) upstreamResult {
	data, _ := json.Marshal(body)
	return openAIRequest(r, http.MethodPost, baseURL, apiKey, path, bytes.NewReader(data), timeout, routeType...)
}

func openAIImageEditPost(r *http.Request, baseURL, apiKey, model, prompt, size string, images []schemas.ChatTestImageRef, timeout time.Duration, routeType ...string) upstreamResult {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", model)
	_ = writer.WriteField("prompt", prompt)
	if size != "" {
		_ = writer.WriteField("size", size)
	}
	for idx, image := range images {
		raw, contentType, err := decodeDataImage(image.URL)
		if err != nil {
			return upstreamResult{message: err.Error()}
		}
		name := strings.TrimSpace(image.Name)
		if name == "" {
			name = fmt.Sprintf("reference-%d.png", idx+1)
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image[]"; filename="%s"`, escapeMultipartFilename(name)))
		header.Set("Content-Type", contentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return upstreamResult{message: "图片请求构造失败：" + err.Error()}
		}
		if _, err := part.Write(raw); err != nil {
			return upstreamResult{message: "图片写入失败：" + err.Error()}
		}
	}
	if err := writer.Close(); err != nil {
		return upstreamResult{message: "图片请求收尾失败：" + err.Error()}
	}
	return openAIRequestWithHeaders(r, http.MethodPost, baseURL, apiKey, "/images/edits", bytes.NewReader(body.Bytes()), timeout, map[string]string{"Content-Type": writer.FormDataContentType()}, routeType...)
}

func openAIRequest(r *http.Request, method, baseURL, apiKey, path string, body io.Reader, timeout time.Duration, routeType ...string) upstreamResult {
	return openAIRequestWithHeaders(r, method, baseURL, apiKey, path, body, timeout, nil, routeType...)
}

func openAIRequestWithHeaders(r *http.Request, method, baseURL, apiKey, path string, body io.Reader, timeout time.Duration, extraHeaders map[string]string, routeType ...string) upstreamResult {
	start := time.Now()
	rt := ""
	if len(routeType) > 0 {
		rt = routeType[0]
	}
	target, err := services.GatewayTargetURL(baseURL, path, "", rt)
	if err != nil {
		return upstreamResult{message: "请求地址构造失败：" + err.Error()}
	}
	req, err := http.NewRequestWithContext(r.Context(), method, target, body)
	if err != nil {
		return upstreamResult{message: "请求构造失败：" + err.Error()}
	}
	for key, value := range services.BuildBrowserHeaders(baseURL, body != nil, bearer(apiKey), "", toolAPIKeyHeaders(apiKey)) {
		req.Header.Set(key, value)
	}
	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	latency := float64(time.Since(start).Microseconds()) / 1000
	if err != nil {
		return upstreamResult{latencyMS: &latency, message: "请求失败：" + err.Error()}
	}
	defer resp.Body.Close()
	status := resp.StatusCode
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	var data map[string]any
	_ = json.Unmarshal(raw, &data)
	ok := status >= 200 && status < 300
	message := "接口返回 " + http.StatusText(status)
	if !ok {
		message = "接口返回 " + http.StatusText(status)
		if len(raw) > 0 {
			message = "接口返回 " + string(raw)
			if len(message) > 300 {
				message = message[:300] + "..."
			}
		}
		if strings.Contains(strings.ToUpper(message), "API_KEY_REQUIRED") {
			message = message + "。上游提示未收到 API Key，请确认该站点保存了 api_key，且模型请求应使用 Authorization: Bearer、x-api-key 或 x-goog-api-key。"
		}
	}
	return upstreamResult{ok: ok, statusCode: &status, latencyMS: &latency, message: message, data: data}
}

func toolModelCandidates(site models.Site) []toolModelCandidate {
	keys := toolSiteAPIKeys(site)
	baseURLs := services.GatewayRequestBaseCandidates(site)
	if len(baseURLs) == 0 {
		baseURLs = []string{services.NormalizeBaseURL(site.BaseURL)}
	}
	baseURLs = normalizeToolStringList(baseURLs)
	candidates := []toolModelCandidate{}
	for _, key := range keys {
		routeType := firstNonEmpty(key.RouteType, toolInferRouteType(site), "codex")
		for _, baseURL := range baseURLs {
			candidates = append(candidates, toolModelCandidate{
				BaseURL:        baseURL,
				APIKey:         key.Value,
				RouteType:      routeType,
				KeyFingerprint: toolFingerprint(key.Value),
				KeyName:        key.Name,
			})
		}
	}
	return candidates
}

func pickToolModelCandidate(site models.Site, keyFingerprint, routeType string) (toolModelCandidate, bool) {
	candidates := toolModelCandidates(site)
	if len(candidates) == 0 {
		return toolModelCandidate{}, false
	}
	keyFingerprint = strings.TrimSpace(keyFingerprint)
	routeType = toolNormalizeRouteType(routeType)
	for _, candidate := range candidates {
		if keyFingerprint != "" && candidate.KeyFingerprint != keyFingerprint {
			continue
		}
		if routeType != "" && candidate.RouteType != routeType {
			continue
		}
		return candidate, true
	}
	return candidates[0], true
}

func toolSiteAPIKeys(site models.Site) []toolSiteKey {
	keys := []toolSiteKey{}
	seen := map[string]bool{}
	if rawKeys, ok := site.Credentials["api_keys"].([]any); ok {
		for _, item := range rawKeys {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			value := strings.TrimSpace(fmt.Sprint(obj["key"]))
			status := strings.ToLower(strings.TrimSpace(fmt.Sprint(obj["status"])))
			if value == "" || seen[value] || status == "disabled" || status == "inactive" || status == "revoked" {
				continue
			}
			routeType := toolNormalizeRouteType(firstNonEmpty(fmt.Sprint(obj["route_type"]), fmt.Sprint(obj["api_type"]), fmt.Sprint(obj["api_format"]), fmt.Sprint(obj["type"])))
			seen[value] = true
			keys = append(keys, toolSiteKey{Value: value, Name: strings.TrimSpace(fmt.Sprint(obj["name"])), RouteType: routeType})
		}
	}
	value := strings.TrimSpace(jsonMapString(site.Credentials, "api_key"))
	if value != "" && !seen[value] {
		keys = append(keys, toolSiteKey{Value: value, Name: "默认 Key", RouteType: toolInferRouteType(site)})
	}
	return keys
}

func toolInferRouteType(site models.Site) string {
	return firstNonEmpty(
		toolNormalizeRouteType(jsonMapString(site.PluginConfig, "gateway_route_type")),
		toolNormalizeRouteType(jsonMapString(site.PluginConfig, "api_format")),
		"codex",
	)
}

func toolNormalizeRouteType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "claude", "anthropic":
		return "claude"
	case "gemini", "google":
		return "gemini"
	case "codex", "gpt", "openai", "chatgpt":
		return "codex"
	default:
		return ""
	}
}

func modelListPath(routeType string) string {
	switch toolNormalizeRouteType(routeType) {
	case "claude":
		return "/models"
	default:
		return "/models"
	}
}

func extractModelIDs(data map[string]any) []string {
	if data == nil {
		return nil
	}
	var rawItems []any
	if items, ok := data["data"].([]any); ok {
		rawItems = items
	} else if items, ok := data["models"].([]any); ok {
		rawItems = items
	} else {
		return nil
	}
	out := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		switch typed := item.(type) {
		case string:
			out = append(out, typed)
		case map[string]any:
			for _, key := range []string{"id", "name", "model"} {
				value := strings.TrimSpace(fmt.Sprint(typed[key]))
				if value != "" && value != "<nil>" {
					out = append(out, value)
					break
				}
			}
		}
	}
	return out
}

func normalizeModelItems(items []schemas.ModelListItem) []schemas.ModelListItem {
	seen := map[string]bool{}
	out := make([]schemas.ModelListItem, 0, len(items))
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" || item.ID == "<nil>" {
			continue
		}
		key := item.ID + "\x00" + item.RouteType + "\x00" + item.KeyFingerprint + "\x00" + item.BaseURL
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func inferChatMode(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(normalized, "gpt-image") ||
		strings.Contains(normalized, "dall-e") ||
		strings.Contains(normalized, "image-generation") ||
		strings.Contains(normalized, "image_generation") ||
		strings.Contains(normalized, "imagen") {
		return "image"
	}
	return "chat"
}

func normalizeToolStringList(values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || containsToolString(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func containsToolString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func toolFingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func toolAPIKeyHeaders(apiKey string) map[string]string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil
	}
	value := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(apiKey, "Bearer "), "bearer "))
	if value == "" {
		return nil
	}
	return map[string]string{
		"x-api-key":      value,
		"x-goog-api-key": value,
	}
}

func isUpstreamAuthFailure(result upstreamResult) bool {
	if result.statusCode != nil && (*result.statusCode == http.StatusUnauthorized || *result.statusCode == http.StatusForbidden) {
		return true
	}
	upper := strings.ToUpper(result.message)
	return strings.Contains(upper, "API_KEY_REQUIRED") || strings.Contains(upper, "INVALID_API_KEY")
}

func decodeDataImage(value string) ([]byte, string, error) {
	if !strings.HasPrefix(value, "data:image/") {
		return nil, "", fmt.Errorf("参考图必须是 data:image/* base64 数据")
	}
	header, encoded, ok := strings.Cut(value, ",")
	if !ok {
		return nil, "", fmt.Errorf("参考图数据格式不正确")
	}
	contentType := strings.TrimPrefix(strings.TrimSpace(header), "data:")
	if semi := strings.Index(contentType, ";"); semi >= 0 {
		contentType = contentType[:semi]
	}
	if contentType == "" {
		contentType = "image/png"
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, "", fmt.Errorf("参考图 base64 解码失败：%w", err)
	}
	return raw, contentType, nil
}

func escapeMultipartFilename(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(value)
}

func bearer(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	if len(apiKey) >= 7 && apiKey[:7] == "Bearer " {
		return apiKey
	}
	return "Bearer " + apiKey
}
