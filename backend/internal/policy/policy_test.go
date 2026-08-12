package policy

import (
	"testing"

	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
)

func apiKey() *principal.Principal {
	return &principal.Principal{TenantID: "t1", Kind: principal.KindAPIKey}
}
func user(sub string) *principal.Principal {
	return &principal.Principal{TenantID: "t1", Kind: principal.KindUser, Subject: sub}
}
func agent(peer bool) *principal.Principal {
	return &principal.Principal{TenantID: "t1", Kind: principal.KindAdmin, AdminID: "a1",
		Assignments: []principal.Assignment{{Role: models.PlatformAgent, Scope: models.RoleScope{Peer: peer}}}}
}
func admin() *principal.Principal {
	return &principal.Principal{TenantID: "t1", Kind: principal.KindAdmin, AdminID: "a3",
		Assignments: principal.Held(models.PlatformAdmin)}
}

// An owner reaching peer conversations holds the agent role for it: peer is that
// role's dimension, and no other role implies it.
func owner(peer bool) *principal.Principal {
	held := principal.Held(models.PlatformOwner)
	if peer {
		held = append(held, principal.Assignment{Role: models.PlatformAgent, Scope: models.RoleScope{Peer: true}})
	}
	return &principal.Principal{TenantID: "t1", Kind: principal.KindAdmin, AdminID: "a2", Assignments: held}
}

// Conversation access across every principal × resource state.
func TestAuthorizeConversation(t *testing.T) {
	support := func(reachable bool) ResourceAttrs {
		return ResourceAttrs{Kind: models.KindSupport, SupportReachable: reachable}
	}
	peer := ResourceAttrs{Kind: models.KindPeer}

	cases := []struct {
		name  string
		p     *principal.Principal
		a     Action
		r     ResourceAttrs
		allow bool
	}{
		{"apikey reads any support", apiKey(), ConversationsRead, support(false), true},
		{"apikey reads peer", apiKey(), ConversationsRead, peer, true},

		{"owner reads support", owner(false), ConversationsRead, support(false), true},
		{"owner without peer_access denied peer", owner(false), ConversationsRead, peer, false},
		{"owner with peer_access reads peer", owner(true), ConversationsRead, peer, true},

		// "reachable" means an assignment EXISTS — anyone's.
		{"agent reads assignment-bearing support", agent(false), ConversationsRead, support(true), true},
		{"agent denied assignment-less support", agent(false), ConversationsRead, support(false), false},
		{"agent without peer_access denied peer", agent(false), ConversationsRead, peer, false},
		{"agent with peer_access reads peer", agent(true), ConversationsRead, peer, true},

		{"member user reads", user("u1"), ConversationsRead, ResourceAttrs{IsMember: true}, true},
		{"non-member user denied", user("u1"), ConversationsRead, ResourceAttrs{}, false},

		{"user may send", user("u1"), MessagesSend, ResourceAttrs{IsMember: true}, true},
		{"user may not close", user("u1"), ConversationsClose, ResourceAttrs{IsMember: true}, false},
		{"agent may close reachable support", agent(false), ConversationsClose, support(true), true},

		{"nil principal denied", nil, ConversationsRead, support(true), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Authorize(tc.p, tc.a, tc.r)
			if tc.allow && err != nil {
				t.Fatalf("expected allow, got %v", err)
			}
			if !tc.allow && err == nil {
				t.Fatal("expected denial, got allow")
			}
		})
	}
}

// Capability checks that carry no resource dimension.
func TestAuthorizeCapability(t *testing.T) {
	cases := []struct {
		name  string
		p     *principal.Principal
		a     Action
		allow bool
	}{
		{"apikey mints tokens", apiKey(), TokensMint, true},
		{"user cannot mint tokens", user("u1"), TokensMint, false},
		{"agent cannot mint tokens", agent(false), TokensMint, false},

		// Server-to-server only: a peer conversation is visible to every
		// peer_access operator.
		{"apikey creates peer", apiKey(), ConversationsCreatePeer, true},
		{"user cannot create peer", user("u1"), ConversationsCreatePeer, false},
		{"owner cannot create peer", owner(true), ConversationsCreatePeer, false},
		{"user creates support", user("u1"), ConversationsCreateSupport, true},

		{"owner manages team", owner(false), TeamManage, true},
		{"agent cannot manage team", agent(false), TeamManage, false},
		{"apikey cannot manage team", apiKey(), TeamManage, false},

		{"user cannot claim", user("u1"), AssignmentsClaim, false},
		{"agent cannot manage members", agent(false), MembersManage, false},

		{"user manages push tokens", user("u1"), PushTokensManage, true},
		{"agent cannot manage push tokens", agent(false), PushTokensManage, false},

		{"owner manages push config", owner(false), PushConfigManage, true},
		{"admin cannot manage push config", admin(), PushConfigManage, false},
		{"agent cannot manage push config", agent(false), PushConfigManage, false},
		{"apikey cannot manage push config", apiKey(), PushConfigManage, false},

		{"any principal reads own identity", user("u1"), PrincipalRead, true},
		{"any principal reads active brand", user("u1"), BrandsReadActive, true},
		{"agent cannot read brand set", agent(false), BrandsRead, false},
		{"owner reads brand set", owner(false), BrandsRead, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Authorize(tc.p, tc.a, ResourceAttrs{})
			if tc.allow != (err == nil) {
				t.Fatalf("allow=%v, err=%v", tc.allow, err)
			}
		})
	}
}

// An undeclared action must deny rather than default-allow.
func TestUndeclaredActionDenies(t *testing.T) {
	if err := Authorize(owner(true), Action("does.not.exist"), ResourceAttrs{}); err == nil {
		t.Fatal("undeclared action was allowed")
	}
}

func TestScope(t *testing.T) {
	t.Run("apikey is unrestricted", func(t *testing.T) {
		s := Scope(apiKey(), ConversationsList)
		if len(s.AllowedKinds()) != 0 || s.RequiresAssignment() {
			t.Fatalf("expected unrestricted, got %+v", s)
		}
		if _, ok := s.Participant(); ok {
			t.Fatal("apikey scope should not pin a participant")
		}
	})

	t.Run("user is pinned to own subject", func(t *testing.T) {
		s := Scope(user("u1"), ConversationsList)
		got, ok := s.Participant()
		if !ok || got != "u1" {
			t.Fatalf("participant=%q ok=%v", got, ok)
		}
	})

	t.Run("agent without peer_access sees support only", func(t *testing.T) {
		s := Scope(agent(false), ConversationsList)
		if s.AllowsKind(models.KindPeer) {
			t.Fatal("peer kind leaked into a non-peer agent scope")
		}
		if !s.RequiresAssignment() {
			t.Fatal("agent scope must require an assignment")
		}
	})

	t.Run("agent with peer_access sees both kinds", func(t *testing.T) {
		s := Scope(agent(true), ConversationsList)
		if !s.AllowsKind(models.KindPeer) || !s.AllowsKind(models.KindSupport) {
			t.Fatalf("expected both kinds, got %v", s.AllowedKinds())
		}
	})

	t.Run("owner is tenant-wide but peer still gated", func(t *testing.T) {
		if Scope(owner(false), ConversationsList).AllowsKind(models.KindPeer) {
			t.Fatal("owner without peer_access must not see peer")
		}
		if Scope(owner(false), ConversationsList).RequiresAssignment() {
			t.Fatal("owner must not be assignment-gated")
		}
	})

	// An empty kinds list means "unrestricted", so no-capability needs a flag.
	t.Run("no capability denies all", func(t *testing.T) {
		s := Scope(user("u1"), TeamManage)
		if !s.DenyAll() {
			t.Fatal("scope without capability must set DenyAll")
		}
		if s.AllowsKind(models.KindSupport) || s.AllowsKind(models.KindPeer) {
			t.Fatal("scope without capability must admit nothing")
		}
	})
}

// A scope handed to a repo must not be widenable through its getters.
func TestScopeGettersReturnCopies(t *testing.T) {
	s := Scope(agent(false), ConversationsList)
	kinds := s.AllowedKinds()
	kinds[0] = models.KindPeer
	if s.AllowsKind(models.KindPeer) {
		t.Fatal("mutating the returned slice widened the scope")
	}
}

// Grants drive frontend affordances, so peer visibility lives in the scope.
func TestGrantsCarryScope(t *testing.T) {
	find := func(gs []Grant, a Action) *Grant {
		for i := range gs {
			if gs[i].Action == a {
				return &gs[i]
			}
		}
		return nil
	}

	withPeer := find(Grants(agent(true)), ConversationsList)
	if withPeer == nil || withPeer.Scope == nil {
		t.Fatal("conversations.list grant missing its scope")
	}
	if len(withPeer.Scope.Kinds) != 0 {
		t.Fatalf("peer_access agent should be unrestricted by kind, got %v", withPeer.Scope.Kinds)
	}

	without := find(Grants(agent(false)), ConversationsList)
	if without == nil || without.Scope == nil {
		t.Fatal("conversations.list grant missing its scope")
	}
	if len(without.Scope.Kinds) != 1 || without.Scope.Kinds[0] != models.KindSupport {
		t.Fatalf("expected support-only kinds, got %v", without.Scope.Kinds)
	}

	if find(Grants(user("u1")), TeamManage) != nil {
		t.Fatal("user was granted team.manage")
	}
}
