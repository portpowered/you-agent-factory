package definitions

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFactoryValidationDocsCommandDescribesStaticGate(t *testing.T) {
	inputs := support.FakeInputs(t.Context(), []string{"you", "docs", "factory-validation"})
	inputs.Input.Env = isolatedHomeEnvironment(t)
	inputs.Input.WorkingDirectory = t.TempDir()

	if err := buildDefinitionsProcess(t).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(docs factory-validation) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	for _, want := range []string{
		"# Factory Validation",
		"required static gate between authoring",
		"start a Factory Session or scheduler",
		"invoke a worker, model, or provider",
		"create or update a persisted named Factory",
		"./examples/basic/factory/factory.json",
		"./packages/packaged-factories/factories/classify/factory.yaml",
		"./examples/basic/factory",
		"exactly one regular Factory",
		"SAME_NAME",
		"ALL_CHILDREN_COMPLETE",
		"at most two",
		"Each workstation input has a `guards[]` array with at most one entry",
		"classificationRoutes",
		"./docs/examples/factory-validation/unsupported-three-input-join.json",
		"Factory validation failed.",
		"observed arity is 3 inputs",
		"at most 2 inputs are supported",
		"supported-two-input-join.json",
		"Factory validation passed.",
	} {
		if !strings.Contains(inputs.Stdout(), want) {
			t.Fatalf("you docs factory-validation missing marker %q:\n%s", want, inputs.Stdout())
		}
	}

	for _, stale := range []string{"you config validate", "you factory validate", "Each workstation input has one optional `guard` object"} {
		if strings.Contains(inputs.Stdout(), stale) {
			t.Fatalf("you docs factory-validation contains stale wording %q:\n%s", stale, inputs.Stdout())
		}
	}
}
