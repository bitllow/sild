package realtime

import (
	"context"
	"testing"

	"github.com/centrifugal/centrifuge"
)

type fakeNode struct {
	published    []string
	subscribed   []string
	unsubscribed []string
}

func (f *fakeNode) Publish(channel string, _ []byte, _ ...centrifuge.PublishOption) (centrifuge.PublishResult, error) {
	f.published = append(f.published, channel)
	return centrifuge.PublishResult{}, nil
}

func (f *fakeNode) Subscribe(userID, channel string, _ ...centrifuge.SubscribeOption) error {
	f.subscribed = append(f.subscribed, userID+"→"+channel)
	return nil
}

func (f *fakeNode) Unsubscribe(userID, channel string, _ ...centrifuge.UnsubscribeOption) error {
	f.unsubscribed = append(f.unsubscribed, userID+"→"+channel)
	return nil
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

// §5.6 internal notes reach agent channels only — never the conv channel clients
// subscribe to, and never a user channel.
func TestPublishInternalChannelOnly(t *testing.T) {
	fn := &fakeNode{}
	p := &CentrifugePublisher{node: fn}
	_ = p.Publish(context.Background(),
		Target{Conversation: "c1", Internal: true, Tenant: "t1"},
		Envelope{Type: "message.created"})
	if len(fn.published) != 1 || fn.published[0] != "agents:t1" {
		t.Fatalf("internal note must publish only to agents:t1, got %v", fn.published)
	}
}

// An internal note in a conversation no operator may observe yet reaches nobody
// live rather than falling back to a channel clients can hear. History is the
// catch-up path (§5.4).
func TestPublishInternalWithoutObserversGoesNowhere(t *testing.T) {
	fn := &fakeNode{}
	p := &CentrifugePublisher{node: fn}
	_ = p.Publish(context.Background(), Target{Conversation: "c1", Internal: true}, Envelope{Type: "message.created"})
	if len(fn.published) != 0 {
		t.Fatalf("internal note leaked to %v", fn.published)
	}
}
