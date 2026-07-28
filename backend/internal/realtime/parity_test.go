package realtime

import (
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// REST/realtime parity. Not set equality: an operator subscribes to tenant
// channels, not to the conversations on them, so what must hold is that an event
// reaches an operator iff policy would let them read it.
//
// This file covers the subscribe side. The publish side — which conversations
// are put on the agents channel at all — is the other half of the same
// guarantee and is covered in domain/agent_fanout_test.go, because an
// unassigned support conversation must never be published there.
//
// operatorChannels mirrors agentSubscriptions off the same policy.Scope.
func operatorChannels(p *principal.Principal) map[string]bool {
	scope := policy.Scope(p, policy.ConversationsList)
	if scope.DenyAll() {
		return nil
	}
	subs := map[string]bool{
		UserChannel(p.AdminID):    true,
		AgentsChannel(p.TenantID): true,
	}
	if scope.AllowsKind(models.KindPeer) {
		subs[PeerChannel(p.TenantID)] = true
	}
	return subs
}

func operator(role models.PlatformRole, peer bool) *principal.Principal {
	return &principal.Principal{
		TenantID: "t1", Kind: principal.KindAdmin, AdminID: "a1",
		Role: role, PeerAccess: peer,
	}
}

// canRead asks policy the REST question for the same conversation.
func canRead(p *principal.Principal, kind models.ConversationKind, reachable bool) bool {
	return policy.Authorize(p, policy.ConversationsRead, policy.ResourceAttrs{
		Kind: kind, SupportReachable: reachable,
	}) == nil
}

func TestEventDeliveryMatchesPolicy(t *testing.T) {
	const tenant = "t1"

	// Where each representative event is published (see publisher.go).
	events := []struct {
		name    string
		channel string
		kind    models.ConversationKind
	}{
		{"support message", AgentsChannel(tenant), models.KindSupport},
		{"assignment update", AgentsChannel(tenant), models.KindSupport},
		{"internal note", AgentsChannel(tenant), models.KindSupport},
		{"peer message", PeerChannel(tenant), models.KindPeer},
	}

	operators := []struct {
		name string
		p    *principal.Principal
	}{
		{"agent without peer_access", operator(models.PlatformAgent, false)},
		{"agent with peer_access", operator(models.PlatformAgent, true)},
		{"owner without peer_access", operator(models.PlatformOwner, false)},
		{"owner with peer_access", operator(models.PlatformOwner, true)},
	}

	for _, op := range operators {
		subs := operatorChannels(op.p)
		for _, ev := range events {
			t.Run(op.name+"/"+ev.name, func(t *testing.T) {
				delivered := subs[ev.channel]
				// The assignment-bearing support conversation is reachable by any
				// operator; the peer conversation is gated on peer_access.
				readable := canRead(op.p, ev.kind, true)
				if delivered != readable {
					t.Fatalf("delivered=%v but policy readable=%v — socket and REST disagree",
						delivered, readable)
				}
			})
		}
	}
}

// Peer is the operator's own opt-in, whatever their platform role.
func TestPeerEventNeverReachesNonPeerOperator(t *testing.T) {
	for _, role := range []models.PlatformRole{models.PlatformAgent, models.PlatformAdmin, models.PlatformOwner} {
		subs := operatorChannels(operator(role, false))
		if subs[PeerChannel("t1")] {
			t.Fatalf("role %s without peer_access is subscribed to the peer channel", role)
		}
	}
}

// A peer_access revoke must stop delivery on a live socket, not at reconnect.
func TestPeerAccessReconciliation(t *testing.T) {
	fn := &fakeNode{}
	p := &CentrifugePublisher{node: fn}

	if err := p.Subscribe("a1", PeerChannel("t1")); err != nil {
		t.Fatal(err)
	}
	if len(fn.subscribed) != 1 || fn.subscribed[0] != "a1→peer:t1" {
		t.Fatalf("grant did not subscribe the live socket: %v", fn.subscribed)
	}

	if err := p.Unsubscribe("a1", PeerChannel("t1")); err != nil {
		t.Fatal(err)
	}
	if len(fn.unsubscribed) != 1 || fn.unsubscribed[0] != "a1→peer:t1" {
		t.Fatalf("revoke did not unsubscribe the live socket: %v", fn.unsubscribed)
	}
}

// The whole point of the tenant channels: an operator's subscription set is
// fixed, so a tenant with thousands of live assignments costs what an empty one
// costs and nothing has to be rebuilt when a conversation appears.
func TestChannelSetDoesNotGrowWithTheQueue(t *testing.T) {
	for _, peer := range []bool{false, true} {
		subs := operatorChannels(operator(models.PlatformAgent, peer))
		want := 2 // user + agents
		if peer {
			want++
		}
		if len(subs) != want {
			t.Fatalf("peer_access=%v subscribes to %d channels, want %d: %v", peer, len(subs), want, subs)
		}
		for ch := range subs {
			if strings.HasPrefix(ch, "conv:") {
				t.Fatalf("per-conversation subscription %q is back — the set now grows with the queue", ch)
			}
		}
	}
}

// Granting peer_access is the only thing that adds the peer channel.
func TestChannelSetFollowsScope(t *testing.T) {
	without := operatorChannels(operator(models.PlatformAgent, false))
	with := operatorChannels(operator(models.PlatformAgent, true))

	if without[PeerChannel("t1")] {
		t.Fatal("peer channel present without peer_access")
	}
	if !with[PeerChannel("t1")] {
		t.Fatal("peer channel missing with peer_access")
	}
	// Everything else is identical — the flag changes exactly one channel.
	if len(with) != len(without)+1 {
		t.Fatalf("peer_access changed more than the peer channel: %v vs %v", with, without)
	}
}
