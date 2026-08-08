package catalog

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// TestEveryPackagedFactoryIsInvocableFromASingleTextPrompt is the standing
// guard for ACP invocability.
//
// A transport that carries only unstructured text -- which is every ACP
// session/prompt -- can fill exactly one invocation parameter: the one bound
// to positional slot one, or failing that a stdin binding. A packaged Factory
// that marks any other parameter required is therefore reachable from the CLI
// and the API but permanently unreachable over ACP, with no diagnostic that
// explains why.
//
// @you/loop, @you/spawn, and @you/tournament each shipped in exactly that
// state (--every, --count, --rounds). This asserts the general property rather
// than those three names, so a future packaged Factory cannot reintroduce it.
func TestEveryPackagedFactoryIsInvocableFromASingleTextPrompt(t *testing.T) {
	inventory, err := packagedfactorycatalog.Discover(
		context.Background(),
		packagedfactories.Source(),
		"factories",
	)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(inventory.Entries) == 0 {
		t.Fatal("packaged Factory inventory is empty")
	}

	checked := 0
	for _, entry := range inventory.Entries {
		if entry.Factory == nil || entry.Factory.InvocationSignature == nil {
			continue
		}
		checked++
		for _, parameter := range entry.Factory.InvocationSignature.Parameters {
			if !parameter.Required {
				continue
			}
			if parameterAcceptsUnstructuredText(parameter) {
				continue
			}
			if parameterHasDefault(parameter) {
				t.Errorf(
					"%s parameter %q is required with no text-carrying binding; it has a default, "+
						"so drop `required` instead of relying on the default to mask it",
					entry.Factory.Name, parameter.Name,
				)
				continue
			}
			t.Errorf(
				"%s parameter %q is required but has no POSITIONAL(1) or STDIN binding and no default, "+
					"so the Factory cannot be invoked from a single text prompt (ACP). Give it a default "+
					"or a text-carrying binding.",
				entry.Factory.Name, parameter.Name,
			)
		}
	}
	if checked == 0 {
		t.Fatal("no packaged Factory declared an invocation signature")
	}
}

// parameterAcceptsUnstructuredText reports whether a transport carrying only
// free text can supply this parameter. Positional slot one is the primary
// carrier; a stdin binding is the documented fallback.
func parameterAcceptsUnstructuredText(parameter factorydefinitions.InvocationParameterConfig) bool {
	for _, binding := range parameter.Bindings {
		switch binding.Kind {
		case "STDIN":
			return true
		case "POSITIONAL":
			if binding.Position == 1 {
				return true
			}
		}
	}
	return false
}

func parameterHasDefault(parameter factorydefinitions.InvocationParameterConfig) bool {
	return parameter.DefaultValue != "" || len(parameter.DefaultValues) > 0
}
