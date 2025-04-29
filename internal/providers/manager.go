package providers

import (
	"errors"

	"github.com/guilhem/token-renewer/shared"
	"github.com/hashicorp/go-plugin"
)

type ProvidersManager struct {
	clients map[string]*plugin.Client
}

func NewProvidersManager() *ProvidersManager {
	return &ProvidersManager{
		clients: make(map[string]*plugin.Client),
	}
}

func (pm *ProvidersManager) RegisterPlugin(name string, pluginConfig *plugin.ClientConfig) {
	client := plugin.NewClient(pluginConfig)
	pm.clients[name] = client
}

func (pm *ProvidersManager) GetProvider(name string) (shared.TokenProvider, error) {
	client, exists := pm.clients[name]
	if !exists {
		return nil, errors.New("plugin not found")
	}

	// Connect to the plugin
	cl, err := client.Client()
	if err != nil {
		return nil, err
	}

	raw, err := cl.Dispense(shared.TokenProviderPluginName)
	if err != nil {
		return nil, err
	}

	provider, ok := raw.(shared.TokenProvider)
	if !ok {
		return nil, errors.New("invalid plugin type")
	}

	return provider, nil
}

func (pm *ProvidersManager) Start() error {
	for _, client := range pm.clients {
		if _, err := client.Start(); err != nil {
			return err
		}
	}

	return nil
}
