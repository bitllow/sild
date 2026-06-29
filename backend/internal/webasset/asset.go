// Package webasset embeds the built web drop-in (§9) so any sild binary serves
// it self-contained — no filesystem layout assumptions, no separate static host.
//
// The artifacts are produced by `cd web && npm run build`, which copies the
// bundle here (see web/build.mjs). Committed placeholders keep `go build` working
// on a fresh clone before the web bundle has been built; the build overwrites
// them. The root `make build` runs the web build first so the binary always
// embeds the current bundle.
package webasset

import _ "embed"

//go:embed widget.js
var Widget []byte

//go:embed demo.html
var Demo []byte
