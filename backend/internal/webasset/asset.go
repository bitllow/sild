// Package webasset serves the web drop-in (§9) from inside the binary, so any
// sild process ships it without a separate static host.
//
// demo.html is a source file, kept in sync from web/public by the web build.
//
// dist/widget.js is a COMPILED artifact and is not in version control — build it
// with `cd web && npm run build`, or `make build`, which orders the web build
// first. It is embedded through an FS over a committed directory rather than named
// directly, so the Go build does not depend on a JavaScript bundle existing:
// `go build` and `go test` work on a fresh clone, and a deployment that skipped the
// web build fails at the one route that needs it, saying so.
package webasset

import "embed"

//go:embed demo.html
var Demo []byte

// all: so the committed .gitkeep counts as a match and the pattern always resolves,
// bundle or no bundle.
//
//go:embed all:dist
var dist embed.FS

// Widget returns the built drop-in bundle. ok is false when the web build has not
// run — callers must answer with an error rather than serve empty JavaScript.
func Widget() (js []byte, ok bool) {
	b, err := dist.ReadFile("dist/widget.js")
	if err != nil || len(b) == 0 {
		return nil, false
	}
	return b, true
}
