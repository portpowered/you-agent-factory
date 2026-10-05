package contracttests

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	goal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/goal"
)

type promptFiles map[string][]byte

func (f promptFiles) ReadFile(path string) ([]byte, error) {
	if body, ok := f[path]; ok {
		return body, nil
	}
	return nil, fs.ErrNotExist
}

func TestPublishedGoalPromptContract(t *testing.T) {
	files := promptFiles{}
	for _, source := range goal.PackagedGoalRolePromptSources {
		body, err := fs.ReadFile(packagedfactories.Source(), "factories/goal/"+source.PromptFile)
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Join("factory", "workstations", source.WorkstationName, source.PromptFile)] = body
	}
	if err := goal.CheckPackagedGoalAssembledPromptDrift(); err != nil {
		t.Fatal(err)
	}
	if err := goal.CheckPackagedGoalMaterializedPromptDrift(files, "factory"); err != nil {
		t.Fatal(err)
	}
	for _, source := range goal.PackagedGoalRolePromptSources {
		path := filepath.Join("factory", "workstations", source.WorkstationName, source.PromptFile)
		original := files[path]
		files[path] = []byte("drifted prompt")
		var drift goal.PackagedGoalPromptDriftError
		if err := goal.CheckPackagedGoalMaterializedPromptDrift(files, "factory"); !errors.As(err, &drift) || drift.Role != source.Role {
			t.Fatalf("drift error = %v", err)
		}
		files[path] = original
	}
}
