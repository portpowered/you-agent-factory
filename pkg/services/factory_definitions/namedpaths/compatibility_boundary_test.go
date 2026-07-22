package namedpaths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformNamedFactoryPathContainsNoLegacyCompatibilityAPI(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob package sources: %v", err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if strings.Contains(string(source), "LegacyLayout") {
			t.Fatalf("%s recreates legacy named-Factory compatibility policy in Platform", file)
		}
	}
}
