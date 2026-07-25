package domain

import (
	"context"
	"errors"
	"strings"

	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// EmailChannel is the resolved email-channel configuration the inbox Channels
// settings render (§6.2, §8): the forwarding address an org points its support
// mailbox at, plus verification status and the per-tenant toggles.
type EmailChannel struct {
	ForwardingAddress string
	InboundDomain     string
	Verified          bool
	AutoReply         bool
	SpamFilter        bool
	FromName          string
	FromAddress       string
}

// EmailChannelUpdate carries the fields the Channels UI may change. nil = leave
// as-is, so PATCH semantics fall out naturally.
type EmailChannelUpdate struct {
	AutoReply   *bool
	SpamFilter  *bool
	FromName    *string
	FromAddress *string
}

// GetEmailChannel returns the tenant's email-channel config, minting a
// forwarding token on first access so the Channels UI always has an address to
// display.
// GetEmailChannelVersioned returns the channel and the version OF THAT snapshot.
func (s *Service) GetEmailChannelVersioned(ctx context.Context, tenantID string) (*EmailChannel, string, error) {
	cfg, err := s.ensureEmailConfig(ctx, tenantID)
	if err != nil {
		return nil, "", err
	}
	return s.emailChannelView(cfg), Version(emailConfigView(cfg)), nil
}

func (s *Service) GetEmailChannel(ctx context.Context, tenantID string) (*EmailChannel, error) {
	cfg, err := s.ensureEmailConfig(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.emailChannelView(cfg), nil
}

// UpdateEmailChannel applies the toggles / sender fields set in the Channels UI.
func (s *Service) UpdateEmailChannel(ctx context.Context, tenantID string, p EmailChannelUpdate, expectVersion string) (*EmailChannel, string, error) {
	if _, err := s.ensureEmailConfig(ctx, tenantID); err != nil {
		return nil, "", err
	}

	// Re-read, verify and write in ONE transaction. Verifying outside it leaves a
	// window where two edits from the same version both pass.
	var cfg *models.TenantEmailConfig
	if err := s.store.Tx(ctx, func(tx store.Store) error {
		current, err := tx.Tenants().GetEmailConfig(ctx, tenantID)
		if err != nil {
			return err
		}
		if !versionMatches(expectVersion, Version(emailConfigView(current))) {
			return conflict(CodeStaleVersion, "the email channel changed since you read it")
		}
		apply(current, p)
		if err := tx.Tenants().SetEmailConfig(ctx, current); err != nil {
			return err
		}
		cfg = current
		return nil
	}); err != nil {
		return nil, "", mapStoreErr(err)
	}
	return s.emailChannelView(cfg), Version(emailConfigView(cfg)), nil
}

func (s *Service) emailChannelView(cfg *models.TenantEmailConfig) *EmailChannel {
	return &EmailChannel{
		ForwardingAddress: cfg.InboundToken + "@" + s.cfg.Email.InboundDomain,
		InboundDomain:     s.cfg.Email.InboundDomain,
		Verified:          cfg.Verified,
		AutoReply:         cfg.AutoReply,
		SpamFilter:        cfg.SpamFilter,
		FromName:          cfg.FromName,
		FromAddress:       cfg.FromAddress,
	}
}

// ensureEmailConfig loads the tenant's email config, creating it with a fresh
// forwarding token (spam filter defaulted on) if none exists yet. The token is
// minted lowercase so it survives MTA case-folding of the recipient local part.
func (s *Service) ensureEmailConfig(ctx context.Context, tenantID string) (*models.TenantEmailConfig, error) {
	cfg, err := s.store.Tenants().GetEmailConfig(ctx, tenantID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		cfg = &models.TenantEmailConfig{TenantID: tenantID, SpamFilter: true}
	}
	if cfg.InboundToken == "" {
		cfg.InboundToken = strings.ToLower(id.New("eml"))
		if err := s.store.Tenants().SetEmailConfig(ctx, cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// emailConfigView is the value an email-channel version is computed over —
// independent of the HTTP rendering, so both sides agree.
func emailConfigView(cfg *models.TenantEmailConfig) map[string]any {
	return map[string]any{
		"auto_reply": cfg.AutoReply, "spam_filter": cfg.SpamFilter,
		"from_name": cfg.FromName, "from_address": cfg.FromAddress,
		"verified": cfg.Verified, "inbound_token": cfg.InboundToken,
	}
}

// apply folds a patch onto a config; absent fields are left alone.
func apply(cfg *models.TenantEmailConfig, p EmailChannelUpdate) {
	if p.AutoReply != nil {
		cfg.AutoReply = *p.AutoReply
	}
	if p.SpamFilter != nil {
		cfg.SpamFilter = *p.SpamFilter
	}
	if p.FromName != nil {
		cfg.FromName = *p.FromName
	}
	if p.FromAddress != nil {
		cfg.FromAddress = *p.FromAddress
	}
}
