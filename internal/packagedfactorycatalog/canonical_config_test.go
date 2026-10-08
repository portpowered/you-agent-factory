package packagedfactorycatalog_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

func TestCanonicalPublicationMatchesNormalizedDefinitionsAndRejectsEdits(t *testing.T) {
	t.Parallel()
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	mapper := factorymapping.NewFactoryConfigMapper()
	for _, definition := range catalog.All() {
		t.Run(definition.Name, func(t *testing.T) {
			config, err := mapper.ExpandStrict(definition.JSON)
			if err != nil {
				t.Fatal(err)
			}
			authored, err := authoredmapping.AuthoredFactoryConfigForExpandedLayout(config)
			if err != nil {
				t.Fatal(err)
			}
			input, err := json.Marshal(authored)
			if err != nil {
				t.Fatal(err)
			}
			want, err := mapper.Flatten(authored)
			if err != nil {
				t.Fatal(err)
			}
			got, err := catalog.ReadCanonicalFactoryConfig(input)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("canonical output differs: %v", err)
			}
			got[0] = 'x'
			again, err := catalog.ReadCanonicalFactoryConfig(input)
			if err != nil || !bytes.Equal(again, want) {
				t.Fatal("canonical output was shared")
			}
			authored.Name += "-customer-edit"
			edited, err := json.Marshal(authored)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := catalog.ReadCanonicalFactoryConfig(edited); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("edited definition hit publication: %v", err)
			}
		})
	}
}

func TestCanonicalPublicationMissesForUnusableOptionalAssets(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"missing", "version", "source-hash", "native-hash", "null"} {
		t.Run(name, func(t *testing.T) {
			fixture := publishedFixture(t)
			entry := readFixtureManifest(t, fixture).Factories[0]
			payload := fixture[entry.JSON.Locator].Data
			path := factorydefinitions.SerializedFactoryConfigInput(payload).Path()
			var conversion factorydefinitions.SerializedFactoryConfig
			if err := json.Unmarshal(fixture[path].Data, &conversion); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "missing":
				delete(fixture, path)
			case "version":
				conversion.Version = "old"
			case "source-hash":
				conversion.PayloadSHA256 = "stale"
			case "native-hash":
				conversion.CanonicalInputSHA256 = "stale"
			case "null":
				conversion.Canonical = json.RawMessage("null")
			}
			if name != "missing" {
				data, err := json.Marshal(conversion)
				if err != nil {
					t.Fatal(err)
				}
				fixture[path].Data = data
			}
			catalog, err := packagedfactorycatalog.LoadDefinitionCatalog(fixture)
			if err != nil {
				t.Fatal(err)
			}
			config, err := factorymapping.NewFactoryConfigMapper().ExpandStrict(payload)
			if err != nil {
				t.Fatal(err)
			}
			authored, err := authoredmapping.AuthoredFactoryConfigForExpandedLayout(config)
			if err != nil {
				t.Fatal(err)
			}
			input, err := json.Marshal(authored)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := catalog.ReadCanonicalFactoryConfig(input); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("unusable asset was indexed: %v", err)
			}
		})
	}
}
