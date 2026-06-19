package config

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"unicode"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/filo"
)

type Config struct {
	IgnorePID bool
	Listen    string
	Command   string
	AuthToken string
	Term      string
	ColorTerm string
	Path      string
	InitFile  string
}

var CFG = &Config{}

// Default values used when neither the environment, command-line flags, nor
// the Filo configuration file provide one.
const (
	defaultListen   = "0.0.0.0:2200"
	defaultInitFile = "init.filo"
	// defaultTerm normalizes the shared session to a widely supported terminal
	// type so colors render consistently in the browser. Empty inherits the
	// host's TERM (not recommended: breaks colors outside tmux).
	defaultTerm = "xterm-256color"
	// defaultColorTerm advertises 24-bit color to programs in the shared
	// session (xterm.js renders truecolor correctly). Empty disables it.
	defaultColorTerm = "truecolor"
)

// defaultInitFilo is written to the configuration directory on first run. It
// documents every key and leaves the overrides commented out so that, out of
// the box, environment variables and command-line flags keep working.
const defaultInitFilo = `;; init.filo — compterm configuration
;;
;; Executed at startup. Each (set Key value) overrides the built-in default,
;; the matching environment variable, and the command-line flag.
;;
;; Booleans are #t (true) and #f (false). Uncomment and edit as needed:
;;
;; (set Listen "0.0.0.0:2200") ; web/websocket listen address
;; (set AuthToken "")          ; viewer access token (empty disables auth)
;; (set Command "/bin/zsh")    ; command to share (defaults to $SHELL)
;; (set Term "xterm-256color") ; TERM for the shared command (empty = inherit)
;; (set ColorTerm "truecolor") ; COLORTERM (empty disables 24-bit color)
;; (set IgnorePID #f)          ; ignore the COMPTERM pid guard
;;
;; getEnv reads an environment variable, falling back to the second argument:
;; (set AuthToken (getEnv "COMPTERM_AUTH_TOKEN" ""))
`

// Load resolves the configuration from defaults, environment variables,
// command-line flags, and finally the Filo configuration file (which takes
// precedence). The resulting values are validated before returning.
func Load() error {
	if err := applyDefaultsAndEnv(CFG); err != nil {
		return err
	}

	parseFlags(CFG)

	if err := loadFilo(CFG); err != nil {
		return err
	}

	return validate(CFG)
}

func applyDefaultsAndEnv(c *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	defaultPath, err := filepath.Abs(filepath.Join(home, ".config", "compterm"))
	if err != nil {
		return err
	}

	c.Listen = envOr("COMPTERM_LISTEN", defaultListen)
	c.AuthToken = os.Getenv("COMPTERM_AUTH_TOKEN")
	c.Command = envOr("COMPTERM_COMMAND", os.Getenv("SHELL"))
	c.Term = envOr("COMPTERM_TERM", defaultTerm)
	c.ColorTerm = envOr("COMPTERM_COLORTERM", defaultColorTerm)
	c.Path = envOr("COMPTERM_PATH", defaultPath)
	c.InitFile = envOr("COMPTERM_INIT_FILE", defaultInitFile)
	c.IgnorePID = os.Getenv("COMPTERM_IGNORE_PID") == "true"

	return nil
}

func parseFlags(c *Config) {
	flag.StringVar(&c.Listen, "listen", c.Listen, "web/websocket listen address")
	flag.StringVar(&c.AuthToken, "auth_token", c.AuthToken, "viewer access token (empty disables authentication)")
	flag.StringVar(&c.Command, "command", c.Command, "command to share (defaults to $SHELL)")
	flag.StringVar(&c.Term, "term", c.Term, "TERM for the shared command (empty inherits the host's)")
	flag.StringVar(&c.ColorTerm, "colorterm", c.ColorTerm, "COLORTERM for the shared command (empty disables truecolor)")
	flag.StringVar(&c.Path, "path", c.Path, "path to configuration files")
	flag.StringVar(&c.InitFile, "init", c.InitFile, "configuration file name")
	flag.BoolVar(&c.IgnorePID, "ignore_pid", c.IgnorePID, "ignore the COMPTERM pid guard")

	flag.Usage = usage
	flag.Parse()
}

// loadFilo seeds the current configuration as Filo globals, evaluates the
// configuration file, and reads the overridable values back. Path and InitFile
// are intentionally not read back because they locate the file itself.
func loadFilo(c *Config) error {
	if err := os.MkdirAll(c.Path, constants.DefaultDirMode); err != nil {
		return err
	}

	path, err := resolveInitFile(c)
	if err != nil {
		return err
	}

	src, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- path comes from operator-controlled config
	if err != nil {
		return fmt.Errorf("reading config %q: %w", path, err)
	}

	f := filo.New()
	defer f.Close()

	f.SetGlobal("Listen", c.Listen)
	f.SetGlobal("AuthToken", c.AuthToken)
	f.SetGlobal("Command", c.Command)
	f.SetGlobal("Term", c.Term)
	f.SetGlobal("ColorTerm", c.ColorTerm)
	f.SetGlobal("IgnorePID", c.IgnorePID)
	f.SetGlobal("Path", c.Path)
	f.SetGlobal("InitFile", c.InitFile)

	if err := f.RegisterBuiltin("getEnv", builtinGetEnv); err != nil {
		return err
	}

	// A file with only comments and whitespace has nothing to evaluate; the
	// seeded globals already hold the effective configuration.
	if hasCode(string(src)) {
		if err := f.DoString(string(src)); err != nil {
			return fmt.Errorf("evaluating config %q: %w", path, err)
		}
	}

	c.Listen = filoString(f, "Listen", c.Listen)
	c.AuthToken = filoString(f, "AuthToken", c.AuthToken)
	c.Command = filoString(f, "Command", c.Command)
	c.Term = filoString(f, "Term", c.Term)
	c.ColorTerm = filoString(f, "ColorTerm", c.ColorTerm)
	c.IgnorePID = filoBool(f, "IgnorePID", c.IgnorePID)

	return nil
}

// resolveInitFile returns the configuration file to evaluate, preferring a
// local file in the working directory and falling back to the configuration
// directory, which is seeded with a default file on first run.
func resolveInitFile(c *Config) (string, error) {
	if _, err := os.Stat(c.InitFile); err == nil {
		return c.InitFile, nil
	}

	fallback := filepath.Join(c.Path, c.InitFile)
	_, err := os.Stat(fallback)
	if err == nil {
		return fallback, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	err = os.WriteFile(fallback, []byte(defaultInitFilo), constants.DefaultFileMode)
	if err != nil {
		return "", fmt.Errorf("creating default config %q: %w", fallback, err)
	}
	return fallback, nil
}

// builtinGetEnv exposes (getEnv "NAME" "fallback") to the configuration file.
func builtinGetEnv(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 2 {
		return filo.Value{}, fmt.Errorf("getEnv expects 2 arguments (name, fallback)")
	}

	name, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("getEnv: first argument must be a string: %w", err)
	}

	fallback, err := args[1].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("getEnv: second argument must be a string: %w", err)
	}

	if v := os.Getenv(name); v != "" {
		return filo.VString(v), nil
	}
	return filo.VString(fallback), nil
}

func validate(c *Config) error {
	if c.Listen == "" {
		return errors.New("listen address must not be empty")
	}
	if c.Command == "" {
		return errors.New("command must not be empty (set $SHELL or COMPTERM_COMMAND)")
	}
	if c.Path == "" {
		return errors.New("config path must not be empty")
	}
	if c.InitFile == "" {
		return errors.New("init file must not be empty")
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// hasCode reports whether src contains any executable token, i.e. anything
// other than whitespace and ; line comments.
func hasCode(src string) bool {
	inComment := false
	for _, r := range src {
		switch {
		case inComment:
			if r == '\n' {
				inComment = false
			}
		case r == ';':
			inComment = true
		case !unicode.IsSpace(r):
			return true
		}
	}
	return false
}

func filoString(f *filo.Filo, name, fallback string) string {
	v, err := f.GetString(name)
	if err != nil {
		log.Printf("config: %v; keeping %q", err, fallback)
		return fallback
	}
	return v
}

func filoBool(f *filo.Filo, name string, fallback bool) bool {
	v, err := f.GetBool(name)
	if err != nil {
		log.Printf("config: %v; keeping %v", err, fallback)
		return fallback
	}
	return v
}

func usage() {
	p := func(msg string) {
		_, _ = os.Stderr.WriteString(msg)
	}

	p("Compterm - A terminal sharing tool\n\n")
	p("Usage: compterm [options]\n\n")
	p("Options:\n")
	flag.PrintDefaults()
	p("\nEnvironment variables (override defaults, overridden by flags and the config file):\n")
	p("    COMPTERM_LISTEN, COMPTERM_AUTH_TOKEN, COMPTERM_COMMAND, COMPTERM_TERM,\n")
	p("    COMPTERM_COLORTERM, COMPTERM_PATH, COMPTERM_INIT_FILE, COMPTERM_IGNORE_PID\n")
	p("\nConfiguration file (Filo):\n")
	p("    Looked up at ./init.filo, then $COMPTERM_PATH/init.filo.\n")
	p("    Overrides every other setting except -path and -init.\n")
	p("    Created with a documented default on first run.\n")
}
