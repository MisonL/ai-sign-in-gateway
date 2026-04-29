package plugins

import (
	"fmt"
	"sort"

	"ai-sign-in-gateway/internal/schemas"
)

type Manager struct {
	plugins map[string]SitePlugin
}

func NewManager() *Manager {
	items := []SitePlugin{
		NewHTTPStation(),
		NewAPISupplier(),
		NewSub2API(),
		NewYellowPeach(),
	}
	m := &Manager{plugins: map[string]SitePlugin{}}
	for _, item := range items {
		m.plugins[item.Meta().Key] = item
	}
	return m
}

func (m *Manager) Get(key string) (SitePlugin, error) {
	plugin, ok := m.plugins[key]
	if !ok {
		return nil, fmt.Errorf("Plugin '%s' not found", key)
	}
	return plugin, nil
}

func (m *Manager) ListMeta() []schemas.PluginMetaResponse {
	out := make([]schemas.PluginMetaResponse, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		out = append(out, plugin.Meta())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
