package plugins

import (
	"context"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/schemas"
)

type AccountStatus struct {
	LoggedIn            bool
	Message             string
	Balance             *float64
	BalanceUnit         *string
	PackageDisplay      *string
	AccountName         *string
	InviteLink          *string
	InviteCode          *string
	UpdatedCredentials  models.JSONMap
	UpdatedPluginConfig models.JSONMap
}

type CheckinResult struct {
	Success            bool
	Message            string
	Balance            *float64
	BalanceUnit        *string
	ResponseExcerpt    *string
	UpdatedCredentials models.JSONMap
}

type SitePlugin interface {
	Meta() schemas.PluginMetaResponse
	Validate(site models.Site) error
	FetchAccountStatus(ctx context.Context, site models.Site, timeoutSeconds int) (AccountStatus, error)
	Checkin(ctx context.Context, site models.Site, timeoutSeconds int) (CheckinResult, error)
}

func Field(name, label, fieldType, placeholder string, required bool, helpText string) schemas.FieldDescriptor {
	return schemas.FieldDescriptor{Name: name, Label: label, Type: fieldType, Placeholder: placeholder, Required: required, HelpText: helpText}
}
