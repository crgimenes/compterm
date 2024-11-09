package session

import (
	"crypto/rand"
	"net/http"
	"strings"
	"time"

	"github.com/crgimenes/compterm/sillyname"
)

type Control struct {
	cookieName     string
	SessionDataMap map[string]SessionData
}

type SessionData struct {
	ExpireAt      time.Time
	CurrentScreen int
	Nick          string
}

func New(cookieName string) *Control {
	return &Control{
		cookieName:     cookieName,
		SessionDataMap: make(map[string]SessionData),
	}
}

func (c *Control) Get(r *http.Request) (string, *SessionData, bool) {
	cookies := r.Cookies()
	if len(cookies) == 0 {
		return "", nil, false
	}

	cookie, err := r.Cookie(c.cookieName)
	if err != nil {
		return "", nil, false
	}

	s, ok := c.SessionDataMap[cookie.Value]
	if !ok {
		return "", nil, false
	}

	if s.ExpireAt.Before(time.Now()) {
		delete(c.SessionDataMap, cookie.Value)
		return "", nil, false
	}

	if s.Nick == "" {
		s.Nick = sillyname.Generate()
	}

	return cookie.Value, &s, true
}

func (c *Control) Delete(w http.ResponseWriter, id string) {
	delete(c.SessionDataMap, id)
	cookie := http.Cookie{
		Name:   c.cookieName,
		Value:  "",
		MaxAge: -1,
	}
	http.SetCookie(w, &cookie)
}

func (c *Control) Save(w http.ResponseWriter, r *http.Request, id string, sessionData *SessionData) {
	expireAt := time.Now().Add(3 * time.Hour)

	// if localhost accept all cookies (secure=false)
	var secure bool = true
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
	c.SessionDataMap[id] = *sessionData

	http.SetCookie(w, cookie)
}

func (c *Control) Create() (string, *SessionData) {
	sessionData := &SessionData{
		CurrentScreen: 0,
		Nick:          sillyname.Generate(),
		ExpireAt:      time.Now().Add(3 * time.Hour),
	}

	return RandomID(), sessionData
}

func (c *Control) RemoveExpired() {
	for k, v := range c.SessionDataMap {
		if v.ExpireAt.Before(time.Now()) {
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
	for i := 0; i < length; i++ {
		b[i] = charset[b[i]%lenCharset]
	}
	return string(b)
}

func (c *Control) List() map[string]SessionData {
	return c.SessionDataMap
}
