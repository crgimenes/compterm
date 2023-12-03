//go:build !dev

package assets

import (
	"embed"
	"net/http"
)

//go:embed *.html *.css *.min.js *.ttf
var assets embed.FS

var FS = http.FS(assets)
