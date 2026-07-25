package api

import (
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
)

// The route manifest is the declared contract for the whole surface: for every
// route, what it authorizes and who may call it.
//
// Dropping the /admin prefix moved authentication from the path to the route
// group, so nothing in a URL says what guards it any more. The manifest says it
// instead, and manifest_test.go enforces that the mounted router matches —
// making "a moved route cannot lose its guard" mechanical rather than reviewed.
type routeClass string

const (
	// classAction is authorized by policy against one of Actions.
	classAction routeClass = "action"
	// classPublic is reachable with no credential: credential acquisition, and
	// the unauthenticated brand read the drop-in needs before it has a token.
	classPublic routeClass = "public"
	// classSignedIngress is gated by a signature rather than a principal.
	classSignedIngress routeClass = "signed_ingress"
	// classInfrastructure serves no tenant data.
	classInfrastructure routeClass = "infrastructure"
)

type routeSpec struct {
	Method string
	Path   string
	Class  routeClass
	// Actions is the finite set a request may resolve to. More than one means
	// the route picks by body or credential — POST /conversations chooses
	// create_support or create_peer, PATCH /assignments/:id claim or close.
	Actions    []policy.Action
	Principals []principal.Kind
}

var (
	anyPrincipal = []principal.Kind{principal.KindAPIKey, principal.KindUser, principal.KindAdmin}
	keyOnly      = []principal.Kind{principal.KindAPIKey}
	userOnly     = []principal.Kind{principal.KindUser}
	adminOnly    = []principal.Kind{principal.KindAdmin}
	signedOnly   = []principal.Kind{principal.KindSigned}
)

var routeManifest = []routeSpec{
	// Infrastructure and well-known.
	{"GET", "/.well-known/jwks.json", classInfrastructure, nil, nil},
	{"GET", "/widget.js", classInfrastructure, nil, nil},

	// Credential acquisition — the deliberate consumer-shaped exception.
	{"GET", "/v1/admin/auth/google", classPublic, nil, nil},
	{"GET", "/v1/admin/auth/google/callback", classPublic, nil, nil},
	{"GET", "/v1/admin/auth/google/dev", classPublic, nil, nil},
	{"POST", "/v1/admin/auth/password", classPublic, nil, nil},
	{"POST", "/v1/admin/auth/logout", classPublic, nil, nil},

	// Signature-gated ingress and signed object access.
	{"POST", "/v1/email/inbound", classSignedIngress, nil, nil},
	{"PUT", "/v1/uploads/local/*objectKey", classAction, []policy.Action{policy.UploadsWrite}, signedOnly},
	{"GET", "/v1/uploads/local/*objectKey", classAction, []policy.Action{policy.UploadsRead}, signedOnly},

	// Conversations.
	{"GET", "/v1/conversations", classAction, []policy.Action{policy.ConversationsList}, anyPrincipal},
	{"POST", "/v1/conversations", classAction, []policy.Action{policy.ConversationsCreateSupport, policy.ConversationsCreatePeer}, anyPrincipal},
	{"GET", "/v1/conversations/:id", classAction, []policy.Action{policy.ConversationsRead}, anyPrincipal},
	{"GET", "/v1/conversations/:id/messages", classAction, []policy.Action{policy.MessagesRead}, anyPrincipal},
	{"POST", "/v1/conversations/:id/messages", classAction, []policy.Action{policy.MessagesSend}, anyPrincipal},
	{"POST", "/v1/conversations/:id/read", classAction, []policy.Action{policy.ReceiptsWrite}, anyPrincipal},
	{"POST", "/v1/conversations/:id/typing", classAction, []policy.Action{policy.TypingWrite}, anyPrincipal},
	{"POST", "/v1/conversations/:id/close", classAction, []policy.Action{policy.ConversationsClose}, anyPrincipal},
	{"POST", "/v1/conversations/:id/assignments", classAction, []policy.Action{policy.AssignmentsCreate}, anyPrincipal},
	{"POST", "/v1/conversations/:id/members", classAction, []policy.Action{policy.MembersManage}, keyOnly},
	{"DELETE", "/v1/conversations/:id/members/:user_id", classAction, []policy.Action{policy.MembersManage}, keyOnly},
	{"POST", "/v1/conversations/:id/members/remap", classAction, []policy.Action{policy.MembersRemap}, keyOnly},

	// Assignments, contacts, identity, uploads, tokens.
	{"PATCH", "/v1/assignments/:id", classAction, []policy.Action{policy.AssignmentsClaim, policy.AssignmentsClose}, adminOnly},
	{"GET", "/v1/contacts", classAction, []policy.Action{policy.ContactsList}, adminOnly},
	{"GET", "/v1/contacts/:external_user_id", classAction, []policy.Action{policy.ContactsRead}, adminOnly},
	{"GET", "/v1/principal", classAction, []policy.Action{policy.PrincipalRead}, anyPrincipal},
	{"GET", "/v1/realtime/token", classAction, []policy.Action{policy.RealtimeToken}, adminOnly},
	{"POST", "/v1/uploads", classAction, []policy.Action{policy.UploadsIssue}, anyPrincipal},
	{"POST", "/v1/tokens", classAction, []policy.Action{policy.TokensMint}, keyOnly},
	{"POST", "/v1/push-tokens", classAction, []policy.Action{policy.PushTokensManage}, userOnly},
	{"DELETE", "/v1/push-tokens", classAction, []policy.Action{policy.PushTokensManage}, userOnly},

	// Brands: active is optional-auth, the set is owner/admin.
	{"GET", "/v1/brands/active", classPublic, []policy.Action{policy.BrandsReadActive}, anyPrincipal},
	{"GET", "/v1/brands", classAction, []policy.Action{policy.BrandsRead}, adminOnly},
	{"PUT", "/v1/brands", classAction, []policy.Action{policy.BrandsWrite}, adminOnly},

	// Settings, owner/admin only.
	{"GET", "/v1/channels/email", classAction, []policy.Action{policy.SettingsRead}, adminOnly},
	{"PATCH", "/v1/channels/email", classAction, []policy.Action{policy.SettingsWrite}, adminOnly},
	{"GET", "/v1/api-keys", classAction, []policy.Action{policy.APIKeysManage}, adminOnly},
	{"POST", "/v1/api-keys", classAction, []policy.Action{policy.APIKeysManage}, adminOnly},
	{"DELETE", "/v1/api-keys/:id", classAction, []policy.Action{policy.APIKeysManage}, adminOnly},
	{"GET", "/v1/webhooks", classAction, []policy.Action{policy.WebhooksManage}, adminOnly},
	{"POST", "/v1/webhooks", classAction, []policy.Action{policy.WebhooksManage}, adminOnly},
	{"PATCH", "/v1/webhooks/:id", classAction, []policy.Action{policy.WebhooksManage}, adminOnly},
	{"DELETE", "/v1/webhooks/:id", classAction, []policy.Action{policy.WebhooksManage}, adminOnly},
	{"GET", "/v1/webhooks/:id/deliveries", classAction, []policy.Action{policy.WebhooksReadDeliveries}, adminOnly},
	{"GET", "/v1/team", classAction, []policy.Action{policy.TeamManage}, adminOnly},
	{"POST", "/v1/team", classAction, []policy.Action{policy.TeamManage}, adminOnly},
	{"PATCH", "/v1/team/:id", classAction, []policy.Action{policy.TeamManage}, adminOnly},
	{"POST", "/v1/team/:id/password", classAction, []policy.Action{policy.TeamManage}, adminOnly},
}

// RouteManifestKeys is the manifest as "METHOD PATH", for the contract test.
func RouteManifestKeys() []string {
	out := make([]string, 0, len(routeManifest))
	for _, r := range routeManifest {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}
