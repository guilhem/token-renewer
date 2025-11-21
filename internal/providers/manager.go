package providers

import (
	"errors"

	"github.com/guilhem/operator-plugin-framework/registry"
	"github.com/guilhem/token-renewer/shared"
)

// ProvidersManager is a thin wrapper around the framework's registry.Manager
// specialized for TokenProvider plugins. It maintains backward compatibility
// while using the shared registry infrastructure from operator-plugin-framework.
type ProvidersManager struct {
	manager *registry.Manager
}

// NewProvidersManager creates a new providers manager using the shared framework.
func NewProvidersManager() *ProvidersManager {
	return &ProvidersManager{
		manager: registry.New(),
	}
}

// RegisterPlugin registers a token provider plugin.
// The provider is wrapped to implement the framework's PluginProvider interface.
func (pm *ProvidersManager) RegisterPlugin(name string, provider shared.TokenProvider) {
	// Wrap TokenProvider to implement framework's PluginProvider interface
	wrapper := &tokenProviderWrapper{
		name:     name,
		provider: provider,
	}
	pm.manager.Register(name, wrapper)
}

// UnregisterPlugin removes a token provider plugin.
func (pm *ProvidersManager) UnregisterPlugin(name string) {
	pm.manager.Unregister(name)
}

// GetProvider returns a token provider by name.
func (pm *ProvidersManager) GetProvider(name string) (shared.TokenProvider, error) {
	plugin, err := pm.manager.Get(name)
	if err != nil {
		return nil, err
	}

	// Unwrap from framework's PluginProvider interface
	wrapper, ok := plugin.(*tokenProviderWrapper)
	if !ok {
		return nil, errors.New("invalid plugin type")
	}

	return wrapper.provider, nil
}

// GetPlugins returns a copy of all registered token providers.
func (pm *ProvidersManager) GetPlugins() map[string]shared.TokenProvider {
	plugins := pm.manager.GetAll()
	result := make(map[string]shared.TokenProvider)

	for name, plugin := range plugins {
		wrapper, ok := plugin.(*tokenProviderWrapper)
		if ok {
			result[name] = wrapper.provider
		}
	}

	return result
}

// tokenProviderWrapper adapts TokenProvider to the framework's PluginProvider interface.
type tokenProviderWrapper struct {
	name     string
	provider shared.TokenProvider
}

// Name returns the plugin name (required by PluginProvider interface).
func (w *tokenProviderWrapper) Name() string {
	return w.name
}
