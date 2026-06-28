package domain

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/mail"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
)

// threadTokenRe extracts the thread token embedded in a subject as [sild#<tok>].
var threadTokenRe = regexp.MustCompile(`sild#([A-Za-z0-9_]+)`)

// SubjectWithToken decorates an outbound subject with the thread token (§6.2).
func SubjectWithToken(subject, token string) string {
	if subject == "" {
		subject = "Re: your conversation"
	}
	return subject + " [sild#" + token + "]"
}

// HandleInbound processes a parsed inbound email (§6.2): verify happens at the
// handler; here we resolve the tenant by recipient domain, enforce the allowlist,
// resolve the thread, and append or create — atomically for the create path.
func (s *Service) HandleInbound(ctx context.Context, in mail.InboundEmail) (*models.Message, error) {
	domainPart := addressDomain(in.Recipient)
	if domainPart == "" {
		return nil, invalid("invalid recipient")
	}
	cfg, err := s.store.Tenants().FindByInboundDomain(ctx, domainPart)
	if err != nil {
		return nil, ErrNotFound // unknown inbound domain — drop
	}
	if !domainAllowed(cfg, domainPart) {
		return nil, ErrForbidden
	}
	// Required gate (§6.2): verify the provider signature. A configured secret
	// must validate; without a secret, only non-production may proceed.
	if cfg.SigningSecret != "" {
		if !s.verifier.Verify(cfg.SigningSecret, in.RawBody, in.Headers) {
			return nil, ErrForbidden
		}
	} else if s.cfg.Env == "production" {
		return nil, ErrForbidden
	}
	tenantID := cfg.TenantID

	if m := threadTokenRe.FindStringSubmatch(in.Subject); m != nil {
		if thread, err := s.store.Email().FindByToken(ctx, m[1]); err == nil && thread.TenantID == tenantID {
			return s.appendInbound(ctx, tenantID, thread, in)
		}
	}
	return s.createFromInbound(ctx, tenantID, in)
}

func (s *Service) appendInbound(ctx context.Context, tenantID string, thread *models.EmailThread, in mail.InboundEmail) (*models.Message, error) {
	from := in.From
	msg := &models.Message{
		TenantID: tenantID, ConversationID: thread.ConversationID,
		SenderKind: models.SenderUser, Visibility: models.VisibilityParticipants,
		Channel: models.ChannelEmail, ExternalUserID: &from, Body: in.TextBody, CreatedAt: s.now(),
		Attachments: inboundAttachments(tenantID, in.Attachments),
	}
	if err := s.store.Messages().Create(ctx, msg); err != nil {
		return nil, err
	}
	thread.LastAddress = from
	thread.LastMessageID = msg.ID
	_ = s.store.Email().Update(ctx, thread)

	data := views.Message(msg, s.attachmentURLFunc())
	s.emit(ctx, realtime.Target{Conversation: thread.ConversationID}, realtime.EventMessageCreated, thread.ConversationID, data)
	_ = s.fireWebhook(ctx, tenantID, thread.ConversationID, "message.created", data)
	return msg, nil
}

// createFromInbound opens a conversation + email member + queued assignment +
// thread + first message in one transaction (§6.2, §1).
func (s *Service) createFromInbound(ctx context.Context, tenantID string, in mail.InboundEmail) (*models.Message, error) {
	from := in.From
	conv := &models.Conversation{TenantID: tenantID, Status: models.ConversationOpen, CreatedAt: s.now()}
	var msg *models.Message
	token := id.New("thr")

	err := s.store.Tx(ctx, func(tx store.Store) error {
		if err := tx.Conversations().Create(ctx, conv); err != nil {
			return err
		}
		if err := tx.Members().Add(ctx, &models.ConversationMember{
			TenantID: tenantID, ConversationID: conv.ID, MemberKind: models.MemberEmail,
			ExternalUserID: &from, ConvRole: models.RoleClient, JoinedAt: s.now(),
		}); err != nil {
			return err
		}
		if err := tx.Assignments().Create(ctx, &models.Assignment{
			TenantID: tenantID, ConversationID: conv.ID, Status: models.AssignmentQueued, CreatedAt: s.now(),
		}); err != nil {
			return err
		}
		if err := tx.Email().CreateThread(ctx, &models.EmailThread{
			ConversationID: conv.ID, TenantID: tenantID, ThreadToken: token, LastAddress: from,
		}); err != nil {
			return err
		}
		msg = &models.Message{
			TenantID: tenantID, ConversationID: conv.ID,
			SenderKind: models.SenderUser, Visibility: models.VisibilityParticipants,
			Channel: models.ChannelEmail, ExternalUserID: &from, Body: in.TextBody, CreatedAt: s.now(),
			Attachments: inboundAttachments(tenantID, in.Attachments),
		}
		return tx.Messages().Create(ctx, msg)
	})
	if err != nil {
		return nil, err
	}
	_ = s.fireWebhook(ctx, tenantID, conv.ID, "conversation.created",
		map[string]any{"id": conv.ID, "channel": "email"})
	return msg, nil
}

// maybeSendOutboundEmail emails a participants message to any email member whose
// address differs from the sender, embedding the thread token (§6.2). Called
// from SendMessage. internal notes are never emailed (§5.6).
func (s *Service) maybeSendOutboundEmail(ctx context.Context, tenantID, convID string, msg *models.Message) {
	if msg.Visibility != models.VisibilityParticipants || msg.Channel == models.ChannelEmail {
		return
	}
	members, err := s.store.Members().ListActive(ctx, tenantID, convID)
	if err != nil {
		return
	}
	var recipients []string
	for _, m := range members {
		if m.MemberKind == models.MemberEmail && m.ExternalUserID != nil &&
			(msg.ExternalUserID == nil || *m.ExternalUserID != *msg.ExternalUserID) {
			recipients = append(recipients, *m.ExternalUserID)
		}
	}
	if len(recipients) == 0 {
		return
	}
	token := s.threadToken(ctx, tenantID, convID)
	cfg, _ := s.store.Tenants().GetEmailConfig(ctx, tenantID)
	for _, to := range recipients {
		out := mail.OutboundEmail{
			To: to, Subject: SubjectWithToken("", token), Body: msg.Body,
			ThreadToken: token, ReplyTo: replyTo(cfg, token),
		}
		if cfg != nil {
			out.FromName, out.FromAddress = cfg.FromName, cfg.FromAddress
		}
		_ = s.mailer.Send(ctx, out)
	}
}

// threadToken returns the conversation's email thread token, creating one if the
// conversation has not yet been emailed.
func (s *Service) threadToken(ctx context.Context, tenantID, convID string) string {
	if t, err := s.store.Email().Get(ctx, tenantID, convID); err == nil {
		return t.ThreadToken
	} else if !errors.Is(err, store.ErrNotFound) {
		return ""
	}
	token := id.New("thr")
	_ = s.store.Email().CreateThread(ctx, &models.EmailThread{ConversationID: convID, TenantID: tenantID, ThreadToken: token})
	return token
}

func inboundAttachments(tenantID string, in []mail.InboundAttachment) []models.MessageAttachment {
	out := make([]models.MessageAttachment, 0, len(in))
	for _, a := range in {
		out = append(out, models.MessageAttachment{
			TenantID: tenantID, Disposition: models.DispositionAttachment,
			ObjectKey: a.ObjectKey, MimeType: a.MimeType, SizeBytes: a.SizeBytes, Filename: a.Filename,
		})
	}
	return out
}

func addressDomain(addr string) string {
	at := strings.LastIndexByte(addr, '@')
	if at < 0 || at == len(addr)-1 {
		return ""
	}
	return strings.ToLower(addr[at+1:])
}

func domainAllowed(cfg *models.TenantEmailConfig, domain string) bool {
	if strings.EqualFold(cfg.InboundDomain, domain) {
		return true
	}
	for _, d := range cfg.AllowedDomains {
		if strings.EqualFold(d.Domain, domain) {
			return true
		}
	}
	return false
}

func replyTo(cfg *models.TenantEmailConfig, token string) string {
	if cfg == nil || cfg.FromAddress == "" {
		return ""
	}
	at := strings.IndexByte(cfg.FromAddress, '@')
	if at < 0 {
		return cfg.FromAddress
	}
	return cfg.FromAddress[:at] + "+sild#" + token + cfg.FromAddress[at:]
}
