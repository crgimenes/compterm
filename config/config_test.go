package config

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestConfig(path string) *Config {
	return &Config{
		Listen:    defaultListen,
		APIListen: defaultAPIListen,
		Command:   "/bin/sh",
		MOTD:      defaultMOTD,
		Path:      path,
		InitFile:  "init.filo",
	}
}

func TestLoadFilo(t *testing.T) {
	tests := []struct {
		name   string
		script string
		env    map[string]string
		check  func(*testing.T, *Config)
	}{
		{
			name:   "override string and bool",
			script: "(set Listen \"127.0.0.1:9999\")\n(set Debug #t)\n",
			check: func(t *testing.T, c *Config) {
				if c.Listen != "127.0.0.1:9999" {
					t.Errorf("Listen = %q, want 127.0.0.1:9999", c.Listen)
				}
				if !c.Debug {
					t.Errorf("Debug = false, want true")
				}
			},
		},
		{
			name:   "getEnv falls back when unset",
			script: "(set MOTD (getEnv \"COMPTERM_TEST_MOTD\" \"fallback motd\"))\n",
			check: func(t *testing.T, c *Config) {
				if c.MOTD != "fallback motd" {
					t.Errorf("MOTD = %q, want fallback motd", c.MOTD)
				}
			},
		},
		{
			name:   "getEnv reads the environment",
			script: "(set MOTD (getEnv \"COMPTERM_TEST_MOTD\" \"fallback\"))\n",
			env:    map[string]string{"COMPTERM_TEST_MOTD": "from env"},
			check: func(t *testing.T, c *Config) {
				if c.MOTD != "from env" {
					t.Errorf("MOTD = %q, want from env", c.MOTD)
				}
			},
		},
		{
			name:   "comments only keep seeded values",
			script: ";; nothing to see here\n",
			check: func(t *testing.T, c *Config) {
				if c.Listen != defaultListen {
					t.Errorf("Listen = %q, want %q", c.Listen, defaultListen)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			dir := t.TempDir()
			err := os.WriteFile(filepath.Join(dir, "init.filo"), []byte(tt.script), 0o600)
			if err != nil {
				t.Fatalf("writing init.filo: %v", err)
			}

			c := newTestConfig(dir)
			if err := loadFilo(c); err != nil {
				t.Fatalf("loadFilo: %v", err)
			}

			tt.check(t, c)
		})
	}
}

func TestResolveInitFileCreatesDefault(t *testing.T) {
	dir := t.TempDir()
	c := newTestConfig(dir)

	path, err := resolveInitFile(c)
	if err != nil {
		t.Fatalf("resolveInitFile: %v", err)
	}

	want := filepath.Join(dir, "init.filo")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("reading created file: %v", err)
	}
	if string(data) != defaultInitFilo {
		t.Fatalf("created file does not match the default template")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{name: "valid", mutate: func(*Config) {}},
		{name: "empty listen", mutate: func(c *Config) { c.Listen = "" }, wantErr: true},
		{name: "empty command", mutate: func(c *Config) { c.Command = "" }, wantErr: true},
		{name: "empty path", mutate: func(c *Config) { c.Path = "" }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestConfig("/tmp")
			c.InitFile = "init.filo"
			tt.mutate(c)

			err := validate(c)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
