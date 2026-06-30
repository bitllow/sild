// Command sild-mail is the email forwarding ingestion daemon (§6.2). Each tenant
// gets a forwarding address <inbound_token>@<SILD_EMAIL_INBOUND_DOMAIN>; an org
// forwards its support mailbox to it, MX records point at this daemon, and every
// message that arrives becomes (or threads into) a conversation in the inbox.
//
//	sild-mail            # listens on SILD_SMTP_ADDR (default :2525)
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/di"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/mail"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c, err := di.New()
	if err != nil {
		log.Fatalf("di: %v", err)
	}

	err = c.Invoke(func(cfg *config.Config, svc *domain.Service) error {
		log.Printf("sild-mail: ingesting forwarded mail for *@%s", cfg.Email.InboundDomain)
		// The handler ingests each message, acknowledging intentional drops and
		// permanent rejects while surfacing transient failures so the MTA retries.
		return mail.Serve(ctx, cfg.Email.SMTPListenAddr, svc.ForwardedMailHandler())
	})
	if err != nil {
		log.Fatalf("sild-mail: %v", err)
	}
}
