package auth

import (
	"context"
	"errors"
	"sync"

	"github.com/bitllow/sild/backend/internal/config"
	oidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// googleAuthenticator implements AdminAuthenticator against Google OIDC (§2.4).
// Provider discovery is lazy so construction never touches the network (keeps
// tests and non-Google deployments fast).
type googleAuthenticator struct {
	cfg config.Auth

	once     sync.Once
	initErr  error
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func newGoogleAuthenticator(cfg config.Auth) *googleAuthenticator {
	return &googleAuthenticator{cfg: cfg}
}

func (g *googleAuthenticator) init(ctx context.Context) error {
	g.once.Do(func() {
		provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
		if err != nil {
			g.initErr = err
			return
		}
		g.oauth = &oauth2.Config{
			ClientID:     g.cfg.GoogleClientID,
			ClientSecret: g.cfg.GoogleClientSecret,
			RedirectURL:  g.cfg.GoogleRedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "email"},
		}
		g.verifier = provider.Verifier(&oidc.Config{ClientID: g.cfg.GoogleClientID})
	})
	return g.initErr
}

func (g *googleAuthenticator) LoginURL(state string) string {
	if err := g.init(context.Background()); err != nil {
		return ""
	}
	return g.oauth.AuthCodeURL(state)
}

func (g *googleAuthenticator) Resolve(ctx context.Context, code string) (string, error) {
	if err := g.init(ctx); err != nil {
		return "", err
	}
	tok, err := g.oauth.Exchange(ctx, code)
	if err != nil {
		return "", err
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		return "", errors.New("no id_token in response")
	}
	idTok, err := g.verifier.Verify(ctx, rawID)
	if err != nil {
		return "", err
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idTok.Claims(&claims); err != nil {
		return "", err
	}
	if !claims.EmailVerified || claims.Email == "" {
		return "", errors.New("email not verified")
	}
	return claims.Email, nil
}

func (g *googleAuthenticator) IsStub() bool { return false }
