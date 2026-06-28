// Package mail holds transport-neutral email types and the Mailer interface
// (§6.2). The platform stores no mailboxes — a provider (SendGrid/Postmark/
// Mailgun) handles transport; connector/email implements Mailer against one.
package mail

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// InboundEmail is a normalized parsed inbound message (provider-agnostic).
type InboundEmail struct {
	Recipient   string // the address it was sent to (domain → tenant)
	From        string // sender address
	Subject     string
	TextBody    string
	Attachments []InboundAttachment
	RawBody     []byte            // for signature verification
	Headers     map[string]string // provider signature headers
}

// InboundAttachment is a parsed attachment (bytes already at the bucket key).
type InboundAttachment struct {
	ObjectKey string
	MimeType  string
	SizeBytes int64
	Filename  string
}

// OutboundEmail is a message leaving via email (§6.2).
type OutboundEmail struct {
	To          string
	FromName    string
	FromAddress string
	Subject     string
	Body        string
	ThreadToken string // embedded in subject + Reply-To for thread resolution
	ReplyTo     string
}

// Mailer sends outbound email. nil-safe via NoopMailer.
type Mailer interface {
	Send(ctx context.Context, msg OutboundEmail) error
}

// NoopMailer drops mail (default until a provider is configured).
type NoopMailer struct{}

func (NoopMailer) Send(context.Context, OutboundEmail) error { return nil }

// SignatureVerifier gates the inbound endpoint (§6.2 required gate).
type SignatureVerifier interface {
	Verify(secret string, raw []byte, headers map[string]string) bool
}

// HMACVerifier verifies X-Signature: sha256=<hex hmac(secret, raw)>. Real
// providers vary; this is the canonical scheme and the default.
type HMACVerifier struct{}

func (HMACVerifier) Verify(secret string, raw []byte, headers map[string]string) bool {
	got := headers["X-Signature"]
	got = strings.TrimPrefix(got, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(got), []byte(want))
}
