package providers

import (
	"errors"

	"github.com/guilhem/token-renewer/shared"
)

type ProvidersManager struct {
	clients map[string]*shared.GRPCClient
}

func NewProvidersManager() *ProvidersManager {
	return &ProvidersManager{
		clients: make(map[string]*shared.GRPCClient),
	}
}

func (pm *ProvidersManager) RegisterPlugin(name string, cl *shared.GRPCClient) {
	pm.clients[name] = cl
}

func (pm *ProvidersManager) GetProvider(name string) (shared.TokenProvider, error) {
	client, exists := pm.clients[name]
	if !exists {
		return nil, errors.New("plugin not found")
	}

	return client, nil
}
