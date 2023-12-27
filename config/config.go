package config

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/crgimenes/compterm/constants"
)

type Config struct {
	CFGPath   string
	Debug     bool
	Listen    string
	APIListen string
	Command   string
	MOTD      string
	APIKey    string
	Path      string
}

var CFG = &Config{}

func Load() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	homedir := home + "/.config/compterm"
	fullpath, err := filepath.Abs(homedir)
	if err != nil {
		return err
	}

	err = os.MkdirAll(fullpath, constants.DefaultDirMode)
	if err != nil {
		return err
	}

	CFG.Path = fullpath

	flag.StringVar(&CFG.Listen,
		"listen", "0.0.0.0:2200", "Listen address default: \"0.0.0.0:2200\"")
	flag.StringVar(&CFG.APIListen,
		"api_listen", "127.0.0.1:2201", "API Listen address default: \"127.0.0.1:2201")
	flag.StringVar(&CFG.APIKey,
		"api_key", "", "API Key")
	flag.StringVar(&CFG.Command,
		"command", os.Getenv("SHELL"), "Command to run default: $SHELL")
	flag.StringVar(&CFG.MOTD,
		"motd", "", "Message of the day")
	flag.BoolVar(&CFG.Debug,
		"debug", false, "Enable debug mode")

	flag.Parse()

	CFG.CFGPath = fullpath

	return err
}
