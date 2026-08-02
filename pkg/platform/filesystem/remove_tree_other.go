//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !hurd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows

package filesystem

import (
	"context"
)

const secureTreeRemovalSupported = false

func removeTreePlatform(ctx context.Context, _ string, _ string) (RemoveTreeResult, error) {
	if err := removalContextError(ctx); err != nil {
		return RemoveTreeResult{State: RemoveTreeNotAttempted}, err
	}
	return RemoveTreeResult{State: RemoveTreeNotAttempted}, errSecureTreeRemovalUnsupported
}
