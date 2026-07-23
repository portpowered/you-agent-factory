package workerconfig

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts/namevalue"
)

func TestCloneDetachesLocalizedDescription(t *testing.T) {
	t.Parallel()

	original := Config{Description: &namevalue.Config{
		Type:    namevalue.TypeLocalizableAsset,
		Value:   "Base description",
		Locales: []string{"en-US"},
		Values:  map[string]string{"fr-FR": "Description"},
	}}
	cloned := Clone(original)
	cloned.Description.Locales[0] = "de-DE"
	cloned.Description.Values["fr-FR"] = "Changed"

	if got := original.Description.Locales[0]; got != "en-US" {
		t.Fatalf("original locale = %q, want en-US", got)
	}
	if got := original.Description.Values["fr-FR"]; got != "Description" {
		t.Fatalf("original localized value = %q, want Description", got)
	}
}
