package plugins

import (
	"context"
	"errors"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/schemas"
)

type Placeholder struct {
	meta schemas.PluginMetaResponse
}

func NewPlaceholder(key, name, description string, capabilities []string) *Placeholder {
	return &Placeholder{meta: schemas.PluginMetaResponse{
		Key:              key,
		Name:             name,
		Description:      description,
		CredentialFields: []schemas.FieldDescriptor{},
		ConfigFields:     []schemas.FieldDescriptor{},
		Capabilities:     capabilities,
	}}
}

func (p *Placeholder) Meta() schemas.PluginMetaResponse { return p.meta }
func (p *Placeholder) Validate(site models.Site) error  { return nil }
func (p *Placeholder) FetchAccountStatus(ctx context.Context, site models.Site, timeoutSeconds int) (AccountStatus, error) {
	return AccountStatus{}, errors.New("该插件的 Go 版本尚未接入真实站点检测")
}
func (p *Placeholder) Checkin(ctx context.Context, site models.Site, timeoutSeconds int) (CheckinResult, error) {
	return CheckinResult{}, errors.New("该插件的 Go 版本尚未接入签到")
}
