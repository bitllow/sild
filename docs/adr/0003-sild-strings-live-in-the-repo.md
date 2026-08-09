# Sild's own strings live in the repo, and tenant rows are overrides only

The platform project's keys, source text and translations are files in this
repo, compiled into every binary and client bundle. A tenant's database holds
only the keys it actually changed. Sild authors its own translations by running
the product against its own instance and exporting the result back into the repo
at release cut, which is the dogfooding made structural rather than aspirational.

## Considered options

Making the database the source of truth is the obvious shape for a translation
platform, and it was rejected because it makes every deployment's schema carry
data it did not author. A fresh self-hosted instance would render blank until
someone seeded it, and every release that added a key would need a data
migration to reach installations that already exist.

Copying the platform set into each tenant at provisioning — a fork rather than an
override layer — was rejected for the upgrade behaviour: keys added in a later
release would be invisible to every tenant provisioned before it, and improved
source text would never reach anyone.

## Consequences

A tenant's sild UI resolves from two layers with independent lifecycles. Source
text moves when the tenant upgrades the SDK; overrides and their translations
move when the tenant publishes. A key added by a sild release therefore appears
in English immediately and in other languages only once someone translates it.

Because source text can move underneath a translation that already exists, every
translation and override carries a hash of the source it was written against and
is flagged for review when that source changes. It keeps rendering — stale text
beats falling back to English for a tenant who bought Latvian.
