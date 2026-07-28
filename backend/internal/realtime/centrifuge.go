package realtime

import (
	"context"
	"encoding/json"

	"github.com/centrifugal/centrifuge"
)

// nodePublisher is the slice of *centrifuge.Node the publisher needs (lets tests
// substitute a fake).
type nodePublisher interface {
	Publish(channel string, data []byte, opts ...centrifuge.PublishOption) (centrifuge.PublishResult, error)
	Subscribe(userID, channel string, opts ...centrifuge.SubscribeOption) error
	Unsubscribe(userID, channel string, opts ...centrifuge.UnsubscribeOption) error
}

// CentrifugePublisher routes envelopes to Centrifuge channels per the channel
// split (§5.1): conv / conv:internal / user. With the Redis broker this fans out
// cluster-wide to whichever sild-ws node holds each connection.
type CentrifugePublisher struct {
	node nodePublisher
}

// NewCentrifugePublisher wraps a node. dig provides realtime.Publisher from this
// in the sild-api role.
func NewCentrifugePublisher(node *centrifuge.Node) *CentrifugePublisher {
	return &CentrifugePublisher{node: node}
}

// Publish marshals the envelope and publishes to every target channel.
func (p *CentrifugePublisher) Publish(_ context.Context, t Target, env Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	for _, ch := range channelsFor(t) {
		if _, err := p.node.Publish(ch, data); err != nil {
			return err
		}
	}
	return nil
}

// Subscribe adds a live server-side subscription for a connected user (§5.2), so
// a newly-granted access takes effect without a reconnect. Propagates cluster-wide
// through the broker.
func (p *CentrifugePublisher) Subscribe(userID, channel string) error {
	return p.node.Subscribe(userID, channel)
}

// Unsubscribe removes a live server-side subscription for a connected user, so a
// revoked access stops delivery immediately.
func (p *CentrifugePublisher) Unsubscribe(userID, channel string) error {
	return p.node.Unsubscribe(userID, channel)
}

// channelsFor computes the destination channels for a target. An internal note
// reaches no client channel at all — only the tenant channels operators watch, so
// the privacy boundary is a subscription fact rather than UI logic (§5.6).
func channelsFor(t Target) []string {
	var channels []string
	if !t.Internal {
		if t.Conversation != "" {
			channels = append(channels, ConvChannel(t.Conversation))
		}
		for _, u := range t.Users {
			channels = append(channels, UserChannel(u))
		}
	}
	if t.Tenant != "" {
		channels = append(channels, AgentsChannel(t.Tenant))
	}
	if t.Peer != "" {
		channels = append(channels, PeerChannel(t.Peer))
	}
	return channels
}
