package domain

import (
	"context"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// RegisterPush stores a device token (§5.5). Upsert by token so re-registration
// and OS rotation are idempotent; one user may have many devices.
func (s *Service) RegisterPush(ctx context.Context, tenantID string, owner store.Participant, platform models.PushPlatform, token string) error {
	if token == "" {
		return invalid("token is required")
	}
	if platform == "" {
		return invalid("platform is required")
	}
	return s.store.PushTokens().Upsert(ctx, &models.PushToken{
		TenantID: tenantID, MemberKind: owner.Kind,
		ExternalUserID: owner.ExternalUserID, InternalActorID: owner.InternalActorID,
		Platform: platform, Token: token,
	})
}

// DeregisterPush removes a device token, scoped to the owner so a signed-out
// device can't receive the next user's messages (§5.5).
func (s *Service) DeregisterPush(ctx context.Context, tenantID string, owner store.Participant, token string) error {
	if token == "" {
		return invalid("token is required")
	}
	return s.store.PushTokens().DeleteByToken(ctx, tenantID, token, owner)
}
