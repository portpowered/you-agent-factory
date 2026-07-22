package operatorsettings

import (
	"strings"
	"testing"
)

// Hermetic S02 failure-baseline fixtures for one-shot run operator-default
// resolution when the symbolic DEFAULT provider cannot resolve to a concrete value.

func TestFailureBaseline_AbsentDefault_ResolveRejectsSymbolicDefaultWithoutConcreteProvider(t *testing.T) {
	_, err := Resolve(ResolveInput{
		Flag: Defaults{WorkerModelProvider: "DEFAULT"},
	}, "/tmp/config.json")
	if err == nil {
		t.Fatal("expected unresolved DEFAULT error")
	}
	if !strings.Contains(err.Error(), "DEFAULT requires a concrete provider") {
		t.Fatalf("error = %q, want unresolved DEFAULT guidance", err.Error())
	}
	if !strings.Contains(err.Error(), "YOU_DEFAULT_WORKER_MODEL_PROVIDER") {
		t.Fatalf("error = %q, want environment override guidance", err.Error())
	}
}
