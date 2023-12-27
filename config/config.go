package config

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/luaengine"
)

type Config struct {
	CFGPath   string
	Debug     bool
	Listen    string
	APIListen string
	Command   string
	MOTD      string
	APIKey    string
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

	luaInit := fullpath + "/init.lua"
	_, err = os.Stat(luaInit)
	if err != nil && !os.IsNotExist(err) {
		log.Printf("error reading init.lua: %s\r\n", err)
		return err
	}
	if os.IsNotExist(err) {
		f, err := os.Create(luaInit)
		if err != nil {
			return err
		}
		_, err = f.WriteString(`-- init.lua
-- This file is executed when compterm starts.
-- You can use this file to load your own lua scripts.
`)
		if err != nil {
			log.Printf("error writing init.lua: %s\r\n", err)
			return err
		}
		f.Close()
	}

	err = luaengine.Startup(luaInit)
	if err != nil {
		return err
	}

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
