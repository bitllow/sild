// Package apiutil holds helpers shared by the audience-specific handler packages
// (internal/api/*): principal inspection and conversation authorization.
//
// Authorization decisions live in internal/policy. This package only gathers the
// resource attributes a decision needs and translates a refusal into HTTP.
package apiutil

import (
	"errors"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
)

// IsAgent reports whether the caller is privileged (API key or admin) — i.e. may
// see internal notes and set visibility=internal (§4.2, §5.6). Users are not.
func IsAgent(c *gin.Context) bool {
	p := middleware.Get(c)
	return p != nil && p.Kind != principal.KindUser
}

// Subject returns the user-JWT subject ("" for non-user principals).
func Subject(c *gin.Context) string {
	if p := middleware.Get(c); p != nil {
		return p.Subject
	}
	return ""
}

// Tenant returns the resolved tenant id.
func Tenant(c *gin.Context) string { return middleware.TenantID(c) }

// Scope returns the policy ceiling for a collection query.
func Scope(c *gin.Context, a policy.Action) policy.ResourceScope {
	return policy.Scope(middleware.Get(c), a)
}

// CallerParticipant maps the principal to a store.Participant (sender/owner).
func CallerParticipant(c *gin.Context) store.Participant {
	p := middleware.Get(c)
	if p == nil {
		return store.Participant{}
	}
	switch p.Kind {
	case principal.KindUser:
		uid := p.Subject
		return store.Participant{Kind: models.MemberUser, ExternalUserID: &uid}
	case principal.KindAdmin:
		aid := p.AdminID
		return store.Participant{Kind: models.MemberAgent, InternalActorID: &aid}
	default:
		return store.Participant{Kind: models.MemberAgent}
	}
}

// Authorize checks a non-resource action and writes a 401/403 on refusal.
func Authorize(c *gin.Context, a policy.Action) bool {
	if err := policy.Authorize(middleware.Get(c), a, policy.ResourceAttrs{}); err != nil {
		failPolicy(c, err)
		return false
	}
	return true
}

// AuthorizeConversation checks an action against one conversation (§4.2, §7).
// It loads the attributes policy needs — kind, archived, assignment existence,
// membership — and writes a 401/403 on refusal.
func AuthorizeConversation(c *gin.Context, svc *domain.Service, a policy.Action, convID string) bool {
	p := middleware.Get(c)
	if p == nil {
		httpx.Unauthorized(c, "authentication required")
		return false
	}
	if err := policy.Authorize(p, a, conversationAttrs(c, svc, p, convID)); err != nil {
		failPolicy(c, err)
		return false
	}
	return true
}

// conversationAttrs loads only what the principal's branch actually needs, so a
// tenant-wide owner/admin doesn't pay for an assignment lookup and a user
// doesn't pay for a kind lookup.
func conversationAttrs(c *gin.Context, svc *domain.Service, p *principal.Principal, convID string) policy.ResourceAttrs {
	ctx, tenant := c.Request.Context(), p.TenantID
	switch p.Kind {
	case principal.KindAPIKey:
		return policy.ResourceAttrs{}

	case principal.KindAdmin:
		access := svc.ClassifyAgentAccess(ctx, tenant, convID, !p.Privileged())
		attrs := policy.ResourceAttrs{SupportReachable: access.SupportOK}
		if access.Peer {
			attrs.Kind = models.KindPeer
		} else {
			attrs.Kind = models.KindSupport
		}
		return attrs

	default: // user JWT
		member := false
		if ok, err := svc.IsMember(ctx, tenant, convID, p.Subject); err == nil && ok {
			member = true
		} else if svc.IsArchivedMember(ctx, tenant, convID, p.Subject) {
			member = true
		}
		return policy.ResourceAttrs{IsMember: member}
	}
}

func failPolicy(c *gin.Context, err error) {
	var d *policy.Denied
	if errors.As(err, &d) {
		if d.Unauthenticated() {
			httpx.Error(c, 401, d.Code, d.Message)
			return
		}
		httpx.Error(c, 403, d.Code, d.Message)
		return
	}
	httpx.Forbidden(c, "forbidden")
}
