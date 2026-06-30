package mail

import (
	"strings"
	"testing"
)

func TestBuildMessage(t *testing.T) {
	out := buildMessage(OutboundEmail{
		To:          "cust@x.com",
		FromName:    "Acme Support",
		FromAddress: "support@inbound.test",
		Subject:     "Re: your ticket",
		Body:        "Hello there.",
		ReplyTo:     "support@inbound.test",
	})
	msg := string(out)

	for _, want := range []string{
		"From: Acme Support <support@inbound.test>\r\n",
		"To: cust@x.com\r\n",
		"Reply-To: support@inbound.test\r\n",
		"Subject: Re: your ticket\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q\n---\n%s", want, msg)
		}
	}
	// Headers are separated from the body by a blank line, body last.
	if !strings.HasSuffix(msg, "\r\nHello there.") {
		t.Fatalf("body not at the end:\n%s", msg)
	}
}

func TestBuildMessageNoFromName(t *testing.T) {
	out := string(buildMessage(OutboundEmail{To: "a@b.com", FromAddress: "x@y.com", Subject: "s", Body: "b"}))
	if !strings.Contains(out, "From: x@y.com\r\n") {
		t.Fatalf("bare From header expected, got:\n%s", out)
	}
}
