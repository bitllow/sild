package domain

import (
	"context"
	"time"
)

// MintToken issues a user JWT for a host user id (§4.1). Guests are the same
// call with a host-generated id (§4.5) — no special handling here. TTL is
// clamped to the configured bounds.
func (s *Service) MintToken(ctx context.Context, tenantID, userID string, ttlSeconds int) (string, time.Time, error) {
	if userID == "" {
		return "", time.Time{}, invalid("user_id is required")
	}
	if ttlSeconds <= 0 {
		ttlSeconds = s.cfg.Auth.DefaultTokenTTLSecs
	}
	if ttlSeconds > s.cfg.Auth.MaxTokenTTLSecs {
		ttlSeconds = s.cfg.Auth.MaxTokenTTLSecs
	}
	return s.keys.Mint(ctx, userID, tenantID, time.Duration(ttlSeconds)*time.Second)
}
