package screen

import (
	"bytes"
	"testing"
)

func TestClipboardFilter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "hello world", "hello world"},
		{"csi passes", "\x1b[31mred\x1b[0m", "\x1b[31mred\x1b[0m"},
		{"osc title BEL passes", "\x1b]0;my title\x07rest", "\x1b]0;my title\x07rest"},
		{"osc title ST passes", "\x1b]2;t\x1b\\rest", "\x1b]2;t\x1b\\rest"},
		{"osc52 BEL dropped", "a\x1b]52;c;SGVsbG8=\x07b", "ab"},
		{"osc52 ST dropped", "a\x1b]52;c;SGVsbG8=\x1b\\b", "ab"},
		{"osc52 only", "\x1b]52;c;Zm9v\x07", ""},
		{"osc52 query dropped", "x\x1b]52;c;?\x07y", "xy"},
		{"non-52 osc near prefix passes", "\x1b]529;x\x07", "\x1b]529;x\x07"},
		{"esc not osc passes", "x\x1by", "x\x1by"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f clipboardFilter
			got := f.filter(nil, []byte(tt.in))
			if !bytes.Equal(got, []byte(tt.want)) {
				t.Errorf("filter(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestClipboardFilterSplit verifies an OSC 52 sequence split across writes is
// still fully removed.
func TestClipboardFilterSplit(t *testing.T) {
	var f clipboardFilter
	var out []byte
	out = f.filter(out, []byte("before\x1b]5"))
	out = f.filter(out, []byte("2;c;QUJD"))
	out = f.filter(out, []byte("\x07after"))

	if string(out) != "beforeafter" {
		t.Fatalf("split filter = %q, want %q", out, "beforeafter")
	}
}
