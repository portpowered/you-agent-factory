package interfaces

import "testing"

type runtimeLookupDefinitionStub struct {
	workers      map[string]*WorkerConfig
	workstations map[string]*FactoryWorkstationConfig
}

func (s *runtimeLookupDefinitionStub) Worker(name string) (*WorkerConfig, bool) {
	worker, ok := s.workers[name]
	return worker, ok
}

func (s *runtimeLookupDefinitionStub) Workstation(name string) (*FactoryWorkstationConfig, bool) {
	workstation, ok := s.workstations[name]
	return workstation, ok
}

type runtimeLookupWorkstationStub struct {
	workstations map[string]*FactoryWorkstationConfig
}

func (s *runtimeLookupWorkstationStub) Workstation(name string) (*FactoryWorkstationConfig, bool) {
	workstation, ok := s.workstations[name]
	return workstation, ok
}

func TestFirstRuntimeDefinitionLookup_ReturnsFirstNonNilCandidate(t *testing.T) {
	t.Parallel()

	first := &runtimeLookupDefinitionStub{
		workers: map[string]*WorkerConfig{
			"planner": {Type: "planner"},
		},
	}
	second := &runtimeLookupDefinitionStub{
		workers: map[string]*WorkerConfig{
			"reviewer": {Type: "reviewer"},
		},
	}

	got := FirstRuntimeDefinitionLookup(nil, first, second)
	if got != first {
		t.Fatalf("FirstRuntimeDefinitionLookup() returned %p, want first non-nil candidate %p", got, first)
	}

	worker, ok := got.Worker("planner")
	if !ok || worker == nil || worker.Type != "planner" {
		t.Fatalf("FirstRuntimeDefinitionLookup() did not preserve the selected lookup behavior, got worker=%#v ok=%v", worker, ok)
	}
}

func TestFirstRuntimeDefinitionLookup_ReturnsNilWhenEveryCandidateIsNil(t *testing.T) {
	t.Parallel()

	if got := FirstRuntimeDefinitionLookup(nil, nil); got != nil {
		t.Fatalf("FirstRuntimeDefinitionLookup() = %p, want nil", got)
	}
}

func TestFirstRuntimeWorkstationLookup_ReturnsFirstNonNilCandidate(t *testing.T) {
	t.Parallel()

	first := &runtimeLookupWorkstationStub{
		workstations: map[string]*FactoryWorkstationConfig{
			"review": {Name: "review"},
		},
	}
	second := &runtimeLookupWorkstationStub{
		workstations: map[string]*FactoryWorkstationConfig{
			"publish": {Name: "publish"},
		},
	}

	got := FirstRuntimeWorkstationLookup(nil, first, second)
	if got != first {
		t.Fatalf("FirstRuntimeWorkstationLookup() returned %p, want first non-nil candidate %p", got, first)
	}

	workstation, ok := got.Workstation("review")
	if !ok || workstation == nil || workstation.Name != "review" {
		t.Fatalf("FirstRuntimeWorkstationLookup() did not preserve the selected lookup behavior, got workstation=%#v ok=%v", workstation, ok)
	}
}

func TestFirstRuntimeWorkstationLookup_ReturnsNilWhenEveryCandidateIsNil(t *testing.T) {
	t.Parallel()

	if got := FirstRuntimeWorkstationLookup(nil, nil); got != nil {
		t.Fatalf("FirstRuntimeWorkstationLookup() = %p, want nil", got)
	}
}
