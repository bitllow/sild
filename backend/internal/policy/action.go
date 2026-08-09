// Package policy is the single authorization decision point. Decisions are
// attribute-based: principal kind, role, peer_access, membership, assignment
// existence, conversation kind and status. It imports neither middleware nor
// store — see internal/principal.
package policy

import (
	"slices"

	"github.com/bitllow/sild/backend/internal/store/models"
)

// Action names something a caller may attempt. The set is API surface.
type Action string

const (
	ConversationsList          Action = "conversations.list"
	ConversationsRead          Action = "conversations.read"
	ConversationsCreateSupport Action = "conversations.create_support"
	ConversationsCreatePeer    Action = "conversations.create_peer"
	ConversationsClose         Action = "conversations.close"

	MembersManage Action = "members.manage"
	MembersRemap  Action = "members.remap"

	MessagesRead Action = "messages.read"
	MessagesSend Action = "messages.send"

	ReceiptsWrite Action = "receipts.write"
	TypingWrite   Action = "typing.write"

	AssignmentsCreate Action = "assignments.create"
	AssignmentsClaim  Action = "assignments.claim"
	AssignmentsClose  Action = "assignments.close"

	ContactsList Action = "contacts.list"
	ContactsRead Action = "contacts.read"
	// ContactsWrite stores a person's profile. Agents are excluded: no agent can
	// write a profile today, and granting it would give the inbox and the device a
	// field to fight over.
	ContactsWrite Action = "contacts.write"

	UploadsIssue Action = "uploads.issue"
	UploadsWrite Action = "uploads.write"
	UploadsRead  Action = "uploads.read"

	TokensMint    Action = "tokens.mint"
	RealtimeToken Action = "realtime.token"
	PrincipalRead Action = "principal.read"

	PushTokensManage Action = "push_tokens.manage"
	// PushConfigManage is separate from SettingsWrite, and owner-only: the
	// credential it accepts confers the ability to notify every one of the
	// tenant's users.
	PushConfigManage Action = "push_config.manage"
	// PushRecipientsManage is the host backend acting for one of its users —
	// suppressing nudges or dropping their devices.
	PushRecipientsManage Action = "push_recipients.manage"

	// Separate from BrandsRead: reachable with no credential at all.
	BrandsReadActive Action = "brands.read_active"
	BrandsRead       Action = "brands.read"
	BrandsWrite      Action = "brands.write"

	SettingsRead  Action = "settings.read"
	SettingsWrite Action = "settings.write"

	APIKeysManage          Action = "api_keys.manage"
	WebhooksManage         Action = "webhooks.manage"
	WebhooksReadDeliveries Action = "webhooks.read_deliveries"
	TeamManage             Action = "team.manage"

	// TranslationsFetch is the client read of a published bundle — every
	// principal that renders Sild's own strings.
	TranslationsFetch   Action = "translations.fetch"
	TranslationsRead    Action = "translations.read"
	TranslationsWrite   Action = "translations.write"
	TranslationsPublish Action = "translations.publish"
	// TranslationsManage covers the project's locales and translator scopes.
	TranslationsManage Action = "translations.manage"
)

// grant lists which principals hold an action — the only place roles map to
// capabilities.
type grant struct {
	apiKey     bool
	user       bool
	admin      bool // admin session: owner, admin or agent
	adminPriv  bool // admin session, owner/admin only
	owner      bool // admin session, owner only
	signed     bool // signed upload capability
	translator bool // admin session, translator only
}

var capabilities = map[Action]grant{
	ConversationsList:          {apiKey: true, user: true, admin: true},
	ConversationsRead:          {apiKey: true, user: true, admin: true},
	ConversationsCreateSupport: {apiKey: true, user: true, admin: true},
	// Server-to-server only: a peer conversation is visible to every peer_access
	// operator.
	ConversationsCreatePeer: {apiKey: true},
	ConversationsClose:      {apiKey: true, admin: true},

	MembersManage: {apiKey: true},
	MembersRemap:  {apiKey: true},

	MessagesRead: {apiKey: true, user: true, admin: true},
	MessagesSend: {apiKey: true, user: true, admin: true},

	ReceiptsWrite: {apiKey: true, user: true, admin: true},
	TypingWrite:   {apiKey: true, user: true, admin: true},

	AssignmentsCreate: {apiKey: true, admin: true},
	AssignmentsClaim:  {admin: true},
	AssignmentsClose:  {admin: true},

	ContactsList:  {admin: true},
	ContactsRead:  {admin: true},
	ContactsWrite: {apiKey: true, user: true},

	UploadsIssue: {apiKey: true, user: true, admin: true},
	UploadsWrite: {signed: true},
	UploadsRead:  {signed: true},

	TokensMint:    {apiKey: true},
	RealtimeToken: {admin: true},
	PrincipalRead: {apiKey: true, user: true, admin: true, translator: true},

	PushTokensManage:     {user: true},
	PushConfigManage:     {owner: true},
	PushRecipientsManage: {apiKey: true},

	BrandsReadActive: {apiKey: true, user: true, admin: true},
	BrandsRead:       {adminPriv: true},
	BrandsWrite:      {adminPriv: true},

	SettingsRead:  {adminPriv: true},
	SettingsWrite: {adminPriv: true},

	APIKeysManage:          {adminPriv: true},
	WebhooksManage:         {adminPriv: true},
	WebhooksReadDeliveries: {adminPriv: true},
	TeamManage:             {adminPriv: true},

	TranslationsFetch:   {apiKey: true, user: true, admin: true},
	TranslationsRead:    {admin: true, translator: true},
	TranslationsWrite:   {adminPriv: true, translator: true},
	TranslationsPublish: {adminPriv: true},
	TranslationsManage:  {adminPriv: true},
}

// TranslatorActions is the translator role's whole capability set, so a test can
// assert it has not widened.
func TranslatorActions() []Action {
	out := []Action{}
	for a, g := range capabilities {
		if g.translator {
			out = append(out, a)
		}
	}
	slices.Sort(out)
	return out
}

// Actions returns every declared action, so the route manifest can be checked
// against the catalog.
func Actions() []Action { return sortedActions() }

// adminRoles is every platform role, narrowest first.
var adminRoles = []models.PlatformRole{
	models.PlatformOwner, models.PlatformAdmin, models.PlatformAgent, models.PlatformTranslator,
}

// AdminRoles returns the platform roles carrying the action, so a route's role
// guard is derived from its declaration rather than assigned beside it.
func AdminRoles(a Action) []models.PlatformRole {
	g, ok := capabilities[a]
	if !ok {
		return nil
	}
	out := make([]models.PlatformRole, 0, len(adminRoles))
	for _, r := range adminRoles {
		if roleHolds(g, r) {
			out = append(out, r)
		}
	}
	return out
}

// AdminOnly reports that no other kind of principal holds the action, so a role
// guard may be mounted without refusing a credential the route admits.
func AdminOnly(a Action) bool {
	g, ok := capabilities[a]
	return ok && !g.apiKey && !g.user && !g.signed
}

// EveryAdminRole is the full role set, so a caller can tell "all of them" from a
// narrowed set without knowing the list.
func EveryAdminRole() []models.PlatformRole { return slices.Clone(adminRoles) }

// roleHolds answers for one platform role. Owner wins over adminPriv, and the
// translator's bit stands alone: a capability added later stays out of reach
// until someone lists it.
func roleHolds(g grant, r models.PlatformRole) bool {
	if r == models.PlatformTranslator {
		return g.translator
	}
	if g.owner {
		return r == models.PlatformOwner
	}
	if g.adminPriv {
		return r == models.PlatformOwner || r == models.PlatformAdmin
	}
	return g.admin
}

func sortedActions() []Action {
	out := make([]Action, 0, len(capabilities))
	for a := range capabilities {
		out = append(out, a)
	}
	slices.Sort(out)
	return out
}
