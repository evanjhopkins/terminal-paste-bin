// Package store persists TPB bins in local JSON files.
package store

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"unicode"
	"unicode/utf8"
)

const maxBinNameLength = 64

const directoryBinPrefix = "@directory:"

var (
	ErrBinNotFound    = errors.New("bin not found")
	ErrInvalidBinName = errors.New("invalid bin name")
	ErrInvalidSlot    = errors.New("invalid slot")
)

type bin struct {
	Directory string `json:",omitempty"`
	Slots     map[int]string
}

// BinInfo identifies a persisted bin and the text used to present it.
type BinInfo struct {
	ID        string
	Name      string
	Directory string
}

// IsDirectory reports whether the bin belongs to a directory.
func (b BinInfo) IsDirectory() bool {
	return b.Directory != ""
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

	store := &Store{paths: paths}
	if err := withFileLock(paths.LockFile, func() error {
		if err := ensureConfigFile(paths.ConfigFile); err != nil {
			return err
		}

		bins, err := loadBins(paths.BinsFile)
		if err != nil {
			return err
		}
		store.bins = bins
		return nil
	}); err != nil {
		return nil, err
	}

	return store, nil
}

// Reset removes all persisted TPB data for one environment.
func Reset(paths Paths) error {
	if err := os.MkdirAll(paths.Directory, 0o700); err != nil {
		return fmt.Errorf("create storage directory: %w", err)
	}

	return withFileLock(paths.LockFile, func() error {
		for _, path := range []string{paths.BinsFile, paths.ConfigFile} {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove persisted file %s: %w", path, err)
			}
		}
		return nil
	})
}

// EnsureBin creates a blank bin if it does not already exist.
func (s *Store) EnsureBin(name string) error {
	if err := ValidateBinName(name); err != nil {
		return err
	}
	return s.update(func(bins map[string]bin) error {
		if _, exists := bins[name]; !exists {
			bins[name] = bin{Slots: make(map[int]string)}
		}
		return nil
	})
}

// EnsureDirectoryBin creates or returns the bin for an absolute directory path.
func (s *Store) EnsureDirectoryBin(directory string) (BinInfo, error) {
	if err := validateDirectory(directory); err != nil {
		return BinInfo{}, err
	}

	info := BinInfo{ID: directoryBinID(directory), Directory: directory}
	if err := s.update(func(bins map[string]bin) error {
		existing, exists := bins[info.ID]
		if !exists {
			bins[info.ID] = bin{Directory: directory, Slots: make(map[int]string)}
			return nil
		}
		if existing.Directory != directory {
			return fmt.Errorf("directory bin identifier collision")
		}
		return nil
	}); err != nil {
		return BinInfo{}, err
	}
	return info, nil
}

// ListBins returns named bins followed by directory bins, alphabetically within
// each group.
func (s *Store) ListBins() []BinInfo {
	bins := make([]BinInfo, 0, len(s.bins))
	for id, stored := range s.bins {
		info := BinInfo{ID: id, Name: id, Directory: stored.Directory}
		if info.IsDirectory() {
			info.Name = ""
		}
		bins = append(bins, info)
	}
	sort.Slice(bins, func(left, right int) bool {
		if bins[left].IsDirectory() != bins[right].IsDirectory() {
			return !bins[left].IsDirectory()
		}
		if bins[left].IsDirectory() {
			return bins[left].Directory < bins[right].Directory
		}
		return bins[left].Name < bins[right].Name
	})
	return bins
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

	return s.update(func(bins map[string]bin) error {
		bin, exists := bins[binName]
		if !exists {
			return fmt.Errorf("%w: %s", ErrBinNotFound, binName)
		}

		bin.Slots[slot] = value
		bins[binName] = bin
		return nil
	})
}

// DeleteSlot clears a slot. Deleting an already blank slot is successful.
func (s *Store) DeleteSlot(binName string, slot int) error {
	if err := validateSlot(slot); err != nil {
		return err
	}

	return s.update(func(bins map[string]bin) error {
		bin, exists := bins[binName]
		if !exists {
			return fmt.Errorf("%w: %s", ErrBinNotFound, binName)
		}

		delete(bin.Slots, slot)
		bins[binName] = bin
		return nil
	})
}

// ValidateBinName applies the initial conservative bin-name policy.
func ValidateBinName(name string) error {
	if name == "" || name == "list" || name == "reset" || name == "doctor" || name == "default" || len(name) > maxBinNameLength || !utf8.ValidString(name) {
		return fmt.Errorf("%w: %q", ErrInvalidBinName, name)
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return fmt.Errorf("%w: %q", ErrInvalidBinName, name)
		}
	}
	return nil
}

func directoryBinID(directory string) string {
	digest := sha256.Sum256([]byte(directory))
	return directoryBinPrefix + base64.RawURLEncoding.EncodeToString(digest[:])
}

func validateDirectory(directory string) error {
	if !filepath.IsAbs(directory) || !utf8.ValidString(directory) {
		return fmt.Errorf("invalid directory path %q", directory)
	}
	for _, character := range directory {
		if unicode.IsControl(character) {
			return fmt.Errorf("invalid directory path %q", directory)
		}
	}
	return nil
}

func ensureConfigFile(path string) error {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := writeFileAtomic(path, []byte("{}\n")); err != nil {
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
	removedReserved := false
	for name, bin := range bins {
		if name == "default" || name == "doctor" {
			delete(bins, name)
			removedReserved = true
			continue
		}
		if bin.Directory == "" {
			if err := ValidateBinName(name); err != nil {
				return nil, fmt.Errorf("parse bins file: %w", err)
			}
		} else {
			if err := validateDirectory(bin.Directory); err != nil {
				return nil, fmt.Errorf("parse bins file: %w", err)
			}
			if name != directoryBinID(bin.Directory) {
				return nil, fmt.Errorf("parse bins file: directory bin identifier does not match path")
			}
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
	if removedReserved {
		if err := saveBins(path, bins); err != nil {
			return nil, err
		}
	}
	return bins, nil
}

func (s *Store) update(change func(map[string]bin) error) error {
	return withFileLock(s.paths.LockFile, func() error {
		bins, err := loadBins(s.paths.BinsFile)
		if err != nil {
			return err
		}
		if err := change(bins); err != nil {
			return err
		}
		if err := saveBins(s.paths.BinsFile, bins); err != nil {
			return err
		}
		s.bins = bins
		return nil
	})
}

func saveBins(path string, bins map[string]bin) error {
	contents, err := json.MarshalIndent(bins, "", "  ")
	if err != nil {
		return fmt.Errorf("encode bins: %w", err)
	}
	if err := writeFileAtomic(path, append(contents, '\n')); err != nil {
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
