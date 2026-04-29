package services

import (
	"net/url"
	"strings"
)

const DefaultBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36"

func BuildBrowserHeaders(baseURL string, includeContentType bool, authorization string, cookie string, extra map[string]string) map[string]string {
	origin := strings.TrimRight(baseURL, "/")
	headers := map[string]string{
		"Accept":             "application/json, text/plain, */*",
		"Accept-Language":    "zh-CN,zh;q=0.9,en;q=0.8",
		"Cache-Control":      "no-cache",
		"Pragma":             "no-cache",
		"Sec-Fetch-Dest":     "empty",
		"Sec-Fetch-Mode":     "cors",
		"Sec-Fetch-Site":     "same-origin",
		"User-Agent":         DefaultBrowserUserAgent,
		"Origin":             origin,
		"Referer":            origin + "/",
		"Sec-CH-UA-Mobile":   "?0",
		"Sec-CH-UA-Platform": `"Windows"`,
	}
	if includeContentType {
		headers["Content-Type"] = "application/json;charset=UTF-8"
	}
	if authorization != "" {
		headers["Authorization"] = authorization
	}
	if cookie != "" {
		headers["Cookie"] = cookie
	}
	for key, value := range extra {
		headers[key] = value
	}
	return headers
}

func NormalizeBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func JoinURL(baseURL, target string) (string, error) {
	normalized := strings.TrimSpace(target)
	if normalized == "" {
		return "", nil
	}
	if strings.HasPrefix(normalized, "http://") || strings.HasPrefix(normalized, "https://") {
		return normalized, nil
	}
	base, err := url.Parse(NormalizeBaseURL(baseURL) + "/")
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(normalized)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}
