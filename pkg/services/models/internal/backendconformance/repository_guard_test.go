package backendconformance

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backendregistry"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

// TestNoDanglingBackendReference proves every shipped inference reference has
// exactly one offline installation path. The repository adapter below is the
// only code that discovers generated files or the checked-in manifest.
func TestNoDanglingBackendReference(t *testing.T) {
	t.Parallel()

	inputs, err := repositoryConformanceInputs()
	if err != nil {
		t.Fatalf("collect repository backend references: %v", err)
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
		t.Fatal("collector did not preserve the generated TTS Factory path for omnivoice-llamacpp")
	}
	if !catalogReference {
		t.Fatal("collector did not preserve the built-in TTS catalog source")
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
	pinnedArtifacts := make([]PinnedArtifact, 0, manifest.ArtifactCount())
	for _, descriptor := range manifest.Artifacts() {
		pinnedArtifacts = append(pinnedArtifacts, PinnedArtifact{
			BackendID: descriptor.Backend.ID,
			TargetID:  descriptor.Target.ID,
			SizeBytes: descriptor.Artifact.SizeBytes,
		})
	}

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

		payload, err := fs.ReadFile(published, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		factory, err := factorymapping.GeneratedFactoryFromOpenAPIJSON(payload)
		if err != nil {
			return fmt.Errorf("decode %s through Factory contract: %w", path, err)
		}
		if factory.Workers == nil {
			return nil
		}
		for index, worker := range *factory.Workers {
			if worker.Type == nil || *worker.Type != factoryapi.WorkerTypeInferenceWorker {
				continue
			}
			if worker.Command == nil || strings.TrimSpace(*worker.Command) == "" {
				continue
			}
			for _, identifier := range []string{*worker.Command} {
				references = append(references, Reference{
					Identifier: identifier,
					Source:     fmt.Sprintf("%s (workers[%d] %s)", path, index, worker.Name),
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk generated packaged Factories: %w", err)
	}
	return references, nil
}
