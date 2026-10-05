package packagedinstallation

import (
	"bytes"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestSelectPayloadOwnsBytesAndSelectsAuthoredRoot(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		format factorydefinitions.PackagedFactoryFormat
		root   string
	}{
		{factorydefinitions.PackagedFactoryFormatJSON, "factory.json"},
		{factorydefinitions.PackagedFactoryFormatYAML, "factory.yaml"},
		{factorydefinitions.PackagedFactoryFormatYML, "factory.yml"},
	} {
		t.Run(string(test.format), func(t *testing.T) {
			t.Parallel()
			definition := installationDefinitionFixture()
			original := bytes.Clone(definition.JSON)
			payload, root, format, err := selectPayload(definition, test.format)
			if err != nil || root != test.root || format != test.format || !bytes.Equal(payload, original) {
				t.Fatalf("selectPayload = %q, %s, %s, %v", payload, root, format, err)
			}
			payload[0] ^= 1
			if !bytes.Equal(definition.JSON, original) {
				t.Fatal("selected payload aliases caller-owned input")
			}
		})
	}
}
