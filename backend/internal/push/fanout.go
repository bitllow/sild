package push

import (
	"context"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// FanOut delivers a message nudge to conversation members who are offline and
// are not the sender (§5.5). Run by sild-worker against Centrifuge presence.
type FanOut struct {
	store    store.Store
	notifier Notifier
	presence PresenceChecker
}

// NewFanOut constructs the fan-out. dig provides it for sild-worker.
func NewFanOut(st store.Store, n Notifier, p PresenceChecker) *FanOut {
	return &FanOut{store: st, notifier: n, presence: p}
}

// Deliver notifies offline user members (excluding senderExternalID) of a new
// message. Connected clients already got the realtime event — no double-notify.
func (f *FanOut) Deliver(ctx context.Context, tenantID, convID, senderExternalID string, p Payload) error {
	members, err := f.store.Members().ListActive(ctx, tenantID, convID)
	if err != nil {
		return err
	}
	for _, m := range members {
		if m.MemberKind != models.MemberUser || m.ExternalUserID == nil {
			continue // email/agent/bot are not push targets here
		}
		uid := *m.ExternalUserID
		if uid == senderExternalID {
			continue
		}
		online, err := f.presence.Online(ctx, tenantID, uid)
		if err == nil && online {
			continue // has a live connection — skip
		}
		tokens, err := f.store.PushTokens().ListForUser(ctx, tenantID, uid)
		if err != nil || len(tokens) == 0 {
			continue
		}
		targets := make([]Target, 0, len(tokens))
		for _, t := range tokens {
			targets = append(targets, Target{Platform: string(t.Platform), Token: t.Token})
		}
		_ = f.notifier.Notify(ctx, targets, p)
	}
	return nil
}
