package plugins

import (
	"testing"

	"ai-sign-in-gateway/internal/models"
)

func TestBuildInviteLinkUsesHostOverridesForSub2APIStyleSites(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{
			name:    "yaoshan uses ref",
			baseURL: "https://www.yaoshanapi.com",
			want:    "https://www.yaoshanapi.com/register?ref=B463F8C1D6",
		},
		{
			name:    "alexai uses aff",
			baseURL: "https://alexai.work",
			want:    "https://alexai.work/register?aff=97f16eca",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildInviteLink(models.Site{
				BaseURL:   tc.baseURL,
				PluginKey: "sub2api-platform",
			}, map[string]string{
				"yaoshan uses ref": "B463F8C1D6",
				"alexai uses aff":  "97f16eca",
			}[tc.name])
			if got != tc.want {
				t.Fatalf("buildInviteLink() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildInviteLinkInfersSU8StyleShortPath(t *testing.T) {
	got := buildInviteLink(models.Site{
		BaseURL:   "https://www.su8.codes",
		PluginKey: "http-relay-station",
		PluginConfig: models.JSONMap{
			"invite_path": "/api/console/referral",
		},
	}, "c18760237581")
	want := "https://www.su8.codes/s/c18760237581"
	if got != want {
		t.Fatalf("buildInviteLink() = %q, want %q", got, want)
	}
}
