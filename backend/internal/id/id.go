// Package id generates sortable, prefixed identifiers for platform-owned rows.
//
// IDs are ULIDs (lexicographically sortable by creation time) with a short
// type prefix, e.g. "c_01J9Z3...". Sortability is load-bearing: cursor
// pagination (?before=/?after=) and the monotonic read-receipt guard
// (GREATEST) both rely on string ordering matching chronological order.
package id

import (
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// Type prefixes for each platform-owned entity. Host-owned ids
// (external_user_id) live in a separate namespace and are not generated here.
const (
	Conversation = "c"
	Message      = "m"
	Member       = "mem"
	Assignment   = "as"
	Tenant       = "t"
	APIKey       = "key"
	AdminUser    = "adm"
	Webhook      = "wh"
	Delivery     = "whd"
	Attachment   = "att"
	Upload       = "up"
	PushToken    = "pt"
	ReadReceipt  = "rr"
	Outbox       = "evt"
)

// New returns a fresh prefixed ULID, e.g. New(Conversation) -> "c_01J9...".
func New(prefix string) string {
	return prefix + "_" + ulid.Make().String()
}

// MinForTime returns the smallest prefixed id at time t (zero entropy). Useful
// as a cursor cutoff: any id > MinForTime(t) was created at/after t. Drives
// archival idle eligibility without a separate timestamp column.
func MinForTime(prefix string, t time.Time) string {
	u := ulid.ULID{}
	u.SetTime(ulid.Timestamp(t)) // entropy left zero → minimum at this ms
	return prefix + "_" + u.String()
}

// Prefix reports the type prefix of an id ("" if it has none).
func Prefix(s string) string {
	if i := strings.IndexByte(s, '_'); i > 0 {
		return s[:i]
	}
	return ""
}
