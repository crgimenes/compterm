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
- `-api_listen` string: control API listen address (default `127.0.0.1:2201`)
- `-api_key` string: control API key (empty disables the API)
- `-command` string: command to share (default `$SHELL`)
- `-motd` string: message of the day
- `-path` string: path to configuration files (default `$HOME/.config/compterm`)
- `-init` string: configuration file name (default `init.filo`)
- `-debug`: enable debug mode
- `-ignore_pid`: ignore the COMPTERM pid guard
- `-proxy_mode`: accept terminal data from `/wsproxy`

It also recognizes the matching environment variables: `COMPTERM_LISTEN`,
`COMPTERM_API_LISTEN`, `COMPTERM_API_KEY`, `COMPTERM_COMMAND`, `COMPTERM_MOTD`,
`COMPTERM_PATH`, `COMPTERM_INIT_FILE`, `COMPTERM_DEBUG`, `COMPTERM_IGNORE_PID`,
and `COMPTERM_PROXY_MODE`.

Finally, Compterm reads a [Filo](https://github.com/crgimenes/filo)
configuration file, looked up at `./init.filo` and then
`$COMPTERM_PATH/init.filo`. A documented default is created on first run. Each
`(set Key value)` overrides the corresponding setting:

```lisp
;; init.filo
(set Listen "0.0.0.0:2200")
(set MOTD "Welcome to my class")
(set Debug #f)

;; getEnv reads an environment variable with a fallback:
(set APIKey (getEnv "COMPTERM_API_KEY" ""))
```

## Configuration Hierarchy

Defaults are overridden by environment variables, then by command-line flags,
and finally by the `init.filo` file (which takes precedence over all of them,
except for `-path` and `-init`, which locate the file itself).

# Contributing

Contributions are welcome! Please refer to our contribution guidelines for details on how to contribute to this project.

# License

This project is licensed under MIT, see the [LICENSE](LICENSE) file for details.

