<div align="center">

# tpb

**A tiny, persistent clipboard register for your terminal.**

![Go](https://img.shields.io/badge/Go-1.26.6-00ADD8?logo=go&logoColor=white)
![Platforms](https://img.shields.io/badge/platform-macOS%20%2B%20Linux-222222)
![License](https://img.shields.io/badge/license-MIT-2ea44f)

Store the text you reach for all day: URLs, commands, IDs, JSON, and multiline snippets. `tpb` gives each named bin ten fast keyboard-addressable slots, with no daemon, account, or cloud sync.

</div>

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

## Why TPB?

- **Ten slots, zero hunting.** Slots map directly to `1` through `9`, then `0`.
- **Named bins.** Keep `myapp`, `production`, and `scratch` separate.
- **Built for the keyboard.** Select, view, copy, write, or delete without pressing Enter.
- **Local by default.** Data lives in small local JSON files with no network activity.

## Quick Start

Requirements: Go `1.26.6`, macOS, or Linux.

```sh
git clone git@github.com:evanjhopkins/terminal-paste-bin.git
cd terminal-paste-bin
go build -o bin/tpb ./cmd/tpb
./bin/tpb --help
./bin/tpb
```

Open the default bin with `tpb`, or create and open a named bin:

```sh
tpb myapp
tpb list
```

Open a bin scoped to the current directory:

```sh
tpb .
```

## Keybindings

| Key | Action |
| --- | --- |
| `1`-`9`, `0` | Select a slot |
| `↑` / `↓` | Move through slots |
| `→` / `v` | View the selected slot in full |
| `←` / `v` | Return to the compact list |
| `r` | Copy the selected slot to the system clipboard and exit |
| `w` | Save the current system clipboard into the selected slot and exit |
| `x` | Execute the selected slot through your shell and exit |
| `d` | Delete the selected slot |
| `q` / `Esc` | Quit |

Reading a blank slot leaves your existing clipboard untouched.

Executing a blank slot leaves TPB open. Commands run without a confirmation prompt: directory bins run in their stored directory, while named bins run in the directory where you launched `tpb`.

## Clipboard Support

macOS uses the built-in `pbcopy` and `pbpaste` commands.

Linux uses the first available option:

- Wayland: `wl-copy` and `wl-paste` from `wl-clipboard`
- X11: `xclip`
- X11 fallback: `xsel`

TPB shows an actionable error if it cannot find a supported clipboard command or graphical session.

## Storage

TPB stores `bins.json` and `config.json` in your operating system's user configuration directory. Files are written atomically.

```text
macOS
  ~/Library/Application Support/tpb/

Linux
  $XDG_CONFIG_HOME/tpb/      (or ~/.config/tpb/)
```

### Resetting Data

Reset removes `bins.json` and `config.json` for the active environment without a confirmation prompt:

```sh
tpb reset
```

### Diagnosing Problems

`tpb doctor` runs diagnostic checks and exits non-zero if any fail. Currently it verifies clipboard access:

```sh
tpb doctor
```

```text
Clipboard access: FAIL
```

Passing checks print in green and failing checks in red when the output is a terminal.

## Development

```sh
go fmt ./...
go vet ./...
go test ./...
go build ./cmd/tpb
```

Contributors can install the current checkout as `tpbd` for local development without sharing storage with a real `tpb` install:

```sh
./scripts/install_dev.sh
```

The script installs to `GOBIN`, or `$(go env GOPATH)/bin` when `GOBIN` is unset. Ensure that directory is on your `PATH`:

```sh
# zsh
echo 'export PATH="$PATH:$HOME/go/bin"' >> ~/.zshrc
source ~/.zshrc
```

Built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea).
