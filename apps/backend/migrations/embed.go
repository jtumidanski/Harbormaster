// Package migrations bundles SQL files for golang-migrate via iofs embed.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
