package domain

import (
	"strings"
	"unicode/utf8"
)

// External user ids are host-supplied and opaque to the platform, but they are
// addressable — GET /v1/contacts/:external_user_id puts one in a path. Gin
// unescapes the path before matching, so an id containing "/" sent as %2F
// mis-routes rather than reaching a handler that could reject it.
//
// The constraint is therefore enforced where ids ENTER the system — token mint
// and membership writes — not where they are read back. Validating only on read
// would be unreachable for exactly the values it targets.
const maxExternalUserIDBytes = 128

// ValidateExternalUserID rejects ids that cannot be addressed later.
func ValidateExternalUserID(id string) error {
	if id == "" {
		return invalid("external user id is required")
	}
	if len(id) > maxExternalUserIDBytes {
		return invalid("external user id exceeds 128 bytes")
	}
	if !utf8.ValidString(id) {
		return invalid("external user id must be valid UTF-8")
	}
	if strings.ContainsRune(id, '/') {
		return invalid("external user id must not contain '/'")
	}
	if strings.TrimSpace(id) == "" {
		return invalid("external user id must not be blank")
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return invalid("external user id must not contain control characters")
		}
	}
	return nil
}
