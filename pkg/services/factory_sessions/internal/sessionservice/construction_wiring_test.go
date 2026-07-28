package service

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestInstallFactoryDefinitionsRejectsMissingDefinitions(t *testing.T) {
	t.Parallel()

	err := InstallFactoryDefinitions(&SessionRuntime{}, nil)
	if err == nil || err.Error() != "factory definitions service is required" {
		t.Fatalf("InstallFactoryDefinitions() error = %v, want %q", err, "factory definitions service is required")
	}
}

func TestInstallFactoryDefinitionsRejectsMissingRuntime(t *testing.T) {
	t.Parallel()

	err := InstallFactoryDefinitions(nil, factorydefinitions.Service(nil))
	if err == nil || err.Error() != "session runtime is required" {
		t.Fatalf("InstallFactoryDefinitions() error = %v, want %q", err, "session runtime is required")
	}
}
