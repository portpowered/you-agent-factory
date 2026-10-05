package orchestrationowner_test

import (
	"context"
	"errors"
	"fmt"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/definitionmapping"
	orchestrationwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/wire"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimeorchestrationowner "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner"
)

type compileOwner struct {
	orchestration.Service
	request orchestration.CompileRequest
	failure error
}

func (owner *compileOwner) Compile(_ context.Context, request orchestration.CompileRequest) (orchestration.CompileResult, error) {
	owner.request = request
	return orchestration.CompileResult{}, owner.failure
}
func TestNewCompilationUsesSuppliedOwner(t *testing.T) {
	t.Parallel()
	failure := errors.New("owned compile failure")
	owner := &compileOwner{failure: failure}
	compiler := factoryruntimeorchestrationowner.NewCompilation(owner)
	config := &factorydefinitions.FactoryConfig{}
	_, err := compiler.Compile(context.Background(), factoryruntime.OrchestrationCompileRequest{Config: config, FactoryDir: "factory"})
	if !errors.Is(err, failure) || owner.request.Config != config || owner.request.FactoryDir != "factory" {
		t.Fatalf("Compile = %v, request = %#v; want supplied owner's failure and exact definition", err, owner.request)
	}
}

func TestNewCompilationCompilesPetriFactory(t *testing.T) {
	t.Parallel()

	compiler := factoryruntimeorchestrationowner.NewCompilation(orchestrationwire.New(testDefinitionMapper(t), nil, nil))
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

func testDefinitionMapper(t *testing.T) *definitionmapping.Mapper {
	t.Helper()
	mapper, err := definitionmapping.New(testIDGenerator())
	if err != nil {
		t.Fatal(err)
	}
	return mapper
}
