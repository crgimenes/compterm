package session

import (
	"crypto/rand"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"
)

const sessionTTL = 3 * time.Hour

type Control struct {
	cookieName     string
	mx             sync.Mutex
	SessionDataMap map[string]SessionData
}

type SessionData struct {
	ExpireAt      time.Time
	Authenticated bool
}

func New(cookieName string) *Control {
	return &Control{
		cookieName:     cookieName,
		SessionDataMap: make(map[string]SessionData),
	}
}

// lookup returns a copy of the session data for key, dropping it if expired.
func (c *Control) lookup(key string) (SessionData, bool) {
	c.mx.Lock()
	defer c.mx.Unlock()

	s, ok := c.SessionDataMap[key]
	if !ok {
		return SessionData{}, false
	}

	if s.ExpireAt.Before(time.Now()) {
		delete(c.SessionDataMap, key)
		return SessionData{}, false
	}

	return s, true
}

func (c *Control) Get(r *http.Request) (string, *SessionData, bool) {
	cookie, err := r.Cookie(c.cookieName)
	if err != nil {
		return "", nil, false
	}

	s, ok := c.lookup(cookie.Value)
	if !ok {
		return "", nil, false
	}

	return cookie.Value, &s, true
}

func (c *Control) Delete(w http.ResponseWriter, id string) {
	c.mx.Lock()
	delete(c.SessionDataMap, id)
	c.mx.Unlock()

	cookie := http.Cookie{
		Name:   c.cookieName,
		Value:  "",
		MaxAge: -1,
	}
	http.SetCookie(w, &cookie)
}

func (c *Control) Save(w http.ResponseWriter, r *http.Request, id string, sessionData *SessionData) {
	expireAt := time.Now().Add(sessionTTL)

	// if localhost accept all cookies (secure=false)
	secure := true
	lhost := strings.Split(r.Host, ":")[0]
	if lhost == "localhost" {
		secure = false
	}

	cookie := &http.Cookie{
		Path:     "/",
		Name:     c.cookieName,
		Value:    id,
		Expires:  expireAt,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteDefaultMode,
	}

	if sessionData == nil {
		sessionData = &SessionData{}
	}
	sessionData.ExpireAt = expireAt

	c.mx.Lock()
	c.SessionDataMap[id] = *sessionData
	c.mx.Unlock()

	http.SetCookie(w, cookie)
}

func (c *Control) Create() (string, *SessionData) {
	sessionData := &SessionData{
		ExpireAt: time.Now().Add(sessionTTL),
	}

	return RandomID(), sessionData
}

func (c *Control) RemoveExpired() {
	c.mx.Lock()
	defer c.mx.Unlock()

	now := time.Now()
	for k, v := range c.SessionDataMap {
		if v.ExpireAt.Before(now) {
			delete(c.SessionDataMap, k)
		}
	}
}

func RandomID() string {
	const (
		length  = 16
		charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	)
	lenCharset := byte(len(charset))
	b := make([]byte, length)
	_, _ = rand.Read(b)
	for i := range length {
		b[i] = charset[b[i]%lenCharset]
	}
	return string(b)
}

func (c *Control) List() map[string]SessionData {
	c.mx.Lock()
	defer c.mx.Unlock()

	return maps.Clone(c.SessionDataMap)
}
