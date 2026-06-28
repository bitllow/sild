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

// AuthorizeConversation ensures the caller may access a conversation (§4.2, §7).
// Scope by principal:
//   - API key:        tenant-wide (server backend)
//   - owner/admin:    tenant-wide (all conversations)
//   - agent:          support inbox only — conversation must carry an assignment
//                     (or be an archived, formerly-support conversation)
//   - user:           must be an (active or archived) member
//
// Writes a 403/401 and returns false on denial.
func AuthorizeConversation(c *gin.Context, svc *domain.Service, convID string) bool {
	p := middleware.Get(c)
	if p == nil {
		httpx.Unauthorized(c, "authentication required")
		return false
	}
	ctx, t := c.Request.Context(), p.TenantID

	switch p.Kind {
	case middleware.KindAPIKey:
		return true

	case middleware.KindAdmin:
		if p.Role == models.PlatformOwner || p.Role == models.PlatformAdmin {
			return true
		}
		// agent: limited to support conversations (§7).
		if svc.HasAssignment(ctx, t, convID) || svc.IsArchived(ctx, t, convID) {
			return true
		}
		httpx.Forbidden(c, "agents may only access support conversations")
		return false

	default: // user JWT
		if ok, err := svc.IsMember(ctx, t, convID, p.Subject); err == nil && ok {
			return true
		}
		if svc.IsArchivedMember(ctx, t, convID, p.Subject) {
			return true
		}
		httpx.Forbidden(c, "not a member of this conversation")
		return false
	}
}
