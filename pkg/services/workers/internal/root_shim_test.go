package internal

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstationswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/wire"
)

func TestNewRootConstructsPublishedWorkersService(t *testing.T) {
	t.Parallel()

	root, err := NewRoot(&recordingRuntimeAssembly{}, workstationswire.NewService(), nil)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewRoot() returned nil service")
	}
	var published workers.Service = root
	if published == nil {
		t.Fatal("constructed root is nil")
	}
}
