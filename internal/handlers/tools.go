package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"ai-sign-in-gateway/internal/httpx"
	"ai-sign-in-gateway/internal/schemas"
	"ai-sign-in-gateway/internal/services"
	"github.com/go-chi/chi/v5"
)

func (a *App) ToolRoutes(r chi.Router) {
	r.Post("/connectivity-test", a.ConnectivityTest)
	r.Post("/chat-test", a.ChatTest)
	r.Post("/mcp-test", a.MCPTest)
}

func (a *App) ConnectivityTest(w http.ResponseWriter, r *http.Request) {
	var payload schemas.ConnectivityTestRequest
	if err := httpx.Decode(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	result := openAIGet(r, payload.BaseURL, payload.APIKey, "/models", 20*time.Second)
	models := []string{}
	if result.data != nil {
		if items, ok := result.data["data"].([]any); ok {
			for _, item := range items {
				if len(models) >= 8 {
					break
				}
				if obj, ok := item.(map[string]any); ok {
					if id, ok := obj["id"].(string); ok && id != "" {
						models = append(models, id)
					}
				}
			}
		}
	}
	message := "连接正常。"
	if !result.ok {
		message = result.message
	}
	writeJSON(w, http.StatusOK, schemas.ConnectivityTestResponse{OK: result.ok, StatusCode: result.statusCode, LatencyMS: result.latencyMS, Message: message, Models: models})
}

func (a *App) ChatTest(w http.ResponseWriter, r *http.Request) {
	var payload schemas.ChatTestRequest
	if err := httpx.Decode(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	body := map[string]any{"model": payload.Model, "messages": []map[string]string{{"role": "user", "content": payload.Prompt}}, "temperature": 0.2}
	result := openAIPost(r, payload.BaseURL, payload.APIKey, "/chat/completions", body, 30*time.Second)
	output := ""
	if result.data != nil {
		if choices, ok := result.data["choices"].([]any); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]any); ok {
				if message, ok := choice["message"].(map[string]any); ok {
					output, _ = message["content"].(string)
				}
			}
		}
	}
	message := "测试完成。"
	if !result.ok {
		message = result.message
	}
	writeJSON(w, http.StatusOK, schemas.ChatTestResponse{OK: result.ok, StatusCode: result.statusCode, LatencyMS: result.latencyMS, Message: message, Output: output})
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

func openAIGet(r *http.Request, baseURL, apiKey, path string, timeout time.Duration) upstreamResult {
	return openAIRequest(r, http.MethodGet, baseURL, apiKey, path, nil, timeout)
}

func openAIPost(r *http.Request, baseURL, apiKey, path string, body any, timeout time.Duration) upstreamResult {
	data, _ := json.Marshal(body)
	return openAIRequest(r, http.MethodPost, baseURL, apiKey, path, bytes.NewReader(data), timeout)
}

func openAIRequest(r *http.Request, method, baseURL, apiKey, path string, body io.Reader, timeout time.Duration) upstreamResult {
	start := time.Now()
	target := services.NormalizeBaseURL(baseURL) + path
	req, err := http.NewRequestWithContext(r.Context(), method, target, body)
	if err != nil {
		return upstreamResult{message: "请求构造失败：" + err.Error()}
	}
	for key, value := range services.BuildBrowserHeaders(baseURL, body != nil, bearer(apiKey), "", nil) {
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
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
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
	}
	return upstreamResult{ok: ok, statusCode: &status, latencyMS: &latency, message: message, data: data}
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
