//go:build aix || android || darwin || dragonfly || freebsd || hurd || illumos || ios || linux || netbsd || openbsd || solaris

package filesystem

import "context"

const secureTreeRemovalSupported = false

// POSIX does not provide an identity-safe unlink/rmdir operation against an
// already-open directory handle. Linux can atomically detach a name, but its
// final directory removal remains a name-based operation and can race a
// replacement. All Unix targets therefore fail closed until an equivalent
// identity-preserving detach/delete implementation is available.
func removeTreePlatform(ctx context.Context, _ string, _ string) (bool, error) {
	if err := removalContextError(ctx); err != nil {
		return false, err
	}
	return false, errSecureTreeRemovalUnsupported
}
