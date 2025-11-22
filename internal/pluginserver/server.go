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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pluginframeworkv1 "github.com/guilhem/operator-plugin-framework/pluginframework/v1"
	"github.com/guilhem/operator-plugin-framework/stream"
	"github.com/guilhem/token-renewer/internal/providers"
	shared "github.com/guilhem/token-renewer/shared"
)

// StreamServer runs inside the controller and only exposes the PluginStream RPC
// so plugin providers can connect. The actual token provider RPCs are implemented
// by the plugins themselves.
type StreamServer struct {
	addr       string
	handler    *StreamHandler
	grpcServer *grpc.Server
	lis        net.Listener
}

// NewServer creates a new controller-side stream server that accepts plugin connections.
func NewServer(
	addr string,
	providersManager *providers.ProvidersManager,
) *StreamServer {
	return &StreamServer{
		addr:    addr,
		handler: NewStreamHandler(providersManager),
	}
}

// Start begins listening for plugin connections.
func (s *StreamServer) Start(ctx context.Context) error {
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
	shared.RegisterTokenProviderServiceServer(s.grpcServer, s.handler)

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
func (s *StreamServer) Stop() {
	logger := log.Log.WithName("pluginserver")
	logger.Info("Stopping plugin server")

	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
		s.grpcServer = nil
	}

	if s.lis != nil {
		_ = s.lis.Close()
		s.lis = nil
	}

	if s.handler != nil {
		s.handler.DropAll()
	}
}

// StreamHandler implements only the PluginStream RPC for controller-side streaming.
// The unary RPCs are intentionally unimplemented to make the separation explicit.
type StreamHandler struct {
	shared.UnimplementedTokenProviderServiceServer

	providersManager *providers.ProvidersManager

	mu            sync.Mutex
	activePlugins map[string]struct{}
}

func NewStreamHandler(providersManager *providers.ProvidersManager) *StreamHandler {
	return &StreamHandler{
		providersManager: providersManager,
		activePlugins:    make(map[string]struct{}),
	}
}

// RenewToken renews a token and returns the new token and expiration time.
// This is implemented by plugins, not by the controller-side stream server.
func (s *StreamHandler) RenewToken(ctx context.Context, in *shared.RenewTokenRequest) (*shared.RenewTokenResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "controller stream server only exposes PluginStream; plugins implement RenewToken")
}

// GetTokenValidity returns the expiration time of a token.
// This is implemented by plugins, not by the controller-side stream server.
func (s *StreamHandler) GetTokenValidity(ctx context.Context, in *shared.GetTokenValidityRequest) (*shared.GetTokenValidityResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "controller stream server only exposes PluginStream; plugins implement GetTokenValidity")
}

// PluginStream handles a bidirectional stream with a plugin.
// It uses the framework's StreamManager directly with the gRPC stream.
func (s *StreamHandler) PluginStream(grpcStream grpc.BidiStreamingServer[pluginframeworkv1.PluginStreamMessage, pluginframeworkv1.PluginStreamMessage]) error {
	logger := log.Log.WithName("pluginserver").WithValues("component", "stream")

	// Create stream manager from the operator-plugin-framework
	streamMgr, err := stream.NewStreamManager(grpcStream)
	if err != nil {
		logger.Error(err, "failed to create stream manager")
		return err
	}

	pluginName := streamMgr.GetPluginName()
	logger = logger.WithValues("plugin", pluginName, "version", streamMgr.GetPluginVersion())
	logger.Info("Plugin connected via stream")

	// Create wrapper that implements TokenProvider using the stream manager
	wrapper := &StreamPluginClient{
		streamMgr:  streamMgr,
		pluginName: pluginName,
	}

	// Register the plugin
	s.registerPlugin(pluginName, wrapper)
	logger.Info("Plugin registered in provider manager")

	// Keep the stream alive and listen for messages
	defer func() {
		s.unregisterPlugin(pluginName)
		logger.Info("Plugin unregistered")
	}()

	// Let the stream manager handle incoming messages
	ctx := grpcStream.Context()
	return streamMgr.ListenForMessages(ctx)
}

func (s *StreamHandler) registerPlugin(name string, provider shared.TokenProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.activePlugins[name] = struct{}{}
	s.providersManager.RegisterPlugin(name, provider)
}

func (s *StreamHandler) unregisterPlugin(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.activePlugins, name)
	s.providersManager.UnregisterPlugin(name)
}

// DropAll forcefully unregisters every plugin currently tracked by the handler.
func (s *StreamHandler) DropAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for name := range s.activePlugins {
		s.providersManager.UnregisterPlugin(name)
		delete(s.activePlugins, name)
	}
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

// StreamPluginClient implements shared.TokenProvider by using the framework's StreamManager.
// It adapts between the gRPC stream and the framework's stream manager.
type StreamPluginClient struct {
	streamMgr  *stream.StreamManager
	pluginName string
}

// RenewToken sends a RenewToken RPC call to the plugin via the stream manager.
func (pc *StreamPluginClient) RenewToken(ctx context.Context, metadata, token string) (string, string, *time.Time, error) {
	req := &shared.RenewTokenRequest{
		Metadata: metadata,
		Token:    token,
	}

	// Use stream manager to call RPC
	respBytes, err := pc.streamMgr.CallRPC(ctx, "RenewToken", req)
	if err != nil {
		return "", "", nil, fmt.Errorf("RPC failed: %w", err)
	}

	// Unmarshal response
	resp := &shared.RenewTokenResponse{}
	if err := proto.Unmarshal(respBytes, resp); err != nil {
		return "", "", nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	expTime := resp.GetExpiration().AsTime()
	return resp.GetToken(), resp.GetNewMetadata(), &expTime, nil
}

// GetTokenValidity sends a GetTokenValidity RPC call to the plugin via the stream manager.
func (pc *StreamPluginClient) GetTokenValidity(ctx context.Context, metadata, token string) (*time.Time, error) {
	req := &shared.GetTokenValidityRequest{
		Metadata: metadata,
		Token:    token,
	}

	// Use stream manager to call RPC
	respBytes, err := pc.streamMgr.CallRPC(ctx, "GetTokenValidity", req)
	if err != nil {
		return nil, fmt.Errorf("RPC failed: %w", err)
	}

	// Unmarshal response
	resp := &shared.GetTokenValidityResponse{}
	if err := proto.Unmarshal(respBytes, resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	expTime := resp.GetExpiration().AsTime()
	return &expTime, nil
}

var _ shared.TokenProvider = (*StreamPluginClient)(nil)
