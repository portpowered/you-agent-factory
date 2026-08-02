//go:build darwin || linux

package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const secureTreeRemovalSupported = true

const (
	unixMaxRemovalDepth     = 128
	unixMaxRemovalEntries   = 100_000
	unixMaxRemovalNameBytes = 16 * 1024 * 1024
	unixReadDirectoryBatch  = 256
)

type unixRemovalBudget struct {
	entries   int
	nameBytes int
}

type unixRemovalProgress struct {
	uncertain bool
}

// removeTreePlatform keeps the parent and target as directory handles for the
// complete operation. Every descendant is opened or removed relative to an
// already-open directory; no path-recursive fallback is permitted here.
//
// POSIX has no portable unlink-by-file-descriptor operation. The final name
// removal is therefore preceded by an identity check and is intentionally
// documented as fail-closed on detected replacement, while the remaining
// threat assumption is that an attacker cannot replace the final name in the
// small interval between that check and unlinkat/rmdir.
func removeTreePlatform(
	ctx context.Context,
	parentName string,
	targetName string,
) (result RemoveTreeResult, err error) {
	result.State = RemoveTreeNotAttempted
	parent, err := openTreeParent(parentName)
	if errors.Is(err, os.ErrNotExist) {
		if cancelErr := removalContextError(ctx); cancelErr != nil {
			return result, cancelErr
		}
		return RemoveTreeResult{State: RemoveTreeAbsent}, nil
	}
	if err != nil {
		return result, fmt.Errorf("open tree parent without symlink traversal: %w", err)
	}
	parentClosed := false
	defer func() {
		if parentClosed {
			return
		}
		if closeErr := parent.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close tree parent: %w", closeErr))
			result.State = RemoveTreeUnknown
		}
	}()

	targetInfo, err := parent.Lstat(targetName)
	if errors.Is(err, os.ErrNotExist) {
		if cancelErr := removalContextError(ctx); cancelErr != nil {
			return result, cancelErr
		}
		return RemoveTreeResult{State: RemoveTreeAbsent}, nil
	}
	if err != nil {
		return result, fmt.Errorf("inspect tree target without following links: %w", err)
	}
	if targetInfo.Mode()&os.ModeSymlink != 0 || !targetInfo.IsDir() {
		return result, fmt.Errorf("tree target is not a non-link directory")
	}
	if err := removalContextError(ctx); err != nil {
		return result, err
	}
	target, err := parent.OpenRoot(targetName)
	if err != nil {
		return result, fmt.Errorf("open tree target without following links: %w", err)
	}
	targetClosed := false
	defer func() {
		if targetClosed {
			return
		}
		if closeErr := target.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close tree target: %w", closeErr))
			result.State = RemoveTreeUnknown
		}
	}()
	openedInfo, err := target.Stat(".")
	if err != nil {
		return result, fmt.Errorf("identify opened tree target: %w", err)
	}
	if !os.SameFile(targetInfo, openedInfo) {
		return result, fmt.Errorf("tree target changed during secure open")
	}

	budget := unixRemovalBudget{}
	progress, err := removeUnixContents(ctx, target, 0, &budget)
	if err != nil {
		if progress.uncertain {
			result.State = RemoveTreeUnknown
		} else {
			result.State = RemoveTreeRemaining
		}
		return result, err
	}
	if err := removalContextError(ctx); err != nil {
		return RemoveTreeResult{State: RemoveTreeRemaining}, err
	}
	currentInfo, err := parent.Lstat(targetName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RemoveTreeResult{State: RemoveTreeUnknown}, fmt.Errorf("tree target disappeared before final removal")
		}
		return RemoveTreeResult{State: RemoveTreeRemaining}, fmt.Errorf("recheck tree target identity: %w", err)
	}
	if !os.SameFile(targetInfo, currentInfo) {
		return RemoveTreeResult{State: RemoveTreeRemaining}, fmt.Errorf("tree target was replaced before final removal")
	}
	if err := target.Close(); err != nil {
		targetClosed = true
		return RemoveTreeResult{State: RemoveTreeUnknown}, fmt.Errorf("close tree target before final removal: %w", err)
	}
	targetClosed = true
	if err := removalContextError(ctx); err != nil {
		return RemoveTreeResult{State: RemoveTreeRemaining}, err
	}
	if err := parent.Remove(targetName); err != nil {
		return RemoveTreeResult{State: RemoveTreeRemaining}, fmt.Errorf("remove tree target: %w", err)
	}
	result.State = RemoveTreeRemoved
	if err := removalContextError(ctx); err != nil {
		return result, err
	}
	return result, nil
}

// openTreeParent resolves each component from an open root. Closing the prior
// root after opening its child is safe because the child root owns its own
// directory descriptor; the returned root is the caller's stable parent
// handle, not a path string.
func openTreeParent(name string) (*os.Root, error) {
	absolute, err := filepath.Abs(filepath.Clean(name))
	if err != nil {
		return nil, fmt.Errorf("make tree parent absolute: %w", err)
	}
	root, err := os.OpenRoot(string(filepath.Separator))
	if err != nil {
		return nil, fmt.Errorf("open filesystem root for tree parent: %w", err)
	}
	relative, err := filepath.Rel(string(filepath.Separator), absolute)
	if err != nil {
		return nil, closeUnixRootOnError(root, fmt.Errorf("relativize tree parent: %w", err))
	}
	if relative == "." {
		return root, nil
	}
	components := strings.Split(relative, string(filepath.Separator))
	if len(components) > unixMaxRemovalDepth {
		return nil, closeUnixRootOnError(root, fmt.Errorf("tree parent depth limit exceeded"))
	}
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return nil, closeUnixRootOnError(root, fmt.Errorf("invalid tree parent component"))
		}
		before, statErr := root.Lstat(component)
		if statErr != nil {
			return nil, closeUnixRootOnError(root, statErr)
		}
		if before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
			return nil, closeUnixRootOnError(root, fmt.Errorf("tree parent component is not a non-link directory"))
		}
		next, openErr := root.OpenRoot(component)
		if openErr != nil {
			return nil, closeUnixRootOnError(root, openErr)
		}
		after, statErr := next.Stat(".")
		if statErr != nil {
			return nil, closeUnixRootsOnError(next, root, fmt.Errorf("identify tree parent component: %w", statErr))
		}
		if !os.SameFile(before, after) {
			return nil, closeUnixRootsOnError(next, root, fmt.Errorf("tree parent component changed during secure open"))
		}
		if closeErr := root.Close(); closeErr != nil {
			return nil, closeUnixRootsOnError(next, nil, fmt.Errorf("close prior tree parent handle: %w", closeErr))
		}
		root = next
	}
	return root, nil
}

func closeUnixRootOnError(root *os.Root, err error) error {
	if root == nil {
		return err
	}
	return errors.Join(err, root.Close())
}

func closeUnixRootsOnError(first, second *os.Root, err error) error {
	if first != nil {
		err = errors.Join(err, first.Close())
	}
	if second != nil {
		err = errors.Join(err, second.Close())
	}
	return err
}

func removeUnixContents(
	ctx context.Context,
	directory *os.Root,
	depth int,
	budget *unixRemovalBudget,
) (unixRemovalProgress, error) {
	progress := unixRemovalProgress{}
	if err := removalContextError(ctx); err != nil {
		return progress, err
	}
	entries, err := readUnixEntries(directory, budget)
	if err != nil {
		return progress, fmt.Errorf("read tree directory: %w", err)
	}
	if depth >= unixMaxRemovalDepth && len(entries) > 0 {
		return progress, fmt.Errorf("tree removal depth limit exceeded")
	}
	for _, entry := range entries {
		if err := removalContextError(ctx); err != nil {
			return progress, err
		}
		before, err := directory.Lstat(entry)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return progress, fmt.Errorf("inspect tree entry: %w", err)
		}
		if before.Mode()&os.ModeSymlink == 0 && before.IsDir() {
			child, openErr := directory.OpenRoot(entry)
			if openErr != nil {
				return progress, fmt.Errorf("open tree directory without following links: %w", openErr)
			}
			opened, statErr := child.Stat(".")
			if statErr != nil {
				return progress, closeUnixRootOnError(child, fmt.Errorf("identify opened tree directory: %w", statErr))
			}
			if !os.SameFile(before, opened) {
				return progress, closeUnixRootOnError(child, fmt.Errorf("tree directory changed during secure open"))
			}
			childProgress, childErr := removeUnixContents(ctx, child, depth+1, budget)
			closeErr := child.Close()
			if childErr != nil {
				if closeErr != nil {
					childProgress.uncertain = true
					childErr = errors.Join(childErr, fmt.Errorf("close tree directory: %w", closeErr))
				}
				progress.uncertain = progress.uncertain || childProgress.uncertain
				return progress, childErr
			}
			if closeErr != nil {
				childProgress.uncertain = true
				progress.uncertain = progress.uncertain || childProgress.uncertain
				return progress, fmt.Errorf("close tree directory: %w", closeErr)
			}
			progress.uncertain = progress.uncertain || childProgress.uncertain
		}
		if err := removalContextError(ctx); err != nil {
			return progress, err
		}
		after, err := directory.Lstat(entry)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return progress, fmt.Errorf("tree entry disappeared before removal")
			}
			return progress, fmt.Errorf("recheck tree entry identity: %w", err)
		}
		if !os.SameFile(before, after) {
			return progress, fmt.Errorf("tree entry was replaced before removal")
		}
		if err := directory.Remove(entry); err != nil {
			return progress, fmt.Errorf("remove tree entry: %w", err)
		}
	}
	return progress, nil
}

func readUnixEntries(directory *os.Root, budget *unixRemovalBudget) (entries []string, err error) {
	file, err := directory.Open(".")
	if err != nil {
		return nil, err
	}
	closed := false
	defer func() {
		if !closed {
			if closeErr := file.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close tree directory reader: %w", closeErr))
			}
		}
	}()
	entries = make([]string, 0)
	for {
		batch, readErr := file.ReadDir(unixReadDirectoryBatch)
		for _, entry := range batch {
			name := entry.Name()
			if err := validateTreeEntryName(name); err != nil {
				return nil, err
			}
			if err := budget.add(name); err != nil {
				return nil, err
			}
			entries = append(entries, name)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, readErr
		}
		if len(batch) == 0 {
			break
		}
	}
	if closeErr := file.Close(); closeErr != nil {
		closed = true
		return nil, closeErr
	}
	closed = true
	sort.Strings(entries)
	return entries, nil
}

func (budget *unixRemovalBudget) add(name string) error {
	if budget == nil {
		return fmt.Errorf("tree removal budget is missing")
	}
	budget.entries++
	budget.nameBytes += len(name)
	if budget.entries > unixMaxRemovalEntries {
		return fmt.Errorf("tree removal entry limit exceeded")
	}
	if budget.nameBytes > unixMaxRemovalNameBytes {
		return fmt.Errorf("tree removal name-memory limit exceeded")
	}
	return nil
}
