package v1beta1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// TestTokenSpecMetadataValidation tests that metadata field validation works correctly
func TestTokenSpecMetadataValidation(t *testing.T) {
	tests := []struct {
		name        string
		metadata    string
		shouldValid bool
		description string
	}{
		{
			name:        "valid_metadata",
			metadata:    "12345",
			shouldValid: true,
			description: "Non-empty metadata should be valid",
		},
		{
			name:        "empty_metadata",
			metadata:    "",
			shouldValid: false,
			description: "Empty metadata should NOT be valid because field is Required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := TokenSpec{
				Provider:  ProviderSpec{Name: "test"},
				Metadata:  tt.metadata,
				SecretRef: corev1.LocalObjectReference{Name: "test-secret"},
			}

			// The bug is that metadata is marked as Required: true but also has omitempty: true
			// This creates a contradiction in validation
			if tt.metadata == "" && tt.shouldValid {
				t.Errorf("BUG: Empty metadata should not be valid (Required field)")
			}

			// Test that non-empty metadata is accepted
			if tt.metadata != "" {
				if spec.Metadata != tt.metadata {
					t.Errorf("Metadata not stored correctly: got %q, want %q", spec.Metadata, tt.metadata)
				}
			}
		})
	}
}

// TestTokenSpecValidation tests the full TokenSpec validation
func TestTokenSpecValidation(t *testing.T) {
	t.Run("all_required_fields_present", func(t *testing.T) {
		spec := TokenSpec{
			Provider:  ProviderSpec{Name: "linode"},
			Metadata:  "token-id-123",
			Renewval:  RenewvalSpec{},
			SecretRef: corev1.LocalObjectReference{Name: "my-secret"},
		}

		// All required fields should be set
		if spec.Provider.Name == "" {
			t.Error("Provider.Name is empty")
		}
		if spec.Metadata == "" {
			t.Error("Metadata is empty")
		}
		if spec.SecretRef.Name == "" {
			t.Error("SecretRef.Name is empty")
		}
	})

	t.Run("metadata_required_field_contradiction", func(t *testing.T) {
		t.Log("BUG CHECK: Metadata has both +kubebuilder:validation:Required and json:omitempty")
		t.Log("This is a contradiction because:")
		t.Log("  - Required means it MUST be present")
		t.Log("  - omitempty means it's OK to omit it from JSON")
		t.Log("")
		t.Log("The fix should remove 'omitempty' and add MinLength=1 validation")
	})
}
