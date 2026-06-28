package auth

import "golang.org/x/crypto/bcrypt"

// Password auth is an alternative admin/inbox login method to Google OIDC (§2.4).
// bcrypt (deliberately slow) is the right tool here — unlike API keys, a password
// is low-entropy and human-chosen.

// HashPassword returns a bcrypt hash for storage.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether plain matches the stored hash (constant-time
// within bcrypt).
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
