package main

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/crgimenes/compterm/config"
)

var (
	ErrorUnauthorized       = errors.New("Unauthorized")
	ErrorMethodNotAllowed   = errors.New("Method Not Allowed")
	ErrorInternalServer     = errors.New("Internal Server Error")
	ErrorWrongPaymentMethod = errors.New("wrong payment method")
	ErrorInvalidData        = errors.New("invalid data")
)

func errorMethodNotAllowed(w http.ResponseWriter) {
	w.WriteHeader(http.StatusMethodNotAllowed)
	w.Write([]byte(`{"error": "method not allowed"}`))
}

func errorBadRequest(w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(`{"error": "bad request"}`))
}

func errorInternalServer(w http.ResponseWriter) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(`{"error": "internal server error"}`))
}

func errorUnauthorized(w http.ResponseWriter) {
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error": "unauthorized"}`))
}

func errorNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`{"error": "not found"}`))
}

type RequestData struct {
	Method  string
	URL     string
	Query   string
	Body    string
	Headers map[string][]string
	Form    map[string][]string
}

func getParameters(prefix string, r *http.Request) []string {
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

func prelude(w http.ResponseWriter, r *http.Request, methods []string, chkAuth bool) (*RequestData, error) {
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
			errorUnauthorized(w)
			return nil, ErrorUnauthorized
		}
	}

	if !methodAllowed {
		errorMethodNotAllowed(w)
		return nil, ErrorMethodNotAllowed
	}

	// TODO: session management

	b, err := io.ReadAll(r.Body)
	if err != nil {
		errorBadRequest(w)
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
