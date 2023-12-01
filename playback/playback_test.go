package playback

import (
	"compterm/config"
	"os"
	"testing"
)

func TestRec(t *testing.T) {

	config.Load()
	config.CFG.PlaybackFile = "out.csv"

	Rec(0x1, []byte("test   1\n"))
	Rec(0x2, []byte("test \n 2\n"))
	Rec(0x3, []byte("test ;   3\n"))
	Rec(0x4, []byte("test      4\n"))
	Rec(0x5, []byte("test       5\n"))

	err := Play(os.Stdout)
	if err != nil {
		t.Errorf("Error: %s", err)
	} else {
		// remove out.csv file
		os.Remove(config.CFG.PlaybackFile)
	}

}
