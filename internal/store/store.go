// Package store persists TPB bins in local JSON files.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"unicode"
	"unicode/utf8"
)

const maxBinNameLength = 64

var (
	ErrBinNotFound    = errors.New("bin not found")
	ErrInvalidBinName = errors.New("invalid bin name")
	ErrInvalidSlot    = errors.New("invalid slot")
)

type bin struct {
	Slots map[int]string
}

// Store holds the bins loaded from a single environment's storage files.
type Store struct {
	paths Paths
	bins  map[string]bin
}

// Open loads an environment's bins and ensures its storage files exist.
func Open(paths Paths) (*Store, error) {
	if err := os.MkdirAll(paths.Directory, 0o700); err != nil {
		return nil, fmt.Errorf("create storage directory: %w", err)
	}

	if err := ensureConfigFile(paths.ConfigFile); err != nil {
		return nil, err
	}

	bins, err := loadBins(paths.BinsFile)
	if err != nil {
		return nil, err
	}

	return &Store{paths: paths, bins: bins}, nil
}

// EnsureBin creates a blank bin if it does not already exist.
func (s *Store) EnsureBin(name string) error {
	if err := ValidateBinName(name); err != nil {
		return err
	}
	if _, exists := s.bins[name]; exists {
		return nil
	}

	s.bins[name] = bin{Slots: make(map[int]string)}
	return s.save()
}

// ListBins returns bin names in alphabetical order.
func (s *Store) ListBins() []string {
	names := make([]string, 0, len(s.bins))
	for name := range s.bins {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ReadSlot returns a slot's value and whether the slot has a stored value.
func (s *Store) ReadSlot(binName string, slot int) (string, bool, error) {
	if err := validateSlot(slot); err != nil {
		return "", false, err
	}

	bin, exists := s.bins[binName]
	if !exists {
		return "", false, fmt.Errorf("%w: %s", ErrBinNotFound, binName)
	}

	value, exists := bin.Slots[slot]
	return value, exists, nil
}

// WriteSlot stores value in a bin slot, replacing any previous value.
func (s *Store) WriteSlot(binName string, slot int, value string) error {
	if err := validateSlot(slot); err != nil {
		return err
	}

	bin, exists := s.bins[binName]
	if !exists {
		return fmt.Errorf("%w: %s", ErrBinNotFound, binName)
	}

	bin.Slots[slot] = value
	s.bins[binName] = bin
	return s.save()
}

// DeleteSlot clears a slot. Deleting an already blank slot is successful.
func (s *Store) DeleteSlot(binName string, slot int) error {
	if err := validateSlot(slot); err != nil {
		return err
	}

	bin, exists := s.bins[binName]
	if !exists {
		return fmt.Errorf("%w: %s", ErrBinNotFound, binName)
	}

	delete(bin.Slots, slot)
	s.bins[binName] = bin
	return s.save()
}

// ValidateBinName applies the initial conservative bin-name policy.
func ValidateBinName(name string) error {
	if name == "" || name == "list" || len(name) > maxBinNameLength || !utf8.ValidString(name) {
		return fmt.Errorf("%w: %q", ErrInvalidBinName, name)
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return fmt.Errorf("%w: %q", ErrInvalidBinName, name)
		}
	}
	return nil
}

func ensureConfigFile(path string) error {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			return fmt.Errorf("create config file: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	var config map[string]json.RawMessage
	if err := json.Unmarshal(contents, &config); err != nil || config == nil {
		if err == nil {
			err = errors.New("configuration must be a JSON object")
		}
		return fmt.Errorf("parse config file: %w", err)
	}
	return nil
}

func loadBins(path string) (map[string]bin, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		bins := make(map[string]bin)
		if err := saveBins(path, bins); err != nil {
			return nil, err
		}
		return bins, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read bins file: %w", err)
	}

	bins := make(map[string]bin)
	if err := json.Unmarshal(contents, &bins); err != nil || bins == nil {
		if err == nil {
			err = errors.New("bins must be a JSON object")
		}
		return nil, fmt.Errorf("parse bins file: %w", err)
	}
	for name, bin := range bins {
		if err := ValidateBinName(name); err != nil {
			return nil, fmt.Errorf("parse bins file: %w", err)
		}
		if bin.Slots == nil {
			bin.Slots = make(map[int]string)
			bins[name] = bin
		}
		for slot := range bin.Slots {
			if err := validateSlot(slot); err != nil {
				return nil, fmt.Errorf("parse bins file: %w", err)
			}
		}
	}
	return bins, nil
}

func (s *Store) save() error {
	return saveBins(s.paths.BinsFile, s.bins)
}

func saveBins(path string, bins map[string]bin) error {
	contents, err := json.MarshalIndent(bins, "", "  ")
	if err != nil {
		return fmt.Errorf("encode bins: %w", err)
	}
	if err := os.WriteFile(path, append(contents, '\n'), 0o600); err != nil {
		return fmt.Errorf("write bins file: %w", err)
	}
	return nil
}

func validateSlot(slot int) error {
	if slot < 0 || slot > 9 {
		return fmt.Errorf("%w: %d", ErrInvalidSlot, slot)
	}
	return nil
}
