//go:build !dev

package assets

import (
	"embed"
	"net/http"
)

//go:embed *.html *.css *.min.js *.ttf *.png *.svg
var assets embed.FS

var FS = http.FS(assets)
