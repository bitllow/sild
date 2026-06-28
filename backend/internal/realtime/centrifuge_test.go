package realtime

import (
	"context"
	"testing"

	"github.com/centrifugal/centrifuge"
)

type fakeNode struct{ published []string }

func (f *fakeNode) Publish(channel string, _ []byte, _ ...centrifuge.PublishOption) (centrifuge.PublishResult, error) {
	f.published = append(f.published, channel)
	return centrifuge.PublishResult{}, nil
}

// §5.1 channel split: a participants message → conv channel + user channels.
func TestPublishParticipantsChannels(t *testing.T) {
	fn := &fakeNode{}
	p := &CentrifugePublisher{node: fn}
	_ = p.Publish(context.Background(), Target{Conversation: "c1", Users: []string{"u1", "u2"}}, Envelope{Type: "message.created"})
	want := map[string]bool{"conv:c1": true, "user:u1": true, "user:u2": true}
	if len(fn.published) != 3 {
		t.Fatalf("expected 3 channels, got %v", fn.published)
	}
	for _, ch := range fn.published {
		if !want[ch] {
			t.Errorf("unexpected channel %q", ch)
		}
	}
}

// §5.6 internal notes go ONLY to the agents-only channel — never the conv
// channel clients subscribe to.
func TestPublishInternalChannelOnly(t *testing.T) {
	fn := &fakeNode{}
	p := &CentrifugePublisher{node: fn}
	_ = p.Publish(context.Background(), Target{Conversation: "c1", Internal: true}, Envelope{Type: "message.created"})
	if len(fn.published) != 1 || fn.published[0] != "conv:c1:internal" {
		t.Fatalf("internal note must publish only to conv:c1:internal, got %v", fn.published)
	}
}
