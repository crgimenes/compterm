package config

import (
	"os"
	"path/filepath"

	"crg.eti.br/go/config"
	_ "crg.eti.br/go/config/ini"
)

type Config struct {
	Debug   bool   `json:"debug" ini:"debug" cfg:"debug" cfgDefault:"false"`
	Listen  string `json:"listen" ini:"listen" cfg:"listen" cfgDefault:"0.0.0.0:2200"`
	Command string `json:"command" ini:"command" cfg:"c" cfgDefault:"sh"`
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
	err = os.MkdirAll(fullpath, 0755)
	if err != nil {
		return err
	}

	CFG = &Config{}
	config.PrefixEnv = "COMPTERM_"
	config.File = fullpath + "/config.ini"
	err = config.Parse(CFG)
	return err
}
