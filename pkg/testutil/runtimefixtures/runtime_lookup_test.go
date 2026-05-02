package runtimefixtures

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestRuntimeWorkstationLookupFixture_Workstation(t *testing.T) {
	fixture := RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"build": {Name: "Build", Type: interfaces.WorkstationTypeLogical},
		},
	}

	workstation, ok := fixture.Workstation("build")
	if !ok {
		t.Fatal("Workstation(build) missing, want configured workstation")
	}
	if workstation.Name != "Build" {
		t.Fatalf("Workstation(build).Name = %q, want %q", workstation.Name, "Build")
	}

	workstation, ok = fixture.Workstation("missing")
	if ok || workstation != nil {
		t.Fatalf("Workstation(missing) = (%#v, %t), want (nil, false)", workstation, ok)
	}
}

func TestRuntimeDefinitionLookupFixture_ZeroValueMisses(t *testing.T) {
	fixture := RuntimeDefinitionLookupFixture{}

	worker, ok := fixture.Worker("missing")
	if ok || worker != nil {
		t.Fatalf("Worker(missing) = (%#v, %t), want (nil, false)", worker, ok)
	}

	workstation, ok := fixture.Workstation("missing")
	if ok || workstation != nil {
		t.Fatalf("Workstation(missing) = (%#v, %t), want (nil, false)", workstation, ok)
	}
}

func TestRuntimeConfigLookupFixture_ImplementsLayeredContract(t *testing.T) {
	fixture := RuntimeConfigLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"router": {Name: "Router", Type: interfaces.WorkstationTypeLogical},
		},
		Workers: map[string]*interfaces.WorkerConfig{
			"writer": {Model: "gpt-5.4"},
		},
		FactoryPath: "/tmp/factory",
	}

	worker, ok := fixture.Worker("writer")
	if !ok || worker == nil {
		t.Fatalf("Worker(writer) = (%#v, %t), want configured worker", worker, ok)
	}
	if worker.Model != "gpt-5.4" {
		t.Fatalf("Worker(writer).Model = %q, want %q", worker.Model, "gpt-5.4")
	}

	workstation, ok := fixture.Workstation("router")
	if !ok || workstation == nil {
		t.Fatalf("Workstation(router) = (%#v, %t), want configured workstation", workstation, ok)
	}
	if workstation.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("Workstation(router).Type = %q, want %q", workstation.Type, interfaces.WorkstationTypeLogical)
	}

	if fixture.FactoryDir() != "/tmp/factory" {
		t.Fatalf("FactoryDir() = %q, want %q", fixture.FactoryDir(), "/tmp/factory")
	}
	if fixture.RuntimeBaseDir() != "/tmp/factory" {
		t.Fatalf("RuntimeBaseDir() = %q, want %q fallback", fixture.RuntimeBaseDir(), "/tmp/factory")
	}

	fixture.RuntimeBasePath = "/tmp/runtime"
	if fixture.RuntimeBaseDir() != "/tmp/runtime" {
		t.Fatalf("RuntimeBaseDir() = %q, want %q explicit value", fixture.RuntimeBaseDir(), "/tmp/runtime")
	}
}
