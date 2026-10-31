package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bitllow/sild/backend/internal/domain"
)

// identitySet configures embedded sign-in for a tenant and prints the embed key
// the host page puts in the iframe URL.
func identitySet(ctx context.Context, svc *domain.Service, args []string) error {
	fs := flag.NewFlagSet("identity set", flag.ExitOnError)
	tenant := fs.String("tenant", "", "tenant id")
	validatorURL := fs.String("validator-url", "", "the integrator's validator endpoint (docs/embedded-sign-in.md)")
	tokenFile := fs.String("validator-token-file", "", "file holding the API token Sild presents to the validator; omit to keep the stored one")
	origins := fs.String("origins", "", "comma-separated origins allowed to frame the inbox")
	embeddedOnly := fs.Bool("embedded-only", false, "members linked to the integrator may sign in only through it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := need("tenant", tenant, "validator-url", validatorURL, "origins", origins); err != nil {
		return err
	}
	in := domain.IdentityProviderInput{ValidatorURL: *validatorURL, AllowedOrigins: split(*origins), EmbeddedOnly: *embeddedOnly}
	if *tokenFile != "" {
		b, err := os.ReadFile(*tokenFile)
		if err != nil {
			return err
		}
		tok := strings.TrimSpace(string(b))
		in.ValidatorToken = &tok
	}
	v, err := svc.SetIdentityProvider(ctx, *tenant, in)
	if err != nil {
		return err
	}
	printIdentity(v)
	fmt.Printf("\nlink members with: sild-admin agent link --tenant %s --email … --sub …\n", *tenant)
	return nil
}

func identityShow(ctx context.Context, svc *domain.Service, args []string) error {
	fs := flag.NewFlagSet("identity show", flag.ExitOnError)
	tenant := fs.String("tenant", "", "tenant id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := need("tenant", tenant); err != nil {
		return err
	}
	v, err := svc.GetIdentityProvider(ctx, *tenant)
	if err != nil {
		return err
	}
	printIdentity(v)
	return nil
}

func printIdentity(v *domain.IdentityProviderView) {
	token := "not set"
	if v.HasValidatorToken {
		token = "set (write-only)"
	}
	fmt.Printf("embed_key  %s\n", v.EmbedKey)
	fmt.Printf("validator  %s\n", v.ValidatorURL)
	fmt.Printf("token      %s\n", token)
	fmt.Printf("origins    %s\n", strings.Join(v.AllowedOrigins, " "))
	fmt.Printf("embedded-only %t\n", v.EmbeddedOnly)
}

func identityRemove(ctx context.Context, svc *domain.Service, args []string) error {
	fs := flag.NewFlagSet("identity remove", flag.ExitOnError)
	tenant := fs.String("tenant", "", "tenant id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := need("tenant", tenant); err != nil {
		return err
	}
	if err := svc.DeleteIdentityProvider(ctx, *tenant); err != nil {
		return err
	}
	fmt.Println("embedded sign-in removed; its sessions are ended")
	return nil
}

// agentLink sets the identity a member signs in with from the integrator's app.
func agentLink(ctx context.Context, svc *domain.Service, args []string) error {
	fs := flag.NewFlagSet("agent link", flag.ExitOnError)
	tenant := fs.String("tenant", "", "tenant id")
	email := fs.String("email", "", "operator email")
	sub := fs.String("sub", "", "the member's identity at the integrator")
	unlink := fs.Bool("unlink", false, "remove the link")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := need("tenant", tenant, "email", email); err != nil {
		return err
	}
	if (*sub == "") != *unlink {
		return errors.New("pass exactly one of --sub or --unlink")
	}
	admin, err := lookup(ctx, svc, *tenant, *email)
	if err != nil {
		return err
	}
	if err := svc.SetExternalSub(ctx, *tenant, admin.ID, *sub); err != nil {
		return err
	}
	if *unlink {
		fmt.Printf("%s unlinked\n", *email)
	} else {
		fmt.Printf("%s linked to %s\n", *email, *sub)
	}
	return nil
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
