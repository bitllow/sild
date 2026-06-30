package domain

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strings"

	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/mail"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
)

// subjectPrefixRe strips leading reply/forward prefixes (Re:, Fwd:, Fw:, Aw:,
// Sv:, …) so a reply threads to the same conversation as the original (§6.2).
var subjectPrefixRe = regexp.MustCompile(`(?i)^\s*(re|fwd?|aw|sv|antw|wg)\s*:\s*`)

// maxSubjectKeyLen caps the threading key to the SubjectKey column width
// (varchar(255)) so an oversized Subject header can't fail the inbound insert.
const maxSubjectKeyLen = 255

// normalizeSubject is the threading key: prefixes stripped, whitespace collapsed,
// lowercased, and truncated to the column width. Two emails with the "same"
// subject (modulo Re:/Fwd:) match.
func normalizeSubject(s string) string {
	s = strings.TrimSpace(s)
	for {
		stripped := subjectPrefixRe.ReplaceAllString(s, "")
		if stripped == s {
			break
		}
		s = stripped
	}
	key := strings.ToLower(strings.Join(strings.Fields(s), " "))
	if r := []rune(key); len(r) > maxSubjectKeyLen {
		key = string(r[:maxSubjectKeyLen]) // rune-safe cap (varchar(255) is char-counted)
	}
	return key
}

// replySubject renders an outbound subject as "Re: <original>" (no token —
// threading rides the Reply-To +subaddress, not the visible subject).
func replySubject(subject string) string {
	s := strings.TrimSpace(subject)
	if s == "" {
		return "Re: your message"
	}
	if strings.HasPrefix(strings.ToLower(s), "re:") {
		return s
	}
	return "Re: " + s
}

// threadTokenRe finds an embedded thread token "sild#<tok>". It's set in the
// outbound Reply-To as a +subaddress, so a reply carries it back in To/recipient
// (surviving forwarding as a header) while the visible subject stays clean.
var threadTokenRe = regexp.MustCompile(`sild#([A-Za-z0-9_]+)`)

// extractThreadToken pulls the thread token from the places a reply preserves it
// — the recipient and the To/Cc headers (+sild#<tok> subaddress), or the subject
// as a last resort. Returns "" when none is present.
func extractThreadToken(in mail.InboundEmail) string {
	for _, s := range []string{in.Recipient, headerValue(in.Headers, "To"), headerValue(in.Headers, "Cc"), in.Subject} {
		if m := threadTokenRe.FindStringSubmatch(s); m != nil {
			return m[1]
		}
	}
	return ""
}

// replyToWithToken plus-addresses the tenant from-address with the thread token,
// so a reply threads back precisely without touching the visible subject (§6.2).
func replyToWithToken(fromAddress, token string) string {
	if fromAddress == "" || token == "" {
		return ""
	}
	at := strings.IndexByte(fromAddress, '@')
	if at < 0 {
		return fromAddress
	}
	return fromAddress[:at] + "+sild#" + token + fromAddress[at:]
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
	return s.ingestAndNotify(ctx, cfg, in)
}

// HandleForwarded ingests an email caught by the sild-mail forwarding daemon
// (§6.2). The daemon is itself the trusted boundary (it sits behind the MX), so
// there is no provider signature: the tenant is resolved by the forwarding
// token in the recipient's local part. Autoresponders are dropped when the
// tenant enables spam filtering, and the first successful ingest verifies the
// tenant's forwarding setup.
func (s *Service) HandleForwarded(ctx context.Context, in mail.InboundEmail) (*models.Message, error) {
	token := forwardingToken(in.Recipient)
	if token == "" {
		return nil, invalid("invalid recipient")
	}
	cfg, err := s.store.Tenants().FindByInboundToken(ctx, token)
	if err != nil {
		return nil, ErrNotFound // unknown forwarding address — drop
	}
	if cfg.SpamFilter && looksLikeAutoresponder(in) {
		return nil, nil // silently dropped; not an error
	}
	msg, err := s.ingestAndNotify(ctx, cfg, in)
	if err != nil {
		return nil, err
	}
	if !cfg.Verified {
		cfg.Verified = true
		_ = s.store.Tenants().SetEmailConfig(ctx, cfg)
	}
	return msg, nil
}

// ForwardedMailHandler returns the ingest handler the SMTP forwarding daemon
// runs (cmd/sild-mail). It ingests each message and returns an error ONLY for
// transient failures, so the daemon asks the MTA to retry instead of
// acknowledging — otherwise a transient DB error would silently lose the mail.
// Intentional drops (spam filtered) and permanent rejects (unknown forwarding
// address, invalid recipient) are acknowledged so the sender isn't retried.
func (s *Service) ForwardedMailHandler() mail.Handler {
	return func(ctx context.Context, in mail.InboundEmail) error {
		msg, err := s.HandleForwarded(ctx, in)
		switch {
		case err == nil && msg != nil:
			log.Printf("sild-mail: ingested mail from %s to %s → conversation %s", in.From, in.Recipient, msg.ConversationID)
			return nil
		case err == nil:
			log.Printf("sild-mail: dropped mail from %s to %s (spam filter)", in.From, in.Recipient)
			return nil
		case errors.Is(err, ErrNotFound):
			log.Printf("sild-mail: dropped mail to %s (no tenant for that forwarding address)", in.Recipient)
			return nil
		case errors.Is(err, ErrValidation), errors.Is(err, ErrForbidden):
			log.Printf("sild-mail: rejected mail to %s: %v", in.Recipient, err)
			return nil // permanent/intentional — don't make the MTA retry forever
		default:
			log.Printf("sild-mail: transient error for mail to %s: %v", in.Recipient, err)
			return err // transient — surface it so the daemon returns a 4xx
		}
	}
}

// ingestAndNotify resolves the thread and appends or creates, then fires the
// auto-reply acknowledgement when a new conversation was opened (§6.2). Shared
// by the provider webhook (HandleInbound) and the forwarding daemon.
func (s *Service) ingestAndNotify(ctx context.Context, cfg *models.TenantEmailConfig, in mail.InboundEmail) (*models.Message, error) {
	msg, created, err := s.ingest(ctx, cfg.TenantID, in)
	if err != nil {
		return nil, err
	}
	if created && cfg.AutoReply {
		s.sendAutoReply(ctx, cfg, msg.ConversationID, in.From)
	}
	return msg, nil
}

// ingest binds the email to an existing OPEN conversation by sender + normalized
// subject, or creates a new one. The bool reports whether a new conversation was
// created. No special tokens — threading is sender + subject (§6.2).
func (s *Service) ingest(ctx context.Context, tenantID string, in mail.InboundEmail) (*models.Message, bool, error) {
	if len(in.RawAttachments) > 0 { // daemon path: upload in-memory bytes to the bucket
		uploaded, err := s.uploadInboundAttachments(ctx, tenantID, in.RawAttachments)
		if err != nil {
			return nil, false, err // fail before creating the message so the MTA retries cleanly
		}
		in.Attachments = append(in.Attachments, uploaded...)
	}
	// Prefer the explicit thread token when a reply carries one (it survives
	// subject edits); otherwise bind by sender + normalized subject. Both match
	// only OPEN conversations — closed is terminal (§1).
	if token := extractThreadToken(in); token != "" {
		if thread, err := s.store.Email().FindOpenByToken(ctx, tenantID, token); err == nil {
			msg, err := s.appendInbound(ctx, tenantID, thread, in)
			return msg, false, err
		}
	}
	if thread, err := s.store.Email().FindOpenBySenderSubject(ctx, tenantID, in.From, normalizeSubject(in.Subject)); err == nil {
		msg, err := s.appendInbound(ctx, tenantID, thread, in)
		return msg, false, err
	}
	msg, err := s.createFromInbound(ctx, tenantID, in)
	return msg, err == nil, err
}

// sendAutoReply emails the sender an acknowledgement. The reply keeps the
// original subject ("Re: …") so the sender's response threads back by
// sender + subject — no token needed (§6.2).
func (s *Service) sendAutoReply(ctx context.Context, cfg *models.TenantEmailConfig, convID, to string) {
	if to == "" {
		return
	}
	out := mail.OutboundEmail{
		To: to, FromName: cfg.FromName, FromAddress: cfg.FromAddress,
		Subject: "Re: your message",
		Body:    "Thanks — we received your message and a member of our team will reply shortly.",
	}
	if t, err := s.store.Email().Get(ctx, cfg.TenantID, convID); err == nil {
		out.Subject = replySubject(t.Subject)
		out.ReplyTo = replyToWithToken(cfg.FromAddress, t.ThreadToken)
	}
	_ = s.mailer.Send(ctx, out)
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
	_ = applyMessageActivity(ctx, s.store, msg)
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
	assignment := &models.Assignment{
		TenantID: tenantID, Status: models.AssignmentQueued, CreatedAt: s.now(),
	}

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
		assignment.ConversationID = conv.ID
		if err := tx.Assignments().Create(ctx, assignment); err != nil {
			return err
		}
		if err := tx.Email().CreateThread(ctx, &models.EmailThread{
			ConversationID: conv.ID, TenantID: tenantID, ThreadToken: id.New("thr"),
			Sender: from, SubjectKey: normalizeSubject(in.Subject), Subject: in.Subject,
			LastAddress: from,
		}); err != nil {
			return err
		}
		msg = &models.Message{
			TenantID: tenantID, ConversationID: conv.ID,
			SenderKind: models.SenderUser, Visibility: models.VisibilityParticipants,
			Channel: models.ChannelEmail, ExternalUserID: &from, Body: in.TextBody, CreatedAt: s.now(),
			Attachments: inboundAttachments(tenantID, in.Attachments),
		}
		if err := tx.Messages().Create(ctx, msg); err != nil {
			return err
		}
		return applyMessageActivity(ctx, tx, msg)
	})
	if err != nil {
		return nil, err
	}
	_ = s.fireWebhook(ctx, tenantID, conv.ID, "conversation.created",
		map[string]any{"id": conv.ID, "channel": "email"})
	// Nudge the agents channel so the new email conversation surfaces in the inbox
	// queue live, without a refresh (§8) — same fan-out as host-created requests.
	s.emit(ctx, realtime.Target{Tenant: tenantID}, realtime.EventAssignmentUpdated, conv.ID, views.Assignment(assignment))
	return msg, nil
}

// maybeSendOutboundEmail emails a participants message to any email member whose
// address differs from the sender, with a clean "Re: <subject>" so the reply
// threads back by sender + subject (§6.2). Called from SendMessage. Internal
// notes are never emailed (§5.6).
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
	cfg, _ := s.store.Tenants().GetEmailConfig(ctx, tenantID)
	thread, _ := s.store.Email().Get(ctx, tenantID, convID)
	subject, replyTo := "Re: your message", ""
	if thread != nil {
		subject = replySubject(thread.Subject)
		if cfg != nil {
			replyTo = replyToWithToken(cfg.FromAddress, thread.ThreadToken)
		}
	}
	for _, to := range recipients {
		out := mail.OutboundEmail{To: to, Subject: subject, Body: msg.Body, ReplyTo: replyTo}
		if cfg != nil {
			out.FromName, out.FromAddress = cfg.FromName, cfg.FromAddress
		}
		_ = s.mailer.Send(ctx, out)
	}
}

// EmailSubject returns the original subject of a conversation's email thread (the
// inbox renders it in place of the opaque conversation id), or "" if there is no
// email thread.
func (s *Service) EmailSubject(ctx context.Context, tenantID, convID string) string {
	if t, err := s.store.Email().Get(ctx, tenantID, convID); err == nil {
		return t.Subject
	}
	return ""
}

// uploadInboundAttachments writes the daemon's in-memory attachment bytes to the
// bucket and returns the resulting object-key references. A failed upload errors
// the whole ingest rather than silently dropping the attachment — the caller
// surfaces it (transient → the MTA retries) instead of acknowledging a message
// with the sender's file missing.
func (s *Service) uploadInboundAttachments(ctx context.Context, tenantID string, raw []mail.ParsedAttachment) ([]mail.InboundAttachment, error) {
	out := make([]mail.InboundAttachment, 0, len(raw))
	for _, a := range raw {
		key := s.bucket.NewObjectKey(tenantID, a.Filename)
		if err := s.bucket.Put(ctx, key, a.Content, a.MimeType); err != nil {
			return nil, err
		}
		out = append(out, mail.InboundAttachment{
			ObjectKey: key, MimeType: a.MimeType, SizeBytes: int64(len(a.Content)), Filename: a.Filename,
		})
	}
	return out, nil
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

// forwardingToken extracts the tenant's forwarding token from a recipient
// address: the local part, lowercased, with any +subaddress stripped (tokens
// are minted lowercase so they survive MTA case-folding). e.g.
// "eml_01j9z3+anything@inbound.sild.io" → "eml_01j9z3".
func forwardingToken(addr string) string {
	at := strings.LastIndexByte(addr, '@')
	if at <= 0 {
		return ""
	}
	local := addr[:at]
	if plus := strings.IndexByte(local, '+'); plus >= 0 {
		local = local[:plus]
	}
	return strings.ToLower(local)
}

// looksLikeAutoresponder reports whether an inbound email is an out-of-office,
// bounce, or other machine-generated reply that should be kept out of the queue
// (§6.2 autoresponder spam filtering). It checks the standard auto-submission
// headers and no-reply/daemon sender patterns.
func looksLikeAutoresponder(in mail.InboundEmail) bool {
	get := func(k string) string { return strings.ToLower(strings.TrimSpace(headerValue(in.Headers, k))) }
	if v := get("Auto-Submitted"); v != "" && v != "no" {
		return true // RFC 3834: auto-generated/auto-replied
	}
	switch get("Precedence") {
	case "bulk", "auto_reply", "junk", "list":
		return true
	}
	if get("X-Autoreply") != "" || get("X-Autorespond") != "" || get("X-Auto-Response-Suppress") != "" {
		return true
	}
	// Match the sender's local part EXACTLY against reserved/no-reply mailboxes —
	// a substring match would wrongly drop a human like "jane.noreply@x.com".
	local := strings.ToLower(in.From)
	if at := strings.IndexByte(local, '@'); at >= 0 {
		local = local[:at]
	}
	switch local {
	case "mailer-daemon", "postmaster", "no-reply", "noreply", "do-not-reply", "donotreply":
		return true
	}
	return false
}

// headerValue does a case-insensitive lookup against the parsed header map.
func headerValue(h map[string]string, key string) string {
	if h == nil {
		return ""
	}
	if v, ok := h[key]; ok {
		return v
	}
	for k, v := range h {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
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
