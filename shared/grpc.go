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

func (m *GRPCClient) RenewToken(ctx context.Context, metadata string) (string, string, time.Time, error) {
	req := &RenewTokenRequest{Metadata: metadata}
	resp, err := m.client.RenewToken(ctx, req)
	if err != nil {
		return "", "", time.Time{}, err
	}
	return resp.Token, resp.NewMetadata, resp.Expiration.AsTime(), nil
}

func (m *GRPCClient) GetTokenValidity(ctx context.Context, token string) (time.Time, error) {
	req := &GetTokenValidityRequest{Token: token}
	resp, err := m.client.GetTokenValidity(ctx, req)
	if err != nil {
		return time.Time{}, err
	}
	return resp.Expiration.AsTime(), nil
}

// Here is the gRPC server that GRPCClient talks to.
type GRPCServer struct {
	UnimplementedTokenProviderServiceServer
	// This is the real implementation
	Impl TokenProvider
}

func (m *GRPCServer) RenewToken(ctx context.Context, req *RenewTokenRequest) (*RenewTokenResponse, error) {
	token, newMetadata, expiration, err := m.Impl.RenewToken(req.Metadata)
	if err != nil {
		return nil, err
	}
	return &RenewTokenResponse{
		Token:       token,
		NewMetadata: newMetadata,
		Expiration:  timestamppb.New(expiration),
	}, nil
}

func (m *GRPCServer) GetTokenValidity(ctx context.Context, req *GetTokenValidityRequest) (*GetTokenValidityResponse, error) {
	expiration, err := m.Impl.GetTokenValidity(req.Token)
	if err != nil {
		return nil, err
	}
	return &GetTokenValidityResponse{
		Expiration: timestamppb.New(expiration),
	}, nil
}
