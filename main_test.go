package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/crgimenes/compterm/config"
)

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []string
		wantErr bool
	}{
		{name: "simple", in: "/bin/zsh", want: []string{"/bin/zsh"}},
		{name: "args", in: "nvim -u NONE file.go", want: []string{"nvim", "-u", "NONE", "file.go"}},
		{name: "double quoted spaces", in: `nvim "my file.go"`, want: []string{"nvim", "my file.go"}},
		{name: "single quoted spaces", in: `sh -c 'echo a b'`, want: []string{"sh", "-c", "echo a b"}},
		{name: "escaped space", in: `nvim my\ file.go`, want: []string{"nvim", `my file.go`}},
		{name: "extra whitespace", in: "  ls   -la  ", want: []string{"ls", "-la"}},
		{name: "empty arg", in: `a "" b`, want: []string{"a", "", "b"}},
		{name: "unterminated quote", in: `nvim "oops`, wantErr: true},
		{name: "trailing backslash", in: `nvim foo\`, wantErr: true},
		{name: "empty", in: "   ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitCommand(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("splitCommand(%q) = %v, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitCommand(%q): %v", tt.in, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("splitCommand(%q) = %q, want %q", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("splitCommand(%q) = %q, want %q", tt.in, got, tt.want)
				}
			}
		})
	}
}

func TestAuthorize(t *testing.T) {
	const token = "s3cr3t"

	tests := []struct {
		name          string
		required      string
		provided      string
		sessionAuthed bool
		want          bool
	}{
		{name: "disabled allows everyone", required: "", provided: "", want: true},
		{name: "disabled ignores wrong token", required: "", provided: "nope", want: true},
		{name: "authenticated session passes", required: token, sessionAuthed: true, want: true},
		{name: "correct token passes", required: token, provided: token, want: true},
		{name: "wrong token fails", required: token, provided: "wrong", want: false},
		{name: "empty token fails when required", required: token, provided: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := authorize(tt.required, tt.provided, tt.sessionAuthed)
			if got != tt.want {
				t.Errorf("authorize(%q, %q, %v) = %v, want %v",
					tt.required, tt.provided, tt.sessionAuthed, got, tt.want)
			}
		})
	}
}

func TestLoginGateAndFlow(t *testing.T) {
	config.CFG.AuthToken = "s3cr3t"
	defer func() { config.CFG.AuthToken = "" }()

	// unauthenticated GET / shows the login page
	rec := httptest.NewRecorder()
	mainHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Access token") {
		t.Fatalf("GET / did not return the login page")
	}

	// unauthenticated /ws is rejected
	wsRec := httptest.NewRecorder()
	wsHandler(wsRec, httptest.NewRequest(http.MethodGet, "/ws", nil))
	if wsRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /ws status = %d, want 401", wsRec.Code)
	}

	// POST /login with the correct token authenticates and redirects
	form := url.Values{"token": {"s3cr3t"}}
	loginRec := httptest.NewRecorder()
	loginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginHandler(loginRec, loginReq)
	if loginRec.Code != http.StatusSeeOther {
		t.Fatalf("POST /login status = %d, want 303", loginRec.Code)
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("POST /login set no session cookie")
	}

	// the session cookie now gets past the /ws auth gate (the upgrade itself
	// fails under httptest, but a 401 would mean auth rejected it)
	wsRec2 := httptest.NewRecorder()
	wsReq2 := httptest.NewRequest(http.MethodGet, "/ws", nil)
	for _, c := range cookies {
		wsReq2.AddCookie(c)
	}
	wsHandler(wsRec2, wsReq2)
	if wsRec2.Code == http.StatusUnauthorized {
		t.Fatalf("authenticated GET /ws was rejected (401)")
	}

	// a wrong token is rejected
	badRec := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{"token": {"nope"}}.Encode()))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginHandler(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("POST /login wrong token status = %d, want 401", badRec.Code)
	}
}

// TestAssetsServed verifies the embedded assets are served and that the
// terminal CSS is vendored locally instead of pulled from a CDN.
func TestAssetsServed(t *testing.T) {
	srv := httptest.NewServer(newMux())
	defer srv.Close()

	for _, path := range []string{"/", "/term.css", "/xterm.css"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", path, resp.StatusCode)
		}

		if path != "/term.css" {
			continue
		}
		if strings.Contains(string(body), "unpkg") {
			t.Errorf("term.css still references the unpkg CDN")
		}
		if !strings.Contains(string(body), `@import "xterm.css"`) {
			t.Errorf("term.css does not import the vendored xterm.css")
		}
	}
}
