package web

import "embed"

//go:embed templates/*.gohtml
var Templates embed.FS
