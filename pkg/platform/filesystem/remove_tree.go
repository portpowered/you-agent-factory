package filesystem

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var errSecureTreeRemovalUnsupported = errors.New("secure tree removal is unsupported on this platform")

// RemoveTree removes one selected directory beneath a caller-owned parent.
// The platform implementation owns the security boundary: it opens every
// parent component without following links, detaches the selected root before
// walking it where the platform can do so safely, and deletes opened objects
// rather than resolving a path after a mutation race.
//
// A true changed result means that at least one filesystem mutation occurred.
// The operation is cooperative: cancellation is observed before every
// destructive boundary and before returning an absence observation. If an
// error follows a mutation, a platform that supports retryable removal leaves
// its retry state in place and returns changed=true.
func (Local) RemoveTree(
	ctx context.Context,
	parentDirectory string,
	targetName string,
) (bool, error) {
	if err := removalContextError(ctx); err != nil {
		return false, err
	}
	if err := validateTreeTargetName(targetName); err != nil {
		return false, err
	}
	if strings.TrimSpace(parentDirectory) == "" {
		return false, fmt.Errorf("tree parent directory is required")
	}
	return removeTreePlatform(ctx, parentDirectory, targetName)
}

func validateTreeTargetName(name string) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name || strings.ContainsRune(name, '\x00') ||
		name == "." || name == ".." ||
		strings.ContainsAny(name, `/\\:`) || filepath.IsAbs(name) ||
		filepath.VolumeName(name) != "" || filepath.Base(name) != name {
		return fmt.Errorf("invalid tree target name %q", name)
	}
	return validateTreeEntryName(name)
}

func validateTreeEntryName(name string) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name || strings.ContainsRune(name, '\x00') ||
		name == "." || name == ".." ||
		strings.ContainsAny(name, `/\\:`) || filepath.IsAbs(name) ||
		filepath.VolumeName(name) != "" || filepath.Base(name) != name {
		return fmt.Errorf("invalid tree entry name %q", name)
	}
	return nil
}

func removalContextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return ctx.Err()
}
