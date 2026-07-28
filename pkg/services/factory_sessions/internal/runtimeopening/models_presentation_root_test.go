package runtimeopening

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

func TestModelsRoot_ReturnsProcessScopedModelService(t *testing.T) {
	t.Parallel()

	root := &recordingModelsService{}
	factory := &Factory{modelService: root}
	if got := factory.ModelsRoot(); got != root {
		t.Fatalf("ModelsRoot() = %v, want process-scoped Models service", got)
	}
	if got := (&Factory{}).ModelsRoot(); got != nil {
		t.Fatalf("ModelsRoot() on empty factory = %v, want nil", got)
	}
	if got := (*Factory)(nil).ModelsRoot(); got != nil {
		t.Fatalf("ModelsRoot() on nil factory = %v, want nil", got)
	}
}

var _ models.Service = (*recordingModelsService)(nil)
