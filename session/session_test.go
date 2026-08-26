package session

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRemoveExpired(t *testing.T) {
	c := New("compterm")
	c.SessionDataMap["live"] = SessionData{ExpireAt: time.Now().Add(time.Hour)}
	c.SessionDataMap["dead"] = SessionData{ExpireAt: time.Now().Add(-time.Hour)}

	c.RemoveExpired()

	if _, ok := c.SessionDataMap["live"]; !ok {
		t.Error("live session was removed")
	}
	if _, ok := c.SessionDataMap["dead"]; ok {
		t.Error("expired session was not removed")
	}
}

// TestSaveCookieSecure pins the rule that broke tailnet auth once: a Secure
// cookie on a plain-http origin is discarded by browsers, so Secure must
// follow the actual transport, never be assumed.
func TestSaveCookieSecure(t *testing.T) {
	tests := []struct {
		name    string
		request func() *http.Request
		want    bool
	}{
		{
			name:    "plain http",
			request: func() *http.Request { return httptest.NewRequest("GET", "http://host.example/", nil) },
			want:    false,
		},
		{
			name:    "direct tls",
			request: func() *http.Request { return httptest.NewRequest("GET", "https://host.example/", nil) },
			want:    true,
		},
		{
			name: "https via reverse proxy",
			request: func() *http.Request {
				r := httptest.NewRequest("GET", "http://host.example/", nil)
				r.Header.Set("X-Forwarded-Proto", "https")
				return r
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New("compterm")
			id, sd := c.Create()
			w := httptest.NewRecorder()

			c.Save(w, tt.request(), id, sd)

			cookies := w.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("got %d cookies, want 1", len(cookies))
			}
			if cookies[0].Secure != tt.want {
				t.Errorf("Secure = %v, want %v", cookies[0].Secure, tt.want)
			}
		})
	}
}

// TestControlConcurrentAccess exercises the map under concurrent access; run
// with -race to verify the mutex protects it.
func TestControlConcurrentAccess(t *testing.T) {
	c := New("compterm")

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {

			id, sd := c.Create()
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)

			c.Save(w, r, id, sd)
			_, _, _ = c.Get(r)
			c.RemoveExpired()
		})
	}
	wg.Wait()
}
