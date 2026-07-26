package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/testutil"
)

// A filename with a space escapes into the object key, and the signature covers
// the escaped form — so the route must verify against the same representation.
func TestSignedUploadHandlesEscapedFilename(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")

	var up struct {
		ObjectKey string `json:"object_key"`
		UploadURL string `json:"upload_url"`
	}
	w := h.Request("POST", "/v1/uploads").Bearer(tok).JSON(map[string]any{
		"mime_type": "image/png", "size_bytes": 4, "filename": "my photo.png",
	}).Do()
	testutil.DecodeJSON(t, w, &up)

	payload := []byte("\x89PNG-escaped")
	if w := h.Request("PUT", mustPath(t, up.UploadURL)).Bearer(tok).Raw(payload, "image/png").Do(); w.Code != http.StatusOK {
		t.Fatalf("signed PUT for an escaped filename: %d %s", w.Code, w.Body)
	}

	// And it reads back through the signed GET, so the on-disk name and the
	// signed key agree in both directions.
	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)
	var msg struct {
		Attachments []struct {
			URL string `json:"url"`
		} `json:"attachments"`
	}
	mw := h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").Bearer(tok).JSON(map[string]any{
		"body":        "see attached",
		"attachments": []map[string]any{{"object_key": up.ObjectKey, "disposition": "inline"}},
	}).Do()
	testutil.DecodeJSON(t, mw, &msg)
	if len(msg.Attachments) != 1 || msg.Attachments[0].URL == "" {
		t.Fatalf("expected a signed download URL, got %+v", msg.Attachments)
	}
	got := h.Request("GET", mustPath(t, msg.Attachments[0].URL)).Bearer(tok).Do()
	if got.Code != http.StatusOK || got.Body.String() != string(payload) {
		t.Fatalf("download of an escaped-filename object: %d %q", got.Code, got.Body.String())
	}
}
