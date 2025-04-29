package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/linode/linodego"
)

type LinodePlugin struct {
}

func (p *LinodePlugin) RenewToken(ctx context.Context, meta, token string) (string, string, *time.Time, error) {
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

func (p *LinodePlugin) GetTokenValidity(ctx context.Context, meta, token string) (*time.Time, error) {
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

	return nil, nil
}

func (p *LinodePlugin) metadataToID(meta string) (int, error) {
	return strconv.Atoi(meta)
}
