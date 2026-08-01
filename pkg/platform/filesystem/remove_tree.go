package filesystem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RemoveTree removes one named directory beneath parentDirectory without
// removing parentDirectory itself. The parent and every descendant are
// accessed through handle-relative os.Root operations: the target directory
// and descendant directories are never followed through a symlink, junction,
// or other reparse point. A descendant link is unlinked as an entry instead.
//
// Entries are removed in lexical order. The operation stops at the first
// filesystem or cancellation error; changed reports whether an earlier effect
// removed anything, so callers can describe partial failure without retrying
// an already-applied effect.
func (Local) RemoveTree(
	ctx context.Context,
	parentDirectory string,
	targetName string,
) (changed bool, err error) {
	if err := removalContextError(ctx); err != nil {
		return false, err
	}
	if err := validateTreeTargetName(targetName); err != nil {
		return false, err
	}
	if strings.TrimSpace(parentDirectory) == "" {
		return false, fmt.Errorf("model asset cache parent directory is required")
	}

	parentRoot, err := openTreeParent(parentDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open model asset cache parent: %w", err)
	}
	parent := localTreeDirectory{root: parentRoot}
	defer parent.Close()

	target, err := parent.OpenDirectory(targetName)
	if errors.Is(err, os.ErrNotExist) {
		if _, statErr := parentRoot.Lstat(targetName); errors.Is(statErr, os.ErrNotExist) {
			return false, nil
		} else if statErr != nil {
			return false, fmt.Errorf("inspect model asset cache target after open failure: %w", statErr)
		}
		// Windows can classify a regular file, junction, or other reparse
		// point as a not-found directory-open failure. It is present, so this
		// is a malformed or unsafe target rather than idempotent absence.
		return false, fmt.Errorf("open model asset cache target: %w", err)
	}
	if err != nil {
		// A target symlink, junction, reparse point, or non-directory is not
		// an absent cache. Leave it untouched and fail closed.
		return false, fmt.Errorf("open model asset cache target: %w", err)
	}

	changed, err = removeTreeContents(ctx, target)
	closeErr := target.Close()
	if err != nil {
		return changed, err
	}
	if closeErr != nil {
		return changed, fmt.Errorf("close model asset cache target: %w", closeErr)
	}
	if err := removalContextError(ctx); err != nil {
		return changed, err
	}

	if err := parent.Remove(targetName); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return changed, nil
		}
		return changed, fmt.Errorf("remove model asset cache target: %w", err)
	}
	if err := removalContextError(ctx); err != nil {
		return true, err
	}
	return true, nil
}

type treeDirectory interface {
	ReadDir() ([]treeEntry, error)
	OpenDirectory(string) (treeDirectory, error)
	Remove(string) error
	Close() error
}

type treeEntry struct {
	name  string
	isDir bool
}

type localTreeDirectory struct {
	root *os.Root
}

func (directory localTreeDirectory) ReadDir() ([]treeEntry, error) {
	file, err := directory.root.Open(".")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	result := make([]treeEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if err := validateTreeEntryName(name); err != nil {
			return nil, err
		}
		result = append(result, treeEntry{name: name, isDir: entry.IsDir()})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result, nil
}

func (directory localTreeDirectory) OpenDirectory(name string) (treeDirectory, error) {
	root, err := directory.root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return localTreeDirectory{root: root}, nil
}

func (directory localTreeDirectory) Remove(name string) error {
	return directory.root.Remove(name)
}

func (directory localTreeDirectory) Close() error {
	return directory.root.Close()
}

func removeTreeContents(ctx context.Context, directory treeDirectory) (changed bool, err error) {
	if err := removalContextError(ctx); err != nil {
		return false, err
	}
	entries, err := directory.ReadDir()
	if err != nil {
		return false, fmt.Errorf("read model asset cache directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	for _, entry := range entries {
		name := entry.name
		if err := removalContextError(ctx); err != nil {
			return changed, err
		}
		if !entry.isDir {
			if err := directory.Remove(name); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return changed, fmt.Errorf("remove model asset %q: %w", name, err)
			}
			changed = true
			continue
		}
		child, openErr := directory.OpenDirectory(name)
		if openErr == nil {
			childChanged, childErr := removeTreeContents(ctx, child)
			closeErr := child.Close()
			changed = changed || childChanged
			if childErr != nil {
				return changed, fmt.Errorf("remove model asset directory %q: %w", name, childErr)
			}
			if closeErr != nil {
				return changed, fmt.Errorf("close model asset directory %q: %w", name, closeErr)
			}
			if err := removalContextError(ctx); err != nil {
				return changed, err
			}
			if err := directory.Remove(name); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return changed, fmt.Errorf("remove model asset directory %q: %w", name, err)
			}
			changed = true
			continue
		}

		if errors.Is(openErr, os.ErrNotExist) {
			continue
		}
		if err := removalContextError(ctx); err != nil {
			return changed, err
		}
		// Opening with Root.OpenRoot is deliberately no-follow. If the
		// entry is a file or a link/reparse point, remove only that entry.
		if err := directory.Remove(name); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return changed, fmt.Errorf("remove model asset %q: %w", name, err)
		}
		changed = true
	}
	return changed, nil
}

func validateTreeTargetName(name string) error {
	if strings.TrimSpace(name) == "" || name == "." || name == ".." ||
		filepath.IsAbs(name) || filepath.VolumeName(name) != "" ||
		filepath.Base(name) != name {
		return fmt.Errorf("invalid model asset cache target name %q", name)
	}
	return validateTreeEntryName(name)
}

func validateTreeEntryName(name string) error {
	if strings.TrimSpace(name) == "" || name == "." || name == ".." ||
		filepath.IsAbs(name) || filepath.VolumeName(name) != "" || filepath.Base(name) != name {
		return fmt.Errorf("invalid model asset cache entry name %q", name)
	}
	return nil
}

func removalContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
