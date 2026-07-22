package goal

import (
	"errors"
	"os"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

var packagedGoalTestFileSystem = platformfilesystem.Local{}

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
