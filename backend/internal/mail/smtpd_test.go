package mail

import (
	"context"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseInboundPlain(t *testing.T) {
	raw := []byte("Subject: Need help\r\n" +
		"From: Mari <mari@x.com>\r\n" +
		"To: eml_x@inbound.test\r\n" +
		"\r\n" +
		"My order hasn't arrived.\r\n")
	in, err := ParseInbound(raw, "eml_x@inbound.test", "envelope@x.com")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if in.Subject != "Need help" {
		t.Fatalf("subject = %q", in.Subject)
	}
	if in.From != "mari@x.com" { // From header wins over the envelope sender
		t.Fatalf("from = %q, want mari@x.com", in.From)
	}
	if strings.TrimSpace(in.TextBody) != "My order hasn't arrived." {
		t.Fatalf("body = %q", in.TextBody)
	}
}

func TestParseInboundMultipartWithAttachment(t *testing.T) {
	raw := []byte("Subject: Receipt\r\n" +
		"From: cust@x.com\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=BOUND\r\n" +
		"\r\n" +
		"--BOUND\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		"See attached receipt.\r\n" +
		"--BOUND\r\n" +
		"Content-Type: text/plain; name=\"receipt.txt\"\r\n" +
		"Content-Disposition: attachment; filename=\"receipt.txt\"\r\n" +
		"\r\n" +
		"PAID $42\r\n" +
		"--BOUND--\r\n")
	in, err := ParseInbound(raw, "eml_x@inbound.test", "cust@x.com")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if strings.TrimSpace(in.TextBody) != "See attached receipt." {
		t.Fatalf("text part = %q", in.TextBody)
	}
	if len(in.RawAttachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(in.RawAttachments))
	}
	a := in.RawAttachments[0]
	if a.Filename != "receipt.txt" || strings.TrimSpace(string(a.Content)) != "PAID $42" {
		t.Fatalf("attachment = %+v (%q)", a, string(a.Content))
	}
}

func TestParseInboundMultipartAlternativePrefersText(t *testing.T) {
	raw := []byte("Subject: Hi\r\n" +
		"Content-Type: multipart/alternative; boundary=B\r\n" +
		"\r\n" +
		"--B\r\n" +
		"Content-Type: text/html\r\n\r\n<p>HTML body</p>\r\n" +
		"--B\r\n" +
		"Content-Type: text/plain\r\n\r\nPlain body\r\n" +
		"--B--\r\n")
	in, _ := ParseInbound(raw, "eml_x@inbound.test", "cust@x.com")
	if strings.TrimSpace(in.TextBody) != "Plain body" {
		t.Fatalf("expected text/plain to win, got %q", in.TextBody)
	}
}

// TestSMTPServerSession drives a full SMTP session over an ephemeral port and
// checks the handler is invoked once per RCPT TO with the parsed message.
func TestSMTPServerSession(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var got []InboundEmail
	done := make(chan struct{})
	go func() {
		_ = ServeListener(ctx, ln, func(_ context.Context, in InboundEmail) error {
			mu.Lock()
			got = append(got, in)
			mu.Unlock()
			return nil
		})
		close(done)
	}()

	addr := ln.Addr().String()
	body := "Subject: Hello\r\nFrom: cust@x.com\r\n\r\nHi there.\r\n"
	if err := smtp.SendMail(addr, nil, "cust@x.com",
		[]string{"eml_a@inbound.test", "eml_b@inbound.test"}, []byte(body)); err != nil {
		t.Fatalf("send: %v", err)
	}

	// Give the goroutine a moment to record both recipients.
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("handler invoked %d times, want 2 (one per recipient)", len(got))
	}
	rcpts := map[string]bool{got[0].Recipient: true, got[1].Recipient: true}
	if !rcpts["eml_a@inbound.test"] || !rcpts["eml_b@inbound.test"] {
		t.Fatalf("recipients = %v", rcpts)
	}
	if got[0].Subject != "Hello" || strings.TrimSpace(got[0].TextBody) != "Hi there." {
		t.Fatalf("parsed message wrong: %+v", got[0])
	}
}
