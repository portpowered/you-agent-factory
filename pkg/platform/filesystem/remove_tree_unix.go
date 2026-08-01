//go:build aix || android || darwin || dragonfly || freebsd || hurd || illumos || ios || linux || netbsd || openbsd || solaris

package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// openTreeParent walks from the filesystem root with no-follow, handle-relative
// opens. The initial os.OpenRoot call is safe because the filesystem root is a
// mount boundary, not a user-controlled path component.
func openTreeParent(name string) (*os.Root, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return nil, fmt.Errorf("make model asset cache parent absolute: %w", err)
	}
	root, err := os.OpenRoot(string(filepath.Separator))
	if err != nil {
		return nil, fmt.Errorf("open filesystem root for model asset cache: %w", err)
	}
	relative, err := filepath.Rel(string(filepath.Separator), filepath.Clean(absolute))
	if err != nil {
		root.Close()
		return nil, fmt.Errorf("relativize model asset cache parent: %w", err)
	}
	if relative == "." {
		return root, nil
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		next, err := root.OpenRoot(component)
		if err != nil {
			root.Close()
			return nil, err
		}
		root.Close()
		root = next
	}
	return root, nil
}
