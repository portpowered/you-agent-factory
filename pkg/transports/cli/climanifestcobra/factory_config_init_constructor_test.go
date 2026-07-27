package climanifestcobra_test

import (
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

type factoryConfigInitHandlerStub struct{}

func TestSessionFamilyCommandExecutesManifestBoundLeaf(t *testing.T) {
	manifest := mustSessionManifest(t)
	calls := 0
	registry := sessionHandlerIDRegistry(t, manifest, func(cmd *cobra.Command, args []string) error {
		calls++
		if cmd.Name() != "show" {
			t.Fatalf("executed command = %q, want show", cmd.Name())
		}
		if len(args) != 1 || args[0] != "session-alpha" {
			t.Fatalf("handler args = %#v, want [session-alpha]", args)
		}
		return nil
	})

	root, err := climanifestcobra.NewSessionFamilyCommandFromManifest(manifest, registry)
	if err != nil {
		t.Fatalf("NewSessionFamilyCommandFromManifest() error = %v", err)
	}
	for _, leaf := range []string{
		"create", "delete", "list", "show", "dispatches", "pause", "resume",
	} {
		command, _, findErr := root.Find([]string{"session", leaf})
		if findErr != nil {
			t.Fatalf("Find(session %s) error = %v", leaf, findErr)
		}
		if command == nil || command.RunE == nil {
			t.Fatalf("session %s is not executable", leaf)
		}
	}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "show", "session-alpha"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("operation calls = %d, want 1", calls)
	}
}

func TestSessionFamilyCommandRejectsInvalidBindingsBeforeExecution(t *testing.T) {
	tests := []struct {
		name        string
		build       func(*testing.T, climanifest.Manifest, commandregistry.RunE) (*commandregistry.Registry, climanifest.Manifest)
		want        string
		nilRegistry bool
	}{
		{name: "nil registry", nilRegistry: true, want: "registry is required"},
		{
			name: "missing handler",
			build: func(t *testing.T, manifest climanifest.Manifest, operation commandregistry.RunE) (*commandregistry.Registry, climanifest.Manifest) {
				return sessionHandlerIDRegistryExcept(t, manifest, "you.session.create.handler", operation), manifest
			},
			want: "you.session.create.handler",
		},
		{
			name: "cross-family handler",
			build: func(t *testing.T, manifest climanifest.Manifest, operation commandregistry.RunE) (*commandregistry.Registry, climanifest.Manifest) {
				registry := sessionHandlerIDRegistry(t, manifest, operation)
				if err := registry.Register("you.work.list.handler", noOpSessionHandler); err != nil {
					t.Fatalf("Register(cross-family) error = %v", err)
				}
				return registry, manifest
			},
			want: "you.work.list.handler",
		},
		{
			name: "duplicate manifest handler",
			build: func(t *testing.T, manifest climanifest.Manifest, operation commandregistry.RunE) (*commandregistry.Registry, climanifest.Manifest) {
				registry := sessionHandlerIDRegistry(t, manifest, operation)
				create := manifest.Commands["you.session.create"]
				deleteCommand := manifest.Commands["you.session.delete"]
				deleteCommand.Handler.ID = create.Handler.ID
				manifest.Commands[deleteCommand.ID] = deleteCommand
				return registry, manifest
			},
			want: "duplicated",
		},
		{
			name: "unbound manifest handler",
			build: func(t *testing.T, manifest climanifest.Manifest, operation commandregistry.RunE) (*commandregistry.Registry, climanifest.Manifest) {
				registry := sessionHandlerIDRegistry(t, manifest, operation)
				show := manifest.Commands["you.session.show"]
				show.Handler.ID = "you.session.show.replacement-handler"
				manifest.Commands[show.ID] = show
				return registry, manifest
			},
			want: "replacement-handler",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := mustSessionManifest(t)
			calls := 0
			var registry *commandregistry.Registry
			if !test.nilRegistry {
				operation := func(*cobra.Command, []string) error {
					calls++
					return nil
				}
				registry, manifest = test.build(t, manifest, operation)
			}
			_, err := climanifestcobra.NewSessionFamilyCommandFromManifest(manifest, registry)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewSessionFamilyCommandFromManifest() error = %v, want %q", err, test.want)
			}
			if calls != 0 {
				t.Fatalf("operation calls = %d, want 0", calls)
			}
		})
	}
}

func mustSessionManifest(t *testing.T) climanifest.Manifest {
	t.Helper()
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		t.Fatalf("SessionFamilyManifest() error = %v", err)
	}
	return manifest
}

func mustSessionHandlerIDs(t *testing.T, manifest climanifest.Manifest) []string {
	t.Helper()
	handlerIDs, err := commandregistry.RunnableSessionHandlerIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableSessionHandlerIDs() error = %v", err)
	}
	return handlerIDs
}

func sessionHandlerIDRegistry(
	t *testing.T,
	manifest climanifest.Manifest,
	handler commandregistry.RunE,
) *commandregistry.Registry {
	t.Helper()
	registry := commandregistry.NewRegistry()
	for _, handlerID := range mustSessionHandlerIDs(t, manifest) {
		if handler == nil {
			handler = noOpSessionHandler
		}
		if err := registry.Register(handlerID, handler); err != nil {
			t.Fatalf("Register(%q) error = %v", handlerID, err)
		}
	}
	return registry
}

func sessionHandlerIDRegistryExcept(
	t *testing.T,
	manifest climanifest.Manifest,
	excluded string,
	handler commandregistry.RunE,
) *commandregistry.Registry {
	t.Helper()
	registry := commandregistry.NewRegistry()
	for _, handlerID := range mustSessionHandlerIDs(t, manifest) {
		if handlerID == excluded {
			continue
		}
		if err := registry.Register(handlerID, handler); err != nil {
			t.Fatalf("Register(%q) error = %v", handlerID, err)
		}
	}
	return registry
}

func noOpSessionHandler(*cobra.Command, []string) error {
	return nil
}

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

func TestFactoryConfigInitFamilyOmitsRetiredInitializationPaths(t *testing.T) {
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(factoryConfigInitHandlerStub{})
	if err != nil {
		t.Fatalf("NewFactoryConfigInitFamilyComponents() error = %v", err)
	}
	if len(components.Config.Commands()) != 0 {
		t.Fatalf("you config subcommands = %v, want retired config init absent", components.Config.Commands())
	}
	for _, name := range []string{"dir", "type", "executor"} {
		if flag := components.Init.Flags().Lookup(name); flag != nil {
			t.Fatalf("you init retained retired --%s flag", name)
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
