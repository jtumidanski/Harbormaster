package server

import "embed"

//go:embed all:spa-dist
var spaFS embed.FS
