package climanifestcobra_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
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

func TestSessionResolvedHandlersMapDefaultsChangedValuesAndStableArguments(t *testing.T) {
	var (
		creates []sessioncli.CreateConfig
		lists   []sessioncli.ListConfig
		deletes []sessioncli.DeleteConfig
		diag    bytes.Buffer
	)
	services := commandregistry.SessionResolvedServices{
		CreateSession: func(cfg sessioncli.CreateConfig) error {
			creates = append(creates, cfg)
			return nil
		},
		ListSessions: func(cfg sessioncli.ListConfig) error {
			lists = append(lists, cfg)
			return nil
		},
		DeleteSession: func(cfg sessioncli.DeleteConfig) error {
			deletes = append(deletes, cfg)
			return nil
		},
		Diagnostics: func(*cobra.Command) io.Writer { return &diag },
	}

	if err := executeResolvedSession(t, services, "session", "create", "--dir", "fleet"); err != nil {
		t.Fatalf("default create Execute() error = %v", err)
	}
	if len(creates) != 1 {
		t.Fatalf("create calls = %d, want 1", len(creates))
	}
	assertDefaultResolvedCreate(t, creates[0], &diag)

	if err := executeResolvedSession(
		t,
		services,
		"--server", "https://factory.example", "--json", "--verbose", "--debug",
		"session", "create", "--dir", "fleet", "--port", "9444",
		"--init-new-factory", "--target-kind", "named", "--target-name", "alpha",
	); err != nil {
		t.Fatalf("changed create Execute() error = %v", err)
	}
	assertChangedResolvedCreate(t, creates[1])

	if err := executeResolvedSession(t, services, "session", "list"); err != nil {
		t.Fatalf("default list Execute() error = %v", err)
	}
	assertDefaultResolvedList(t, lists[0])
	if err := executeResolvedSession(
		t, services, "--server", "https://factory.example",
		"session", "list", "--scope", "all",
	); err != nil {
		t.Fatalf("changed list Execute() error = %v", err)
	}
	assertChangedResolvedList(t, lists[1])

	if err := executeResolvedSession(t, services, "--json", "session", "delete", "session-beta"); err != nil {
		t.Fatalf("delete Execute() error = %v", err)
	}
	assertResolvedDelete(t, deletes)
}

func TestSessionResolvedHandlersRejectInvalidInputsBeforeOperation(t *testing.T) {
	calls := 0
	services := commandregistry.SessionResolvedServices{
		CreateSession: func(sessioncli.CreateConfig) error {
			calls++
			return nil
		},
		DeleteSession: func(sessioncli.DeleteConfig) error {
			calls++
			return nil
		},
	}
	if err := executeResolvedSession(
		t, services, "session", "create", "--dir", "fleet", "--port", "not-an-int",
	); err == nil {
		t.Fatal("invalid typed port error = nil")
	}
	if err := executeResolvedSession(t, services, "session", "delete"); err == nil {
		t.Fatal("missing session ID error = nil")
	}
	if calls != 0 {
		t.Fatalf("operation calls = %d, want 0", calls)
	}
}

func TestSessionCompatibilityInputsRetainResolvedProvenance(t *testing.T) {
	defaultLocal, defaultInherited := observeSessionCreateInputs(
		t, "session", "create", "--dir", "fleet",
	)
	assertSessionResolvedState(
		t, defaultLocal, "you.session.create.flag.port",
		resolvedinput.SourceManifestDefault, false, true,
	)
	assertSessionResolvedState(
		t, defaultInherited, "you.flag.server",
		resolvedinput.SourceManifestDefault, false, true,
	)

	changedLocal, changedInherited := observeSessionCreateInputs(
		t,
		"--server", "https://factory.example",
		"session", "create", "--dir", "fleet", "--port", "9444",
	)
	assertSessionResolvedState(
		t, changedLocal, "you.session.create.flag.dir",
		resolvedinput.SourceCLIFlag, true, false,
	)
	assertSessionResolvedState(
		t, changedLocal, "you.session.create.flag.port",
		resolvedinput.SourceCLIFlag, true, false,
	)
	assertSessionResolvedState(
		t, changedInherited, "you.flag.server",
		resolvedinput.SourceCLIFlag, true, false,
	)
}

func observeSessionCreateInputs(
	t *testing.T,
	args ...string,
) (resolvedinput.Inputs, resolvedinput.Inputs) {
	t.Helper()
	manifest := mustSessionManifest(t)
	registry := commandregistry.NewRegistry()
	var local, inherited resolvedinput.Inputs
	for _, record := range manifest.Commands {
		if !record.Runnable {
			continue
		}
		commandID := record.ID
		err := registry.RegisterResolved(
			record.Handler.ID,
			func(_ *cobra.Command, inputs, globals resolvedinput.Inputs) error {
				if commandID == "you.session.create" {
					local, inherited = inputs, globals
				}
				return nil
			},
		)
		if err != nil {
			t.Fatalf("RegisterResolved(%q) error = %v", record.Handler.ID, err)
		}
	}
	root, err := climanifestcobra.NewSessionFamilyCommandFromManifest(manifest, registry)
	if err != nil {
		t.Fatalf("NewSessionFamilyCommandFromManifest() error = %v", err)
	}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("session create Execute() error = %v", err)
	}
	return local, inherited
}

func assertSessionResolvedState(
	t *testing.T,
	inputs resolvedinput.Inputs,
	inputID string,
	provenance resolvedinput.Source,
	changed bool,
	defaulted bool,
) {
	t.Helper()
	state, ok := inputs.State(inputID)
	if !ok {
		t.Fatalf("resolved input %q state is missing", inputID)
	}
	if state.Provenance != provenance || state.Changed != changed || state.Default != defaulted {
		t.Fatalf("resolved input %q state = %#v", inputID, state)
	}
}

func executeResolvedSession(
	t *testing.T,
	services commandregistry.SessionResolvedServices,
	args ...string,
) error {
	t.Helper()
	manifest := mustSessionManifest(t)
	registry, err := commandregistry.NewSessionResolvedRegistry(manifest, services)
	if err != nil {
		t.Fatalf("NewSessionResolvedRegistry() error = %v", err)
	}
	root, err := climanifestcobra.NewSessionFamilyCommandFromManifest(manifest, registry)
	if err != nil {
		t.Fatalf("NewSessionFamilyCommandFromManifest() error = %v", err)
	}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.Execute()
}

func assertDefaultResolvedCreate(
	t *testing.T,
	cfg sessioncli.CreateConfig,
	diagnostics io.Writer,
) {
	t.Helper()
	if cfg.Dir != "fleet" || cfg.Port != 7437 ||
		cfg.PortExplicit || cfg.Server != "http://localhost:7437" ||
		cfg.InitNewFactory || cfg.ValidateOnly ||
		cfg.JSON || cfg.Verbose || cfg.Debug {
		t.Fatalf("default create config = %#v", cfg)
	}
	if cfg.Diagnostics != diagnostics || cfg.Output == nil {
		t.Fatalf("default create writers = output:%T diagnostics:%T", cfg.Output, cfg.Diagnostics)
	}
}

func assertChangedResolvedCreate(t *testing.T, cfg sessioncli.CreateConfig) {
	t.Helper()
	if cfg.Server != "https://factory.example" ||
		cfg.Port != 9444 || !cfg.PortExplicit ||
		!cfg.JSON || !cfg.Verbose || !cfg.Debug ||
		!cfg.InitNewFactory ||
		cfg.TargetKind != "named" || cfg.TargetName != "alpha" {
		t.Fatalf("changed create config = %#v", cfg)
	}
}

func assertDefaultResolvedList(t *testing.T, cfg sessioncli.ListConfig) {
	t.Helper()
	if cfg.Scope != "live" || cfg.Port != 7437 || cfg.Server != "" {
		t.Fatalf("default list config = %#v", cfg)
	}
}

func assertChangedResolvedList(t *testing.T, cfg sessioncli.ListConfig) {
	t.Helper()
	if cfg.Scope != "all" || cfg.Server != "https://factory.example" {
		t.Fatalf("changed list config = %#v", cfg)
	}
}

func assertResolvedDelete(t *testing.T, configs []sessioncli.DeleteConfig) {
	t.Helper()
	if len(configs) != 1 || configs[0].SessionID != "session-beta" ||
		configs[0].Port != 7437 || !configs[0].JSON {
		t.Fatalf("delete configs = %#v", configs)
	}
}
