package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/services"
)

type inviteRequestSpec struct {
	Method string
	Target string
	Body   any
}

var defaultInviteLinkPaths = []string{
	"data.aff_link",
	"data.invite_link",
	"data.invitation_link",
	"data.referral_link",
	"data.affiliate_link",
	"data.invite_url",
	"data.referral_url",
	"data.aff.url",
	"data.invite.url",
	"data.invitation.url",
	"data.referral.url",
	"data.links.invite",
	"data.links.referral",
	"aff_link",
	"invite_link",
	"invitation_link",
	"referral_link",
	"affiliate_link",
	"invite_url",
	"referral_url",
}

var defaultInviteCodePaths = []string{
	"data.aff_code",
	"data.invite_code",
	"data.invitation_code",
	"data.referral_code",
	"data.affiliate_code",
	"data.aff.code",
	"data.invite.code",
	"data.invitation.code",
	"data.referral.code",
	"data.affCode",
	"data.inviteCode",
	"data.invitationCode",
	"data.referralCode",
	"data.code",
	"aff_code",
	"invite_code",
	"invitation_code",
	"referral_code",
	"affiliate_code",
	"affCode",
	"inviteCode",
	"invitationCode",
	"referralCode",
}

func extractInviteInfo(payload map[string]any, site models.Site) (*string, *string) {
	return extractInviteInfoWithConfig(payload, site, []string{"status_invite_link_path"}, []string{"status_invite_code_path"})
}

func extractInviteInfoWithConfig(payload map[string]any, site models.Site, linkConfigKeys []string, codeConfigKeys []string) (*string, *string) {
	linkPaths := append(invitePaths(site, linkConfigKeys...), defaultInviteLinkPaths...)
	codePaths := append(invitePaths(site, codeConfigKeys...), defaultInviteCodePaths...)

	link := firstNonEmptyPathString(payload, linkPaths)
	code := firstNonEmptyPathString(payload, codePaths)

	if link != "" {
		if normalized, err := services.JoinURL(site.BaseURL, link); err == nil && normalized != "" {
			link = normalized
		}
		if code == "" {
			code = extractInviteCodeFromLink(link)
		}
	}

	if link == "" && code != "" {
		link = buildInviteLink(site, code)
	}

	return ptrIfNonEmpty(link), ptrIfNonEmpty(code)
}

func invitePaths(site models.Site, configKeys ...string) []string {
	out := make([]string, 0, len(configKeys))
	seen := map[string]struct{}{}
	for _, key := range configKeys {
		for _, item := range splitInvitePathList(stringValue(site.PluginConfig, key, "")) {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

func loadInviteRequestSpec(site models.Site) (inviteRequestSpec, bool, error) {
	target := strings.TrimSpace(stringValue(site.PluginConfig, "invite_path", ""))
	if target == "" {
		target = strings.TrimSpace(stringValue(site.PluginConfig, "invite_api_path", ""))
	}
	if target == "" {
		target = inferredInviteRequestTarget(site)
	}
	if target == "" {
		return inviteRequestSpec{}, false, nil
	}

	method := strings.ToUpper(strings.TrimSpace(stringValue(site.PluginConfig, "invite_method", "")))
	if method == "" {
		method = strings.ToUpper(strings.TrimSpace(stringValue(site.PluginConfig, "invite_api_method", "")))
	}
	if method == "" {
		method = "GET"
	}

	body, _, err := jsonConfigValue(site.PluginConfig, "invite_body_json", "invite_api_body_json")
	if err != nil {
		return inviteRequestSpec{}, true, err
	}

	return inviteRequestSpec{
		Method: method,
		Target: target,
		Body:   body,
	}, true, nil
}

func fetchInviteInfo(
	ctx context.Context,
	site models.Site,
	fetcher func(context.Context, inviteRequestSpec) (map[string]any, error),
) (*string, *string, error) {
	spec, ok, err := loadInviteRequestSpec(site)
	if err != nil || !ok {
		return nil, nil, err
	}
	payload, err := fetcher(ctx, spec)
	if err != nil || payload == nil {
		return nil, nil, err
	}
	link, code := extractInviteInfoWithConfig(
		payload,
		site,
		[]string{"invite_link_path", "status_invite_link_path"},
		[]string{"invite_code_path", "status_invite_code_path"},
	)
	return link, code, nil
}

func mergeInviteInfo(site models.Site, preferredLink, preferredCode, fallbackLink, fallbackCode *string) (*string, *string) {
	link := normalizeInvitePtr(preferredLink)
	code := normalizeInvitePtr(preferredCode)
	if link == nil {
		link = normalizeInvitePtr(fallbackLink)
	}
	if code == nil {
		code = normalizeInvitePtr(fallbackCode)
	}
	if code == nil && link != nil {
		code = ptrIfNonEmpty(extractInviteCodeFromLink(*link))
	}
	if link == nil && code != nil {
		link = ptrIfNonEmpty(buildInviteLink(site, *code))
	}
	return link, code
}

func normalizeInvitePtr(value *string) *string {
	if value == nil {
		return nil
	}
	return ptrIfNonEmpty(*value)
}

func jsonConfigValue(config models.JSONMap, keys ...string) (any, bool, error) {
	for _, key := range keys {
		raw := strings.TrimSpace(stringValue(config, key, ""))
		if raw == "" {
			continue
		}
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, true, fmt.Errorf("%s 必须是有效 JSON", key)
		}
		return value, true, nil
	}
	return nil, false, nil
}

func buildInviteLink(site models.Site, code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}

	template := strings.TrimSpace(stringValue(site.PluginConfig, "invite_link_template", ""))
	if template == "" {
		template = inferredInviteLinkTemplate(site)
	}
	if template == "" {
		return ""
	}

	replaced := strings.ReplaceAll(template, "{code}", url.QueryEscape(code))
	if normalized, err := services.JoinURL(site.BaseURL, replaced); err == nil && normalized != "" {
		return normalized
	}
	return replaced
}

func inferredInviteRequestTarget(site models.Site) string {
	switch site.PluginKey {
	case "sub2api-platform":
		return "/api/v1/user/referral?timezone=Asia%2FShanghai"
	case "yellowpeach-newapi":
		return "/api/user/aff"
	default:
		return ""
	}
}

func inferredInviteLinkTemplate(site models.Site) string {
	if template := inviteTemplateOverrideByHost(site.BaseURL); template != "" {
		return template
	}

	invitePath := strings.ToLower(strings.TrimSpace(stringValue(site.PluginConfig, "invite_path", "")))
	if invitePath == "" {
		invitePath = strings.ToLower(strings.TrimSpace(stringValue(site.PluginConfig, "invite_api_path", "")))
	}
	if invitePath == "" {
		invitePath = strings.ToLower(strings.TrimSpace(inferredInviteRequestTarget(site)))
	}

	switch {
	case strings.Contains(invitePath, "/api/console/referral"):
		return "/s/{code}"
	case strings.Contains(invitePath, "/referral"):
		return "/register?ref={code}"
	case strings.Contains(invitePath, "/aff"):
		return "/register?aff={code}"
	}

	switch site.PluginKey {
	case "sub2api-platform":
		return "/register?aff={code}"
	case "yellowpeach-newapi":
		return "/register?aff={code}"
	case "http-relay-station":
		return "/register?code={code}"
	default:
		return ""
	}
}

func inviteTemplateOverrideByHost(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	host = strings.TrimSuffix(host, ".")
	host = strings.TrimPrefix(host, "www.")
	if host == "" {
		return ""
	}
	switch host {
	case "yaoshanapi.com":
		return "/register?ref={code}"
	case "alexai.work":
		return "/register?aff={code}"
	}
	if parsed.Host != "" {
		if normalizedHost, _, err := net.SplitHostPort(parsed.Host); err == nil {
			switch strings.TrimPrefix(strings.ToLower(normalizedHost), "www.") {
			case "yaoshanapi.com":
				return "/register?ref={code}"
			case "alexai.work":
				return "/register?aff={code}"
			}
		}
	}
	return ""
}

func splitInvitePathList(raw string) []string {
	return strings.FieldsFunc(strings.TrimSpace(raw), func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
}

func firstNonEmptyPathString(payload map[string]any, paths []string) string {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		value := strings.TrimSpace(pathString(payload, path, ""))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func extractInviteCodeFromLink(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	query := parsed.Query()
	for _, key := range []string{"aff", "aff_code", "invite", "invite_code", "code", "ref", "referral"} {
		if value := strings.TrimSpace(query.Get(key)); value != "" {
			return value
		}
	}
	return ""
}
