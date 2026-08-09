# Sild

A multi-tenant chat platform. Tenants embed Sild in their own apps and sites so
their end users can talk to support agents and to each other, and work the
resulting conversations from an inbox.

## Language

### Conversations

**Conversation**:
A thread of messages between members, belonging to exactly one tenant. Always
of one kind — support or peer.
_Avoid_: chat, ticket

**Support conversation**:
A conversation between an end user and the tenant's agents. Carries an
assignment.

**Peer conversation**:
A conversation between end users. No agent, no assignment.

**Member**:
A participant in a conversation — a user, an agent, an email participant, or a
bot.
_Avoid_: participant, subscriber

**User**:
An end user of the tenant's product, identified by the tenant's own external
user id.
_Avoid_: customer, account

**Contact**:
A person the tenant has talked to, and the profile the tenant keeps on them —
one per external user id, shared by every conversation they are in. Only
becomes visible through membership: a profile alone is not someone the tenant
has talked to.
_Avoid_: customer, profile, lead

**Agent**:
A person working the tenant's inbox.
_Avoid_: operator, admin, staff

**Internal note**:
A message visible only to agents. Never leaves the agent channels — not
webhooked, not emailed, not pushed.

**Brand**:
One named messenger appearance for a tenant. Names the tenant in anything an
end user sees, including notifications about support conversations.

### Push

**Push token**:
An identifier the platform's push service issues for one installation of one
app on one device, held against the member signed in when it was issued. A
member may hold many; a token belongs to one member.
_Avoid_: device token, registration token, FCM token

**Push credential**:
The secret a tenant supplies authorising Sild to ask that tenant's own push
project to deliver notifications to that tenant's app. One credential covers
every platform the app ships on; the platform-specific keys stay inside the
tenant's project and Sild never holds them.
_Avoid_: server key, service account, API key

**Nudge**:
What a device is told about a message and where to go when tapped. Composed by
Sild rather than by the device, because a device whose app is not in the
foreground displays what arrived and runs no app code.
_Avoid_: push payload, alert

**Push fan-out**:
Choosing which push tokens receive a nudge for a message, and sending it. Never
includes the sender's own devices, or members who have opted out.
_Avoid_: broadcast, dispatch

**Push opt-out**:
A standing instruction from the tenant's own systems that a member is not to be
sent nudges. Unlike removing a push token, it survives reinstallation and holds
until lifted.
_Avoid_: mute, disable, unsubscribe

### Translations

**Project**:
A tenant-owned set of keys, the locales it offers, and the people allowed to
translate them. A tenant has as many as it has things to localize.
_Avoid_: app, translation group, workspace

**Platform project**:
Sild's own strings — everything sild's surfaces render, including the text of a
nudge. Present in every tenant, authored by sild, and overridable but never
edited in place.

**Namespace**:
A prefix inside a key that groups related keys within a project. Not something
that can be owned, granted, or translated on its own.
_Avoid_: group, folder, module

**Key**:
A named slot for one piece of text in one project. Exists from the moment it is
declared, whether or not any locale has text for it.
_Avoid_: string id, msgid, token

**Source text**:
The English a key was written with. What a translator works from, and what
renders when nothing better exists.

**Translation**:
The text for one key in one locale.

**Override**:
A tenant's own text shadowing sild's for a key in the platform project. Only
the keys a tenant actually changed exist as overrides.
_Avoid_: fork, customization

**Locale**:
A language a project offers, named by its BCP-47 tag. Latvian is `lv`, Estonian
is `et`.
_Avoid_: language code, country

**Needs review**:
The state of a translation or override whose source text has changed since it
was written. It still renders — stale beats English — but it is flagged.
_Avoid_: outdated, invalid, stale

**Translator**:
Someone invited to write translations and nothing else. Sees no conversation,
contact, or message in the tenant.

**Scope**:
Which projects and locales a person's role applies to — all of them, or a named
set.
_Avoid_: permission, access level

**Release**:
A project's translations frozen at a point in time and given a version. What a
device reads; editing a translation changes nothing anyone sees until someone
publishes.
_Avoid_: version, snapshot, deploy

**Bundled defaults**:
The text shipped inside an app, used before anything is fetched and whenever a
fetch fails. Always present, never the freshest.
_Avoid_: fallback bundle, baked-in strings
