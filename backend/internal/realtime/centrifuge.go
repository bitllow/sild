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

// channelsFor computes the destination channels for a target. Internal notes go
// ONLY to the agents-only channel — clients are never subscribed there, so the
// privacy boundary is a subscription fact, not UI logic (§5.6).
func channelsFor(t Target) []string {
	var channels []string
	if t.Conversation != "" {
		if t.Internal {
			channels = append(channels, ConvInternalChannel(t.Conversation))
		} else {
			channels = append(channels, ConvChannel(t.Conversation))
		}
	}
	for _, u := range t.Users {
		channels = append(channels, UserChannel(u))
	}
	return channels
}
