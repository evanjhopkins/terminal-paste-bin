package main

import (
	"fmt"
	"io"
	"os"

	"github.com/evanjhopkins/terminal-paste-bin/internal/store"
	"github.com/evanjhopkins/terminal-paste-bin/internal/tui"
)

func main() {
	paths, err := store.DefaultPaths(os.Args[0])
	var launch *binLaunch
	if err == nil {
		launch, err = run(os.Args[1:], paths, os.Stdout)
	}
	if err == nil && launch != nil {
		err = tui.Run(launch.name, launch.slots)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tpb:", err)
		os.Exit(1)
	}
}

type binLaunch struct {
	name  string
	slots map[int]string
}

func run(args []string, paths store.Paths, output io.Writer) (*binLaunch, error) {
	if len(args) == 0 {
		return loadBin("default", paths)
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("usage: tpb [bin-name | list | reset]")
	}

	switch args[0] {
	case "list":
		bins, err := store.Open(paths)
		if err != nil {
			return nil, err
		}
		for _, name := range bins.ListBins() {
			if _, err := fmt.Fprintln(output, name); err != nil {
				return nil, fmt.Errorf("write bin list: %w", err)
			}
		}
		return nil, nil
	case "reset":
		if err := store.Reset(paths); err != nil {
			return nil, err
		}
		if _, err := fmt.Fprintln(output, "Reset complete."); err != nil {
			return nil, fmt.Errorf("write reset result: %w", err)
		}
	default:
		return loadBin(args[0], paths)
	}
	return nil, nil
}

func loadBin(name string, paths store.Paths) (*binLaunch, error) {
	if err := store.ValidateBinName(name); err != nil {
		return nil, err
	}

	bins, err := store.Open(paths)
	if err != nil {
		return nil, err
	}
	if err := bins.EnsureBin(name); err != nil {
		return nil, err
	}

	slots := make(map[int]string, 10)
	for slot := 0; slot <= 9; slot++ {
		value, exists, err := bins.ReadSlot(name, slot)
		if err != nil {
			return nil, err
		}
		if exists {
			slots[slot] = value
		}
	}
	return &binLaunch{name: name, slots: slots}, nil
}
