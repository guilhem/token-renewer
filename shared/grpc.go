package shared

import (
	context "context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// GRPCClient is an implementation of KV that talks over RPC.
type GRPCClient struct{ client TokenProviderServiceClient }

func NewGRPCClient(client TokenProviderServiceClient) *GRPCClient {
	return &GRPCClient{client: client}
}

func (m *GRPCClient) RenewToken(ctx context.Context, metadata, token string) (string, string, *time.Time, error) {
	req := &RenewTokenRequest{Metadata: metadata, Token: token}
	resp, err := m.client.RenewToken(ctx, req)
	if err != nil {
		return "", "", nil, err
	}
	t := resp.Expiration.AsTime()
	return resp.Token, resp.NewMetadata, &t, nil
}

func (m *GRPCClient) GetTokenValidity(ctx context.Context, metadata, token string) (*time.Time, error) {
	req := &GetTokenValidityRequest{Token: token, Metadata: metadata}
	resp, err := m.client.GetTokenValidity(ctx, req)
	if err != nil {
		return nil, err
	}
	t := resp.Expiration.AsTime()
	return &t, nil
}

// Here is the gRPC server that GRPCClient talks to.
type GRPCServer struct {
	UnimplementedTokenProviderServiceServer
	// This is the real implementation
	Impl TokenProvider
}

func (m *GRPCServer) RenewToken(ctx context.Context, req *RenewTokenRequest) (*RenewTokenResponse, error) {
	token, newMetadata, expiration, err := m.Impl.RenewToken(ctx, req.Metadata, req.Token)
	if err != nil {
		return nil, err
	}
	return &RenewTokenResponse{
		Token:       token,
		NewMetadata: newMetadata,
		Expiration:  timestamppb.New(*expiration),
	}, nil
}

func (m *GRPCServer) GetTokenValidity(ctx context.Context, req *GetTokenValidityRequest) (*GetTokenValidityResponse, error) {
	expiration, err := m.Impl.GetTokenValidity(ctx, req.Metadata, req.Token)
	if err != nil {
		return nil, err
	}
	return &GetTokenValidityResponse{
		Expiration: timestamppb.New(*expiration),
	}, nil
}
