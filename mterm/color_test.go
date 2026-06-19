package mterm

import (
	"strings"
	"testing"
)

// TestSnapshotColorEncoding verifies the snapshot re-encodes colors as standard
// semicolon-separated SGR, normalizing colon-form (ITU) input. Web terminals
// render the semicolon form reliably, so this normalization protects the
// snapshot path regardless of what the host emits.
func TestSnapshotColorEncoding(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string // SGR substring expected in the snapshot
	}{
		{"fg 16", "\033[31mR", ";31"},
		{"fg 256", "\033[38;5;208mR", ";38;5;208"},
		{"truecolor semicolon", "\033[38;2;173;216;230mR", ";38;2;173;216;230"},
		{"truecolor colon with empty field", "\033[38:2::173:216:230mR", ";38;2;173;216;230"},
		{"truecolor colon", "\033[38:2:173:216:230mR", ";38;2;173;216;230"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := New(1, 3)
			if _, err := term.Write([]byte(tt.in)); err != nil {
				t.Fatalf("write: %v", err)
			}

			out := string(term.GetScreenAsAnsi())
			if !strings.Contains(out, tt.want) {
				t.Errorf("snapshot = %q, want to contain %q", out, tt.want)
			}
			if strings.Contains(out, ":") {
				t.Errorf("snapshot still contains colon-form SGR: %q", out)
			}
		})
	}
}
