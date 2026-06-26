package session

import (
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
			r.Host = "localhost"

			c.Save(w, r, id, sd)
			_, _, _ = c.Get(r)
			c.RemoveExpired()
		})
	}
	wg.Wait()
}
