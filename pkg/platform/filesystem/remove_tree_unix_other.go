//go:build aix || android || dragonfly || freebsd || hurd || illumos || ios || netbsd || openbsd || solaris

package filesystem

import (
	"context"
)

const secureTreeRemovalSupported = false

// This Unix target is not covered by the native handle-relative implementation
// used on Linux and Darwin, so it fails closed rather than falling back to a
// path-recursive or preflight-only removal.
func removeTreePlatform(ctx context.Context, _ string, _ string) (RemoveTreeResult, error) {
	if err := removalContextError(ctx); err != nil {
		return RemoveTreeResult{State: RemoveTreeNotAttempted}, err
	}
	return RemoveTreeResult{State: RemoveTreeNotAttempted}, errSecureTreeRemovalUnsupported
}
