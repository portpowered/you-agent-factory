package inference_test

import (
	"errors"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

func TestInertArtifactFileSystemRejectsUninjectedEffects(t *testing.T) {
	t.Parallel()

	fileSystem := inference.InertArtifactFileSystem{}
	for _, operation := range []struct {
		name string
		call func() error
	}{
		{name: "open", call: func() error { _, err := fileSystem.Open("fixture"); return err }},
		{name: "create", call: func() error { _, err := fileSystem.Create("fixture"); return err }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			err := operation.call()
			if err == nil || !strings.Contains(err.Error(), "explicit export filesystem") {
				t.Fatalf("%s error = %v, want explicit export filesystem diagnostic", operation.name, err)
			}
			if errors.Is(err, models.ErrUnavailable) {
				t.Fatalf("%s error = %v, did not expect unrelated unavailable classification", operation.name, err)
			}
		})
	}
}
