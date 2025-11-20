package main

import (
	"testing"

	"github.com/guilhem/token-renewer/shared"
)

// TestPluginServerInterface ensures plugin implements the gRPC server interface
func TestPluginServerInterface(t *testing.T) {
	var _ shared.TokenProviderServiceServer = (*LinodePlugin)(nil)
	t.Log("✓ LinodePlugin implements shared.TokenProviderServiceServer interface")
}

// TestPluginMetadataConversion tests metadata ID conversion
func TestPluginMetadataConversion(t *testing.T) {
	plugin := &LinodePlugin{}

	tests := []struct {
		name     string
		metadata string
		wantErr  bool
	}{
		{"valid_id", "12345", false},
		{"invalid_string", "invalid", true},
		{"empty_string", "", true},
		{"negative_number", "-1", false}, // atoi accepts negative
		{"large_number", "999999999", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := plugin.metadataToID(tt.metadata)
			if (err != nil) != tt.wantErr {
				t.Errorf("metadataToID(%q) error = %v, wantErr %v", tt.metadata, err, tt.wantErr)
			}
		})
	}
}
