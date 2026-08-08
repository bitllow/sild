package push

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// fcmScope is the only scope a message send needs.
const fcmScope = "https://www.googleapis.com/auth/firebase.messaging"

// NotificationChannel is the Android channel every nudge names. The host app
// creates it during integration; without it Android 8+ drops the notification.
const NotificationChannel = "sild_messages"

// nudgeTTL is how long the provider may keep trying before discarding. A
// notification about an hour-old message is worse than none.
const nudgeTTL = time.Hour

// FCM sends through Firebase Cloud Messaging v1, which reaches Android directly
// and iOS through the tenant's own APNs key held in their Firebase project — so
// Sild never stores platform keys.
type FCM struct {
	client *http.Client
	// baseURL is the send endpoint, overridden in tests.
	baseURL string
	// sources caches one token source per credential: minting an access token
	// re-signs a JWT and costs a round-trip, and a fan-out sends per device.
	sources sync.Map // credential fingerprint -> oauth2.TokenSource
}

// NewFCM constructs the transport. dig provides it as the Notifier.
func NewFCM() *FCM {
	return &FCM{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://fcm.googleapis.com",
	}
}

func (f *FCM) Notify(ctx context.Context, cred Credential, tgt Target, n Nudge) error {
	src, err := f.tokenSource(ctx, cred)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCredential, err)
	}
	// Retryable, not a credential error: minting is a network call, so a failure
	// here is usually the network. Check treats the same failure as the credential.
	tok, err := src.Token()
	if err != nil {
		return fmt.Errorf("%w: minting an access token: %v", ErrRetryable, err)
	}

	body, err := json.Marshal(map[string]any{"message": fcmMessage(tgt, n)})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/v1/projects/%s/messages:send", f.baseURL, cred.ProjectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	tok.SetAuthHeader(req)

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRetryable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return classify(resp.StatusCode, resp.Body)
}

// Check exchanges the credential for an access token. That proves the JSON
// parses, the key is live, the service account exists and the clock is sane —
// separating "your credential is wrong" from "that device is wrong" when a test
// send fails.
func (f *FCM) Check(ctx context.Context, cred Credential) error {
	src, err := f.tokenSource(ctx, cred)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCredential, err)
	}
	if _, err := src.Token(); err != nil {
		return fmt.Errorf("%w: %v", ErrCredential, err)
	}
	return nil
}

// fcmErrorType is the details entry that carries FCM's own reason. A 400 can
// instead carry a google.rpc.BadRequest describing a bad payload, which says
// nothing about the device.
const fcmErrorType = "type.googleapis.com/google.firebase.fcm.v1.FcmError"

// deadTokenCodes are the reasons that mean this device is gone. Deliberately
// short: deleting a token is unrecoverable until the app registers again,
// whereas keeping a stale one costs a single failed send. SENDER_ID_MISMATCH is
// excluded — the wrong credential produces it for every valid token at once.
var deadTokenCodes = map[string]bool{
	"UNREGISTERED":     true, // uninstalled, or the token was reissued
	"INVALID_ARGUMENT": true, // malformed token
}

// classify maps a send failure onto what the fan-out should do about it.
func classify(code int, body io.Reader) error {
	raw, _ := io.ReadAll(io.LimitReader(body, 4096))
	reason := fcmErrorCode(raw)
	if deadTokenCodes[reason] {
		return fmt.Errorf("%w: %d %s", ErrTokenDead, code, reason)
	}
	switch {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return fmt.Errorf("%w: %d %s", ErrCredential, code, raw)
	case code == http.StatusTooManyRequests || code >= 500:
		return fmt.Errorf("%w: %d %s", ErrRetryable, code, raw)
	}
	// An unrecognised 4xx is a payload or project problem, not a dead device.
	return fmt.Errorf("%w: %d %s", ErrCredential, code, raw)
}

// fcmErrorCode reads FCM's reason from the FcmError detail, and only from there:
// the top-level status is shared with payload and project errors, so falling
// back to it would prune live tokens.
func fcmErrorCode(raw []byte) string {
	var body struct {
		Error struct {
			Details []struct {
				Type      string `json:"@type"`
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return ""
	}
	for _, d := range body.Error.Details {
		if d.Type == fcmErrorType {
			return d.ErrorCode
		}
	}
	return ""
}

// fcmMessage builds the v1 payload. It carries BOTH notification and data: the
// notification is what the system displays when the app is backgrounded, and the
// data is what the app reads when it is foregrounded and decides to suppress.
// A data-only message would put iOS on silent pushes, which are throttled and
// not guaranteed (ADR 0002).
func fcmMessage(tgt Target, n Nudge) map[string]any {
	data := map[string]string{
		"sild":              "1", // lets a host app tell our messages from its own
		"conversation_id":   n.ConversationID,
		"conversation_kind": n.Kind,
		"message_id":        n.MessageID,
		"unread_count":      strconv.Itoa(n.UnreadCount),
	}
	notification := map[string]any{"title": n.Title}
	if n.Body != "" {
		notification["body"] = n.Body
	}
	return map[string]any{
		"token":        tgt.Token,
		"notification": notification,
		"data":         data,
		// collapse_key/tag and apns-collapse-id all key on the conversation, so an
		// active thread updates one notification instead of stacking.
		"android": map[string]any{
			"priority":     "high",
			"ttl":          strconv.Itoa(int(nudgeTTL.Seconds())) + "s",
			"collapse_key": n.ConversationID,
			"notification": map[string]any{
				"channel_id": NotificationChannel,
				"tag":        n.ConversationID,
			},
		},
		"apns": map[string]any{
			"headers": map[string]string{
				"apns-collapse-id": n.ConversationID,
				"apns-priority":    "10",
				"apns-expiration":  strconv.FormatInt(time.Now().Add(nudgeTTL).Unix(), 10),
			},
			"payload": map[string]any{
				"aps": map[string]any{"thread-id": n.ConversationID, "sound": "default"},
			},
		},
	}
}

// tokenSource returns the cached source for a credential, building one on first
// use. Keyed by digest so a rotated credential gets a new source rather than
// reusing the old project's.
func (f *FCM) tokenSource(ctx context.Context, cred Credential) (oauth2.TokenSource, error) {
	sum := sha256.Sum256(cred.ServiceAccountJSON)
	key := hex.EncodeToString(sum[:])
	if got, ok := f.sources.Load(key); ok {
		return got.(oauth2.TokenSource), nil
	}
	cfg, err := google.JWTConfigFromJSON(cred.ServiceAccountJSON, fcmScope)
	if err != nil {
		return nil, err
	}
	// The source outlives this call and refreshes hourly, so it must not hold the
	// caller's context — a cached source built from a request would refresh
	// against a cancelled one forever after.
	src := oauth2.ReuseTokenSource(nil, cfg.TokenSource(context.WithoutCancel(ctx)))
	actual, _ := f.sources.LoadOrStore(key, src)
	return actual.(oauth2.TokenSource), nil
}

var _ Notifier = (*FCM)(nil)
