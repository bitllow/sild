package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// expandFixture is a tenant with one support conversation whose client has a
// stored profile, plus an owner session to read it back with.
type expandFixture struct {
	h      *testutil.Harness
	tenant *models.Tenant
	owner  string
	convID string
}

func newExpandFixture(t *testing.T) *expandFixture {
	t.Helper()
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	h.SeedContact(tenant.ID, "u_mari", `{"name":"Mari Tamm","plan":"gold"}`)
	return &expandFixture{h: h, tenant: tenant, owner: owner, convID: newConversation(t, h, tenant.ID, "u_mari")}
}

// get reads a path as the owner and decodes the top-level object.
func (f *expandFixture) get(t *testing.T, path string) map[string]any {
	t.Helper()
	w := f.h.Request("GET", path).Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", path, w.Code, w.Body)
	}
	var body map[string]any
	testutil.DecodeJSON(t, w, &body)
	return body
}

// contactsBlock reads the expansion block off a response.
func contactsBlock(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["contacts"].([]any)
	if !ok {
		t.Fatalf("no contacts block: %v", body)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		m, _ := e.(map[string]any)
		out = append(out, m)
	}
	return out
}

// The list is where the expansion earns its keep — the inbox renders many names
// at once, and a per-row fetch is the fan-out this exists to remove. The bare
// name yields identity and display name, and NOT the profile blob.
func TestExpandContactsOnListAndDetail(t *testing.T) {
	f := newExpandFixture(t)

	for _, path := range []string{
		"/v1/conversations?kind=support&expand=contacts",
		"/v1/conversations/" + f.convID + "?expand=contacts",
	} {
		block := contactsBlock(t, f.get(t, path))
		if len(block) != 1 {
			t.Fatalf("%s: block has %d entries, want 1", path, len(block))
		}
		if block[0]["external_user_id"] != "u_mari" {
			t.Fatalf("%s: %v", path, block[0])
		}
		if block[0]["name"] != "Mari Tamm" {
			t.Fatalf("%s: name = %v", path, block[0]["name"])
		}
		if _, present := block[0]["metadata"]; present {
			t.Fatalf("%s: the default expansion carried the blob: %v", path, block[0])
		}
	}
}

// The blob is a separate entity: it arrives only when a path names it.
func TestExpandMetadataIsOptIn(t *testing.T) {
	f := newExpandFixture(t)
	block := contactsBlock(t, f.get(t, "/v1/conversations/"+f.convID+"?expand=contacts.metadata"))[0]

	meta, _ := block["metadata"].(map[string]any)
	if meta["name"] != "Mari Tamm" || meta["plan"] != "gold" {
		t.Fatalf("metadata = %v", block["metadata"])
	}
	// Identity rides along regardless, or the caller cannot attribute the profile.
	if block["external_user_id"] != "u_mari" {
		t.Fatalf("no identity on a metadata-only path: %v", block)
	}
}

// One key of the blob, for a caller that wants a field and not a page of JSON.
func TestExpandSelectsOneMetadataKey(t *testing.T) {
	f := newExpandFixture(t)
	block := contactsBlock(t, f.get(t, "/v1/conversations/"+f.convID+"?expand=contacts.metadata.plan"))[0]

	meta, _ := block["metadata"].(map[string]any)
	if meta["plan"] != "gold" {
		t.Fatalf("selected key missing: %v", block["metadata"])
	}
	if _, present := meta["name"]; present {
		t.Fatalf("an unselected key came along: %v", meta)
	}
}

// Asking for the field whole and for one of its keys must not return less than
// the whole — whichever order the paths arrive in.
func TestExpandWholeFieldBeatsKeySelection(t *testing.T) {
	f := newExpandFixture(t)
	for _, expand := range []string{
		"contacts.metadata,contacts.metadata.plan",
		"contacts.metadata.plan,contacts.metadata",
	} {
		block := contactsBlock(t, f.get(t, "/v1/conversations/"+f.convID+"?expand="+expand))[0]
		meta, _ := block["metadata"].(map[string]any)
		if meta["name"] != "Mari Tamm" || meta["plan"] != "gold" {
			t.Fatalf("expand=%s narrowed the whole field: %v", expand, meta)
		}
	}
}

// A dotted path narrows the block; several paths for one resource union.
func TestExpandFieldSelection(t *testing.T) {
	f := newExpandFixture(t)
	base := "/v1/conversations/" + f.convID

	narrow := contactsBlock(t, f.get(t, base+"?expand=contacts.name"))[0]
	if _, present := narrow["metadata"]; present {
		t.Fatalf("a narrow expansion carried metadata: %v", narrow)
	}
	if narrow["external_user_id"] != "u_mari" {
		t.Fatalf("narrow identity = %v", narrow["external_user_id"])
	}

	union := contactsBlock(t, f.get(t, base+"?expand=contacts,contacts.metadata"))[0]
	if union["name"] != "Mari Tamm" {
		t.Fatalf("union lost the default fields: %v", union)
	}
	if _, present := union["metadata"]; !present {
		t.Fatalf("union lost the named field: %v", union)
	}
}

// A typo must not return a page that merely looks unenriched.
func TestExpandRejectsUnknownNames(t *testing.T) {
	f := newExpandFixture(t)

	for _, expand := range []string{
		"contact", "contacts.id", "contacts.search_text", "conversations",
		"contacts.name.first", // name is a plain field; it has no keys
		"contacts.metadata.",  // an empty key selects nothing
	} {
		w := f.h.Request("GET", "/v1/conversations/"+f.convID+"?expand="+expand).
			Cookie("sild_admin", f.owner).Do()
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expand=%s = %d %s, want 400", expand, w.Code, w.Body)
		}
	}
}

// Naming no expansion leaves the response exactly as it was.
func TestNoExpandCarriesNoBlock(t *testing.T) {
	f := newExpandFixture(t)
	if _, present := f.get(t, "/v1/conversations/"+f.convID)["contacts"]; present {
		t.Fatal("an unexpanded read carried a contacts block")
	}
}
