package auth

import "testing"

func TestAPIKeyRoundTrip(t *testing.T) {
	k, err := GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	prefix, secret, err := ParseAPIKey(k.Full)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if prefix != k.Prefix {
		t.Errorf("prefix mismatch: %s != %s", prefix, k.Prefix)
	}
	if !VerifySecret(secret, k.Hash) {
		t.Error("secret should verify against its hash")
	}
	if VerifySecret("wrong", k.Hash) {
		t.Error("wrong secret must not verify")
	}
}

func TestParseAPIKeyMalformed(t *testing.T) {
	for _, s := range []string{"", "nope", "sild_live_", "sild_live_onlyprefix", "bearer x"} {
		if _, _, err := ParseAPIKey(s); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

func TestES256KeyGen(t *testing.T) {
	priv, pub, err := GenerateES256()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseECPrivate(priv); err != nil {
		t.Errorf("parse private: %v", err)
	}
	if _, err := parseECPublic(pub); err != nil {
		t.Errorf("parse public: %v", err)
	}
}

func TestSessionToken(t *testing.T) {
	tok, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if HashSessionToken(tok.Raw) != tok.Hash {
		t.Error("hash should be deterministic for the raw token")
	}
}
