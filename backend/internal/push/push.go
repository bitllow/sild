// Package push delivers offline notifications (§5.5). A nudge goes to every
// member's devices except the sender's; a device whose app is in the foreground
// discards it, which is why there is no presence check here — see
// docs/adr/0002-nudges-are-sent-without-a-presence-check.md.
//
// Notifications are addressed through the TENANT's own push project, so every
// send carries that tenant's credential rather than a process-wide one.
package push

import (
	"context"
	"encoding/json"
	"errors"
)

// Nudge is what a device is told about a message. Composed server-side: a device
// whose app is not in the foreground displays what arrived and runs no app code.
type Nudge struct {
	ConversationID string
	// Kind is the conversation kind, so a tap routes without a round-trip.
	Kind        string
	MessageID   string
	Title       string
	Body        string
	UnreadCount int
}

// Target is one device to notify.
type Target struct {
	Platform string
	Token    string
}

// Credential is a tenant's decrypted push-project credential.
type Credential struct {
	ProjectID string
	// ServiceAccountJSON authenticates to the tenant's project.
	ServiceAccountJSON []byte
}

// Delivery outcomes the fan-out must tell apart.
var (
	// ErrTokenDead means the device is gone — uninstalled, or the token was
	// reissued. The row is pruned; this is the only path by which the token table
	// shrinks other than an explicit deregistration.
	ErrTokenDead = errors.New("push token is no longer registered")
	// ErrRetryable is a transient provider failure: rate limit or 5xx.
	ErrRetryable = errors.New("push provider unavailable")
	// ErrCredential means the tenant's credential was rejected. Retrying with the
	// same credential cannot help, so the nudge is dropped rather than requeued.
	ErrCredential = errors.New("push credential rejected")
)

// Notifier delivers one nudge to one device. One call per device because FCM v1
// has no multicast — the batch endpoint was removed.
type Notifier interface {
	Notify(ctx context.Context, cred Credential, tgt Target, n Nudge) error
	// Check exercises the credential without sending anything, so a tenant
	// saving a bad one is told at that moment rather than by a user who never
	// got notified. It is also the only check available before any device exists.
	Check(ctx context.Context, cred Credential) error
}

// NoopNotifier drops nudges. Used where no transport is wired.
type NoopNotifier struct{}

func (NoopNotifier) Notify(context.Context, Credential, Target, Nudge) error { return nil }
func (NoopNotifier) Check(context.Context, Credential) error                 { return nil }

// ServiceAccount is the non-secret identity inside a push credential.
type ServiceAccount struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
}

// ParseServiceAccount reads a credential's identity and rejects anything that
// could not authenticate, without a network call.
func ParseServiceAccount(raw []byte) (ServiceAccount, error) {
	var sa ServiceAccount
	if err := json.Unmarshal(raw, &sa); err != nil {
		return sa, errors.New("credential is not valid JSON")
	}
	switch {
	case sa.Type != "service_account":
		return sa, errors.New(`credential type must be "service_account"`)
	case sa.ProjectID == "":
		return sa, errors.New("credential has no project_id")
	case sa.ClientEmail == "":
		return sa, errors.New("credential has no client_email")
	case sa.PrivateKey == "":
		return sa, errors.New("credential has no private_key")
	}
	return sa, nil
}
