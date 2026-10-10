package namedfactories_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	catalognamedfactories "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedfactories"
	catalognamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
)

type selectedResolutionPaths struct {
	factorydefinitions.NamedPathResolver
	resolve func(string, string) (string, error)
}

func (p selectedResolutionPaths) ResolveExistingDir(root, name string) (string, error) {
	return p.resolve(root, name)
}

// The catalog owns precedence and public error classification. The path
// resolver supplies controlled candidates; no real path owner is assembled.
func TestCatalogResolutionPreservesPrecedenceAndTypedFailures(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"project-over-global", "project-only", "global", "missing", "invalid", "failure"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			checkSelectedResolution(t, scenario)
		})
	}
}

func checkSelectedResolution(t *testing.T, scenario string) {
	t.Helper()
	cause := errors.New("selected resolver failure")
	var calls []string
	paths := selectedResolutionPaths{resolve: func(root, name string) (string, error) {
		if name != "shared" {
			t.Fatalf("resolver name = %q, want shared", name)
		}
		calls = append(calls, root)
		if scenario == "failure" {
			return "", cause
		}
		if scenario == "project-over-global" || scenario == "project-only" && root == "project" || scenario == "global" && root == "global" {
			return root + "/shared", nil
		}
		return "", catalognamedpaths.ErrNotFound
	}}
	name := "shared"
	if scenario == "invalid" {
		name = "../escape"
	}
	got, err := catalognamedfactories.ResolveAcrossRoots(paths, "project", "global", name)
	checkResolutionOutcome(t, scenario, got, err, cause)
	wantCalls := []string{"project", "global"}
	switch scenario {
	case "invalid":
		wantCalls = nil
	case "failure":
		wantCalls = []string{"project"}
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("resolver calls = %v, want %v", calls, wantCalls)
	}
}

func checkResolutionOutcome(t *testing.T, scenario string, got *factorydefinitions.NamedFactoryResolution, err, cause error) {
	t.Helper()
	failures := map[string]error{
		"invalid": factorydefinitions.ErrInvalidNamedFactoryName,
		"missing": factorydefinitions.ErrNamedFactoryNotFound,
		"failure": cause,
	}
	if wantErr, failed := failures[scenario]; failed {
		if got != nil || !errors.Is(err, wantErr) {
			t.Fatalf("resolution = %#v, error = %v, want nil/%v", got, err, wantErr)
		}
		if scenario == "invalid" && errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
			t.Fatal("invalid name also classified as missing")
		}
		return
	}
	want := factorydefinitions.NamedFactoryResolution{
		Name: "shared", FactoryDir: "project/shared",
		Source:      factorydefinitions.NamedFactoryResolutionSourceProjectLocal,
		ProjectRoot: "project", GlobalRoot: "global",
		PrecedenceDecision: factorydefinitions.NamedFactoryPrecedenceDecisionNone,
	}
	switch scenario {
	case "project-over-global":
		want.PrecedenceDecision = factorydefinitions.NamedFactoryPrecedenceDecisionProjectOverGlobal
	case "global":
		want.FactoryDir = "global/shared"
		want.Source = factorydefinitions.NamedFactoryResolutionSourceGlobal
	}
	if err != nil || got == nil || *got != want {
		t.Fatalf("resolution = %#v, error = %v, want %#v", got, err, want)
	}
}

func TestNewRequiresAllExternalEffects(t *testing.T) {
	t.Parallel()

	fileSystem := platformfilesystem.Local{}
	paths := selectedResolutionPaths{}

	if _, err := catalognamedfactories.New(nil, fileSystem); err == nil || !strings.Contains(err.Error(), "path resolver is required") {
		t.Fatalf("New(nil, filesystem) error = %v, want required path resolver", err)
	}
	if _, err := catalognamedfactories.New(paths, nil); err == nil || !strings.Contains(err.Error(), "catalog filesystem is required") {
		t.Fatalf("New(paths, nil) error = %v, want required catalog filesystem", err)
	}
}
