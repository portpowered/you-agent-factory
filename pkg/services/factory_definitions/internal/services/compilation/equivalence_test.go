package compilation_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationcanonical "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/canonical"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
	factorydefinitiontestcomposition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/testcomposition"
)

func TestCompilationOwner_AuthoredDirectoryAndCanonicalBytesProduceIdenticalEffectiveOutcome(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	writeAuthoredFactory(t, factoryDir)

	composition := testComposition()
	loader := composition.Loader()
	canonical, err := loader.FlattenFactoryConfig(factoryDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	compilation := newCompilationServiceFromComposition(t, composition)

	fromDirectory, err := compilation.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{FactoryDir: factoryDir},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource(directory): %v", err)
	}

	fromCanonical, err := compilation.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{
			Canonical:  canonical,
			FactoryDir: factoryDir,
		},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource(canonical): %v", err)
	}

	assertEquivalentEffectiveOutcomes(t, fromDirectory, fromCanonical)

	directoryFacts := effectiveWorkerAndWorkstationFacts(t, fromDirectory.Effective.ContentIdentity)
	canonicalFacts := effectiveWorkerAndWorkstationFacts(t, fromCanonical.Effective.ContentIdentity)
	if directoryFacts != canonicalFacts {
		t.Fatalf(
			"merged worker/workstation facts differ: directory=%#v canonical=%#v",
			directoryFacts,
			canonicalFacts,
		)
	}
	if directoryFacts.workerName != "executor" || directoryFacts.workerCommand != "go" {
		t.Fatalf("directory worker facts = %#v, want executor with command go", directoryFacts)
	}
	if directoryFacts.workstationName != "execute-story" || directoryFacts.workstationWorker != "executor" {
		t.Fatalf("directory workstation facts = %#v, want execute-story bound to executor", directoryFacts)
	}
}

func TestCompilationOwner_ByteIdenticalCanonicalPayloadsProduceIdenticalEffectiveOutcome(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	writeAuthoredFactory(t, factoryDir)

	composition := testComposition()
	canonical, err := composition.Loader().FlattenFactoryConfig(factoryDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	compilation := newCompilationServiceFromComposition(t, composition)
	payload := append([]byte(nil), canonical...)

	first, err := compilation.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{Canonical: payload},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource first: %v", err)
	}

	second, err := compilation.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{Canonical: append([]byte(nil), payload...)},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource second: %v", err)
	}

	assertEquivalentEffectiveOutcomes(t, first, second)
}

func TestCompilationOwner_DeliberateAuthoredDifferenceProducesDifferentEffectiveOutcome(t *testing.T) {
	t.Parallel()

	baselineDir := t.TempDir()
	writeAuthoredFactory(t, baselineDir)

	variantDir := t.TempDir()
	writeAuthoredFactoryWithWorkerName(t, variantDir, "runner")

	composition := testComposition()
	compilation := newCompilationServiceFromComposition(t, composition)

	baseline, err := compilation.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{FactoryDir: baselineDir},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource(baseline): %v", err)
	}

	variant, err := compilation.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{FactoryDir: variantDir},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource(variant): %v", err)
	}

	if baseline.Effective.ContentIdentity == "" || variant.Effective.ContentIdentity == "" {
		t.Fatal("ContentIdentity must not be empty for deliberate difference proof")
	}
	if baseline.Effective.ContentIdentity == variant.Effective.ContentIdentity {
		t.Fatalf(
			"deliberate worker rename produced identical ContentIdentity: %q",
			baseline.Effective.ContentIdentity,
		)
	}

	baselineFacts := effectiveWorkerAndWorkstationFacts(t, baseline.Effective.ContentIdentity)
	variantFacts := effectiveWorkerAndWorkstationFacts(t, variant.Effective.ContentIdentity)
	if baselineFacts.workerName == variantFacts.workerName {
		t.Fatalf(
			"worker names should differ after deliberate rename: baseline=%q variant=%q",
			baselineFacts.workerName,
			variantFacts.workerName,
		)
	}
}

func newCompilationServiceFromComposition(
	t *testing.T,
	composition factorydefinitiontestcomposition.Composition,
) compilationservice.Service {
	t.Helper()

	loader := composition.Loader()
	compilation, err := compilationwire.NewService(compilationservice.Dependencies{
		LoadCanonical:      loader.LoadSourceFromCanonicalJSON,
		LoadFromFactoryDir: loader.LoadSourceFromFactoryDir,
		EncodeFactory:      compilationcanonical.EncodeFactoryPort(),
	})
	if err != nil {
		t.Fatalf("compilationwire.NewService: %v", err)
	}
	return compilation
}

type effectiveMergedFacts struct {
	workerName        string
	workerCommand     string
	workstationName   string
	workstationType   string
	workstationWorker string
}

func effectiveWorkerAndWorkstationFacts(t *testing.T, contentIdentity string) effectiveMergedFacts {
	t.Helper()

	var cfg factoryroot.FactoryConfig
	if err := json.Unmarshal([]byte(contentIdentity), &cfg); err != nil {
		t.Fatalf("decode ContentIdentity: %v", err)
	}
	if len(cfg.Workers) != 1 || len(cfg.Workstations) != 1 {
		t.Fatalf("effective config = %#v, want one worker and one workstation", cfg)
	}
	return effectiveMergedFacts{
		workerName:        cfg.Workers[0].Name,
		workerCommand:     cfg.Workers[0].Command,
		workstationName:   cfg.Workstations[0].Name,
		workstationType:   cfg.Workstations[0].Type,
		workstationWorker: cfg.Workstations[0].WorkerTypeName,
	}
}

func assertEquivalentEffectiveOutcomes(
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

func writeAuthoredFactoryWithWorkerName(t *testing.T, factoryDir, workerName string) {
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
  "workers": [{"name": "` + workerName + `"}],
  "workstations": [{
    "name": "execute-story",
    "worker": "` + workerName + `",
    "inputs": [{"workType": "story", "state": "init"}],
    "outputs": [{"workType": "story", "state": "complete"}]
  }]
}`
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(factoryJSON), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
	workerDir := filepath.Join(factoryDir, "workers", workerName)
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
worker: ` + workerName + `
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
