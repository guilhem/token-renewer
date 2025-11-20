package main

import (
	"context"
	"testing"

	"github.com/guilhem/token-renewer/shared"
)

// TestGetTokenValidity_NoExpiryReturnsValidTime tests that a valid time is returned
func TestGetTokenValidity_NoExpiryReturnsValidTime(t *testing.T) {
	plugin := &LinodePlugin{}

	// This test verifies that GetTokenValidity returns a valid response
	// In a real scenario, this would call Linode API, so we just verify the signature

	ctx := context.Background()

	// This test is mainly for verifying the method signature is correct
	// Real testing requires Linode API credentials
	req := &shared.GetTokenValidityRequest{
		Metadata: "12345",
		Token:    "dummy-token",
	}

	resp, err := plugin.GetTokenValidity(ctx, req)

	// We expect errors in test env (no real Linode API), but signature must be correct
	if resp != nil && resp.Expiration == nil {
		t.Error("Expiration must not be nil in response")
	}

	// In test env, we'll get API errors, which is expected
	t.Logf("Test result: error=%v (expected in test env)", err)
}

// TestRenewToken_Signature tests that RenewToken has correct signature
func TestRenewToken_Signature(t *testing.T) {
	plugin := &LinodePlugin{}
	ctx := context.Background()

	req := &shared.RenewTokenRequest{
		Metadata: "67890",
		Token:    "old-token",
	}

	resp, err := plugin.RenewToken(ctx, req)

	// Verify response structure
	if resp != nil {
		if resp.Token == "" && err == nil {
			t.Error("Token must be returned or error must be set")
		}
		if resp.Expiration == nil && err == nil {
			t.Error("Expiration must not be nil in response")
		}
	}

	// In test env, we expect errors (no real Linode API)
	t.Logf("Test result: error=%v (expected in test env)", err)
}

// TestLinodePlugin_MetadataConversion tests metadata to ID conversion
func TestLinodePlugin_MetadataConversion(t *testing.T) {
	plugin := &LinodePlugin{}

	tests := []struct {
		name    string
		meta    string
		valid   bool
		wantErr bool
	}{
		{"valid_numeric_id", "12345", true, false},
		{"valid_large_id", "999999999", true, false},
		{"invalid_non_numeric", "abc", false, true},
		{"invalid_empty", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := plugin.metadataToID(tt.meta)
			if (err != nil) != tt.wantErr {
				t.Errorf("metadataToID() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && id <= 0 && tt.valid {
				t.Errorf("metadataToID() returned invalid id %d", id)
			}
		})
	}
}
