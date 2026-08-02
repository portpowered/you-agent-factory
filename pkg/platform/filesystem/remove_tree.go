package filesystem

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var errSecureTreeRemovalUnsupported = errors.New("secure tree removal is unsupported on this platform")

// RemoveTreeState is the platform boundary's explicit completion state.
type RemoveTreeState string

const (
	RemoveTreeNotAttempted RemoveTreeState = "NOT_ATTEMPTED"
	RemoveTreeAbsent       RemoveTreeState = "ABSENT"
	RemoveTreeRemoved      RemoveTreeState = "REMOVED"
	RemoveTreeRemaining    RemoveTreeState = "REMAINING"
	RemoveTreeUnknown      RemoveTreeState = "UNKNOWN"
)

// RemoveTreeResult reports whether a selected tree is absent, removed,
// remaining for retry, or in an unknown final state.
type RemoveTreeResult struct {
	State RemoveTreeState
}

// RemoveTree removes one selected directory beneath a caller-owned parent.
// The platform implementation owns the security boundary: it opens every
// parent component without following links, deletes opened objects relative to
// their containing handles, and uses the platform's final disposition/remove
// operation rather than resolving a path recursively after a mutation race.
//
// The operation is cooperative: cancellation is observed before every
// destructive boundary and before returning an absence observation. If an
// error follows a mutation, the returned state identifies whether the target
// remains retryable or its final state is unknown.
func (Local) RemoveTree(
	ctx context.Context,
	parentDirectory string,
	targetName string,
) (RemoveTreeResult, error) {
	if err := removalContextError(ctx); err != nil {
		return RemoveTreeResult{State: RemoveTreeNotAttempted}, err
	}
	if err := validateTreeTargetName(targetName); err != nil {
		return RemoveTreeResult{State: RemoveTreeNotAttempted}, err
	}
	if strings.TrimSpace(parentDirectory) == "" {
		return RemoveTreeResult{State: RemoveTreeNotAttempted}, fmt.Errorf("tree parent directory is required")
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
