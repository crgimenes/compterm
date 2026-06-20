# Overview

Compterm is a versatile terminal sharing application designed for a variety of use cases, including educational, developmental, and nostalgic experiences. It's particularly useful for the Brazilian Golang Study Group, the Atomic Blast BBS system, and for efficient, low-bandwidth pair programming sessions.

# Installation

To use Compterm, Go developers can build the program for production using:

```bash
make
```

For development mode, which uses resources from the assets directory, use:

```bash
make dev
```

In production mode, resources are integrated into the executable itself.

# Configuration

Compterm accepts the following command-line flags:

- `-listen` string: web/websocket listen address (default `0.0.0.0:2200`)
- `-auth_token` string: viewer access token (empty disables authentication)
- `-command` string: command to share (default `$SHELL`)
- `-term` string: TERM for the shared command (default `xterm-256color`; empty inherits the host's)
- `-colorterm` string: COLORTERM for the shared command (default `truecolor`; empty disables 24-bit color)
- `-path` string: path to configuration files (default `$HOME/.config/compterm`)
- `-init` string: configuration file name (default `init.filo`)
- `-ignore_pid`: ignore the COMPTERM pid guard

It also recognizes the matching environment variables: `COMPTERM_LISTEN`,
`COMPTERM_AUTH_TOKEN`, `COMPTERM_COMMAND`, `COMPTERM_TERM`, `COMPTERM_COLORTERM`,
`COMPTERM_PATH`, `COMPTERM_INIT_FILE`, and `COMPTERM_IGNORE_PID`.

Finally, Compterm reads a [Filo](https://github.com/crgimenes/filo)
configuration file, looked up at `./init.filo` and then
`$COMPTERM_PATH/init.filo`. A documented default is created on first run. Each
`(set Key value)` overrides the corresponding setting:

```lisp
;; init.filo
(set Listen "0.0.0.0:2200")

;; Require a token to connect (empty = open). getEnv reads an
;; environment variable with a fallback:
(set AuthToken (getEnv "COMPTERM_AUTH_TOKEN" ""))
```

## Authentication

Authentication is optional. With an empty `AuthToken` (the default) anyone who
can reach the page connects immediately — convenient for open demonstrations.
Set a token (via flag, `COMPTERM_AUTH_TOKEN`, or `init.filo`) to require it:
viewers get a login page, and a shared link of the form `?token=<token>` logs
in automatically.

## Configuration Hierarchy

Defaults are overridden by environment variables, then by command-line flags,
and finally by the `init.filo` file (which takes precedence over all of them,
except for `-path` and `-init`, which locate the file itself).

# Terminal viewer

Besides the browser, a session can be watched from a terminal:

```bash
go run ./cmd/client -url ws://localhost:2200/ws
```

Use `-token` (or `$COMPTERM_AUTH_TOKEN`) when the server requires
authentication, and a `wss://` URL when connecting through a TLS reverse proxy.
Press `q` or `Ctrl-C` to quit.

# Colors

Compterm relays the host's raw terminal stream, so colors appear in the browser
exactly as the program emits them. Truecolor (24-bit RGB) is absolute and always
matches your terminal. The indexed *palette* colors (16/256), however, are
resolved against each terminal's own palette, so a program that uses them can
look different in the browser than locally.

If colors differ, enable truecolor in the program. For Neovim:

```lua
vim.opt.termguicolors = true -- init.lua
```

Compterm already advertises truecolor to the shared session
(`COLORTERM=truecolor`) so most programs pick it up automatically.

To make the viewer's palette match your terminal, drop a `theme.json` in the
configuration directory (`$COMPTERM_PATH/theme.json`) with any
[xterm.js theme](https://xtermjs.org/docs/api/terminal/interfaces/itheme/)
fields:

```json
{ "background": "#1e1e2e", "foreground": "#cdd6f4", "red": "#f38ba8" }
```

# Inline images

The browser viewer understands the iTerm2 inline-image escape (`OSC 1337 ;
File=`), so a program in the shared session can draw a picture straight into the
terminal:

```bash
printf '\033]1337;File=inline=1:%s\a\n' "$(base64 < image.png)"
```

The image is auto-sized from its own dimensions, scaled by `1 / devicePixelRatio`
by default (one screen pixel per image pixel, as iTerm2 does on a Retina
display). Because the browser's font may differ from your terminal's, images can
look larger or smaller in the browser than in your terminal; set `imageScale` in
`theme.json` to correct it — e.g. `{ "imageScale": 0.36 }` renders images at 36%
of their natural size. Per-image `width=`/`height=` (cells, `px`, or `%`)
override the auto-size. As in iTerm2 the cursor ends at the image's
bottom-right, so text after the image lines up with its base and the next line
feed continues below it.

Images are part of the live stream, not the screen snapshot: viewers connected
when the image is emitted see it, but someone who joins afterwards only sees it
the next time it is sent — the same way a real terminal keeps images in
scrollback rather than on a redrawable screen.

# Contributing

Contributions are welcome! Please refer to our contribution guidelines for details on how to contribute to this project.

Before sending changes, run the verification gate:

```bash
make check   # go fix, gofmt, go vet, gosec, and the race-enabled tests
```

# License

This project is licensed under MIT, see the [LICENSE](LICENSE) file for details.

