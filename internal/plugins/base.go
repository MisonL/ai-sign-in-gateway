package plugins

import (
	"context"

	"ai-sign-in-gateway/internal/models"
	"ai-sign-in-gateway/internal/schemas"
	"ai-sign-in-gateway/internal/services"
)

type AccountStatus struct {
	LoggedIn            bool
	Message             string
	Balance             *float64
	BalanceUnit         *string
	PackageRemaining    *float64
	PackageTotal        *float64
	PackageUsed         *float64
	PackageUnit         *string
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

type APIKeySyncResult struct {
	UpdatedCredentials models.JSONMap
	PrimaryKey         string
	Message            string
}

type AccountRegistrationRequest struct {
	Email       string
	Password    string
	AccountName string
}

type AccountRegistrationResult struct {
	Message      string
	Credentials  models.JSONMap
	PluginConfig models.JSONMap
	PrimaryKey   string
	APIKeyCount  int
	AccountName  string
}

type SitePlugin interface {
	Meta() schemas.PluginMetaResponse
	Validate(site models.Site) error
	FetchAccountStatus(ctx context.Context, site models.Site, timeoutSeconds int) (AccountStatus, error)
	Checkin(ctx context.Context, site models.Site, timeoutSeconds int) (CheckinResult, error)
}

type APIKeySyncer interface {
	SyncAPIKeys(ctx context.Context, site models.Site, timeoutSeconds int) (APIKeySyncResult, error)
}

type AccountRegistrar interface {
	RegisterAccount(ctx context.Context, site models.Site, request AccountRegistrationRequest, timeoutSeconds int) (AccountRegistrationResult, error)
}

func apiKeyUpdateCount(credentials models.JSONMap) int {
	if credentials == nil {
		return 0
	}
	switch raw := credentials["api_keys"].(type) {
	case []map[string]any:
		return len(raw)
	case []any:
		count := 0
		for _, item := range raw {
			if item != nil {
				count++
			}
		}
		if count > 0 {
			return count
		}
	}
	if value, ok := credentials["api_key"].(string); ok && value != "" {
		return 1
	}
	return 0
}

func Field(name, label, fieldType, placeholder string, required bool, helpText string) schemas.FieldDescriptor {
	return schemas.FieldDescriptor{Name: name, Label: label, Type: fieldType, Placeholder: placeholder, Required: required, HelpText: helpText}
}

func clonePluginJSON(value models.JSONMap) models.JSONMap {
	out := models.JSONMap{}
	for key, item := range value {
		out[key] = item
	}
	return out
}

func normalizeBalanceUnit(unit string) string {
	return services.NormalizeBalanceUnit(unit)
}

func balanceUnitIsSymbol(unit string) bool {
	return services.BalanceUnitIsSymbol(unit)
}
