package runtimeapplication

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

type typedNilLifecycleComponent struct{}

func (*typedNilLifecycleComponent) Start(context.Context) error { return nil }
func (*typedNilLifecycleComponent) Stop(context.Context) error  { return nil }

func TestRuntimeApplicationCarriesExactRolesInsteadOfProductBundle(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read runtime application package: %v", err)
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
			"pkg/services/bundle",
			"ProductServices",
			"productbundle.Bundle",
			"factorysessions.OpenedRuntime",
			"type OpenedRuntime struct",
			"ScopeAttacher",
			"AttachApplication",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s contains aggregate product-service lookup %q; runtime scopes must carry exact roles", entry.Name(), forbidden)
			}
		}
	}
}

func TestManagedRunnerFailsClosedWithTypedNilRequiredComponent(t *testing.T) {
	var transport *typedNilLifecycleComponent
	if _, err := NewManagedRunner(lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "transport", Component: transport, Primary: true,
	}}}, runtimeartifact.Diagnostics{}); err == nil ||
		!strings.Contains(err.Error(), "transport") {
		t.Fatalf("typed nil component error = %v", err)
	}
}
