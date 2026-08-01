package goal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
)

var packagedGoalTestFileSystem = platformfilesystem.Local{}

type packagedGoalPromptConfig struct {
	Workers []struct {
		Name string `json:"name"`
		Body string `json:"body"`
	} `json:"workers"`
	Workstations []struct {
		Name string `json:"name"`
		Body string `json:"body"`
	} `json:"workstations"`
}

// PackagedGoalPromptDriftError reports canonical-vs-derived prompt mismatch for one role.
type PackagedGoalPromptDriftError struct {
	Role string
}

func (e PackagedGoalPromptDriftError) Error() string {
	return fmt.Sprintf("packaged goal prompt drift for role %q: derived prompt does not match canonical packaged source", e.Role)
}

// CheckPackagedGoalMaterializedPromptDrift returns an error when any materialized
// on-disk prompt for a packaged goal role does not match the canonical authored source.
func CheckPackagedGoalMaterializedPromptDrift(
	fileSystem factoryeffects.PackagedGoalPromptFileSystem,
	factoryDir string,
) error {
	if fileSystem == nil {
		return fmt.Errorf("packaged Goal prompt filesystem is required")
	}
	canonical, err := decodePackagedGoalPromptConfig()
	if err != nil {
		return fmt.Errorf("load assembled built-in goal factory: %w", err)
	}
	for _, source := range PackagedGoalRolePromptSources {
		authoredPrompt, err := assembledPackagedGoalRolePrompt(canonical, source)
		if err != nil {
			return fmt.Errorf("load canonical prompt for role %q: %w", source.Role, err)
		}

		derivedPrompt, err := loadPackagedGoalRolePrompt(fileSystem, factoryDir, source)
		if err != nil {
			return fmt.Errorf("load derived prompt for role %q: %w", source.Role, err)
		}
		if derivedPrompt != authoredPrompt {
			return PackagedGoalPromptDriftError{Role: source.Role}
		}
	}
	return nil
}

// CheckPackagedGoalAssembledPromptDrift verifies that every declared goal role
// resolves to a non-empty prompt in the shared assembler's canonical payload.
func CheckPackagedGoalAssembledPromptDrift() error {
	canonical, err := decodePackagedGoalPromptConfig()
	if err != nil {
		return fmt.Errorf("load assembled built-in goal factory: %w", err)
	}
	for _, source := range PackagedGoalRolePromptSources {
		prompt, err := assembledPackagedGoalRolePrompt(canonical, source)
		if err != nil {
			return fmt.Errorf("load assembled prompt for role %q: %w", source.Role, err)
		}
		if strings.TrimSpace(prompt) == "" {
			return fmt.Errorf("assembled prompt for role %q is empty", source.Role)
		}
	}
	return nil
}

func decodePackagedGoalPromptConfig() (*packagedGoalPromptConfig, error) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		return nil, fmt.Errorf("load generated packaged Factory catalog: %w", err)
	}
	definition, ok := catalog.Lookup(PackagedFactoryName)
	if !ok {
		return nil, fmt.Errorf("generated packaged Factory catalog is missing %s", PackagedFactoryName)
	}
	var config packagedGoalPromptConfig
	if err := json.Unmarshal(definition.JSON, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func assembledPackagedGoalRolePrompt(cfg *packagedGoalPromptConfig, source PackagedGoalRolePromptSource) (string, error) {
	switch source.SourceKind {
	case PackagedGoalRolePromptSourceKindWorkerBody:
		for _, worker := range cfg.Workers {
			if worker.Name == source.WorkerName {
				return worker.Body, nil
			}
		}
		return "", fmt.Errorf("missing worker %q", source.WorkerName)
	default:
		for _, workstation := range cfg.Workstations {
			if workstation.Name == source.WorkstationName {
				return workstation.Body, nil
			}
		}
		return "", fmt.Errorf("missing workstation %q", source.WorkstationName)
	}
}

func TestPackagedGoalPromptDriftFailsClosedWithoutFileSystem(t *testing.T) {
	if err := CheckPackagedGoalMaterializedPromptDrift(nil, t.TempDir()); err == nil {
		t.Fatal("expected missing packaged Goal prompt filesystem error")
	}
}

func TestPackagedGoalPromptDrift_FreshMaterializationMatchesCanonicalSource(t *testing.T) {
	factoryDir := materializePackagedGoalFactory(t, t.TempDir())

	if err := CheckPackagedGoalMaterializedPromptDrift(packagedGoalTestFileSystem, factoryDir); err != nil {
		t.Fatalf("materialized prompt drift check: %v", err)
	}
	if err := CheckPackagedGoalAssembledPromptDrift(); err != nil {
		t.Fatalf("assembled prompt drift check: %v", err)
	}
}

func TestPackagedGoalPromptDrift_FailsWhenMaterializedPromptDrifts(t *testing.T) {
	for _, source := range PackagedGoalRolePromptSources {
		source := source
		t.Run(source.Role, func(t *testing.T) {
			factoryDir := materializePackagedGoalFactory(t, t.TempDir())
			promptPath := packagedGoalMaterializedPromptPath(factoryDir, source)
			if err := os.WriteFile(promptPath, []byte("drifted packaged prompt copy\n"), 0o644); err != nil {
				t.Fatalf("write drifted prompt %s: %v", promptPath, err)
			}

			err := CheckPackagedGoalMaterializedPromptDrift(packagedGoalTestFileSystem, factoryDir)
			if err == nil {
				t.Fatalf("expected prompt drift check to fail for role %q", source.Role)
			}

			var driftErr PackagedGoalPromptDriftError
			if !errors.As(err, &driftErr) {
				t.Fatalf("prompt drift error = %T(%v), want PackagedGoalPromptDriftError", err, err)
			}
			if driftErr.Role != source.Role {
				t.Fatalf("drift role = %q, want %q", driftErr.Role, source.Role)
			}
		})
	}
}
