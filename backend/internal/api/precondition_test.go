package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

func brandsBody(name string) map[string]any {
	return map[string]any{
		"active_brand_id": "b1",
		"brands":          []any{map[string]any{"id": "b1", "name": name, "config": map[string]any{}}},
	}
}

// Two operators editing the same screen: the second save is based on a version
// the first already replaced, so it must lose loudly rather than overwrite.
func TestStaleConfigWriteIsRejected(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	read := h.Request("GET", "/v1/brands").Cookie("sild_admin", owner).Do()
	if read.Code != http.StatusOK {
		t.Fatalf("read brands: %d %s", read.Code, read.Body)
	}
	etag := read.Header().Get("ETag")
	if etag == "" {
		t.Fatal("a config read must carry an ETag")
	}

	// Operator A saves against that version.
	if w := h.Request("PUT", "/v1/brands").Cookie("sild_admin", owner).
		Header("If-Match", etag).JSON(brandsBody("A")).Do(); w.Code != http.StatusOK {
		t.Fatalf("first save: %d %s", w.Code, w.Body)
	}

	// Operator B saves against the SAME (now stale) version.
	w := h.Request("PUT", "/v1/brands").Cookie("sild_admin", owner).
		Header("If-Match", etag).JSON(brandsBody("B")).Do()
	if w.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale save should be 412, got %d %s", w.Code, w.Body)
	}

	// A's write survived.
	after := h.Request("GET", "/v1/brands").Cookie("sild_admin", owner).Do()
	var got struct {
		Brands []struct {
			Name string `json:"name"`
		} `json:"brands"`
	}
	testutil.DecodeJSON(t, after, &got)
	if len(got.Brands) != 1 || got.Brands[0].Name != "A" {
		t.Fatalf("the stale write overwrote the earlier one: %+v", got.Brands)
	}
}

// Absent and malformed preconditions are distinct causes with distinct statuses.
func TestPreconditionStatuses(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	if w := h.Request("PUT", "/v1/brands").Cookie("sild_admin", owner).
		JSON(brandsBody("x")).Do(); w.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing If-Match should be 428, got %d %s", w.Code, w.Body)
	}
	if w := h.Request("PUT", "/v1/brands").Cookie("sild_admin", owner).
		Header("If-Match", "not-a-tag").JSON(brandsBody("x")).Do(); w.Code != http.StatusBadRequest {
		t.Fatalf("malformed If-Match should be 400, got %d %s", w.Code, w.Body)
	}
}

// A write must return the NEW version, or the client falls back to If-Match: *
// on the next edit and quietly loses the protection.
func TestConfigWriteReturnsNewETag(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	w := h.Request("PATCH", "/v1/channels/email").Cookie("sild_admin", owner).
		Header("If-Match", "*").JSON(map[string]any{"auto_reply": true}).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("update channel: %d %s", w.Code, w.Body)
	}
	next := w.Header().Get("ETag")
	if next == "" {
		t.Fatal("a config write must return the new ETag")
	}
	// And that version is immediately usable for the following edit.
	if w2 := h.Request("PATCH", "/v1/channels/email").Cookie("sild_admin", owner).
		Header("If-Match", next).JSON(map[string]any{"spam_filter": true}).Do(); w2.Code != http.StatusOK {
		t.Fatalf("chained edit with the returned ETag: %d %s", w2.Code, w2.Body)
	}
}
