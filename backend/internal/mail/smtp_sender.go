package mail

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// SMTPMailer delivers outbound email through a configured relay (§6.2). Agent
// replies leave through it carrying the thread token in the Subject + Reply-To
// so the recipient's reply threads back into the same conversation.
type SMTPMailer struct {
	addr string    // host:port of the relay
	auth smtp.Auth // nil when the relay needs no auth
	from string    // fallback From when a message sets none
}

// NewSMTPMailer builds a relay-backed mailer. PLAIN auth is used when a user is
// configured; otherwise the relay is treated as open (dev / internal relays).
func NewSMTPMailer(addr, user, pass, from string) *SMTPMailer {
	var auth smtp.Auth
	if user != "" {
		host := addr
		if i := strings.LastIndexByte(addr, ':'); i >= 0 {
			host = addr[:i]
		}
		auth = smtp.PlainAuth("", user, pass, host)
	}
	return &SMTPMailer{addr: addr, auth: auth, from: from}
}

func (m *SMTPMailer) Send(_ context.Context, msg OutboundEmail) error {
	if msg.FromAddress == "" {
		msg.FromAddress = m.from
	}
	return smtp.SendMail(m.addr, m.auth, msg.FromAddress, []string{msg.To}, buildMessage(msg))
}

// buildMessage renders an OutboundEmail to RFC-5322 wire bytes. Pure (no I/O) so
// it is unit-testable.
func buildMessage(msg OutboundEmail) []byte {
	fromHeader := msg.FromAddress
	if msg.FromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", msg.FromName, msg.FromAddress)
	}
	var b strings.Builder
	b.WriteString("From: " + fromHeader + "\r\n")
	b.WriteString("To: " + msg.To + "\r\n")
	if msg.ReplyTo != "" {
		b.WriteString("Reply-To: " + msg.ReplyTo + "\r\n")
	}
	b.WriteString("Subject: " + msg.Subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.Body)
	return []byte(b.String())
}
