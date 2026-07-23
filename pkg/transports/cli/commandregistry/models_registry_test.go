package commandregistry_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/spf13/cobra"
)

type modelsHandlerStub struct{}

func (modelsHandlerStub) List(*cobra.Command, []string) error    { return nil }
func (modelsHandlerStub) Inspect(*cobra.Command, []string) error { return nil }
func (modelsHandlerStub) Invoke(*cobra.Command, []string) error  { return nil }
func (modelsHandlerStub) Pull(*cobra.Command, []string) error    { return nil }

func TestNewModelsRegistryRequiresHandler(t *testing.T) {
	if _, err := commandregistry.NewModelsRegistry(nil); err == nil {
		t.Fatal("NewModelsRegistry(nil) error = nil, want failure")
	}
}

func TestModelsRegistryContainsOnlyModelsHandlers(t *testing.T) {
	models, err := commandregistry.NewModelsRegistry(modelsHandlerStub{})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"you.models.list", "you.models.inspect", "you.models.invoke", "you.models.pull"} {
		if _, err := models.Lookup(id); err != nil {
			t.Fatalf("Lookup(%s): %v", id, err)
		}
	}
	if _, err := models.Lookup("you.docs"); err == nil {
		t.Fatal("Models registry unexpectedly contains Docs handler")
	}
}
