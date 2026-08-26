//go:build !dev

package assets

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
)

//go:embed *.html *.css *.js *.woff2 *.png *.svg
var assets embed.FS

var FS = http.FS(assets)

// ETags maps each embedded asset to a strong validator. Assets are served
// with Cache-Control: no-cache so an upgraded binary is never stale; the
// validator makes that policy cost a 304 instead of a refetch (the font alone
// is ~900KB, downloaded on every page load otherwise).
var ETags = func() map[string]string {
	m := map[string]string{}
	entries, err := assets.ReadDir(".")
	if err != nil {
		return m
	}
	for _, e := range entries {
		data, err := fs.ReadFile(assets, e.Name())
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		m[e.Name()] = `"` + hex.EncodeToString(sum[:8]) + `"`
	}
	return m
}()
