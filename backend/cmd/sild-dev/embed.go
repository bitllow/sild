package main

import (
	"context"
	"log"
	"os"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// devEmbedKey is fixed so the fake integrator's iframe URL needs no lookup.
const devEmbedKey = "emb_dev_fleetline"

// devValidatorToken matches FAKE_VALIDATOR_TOKEN's default in tools/fake-integrator.
const devValidatorToken = "fleetline-dev-validator-token"

// devSeedEmbed wires embedded sign-in to tools/fake-integrator: its /validate
// endpoint, and the seeded staff linked to the integrator's account ids
// ("{n}.{i}"). SILD_DEV_EMBED=0 skips it.
func devSeedEmbed(ctx context.Context, st store.Store, svc *domain.Service, tenantID, queue string) {
	if os.Getenv("SILD_DEV_EMBED") == "0" {
		return
	}
	origin := envOr("SILD_DEV_EMBED_ORIGIN", "http://localhost:4400")
	validator := envOr("SILD_DEV_EMBED_VALIDATOR_URL", origin+"/validate")
	token := envOr("SILD_DEV_EMBED_VALIDATOR_TOKEN", devValidatorToken)
	if _, err := svc.SetIdentityProvider(ctx, tenantID, domain.IdentityProviderInput{
		ValidatorURL: validator, ValidatorToken: &token,
		AllowedOrigins: []string{origin}, EmbedKey: devEmbedKey,
	}); err != nil {
		log.Printf("dev seed embed: %v", err)
		return
	}
	if admins, _ := st.Admins().FindByEmail(ctx, "liis@sild.local"); len(admins) == 0 {
		liis, err := svc.InviteAgent(ctx, tenantID, "liis@sild.local", "Liis", "Mägi", models.PlatformAgent,
			models.RoleScope{Queues: []string{models.QueueSupport, queue}})
		if err != nil {
			log.Printf("dev seed embed agent: %v", err)
			return
		}
		_ = svc.SetAdminPassword(ctx, tenantID, liis.ID, "password123")
	}
	links := map[string]string{
		"admin@sild.local": "fleetline.2",
		"liis@sild.local":  "fleetline.1",
		"marko@sild.local": "fleetline.7",
	}
	for email, sub := range links {
		admin, err := svc.FindAdminByEmail(ctx, tenantID, email)
		if err == nil {
			err = svc.SetExternalSub(ctx, tenantID, admin.ID, sub)
		}
		if err != nil {
			log.Printf("dev seed embed link %s: %v", email, err)
		}
	}
	log.Printf("│ embedded  : key %s, framed by %s, validated by %s (tools/fake-integrator)", devEmbedKey, origin, validator)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
