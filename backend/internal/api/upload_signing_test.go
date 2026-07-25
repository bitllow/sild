package api_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/testutil"
)

// issueUpload returns a fresh signed PUT grant.
func issueUpload(t *testing.T, h *testutil.Harness, tok string) (objectKey, putPath string) {
	t.Helper()
	var up struct {
		ObjectKey string `json:"object_key"`
		UploadURL string `json:"upload_url"`
	}
	w := h.Request("POST", "/v1/uploads").Bearer(tok).JSON(map[string]any{
		"mime_type": "image/png", "size_bytes": 4, "filename": "a.png",
	}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("issue upload: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &up)
	return up.ObjectKey, mustPath(t, up.UploadURL)
}

// A local upload URL is a signed capability, not an address. Without a valid
// signature it must not be usable — otherwise the URL is a permanent bearer
// token for any object key that can be guessed.
func TestLocalUploadRejectsUnsignedURL(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	objectKey, putPath := issueUpload(t, h, tok)

	bare := strings.SplitN(putPath, "?", 2)[0]
	if w := h.Request("PUT", bare).Bearer(tok).Raw([]byte("x"), "image/png").Do(); w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned PUT should be 401, got %d %s", w.Code, w.Body)
	}
	if w := h.Request("GET", bare).Bearer(tok).Do(); w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned GET should be 401, got %d", w.Code)
	}

	// A tampered signature is no better than none.
	tampered := putPath + "x"
	if w := h.Request("PUT", tampered).Bearer(tok).Raw([]byte("x"), "image/png").Do(); w.Code != http.StatusUnauthorized {
		t.Fatalf("tampered signature should be 401, got %d", w.Code)
	}
	_ = objectKey
}

// The signature binds the verb, so a read grant cannot be replayed as a write.
func TestLocalUploadSignatureBindsMethod(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	_, putPath := issueUpload(t, h, tok)

	// PUT grant replayed as GET.
	if w := h.Request("GET", putPath).Bearer(tok).Do(); w.Code != http.StatusUnauthorized {
		t.Fatalf("PUT grant must not authorize GET, got %d %s", w.Code, w.Body)
	}
}

// The signature binds the object key, so a grant for one object cannot reach
// another — including another tenant's, since the tenant is the key prefix.
func TestLocalUploadSignatureBindsObjectKey(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	_, putPath := issueUpload(t, h, tok)

	u, err := url.Parse(putPath)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the signature, point it at a different object.
	swapped := "/v1/uploads/local/" + tenant.ID + "/obj_other/evil.png?" + u.RawQuery
	if w := h.Request("PUT", swapped).Bearer(tok).Raw([]byte("x"), "image/png").Do(); w.Code != http.StatusUnauthorized {
		t.Fatalf("signature must not transfer to another object key, got %d %s", w.Code, w.Body)
	}
}

// An oversized body must not destroy the object already at that key. os.Create
// truncates before the copy, so a naive cap would turn "upload something huge"
// into a way to delete any attachment whose key you can guess.
func TestOversizedUploadLeavesExistingObjectIntact(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	objectKey, putPath := issueUpload(t, h, tok)

	original := []byte("\x89PNG-original")
	if w := h.Request("PUT", putPath).Bearer(tok).Raw(original, "image/png").Do(); w.Code != http.StatusOK {
		t.Fatalf("seed upload: %d %s", w.Code, w.Body)
	}

	// Attach it first, so we hold a signed GET URL that outlives the failed write.
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
		"body":        "x",
		"attachments": []map[string]any{{"object_key": objectKey, "disposition": "inline"}},
	}).Do()
	if mw.Code != http.StatusCreated {
		t.Fatalf("attach: %d %s", mw.Code, mw.Body)
	}
	testutil.DecodeJSON(t, mw, &msg)
	if len(msg.Attachments) != 1 || msg.Attachments[0].URL == "" {
		t.Fatalf("expected an attachment with a download URL, got %+v", msg.Attachments)
	}
	getPath := mustPath(t, msg.Attachments[0].URL)

	// Re-PUT the same key with a body past the route cap.
	huge := make([]byte, 51<<20)
	if w := h.Request("PUT", putPath).Bearer(tok).Raw(huge, "image/png").Do(); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized upload should be 413, got %d %s", w.Code, w.Body)
	}

	got := h.Request("GET", getPath).Bearer(tok).Do()
	if got.Code != http.StatusOK {
		t.Fatalf("download after rejected upload: %d %s", got.Code, got.Body)
	}
	if got.Body.String() != string(original) {
		t.Fatalf("rejected oversized upload corrupted the object: got %q want %q",
			got.Body.String(), string(original))
	}
}
