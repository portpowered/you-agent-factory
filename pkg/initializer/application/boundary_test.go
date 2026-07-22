package application

import (
	"os"
	"strings"
	"testing"
)

func TestInitializerApplicationDoesNotConstructConcreteTransportsOrMappingAdapters(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read application initializer package: %v", err)
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
			"github.com/portpowered/infinite-you/pkg/services/",
			"transporthttp.NewServer",
			"mcpserver.New",
			"factorydefinitionmapping.New",
			"factorysessionmapping.New",
			"modelshttp.New",
			"FactoryServiceBuilder",
			"InvocationBootstrapBuilder",
			"ProductServices",
			"pkg/services/bundle",
			"lifecycle.New",
			"runtimeapplication.New",
			"httpapplication.New",
			"mcpstdio.New",
			"ApplyRuntimeOverrides",
			"serviceedges.Merge(",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s contains initializer-owned construction %q", entry.Name(), forbidden)
			}
		}
	}
}

func TestInitializerApplicationHasNoRuntimeScopeOpeningPath(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("builders.go")
	if err != nil {
		t.Fatalf("read builders.go: %v", err)
	}
	for _, forbidden := range []string{
		"NewRuntimeScopeOpener",
		"RuntimeScopeOpener",
		"OpenApplicationRuntime(",
		"ApplicationRuntimeAdapter",
		"RuntimeInputResolver",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("builders.go contains operation-time runtime assembly %q", forbidden)
		}
	}
}
