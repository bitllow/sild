// Package policy is the single authorization decision point. Authorization here
// is attribute-based, not role-based: a decision depends on principal kind,
// platform role, peer_access, membership, assignment existence, conversation kind
// and status. Handlers call Authorize or Scope; they never read Role or
// PeerAccess themselves, and repositories take a ResourceScope only this package
// can construct.
//
// policy imports neither middleware nor store — see internal/principal.
package policy

import "slices"

// Action names a thing a caller may attempt. Every route declares the finite set
// of actions it can select from; the set is API surface and changes with the
// version.
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

	UploadsIssue Action = "uploads.issue"
	UploadsWrite Action = "uploads.write"
	UploadsRead  Action = "uploads.read"

	TokensMint    Action = "tokens.mint"
	RealtimeToken Action = "realtime.token"
	PrincipalRead Action = "principal.read"

	PushTokensManage Action = "push_tokens.manage"

	// BrandsReadActive is separate from BrandsRead because it is reachable with
	// no credential at all; collapsing them would give the public path an
	// operator capability.
	BrandsReadActive Action = "brands.read_active"
	BrandsRead       Action = "brands.read"
	BrandsWrite      Action = "brands.write"

	SettingsRead  Action = "settings.read"
	SettingsWrite Action = "settings.write"

	APIKeysManage          Action = "api_keys.manage"
	WebhooksManage         Action = "webhooks.manage"
	WebhooksReadDeliveries Action = "webhooks.read_deliveries"
	TeamManage             Action = "team.manage"

	EmailInbound Action = "email.inbound"
)

// grant lists which principals hold an action. This table is the only place
// roles map to capabilities.
type grant struct {
	apiKey    bool
	user      bool
	admin     bool // admin session, any platform role
	adminPriv bool // admin session, owner/admin only
	signed    bool // signed upload capability
}

var capabilities = map[Action]grant{
	ConversationsList:          {apiKey: true, user: true, admin: true},
	ConversationsRead:          {apiKey: true, user: true, admin: true},
	ConversationsCreateSupport: {apiKey: true, user: true, admin: true},
	// Peer creation stays server-to-server: a peer conversation is visible to
	// every peer_access operator, so a user must not be able to mint one.
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

	ContactsList: {admin: true},
	ContactsRead: {admin: true},

	UploadsIssue: {apiKey: true, user: true, admin: true},
	UploadsWrite: {signed: true},
	UploadsRead:  {signed: true},

	TokensMint:    {apiKey: true},
	RealtimeToken: {admin: true},
	PrincipalRead: {apiKey: true, user: true, admin: true},

	PushTokensManage: {user: true},

	BrandsReadActive: {apiKey: true, user: true, admin: true},
	BrandsRead:       {adminPriv: true},
	BrandsWrite:      {adminPriv: true},

	SettingsRead:  {adminPriv: true},
	SettingsWrite: {adminPriv: true},

	APIKeysManage:          {adminPriv: true},
	WebhooksManage:         {adminPriv: true},
	WebhooksReadDeliveries: {adminPriv: true},
	TeamManage:             {adminPriv: true},

	EmailInbound: {},
}

// Actions returns every declared action in a stable order, so tests that assert
// the catalog covers the manifest do not flake on map iteration.
func Actions() []Action { return sortedActions() }

func sortedActions() []Action {
	out := make([]Action, 0, len(capabilities))
	for a := range capabilities {
		out = append(out, a)
	}
	slices.Sort(out)
	return out
}
