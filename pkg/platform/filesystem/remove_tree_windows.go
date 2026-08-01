//go:build windows

package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// openTreeParent starts at a volume root and walks each user-controlled path
// component with Root.OpenRoot. Windows Root.OpenRoot uses a handle-relative
// no-reparse open, so a replaced junction/reparse point cannot redirect the
// deletion outside the selected cache volume.
func openTreeParent(name string) (*os.Root, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return nil, fmt.Errorf("make model asset cache parent absolute: %w", err)
	}
	absolute = filepath.Clean(absolute)
	volume := filepath.VolumeName(absolute)
	if volume == "" {
		return nil, fmt.Errorf("model asset cache parent has no volume")
	}
	volumeRoot := volume + string(filepath.Separator)
	root, err := os.OpenRoot(volumeRoot)
	if err != nil {
		return nil, fmt.Errorf("open model asset cache volume root: %w", err)
	}
	relative, err := filepath.Rel(volumeRoot, absolute)
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
