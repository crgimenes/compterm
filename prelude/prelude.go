package prelude

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/crgimenes/compterm/config"
)

var (
	GitTag string

	ErrorUnauthorized       = errors.New("Unauthorized")
	ErrorMethodNotAllowed   = errors.New("Method Not Allowed")
	ErrorInternalServer     = errors.New("Internal Server Error")
	ErrorWrongPaymentMethod = errors.New("wrong payment method")
	ErrorInvalidData        = errors.New("invalid data")
)

func RErrorMethodNotAllowed(w http.ResponseWriter) {
	w.WriteHeader(http.StatusMethodNotAllowed)
	_, _ = w.Write([]byte(`{"error": "method not allowed"}`))
}

func RErrorBadRequest(w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`{"error": "bad request"}`))
}

func RErrorInternalServer(w http.ResponseWriter) {
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(`{"error": "internal server error"}`))
}

func RErrorUnauthorized(w http.ResponseWriter) {
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error": "unauthorized"}`))
}

func RErrorNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"error": "not found"}`))
}

type RequestData struct {
	Method  string
	URL     string
	Query   string
	Body    string
	Headers map[string][]string
	Form    map[string][]string
}

func GetParameters(prefix string, r *http.Request) []string {
	path := strings.TrimPrefix(r.URL.Path, prefix)
	path = strings.TrimSuffix(path, "/")
	path = strings.TrimSpace(path)
	a := strings.Split(path, "/")

	b := make([]string, len(a))
	i := 0

	for _, v := range a {
		if v != "" {
			b[i] = v
			i++
		}
	}

	return b[:i]
}

func Prepare(w http.ResponseWriter, r *http.Request, methods []string, chkAuth bool) (*RequestData, error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-API-Version", GitTag)

	methodAllowed := false
	for _, m := range methods {
		if r.Method == m {
			methodAllowed = true
			break
		}
	}

	if chkAuth {
		key := r.Header.Get("X-API-Key")
		if key != config.CFG.APIKey {
			RErrorUnauthorized(w)
			return nil, ErrorUnauthorized
		}
	}

	if !methodAllowed {
		RErrorMethodNotAllowed(w)
		return nil, ErrorMethodNotAllowed
	}

	// TODO: session management

	b, err := io.ReadAll(r.Body)
	if err != nil {
		RErrorBadRequest(w)
		return nil, ErrorInvalidData
	}

	ret := &RequestData{
		Method:  r.Method,
		URL:     r.URL.String(),
		Query:   r.URL.RawQuery,
		Body:    string(b),
		Headers: r.Header,
		Form:    r.Form,
	}

	return ret, nil
}
