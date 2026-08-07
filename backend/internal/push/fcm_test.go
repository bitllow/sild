package push

// Transport-level tests: wire format and error classification are invisible
// above the Notifier interface, so they are exercised against a stand-in server
// the way the webhook relay tests its endpoint. Internal to the package so the
// endpoint can be redirected without widening the API for tests.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeFCM stands in for both the OAuth token endpoint and the send endpoint.
type fakeFCM struct {
	server *httptest.Server
	// status is what the send endpoint answers with (200 unless a test says else).
	status int
	body   string
	// sent captures the last decoded send payload.
	sent map[string]any
	// sends counts delivery attempts.
	sends int
}

func newFakeFCM(t *testing.T) *fakeFCM {
	t.Helper()
	f := &fakeFCM{status: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/v1/projects/", func(w http.ResponseWriter, r *http.Request) {
		f.sends++
		var body struct {
			Message map[string]any `json:"message"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.sent = body.Message
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// credential builds a service account whose token endpoint is the fake, so the
// whole exchange runs offline.
func (f *fakeFCM) credential(t *testing.T) Credential {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: mustPKCS8(t, key)})
	sa, _ := json.Marshal(map[string]string{
		"type":         "service_account",
		"project_id":   "test-project",
		"client_email": "push@test-project.iam.gserviceaccount.com",
		"private_key":  string(pemKey),
		"token_uri":    f.server.URL + "/token",
	})
	return Credential{ProjectID: "test-project", ServiceAccountJSON: sa}
}

func mustPKCS8(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return b
}

func newTestFCM(base string) *FCM {
	f := NewFCM()
	f.baseURL = base
	return f
}

func TestNotifySendsAnAlertWithData(t *testing.T) {
	fake := newFakeFCM(t)
	f := newTestFCM(fake.server.URL)

	err := f.Notify(context.Background(), fake.credential(t),
		Target{Platform: "android", Token: "device-1"},
		Nudge{ConversationID: "c_1", Kind: "peer", MessageID: "m_1", Title: "Alice", Body: "hi", UnreadCount: 3})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}

	// Both halves must be present: the notification is what the system displays
	// while backgrounded, the data is what a foregrounded app reads to suppress.
	if _, ok := fake.sent["notification"]; !ok {
		t.Fatal("payload carries no notification — a foregrounded app would be the only thing that could display it")
	}
	data, _ := fake.sent["data"].(map[string]any)
	if data["conversation_id"] != "c_1" || data["conversation_kind"] != "peer" {
		t.Fatalf("data does not identify the conversation: %v", data)
	}
	if data["unread_count"] != "3" {
		t.Fatalf("unread_count = %v, want \"3\"", data["unread_count"])
	}
	if data["sild"] != "1" {
		t.Fatal("payload is not marked as ours — a host app could not tell it from its own")
	}
}

// One notification per conversation that updates in place, rather than a stack
// of them for an active thread.
func TestNotifyCollapsesPerConversation(t *testing.T) {
	fake := newFakeFCM(t)
	f := newTestFCM(fake.server.URL)

	err := f.Notify(context.Background(), fake.credential(t), Target{Token: "device-1"},
		Nudge{ConversationID: "c_42", Title: "Alice"})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	android, _ := fake.sent["android"].(map[string]any)
	if android["collapse_key"] != "c_42" {
		t.Fatalf("android collapse_key = %v", android["collapse_key"])
	}
	apns, _ := fake.sent["apns"].(map[string]any)
	headers, _ := apns["headers"].(map[string]any)
	if headers["apns-collapse-id"] != "c_42" {
		t.Fatalf("apns-collapse-id = %v", headers["apns-collapse-id"])
	}
}

// The host app creates this channel during integration; a payload naming a
// different one is dropped by Android silently.
func TestNotifyNamesTheNotificationChannel(t *testing.T) {
	fake := newFakeFCM(t)
	f := newTestFCM(fake.server.URL)

	if err := f.Notify(context.Background(), fake.credential(t), Target{Token: "d"}, Nudge{ConversationID: "c_1"}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	android, _ := fake.sent["android"].(map[string]any)
	notif, _ := android["notification"].(map[string]any)
	if notif["channel_id"] != NotificationChannel {
		t.Fatalf("channel_id = %v, want %q", notif["channel_id"], NotificationChannel)
	}
}

// Classification is what decides between pruning a device, retrying, and giving
// up — getting it wrong either loses tokens or keeps a dead one forever.
func TestNotifyClassifiesFailures(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"unregistered device", http.StatusNotFound, fcmError("NOT_FOUND", "UNREGISTERED"), ErrTokenDead},
		{"malformed token", http.StatusBadRequest, fcmError("INVALID_ARGUMENT", "INVALID_ARGUMENT"), ErrTokenDead},
		{"credential rejected", http.StatusUnauthorized, fcmError("UNAUTHENTICATED", ""), ErrCredential},
		{"rate limited", http.StatusTooManyRequests, fcmError("RESOURCE_EXHAUSTED", "QUOTA_EXCEEDED"), ErrRetryable},
		{"provider down", http.StatusServiceUnavailable, fcmError("UNAVAILABLE", "UNAVAILABLE"), ErrRetryable},
		// The dangerous ones: statuses a dead token also produces, but here the
		// project is misconfigured and every token would be pruned at once.
		{"project not found", http.StatusNotFound, fcmError("NOT_FOUND", ""), ErrCredential},
		{"messaging api disabled", http.StatusForbidden, fcmError("PERMISSION_DENIED", ""), ErrCredential},
		{"malformed payload", http.StatusBadRequest, badRequestError(), ErrCredential},
		{"wrong project's credential", http.StatusForbidden, fcmError("PERMISSION_DENIED", "SENDER_ID_MISMATCH"), ErrCredential},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeFCM(t)
			fake.status, fake.body = tc.status, tc.body
			f := newTestFCM(fake.server.URL)

			err := f.Notify(context.Background(), fake.credential(t), Target{Token: "d"}, Nudge{ConversationID: "c_1"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("status %d classified as %v, want %v", tc.status, err, tc.want)
			}
		})
	}
}

// badRequestError is FCM's other 400: a payload complaint carrying a
// google.rpc.BadRequest detail, which says nothing about the device.
func badRequestError() string {
	return `{"error":{"status":"INVALID_ARGUMENT","details":[` +
		`{"@type":"type.googleapis.com/google.rpc.BadRequest",` +
		`"fieldViolations":[{"field":"message.android.ttl"}]}]}}`
}

// fcmError builds a v1 error body: a top-level status plus the FcmError detail
// that actually says why.
func fcmError(status, errorCode string) string {
	if errorCode == "" {
		return `{"error":{"status":"` + status + `"}}`
	}
	return `{"error":{"status":"` + status + `","details":[` +
		`{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"` + errorCode + `"}]}}`
}

func TestCheckExchangesTheCredential(t *testing.T) {
	fake := newFakeFCM(t)
	f := newTestFCM(fake.server.URL)

	if err := f.Check(context.Background(), fake.credential(t)); err != nil {
		t.Fatalf("check: %v", err)
	}
	if fake.sends != 0 {
		t.Fatal("Check delivered a notification — it must not send anything")
	}
}

func TestCheckRejectsAMalformedCredential(t *testing.T) {
	f := newTestFCM("http://unused.invalid")
	err := f.Check(context.Background(), Credential{ProjectID: "p", ServiceAccountJSON: []byte(`{"type":"service_account"}`)})
	if !errors.Is(err, ErrCredential) {
		t.Fatalf("check with no private key = %v, want ErrCredential", err)
	}
}

func TestParseServiceAccountRejectsIncompleteCredentials(t *testing.T) {
	cases := []struct{ name, raw, want string }{
		{"not json", `nope`, "valid JSON"},
		{"wrong type", `{"type":"authorized_user","project_id":"p"}`, "service_account"},
		{"no project", `{"type":"service_account","client_email":"a@b","private_key":"k"}`, "project_id"},
		{"no client email", `{"type":"service_account","project_id":"p","private_key":"k"}`, "client_email"},
		{"no private key", `{"type":"service_account","project_id":"p","client_email":"a@b"}`, "private_key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseServiceAccount([]byte(tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParseServiceAccount(%s) = %v, want an error naming %q", tc.raw, err, tc.want)
			}
		})
	}
}
