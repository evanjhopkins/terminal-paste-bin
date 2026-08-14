# Terminal Paste Bin

Terminal Paste Bin is a small Go command-line project with the executable name `tpb`.

## Development

Build and list existing bins:

```sh
go build -o bin/tpb ./cmd/tpb
./bin/tpb list
```

Reset all persisted data for the current environment:

```sh
./bin/tpb reset
```

`tpbd reset` clears only development data; `tpb reset` clears only production data. Reset has no confirmation prompt.

Run `tpb` for the default bin or `tpb <name>` for a named bin. The initial TUI supports slot selection and quitting; clipboard actions are still pending.

Format, vet, and test the project:

```sh
go fmt ./...
go vet ./...
go test ./...
```

Install the current checkout as the `tpbd` development command on macOS or Linux:

```sh
./scripts/install_dev.sh
```

The script installs to `GOBIN`, or to `$(go env GOPATH)/bin` when `GOBIN` is unset.
Make sure that directory is on your `PATH`. For zsh, add this to `~/.zshrc`:

```sh
export PATH="$PATH:$HOME/go/bin"
```

Reload your shell configuration after making the change:

```sh
source ~/.zshrc
```
