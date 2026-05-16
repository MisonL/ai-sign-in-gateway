package features

import (
	"sort"
	"sync"

	"ai-sign-in-gateway/internal/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type Runtime struct {
	DB            *gorm.DB
	PluginManager any
	Settings      func() (models.SystemSetting, error)
}

type Module struct {
	Key            string
	Name           string
	Description    string
	RoutePath      string
	FrontendPath   string
	DefaultEnabled bool
	RegisterRoutes func(Runtime, chi.Router)
	Migrate        func(*gorm.DB) error
}

var (
	mu      sync.RWMutex
	modules = map[string]Module{}
)

func Register(module Module) {
	if module.Key == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	modules[module.Key] = module
}

func List() []Module {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Module, 0, len(modules))
	for _, module := range modules {
		out = append(out, module)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func AutoMigrate(db *gorm.DB) error {
	for _, module := range List() {
		if module.Migrate == nil {
			continue
		}
		if err := module.Migrate(db); err != nil {
			return err
		}
	}
	return nil
}
