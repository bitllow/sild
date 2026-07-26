package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
	"github.com/gin-gonic/gin"
)

// Multi-action routes resolve their action from the request, so the manifest
// cannot see the pick. Mount publishes the declared set and the authorization
// helpers check it — these prove the check binds in both directions.

// Both arms of each multi-action route must land on a declared action. An
// undeclared one answers 500, so a satisfied request proves the pick was legal.
func TestMultiActionRoutesResolveWithinTheirDeclaredSet(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	jwt := h.MintToken(tenant.ID, "u_probe")
	h.SeedAdmin(tenant.ID, "owner@probe", models.PlatformOwner)
	owner := loginAs(t, h, "owner@probe")

	// POST /v1/conversations declares {create_support, create_peer}, picked by
	// open_assignment. Drive both arms.
	support := h.Request("POST", "/v1/conversations").Bearer(jwt).JSON(map[string]any{}).Do()
	if support.Code != http.StatusCreated {
		t.Fatalf("support arm: %d %s", support.Code, support.Body)
	}
	peer := h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"open_assignment": false,
		"members":         []map[string]any{{"user_id": "u_a"}, {"user_id": "u_b"}},
	}).Do()
	if peer.Code != http.StatusCreated {
		t.Fatalf("peer arm: %d %s", peer.Code, peer.Body)
	}

	// PATCH /v1/assignments/:id declares {claim, close}, picked by the body.
	var conv struct {
		ID         string `json:"id"`
		Assignment *struct {
			ID string `json:"id"`
		} `json:"assignment"`
	}
	testutil.DecodeJSON(t, support, &conv)
	if conv.Assignment == nil {
		t.Fatal("support conversation carries no assignment")
	}
	claim := h.Request("PATCH", "/v1/assignments/"+conv.Assignment.ID).
		Cookie("sild_admin", owner).JSON(map[string]any{"assignee_actor_id": "me"}).Do()
	if claim.Code != http.StatusOK {
		t.Fatalf("claim arm: %d %s", claim.Code, claim.Body)
	}
	closed := h.Request("PATCH", "/v1/assignments/"+conv.Assignment.ID).
		Cookie("sild_admin", owner).JSON(map[string]any{"status": "closed"}).Do()
	if closed.Code != http.StatusOK {
		t.Fatalf("close arm: %d %s", closed.Code, closed.Body)
	}
}

// The negative direction: a handler naming an action outside the route's declared
// set is refused, not quietly allowed. Without this the declaration is a comment.
func TestUndeclaredActionIsRefused(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name    string
		declare []policy.Action
		attempt policy.Action
		wantOK  bool
	}{
		{"declared", []policy.Action{policy.ConversationsCreateSupport, policy.ConversationsCreatePeer},
			policy.ConversationsCreatePeer, true},
		{"outside the set", []policy.Action{policy.ConversationsCreateSupport},
			policy.ConversationsCreatePeer, false},
		{"unrelated action", []policy.Action{policy.ContactsList}, policy.TeamManage, false},
		// A route declaring nothing makes no policy decision to constrain.
		{"no declaration", nil, policy.TeamManage, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := gin.New()
			var authorized bool
			e.GET("/probe", apiutil.DeclareActions(tc.declare), func(c *gin.Context) {
				// No principal, so Authorize refuses on policy grounds too — the
				// distinction under test is 401/403 (reached policy) vs 500 (refused
				// before it).
				authorized = apiutil.Authorize(c, tc.attempt)
			})

			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest("GET", "/probe", nil))
			if authorized {
				t.Fatalf("unauthenticated probe was authorized")
			}
			reachedPolicy := w.Code != http.StatusInternalServerError
			if reachedPolicy != tc.wantOK {
				t.Errorf("declaring %v then attempting %q: status %d (body %s); want reachedPolicy=%v",
					tc.declare, tc.attempt, w.Code, w.Body, tc.wantOK)
			}
		})
	}
}
