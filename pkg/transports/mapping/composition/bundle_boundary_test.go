package composition

import (
	"os"
	"strings"
	"testing"
)

func TestProductionMappingsDoNotInspectProductServiceBundle(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read mapping composition package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		for _, forbidden := range []string{
			"pkg/services/bundle",
			"productbundle.Bundle",
			"bundle.Bundle",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s contains aggregate product-service lookup %q; mappings must receive exact roles", entry.Name(), forbidden)
			}
		}
	}
}
