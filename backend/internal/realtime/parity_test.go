package realtime

import (
	"testing"

	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// REST/realtime parity. Not a set-equality check: the channel set mixes
// granularities — agents:<tenant> and peer:<tenant> are tenant-wide while
// conv:<id> is per-conversation — so there is no conversation-id set to compare
// a scope against. What must hold is that an event reaches an operator iff
// policy would let them read it.
//
// operatorChannels mirrors what agentSubscriptions builds, driven by the same
// policy.Scope, so a divergence between the two shows up here.
func operatorChannels(p *principal.Principal, assignedConvIDs []string) map[string]bool {
	scope := policy.Scope(p, policy.ConversationsList)
	if scope.DenyAll() {
		return nil
	}
	subs := map[string]bool{
		UserChannel(p.AdminID):    true,
		AgentsChannel(p.TenantID): true,
	}
	for _, cid := range assignedConvIDs {
		subs[ConvChannel(cid)] = true
		subs[ConvInternalChannel(cid)] = true
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
	const tenant, assignedConv, peerConv = "t1", "c_assigned", "c_peer"

	// Where each representative event is published (see publisher.go).
	events := []struct {
		name    string
		channel string
		kind    models.ConversationKind
	}{
		{"support message", ConvChannel(assignedConv), models.KindSupport},
		{"assignment update", AgentsChannel(tenant), models.KindSupport},
		{"internal note", ConvInternalChannel(assignedConv), models.KindSupport},
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
		subs := operatorChannels(op.p, []string{assignedConv})
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

// A peer event must never reach an operator without peer_access, whatever their
// platform role. Owner/admin are tenant-wide for support but peer is their own
// opt-in.
func TestPeerEventNeverReachesNonPeerOperator(t *testing.T) {
	for _, role := range []models.PlatformRole{models.PlatformAgent, models.PlatformAdmin, models.PlatformOwner} {
		subs := operatorChannels(operator(role, false), nil)
		if subs[PeerChannel("t1")] {
			t.Fatalf("role %s without peer_access is subscribed to the peer channel", role)
		}
	}
}

// peer_access changes take effect on a live socket, not only at reconnect — a
// revoke must stop delivery immediately.
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

// The channel set must follow the scope, so granting peer_access is the only
// thing that adds the peer channel.
func TestChannelSetFollowsScope(t *testing.T) {
	without := operatorChannels(operator(models.PlatformAgent, false), nil)
	with := operatorChannels(operator(models.PlatformAgent, true), nil)

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
