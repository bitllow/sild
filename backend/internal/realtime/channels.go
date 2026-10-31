package realtime

import (
	"context"

	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// ChannelInputs is what an agent's channel set reads besides the principal.
type ChannelInputs struct {
	Queues        []string                  // every queue the tenant defines
	Conversations []store.ActorConversation // the member's active conversations
}

// LoadChannelInputs reads what AgentChannels needs for any of ps, all of them
// the same member.
func LoadChannelInputs(ctx context.Context, st store.Store, tenantID, adminID string, ps ...*principal.Principal) (ChannelInputs, error) {
	var in ChannelInputs
	var allQueues, requester bool
	for _, p := range ps {
		_, all := p.Queues()
		allQueues = allQueues || all
		requester = requester || p.HasRole(models.PlatformRequester)
	}
	if allQueues {
		defined, err := st.Queues().List(ctx, tenantID)
		if err != nil {
			return in, err
		}
		for _, q := range defined {
			in.Queues = append(in.Queues, q.Key)
		}
	}
	if requester {
		convs, err := st.Members().ListActiveConversationsForActor(ctx, tenantID, adminID)
		if err != nil {
			return in, err
		}
		in.Conversations = convs
	}
	return in, nil
}

// AgentChannels is the server-side channel set of an inbox connection (§5.2),
// nil when policy admits nothing. It does not grow with new conversations.
func AgentChannels(p *principal.Principal, in ChannelInputs) map[string]bool {
	scope := policy.Scope(p, policy.ConversationsList)
	if scope.DenyAll() {
		return nil
	}
	tenantID := p.TenantID
	chans := map[string]bool{UserChannel(p.AdminID): true}
	// A requester follows their own conversations in the queues they still use,
	// like an end user; a requester-only member gets no operator channel.
	if p.HasRole(models.PlatformRequester) {
		for _, c := range in.Conversations {
			// An operator of the queue already gets the full events on its channel.
			if c.Kind != models.KindPeer && p.RequesterReaches(c.Queue) && !p.ReachesQueue(c.Queue) {
				chans[ConvChannel(c.ConversationID)] = true
			}
		}
		if p.RequesterOnly() {
			return chans
		}
	}
	if p.ReachesQueue(models.QueueSupport) {
		chans[AgentsChannel(tenantID)] = true
	}
	queues, all := p.Queues()
	if all {
		queues = in.Queues
	}
	for _, q := range queues {
		if q != models.QueueSupport {
			chans[QueueChannel(tenantID, q)] = true
		}
	}
	// Operators whose scope admits peer conversations observe the whole peer
	// surface on the single tenant peer channel.
	if scope.AllowsKind(models.KindPeer) {
		chans[PeerChannel(tenantID)] = true
	}
	return chans
}
