package shared

import (
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "TOKEN_RENEWER_PLUGIN",
	MagicCookieValue: "token-renewer",
}

// TokenPlugin is the implementation of the plugin interface.
type TokenPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	plugin.GRPCPlugin
	Impl TokenProvider
}

// GRPCServer returns the plugin's gRPC server.
func (p *TokenPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	RegisterTokenProviderServiceServer(s, &GRPCServer{Impl: p.Impl})
	return nil
}

// GRPCClient returns the plugin's gRPC client.
func (p *TokenPlugin) GRPCClient(broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &TokenPlugin{Impl: p.Impl}, nil
}

// TokenProviderPluginName is the name of the plugin.
const TokenProviderPluginName = "token-provider"
