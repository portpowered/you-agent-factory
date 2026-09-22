//go:build backendconformance

package backendconformance

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backendregistry"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

// TestNoDanglingBackendReferenceConformance proves every shipped inference
// reference has exactly one offline installation path. The repository adapter
// below is the only code that discovers generated files or the checked-in manifest.
func TestNoDanglingBackendReferenceConformance(t *testing.T) {
	t.Parallel()

	inputs, err := repositoryConformanceInputs()
	if err != nil {
		t.Fatalf("collect repository backend references: %v", err)
	}
	if len(inputs.PinnedArtifacts) != 9 {
		t.Fatalf("checked-in default manifest has %d artifacts, want the existing nine-entry baseline", len(inputs.PinnedArtifacts))
	}
	if err := Validate(inputs); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryBackendReferenceCollectorPreservesCustomerSources(t *testing.T) {
	t.Parallel()

	inputs, err := repositoryConformanceInputs()
	if err != nil {
		t.Fatalf("collect repository backend references: %v", err)
	}

	var factoryReference, catalogReference bool
	for _, reference := range inputs.References {
		if strings.Contains(reference.Source, "generated/factories/tts/factory.json") && reference.Identifier != "" {
			factoryReference = true
		}
		if strings.Contains(reference.Source, "BuiltInCatalog.ModelDefinitions") && reference.Identifier == "localai-vibevoice" {
			catalogReference = true
		}
	}
	if !factoryReference {
		t.Fatal("collector did not preserve the generated TTS Factory path for localai-vibevoice")
	}
	if !catalogReference {
		t.Fatal("collector did not preserve the built-in TTS catalog source")
	}
}

func TestWindowsCUDAVariantFixtureTraversesRepositoryConformanceSpine(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("../artifacts/testdata/windows-cuda-variant-manifest.json")
	if err != nil {
		t.Fatalf("read exact Windows CUDA fixture: %v", err)
	}
	manifest, err := artifacts.Decode(fixture)
	if err != nil {
		t.Fatalf("Decode(exact Windows CUDA fixture): %v", err)
	}

	pinnedArtifacts := pinnedArtifactsFromManifest(manifest)
	wantTargets := map[string]int{
		TargetDarwinArm64:      1,
		TargetLinuxAmd64:       1,
		TargetWindowsAmd64:     1,
		TargetWindowsAmd64CUDA: 1,
	}
	observedTargets := make(map[string]int, len(pinnedArtifacts))
	for _, artifact := range pinnedArtifacts {
		if artifact.BackendID != ApprovedCUDABackend {
			continue
		}
		observedTargets[artifact.TargetID]++
	}
	if len(pinnedArtifacts) != 10 || len(observedTargets) != len(wantTargets) {
		t.Fatalf("projected exact fixture = %d artifacts; approved backend targets = %#v, want three baselines plus one CUDA entry", len(pinnedArtifacts), observedTargets)
	}
	for target, wantCount := range wantTargets {
		if observedTargets[target] != wantCount {
			t.Fatalf("projected target %q count = %d, want %d", target, observedTargets[target], wantCount)
		}
	}

	inputs := Inputs{
		References:         []Reference{{Identifier: ApprovedCUDABackend, Source: "exact Windows CUDA fixture"}},
		RegisteredBackends: []string{ApprovedCUDABackend},
		PinnedArtifacts:    pinnedArtifacts,
	}
	if err := Validate(inputs); err != nil {
		t.Fatalf("Validate(projected exact Windows CUDA fixture): %v", err)
	}
}

func repositoryConformanceInputs() (Inputs, error) {
	references, err := collectPackagedFactoryReferences()
	if err != nil {
		return Inputs{}, err
	}
	for _, definition := range (models.BuiltInCatalog{}).ModelDefinitions() {
		if strings.TrimSpace(definition.Backend) == "" {
			return Inputs{}, fmt.Errorf("built-in model %q has an empty backend", definition.Name)
		}
		references = append(references, Reference{
			Identifier: definition.Backend,
			Source:     fmt.Sprintf("BuiltInCatalog.ModelDefinitions[%s]", definition.Name),
		})
	}

	manifest, err := artifacts.DefaultManifest()
	if err != nil {
		return Inputs{}, fmt.Errorf("decode default backend manifest: %w", err)
	}
	pinnedArtifacts := pinnedArtifactsFromManifest(manifest)

	registeredBackends := make([]string, 0)
	for _, record := range backendregistry.Records() {
		registeredBackends = append(registeredBackends, record.Artifact.ID)
	}
	return Inputs{
		References:         references,
		RegisteredBackends: registeredBackends,
		PinnedArtifacts:    pinnedArtifacts,
	}, nil
}

func pinnedArtifactsFromManifest(manifest artifacts.Manifest) []PinnedArtifact {
	pinnedArtifacts := make([]PinnedArtifact, 0, manifest.ArtifactCount())
	for _, descriptor := range manifest.Artifacts() {
		pinnedArtifacts = append(pinnedArtifacts, PinnedArtifact{
			BackendID:       descriptor.Backend.ID,
			TargetID:        descriptor.Target.ID,
			OperatingSystem: descriptor.Target.OperatingSystem,
			Architecture:    descriptor.Target.Architecture,
			Accelerators:    append([]string(nil), descriptor.Target.Accelerators...),
			SizeBytes:       descriptor.Artifact.SizeBytes,
		})
	}
	return pinnedArtifacts
}

func collectPackagedFactoryReferences() ([]Reference, error) {
	published := packagedfactories.Published()
	references := make([]Reference, 0)
	err := fs.WalkDir(published, "generated/factories", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "factory.json" {
			return nil
		}

		fileReferences, err := readPackagedFactoryReferences(published, path)
		if err != nil {
			return err
		}
		references = append(references, fileReferences...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk generated packaged Factories: %w", err)
	}
	return references, nil
}

func readPackagedFactoryReferences(published fs.FS, path string) ([]Reference, error) {
	payload, err := fs.ReadFile(published, path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	factory, err := factorymapping.GeneratedFactoryFromOpenAPIJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("decode %s through Factory contract: %w", path, err)
	}

	references := packagedFactoryResourceReferences(path, factory.Resources)
	return append(references, packagedFactoryWorkerReferences(path, factory.Workers)...), nil
}

func packagedFactoryResourceReferences(path string, resources *[]factoryapi.Resource) []Reference {
	if resources == nil {
		return nil
	}
	references := make([]Reference, 0, len(*resources))
	for index, resource := range *resources {
		if resource.Type == nil || *resource.Type != factoryapi.ResourceTypeModel ||
			resource.Backend == nil || strings.TrimSpace(*resource.Backend) == "" {
			continue
		}
		references = append(references, Reference{
			Identifier: canonicalPackagedFactoryBackend(*resource.Backend),
			Source:     fmt.Sprintf("%s (resources[%d] %s)", path, index, resource.Name),
		})
	}
	return references
}

func packagedFactoryWorkerReferences(path string, workers *[]factoryapi.Worker) []Reference {
	if workers == nil {
		return nil
	}
	references := make([]Reference, 0, len(*workers))
	for index, worker := range *workers {
		if worker.Type == nil || *worker.Type != factoryapi.WorkerTypeInferenceWorker ||
			worker.Command == nil || strings.TrimSpace(*worker.Command) == "" {
			continue
		}
		references = append(references, Reference{
			Identifier: *worker.Command,
			Source:     fmt.Sprintf("%s (workers[%d] %s)", path, index, worker.Name),
		})
	}
	return references
}

func canonicalPackagedFactoryBackend(identifier string) string {
	trimmed := strings.TrimSpace(identifier)
	if strings.HasPrefix(strings.ToLower(trimmed), "localai-") {
		return strings.ToLower(trimmed)
	}
	return trimmed
}
