package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

// ValidatorTimeout bounds one call to a tenant's validator.
const ValidatorTimeout = 5 * time.Second

const maxValidatorBody = 64 << 10

var reasonPattern = regexp.MustCompile(`^[a-z0-9_.-]{1,64}$`)

// ValidatorError is a validator call that yielded no usable answer. Detail is
// for the log; the caller only learns that the validator failed.
type ValidatorError struct{ Detail string }

func (e *ValidatorError) Error() string { return "identity validator failed: " + e.Detail }

func validatorErr(format string, a ...any) error {
	return &ValidatorError{Detail: fmt.Sprintf(format, a...)}
}

// Verdict is a validator's answer about one integrator token.
type Verdict struct {
	Active bool
	Sub    string
	Reason string
	// Expiry is zero when the validator gave none.
	Expiry time.Time
}

// Validator asks a tenant's own validator who an integrator token belongs to
// (docs/embedded-sign-in.md).
type Validator struct{ client *http.Client }

// NewValidator builds a client that never follows redirects: the API token is
// for the configured URL only.
func NewValidator(timeout time.Duration) *Validator {
	return &Validator{client: &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Validate posts token to endpoint with apiToken as the bearer.
func (v *Validator) Validate(ctx context.Context, endpoint, apiToken, token string) (*Verdict, error) {
	body, _ := json.Marshal(map[string]string{"token": token})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, validatorErr("bad request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := v.client.Do(req)
	if err != nil {
		return nil, validatorErr("unreachable: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return nil, validatorErr("answered %d", res.StatusCode)
	}
	var out struct {
		Active *bool  `json:"active"`
		Sub    string `json:"sub"`
		Exp    *int64 `json:"exp"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxValidatorBody)).Decode(&out); err != nil {
		return nil, validatorErr("response is not the contract's JSON: %v", err)
	}
	switch {
	case out.Active == nil:
		return nil, validatorErr("response has no active field")
	case len(out.Sub) > 255:
		return nil, validatorErr("sub is longer than 255 characters")
	case *out.Active && out.Sub == "":
		return nil, validatorErr("active response has no sub")
	case out.Exp != nil && *out.Exp <= 0:
		return nil, validatorErr("exp is not unix seconds")
	}
	verdict := &Verdict{Active: *out.Active, Sub: out.Sub}
	if out.Exp != nil {
		verdict.Expiry = time.Unix(*out.Exp, 0)
	}
	if !verdict.Active {
		verdict.Reason = out.Reason
		if verdict.Reason == "" {
			verdict.Reason = "inactive"
		}
		if !reasonPattern.MatchString(verdict.Reason) {
			return nil, validatorErr("reason must match %s", reasonPattern)
		}
	}
	return verdict, nil
}
