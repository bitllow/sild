package storage

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Local upload URLs are signed capabilities, not addresses: unsigned, the URL is
// a permanent bearer token for any guessable object key.
//
// The signature binds the verb, the object key and an expiry. The tenant comes
// along free — it is the key's first path segment.

// SignedParams are the query parameters carried by a signed local URL.
const (
	ParamExpires   = "exp"
	ParamSignature = "sig"
)

// LocalSigner signs and verifies local object URLs.
type LocalSigner struct{ key []byte }

// NewLocalSigner returns a signer for the configured key. An empty key gets a
// random per-process one — fine for one dev node, wrong for a fleet.
func NewLocalSigner(key string) *LocalSigner {
	if key == "" {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		return &LocalSigner{key: b}
	}
	return &LocalSigner{key: []byte(key)}
}

// Sign returns the query string (without "?") authorizing method+objectKey until
// expiry.
func (s *LocalSigner) Sign(method, objectKey string, expires time.Time) string {
	exp := strconv.FormatInt(expires.Unix(), 10)
	v := url.Values{}
	v.Set(ParamExpires, exp)
	v.Set(ParamSignature, s.mac(method, objectKey, exp))
	return v.Encode()
}

// Verify checks a signature for method+objectKey and reports whether it is
// currently valid.
func (s *LocalSigner) Verify(method, objectKey, exp, sig string) bool {
	if exp == "" || sig == "" {
		return false
	}
	unix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().After(time.Unix(unix, 0)) {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(s.mac(method, objectKey, exp)))
}

func (s *LocalSigner) mac(method, objectKey, exp string) string {
	m := hmac.New(sha256.New, s.key)
	// Unambiguous without length prefixes: a method has no newline and keys are
	// path-cleaned.
	m.Write([]byte(strings.ToUpper(method) + "\n" + strings.TrimPrefix(objectKey, "/") + "\n" + exp))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// TenantForObjectKey returns the tenant a key belongs to. Keys are minted as
// "<tenant>/<obj id>/<filename>", so the prefix is the binding.
func TenantForObjectKey(objectKey string) string {
	k := strings.TrimPrefix(objectKey, "/")
	if i := strings.IndexByte(k, '/'); i > 0 {
		return k[:i]
	}
	return ""
}
