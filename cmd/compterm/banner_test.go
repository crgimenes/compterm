package main

import (
	"strings"
	"testing"
)

func TestShareURLs(t *testing.T) {
	tests := []struct {
		name   string
		listen string
		token  string
		want   []string
	}{
		{
			name:   "specific host without token",
			listen: "192.168.1.10:2200",
			want:   []string{"http://192.168.1.10:2200/"},
		},
		{
			name:   "specific host with token",
			listen: "192.168.1.10:2200",
			token:  "secret",
			want:   []string{"http://192.168.1.10:2200/?token=secret"},
		},
		{
			name:   "token is query-escaped",
			listen: "10.0.0.1:80",
			token:  "a b&c",
			want:   []string{"http://10.0.0.1:80/?token=a+b%26c"},
		},
		{
			name:   "invalid listen address",
			listen: "not-an-address",
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shareURLs(tt.listen, tt.token)
			if len(got) != len(tt.want) {
				t.Fatalf("shareURLs(%q, %q) = %v, want %v", tt.listen, tt.token, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("url[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestShareURLsWildcard(t *testing.T) {
	got := shareURLs("0.0.0.0:2200", "tok")
	if len(got) == 0 {
		t.Fatal("wildcard listen produced no URLs")
	}
	for _, u := range got {
		if !strings.HasPrefix(u, "http://") {
			t.Errorf("url %q does not start with http://", u)
		}
		if !strings.HasSuffix(u, ":2200/?token=tok") {
			t.Errorf("url %q does not end with :2200/?token=tok", u)
		}
	}
}
