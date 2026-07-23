package wire

import (
	"bytes"
	"io/fs"
	"testing"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
)

func TestProvidePackagedFactoryDefinitions_LoadsDetachedGeneratedCatalog(t *testing.T) {
	definitions, err := providePackagedFactoryDefinitions()
	if err != nil {
		t.Fatalf("providePackagedFactoryDefinitions() error = %v", err)
	}

	wantNames := []string{
		"@you/deep-research",
		"@you/fusion",
		"@you/goal",
		"@you/quorum",
		"@you/review",
		"@you/subagent",
		"@you/tts",
	}
	if len(definitions) != len(wantNames) {
		t.Fatalf("definition count = %d, want %d", len(definitions), len(wantNames))
	}
	for index, wantName := range wantNames {
		if definitions[index].Name != wantName {
			t.Fatalf("definitions[%d].Name = %q, want %q", index, definitions[index].Name, wantName)
		}
	}

	publishedGoal, err := fs.ReadFile(
		packagedfactories.Published(),
		"generated/factories/goal/factory.json",
	)
	if err != nil {
		t.Fatalf("read published Goal definition: %v", err)
	}
	if !bytes.Equal(definitions[2].JSON, publishedGoal) {
		t.Fatal("injected Goal definition differs from the generated publication artifact")
	}

	definitions[2].JSON[0] ^= 0xff
	reloaded, err := providePackagedFactoryDefinitions()
	if err != nil {
		t.Fatalf("second providePackagedFactoryDefinitions() error = %v", err)
	}
	if !bytes.Equal(reloaded[2].JSON, publishedGoal) {
		t.Fatal("mutating one injected catalog changed a later injected catalog")
	}
}
