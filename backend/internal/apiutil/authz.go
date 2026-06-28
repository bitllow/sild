// Package apiutil holds helpers shared by the audience-specific handler packages
// (internal/api/*): principal inspection and conversation authorization.
package apiutil

import (
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
)

// IsAgent reports whether the caller is privileged (API key or admin) — i.e. may
// see internal notes and set visibility=internal (§4.2, §5.6). Users are not.
func IsAgent(c *gin.Context) bool {
	p := middleware.Get(c)
	return p != nil && p.Kind != middleware.KindUser
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

// CallerParticipant maps the principal to a store.Participant (sender/owner).
func CallerParticipant(c *gin.Context) store.Participant {
	p := middleware.Get(c)
	if p == nil {
		return store.Participant{}
	}
	switch p.Kind {
	case middleware.KindUser:
		uid := p.Subject
		return store.Participant{Kind: models.MemberUser, ExternalUserID: &uid}
	case middleware.KindAdmin:
		aid := p.AdminID
		return store.Participant{Kind: models.MemberAgent, InternalActorID: &aid}
	default:
		return store.Participant{Kind: models.MemberAgent}
	}
}

// AuthorizeConversation ensures the caller may access a conversation. For user
// principals this is the membership check (§4.2, §7); agents/keys have tenant
// access. Writes a 403 and returns false on denial.
func AuthorizeConversation(c *gin.Context, svc *domain.Service, convID string) bool {
	p := middleware.Get(c)
	if p == nil {
		httpx.Unauthorized(c, "authentication required")
		return false
	}
	if p.Kind == middleware.KindUser {
		ok, err := svc.IsMember(c.Request.Context(), p.TenantID, convID, p.Subject)
		if err != nil || !ok {
			httpx.Forbidden(c, "not a member of this conversation")
			return false
		}
	}
	return true
}
