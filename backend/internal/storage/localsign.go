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

// Local upload URLs are signed capabilities, not addresses. Without a signature
// the URL is a permanent bearer token: anyone holding it can re-read or overwrite
// the object forever, and any object key can be guessed.
//
// The signature binds the verb, the object key and an expiry, so a GET grant
// cannot be replayed as a PUT and neither outlives its window. The tenant is
// bound too — it is the first path segment of every object key (see
// localBucket.NewObjectKey) — so a valid signature for one tenant's key says
// nothing about another's.

// SignedParams are the query parameters carried by a signed local URL.
const (
	ParamExpires   = "exp"
	ParamSignature = "sig"
)

// LocalSigner signs and verifies local object URLs.
type LocalSigner struct{ key []byte }

// NewLocalSigner returns a signer for the configured key. An empty key gets a
// random per-process one: correct for a single dev node, wrong for a fleet,
// which is why the config documents it.
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
	// Length-prefix free: the separator can't appear in a method and object keys
	// are path-cleaned, so the concatenation is unambiguous.
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
