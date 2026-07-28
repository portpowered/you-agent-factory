package factorydefinition_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationcanonical "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/canonical"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
)

// newRootCompileServiceForPeer attaches private compilation behind the public root
// Service. Construction may import owner-local wire; peer exercise below must
// not depend on compilation or other Definitions internals beyond the root Service.
func newRootCompileServiceForPeer(t *testing.T) factoryroot.Service {
	t.Helper()

	composition := factorydefinitioncomposition
	loader := composition.Loader()
	compilation, err := compilationwire.NewService(compilationservice.Dependencies{
		LoadCanonical:      loader.LoadSourceFromCanonicalJSON,
		LoadFromFactoryDir: loader.LoadSourceFromFactoryDir,
		EncodeFactory:      compilationcanonical.EncodeFactoryPort(),
	})
	if err != nil {
		t.Fatalf("compilationwire.NewService: %v", err)
	}
	return factorydefinition.NewWithCompilation(nil, compilation)
}

// peerExerciseRootCompileSuccess proves a peer-shaped consumer can drive
// CTR-DEF compile success cases through the attached private implementation
// while depending only on the root Service vocabulary.
func peerExerciseRootCompileSuccess(
	t *testing.T,
	service factoryroot.Service,
	factoryDir string,
	canonical []byte,
) {
	t.Helper()
	ctx := context.Background()

	fromDirectory, err := service.CompileEffectiveFactorySource(
		ctx,
		factoryroot.CompileEffectiveFactorySourceRequest{FactoryDir: factoryDir},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource(directory): %v", err)
	}

	fromCanonical, err := service.CompileEffectiveFactorySource(
		ctx,
		factoryroot.CompileEffectiveFactorySourceRequest{
			Canonical:  canonical,
			FactoryDir: factoryDir,
		},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource(canonical): %v", err)
	}

	assertPeerEquivalentEffectiveOutcomes(t, fromDirectory, fromCanonical)

	facts := peerEffectiveWorkerAndWorkstationFacts(t, fromDirectory.Effective.ContentIdentity)
	if facts.workerName != "executor" || facts.workerCommand != "go" {
		t.Fatalf("worker facts = %#v, want executor with command go", facts)
	}
	if facts.workstationName != "execute-story" || facts.workstationWorker != "executor" {
		t.Fatalf("workstation facts = %#v, want execute-story bound to executor", facts)
	}
	if fromDirectory.Effective.FactoryDir != factoryDir ||
		fromDirectory.Effective.RuntimeBaseDir != factoryDir {
		t.Fatalf(
			"directory effective = %#v, want factory directory identity %q",
			fromDirectory.Effective,
			factoryDir,
		)
	}
}

// peerExerciseRootCompileTypedFailures proves a peer-shaped consumer can
// distinguish CTR-DEF typed compile failures through the attached private
// implementation using only root vocabulary.
func peerExerciseRootCompileTypedFailures(t *testing.T, service factoryroot.Service) {
	t.Helper()
	ctx := context.Background()

	_, invalidErr := service.CompileEffectiveFactorySource(
		ctx,
		factoryroot.CompileEffectiveFactorySourceRequest{Canonical: []byte("{")},
	)
	if !errors.Is(invalidErr, factoryroot.ErrInvalidAuthoredFactorySource) {
		t.Fatalf(
			"CompileEffectiveFactorySource invalid-source error = %v, want %v",
			invalidErr,
			factoryroot.ErrInvalidAuthoredFactorySource,
		)
	}

	_, unresolvedErr := service.CompileEffectiveFactorySource(
		ctx,
		factoryroot.CompileEffectiveFactorySourceRequest{
			Canonical: []byte(`{"worker":"$unresolved"}`),
		},
	)
	if !errors.Is(unresolvedErr, factoryroot.ErrUnresolvedDefinitionReference) {
		t.Fatalf(
			"CompileEffectiveFactorySource unresolved error = %v, want %v",
			unresolvedErr,
			factoryroot.ErrUnresolvedDefinitionReference,
		)
	}
	if errors.Is(unresolvedErr, factoryroot.ErrInvalidAuthoredFactorySource) {
		t.Fatal("unresolved definition reference must not also match ErrInvalidAuthoredFactorySource")
	}
}

func TestRootCompileEquivalence_CTRDEFSuccessThroughPrivateImplementation(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	writeCompileEquivalenceAuthoredFactory(t, factoryDir)

	composition := factorydefinitioncomposition
	canonical, err := composition.Loader().FlattenFactoryConfig(factoryDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	service := newRootCompileServiceForPeer(t)
	peerExerciseRootCompileSuccess(t, service, factoryDir, canonical)
}

func TestRootCompileEquivalence_CTRDEFTypedFailuresThroughPrivateImplementation(t *testing.T) {
	t.Parallel()

	service := newRootCompileServiceForPeer(t)
	peerExerciseRootCompileTypedFailures(t, service)
}

func TestRootCompileEquivalence_PeerExercisesRootWithoutCompilationImport(t *testing.T) {
	t.Parallel()

	// Owner-local construction attaches private compilation. The peer exercise
	// helpers accept only factoryroot.Service and root request/result/error
	// types, proving a peer can drive the slice end-to-end without importing
	// loading, loadedsource, runtimeconfig, or other Definitions internals.
	factoryDir := t.TempDir()
	writeCompileEquivalenceAuthoredFactory(t, factoryDir)
	canonical, err := factorydefinitioncomposition.Loader().FlattenFactoryConfig(factoryDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	successService := newRootCompileServiceForPeer(t)
	peerExerciseRootCompileSuccess(t, successService, factoryDir, canonical)

	failureService := newRootCompileServiceForPeer(t)
	peerExerciseRootCompileTypedFailures(t, failureService)
}

type peerEffectiveMergedFacts struct {
	workerName        string
	workerCommand     string
	workstationName   string
	workstationWorker string
}

func peerEffectiveWorkerAndWorkstationFacts(t *testing.T, contentIdentity string) peerEffectiveMergedFacts {
	t.Helper()

	var cfg factoryroot.FactoryConfig
	if err := json.Unmarshal([]byte(contentIdentity), &cfg); err != nil {
		t.Fatalf("decode ContentIdentity: %v", err)
	}
	if len(cfg.Workers) != 1 || len(cfg.Workstations) != 1 {
		t.Fatalf("effective config = %#v, want one worker and one workstation", cfg)
	}
	return peerEffectiveMergedFacts{
		workerName:        cfg.Workers[0].Name,
		workerCommand:     cfg.Workers[0].Command,
		workstationName:   cfg.Workstations[0].Name,
		workstationWorker: cfg.Workstations[0].WorkerTypeName,
	}
}

func assertPeerEquivalentEffectiveOutcomes(
	t *testing.T,
	first factoryroot.CompileEffectiveFactorySourceResult,
	second factoryroot.CompileEffectiveFactorySourceResult,
) {
	t.Helper()

	if first.Effective.ContentIdentity == "" || second.Effective.ContentIdentity == "" {
		t.Fatal("ContentIdentity must not be empty")
	}
	if first.Effective.ContentIdentity != second.Effective.ContentIdentity {
		t.Fatalf(
			"equivalent inputs produced different ContentIdentity: %q vs %q",
			first.Effective.ContentIdentity,
			second.Effective.ContentIdentity,
		)
	}
	if first.Effective.FactoryDir != second.Effective.FactoryDir {
		t.Fatalf(
			"equivalent inputs produced different FactoryDir: %q vs %q",
			first.Effective.FactoryDir,
			second.Effective.FactoryDir,
		)
	}
	if first.Effective.RuntimeBaseDir != second.Effective.RuntimeBaseDir {
		t.Fatalf(
			"equivalent inputs produced different RuntimeBaseDir: %q vs %q",
			first.Effective.RuntimeBaseDir,
			second.Effective.RuntimeBaseDir,
		)
	}
}

func writeCompileEquivalenceAuthoredFactory(t *testing.T, factoryDir string) {
	t.Helper()

	factoryJSON := `{
  "name": "factory",
  "workTypes": [{
    "name": "story",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"}
    ]
  }],
  "resources": [],
  "workers": [{"name": "executor"}],
  "workstations": [{
    "name": "execute-story",
    "worker": "executor",
    "inputs": [{"workType": "story", "state": "init"}],
    "outputs": [{"workType": "story", "state": "complete"}]
  }]
}`
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(factoryJSON), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
	workerDir := filepath.Join(factoryDir, "workers", "executor")
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("mkdir worker dir: %v", err)
	}
	workerBody := `---
type: SCRIPT_WORKER
command: go
args: ["test", "./..."]
---
Run tests.
`
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(workerBody), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}
	workstationDir := filepath.Join(factoryDir, "workstations", "execute-story")
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("mkdir workstation dir: %v", err)
	}
	workstationBody := `---
type: MODEL_WORKSTATION
worker: executor
promptFile: prompt.md
---
Implement {{ .WorkID }}.
`
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(workstationBody), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workstationDir, "prompt.md"), []byte("Implement {{ .WorkID }}."), 0o644); err != nil {
		t.Fatalf("write prompt.md: %v", err)
	}
}
