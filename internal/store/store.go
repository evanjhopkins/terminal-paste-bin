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
	"syscall"
	"unicode"
	"unicode/utf8"
)

const maxBinNameLength = 64

const directoryBinPrefix = "@directory:"

var (
	ErrBinNotFound    = errors.New("bin not found")
	ErrBinExists      = errors.New("bin already exists")
	ErrDirectoryBin   = errors.New("directory bins are keyed by path and cannot be modified by name")
	ErrInvalidBinName = errors.New("invalid bin name")
	ErrInvalidSlot    = errors.New("invalid slot")
)

// reservedBinNames are command words that cannot be used as bin names.
var reservedBinNames = map[string]bool{
	"list":    true,
	"reset":   true,
	"doctor":  true,
	"default": true,
	"delete":  true,
	"rename":  true,
	"prune":   true,
	"search":  true,
}

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

// Lookup returns the bin stored under an identifier and whether it exists.
func (s *Store) Lookup(binName string) (BinInfo, bool) {
	stored, exists := s.bins[binName]
	if !exists {
		return BinInfo{}, false
	}
	info := BinInfo{ID: binName, Name: binName, Directory: stored.Directory}
	if info.IsDirectory() {
		info.Name = ""
	}
	return info, true
}

// DeleteBin permanently removes a named bin and all of its slots.
func (s *Store) DeleteBin(binName string) error {
	return s.update(func(bins map[string]bin) error {
		stored, exists := bins[binName]
		if !exists {
			return fmt.Errorf("%w: %s", ErrBinNotFound, binName)
		}
		if stored.Directory != "" {
			return fmt.Errorf("%w: %s", ErrDirectoryBin, binName)
		}
		delete(bins, binName)
		return nil
	})
}

// RenameBin moves a named bin to a new name, preserving its slots. The new
// name must be valid and unused.
func (s *Store) RenameBin(oldName, newName string) error {
	if err := ValidateBinName(newName); err != nil {
		return err
	}
	return s.update(func(bins map[string]bin) error {
		stored, exists := bins[oldName]
		if !exists {
			return fmt.Errorf("%w: %s", ErrBinNotFound, oldName)
		}
		if stored.Directory != "" {
			return fmt.Errorf("%w: %s", ErrDirectoryBin, oldName)
		}
		if _, taken := bins[newName]; taken {
			return fmt.Errorf("%w: %s", ErrBinExists, newName)
		}
		delete(bins, oldName)
		bins[newName] = stored
		return nil
	})
}

// StaleDirectoryBins returns the directory bins whose directory no longer
// exists, sorted by directory.
func (s *Store) StaleDirectoryBins() []BinInfo {
	return staleDirectoryBins(s.bins)
}

// EmptyBins returns the bins whose slots are all blank (no stored value or
// only empty-string values), covering both named and directory bins. Named
// bins come first alphabetically, then directory bins by path, mirroring
// ListBins.
func (s *Store) EmptyBins() []BinInfo {
	return emptyBins(s.bins)
}

// PrunableBins returns the bins `tpb prune` would remove: stale directory
// bins plus empty bins of either kind, each listed once. Named bins come
// first alphabetically, then directory bins by path.
func (s *Store) PrunableBins() []BinInfo {
	return prunableBins(s.bins)
}

// PruneEmptyBins removes bins whose slots are all blank and returns the bins
// it removed in EmptyBins order.
func (s *Store) PruneEmptyBins() ([]BinInfo, error) {
	var pruned []BinInfo
	err := s.update(func(bins map[string]bin) error {
		pruned = emptyBins(bins)
		for _, info := range pruned {
			delete(bins, info.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pruned, nil
}

// PruneStaleAndEmptyBins removes stale directory bins and empty bins of
// either kind in a single update, so a bin that is both stale and empty is
// removed and reported exactly once. It returns the removed bins with named
// bins first alphabetically, then directory bins by path.
func (s *Store) PruneStaleAndEmptyBins() ([]BinInfo, error) {
	var pruned []BinInfo
	err := s.update(func(bins map[string]bin) error {
		pruned = prunableBins(bins)
		for _, info := range pruned {
			delete(bins, info.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pruned, nil
}

// binEmpty reports whether a bin holds zero non-blank slots. A slot counts
// as blank when it has no stored value or its value is the empty string.
func binEmpty(stored bin) bool {
	for _, value := range stored.Slots {
		if value != "" {
			return false
		}
	}
	return true
}

func emptyBins(bins map[string]bin) []BinInfo {
	empty := make([]BinInfo, 0)
	for id, stored := range bins {
		if !binEmpty(stored) {
			continue
		}
		info := BinInfo{ID: id, Name: id, Directory: stored.Directory}
		if info.IsDirectory() {
			info.Name = ""
		}
		empty = append(empty, info)
	}
	sort.Slice(empty, func(left, right int) bool {
		if empty[left].IsDirectory() != empty[right].IsDirectory() {
			return !empty[left].IsDirectory()
		}
		if empty[left].IsDirectory() {
			return empty[left].Directory < empty[right].Directory
		}
		return empty[left].Name < empty[right].Name
	})
	return empty
}

// prunableBins returns the union of stale directory bins and empty bins,
// deduplicated by bin ID so a directory bin that is both stale and empty
// appears exactly once.
func prunableBins(bins map[string]bin) []BinInfo {
	staleIDs := make(map[string]bool)
	for _, info := range staleDirectoryBins(bins) {
		staleIDs[info.ID] = true
	}
	prunable := make([]BinInfo, 0)
	seen := make(map[string]bool)
	for id, stored := range bins {
		info := BinInfo{ID: id, Name: id, Directory: stored.Directory}
		if info.IsDirectory() {
			info.Name = ""
		}
		if staleIDs[id] || binEmpty(stored) {
			if !seen[id] {
				seen[id] = true
				prunable = append(prunable, info)
			}
		}
	}
	sort.Slice(prunable, func(left, right int) bool {
		if prunable[left].IsDirectory() != prunable[right].IsDirectory() {
			return !prunable[left].IsDirectory()
		}
		if prunable[left].IsDirectory() {
			return prunable[left].Directory < prunable[right].Directory
		}
		return prunable[left].Name < prunable[right].Name
	})
	return prunable
}

// PruneDirectoryBins removes directory bins whose directory no longer exists
// and returns the bins it removed, sorted by directory.
func (s *Store) PruneDirectoryBins() ([]BinInfo, error) {
	var pruned []BinInfo
	err := s.update(func(bins map[string]bin) error {
		pruned = staleDirectoryBins(bins)
		for _, info := range pruned {
			delete(bins, info.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pruned, nil
}

func staleDirectoryBins(bins map[string]bin) []BinInfo {
	stale := make([]BinInfo, 0)
	for id, stored := range bins {
		if stored.Directory == "" || !directoryMissing(stored.Directory) {
			continue
		}
		stale = append(stale, BinInfo{ID: id, Directory: stored.Directory})
	}
	sort.Slice(stale, func(left, right int) bool {
		return stale[left].Directory < stale[right].Directory
	})
	return stale
}

// directoryMissing reports whether a stored directory path definitely no
// longer refers to a directory. Errors other than "does not exist" (such as
// permission problems) are treated as present so that bins are never pruned
// on uncertain evidence.
func directoryMissing(directory string) bool {
	info, err := os.Stat(directory)
	if err == nil {
		return !info.IsDir()
	}
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// ValidateBinName applies the initial conservative bin-name policy: names must
// be well-formed and must not collide with a tpb command word.
func ValidateBinName(name string) error {
	if reservedBinNames[name] {
		return fmt.Errorf("%w: %q is reserved", ErrInvalidBinName, name)
	}
	return validateBinNameSyntax(name)
}

// validateBinNameSyntax checks the structural rules for a bin name without
// rejecting reserved words. Persisted bins are checked with this so that a bin
// created before its name became reserved still loads and can be renamed.
func validateBinNameSyntax(name string) error {
	if name == "" || len(name) > maxBinNameLength || !utf8.ValidString(name) {
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
			if err := validateBinNameSyntax(name); err != nil {
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
