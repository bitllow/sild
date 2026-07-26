package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/gin-gonic/gin"
)

// OpenAPI is GENERATED from the route manifest, never maintained beside it — a
// hand-written spec drifts the moment a guard changes, which is the one thing a
// reader consults it for.
//
// What the manifest knows, the document states precisely: paths, methods, path
// parameters, which credentials are accepted, the rate bucket, the body cap, and
// the policy actions a request may resolve to. Request and response SCHEMAS are
// not in the manifest yet, so they are absent rather than guessed — add a
// Request/Response field to routeSpec and they appear here for free.

const openAPIVersion = "3.1.0"

// securitySchemeFor names the scheme a principal kind authenticates with.
var securitySchemeFor = map[principal.Kind]string{
	principal.KindAPIKey: "apiKey",
	principal.KindUser:   "userToken",
	principal.KindAdmin:  "adminSession",
	principal.KindSigned: "urlSignature",
}

func openAPISecuritySchemes() map[string]any {
	return map[string]any{
		"apiKey": map[string]any{
			"type": "http", "scheme": "bearer",
			"description": "Server-to-server API key, `sild_live_…`, as a bearer token.",
		},
		"userToken": map[string]any{
			"type": "http", "scheme": "bearer", "bearerFormat": "JWT",
			"description": "End-user JWT minted by POST /v1/tokens.",
		},
		"adminSession": map[string]any{
			"type": "apiKey", "in": "cookie", "name": "sild_admin",
			"description": "Operator session cookie set by the admin login routes.",
		},
		"urlSignature": map[string]any{
			"type": "apiKey", "in": "query", "name": "sig",
			"description": "HMAC over verb, object key and expiry. The signature IS the credential; these routes carry no principal.",
		},
	}
}

// openAPIDocument renders the manifest as an OpenAPI document.
func (h *Handler) openAPIDocument() map[string]any {
	paths := map[string]any{}
	for _, r := range routeManifest() {
		if r.Enabled != nil && !r.Enabled(h) {
			continue // don't document what this deployment does not serve
		}
		tmpl := openAPIPath(r.Path)
		item, _ := paths[tmpl].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[tmpl] = item
		}
		item[strings.ToLower(r.Method)] = r.openAPIOperation()
	}
	return map[string]any{
		"openapi": openAPIVersion,
		"info": map[string]any{
			"title":       "Sild",
			"version":     "v1",
			"description": "Paths name the data model, not the consumer: the credential decides which subset a caller sees.",
		},
		"paths": paths,
		"components": map[string]any{
			"securitySchemes": openAPISecuritySchemes(),
			"schemas":         map[string]any{"Error": openAPIErrorSchema()},
		},
	}
}

func (r routeSpec) openAPIOperation() map[string]any {
	op := map[string]any{
		"operationId":  r.operationID(),
		"x-sild-class": string(r.Class),
		"responses":    r.openAPIResponses(),
	}
	if params := openAPIPathParams(r.Path); len(params) > 0 {
		op["parameters"] = params
	}
	if len(r.Actions) > 0 {
		actions := make([]string, 0, len(r.Actions))
		for _, a := range r.Actions {
			actions = append(actions, string(a))
		}
		// A set, not one value: the merged routes select their action from the
		// request. Stated so a reader knows which decisions a call can reach.
		op["x-sild-actions"] = actions
	}
	if sec := r.openAPISecurity(); sec != nil {
		op["security"] = sec
	}
	if r.privilegedOnly() {
		op["x-sild-privileged"] = true
	}
	if r.Rate != rateNone {
		op["x-sild-rate-class"] = string(r.Rate)
	}
	if r.IdempotencyField != "" {
		// Conditional, so the document says what buys idempotence rather than
		// claiming the route has it unconditionally.
		op["x-sild-idempotent-when"] = map[string]any{
			"field":       r.IdempotencyField,
			"description": "Supply this field to make a retry return the original result; omit it and a retry creates a new record.",
		}
	}
	op["x-sild-max-request-bytes"] = r.bodyLimit()
	return op
}

// openAPISecurity lists the accepted credentials as alternatives. An empty slice
// (not nil) means "no credential required" — OpenAPI spells that `security: []`,
// and the distinction matters for the public routes.
func (r routeSpec) openAPISecurity() []map[string][]string {
	if r.Class == classInfrastructure {
		return nil
	}
	if r.Class == classPublic && len(r.Actions) == 0 {
		return []map[string][]string{} // credential acquisition
	}
	out := make([]map[string][]string, 0, len(r.Principals))
	for _, k := range r.Principals {
		if name, ok := securitySchemeFor[k]; ok {
			out = append(out, map[string][]string{name: {}})
		}
	}
	if r.Class == classSignedIngress {
		out = append(out, map[string][]string{"urlSignature": {}})
	}
	if r.Class == classPublic {
		// Optional auth: reachable with any declared credential OR none at all.
		out = append(out, map[string][]string{})
	}
	return out
}

func (r routeSpec) openAPIResponses() map[string]any {
	res := map[string]any{
		strconv.Itoa(r.successStatus()): map[string]any{"description": "Success"},
		"400":                           errorResponse("Malformed request, unknown field, or an unusable cursor"),
		"500":                           errorResponse("Internal error"),
	}
	if len(r.openAPISecurity()) > 0 {
		res["401"] = errorResponse("Missing, invalid or expired credential")
	}
	// 403 needs an action a credential can fail to carry; optional-auth and
	// signature routes have no principal to refuse on those grounds.
	if r.Class == classAction {
		res["403"] = errorResponse("Credential is valid but does not carry this action")
	}
	if r.Rate != rateNone {
		res["429"] = errorResponse("Rate limited; retry after the Retry-After interval")
	}
	if r.bodyLimit() > 0 && r.Method != http.MethodGet {
		res["413"] = errorResponse("Request body exceeds this route's limit")
	}
	return res
}

func errorResponse(desc string) map[string]any {
	return map[string]any{
		"description": desc,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{"$ref": "#/components/schemas/Error"},
			},
		},
	}
}

// openAPIErrorSchema is the one error envelope every route answers with (§4).
func openAPIErrorSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"error"},
		"properties": map[string]any{
			"error": map[string]any{
				"type":     "object",
				"required": []string{"code", "message"},
				"properties": map[string]any{
					"code":       map[string]any{"type": "string", "description": "Stable and machine-readable; SDKs branch on this, never on message."},
					"message":    map[string]any{"type": "string"},
					"request_id": map[string]any{"type": "string"},
					"fields": map[string]any{
						"type":                 "object",
						"additionalProperties": map[string]any{"type": "string"},
						"description":          "Per-field detail, so a client can point at the offending input.",
					},
				},
			},
		},
	}
}

// openAPIPath converts gin's `:id` / `*objectKey` to OpenAPI's `{id}`.
func openAPIPath(p string) string {
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		if strings.HasPrefix(seg, ":") || strings.HasPrefix(seg, "*") {
			parts[i] = "{" + seg[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

func openAPIPathParams(p string) []map[string]any {
	var out []map[string]any
	for _, seg := range strings.Split(p, "/") {
		if !strings.HasPrefix(seg, ":") && !strings.HasPrefix(seg, "*") {
			continue
		}
		param := map[string]any{
			"name": seg[1:], "in": "path", "required": true,
			"schema": map[string]any{"type": "string"},
		}
		if strings.HasPrefix(seg, "*") {
			// gin's catch-all spans slashes; the object key is a path.
			param["description"] = "Path-shaped; may contain slashes."
		}
		out = append(out, param)
	}
	return out
}

// operationID is a stable client-facing name: method + path with separators
// collapsed, e.g. "getConversationsIdMessages".
func (r routeSpec) operationID() string {
	var b strings.Builder
	b.WriteString(strings.ToLower(r.Method))
	for _, seg := range strings.Split(r.Path, "/") {
		if seg == "" || seg == "v1" {
			continue
		}
		seg = strings.TrimLeft(seg, ":*")
		for _, word := range strings.FieldsFunc(seg, func(c rune) bool { return c == '-' || c == '.' || c == '_' }) {
			b.WriteString(strings.ToUpper(word[:1]) + word[1:])
		}
	}
	return b.String()
}

// serveOpenAPI publishes the generated document.
func (h *Handler) serveOpenAPI(c *gin.Context) {
	c.JSON(http.StatusOK, h.openAPIDocument())
}
