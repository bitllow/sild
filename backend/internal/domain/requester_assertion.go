package domain

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"gorm.io/datatypes"

	"github.com/bitllow/sild/backend/internal/store/models"
)

const (
	requesterAssertionTTL        = 5 * time.Minute
	maxAssertionsPerConversation = 50
	maxAdapterRequestBytes       = 64 << 10
)

var adapterRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// adapterRequest is the part of a data-adapter request Sild vouches for.
type adapterRequest struct {
	ID        string `json:"id"`
	Requester struct {
		ID string `json:"id"`
	} `json:"requester"`
	ConversationRef string          `json:"conversation_ref"`
	Source          string          `json:"source"`
	Params          json.RawMessage `json:"params"`
}

// MintRequesterAssertion signs, for a data adapter, who asks one request: only
// for the assistant holding the open conversation, and for its own requester.
func (s *Service) MintRequesterAssertion(ctx context.Context, tenantID, convID, actorID string, body []byte) (string, time.Time, error) {
	if len(body) == 0 || len(body) > maxAdapterRequestBytes {
		return "", time.Time{}, invalid("body must be the adapter request, at most 64 KiB")
	}
	var req adapterRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return "", time.Time{}, invalid("body is not an adapter request")
	}
	if !adapterRequestID.MatchString(req.ID) || req.Source == "" || req.ConversationRef != convID {
		return "", time.Time{}, invalid("body needs an id, a source, and this conversation as conversation_ref")
	}

	conv, members, assignment, err := s.GetConversation(ctx, tenantID, convID)
	if err != nil {
		return "", time.Time{}, err
	}
	if conv.Status == models.ConversationClosed {
		return "", time.Time{}, conflict(CodeConversationClosed, "the conversation is closed")
	}
	if assignment == nil || assignment.Status != models.AssignmentAssigned ||
		assignment.AssigneeActorID == nil || *assignment.AssigneeActorID != actorID {
		return "", time.Time{}, ErrForbidden
	}
	sub, err := s.requesterSub(ctx, tenantID, members)
	if err != nil {
		return "", time.Time{}, err
	}
	if sub == "" {
		return "", time.Time{}, conflict(CodeRequesterNotLinked, "the requester has no integrator identity")
	}
	if req.Requester.ID != sub {
		return "", time.Time{}, invalid("requester.id is not this conversation's requester")
	}

	repo := s.store.RequesterAssertions()
	n, err := repo.CountByConversation(ctx, tenantID, convID)
	if err != nil {
		return "", time.Time{}, err
	}
	if n >= maxAssertionsPerConversation {
		return "", time.Time{}, conflict(CodeRequestLimit, "this conversation has made all the data requests it may")
	}
	if err := repo.Create(ctx, &models.RequesterAssertion{
		TenantID: tenantID, ConversationID: convID, ActorID: actorID, RequesterSub: sub,
		RequestID: req.ID, Source: req.Source, Params: datatypes.JSON(req.Params), CreatedAt: s.now(),
	}); err != nil {
		return "", time.Time{}, err
	}
	sum := sha256.Sum256(body)
	return s.keys.MintRequester(ctx, tenantID, sub, convID, req.ID, base64.RawURLEncoding.EncodeToString(sum[:]), requesterAssertionTTL)
}

// requesterActor is the team member who asks in a conversation, or nil.
func requesterActor(members []models.ConversationMember) *string {
	for _, m := range members {
		if m.ConvRole == models.RoleClient && m.InternalActorID != nil {
			return m.InternalActorID
		}
	}
	return nil
}

// requesterSub is the requester's integrator identity, or "" when they are not linked.
func (s *Service) requesterSub(ctx context.Context, tenantID string, members []models.ConversationMember) (string, error) {
	actor := requesterActor(members)
	if actor == nil {
		return "", nil
	}
	admin, err := s.store.Admins().Get(ctx, tenantID, *actor)
	if err != nil {
		return "", mapStoreErr(err)
	}
	if admin.ExternalSub == nil {
		return "", nil
	}
	return *admin.ExternalSub, nil
}

// ApproveDataAccess records, as a system message from the requester, that they
// approved data access at the integrator; the assistant retries what waited for it.
func (s *Service) ApproveDataAccess(ctx context.Context, tenantID, convID, actorID string) (*models.Message, error) {
	conv, members, _, err := s.GetConversation(ctx, tenantID, convID)
	if err != nil {
		return nil, err
	}
	if conv.Status == models.ConversationClosed {
		return nil, conflict(CodeConversationClosed, "the conversation is closed")
	}
	if r := requesterActor(members); r == nil || *r != actorID {
		return nil, ErrForbidden
	}
	items, err := s.awaitingApproval(ctx, tenantID, convID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, conflict(CodeNothingToApprove, "nothing waits for approval")
	}
	parts, err := json.Marshal([]messagePart{{Type: "approval", Items: items}})
	if err != nil {
		return nil, err
	}
	body := "Approved data access"
	if len(items) > 0 {
		body += ": " + strings.Join(items, "; ")
	}
	return s.SendMessage(ctx, tenantID, convID, SendInput{
		SenderKind: models.SenderSystem, Internal: &actorID, Conv: conv, Body: body, Parts: parts,
	})
}

// awaitingApproval names the assistant's steps that wait for the requester's approval, as the steps label them.
func (s *Service) awaitingApproval(ctx context.Context, tenantID, convID string) ([]string, error) {
	page, err := s.store.Messages().ListBefore(ctx, tenantID, convID, "", 100, false)
	if err != nil {
		return nil, err
	}
	var items []string
	seen := map[string]bool{}
	for _, m := range page.Messages {
		var parts []messagePart
		if m.SenderKind != models.SenderBot || json.Unmarshal(m.Parts, &parts) != nil {
			continue
		}
		for _, p := range parts {
			for _, st := range p.Steps {
				if st.State == "needs_approval" && !seen[st.Label] && len(items) < maxApproved {
					seen[st.Label] = true
					items = append(items, st.Label)
				}
			}
		}
	}
	return items, nil
}
