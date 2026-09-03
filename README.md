# tpb

`tpb` is a terminal UI for keeping small pieces of text in persistent clipboard
registers. Each bin has ten slots mapped to the number keys. A slot can hold a
URL, command, identifier, JSON document, or other multiline text, and can be
copied to or replaced from the system clipboard.

Bins are stored locally as JSON. There is no daemon, account, network service,
or synchronization layer.

```text
Terminal Paste Bin - myapp

  1  postgres://localhost:5432/myapp
> 2  deploy@prod.example.com
  3  {"customer":"cus_123","enabled":true}
  4  <blank>
  5  <blank>
  6  <blank>
  7  hello ↵ world
  8  <blank>
  9  <blank>
  0  <blank>

↑/↓ move   1-0 select   →/v view   r read   w write   x execute   d delete   q quit
```

## Features

- Ten keyboard-addressable slots per bin, including multiline values
- Named bins for reusable groups of values
- Directory bins tied to the canonical path of the current working directory
- System clipboard integration on macOS, Wayland, and X11
- Case-insensitive search across all bins
- Bin rename, deletion, stale-bin cleanup, and diagnostics
- Atomic writes and file locking for local JSON storage
- Direct execution of a stored shell command

## Requirements

- macOS or Linux
- Go 1.26.6 or newer when installing from source
- For clipboard reads and writes on Linux, one supported clipboard tool:
  - Wayland: `wl-copy` and `wl-paste` from
    [`wl-clipboard`](https://github.com/bugaevc/wl-clipboard)
  - X11: `xclip` or `xsel`

macOS uses the built-in `pbcopy` and `pbpaste` commands. On Linux, `tpb`
prefers `wl-clipboard` when `WAYLAND_DISPLAY` is set, then `xclip`, then
`xsel`.

## Installation

Install the command directly with Go:

```sh
go install github.com/evanjhopkins/terminal-paste-bin/cmd/tpb@latest
```

The Go binary directory must be on `PATH`. This is `GOBIN` when set, otherwise
`$(go env GOPATH)/bin`.

To build a checkout instead:

```sh
git clone https://github.com/evanjhopkins/terminal-paste-bin.git
cd terminal-paste-bin
go build -o bin/tpb ./cmd/tpb
./bin/tpb --help
```

The executable must be named `tpb` (or `tpbd` for an isolated development
installation), because the name selects its storage directory.

## Quick Start

Open the bin associated with the current directory:

```sh
tpb
```

To store the current system clipboard in slot 1, press `1`, then `w`. The TUI
exits after writing. Run `tpb` again, press `1`, then `r` to copy that slot back
to the system clipboard.

Use an argument to create or open a named bin instead:

```sh
tpb myapp
```

Directory bins use canonical paths, so opening `tpb` through a symlink to the
same directory reaches the same bin. Named bins run independently of the
current directory.

## TUI Controls

| Key | Action |
| --- | --- |
| `1`-`9`, `0` | Select a slot |
| `↑`, `↓` | Move between slots |
| `→`, `v` | Expand the selected slot |
| `←`, `v` | Return to the compact view |
| `r` | Copy the selected slot to the system clipboard and exit |
| `w` | Replace the selected slot with the system clipboard and exit |
| `x` | Execute the selected slot through the shell and exit |
| `d` | Clear the selected slot |
| `q`, `Esc` | Quit |

Reading or executing a blank slot leaves the TUI open. Reading it does not
change the clipboard.

`x` does not ask for confirmation. It runs the slot as `$SHELL -c <value>`, or
with `/bin/sh` when `SHELL` is unset. Commands in directory bins run from the
stored directory; commands in named bins run from the directory where `tpb`
was started.

## CLI Reference

```text
tpb [bin-name]
tpb list
tpb delete [--yes] <bin>
tpb rename <old> <new>
tpb prune [--dry-run]
tpb reset
tpb doctor
tpb search <query>
tpb --help
tpb --version
```

### List and search

```sh
tpb list
tpb search postgres
```

`list` prints named bins first, followed by directory bins in the form
`(dir) /absolute/path`. Both groups are sorted alphabetically.

`search` performs a case-insensitive substring match against every non-blank
slot. Results are tab-separated and contain the bin, slot number, and a
single-line preview:

```text
myapp	1	postgres://localhost:5432/myapp
production	4	POSTGRES_PASSWORD=...
(dir) /Users/you/code/app	7	hello ↵ world
```

A search with no matches prints nothing and exits with status 2. Errors reported
by `tpb` exit with status 1; a command launched with `x` propagates that
command's exit status.

### Manage bins

```sh
tpb rename myapp myapp-v2
tpb delete myapp-v2
tpb delete --yes myapp-v2
tpb prune --dry-run
tpb prune
```

`delete` applies only to named bins. It prompts on a terminal; `--yes` or `-y`
is required to skip the prompt and is also required when input or output is not
a terminal.

`prune` removes:

- directory bins whose stored directory no longer exists
- named or directory bins whose ten slots are all blank

Use `--dry-run` (or `-n`) to inspect the result first. Directory bins cannot be
renamed or deleted individually.

Bin names may contain up to 64 bytes of valid UTF-8 but cannot contain control
characters. Command names are reserved: `list`, `delete`, `rename`, `prune`,
`reset`, `doctor`, and `search`; `default` is also reserved.

### Diagnose and reset

```sh
tpb doctor
tpb reset
```

`doctor` reports the selected clipboard backend, stale directory bins, and
empty bins. Clipboard failure makes the command exit with status 1. Stale and
empty bins are warnings and do not change the exit status or modify storage.
Set `NO_COLOR` to any non-empty value to disable color in terminal output.

`reset` deletes all bins and persisted configuration for the active executable
without confirmation. A `tpbd` development installation has separate data and
is not affected by `tpb reset`.

## Storage and Configuration

`tpb` uses the operating system's user configuration directory:

```text
macOS: ~/Library/Application Support/tpb/
Linux: $XDG_CONFIG_HOME/tpb/ or ~/.config/tpb/
```

The directory contains `bins.json`, `config.json`, and `bins.lock`. Storage
files are created on first use. The directory is mode `0700`; newly written
data and lock files are mode `0600`.

There are currently no user-facing options in `config.json`. It must remain a
valid JSON object, but its contents are not otherwise consumed. `SHELL` selects
the shell used to execute a slot, `NO_COLOR` disables diagnostic colors, and
the standard `XDG_CONFIG_HOME` variable changes the Linux storage root.

## Development

```sh
go fmt ./...
go vet ./...
go test ./...
go build ./cmd/tpb
```

Install the current checkout as `tpbd` to test it without touching normal
`tpb` data:

```sh
./scripts/install_dev.sh
tpbd --version
```

The main packages are organized as follows:

```text
cmd/tpb/             command parsing and application wiring
internal/clipboard/  platform-specific clipboard backends
internal/store/      JSON persistence, locking, and bin lifecycle
internal/tui/        Bubble Tea interface and key handling
scripts/             development installation helper
```

## License

[MIT](LICENSE) © 2026 Evan Hopkins
