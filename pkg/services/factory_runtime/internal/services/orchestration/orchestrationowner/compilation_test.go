package orchestrationowner_test

import (
	"context"
	"fmt"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimeorchestrationowner "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner"
)

func TestNewCompilationRequiresIDGenerator(t *testing.T) {
	t.Parallel()

	if got := factoryruntimeorchestrationowner.NewCompilation(nil, nil, nil); got != nil {
		t.Fatalf("NewCompilation(nil) = %#v, want nil", got)
	}
}

func TestNewCompilationCompilesPetriFactory(t *testing.T) {
	t.Parallel()

	compiler := factoryruntimeorchestrationowner.NewCompilation(
		testIDGenerator(),
		nil,
		nil,
	)
	cfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
		}},
	}
	net, err := compiler.CompilePetriNet(context.Background(), factoryruntime.OrchestrationCompileRequest{
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("CompilePetriNet() error = %v", err)
	}
	if net == nil {
		t.Fatal("CompilePetriNet() net = nil, want compiled Petri net")
	}
}

func testIDGenerator() factoryruntime.IDGenerator {
	next := 0
	return func() string {
		next++
		return fmt.Sprintf("orchestration-owner-test-id-%d", next)
	}
}
