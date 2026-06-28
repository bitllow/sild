package api_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/bitllow/sild/backend/internal/testutil"
)

// §11: the local backend's signed PUT/GET URLs are actually served, so an
// upload → attach → download round-trip works in the default configuration.
func TestLocalUploadRoundTrip(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")

	var conv struct{ ID string `json:"id"` }
	w := h.Request("POST", "/v1/me/support-requests").Bearer(tok).JSON(map[string]any{}).Do()
	testutil.DecodeJSON(t, w, &conv)

	// 1) request a signed upload URL
	var up struct {
		ObjectKey string `json:"object_key"`
		UploadURL string `json:"upload_url"`
	}
	w = h.Request("POST", "/v1/uploads").Bearer(tok).JSON(map[string]any{
		"mime_type": "image/png", "size_bytes": 4, "filename": "a.png",
	}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("issue upload: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &up)

	// 2) PUT the bytes to the signed URL (path served by the local backend)
	putPath := mustPath(t, up.UploadURL)
	payload := []byte("\x89PNG")
	if w = h.Request("PUT", putPath).Bearer(tok).Raw(payload, "image/png").Do(); w.Code != http.StatusOK {
		t.Fatalf("PUT upload: %d %s", w.Code, w.Body)
	}

	// 3) attach to a message
	var msg struct {
		Attachments []struct {
			ObjectKey string `json:"object_key"`
			URL       string `json:"url"`
		} `json:"attachments"`
	}
	w = h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").Bearer(tok).JSON(map[string]any{
		"body": "see attached",
		"attachments": []map[string]any{{"object_key": up.ObjectKey, "disposition": "inline"}},
	}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("send with attachment: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &msg)
	if len(msg.Attachments) != 1 || msg.Attachments[0].URL == "" {
		t.Fatalf("expected an attachment with a download URL, got %+v", msg.Attachments)
	}

	// 4) GET the bytes back via the signed download URL
	w = h.Request("GET", mustPath(t, msg.Attachments[0].URL)).Bearer(tok).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("GET attachment: %d", w.Code)
	}
	if w.Body.String() != string(payload) {
		t.Fatalf("downloaded bytes differ: %q", w.Body.String())
	}
}

func mustPath(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url %q: %v", raw, err)
	}
	if u.RawQuery != "" {
		return u.Path + "?" + u.RawQuery
	}
	return u.Path
}
