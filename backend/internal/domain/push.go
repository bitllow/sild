package domain

import (
	"context"
	"errors"
	"time"

	"github.com/bitllow/sild/backend/internal/push"
	"github.com/bitllow/sild/backend/internal/secrets"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// RegisterPush stores a device token (§5.5). Upsert by token so re-registration
// and OS rotation are idempotent; one user may have many devices.
func (s *Service) RegisterPush(ctx context.Context, tenantID string, owner store.Participant, platform models.PushPlatform, token string) error {
	if token == "" {
		return invalid("token is required")
	}
	if !platform.Valid() {
		return invalid("platform must be ios, android or web")
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

// PushConfigView is the readable half of a tenant's push credential. The
// credential itself is never returned — not to an owner, not to anyone.
type PushConfigView struct {
	ProjectID     string    `json:"project_id"`
	ClientEmail   string    `json:"client_email"`
	Verified      bool      `json:"verified"`
	UpdatedAt     time.Time `json:"updated_at"`
	IncludeSender bool      `json:"include_sender"`
	IncludeBody   bool      `json:"include_body"`
	SenderSource  string    `json:"sender_source"`
}

// GetPushConfig returns the tenant's push setup. A tenant with no credential
// still has notification settings, so this reports them either way.
func (s *Service) GetPushConfig(ctx context.Context, tenantID string) (*PushConfigView, error) {
	tenant, err := s.store.Tenants().Get(ctx, tenantID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	v := &PushConfigView{
		IncludeSender: tenant.PushIncludeSender,
		IncludeBody:   tenant.PushIncludeBody,
		SenderSource:  string(tenant.PushSenderSource),
	}
	cfg, err := s.store.PushConfigs().Get(ctx, tenantID)
	if errors.Is(err, store.ErrNotFound) {
		return v, nil
	}
	if err != nil {
		return nil, err
	}
	v.ProjectID, v.ClientEmail, v.Verified, v.UpdatedAt = cfg.ProjectID, cfg.ClientEmail, cfg.Verified, cfg.UpdatedAt
	return v, nil
}

// SetPushCredential seals a tenant's push credential after checking that it can
// actually authenticate. Replacing it clears Verified: a new project has proved
// nothing yet.
func (s *Service) SetPushCredential(ctx context.Context, tenantID string, raw []byte) error {
	sa, err := push.ParseServiceAccount(raw)
	if err != nil {
		return invalid(err.Error())
	}
	if !s.secrets.Configured() {
		return invalid(secrets.ErrNoKey.Error())
	}
	if err := s.notifier.Check(ctx, push.Credential{ProjectID: sa.ProjectID, ServiceAccountJSON: raw}); err != nil {
		return invalid("credential was rejected by the push provider: " + err.Error())
	}
	sealed, err := s.secrets.Seal(raw)
	if err != nil {
		return err
	}
	return s.store.PushConfigs().Upsert(ctx, &models.TenantPushConfig{
		TenantID: tenantID, ProjectID: sa.ProjectID, ClientEmail: sa.ClientEmail,
		CredentialSealed: sealed, CredentialKeyID: s.secrets.KeyID(), Verified: false,
	})
}

// DeletePushCredential removes the credential, stopping delivery for the tenant.
func (s *Service) DeletePushCredential(ctx context.Context, tenantID string) error {
	return s.store.PushConfigs().Delete(ctx, tenantID)
}

// PushSettings are the tenant's choices about what a nudge reveals.
type PushSettings struct {
	IncludeSender bool
	IncludeBody   bool
	SenderSource  models.PushSenderSource
}

// SetPushSettings updates what a nudge may say. Tenant-level: one tenant has one
// privacy posture, whatever brands it runs.
func (s *Service) SetPushSettings(ctx context.Context, tenantID string, in PushSettings) error {
	if !in.SenderSource.Valid() {
		return invalid("sender_source must be brand or agent")
	}
	return s.store.Tenants().SetPushSettings(ctx, tenantID, in.IncludeSender, in.IncludeBody, in.SenderSource)
}

// TestPushSend delivers a labelled test nudge to one device token, so a tenant
// can confirm the whole path before shipping.
func (s *Service) TestPushSend(ctx context.Context, tenantID, token string) error {
	if token == "" {
		return invalid("token is required")
	}
	cfg, err := s.store.PushConfigs().Get(ctx, tenantID)
	if errors.Is(err, store.ErrNotFound) {
		return invalid("no push credential is configured")
	}
	if err != nil {
		return err
	}
	raw, err := s.secrets.Open(cfg.CredentialSealed, cfg.CredentialKeyID)
	if err != nil {
		return err
	}
	err = s.notifier.Notify(ctx,
		push.Credential{ProjectID: cfg.ProjectID, ServiceAccountJSON: raw},
		push.Target{Token: token},
		push.Nudge{Title: "Test notification", Body: "Push is wired up correctly."})
	// The two inputs fail differently and the tenant fixes them in different
	// places, so the setup screen must be able to tell them apart.
	switch {
	case errors.Is(err, push.ErrTokenDead):
		return invalid("device token was rejected: " + err.Error())
	case errors.Is(err, push.ErrCredential):
		return invalid("credential was rejected by the push provider: " + err.Error())
	case err != nil:
		return err
	}
	return s.store.PushConfigs().MarkVerified(ctx, tenantID)
}

// DeleteUserPushTokens drops every device a user registered — the host's
// account-deletion call. The app can register again, so this is not a
// preference; SetContactPush is.
func (s *Service) DeleteUserPushTokens(ctx context.Context, tenantID, externalUserID string) (int, error) {
	if externalUserID == "" {
		return 0, invalid("user_id is required")
	}
	return s.store.PushTokens().DeleteForUser(ctx, tenantID, externalUserID)
}

// enqueuePush queues a message's nudge INSIDE the caller's tx, so a crash
// between the two cannot lose it (§5.5).
func (s *Service) enqueuePush(ctx context.Context, tx store.Store, msg *models.Message) error {
	return tx.PushOutbox().Enqueue(ctx, &models.PushOutbox{
		TenantID: msg.TenantID, ConversationID: msg.ConversationID, MessageID: msg.ID,
	})
}
