package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/linode/linodego"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/guilhem/token-renewer/shared"
)

// LinodePlugin implements the TokenProvider interface for Linode API tokens.
// It uses the Linode API to create, retrieve, and delete tokens.
type LinodePlugin struct {
	shared.UnimplementedTokenProviderServiceServer
}

// Ensure LinodePlugin implements shared.TokenProviderServiceServer interface
var _ shared.TokenProviderServiceServer = (*LinodePlugin)(nil)

// RenewToken implements TokenProviderServiceServer.RenewToken.
func (p *LinodePlugin) RenewToken(ctx context.Context, req *shared.RenewTokenRequest) (*shared.RenewTokenResponse, error) {
	token, newMetadata, expiration, err := p.renewToken(ctx, req.GetMetadata(), req.GetToken())
	if err != nil {
		return nil, err
	}

	return &shared.RenewTokenResponse{
		Token:       token,
		NewMetadata: newMetadata,
		Expiration:  timestamppb.New(*expiration),
	}, nil
}

// GetTokenValidity implements TokenProviderServiceServer.GetTokenValidity.
func (p *LinodePlugin) GetTokenValidity(ctx context.Context, req *shared.GetTokenValidityRequest) (*shared.GetTokenValidityResponse, error) {
	expiration, err := p.getTokenValidity(ctx, req.GetMetadata(), req.GetToken())
	if err != nil {
		return nil, err
	}

	return &shared.GetTokenValidityResponse{
		Expiration: timestamppb.New(*expiration),
	}, nil
}

// renewToken is the internal implementation for token renewal.
func (p *LinodePlugin) renewToken(ctx context.Context, meta, token string) (string, string, *time.Time, error) {
	id, err := p.metadataToID(meta)
	if err != nil {
		return "", "", nil, fmt.Errorf("invalid metadata: %w", err)
	}

	cl := linodego.NewClient(nil)
	cl.SetToken(token)

	oldToken, err := cl.GetToken(ctx, id)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to get token: %w", err)
	}

	expireTime := time.Now().Add(24 * time.Hour)

	newToken, err := cl.CreateToken(ctx, linodego.TokenCreateOptions{
		Label:  oldToken.Label,
		Scopes: oldToken.Scopes,
		Expiry: &expireTime,
	})
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to create token: %w", err)
	}

	if err := cl.DeleteToken(ctx, id); err != nil {
		return "", "", nil, fmt.Errorf("failed to delete old token: %w", err)
	}

	return newToken.Token, strconv.Itoa(newToken.ID), &expireTime, nil
}

// getTokenValidity is the internal implementation for validity check.
func (p *LinodePlugin) getTokenValidity(ctx context.Context, meta, token string) (*time.Time, error) {
	id, err := p.metadataToID(meta)
	if err != nil {
		return nil, fmt.Errorf("invalid metadata: %w", err)
	}

	cl := linodego.NewClient(nil)
	cl.SetToken(token)

	oldToken, err := cl.GetToken(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	if oldToken.Expiry != nil {
		return oldToken.Expiry, nil
	}

	// If token has no expiry date, treat it as non-expiring (return a far future date)
	// This handles tokens that never expire by setting expiration to 10 years from now
	futureTime := time.Now().AddDate(10, 0, 0)
	return &futureTime, nil
}

func (p *LinodePlugin) metadataToID(meta string) (int, error) {
	return strconv.Atoi(meta)
}
