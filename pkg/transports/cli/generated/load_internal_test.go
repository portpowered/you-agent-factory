package generated

import (
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

func TestGeneratedFamilyAccessorsReturnFreshValues(t *testing.T) {
	t.Parallel()
	accessors := []struct {
		name string
		load func() (climanifest.Manifest, error)
	}{
		{name: "representative", load: RepresentativeFamilyManifest},
		{name: "session", load: SessionFamilyManifest},
		{name: "work", load: WorkFamilyManifest},
		{name: "factory config init", load: FactoryConfigInitFamilyManifest},
		{name: "models docs", load: ModelsDocsFamilyManifest},
		{name: "run submit", load: RunSubmitFamilyManifest},
		{name: "mcp", load: MCPFamilyManifest},
	}
	for _, accessor := range accessors {
		accessor := accessor
		t.Run(accessor.name, func(t *testing.T) {
			t.Parallel()
			first, err := accessor.load()
			if err != nil {
				t.Fatal(err)
			}
			second, err := accessor.load()
			if err != nil {
				t.Fatal(err)
			}
			for id := range first.Commands {
				delete(first.Commands, id)
				if _, ok := second.Commands[id]; !ok {
					t.Fatalf("second manifest lost command %q after first was mutated", id)
				}
				break
			}
		})
	}
}

func TestGeneratedManifestNestedCollectionsAndPointersAreIndependent(t *testing.T) {
	first, err := RepresentativeFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}
	second, err := RepresentativeFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}

	for id, command := range first.Commands {
		original := second.Commands[id]
		if len(command.Aliases) > 0 {
			command.Aliases[0] = "mutated"
			if original.Aliases[0] == "mutated" {
				t.Fatalf("command %q aliases share storage", id)
			}
		}
		if len(command.Flags) > 0 {
			for flagID := range command.Flags {
				delete(command.Flags, flagID)
				if _, ok := original.Flags[flagID]; !ok {
					t.Fatalf("command %q flags share storage", id)
				}
				break
			}
		}
		if command.Handler != nil {
			command.Handler.ID = "mutated"
			if original.Handler != nil && original.Handler.ID == "mutated" {
				t.Fatalf("command %q handler pointer is shared", id)
			}
		}
		first.Commands[id] = command
	}
}

func TestRuntimeGeneratedManifestAccessHasNoSourceLoadingPolicy(t *testing.T) {
	t.Parallel()
	payload, err := os.ReadFile("load.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"embed", "encoding/json", "json.Unmarshal", "SourceStore", "ReadFile", "parseFamilyManifest"} {
		if strings.Contains(string(payload), forbidden) {
			t.Errorf("load.go contains runtime source-loading seam %q", forbidden)
		}
	}
}
