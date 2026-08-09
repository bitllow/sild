package push

import (
	"context"
	"errors"
	"time"

	"github.com/bitllow/sild/backend/internal/secrets"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// backoffSchedule is the retry delay (seconds) per attempt: a nudge about a
// message nobody has seen for ten minutes has already failed at its job.
var backoffSchedule = []int{30, 60, 180}

// renewAfter is when a running batch starts re-locking its rows.
const renewAfter = store.ClaimTTL / 2

// errClaimLost means the row is no longer ours — another worker took it, or the
// lock could not be renewed. Neither settling nor rescheduling is ours to do:
// the row belongs to whoever holds it, and a lapsed lock frees it anyway.
var errClaimLost = errors.New("claim lost")

// FanOut drains the nudge queue and delivers through each tenant's own push
// project.
type FanOut struct {
	store    store.Store
	notifier Notifier
	box      *secrets.Box
}

// NewFanOut constructs the fan-out. dig provides it for the push job.
func NewFanOut(st store.Store, n Notifier, box *secrets.Box) *FanOut {
	return &FanOut{store: st, notifier: n, box: box}
}

// tenantPush is everything a tenant's nudges need, resolved once per pass —
// a batch is usually one busy tenant.
type tenantPush struct {
	tenantID string
	cred     Credential
	settings Settings
	verified bool
}

// ProcessOnce delivers up to limit queued nudges and reports how many it
// processed. Rows abandoned by a worker that died need no sweep — ClaimDue
// treats a lapsed lock as unclaimed.
func (f *FanOut) ProcessOnce(ctx context.Context, limit int) (int, error) {
	rows, token, err := f.store.PushOutbox().ClaimDue(ctx, limit)
	if err != nil {
		return 0, err
	}
	renewFrom := time.Now().Add(renewAfter)
	resolved := map[string]*tenantPush{}
	done := 0
	for i := range rows {
		if ours, err := f.holds(ctx, rows[i].ID, token, renewFrom); err != nil {
			return done, err
		} else if !ours {
			continue
		}
		f.deliverRow(ctx, &rows[i], token, renewFrom, resolved)
		done++
	}
	return done, nil
}

func (f *FanOut) holds(ctx context.Context, id, token string, renewFrom time.Time) (bool, error) {
	if time.Now().Before(renewFrom) {
		return true, nil
	}
	return f.store.PushOutbox().RenewClaim(ctx, id, token)
}

// deliverRow settles one queued nudge. Only a rejected credential is terminal:
// failing drops the notification for good, where a retry costs one pass.
func (f *FanOut) deliverRow(ctx context.Context, row *models.PushOutbox, token string, renewFrom time.Time, resolved map[string]*tenantPush) {
	tp, ok := resolved[row.TenantID]
	if !ok {
		var err error
		if tp, err = f.resolve(ctx, row.TenantID); err != nil {
			f.reschedule(ctx, row)
			return
		}
		resolved[row.TenantID] = tp
	}
	err := f.deliver(ctx, tp, row.MessageID, func() error {
		ours, err := f.holds(ctx, row.ID, token, renewFrom)
		if err != nil {
			return err
		}
		if !ours {
			return errClaimLost
		}
		return nil
	})
	switch {
	case errors.Is(err, errClaimLost):
		// Leave the row alone; settling it would stomp the worker that holds it.
	case err == nil:
		_ = f.store.PushOutbox().MarkDelivered(ctx, row.ID)
	case errors.Is(err, ErrCredential):
		_ = f.store.PushOutbox().MarkFailed(ctx, row.ID)
	default:
		f.reschedule(ctx, row)
	}
}

func (f *FanOut) reschedule(ctx context.Context, row *models.PushOutbox) {
	attempts := row.Attempts + 1
	if attempts >= len(backoffSchedule) {
		_ = f.store.PushOutbox().MarkFailed(ctx, row.ID)
		return
	}
	_ = f.store.PushOutbox().Reschedule(ctx, row.ID, attempts, backoffSchedule[attempts])
}

// deliver sends one message's nudge to every eligible device. stillOurs guards
// the gap between devices: a long fan-out can outlive its claim.
func (f *FanOut) deliver(ctx context.Context, tp *tenantPush, messageID string, stillOurs func() error) error {
	if tp.cred.ProjectID == "" {
		return nil // tenant has not configured push
	}
	msg, err := f.store.Messages().Get(ctx, tp.tenantID, messageID)
	if err != nil {
		return err
	}
	// Internal notes never leave the agent channels (§5.6). Nothing should queue
	// one; this is the backstop that makes that structural.
	if msg.Visibility != models.VisibilityParticipants {
		return nil
	}
	tenantID, convID := msg.TenantID, msg.ConversationID
	conv, err := f.store.Conversations().Get(ctx, tenantID, convID)
	if err != nil {
		return err
	}
	members, err := f.store.Members().ListActive(ctx, tenantID, convID)
	if err != nil {
		return err
	}
	recipients, err := f.recipients(ctx, tenantID, members, msg)
	if err != nil || len(recipients) == 0 {
		return err
	}

	var sender string
	if tp.settings.IncludeSender {
		sender = f.senderName(ctx, tenantID, tp.settings, conv, msg)
	}
	title, body := Compose(tp.settings, sender, msg.Body)

	var failed error
	sent := false
	for _, uid := range recipients {
		tokens, err := f.store.PushTokens().ListForUser(ctx, tenantID, uid)
		if err != nil {
			return err // not "this user has no devices" — try the row again
		}
		if len(tokens) == 0 {
			continue
		}
		n := Nudge{
			ConversationID: convID, Kind: string(conv.Kind), MessageID: messageID,
			Title: title, Body: body, UnreadCount: f.unread(ctx, tenantID, convID, uid),
		}
		for _, t := range tokens {
			if err := stillOurs(); err != nil {
				return err // lost the claim, or could not tell — do not settle it
			}
			switch err := f.notifier.Notify(ctx, tp.cred, Target{Platform: string(t.Platform), Token: t.Token}, n); {
			case err == nil:
				sent = true
			case errors.Is(err, ErrTokenDead):
				_ = f.store.PushTokens().Prune(ctx, tenantID, t.Token)
			default:
				failed = err
			}
		}
	}
	// A first real delivery is what proves the integration, not a credential that
	// merely parsed.
	if sent && !tp.verified {
		if err := f.store.PushConfigs().MarkVerified(ctx, tenantID); err == nil {
			tp.verified = true
		}
	}
	return failed
}

// resolve loads and decrypts a tenant's credential with the settings that shape
// its nudges. A tenant that has not configured push has nothing to send, which
// is not an error.
func (f *FanOut) resolve(ctx context.Context, tenantID string) (*tenantPush, error) {
	cfg, err := f.store.PushConfigs().Get(ctx, tenantID)
	if errors.Is(err, store.ErrNotFound) {
		return &tenantPush{tenantID: tenantID}, nil
	}
	if err != nil {
		return nil, err
	}
	raw, err := f.box.Open(cfg.CredentialSealed, cfg.CredentialKeyID)
	if err != nil {
		return nil, err
	}
	tenant, err := f.store.Tenants().Get(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return &tenantPush{
		tenantID: tenantID,
		cred:     Credential{ProjectID: cfg.ProjectID, ServiceAccountJSON: raw},
		settings: Settings{
			IncludeSender: tenant.PushIncludeSender,
			IncludeBody:   tenant.PushIncludeBody,
			SenderSource:  tenant.PushSenderSource,
		},
		verified: cfg.Verified,
	}, nil
}

// recipients are the user members other than the sender, minus anyone the
// tenant has opted out. Agents, bots and email participants are not push targets.
func (f *FanOut) recipients(ctx context.Context, tenantID string, members []models.ConversationMember, msg *models.Message) ([]string, error) {
	var ids []string
	for _, m := range members {
		if m.MemberKind != models.MemberUser || m.ExternalUserID == nil {
			continue
		}
		if msg.ExternalUserID != nil && *m.ExternalUserID == *msg.ExternalUserID {
			continue // the sender's own devices
		}
		ids = append(ids, *m.ExternalUserID)
	}
	optedOut, err := f.store.Contacts().OptedOut(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	out := ids[:0]
	for _, id := range ids {
		if !optedOut[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// senderName resolves whose name the nudge carries: for a support conversation
// the tenant chooses brand or agent; a peer conversation always names the member.
func (f *FanOut) senderName(ctx context.Context, tenantID string, s Settings, conv *models.Conversation, msg *models.Message) string {
	if conv.Kind == models.KindSupport && msg.InternalActorID != nil {
		if s.SenderSource == models.PushSenderBrand {
			brand, err := f.store.Brands().Active(ctx, tenantID)
			if err != nil {
				return ""
			}
			return brand.Name
		}
		a, err := f.store.Admins().Get(ctx, tenantID, *msg.InternalActorID)
		if err != nil || a == nil {
			return ""
		}
		return a.DisplayName()
	}
	if msg.ExternalUserID == nil {
		return ""
	}
	return f.contactName(ctx, tenantID, *msg.ExternalUserID)
}

// contactName reads the sender's display name off the narrow contact row, the
// same name the inbox and widget render. No blob: a nudge needs one field.
func (f *FanOut) contactName(ctx context.Context, tenantID, externalUserID string) string {
	names, err := f.store.Contacts().Names(ctx, tenantID, []string{externalUserID})
	if err != nil || names[externalUserID] == "" {
		return externalUserID
	}
	return names[externalUserID]
}

func (f *FanOut) unread(ctx context.Context, tenantID, convID, externalUserID string) int {
	counts, err := f.store.Messages().UnreadCounts(ctx, tenantID, []string{convID}, externalUserID)
	if err != nil {
		return 0
	}
	return counts[convID]
}
