package secrets_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/bitllow/sild/backend/internal/secrets"
)

const (
	keyA = "ZGV2LW9ubHktdGVzdC1rZXktMzJieXRlcy1sb25nISE="
	keyB = "YW5vdGhlci10ZXN0LWtleS0zMmJ5dGVzLWxvbmchISE="
)

func box(t *testing.T, key string) *secrets.Box {
	t.Helper()
	b, err := secrets.New(key)
	if err != nil {
		t.Fatalf("new box: %v", err)
	}
	return b
}

func TestSealOpenRoundTrip(t *testing.T) {
	b := box(t, keyA)
	plain := []byte(`{"type":"service_account","project_id":"acme"}`)

	sealed, err := b.Seal(plain)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Contains(sealed, []byte("service_account")) {
		t.Fatal("sealed value still contains the plaintext")
	}
	got, err := b.Open(sealed, b.KeyID())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round trip = %s, want %s", got, plain)
	}
}

// Two seals of one value must differ, or a repeated credential is recognisable
// in the column without the key.
func TestSealIsNonDeterministic(t *testing.T) {
	b := box(t, keyA)
	first, _ := b.Seal([]byte("same"))
	second, _ := b.Seal([]byte("same"))
	if bytes.Equal(first, second) {
		t.Fatal("two seals of one plaintext are identical")
	}
}

// A rotated or mistyped key must say so rather than return garbage.
func TestOpenWithTheWrongKeyFails(t *testing.T) {
	sealed, err := box(t, keyA).Seal([]byte("secret"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	other := box(t, keyB)
	if _, err := other.Open(sealed, other.KeyID()); !errors.Is(err, secrets.ErrWrongKey) {
		t.Fatalf("open with wrong key = %v, want ErrWrongKey", err)
	}
}

// The key id travels with the ciphertext so a mismatch is reported as one,
// rather than as an opaque authentication failure.
func TestOpenReportsAKeyIDMismatch(t *testing.T) {
	b := box(t, keyA)
	sealed, _ := b.Seal([]byte("secret"))
	if _, err := b.Open(sealed, "deadbeef"); !errors.Is(err, secrets.ErrWrongKey) {
		t.Fatalf("open with foreign key id = %v, want ErrWrongKey", err)
	}
}

func TestKeyIDDiffersPerKey(t *testing.T) {
	if box(t, keyA).KeyID() == box(t, keyB).KeyID() {
		t.Fatal("two keys share a key id")
	}
}

// An unconfigured box must fail loudly at the write, never store plaintext.
func TestNoKeyFailsEveryOperation(t *testing.T) {
	b := box(t, "")
	if b.Configured() {
		t.Fatal("a box with no key reports itself configured")
	}
	if _, err := b.Seal([]byte("x")); !errors.Is(err, secrets.ErrNoKey) {
		t.Fatalf("seal without a key = %v, want ErrNoKey", err)
	}
	if _, err := b.Open([]byte("x"), ""); !errors.Is(err, secrets.ErrNoKey) {
		t.Fatalf("open without a key = %v, want ErrNoKey", err)
	}
}

func TestMalformedKeysAreRejected(t *testing.T) {
	for _, tc := range []struct{ name, key string }{
		{"not base64", "not-base64!!"},
		{"too short", "c2hvcnQ="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := secrets.New(tc.key); err == nil {
				t.Fatal("accepted a malformed key")
			}
		})
	}
}
