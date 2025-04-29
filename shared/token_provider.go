package shared

import "time"

// TokenProvider defines the interface for token management.
type TokenProvider interface {
	// RenewToken renews a token and returns the new token, metadata, and expiration time.
	RenewToken(metadata string) (token string, newMetadata string, expiration time.Time, err error)

	// GetTokenValidity checks the validity of a token and returns its expiration time.
	GetTokenValidity(token string) (expiration time.Time, err error)
}
