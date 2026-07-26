package climanifestcobra_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

type factoryConfigInitHandlerStub struct{}

func (factoryConfigInitHandlerStub) FactoryQuery(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryList(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryCreate(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryUpdate(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryDelete(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryReplaceCurrent(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryConfigValidate(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryConfigFlatten(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryConfigExpand(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) ConfigInit(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) Init(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}

func TestNewFactoryConfigInitFamilyComponentsProjectsContractedTree(t *testing.T) {
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(factoryConfigInitHandlerStub{})
	if err != nil {
		t.Fatalf("NewFactoryConfigInitFamilyComponents() error = %v", err)
	}
	for _, test := range []struct {
		root *cobra.Command
		path []string
	}{
		{root: components.Factory, path: []string{"query"}},
		{root: components.Factory, path: []string{"list"}},
		{root: components.Factory, path: []string{"create"}},
		{root: components.Factory, path: []string{"config", "validate"}},
		{root: components.Config, path: []string{"init"}},
		{root: components.Init, path: nil},
	} {
		command, _, findErr := test.root.Find(test.path)
		if findErr != nil {
			t.Fatalf("%s Find(%v) error = %v", test.root.Name(), test.path, findErr)
		}
		if command == nil {
			t.Fatalf("%s Find(%v) returned nil", test.root.Name(), test.path)
		}
	}
}

func TestFactoryConfigInitFamilyUsesManifestDefaultsAndRequiredness(t *testing.T) {
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(factoryConfigInitHandlerStub{})
	if err != nil {
		t.Fatalf("NewFactoryConfigInitFamilyComponents() error = %v", err)
	}
	create, _, err := components.Factory.Find([]string{"create"})
	if err != nil {
		t.Fatalf("find factory create: %v", err)
	}
	if got := create.Flags().Lookup("dir").DefValue; got != "factory" {
		t.Fatalf("factory create --dir default = %q, want factory", got)
	}
	components.Factory.SetArgs([]string{"create", "staging"})
	components.Factory.SetOut(&strings.Builder{})
	components.Factory.SetErr(&strings.Builder{})
	if err := components.Factory.Execute(); err == nil || !strings.Contains(err.Error(), `required flag(s) "--from" not set`) {
		t.Fatalf("factory create missing --from error = %v", err)
	}
}

func TestFactoryConfigInitFamilyProjectsStaticCompletionMetadata(t *testing.T) {
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(factoryConfigInitHandlerStub{})
	if err != nil {
		t.Fatalf("NewFactoryConfigInitFamilyComponents() error = %v", err)
	}
	for _, name := range []string{"type", "executor"} {
		flag := components.Init.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("you init --%s missing", name)
		}
		if got := flag.Annotations["infinite-you/completion"]; len(got) != 1 || got[0] != "static" {
			t.Fatalf("you init --%s completion = %#v, want static", name, got)
		}
		complete, exists := components.Init.GetFlagCompletionFunc(name)
		if !exists {
			t.Fatalf("you init --%s completion callback missing", name)
		}
		values, directive := complete(components.Init, nil, "")
		if directive != cobra.ShellCompDirectiveNoFileComp || len(values) != 2 {
			t.Fatalf("you init --%s completion = %#v, %v", name, values, directive)
		}
	}
}

func TestFactoryConfigInitFamilyProjectsProviderModelSetupInputs(t *testing.T) {
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(factoryConfigInitHandlerStub{})
	if err != nil {
		t.Fatalf("NewFactoryConfigInitFamilyComponents() error = %v", err)
	}
	for _, name := range []string{"provider", "model"} {
		if flag := components.Init.Flags().Lookup(name); flag == nil {
			t.Fatalf("you init --%s missing", name)
		}
	}
	if got := components.Init.Short; got != "Configure provider and model defaults" {
		t.Fatalf("you init short help = %q", got)
	}
}

func TestNewFactoryConfigInitFamilyComponentsRejectsNilHandler(t *testing.T) {
	if _, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(nil); err == nil {
		t.Fatal("NewFactoryConfigInitFamilyComponents(nil) error = nil")
	}
}
