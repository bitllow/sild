package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Version is the identity of a configuration document's current state, used as
// an ETag and as the If-Match precondition on write.
//
// Content-derived rather than a stored counter: no column, and it cannot drift
// from what was served. Compared INSIDE the write transaction, so two operators
// submitting from the same version cannot both win.
func Version(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// versionMatches reports whether an If-Match value accepts the current state.
// "" and "*" accept anything (no precondition / any current state).
func versionMatches(expect, current string) bool {
	return expect == "" || expect == "*" || expect == current
}
