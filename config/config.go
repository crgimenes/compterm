package config

import (
	"os"
	"path/filepath"

	"crg.eti.br/go/config"
	_ "crg.eti.br/go/config/ini"
)

type Config struct {
	Debug        bool   `json:"debug" ini:"debug" cfg:"debug" cfgDefault:"false" cfgHelper:"Enable debug mode"`
	Listen       string `json:"listen" ini:"listen" cfg:"listen" cfgDefault:"0.0.0.0:2200" cfgHelper:"Listen address"`
	Command      string `json:"command" ini:"command" cfg:"c" cfgHelper:"Command to run default: $SHELL"`
	MOTD         string `json:"motd" ini:"motd" cfg:"motd" cfgHelper:"Message of the day"`
	PlaybackFile string `json:"playback_file" ini:"playback_file" cfg:"playback_file" cfgDefault:"out.csv" cfgHelper:"Playback file"`
}

var CFG *Config

func Load() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	homedir := home + "/.config/compterm"
	// full path do arquivo
	fullpath, err := filepath.Abs(homedir)
	if err != nil {
		return err
	}

	// cria o diretório
	err = os.MkdirAll(fullpath, 0750)
	if err != nil {
		return err
	}

	CFG = &Config{}
	config.PrefixEnv = "COMPTERM"
	config.File = fullpath + "/config.ini"
	err = config.Parse(CFG)
	if err != nil {
		return err
	}

	if CFG.Command == "" {
		CFG.Command = os.Getenv("SHELL")
	}

	return err
}
