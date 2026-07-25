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
// from what was served. Compared inside the write transaction, which catches the
// case that actually happens — a stale tab saving over a newer edit.
//
// Two saves landing in the same instant can still both pass on a dialect that
// does not lock a plain read. Not defended against: it needs a row lock or a
// version column, and the cost of losing is one settings edit to redo.
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
