package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// QueryBool reads a flag from the query string. Absent is false, and a present
// parameter with no value ("?dry_run") is true — what a hand-written curl means
// by it.
func QueryBool(c *gin.Context, name string) bool {
	raw, ok := c.GetQuery(name)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "1", "true", "yes":
		return true
	}
	return false
}

// DecodeJSON is the one way a mutation reads its body.
//
// It rejects unknown fields rather than ignoring them: a client sending
// `assigneeId` instead of `assignee_actor_id` learns immediately instead of
// having the value silently dropped. It also answers 413 for an oversized body,
// which a bare ShouldBindJSON would collapse into a generic 400.
//
// Returns false having already written the response.
func DecodeJSON(c *gin.Context, dst any) bool {
	if ct := c.GetHeader("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		Error(c, http.StatusUnsupportedMediaType, "unsupported_media_type", "expected application/json")
		return false
	}

	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		failDecode(c, err)
		return false
	}
	// A second value means trailing data — the request said more than it meant.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		BadRequest(c, "body must contain exactly one JSON object")
		return false
	}
	return true
}

// DecodeJSONOptional is DecodeJSON for bodies that may legitimately be empty.
// An absent body is detected by reading, not by ContentLength, which a chunked
// request does not declare.
func DecodeJSONOptional(c *gin.Context, dst any) bool {
	probe, err := io.ReadAll(io.LimitReader(c.Request.Body, 1))
	if err != nil {
		failDecode(c, err)
		return false
	}
	if len(probe) == 0 {
		return true
	}
	c.Request.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(probe), c.Request.Body), c.Request.Body}
	return DecodeJSON(c, dst)
}

func failDecode(c *gin.Context, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		PayloadTooLarge(c, "request body exceeds the limit for this route")
		return
	}
	if field, ok := unknownField(err); ok {
		FieldError(c, http.StatusBadRequest, CodeUnknownField, "unknown field",
			map[string]string{field: "not a field of this request"})
		return
	}
	BadRequest(c, "invalid body")
}

// unknownField pulls the offending name out of encoding/json's error text, which
// is the only place it is reported.
func unknownField(err error) (string, bool) {
	const prefix = "json: unknown field "
	msg := err.Error()
	i := strings.Index(msg, prefix)
	if i < 0 {
		return "", false
	}
	return strings.Trim(msg[i+len(prefix):], `"`), true
}

// RejectFields refuses named fields the caller may not set. A shared URL must
// not imply a shared body: an API key may set a message's sender identity, a
// user or operator may not — and silently dropping those is how a client comes
// to believe it posted as someone else.
//
// Returns false having already written a 400 naming the field.
func RejectFields(c *gin.Context, present map[string]bool, notWritable ...string) bool {
	for _, f := range notWritable {
		if present[f] {
			FieldError(c, http.StatusBadRequest, CodeFieldNotWritable,
				"field is not writable with this credential",
				map[string]string{f: "not writable by this principal"})
			return false
		}
	}
	return true
}
