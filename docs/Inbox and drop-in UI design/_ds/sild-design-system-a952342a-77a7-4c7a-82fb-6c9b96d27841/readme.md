# Sild Design System

**Sild** ("bridge" in Estonian) is a multi-tenant chat platform that bridges client, driver,
dispatcher, support, and email into one conversation primitive. This repository is its design
system: brand foundations, design tokens, reusable UI components, and high-fidelity recreations of
the product surfaces.

> Sild is an open, self-hostable take on the embedded-support / conversational-messaging category.
> The brand is **original** — built around the "bridge" idea (connecting two parties), not a clone of
> any vendor's proprietary visual identity.

---

## Product context

Sild is one untyped conversation primitive serving every case — dispatcher↔client, client↔driver,
client↔support. "Support" is not a type; it's any conversation carrying an **assignment**. The system
spans several real surfaces, of which two are recreated here as UI kits:

| Surface | Phase | What it is | UI kit |
|---|---|---|---|
| **Support inbox** | 2 | Agent-facing React web app = the assignment queue. Inbox list, conversation view, search, settings (API keys, webhooks, team). | `ui_kits/inbox/` |
| **Web drop-in widget** | 3 | Embeddable end-user chat (script tag / React / WordPress), shadow-DOM isolated. | `ui_kits/widget/` |
| Native SDKs (Swift/Kotlin) | 4 | Same API/WS contract; not recreated here (no UI chrome of its own). | — |
| Email connector | 5 | Email is just another channel into a conversation. | shown inline in inbox |

### Sources given
- `uploads/chat-platform-spec(1).md` — the full technical spec (auth, data model, REST/WS API,
  realtime, connectors, RBAC, inbox screens §8, web drop-in §9, native SDK §10, storage, archival).
- **No codebase, Figma, or existing brand assets were provided.** The visual identity, logo, color,
  type, and component styling in this system were designed from scratch to fit the product and the
  "bridge" concept. Treat them as a proposed v1 to react to, not a reproduction of an existing design.

---

## Brand idea

Sild **bridges conversations**. The personality is *trustworthy infrastructure that stays out of the
way*: precise, calm, developer-respecting, quietly Nordic. Not cute, not loud — confident and clear.
The logo mark is a single arch bridging three chat bubbles: two parties connected by a conversation.

---

## Content fundamentals

How Sild writes. Copy is **plain, direct, and low-ceremony** — it reads like a competent teammate,
not a mascot.

- **Voice:** second person for the reader ("you"), first-person plural sparingly for the platform
  ("we keep your data scoped per tenant"). Address the agent/developer as a peer.
- **Casing:** **Sentence case everywhere** — buttons, menus, headings, table headers. Never Title
  Case UI. Product nouns are lowercase unless first word ("support request", "internal note",
  "assignment", "conversation").
- **Tone:** factual and reassuring. Lead with the verb. "Send", "Claim", "Close conversation",
  "Add agent". Empty states explain the next action, not feelings.
- **Length:** terse. Button labels 1–2 words. Helper text one line. Error messages say what happened
  and what to do: *"That assignment is already closed. Open a new support request to continue."*
- **Technical honesty:** the audience includes developers. Don't dumb down API/auth language — say
  "API key", "user JWT", "tenant", "webhook secret". Show real shapes (`sild_live_…`,
  `conv:<id>:internal`).
- **No emoji** in product UI or docs. (Customers may type them in chat — render those faithfully; we
  just don't author with them.)
- **No exclamation marks** in system copy except genuine success confirmations, and even then rarely.

**Examples**
- Button: `Open support request` · `Claim` · `Close conversation` · `Revoke key`
- Empty inbox: *"No conversations in this view. New support requests land here the moment they're
  assigned."*
- Toast: *"API key created. Copy it now — it won't be shown again."*
- Internal note placeholder: *"Add an internal note (only your team sees this)…"*
- Tooltip: *"Internal notes are never delivered to the customer."*

---

## Visual foundations

The look is **clean, dense-but-breathable product UI** on cool slate neutrals with one confident
blue. Restraint is the rule; color is earned.

- **Color.** Cool Nordic slate ink (`--slate-*`, near-black `#14181F` for text) carries 90% of the
  UI. **Signal Blue** (`--blue-500 #2563FD`) is the single brand/interactive color — primary
  buttons, links, selection, the launcher, focus. A **warm coral** (`--coral-500 #FF7A45`) is the
  lone accent, used *sparingly* for unread/attention moments so it stays loud. Status uses green
  (online), amber (queued), slate (closed), red (destructive). See `tokens/colors.css`.
- **Type.** `Schibsted Grotesk` for everything UI + display (a Scandinavian grotesk — on-brand for an
  Estonian name); `JetBrains Mono` for code, API keys, IDs, technical surfaces. Tight tracking on
  headings (`-0.02em`), normal on body. Dense product text sits at 14px; body at 16px. See
  `tokens/typography.css`. *Both substituted from Google Fonts — flag to swap if Sild licenses a
  proprietary face.*
- **Spacing.** Strict 4px grid (`--space-*`). Product UI is compact (8/12/16 rhythm); marketing
  breathes (48/64/80).
- **Backgrounds.** Flat. Page is `--slate-50`, cards are white. **No gradients** in product UI (the
  one exception: the floating launcher may carry a subtle brand glow shadow). No textures, no
  photographic hero backdrops in-app. Marketing may use generous whitespace and a single product
  screenshot rather than decoration.
- **Corner radii.** Friendly but not bubbly: inputs/buttons `--radius-md (8px)`, cards/panels
  `--radius-lg (12px)`, the widget panel `--radius-xl (16px)`, chat bubbles `--radius-bubble (18px)`,
  avatars/pills/status dots `--radius-full`.
- **Cards.** White surface, `1px --border-default` hairline, `--radius-lg`, `--shadow-sm`. Elevation
  rises with float: menus `--shadow-md`, popovers `--shadow-lg`, widget `--shadow-widget`, modals
  `--shadow-xl` over a `--surface-overlay` scrim. Shadows are soft and cool-tinted; **no hard or
  black drop shadows**.
- **Borders.** Hairline `1px` dividers (`--border-subtle`/`--border-default`). Structure comes from
  borders + spacing, not heavy shadow.
- **Hover.** Surfaces shift one step toward `--surface-hover`/`--surface-active` (slate tints).
  Primary buttons darken (`--brand → --brand-hover`). Links underline. Never opacity-fade for hover.
- **Press / active.** Buttons darken one more step and use a subtle `transform: translateY(1px)` or
  `scale(0.98)` — quick, no bounce.
- **Focus.** Always visible: `--ring` (3px 32%-blue halo) on a `--border-focus` outline. Never remove
  focus styles.
- **Motion.** Quick and confident. `--duration-fast/base` with `--ease-standard`/`--ease-out`. Enters
  fade+rise a few px; exits fade. **No bounce in product UI** — the one playful exception is the
  launcher opening with `--ease-spring`. Respect `prefers-reduced-motion`. Typing indicator is three
  dots with a gentle staggered fade.
- **Transparency / blur.** Used only for overlays (modal scrim `rgba(20,24,31,.55)`) and the
  occasional sticky header backdrop-blur. Not decorative.
- **Imagery vibe.** Avatars and customer photos are full-color; UI imagery (illustrations for empty
  states) is line-based and monochrome-blue, never stocky or gradient-heavy.

---

## Iconography

- **Library: [Lucide](https://lucide.dev)** — clean 24×24, 2px round-cap stroke icons. Chosen as the
  closest open-source match to Sild's precise-but-friendly line style. **Substitution flag:** Sild has
  no proprietary icon set; Lucide is the recommended stand-in. Loaded via CDN
  (`https://unpkg.com/lucide@latest`) — see usage in the UI kits and the `IconButton` component.
- **Style rules:** 2px stroke, round caps/joins, no fills, `currentColor` so icons inherit text color.
  Default size 20px in dense UI, 24px in comfortable contexts, 16px inline with `--text-sm`.
- **No emoji as icons.** No multicolor/duotone icon sets. No mixing icon families.
- **Brand mark** lives in `assets/` as SVG (`sild-mark.svg` standalone, `sild-mark-tile.svg` on blue,
  `sild-logo.svg` lockup, `sild-logo-on-dark.svg`). The mark uses `currentColor` so it tints to any
  context.
- **Unicode** is acceptable only for true typographic glyphs (·, →, ×) inside text, never as UI icons.

---

## Index / manifest

**Root**
- `styles.css` — global entry point (consumers link this). `@import` manifest only.
- `tokens/` — `fonts.css`, `colors.css`, `typography.css`, `spacing.css`, `radius.css`,
  `shadows.css`, `motion.css`, `base.css`.
- `assets/` — logo + mark SVGs.
- `readme.md` — this file. `SKILL.md` — Agent-Skill wrapper.

**Foundations** (`guidelines/`) — specimen cards for the Design System tab (Type, Colors, Spacing, Brand).

**Components** (`components/core/`) — Button, IconButton, Input, Textarea, Select, Checkbox, Switch,
Badge, Tag, Avatar, AvatarStack, Card, Banner, Tooltip, Spinner, Dialog, plus chat-specific
MessageBubble, ConversationRow, StatusPill, ComposerBar. Each has `.jsx` + `.d.ts` + `.prompt.md` and
a directory card.

**UI kits**
- `ui_kits/inbox/` — Support inbox: login, inbox list, conversation view, settings.
- `ui_kits/widget/` — Web chat drop-in: launcher, conversation list, thread.

See each kit's `README.md` for screen breakdowns.
