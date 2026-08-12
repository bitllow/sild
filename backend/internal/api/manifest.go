package api

import (
	"net/http"
	"slices"

	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
)

// The route manifest is the whole REST surface: for every route, what it
// authorizes, who may call it, what it will read and which limiter it answers to.
// Mount builds the router FROM these descriptors, so a route cannot exist without
// a declared classification — and its guard is DERIVED from the declaration
// rather than assigned beside it.
//
// Dropping the /admin prefix moved authentication off the path, so nothing in a
// URL says what protects it any more. This does.
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

// rateClass names a limiter bucket. Routes sharing a class share the bucket —
// two credential-acquisition routes with a budget each is two ways to spend one.
type rateClass string

const (
	rateNone    rateClass = ""
	rateAuth    rateClass = "auth"    // credential acquisition
	rateIngress rateClass = "ingress" // unauthenticated write ingress
)

// handlerFn is a handler as a method expression, so a descriptor names the method
// itself: `(*Handler).listConversations`.
type handlerFn func(*Handler, *gin.Context)

type routeSpec struct {
	Method string
	Path   string
	Class  routeClass
	// Actions is the finite set a request may resolve to. More than one means
	// the route picks by body or credential — POST /conversations chooses
	// create_support or create_peer, PATCH /assignments/:id claim or close.
	Actions    []policy.Action
	Principals []principal.Kind
	Handler    handlerFn
	// BodyLimit caps the request body. Zero means the JSON default; the raw
	// upload and inbound-email paths declare their own, larger cap.
	BodyLimit int64
	Rate      rateClass
	// IdempotencyField names the request field that makes a repeat safe, when one
	// exists. Conditional on purpose: `client_msg_id` is optional, so retrying
	// WITHOUT it creates a second message — a route-level "idempotent: true" would
	// promise something the server does not give. `Idempotency-Key` is not
	// implemented, and is not described here until it is.
	IdempotencyField string
	// Success is the status a satisfied request answers with. Zero means 200 —
	// declared rather than inferred from the method, because POST is 200, 201 and
	// 204 across this surface and a documented status that lies is worse than none.
	Success int
	// Enabled gates registration on deployment shape (storage backend, dev-only
	// login). Nil means always mounted.
	Enabled func(*Handler) bool
}

var (
	anyPrincipal = []principal.Kind{principal.KindAPIKey, principal.KindUser, principal.KindAdmin}
	keyOnly      = []principal.Kind{principal.KindAPIKey}
	userOnly     = []principal.Kind{principal.KindUser}
	adminOnly    = []principal.Kind{principal.KindAdmin}
	signedOnly   = []principal.Kind{principal.KindSigned}
)

// routeManifest is a function, not a var: the OpenAPI route is itself declared
// here and reads the manifest, which as a package var would be an init cycle.
func routeManifest() []routeSpec {
	return []routeSpec{
		// Infrastructure and well-known.
		{Method: "GET", Path: "/.well-known/jwks.json", Class: classInfrastructure, Handler: (*Handler).jwks},
		{Method: "GET", Path: "/widget.js", Class: classInfrastructure, Handler: (*Handler).widgetBundle},
		// Generated from this manifest, so the document cannot describe a surface the
		// router does not serve.
		{Method: "GET", Path: "/openapi.json", Class: classInfrastructure, Handler: (*Handler).serveOpenAPI},
		{Method: "GET", Path: "/docs", Class: classInfrastructure, Handler: (*Handler).serveDocs,
			Enabled: (*Handler).docsAvailable},

		// Credential acquisition — the deliberate consumer-shaped exception.
		{Method: "GET", Path: "/v1/admin/auth/google", Class: classPublic, Handler: (*Handler).adminGoogleLogin, Success: http.StatusFound},
		{Method: "GET", Path: "/v1/admin/auth/google/callback", Class: classPublic, Handler: (*Handler).adminGoogleCallback},
		{Method: "GET", Path: "/v1/admin/auth/google/dev", Class: classPublic, Handler: (*Handler).adminDevLogin,
			Enabled: (*Handler).devLoginAvailable},
		{Method: "POST", Path: "/v1/admin/auth/password", Class: classPublic, Handler: (*Handler).adminPasswordLogin, Rate: rateAuth},
		{Method: "POST", Path: "/v1/admin/auth/logout", Class: classPublic, Handler: (*Handler).adminLogout, Success: http.StatusNoContent},

		// Signature-gated ingress and signed object access.
		{Method: "POST", Path: "/v1/email/inbound", Class: classSignedIngress, Handler: (*Handler).emailInbound,
			BodyLimit: middleware.BodyLimitEmail, Rate: rateIngress},
		{Method: "PUT", Path: "/v1/uploads/local/*objectKey", Class: classAction,
			Actions: []policy.Action{policy.UploadsWrite}, Principals: signedOnly, Handler: (*Handler).localUploadPut,
			BodyLimit: middleware.BodyLimitUpload, Rate: rateIngress, Enabled: (*Handler).localStorageServed},
		{Method: "GET", Path: "/v1/uploads/local/*objectKey", Class: classAction,
			Actions: []policy.Action{policy.UploadsRead}, Principals: signedOnly, Handler: (*Handler).localUploadGet,
			Enabled: (*Handler).localStorageServed},

		// Conversations.
		{Method: "GET", Path: "/v1/conversations", Class: classAction,
			Actions: []policy.Action{policy.ConversationsList}, Principals: anyPrincipal, Handler: (*Handler).listConversations},
		{Method: "POST", Path: "/v1/conversations", Class: classAction,
			Actions:    []policy.Action{policy.ConversationsCreateSupport, policy.ConversationsCreatePeer},
			Principals: anyPrincipal, Handler: (*Handler).createConversation, Success: http.StatusCreated},
		{Method: "GET", Path: "/v1/conversations/:id", Class: classAction,
			Actions: []policy.Action{policy.ConversationsRead}, Principals: anyPrincipal, Handler: (*Handler).getConversation},
		{Method: "GET", Path: "/v1/conversations/:id/messages", Class: classAction,
			Actions: []policy.Action{policy.MessagesRead}, Principals: anyPrincipal, Handler: (*Handler).listMessages},
		{Method: "POST", Path: "/v1/conversations/:id/messages", Class: classAction,
			Actions: []policy.Action{policy.MessagesSend}, Principals: anyPrincipal, Handler: (*Handler).postMessage, Success: http.StatusCreated, IdempotencyField: "client_msg_id"},
		{Method: "POST", Path: "/v1/conversations/:id/read", Class: classAction,
			Actions: []policy.Action{policy.ReceiptsWrite}, Principals: anyPrincipal, Handler: (*Handler).markRead, Success: http.StatusNoContent},
		{Method: "POST", Path: "/v1/conversations/:id/typing", Class: classAction,
			Actions: []policy.Action{policy.TypingWrite}, Principals: anyPrincipal, Handler: (*Handler).typing, Success: http.StatusNoContent},
		{Method: "POST", Path: "/v1/conversations/:id/close", Class: classAction,
			Actions: []policy.Action{policy.ConversationsClose}, Principals: anyPrincipal, Handler: (*Handler).closeConversation},
		{Method: "POST", Path: "/v1/conversations/:id/assignments", Class: classAction,
			Actions: []policy.Action{policy.AssignmentsCreate}, Principals: anyPrincipal, Handler: (*Handler).addAssignment, Success: http.StatusCreated},
		{Method: "POST", Path: "/v1/conversations/:id/members", Class: classAction,
			Actions: []policy.Action{policy.MembersManage}, Principals: keyOnly, Handler: (*Handler).addMember, Success: http.StatusCreated},
		{Method: "DELETE", Path: "/v1/conversations/:id/members/:user_id", Class: classAction,
			Actions: []policy.Action{policy.MembersManage}, Principals: keyOnly, Handler: (*Handler).removeMember, Success: http.StatusNoContent},
		{Method: "POST", Path: "/v1/conversations/:id/members/remap", Class: classAction,
			Actions: []policy.Action{policy.MembersRemap}, Principals: keyOnly, Handler: (*Handler).remap},

		// Assignments, contacts, identity, uploads, tokens.
		{Method: "PATCH", Path: "/v1/assignments/:id", Class: classAction,
			Actions:    []policy.Action{policy.AssignmentsClaim, policy.AssignmentsClose},
			Principals: adminOnly, Handler: (*Handler).patchAssignment},
		{Method: "GET", Path: "/v1/contacts", Class: classAction,
			Actions: []policy.Action{policy.ContactsList}, Principals: adminOnly, Handler: (*Handler).listContacts},
		{Method: "GET", Path: "/v1/contacts/:external_user_id", Class: classAction,
			Actions: []policy.Action{policy.ContactsRead}, Principals: adminOnly, Handler: (*Handler).getContact},
		// Profile writes. The self-scoped route is the SDK's first call on start-up;
		// the id-scoped one is the host's own backend writing from its system of
		// record. Both replace the profile whole, and both are idempotent.
		{Method: "PUT", Path: "/v1/contacts/me", Class: classAction,
			Actions: []policy.Action{policy.ContactsWrite}, Principals: userOnly, Handler: (*Handler).putMyContact, Success: http.StatusNoContent},
		{Method: "PUT", Path: "/v1/contacts/:external_user_id", Class: classAction,
			Actions: []policy.Action{policy.ContactsWrite}, Principals: keyOnly, Handler: (*Handler).putContact, Success: http.StatusNoContent},
		{Method: "GET", Path: "/v1/principal", Class: classAction,
			Actions: []policy.Action{policy.PrincipalRead}, Principals: anyPrincipal, Handler: (*Handler).getPrincipal},
		{Method: "GET", Path: "/v1/realtime/token", Class: classAction,
			Actions: []policy.Action{policy.RealtimeToken}, Principals: adminOnly, Handler: (*Handler).realtimeToken},
		{Method: "POST", Path: "/v1/uploads", Class: classAction,
			Actions: []policy.Action{policy.UploadsIssue}, Principals: anyPrincipal, Handler: (*Handler).issueUpload, Success: http.StatusCreated},
		{Method: "POST", Path: "/v1/tokens", Class: classAction,
			Actions: []policy.Action{policy.TokensMint}, Principals: keyOnly, Handler: (*Handler).mintToken, Rate: rateAuth},
		{Method: "POST", Path: "/v1/push-tokens", Class: classAction,
			Actions: []policy.Action{policy.PushTokensManage}, Principals: userOnly, Handler: (*Handler).registerPush, Success: http.StatusCreated},
		{Method: "DELETE", Path: "/v1/push-tokens", Class: classAction,
			Actions: []policy.Action{policy.PushTokensManage}, Principals: userOnly, Handler: (*Handler).deregisterPush, Success: http.StatusNoContent},

		// Push control for the host's own backend: acting for one of its users, so a
		// server credential rather than that user's token. Addressed as the contact,
		// which is the model a person now is.
		{Method: "PUT", Path: "/v1/contacts/:external_user_id/push", Class: classAction,
			Actions: []policy.Action{policy.PushRecipientsManage}, Principals: keyOnly, Handler: (*Handler).setContactPush, Success: http.StatusNoContent},
		{Method: "DELETE", Path: "/v1/contacts/:external_user_id/push-tokens", Class: classAction,
			Actions: []policy.Action{policy.PushRecipientsManage}, Principals: keyOnly, Handler: (*Handler).deleteContactPushTokens},

		// Brands: active is optional-auth, the set is owner/admin.
		{Method: "GET", Path: "/v1/brands/active", Class: classPublic,
			Actions: []policy.Action{policy.BrandsReadActive}, Principals: anyPrincipal, Handler: (*Handler).getActiveBrand},
		{Method: "GET", Path: "/v1/brands", Class: classAction,
			Actions: []policy.Action{policy.BrandsRead}, Principals: adminOnly, Handler: (*Handler).listBrands},
		{Method: "PUT", Path: "/v1/brands", Class: classAction,
			Actions: []policy.Action{policy.BrandsWrite}, Principals: adminOnly, Handler: (*Handler).saveBrands},

		// Settings, owner/admin only.
		{Method: "GET", Path: "/v1/channels/email", Class: classAction,
			Actions: []policy.Action{policy.SettingsRead}, Principals: adminOnly, Handler: (*Handler).getEmailChannel},
		{Method: "PATCH", Path: "/v1/channels/email", Class: classAction,
			Actions: []policy.Action{policy.SettingsWrite}, Principals: adminOnly, Handler: (*Handler).updateEmailChannel},
		// Push setup, owner-only. The credential carries its own action rather than
		// SettingsWrite: supplying it confers the ability to notify every user.
		{Method: "GET", Path: "/v1/channels/push", Class: classAction,
			Actions: []policy.Action{policy.PushConfigManage}, Principals: adminOnly, Handler: (*Handler).getPushConfig},
		{Method: "PATCH", Path: "/v1/channels/push", Class: classAction,
			Actions: []policy.Action{policy.PushConfigManage}, Principals: adminOnly, Handler: (*Handler).updatePushSettings, Success: http.StatusNoContent},
		{Method: "PUT", Path: "/v1/channels/push/credential", Class: classAction,
			Actions: []policy.Action{policy.PushConfigManage}, Principals: adminOnly, Handler: (*Handler).setPushCredential, Success: http.StatusNoContent},
		{Method: "DELETE", Path: "/v1/channels/push/credential", Class: classAction,
			Actions: []policy.Action{policy.PushConfigManage}, Principals: adminOnly, Handler: (*Handler).deletePushCredential, Success: http.StatusNoContent},
		{Method: "POST", Path: "/v1/channels/push/test", Class: classAction,
			Actions: []policy.Action{policy.PushConfigManage}, Principals: adminOnly, Handler: (*Handler).testPushSend, Success: http.StatusNoContent},
		{Method: "GET", Path: "/v1/api-keys", Class: classAction,
			Actions: []policy.Action{policy.APIKeysManage}, Principals: adminOnly, Handler: (*Handler).listAPIKeys},
		{Method: "POST", Path: "/v1/api-keys", Class: classAction,
			Actions: []policy.Action{policy.APIKeysManage}, Principals: adminOnly, Handler: (*Handler).createAPIKey, Success: http.StatusCreated},
		{Method: "DELETE", Path: "/v1/api-keys/:id", Class: classAction,
			Actions: []policy.Action{policy.APIKeysManage}, Principals: adminOnly, Handler: (*Handler).revokeAPIKey, Success: http.StatusNoContent},
		{Method: "GET", Path: "/v1/webhooks", Class: classAction,
			Actions: []policy.Action{policy.WebhooksManage}, Principals: adminOnly, Handler: (*Handler).listWebhooks},
		{Method: "POST", Path: "/v1/webhooks", Class: classAction,
			Actions: []policy.Action{policy.WebhooksManage}, Principals: adminOnly, Handler: (*Handler).createWebhook, Success: http.StatusCreated},
		{Method: "PATCH", Path: "/v1/webhooks/:id", Class: classAction,
			Actions: []policy.Action{policy.WebhooksManage}, Principals: adminOnly, Handler: (*Handler).updateWebhook, Success: http.StatusNoContent},
		{Method: "DELETE", Path: "/v1/webhooks/:id", Class: classAction,
			Actions: []policy.Action{policy.WebhooksManage}, Principals: adminOnly, Handler: (*Handler).deleteWebhook, Success: http.StatusNoContent},
		{Method: "GET", Path: "/v1/webhooks/:id/deliveries", Class: classAction,
			Actions: []policy.Action{policy.WebhooksReadDeliveries}, Principals: adminOnly, Handler: (*Handler).listDeliveries},
		{Method: "GET", Path: "/v1/team", Class: classAction,
			Actions: []policy.Action{policy.TeamManage}, Principals: adminOnly, Handler: (*Handler).listTeam},
		{Method: "POST", Path: "/v1/team", Class: classAction,
			Actions: []policy.Action{policy.TeamManage}, Principals: adminOnly, Handler: (*Handler).inviteAgent, Success: http.StatusCreated},
		{Method: "GET", Path: "/v1/roles", Class: classAction,
			Actions: []policy.Action{policy.TeamManage}, Principals: adminOnly, Handler: (*Handler).listRoles},
		{Method: "POST", Path: "/v1/team/:id/roles", Class: classAction,
			Actions: []policy.Action{policy.TeamManage}, Principals: adminOnly, Handler: (*Handler).assignRole, Success: http.StatusNoContent},
		{Method: "PUT", Path: "/v1/team/:id/roles/:role", Class: classAction,
			Actions: []policy.Action{policy.TeamManage}, Principals: adminOnly, Handler: (*Handler).updateRoleScope, Success: http.StatusNoContent},
		{Method: "DELETE", Path: "/v1/team/:id/roles/:role", Class: classAction,
			Actions: []policy.Action{policy.TeamManage}, Principals: adminOnly, Handler: (*Handler).removeRole, Success: http.StatusNoContent},
		{Method: "POST", Path: "/v1/team/:id/password", Class: classAction,
			Actions: []policy.Action{policy.TeamManage}, Principals: adminOnly, Handler: (*Handler).setAgentPassword, Success: http.StatusNoContent},

		// Translations. The manifest and bundle reads are what every client polls;
		// the rest is the editor.
		{Method: "GET", Path: "/v1/translations/manifest", Class: classAction,
			Actions: []policy.Action{policy.TranslationsFetch}, Principals: anyPrincipal, Handler: (*Handler).translationManifest},
		{Method: "GET", Path: "/v1/translations/bundle", Class: classAction,
			Actions: []policy.Action{policy.TranslationsFetch}, Principals: anyPrincipal, Handler: (*Handler).translationBundle},
		{Method: "GET", Path: "/v1/translations/projects", Class: classAction,
			Actions: []policy.Action{policy.TranslationsRead}, Principals: adminOnly, Handler: (*Handler).listTranslationProjects},
		{Method: "POST", Path: "/v1/translations/projects", Class: classAction,
			Actions: []policy.Action{policy.TranslationsManage}, Principals: adminOnly, Handler: (*Handler).createTranslationProject, Success: http.StatusCreated},
		{Method: "PUT", Path: "/v1/translations/projects/:project", Class: classAction,
			Actions: []policy.Action{policy.TranslationsManage}, Principals: adminOnly, Handler: (*Handler).saveTranslationProject, Success: http.StatusNoContent},
		{Method: "DELETE", Path: "/v1/translations/projects/:project", Class: classAction,
			Actions: []policy.Action{policy.TranslationsManage}, Principals: adminOnly, Handler: (*Handler).deleteTranslationProject, Success: http.StatusNoContent},
		// A key is the tenant's declaration; the values under it are translations.
		{Method: "POST", Path: "/v1/translations/projects/:project/declarations", Class: classAction,
			Actions: []policy.Action{policy.TranslationsManage}, Principals: adminOnly, Handler: (*Handler).declareTranslationKey, Success: http.StatusNoContent},
		{Method: "DELETE", Path: "/v1/translations/projects/:project/declarations/:key", Class: classAction,
			Actions: []policy.Action{policy.TranslationsManage}, Principals: adminOnly, Handler: (*Handler).undeclareTranslationKey, Success: http.StatusNoContent},
		{Method: "GET", Path: "/v1/translations/projects/:project/keys", Class: classAction,
			Actions: []policy.Action{policy.TranslationsRead}, Principals: adminOnly, Handler: (*Handler).listTranslationKeys},
		{Method: "PUT", Path: "/v1/translations/projects/:project/keys/:key", Class: classAction,
			Actions: []policy.Action{policy.TranslationsWrite}, Principals: adminOnly, Handler: (*Handler).putTranslationKey, Success: http.StatusNoContent},
		{Method: "DELETE", Path: "/v1/translations/projects/:project/keys/:key", Class: classAction,
			Actions: []policy.Action{policy.TranslationsWrite}, Principals: adminOnly, Handler: (*Handler).deleteTranslationKey, Success: http.StatusNoContent},
		{Method: "GET", Path: "/v1/translations/projects/:project/releases", Class: classAction,
			Actions: []policy.Action{policy.TranslationsRead}, Principals: adminOnly, Handler: (*Handler).listTranslationReleases},
		{Method: "POST", Path: "/v1/translations/projects/:project/releases", Class: classAction,
			Actions: []policy.Action{policy.TranslationsPublish}, Principals: adminOnly, Handler: (*Handler).publishTranslations, Success: http.StatusCreated},
		{Method: "POST", Path: "/v1/translations/projects/:project/releases/:version/rollback", Class: classAction,
			Actions: []policy.Action{policy.TranslationsPublish}, Principals: adminOnly, Handler: (*Handler).rollbackTranslations, Success: http.StatusCreated},
	}
}

// RouteManifestKeys is the manifest as "METHOD PATH", for the contract test.
func RouteManifestKeys() []string {
	out := make([]string, 0, len(routeManifest()))
	for _, r := range routeManifest() {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}

// RouteGuard is one declared guard, exported so the conformance test can fire
// real requests at the mounted router and check the declaration holds.
type RouteGuard struct {
	Method     string
	Path       string
	Actions    []policy.Action
	Principals []principal.Kind
	// Roles is the platform roles the mounted guard admits — the dimension
	// Principals cannot express, since every admin role is principal.KindAdmin.
	// Empty means the route mounts no role guard.
	Roles []models.PlatformRole
}

// RouteSuccess is a route's declared success status, exported so a test can
// drive the route and check the document does not publish a status it never
// answers with.
type RouteSuccess struct {
	Method string
	Path   string
	Status int
}

// RouteSuccessStatuses returns the declared success status of every route.
func RouteSuccessStatuses() []RouteSuccess {
	out := make([]RouteSuccess, 0, len(routeManifest()))
	for _, r := range routeManifest() {
		out = append(out, RouteSuccess{Method: r.Method, Path: r.Path, Status: r.successStatus()})
	}
	return out
}

// RouteGuards returns the action-classified routes and the guards they declare.
func RouteGuards() []RouteGuard {
	out := make([]RouteGuard, 0, len(routeManifest()))
	for _, r := range routeManifest() {
		if r.Class != classAction {
			continue
		}
		out = append(out, RouteGuard{
			Method: r.Method, Path: r.Path,
			Actions: r.Actions, Principals: r.Principals,
			Roles: r.roles(),
		})
	}
	return out
}

// roles is the platform roles the route admits: the union across its declared
// actions, since a multi-action route picks one at runtime and the handler
// narrows from there. Empty when any action is reachable by a non-admin
// credential — a role guard there would refuse a caller the route admits.
func (r routeSpec) roles() []models.PlatformRole {
	if len(r.Actions) == 0 {
		return nil
	}
	var out []models.PlatformRole
	for _, a := range r.Actions {
		if !policy.AdminOnly(a) {
			return nil
		}
		for _, role := range policy.AdminRoles(a) {
			if !slices.Contains(out, role) {
				out = append(out, role)
			}
		}
	}
	if len(out) == len(policy.EveryAdminRole()) {
		return nil // admits every role: nothing left to guard
	}
	return out
}
