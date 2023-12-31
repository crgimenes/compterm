package config

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/crgimenes/compterm/constants"
)

type Config struct {
	Debug     bool
	IgnorePID bool
	Listen    string
	APIListen string
	Command   string
	MOTD      string
	APIKey    string
	Path      string
	InitFile  string
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

	log.Println("CFG.Path:", CFG.Path)

	// Parse environment variables
	CFG.Listen = os.Getenv("COMPTERM_LISTEN")
	if CFG.Listen == "" {
		CFG.Listen = "0.0.0.0:2200"
	}
	CFG.APIListen = os.Getenv("COMPTERM_API_LISTEN")
	if CFG.APIListen == "" {
		CFG.APIListen = "127.0.0.1:2201"
	}
	CFG.APIKey = os.Getenv("COMPTERM_API_KEY")
	CFG.MOTD = os.Getenv("COMPTERM_MOTD")
	if CFG.MOTD == "" {
		CFG.MOTD = "Welcome to Compterm"
	}
	CFG.Debug = os.Getenv("COMPTERM_DEBUG") == "true"
	CFG.Command = os.Getenv("COMPTERM_COMMAND")
	if CFG.Command == "" {
		CFG.Command = os.Getenv("SHELL")
	}
	CFG.Path = os.Getenv("COMPTERM_PATH")
	if CFG.Path == "" {
		CFG.Path = fullpath
	}
	CFG.InitFile = os.Getenv("COMPTERM_INIT_FILE")
	if CFG.InitFile == "" {
		CFG.InitFile = "init.lua"
	}
	if CFG.IgnorePID {
		CFG.IgnorePID = os.Getenv("COMPTERM_IGNORE_PID") == "true"
	}

	// Parse command line flags
	flag.StringVar(&CFG.Listen, "listen", CFG.Listen, "")
	flag.StringVar(&CFG.APIListen, "api_listen", CFG.APIListen, "")
	flag.StringVar(&CFG.APIKey, "api_key", CFG.APIKey, "")
	flag.StringVar(&CFG.Command, "command", CFG.Command, "")
	flag.StringVar(&CFG.MOTD, "motd", CFG.MOTD, "")
	flag.StringVar(&CFG.Path, "path", CFG.Path, "")
	flag.BoolVar(&CFG.Debug, "debug", CFG.Debug, "")
	flag.StringVar(&CFG.InitFile, "init", CFG.InitFile, "")
	flag.BoolVar(&CFG.IgnorePID, "ignore_pid", CFG.IgnorePID, "")

	p := func(msg string) {
		_, _ = os.Stderr.WriteString(msg)
	}

	flag.Usage = func() {
		p("Compterm - A terminal sharing tool\n")
		p("\n")
		p("Environment variables:\n")
		p("    COMPTERM_LISTEN\n")
		p("        Listen address default: \"0.0.0.0:2200\"\n")
		p("    COMPTERM_API_LISTEN\n")
		p("        API Listen address default: \"127.0.0.1:2201\"\n")
		p("    COMPTERM_API_KEY\n")
		p("        API Key\n")
		p("    COMPTERM_COMMAND\n")
		p("        Command to run default: $SHELL\n")
		p("    COMPTERM_MOTD\n")
		p("        Message of the day\n")
		p("    COMPTERM_PATH\n")
		p("        Path to config files default: $HOME/.config/compterm\n")
		p("    COMPTERM_DEBUG\n")
		p("        Enable debug mode default: false\n")
		p("    COMPTERM_INIT_FILE\n")
		p("        Init file default: init.lua\n")
		p("    COMPTERM_IGNORE_PID\n")
		p("        Ignore PID file default: false\n")
		p("\n")
		p("Usage: compterm [options]\n")
		p("Options:\n")
		p("    -listen string\n")
		p("        Listen address default: \"0.0.0.0:2200\"\n")
		p("        Override the environment variable $COMPTERM_LISTEN\n")
		p("    -api_listen string\n")
		p("        API Listen address default: \"127.0.0.1:2201\"\n")
		p("        Override the environment variable $COMPTERM_API_LISTEN\n")
		p("    -api_key string\n")
		p("        API Key\n")
		p("        Override the environment variable $COMPTERM_API_KEY\n")
		p("    -command string\n")
		p("        Command to run default: $SHELL\n")
		p("        Override the environment variable $COMPTERM_COMMAND\n")
		p("    -motd string\n")
		p("        Message of the day\n")
		p("        Override the environment variable $COMPTERM_MOTD\n")
		p("    -path string\n")
		p("        Path to config files default: $HOME/.config/compterm\n")
		p("        Override the environment variable $COMPTERM_PATH\n")
		p("    -debug\n")
		p("        Enable debug mode default: false\n")
		p("        Override the environment variable $COMPTERM_DEBUG\n")
		p("    -init string\n")
		p("        Init file default: init.lua\n")
		p("        Override the environment variable $COMPTERM_INIT_FILE\n")
		p("    -ignore_pid\n")
		p("        Ignore PID file default: false\n")
		p("        Override the environment variable $COMPTERM_IGNORE_PID\n")
		p("\n")
		p("init.lua:\n")
		p("    The init.lua file is located at $HOME/.config/compterm/init.lua\n")
		p("    This file is used to configure the compterm and can override all other settings and command line flags except -path and -init\n")
		p("    If the file does not exist it will be created with the default content.\n")
		p("\n")

	}

	flag.Parse()

	return err
}
