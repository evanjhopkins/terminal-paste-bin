package main

import (
	"fmt"
	"io"
	"os"

	"github.com/evanjhopkins/terminal-paste-bin/internal/store"
)

func main() {
	paths, err := store.DefaultPaths(os.Args[0])
	if err == nil {
		err = run(os.Args[1:], paths, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tpb:", err)
		os.Exit(1)
	}
}

func run(args []string, paths store.Paths, output io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("only \"tpb list\" and \"tpb reset\" are available until interactive mode is implemented")
	}

	switch args[0] {
	case "list":
		bins, err := store.Open(paths)
		if err != nil {
			return err
		}
		for _, name := range bins.ListBins() {
			if _, err := fmt.Fprintln(output, name); err != nil {
				return fmt.Errorf("write bin list: %w", err)
			}
		}
		return nil
	case "reset":
		if err := store.Reset(paths); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(output, "Reset complete."); err != nil {
			return fmt.Errorf("write reset result: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("only \"tpb list\" and \"tpb reset\" are available until interactive mode is implemented")
	}
}
