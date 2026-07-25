package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/testutil"
)

// A route's own larger body limit must actually apply: a nested MaxBytesReader
// can only ever lower the cap, so a group-wide JSON limit would silently clamp
// the raw-body routes.
func TestRawRouteHonorsItsOwnBodyLimit(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	_, putPath := issueUpload(t, h, tok)

	// 1 MiB: over the JSON cap (256 KiB), well under the upload cap (50 MiB).
	payload := make([]byte, 1<<20)
	if w := h.Request("PUT", putPath).Bearer(tok).Raw(payload, "image/png").Do(); w.Code != http.StatusOK {
		t.Fatalf("1 MiB upload should be accepted, got %d %s", w.Code, w.Body)
	}
}
