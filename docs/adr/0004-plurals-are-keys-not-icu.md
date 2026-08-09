# Plurals are separate keys per CLDR category, not ICU MessageFormat

A translation is a plain string with named `{placeholders}`. Where a language
needs plural forms, each CLDR category is its own key — `.one`, `.few`, `.many`,
`.other`, and `.zero` where the language has one — and a small shared function
picks the category. Russian, Lithuanian and Latvian all need this; English's
one/other does not generalise, and Latvian has a zero form.

## Considered options

ICU MessageFormat is the standard answer and puts plural selection inside the
string. It was rejected on two counts. It needs a parser and a CLDR data set on
five runtimes — Go, TypeScript, Swift, Kotlin — where the mature implementations
differ in coverage and size. And its syntax is exactly what a non-technical
translator breaks: a contractor editing a nested `{count, plural, ...}` in a text
box produces a string that fails to parse at render time, on a device, in a
language nobody on the team reads.

## Consequences

A translation value is a dumb string everywhere, so the editor is a text box and
every import and export format can carry it. CLDR category data lives in one
shared function per platform instead of a parser.

The key list is longer, and the editing UI has to know which categories a locale
actually has — offering a Latvian translator an `.other` box and an English one a
`.few` box is how the set rots. Export to Android `<plurals>` and iOS
stringsdict reassembles the sibling keys; those formats are export-first, and
JSON is the only lossless round-trip.
