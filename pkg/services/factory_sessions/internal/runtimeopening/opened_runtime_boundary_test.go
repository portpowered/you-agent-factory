package runtimeopening

import (
	"os"
	"strings"
	"testing"
)

func TestRuntimeOpeningExposesOnlyOperationSpecificOpenedViews(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"../../opened_runtime.go", "factory.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, forbidden := range []string{
			"type OpenedRuntime struct",
			") (factorysessions.OpenedRuntime, error)",
			"func (f *Factory) OpenRuntime(",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s exposes broad runtime product %q", path, forbidden)
			}
		}
	}
}
