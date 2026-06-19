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
- `-path` string: path to configuration files (default `$HOME/.config/compterm`)
- `-init` string: configuration file name (default `init.filo`)
- `-ignore_pid`: ignore the COMPTERM pid guard

It also recognizes the matching environment variables: `COMPTERM_LISTEN`,
`COMPTERM_AUTH_TOKEN`, `COMPTERM_COMMAND`, `COMPTERM_PATH`, `COMPTERM_INIT_FILE`,
and `COMPTERM_IGNORE_PID`.

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

# Contributing

Contributions are welcome! Please refer to our contribution guidelines for details on how to contribute to this project.

# License

This project is licensed under MIT, see the [LICENSE](LICENSE) file for details.

