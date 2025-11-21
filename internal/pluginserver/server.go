/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pluginserver

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/guilhem/token-renewer/internal/providers"
	"github.com/guilhem/token-renewer/shared"
)

// Server manages plugin connections and implements the TokenProviderService.
// Plugins connect via standard gRPC RPC calls.
type Server struct {
	shared.UnimplementedTokenProviderServiceServer

	addr             string
	providersManager *providers.ProvidersManager
	plugins          map[string]shared.TokenProviderServiceClient
	pluginsMu        sync.RWMutex
	grpcServer       *grpc.Server
	lis              net.Listener
}

// NewServer creates a new plugin server.
// Authentication is delegated to kube-rbac-proxy sidecar.
func NewServer(
	addr string,
	providersManager *providers.ProvidersManager,
) *Server {
	return &Server{
		addr:             addr,
		providersManager: providersManager,
		plugins:          make(map[string]shared.TokenProviderServiceClient),
	}
}

// Start begins listening for plugin connections.
func (s *Server) Start(ctx context.Context) error {
	logger := log.FromContext(ctx)

	network, addr, err := parseAddr(s.addr)
	if err != nil {
		return fmt.Errorf("invalid server address: %w", err)
	}

	lis, err := net.Listen(network, addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s %s: %w", network, addr, err)
	}
	s.lis = lis

	s.grpcServer = grpc.NewServer()
	shared.RegisterTokenProviderServiceServer(s.grpcServer, s)

	logger.Info("Starting plugin server", "network", network, "addr", addr)

	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			logger.Error(err, "gRPC server error")
		}
	}()

	<-ctx.Done()
	s.Stop()
	return nil
}

// Stop gracefully stops the plugin server.
func (s *Server) Stop() {
	logger := log.Log
	logger.Info("Stopping plugin server")

	s.pluginsMu.Lock()
	defer s.pluginsMu.Unlock()

	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	for name := range s.plugins {
		s.providersManager.UnregisterPlugin(name)
	}
	s.plugins = make(map[string]shared.TokenProviderServiceClient)
}

// RenewToken renews a token and returns the new token and expiration time.
func (s *Server) RenewToken(ctx context.Context, in *shared.RenewTokenRequest) (*shared.RenewTokenResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// GetTokenValidity returns the expiration time of a token.
func (s *Server) GetTokenValidity(ctx context.Context, in *shared.GetTokenValidityRequest) (*shared.GetTokenValidityResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// parseAddr parses an address string into network and address components.
func parseAddr(addr string) (string, string, error) {
	if len(addr) < 8 {
		return "", "", fmt.Errorf("invalid address format: %s", addr)
	}

	scheme := addr[:5]
	switch scheme {
	case "unix:":
		return "unix", addr[7:], nil
	case "tcp:/":
		return "tcp", addr[6:], nil
	default:
		return "", "", fmt.Errorf("unsupported address scheme: %s", scheme)
	}
}

// PluginClient implements shared.TokenProvider using direct gRPC calls to a plugin.
type PluginClient struct {
	client shared.TokenProviderServiceClient
}

// RenewToken renews a token via the plugin client.
func (pc *PluginClient) RenewToken(ctx context.Context, metadata, token string) (string, string, *time.Time, error) {
	resp, err := pc.client.RenewToken(ctx, &shared.RenewTokenRequest{
		Metadata: metadata,
		Token:    token,
	})
	if err != nil {
		return "", "", nil, err
	}
	expTime := resp.GetExpiration().AsTime()
	return resp.GetToken(), resp.GetNewMetadata(), &expTime, nil
}

// GetTokenValidity returns the expiration time of a token via the plugin client.
func (pc *PluginClient) GetTokenValidity(ctx context.Context, metadata, token string) (*time.Time, error) {
	resp, err := pc.client.GetTokenValidity(ctx, &shared.GetTokenValidityRequest{
		Metadata: metadata,
		Token:    token,
	})
	if err != nil {
		return nil, err
	}
	expTime := resp.GetExpiration().AsTime()
	return &expTime, nil
}

var _ shared.TokenProvider = (*PluginClient)(nil)
