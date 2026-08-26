//go:build dev

package assets

import "net/http"

var FS = http.Dir("./assets")

// ETags is empty in dev mode: files change on disk while the server runs, so
// a startup-computed validator would serve stale 304s.
var ETags map[string]string
