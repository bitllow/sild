# Access is a set of role assignments, each carrying a scope

A member holds any number of roles, at most one of each, and every assignment
carries a scope document the role defines: an agent assignment says whether it
reaches peer conversations, a translator assignment names projects, languages and
whether it may publish. Capabilities are the union across a member's assignments,
so a second role only ever widens. Scope is JSON, validated on write against the
role's declared dimensions and read through a typed accessor.

## Considered options

**One role per member, with richer scope**, is the smaller change: the role
column stays, `holds()` keeps its shape, and scope grows fields. It cannot express
a person who translates Latvian and also works the support queue — the case that
started this — and the workaround is a role per combination, which multiplies with
every dimension.

**Typed columns per dimension** — a `peer` boolean, a `locales` list, a
`projects` list — keeps the database honest and every query legible. It was
rejected because the dimension set is the thing that changes: a queue restriction
on agents, a project restriction on API keys, an environment restriction on
anything are all plausible next steps, and each would be a column, a migration, an
API field and a UI control. The design this work implements promises that a new
scope needs no new column, and only a document delivers that.

**A row per dimension value** normalises the JSON away and makes "who has Latvian"
an index lookup. It buys a query nobody runs — scopes are read for one member at a
time, on a request that already loaded them — and costs a join on the hot
authorization path.

## Consequences

Access has one shape everywhere. Peer access stops being a boolean that beats the
role, and the translator grant stops being a table read separately from it; both
become dimensions of the assignment that carries them. The Team screen shows the
same structure the policy enforces.

The union rule means no assignment can subtract. A restriction has to be expressed
as the absence of a grant, never as a deny — so "an admin who cannot see peer
conversations" is not expressible, and is not meant to be: peer access limits the
agent role, and an admin who should not read peer conversations should not be an
admin.

The database can no longer answer "which members may write Latvian" with a plain
`WHERE`. The scope document is opaque to SQL by design, and any such question is a
scan or an application-side filter until a dimension earns an index.

Validation is the only thing standing between a scope document and a junk drawer.
A dimension the role does not declare is refused on write, and the policy package
reads scope solely through per-role accessors, so a typo becomes a rejected
request rather than a silently ignored limit.
