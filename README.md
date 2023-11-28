# Overview

Compterm is a versatile terminal sharing application designed for a variety of use cases, including educational, developmental, and nostalgic experiences. It's particularly useful for the Brazilian Golang Study Group, the Atomic Blast BBS system, and for efficient, low-bandwidth pair programming sessions.

# Key Features

- Terminal Sharing for Study Groups: Enables participants of the Brazilian Golang Study Group to view and discuss the same terminal, enhancing collaborative learning and code discussion.
- BBS System Interface: Acts as a terminal for accessing the Atomic Blast BBS, emulating the look and feel of classic 90s BBSs.
- Efficient Pair Programming: Facilitates quick and easy terminal sharing for pair programming, minimizing bandwidth usage.

# Installation

To use Compterm, Go developers can build the program for production using:

```bash
Copy code
go build -ldflags '-s -w'
# or
make
```

For development mode, which uses resources from the assets directory, use:

```bash
Copy code
go run -tags dev .
# or
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

Additionally, you can configure Compterm using a config.ini file located in ~/.config/compterm/config.ini with the following parameters:

- debug
- listen
- command
- motd

## Configuration Hierarchy

Command-line parameters override environment variables, which in turn override the configuration file settings.

# Contributing

Contributions are welcome! Please refer to our contribution guidelines for details on how to contribute to this project.

# License

This project is licensed under [include license type here], see the LICENSE file for details.

