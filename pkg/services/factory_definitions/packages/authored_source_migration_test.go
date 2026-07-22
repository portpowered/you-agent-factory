package packages_test

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"testing"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	deepresearch "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/deepresearch"
	fusion "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/fusion"
	goal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/goal"
	quorum "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/quorum"
	review "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/review"
	subagent "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/subagent"
	tts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/tts"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/packageassets"
)

func TestAuthoredFactoriesPreserveCompatibilityPayloads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		packageName   string
		legacyPayload []byte
	}{
		{name: "deep-research", packageName: "@you/deep-research", legacyPayload: deepresearch.BuiltInFactoryJSON},
		{name: "fusion", packageName: "@you/fusion", legacyPayload: fusion.BuiltInFactoryJSON},
		{name: "goal", packageName: "@you/goal", legacyPayload: goal.BuiltInGoalFactoryJSON},
		{name: "quorum", packageName: "@you/quorum", legacyPayload: quorum.BuiltInFactoryJSON},
		{name: "review", packageName: "@you/review", legacyPayload: review.BuiltInReviewFactoryJSON},
		{name: "subagent", packageName: "@you/subagent", legacyPayload: subagent.BuiltInSubagentFactoryJSON},
		{name: "tts", packageName: "@you/tts", legacyPayload: tts.BuiltInFactoryJSON},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			factoryRoot := "factories/" + test.name
			authoredJSON, err := fs.ReadFile(packagedfactories.Source(), factoryRoot+"/factory.json")
			if err != nil {
				t.Fatalf("read authored Factory: %v", err)
			}
			assets, err := fs.Sub(packagedfactories.Source(), factoryRoot)
			if err != nil {
				t.Fatalf("open authored Factory assets: %v", err)
			}
			assembled, err := packageassets.Assemble(packageassets.Definition{
				Package:     test.packageName,
				FactoryJSON: authoredJSON,
				Assets:      assets,
			})
			if err != nil {
				t.Fatalf("assemble authored Factory and owned assets: %v", err)
			}

			assertEquivalentJSON(t, assembled, test.legacyPayload)
		})
	}
}

func assertEquivalentJSON(t *testing.T, wantJSON, gotJSON []byte) {
	t.Helper()

	var want any
	if err := json.Unmarshal(wantJSON, &want); err != nil {
		t.Fatalf("decode authored payload: %v", err)
	}
	var got any
	if err := json.Unmarshal(gotJSON, &got); err != nil {
		t.Fatalf("decode compatibility payload: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("compatibility payload differs from the authored Factory and its owned assets")
	}
}
