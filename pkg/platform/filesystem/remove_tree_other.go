//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !hurd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows

package filesystem

import (
	"fmt"
	"os"
)

// openTreeParent refuses deletion on platforms without the explicit
// handle-relative implementation. The supported desktop/server platforms use
// the no-follow walkers in remove_tree_unix.go and remove_tree_windows.go.
func openTreeParent(string) (*os.Root, error) {
	return nil, fmt.Errorf("secure model asset tree removal is unsupported on this platform")
}
