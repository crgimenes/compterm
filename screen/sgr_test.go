package screen

import (
	"bytes"
	"testing"
)

func TestSGRFilter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "hello", "hello"},
		{"semicolon truecolor unchanged", "\033[38;2;1;2;3mX", "\033[38;2;1;2;3mX"},
		{"colon truecolor (nvim form)", "\033[38:2:189:147:249mX", "\033[38;2;189;147;249mX"},
		{"colon truecolor empty colorspace", "\033[38:2::189:147:249mX", "\033[38;2;189;147;249mX"},
		{"colon bg truecolor", "\033[48:2:10:20:30mX", "\033[48;2;10;20;30mX"},
		{"colon 256", "\033[38:5:208mX", "\033[38;5;208mX"},
		{"underline color", "\033[58:2:1:2:3mX", "\033[58;2;1;2;3mX"},
		{"undercurl preserved", "\033[4:3mX", "\033[4:3mX"},
		{"mixed params", "\033[1;38:2:189:147:249;4mX", "\033[1;38;2;189;147;249;4mX"},
		{"reset", "\033[0mX", "\033[0mX"},
		{"empty sgr", "\033[mX", "\033[mX"},
		{"non-SGR CSI clear", "\033[2J", "\033[2J"},
		{"non-SGR CSI private", "\033[?25l", "\033[?25l"},
		{"esc not csi", "a\033b", "a\033b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f sgrFilter
			got := f.filter(nil, []byte(tt.in))
			if !bytes.Equal(got, []byte(tt.want)) {
				t.Errorf("filter(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestSGRFilterSplit verifies a colon color sequence split across writes is
// normalized correctly.
func TestSGRFilterSplit(t *testing.T) {
	var f sgrFilter
	var out []byte
	out = f.filter(out, []byte("x\033[38:2:1"))
	out = f.filter(out, []byte("89:147:24"))
	out = f.filter(out, []byte("9my"))

	if string(out) != "x\033[38;2;189;147;249my" {
		t.Fatalf("split = %q, want %q", out, "x\033[38;2;189;147;249my")
	}
}
