package playback

import (
	"compterm/config"
	"os"
	"testing"
)

func TestRec(t *testing.T) {

	config.Load()

	o := New("out.csv")
	o.OpenToAppend()

	o.Rec(0x1, []byte("test   1\n"))
	o.Rec(0x2, []byte("test \n 2\n"))
	o.Rec(0x3, []byte("test ;   3\n"))
	o.Rec(0x4, []byte("test      4\n"))
	o.Rec(0x5, []byte("test       5\n"))

	o.Close()

	o.Open()
	err := o.Play(os.Stdout)
	if err != nil {
		t.Errorf("Error: %s", err)
	} else {
		// remove out.csv file
		os.Remove("out.csv")
	}

}
