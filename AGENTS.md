# AGENTS.md — compterm

Guidance for agents (and humans) working on compterm. The canonical house
style lives in `~/Projects/eprojects/AGENTS*.md`; this file records what is
specific to compterm and the rules that actually apply here.

## What compterm is

A one-to-many terminal broadcaster. The host runs a shell inside a pty; the
output is fed to an in-memory terminal emulator and streamed, framed, over
websockets to any number of browsers (read-only by default).

```
shell ⇄ pty (creack/pty) ──► screen.Write ──► mterm (grid)  + stream (fan-out)
                                                   │
                              writeToAttachedClients ──► protocol.Encode ──► websocket ──► xterm.js
```

### Packages

- `main.go` — entry point, HTTP/websocket handlers, pty wiring, control API.
- `config` — configuration via the Filo language (see below).
- `mterm` — full in-memory ANSI terminal emulator (parser + grid + scrollback).
  Produces the snapshot that brings a newly connected client up to date.
- `screen` — clients, write permissions, broadcast, resize.
- `stream` — blocking byte fan-out (`bytes.Buffer` + `sync.Cond`).
- `protocol` — binary framing `[cmd][counter][len][payload][fnv32]`.
- `session` — cookie-based session identity (not authentication).
- `prelude` — small HTTP helpers + control API auth.
- `cmd/client` — terminal viewer prototype (renders the stream with `mterm`).
- `cmd/proxytx`, `cmd/rawsocket` — remote producers feeding `/wsproxy` and TCP.

## Configuration (Filo)

Configuration uses the Filo language (`github.com/crgimenes/filo`), not Lua.
Resolution order: built-in defaults → `COMPTERM_*` environment variables →
command-line flags → `init.filo` (which overrides everything except `-path`
and `-init`, since those locate the file). The file is looked up at
`./init.filo` then `$COMPTERM_PATH/init.filo`, and a documented default is
created on first run. The `getEnv` builtin reads env vars with a fallback.
Every value is validated in `config.validate`.

## Code style (what makes a change adherent)

- **Go 1.26+, standard library first.** Third-party deps must be justified;
  the current set (`coder/websocket`, `creack/pty`, `golang.org/x/term`,
  `crgimenes/filo`) is. Do not add more without a clear reason.
- **US English** for code, comments, and identifiers.
- **Guard clauses / early returns.** Never use `else` after a terminal branch
  (`return`, `continue`, `break`, `log.Fatal`, `panic`).
- **Handle errors explicitly and immediately.** Avoid panics except for
  unrecoverable programmer errors.
- **No hidden global state**; prefer explicit wiring. (compterm still has some
  package-level globals in `main` — reduce them, don't add more.)
- **Never log secrets** (API keys, tokens).
- **Tests:** table-driven, isolated, order-independent. Use `t.TempDir()` for
  filesystem tests; race-test anything concurrent (`go test -race`).

## Build & run

```bash
make          # production build (bundles assets, -trimpath, embeds resources)
make dev      # dev mode, serves ./assets from disk
make dev-race # dev mode with the race detector
```

`-trimpath` is mandatory in every build path (already set in the Makefile).

## Verification flow (run before declaring a change done)

```bash
go fix ./... && go fix -inline ./...
go vet ./...
gosec ./...        # if installed
go test ./...      # add -race for concurrent code
gofmt -l .         # must print nothing
```

## Current state / roadmap

Ongoing improvement program (see the session memory for details):

1. ✅ Config migrated from Lua to Filo.
2. ◐ Adherence to the house AGENTS rules (this file; early-returns; safe bug
   fixes done: session map race + sweeper, `IgnorePID`, `NewManager` rows/cols,
   empty-API-key auth).
3. ☐ User validation before connection — **shared password/token** model,
   gating `/ws` (viewers) and `/wsproxy` (producer).
4. ☐ TUI **viewer** client (polish of `cmd/client` over `mterm`).
5. ☐ (later) drop xterm.js using mterm's grid.

**Known deferred debt:** `screen.go` locking is inconsistent (some methods take
`s.mx`, others don't; `Send` mutates `Clients` mid-range; `Screen.Input` is
unsynchronized). This is a dedicated, reviewable refactor — do not bolt it onto
unrelated changes.
