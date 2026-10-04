package backendsizecheck

import (
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractguard"
)

func ShouldSkipDir(scanRoot, path string) bool {
	rootName := filepath.Base(filepath.Clean(scanRoot))
	switch filepath.ToSlash(rootName) {
	case "cmd", "internal":
		return contractguard.ShouldSkipDir(scanRoot, path)
	case "pkg":
		return shouldSkipByRules(scanRoot, path, []string{
			"transports/http/generated",
			"transports/http/client",
			"testdata",
		})
	case "tests":
		return shouldSkipByRules(scanRoot, path, []string{"testdata"})
	case "vendor":
		return true
	default:
		return false
	}
}

func shouldSkipByRules(scanRoot string, path string, explicitSkips []string) bool {
	if contractguard.ShouldSkipDir(scanRoot, path, explicitSkips...) {
		return true
	}

	relativePath, err := filepath.Rel(scanRoot, path)
	if err != nil {
		return false
	}
	relativePath = filepath.ToSlash(filepath.Clean(relativePath))
	if relativePath == "." {
		return false
	}

	for _, segment := range strings.Split(relativePath, "/") {
		if segment == "testdata" {
			return true
		}
	}
	return false
}
