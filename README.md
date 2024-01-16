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

Compterm accepts the following command-line arguments:

- -c string: Command to run (default: $SHELL)
- -debug: Enable debug mode
- -listen string: Listen address (default "0.0.0.0:2200")
- -motd string: Message of the day

It also recognizes these environment variables:

- $COMPTERM_DEBUG bool (default "false")
- $COMPTERM_LISTEN string (default "0.0.0.0:2200")
- $COMPTERM_C string
- $COMPTERM_MOTD string (default "")

Additionally, you can configure Compterm using a init.lua file located in ~/.config/compterm/init.lua.

## Configuration Hierarchy

Command-line parameters override environment variables, and the lua in the init.lua can override both variables and commands.

# Contributing

Contributions are welcome! Please refer to our contribution guidelines for details on how to contribute to this project.

# License

This project is licensed under MIT, see the [LICENSE](LICENSE) file for details.

