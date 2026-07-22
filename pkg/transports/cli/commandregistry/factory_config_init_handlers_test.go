package commandregistry_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	configcli "github.com/portpowered/infinite-you/pkg/transports/cli/config"
	configinitcmd "github.com/portpowered/infinite-you/pkg/transports/cli/configinit"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

func TestNewFactoryConfigInitRegistryRegistersContractedRunnableIDs(t *testing.T) {
	registry, err := commandregistry.NewFactoryConfigInitRegistry(factoryConfigInitNoopHandlers())
	if err != nil {
		t.Fatalf("NewFactoryConfigInitRegistry() error = %v", err)
	}
	for _, commandID := range []string{
		"you.factory.query",
		"you.factory.list",
		"you.factory.create",
		"you.factory.update",
		"you.factory.delete",
		"you.factory.replace-current",
		"you.factory.config.validate",
		"you.factory.config.flatten",
		"you.factory.config.expand",
		"you.config.init",
		"you.init",
	} {
		if _, lookupErr := registry.Lookup(commandID); lookupErr != nil {
			t.Fatalf("Lookup(%q) error = %v", commandID, lookupErr)
		}
	}
}

func TestFactoryQueryRunEUsesHandwrittenServicePath(t *testing.T) {
	var called bool
	registry, err := commandregistry.NewFactoryConfigInitRegistry(commandregistry.FactoryConfigInitHandlers{
		FactoryQueryRunE: commandregistry.FactoryQueryRunE(commandregistry.FactoryQueryBinding{
			Query: func(factorycli.QueryConfig) error {
				called = true
				return nil
			},
		}),
		FactoryListRunE:           noopRunE,
		FactoryCreateRunE:         noopRunE,
		FactoryUpdateRunE:         noopRunE,
		FactoryDeleteRunE:         noopRunE,
		FactoryReplaceCurrentRunE: noopRunE,
		FactoryConfigValidateRunE: noopRunE,
		FactoryConfigFlattenRunE:  noopRunE,
		FactoryConfigExpandRunE:   noopRunE,
		ConfigInitRunE:            noopRunE,
		InitRunE:                  noopRunE,
	})
	if err != nil {
		t.Fatalf("NewFactoryConfigInitRegistry() error = %v", err)
	}

	cmd := &cobra.Command{Use: "query"}
	cmd.SetArgs([]string{})
	if err := registry.AttachRunE(cmd, "you.factory.query"); err != nil {
		t.Fatalf("AttachRunE() error = %v", err)
	}
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("expected factory query handler to invoke handwritten service path")
	}
}

func TestConfigInitRunEUsesHandwrittenServicePath(t *testing.T) {
	var called bool
	runE := commandregistry.ConfigInitRunE(commandregistry.ConfigInitBinding{
		JSON:    func() bool { return true },
		HomeDir: func() (string, error) { return t.TempDir(), nil },
		Init: func(configinitcmd.InitConfig) error {
			called = true
			return nil
		},
	})
	cmd := &cobra.Command{Use: "init"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if !called {
		t.Fatal("expected config init handler to invoke handwritten service path")
	}
}

func TestInitRunEUsesHandwrittenServicePath(t *testing.T) {
	var called bool
	runE := commandregistry.InitRunE(commandregistry.InitBinding{
		Init: func(factorydefinitions.ScaffoldConfig) error {
			called = true
			return nil
		},
	})
	cmd := &cobra.Command{Use: "init"}
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if !called {
		t.Fatal("expected init handler to invoke handwritten service path")
	}
}

func TestNewFactoryConfigInitRegistryRejectsMissingHandlers(t *testing.T) {
	handlers := factoryConfigInitNoopHandlers()
	handlers.FactoryQueryRunE = nil
	if _, err := commandregistry.NewFactoryConfigInitRegistry(handlers); err == nil {
		t.Fatal("NewFactoryConfigInitRegistry() missing query handler = nil, want error")
	}
}

func factoryConfigInitNoopHandlers() commandregistry.FactoryConfigInitHandlers {
	return commandregistry.FactoryConfigInitHandlers{
		FactoryQueryRunE:          noopRunE,
		FactoryListRunE:           noopRunE,
		FactoryCreateRunE:         noopRunE,
		FactoryUpdateRunE:         noopRunE,
		FactoryDeleteRunE:         noopRunE,
		FactoryReplaceCurrentRunE: noopRunE,
		FactoryConfigValidateRunE: noopRunE,
		FactoryConfigFlattenRunE:  noopRunE,
		FactoryConfigExpandRunE:   noopRunE,
		ConfigInitRunE:            noopRunE,
		InitRunE:                  noopRunE,
	}
}

func TestFactoryListRunEMapsBindings(t *testing.T) {
	dir := "my-factory"
	json := true
	runE := commandregistry.FactoryListRunE(commandregistry.FactoryListBinding{
		Dir:  &dir,
		JSON: &json,
		List: func(cfg factorycli.ListConfig) error {
			if cfg.Dir != dir || !cfg.JSON {
				t.Fatalf("list config = %+v, want dir=%q json=true", cfg, dir)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "list"}
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestFactoryCreateRunEMapsBindings(t *testing.T) {
	dir := "factory"
	from := "./factory.json"
	setCurrent := true
	json := true
	runE := commandregistry.FactoryCreateRunE(commandregistry.FactoryCreateBinding{
		Dir:        &dir,
		From:       &from,
		SetCurrent: &setCurrent,
		JSON:       &json,
		Create: func(cfg factorycli.CreateFromFileConfig) error {
			if cfg.Name != "staging" || cfg.Dir != dir || cfg.From != from || !cfg.SetCurrent || !cfg.JSON {
				t.Fatalf("create config = %+v", cfg)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "create"}
	if err := runE(cmd, []string{"staging"}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestFactoryUpdateRunEMapsBindings(t *testing.T) {
	dir := "factory"
	from := "./factory.json"
	runE := commandregistry.FactoryUpdateRunE(commandregistry.FactoryUpdateBinding{
		Dir:  &dir,
		From: &from,
		Update: func(cfg factorycli.UpdateFromFileConfig) error {
			if cfg.Name != "staging" || cfg.Dir != dir || cfg.From != from {
				t.Fatalf("update config = %+v", cfg)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "update"}
	if err := runE(cmd, []string{"staging"}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestFactoryDeleteRunEMapsBindings(t *testing.T) {
	dir := "factory"
	runE := commandregistry.FactoryDeleteRunE(commandregistry.FactoryDeleteBinding{
		Dir: &dir,
		Delete: func(cfg factorycli.DeleteConfig) error {
			if cfg.Name != "staging" || cfg.Dir != dir {
				t.Fatalf("delete config = %+v", cfg)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "delete"}
	if err := runE(cmd, []string{"staging"}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestFactoryReplaceCurrentRunEMapsBindings(t *testing.T) {
	server := "http://127.0.0.1:8080"
	sessionID := "session-beta"
	verbose := true
	var diagnostic bytes.Buffer
	runE := commandregistry.FactoryReplaceCurrentRunE(commandregistry.FactoryReplaceCurrentBinding{
		Server:    &server,
		SessionID: &sessionID,
		Verbose:   func() bool { return verbose },
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return &diagnostic
		},
		ReplaceCurrent: func(cfg factorycli.ReplaceCurrentConfig) error {
			if cfg.Server != server || cfg.SessionID != sessionID || !cfg.Verbose || cfg.Diagnostics != &diagnostic {
				t.Fatalf("replace-current config = %+v", cfg)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "replace-current"}
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestFactoryConfigValidateRunEMapsBindings(t *testing.T) {
	json := true
	runE := commandregistry.FactoryConfigValidateRunE(commandregistry.FactoryConfigValidateBinding{
		JSON: &json,
		Validate: func(cfg factorycli.ValidateConfig) error {
			if cfg.Path != "./factory.json" || !cfg.JSON {
				t.Fatalf("validate config = %+v", cfg)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "validate"}
	if err := runE(cmd, []string{"./factory.json"}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestFactoryConfigFlattenRunEMapsBindings(t *testing.T) {
	debug := true
	var diagnostic bytes.Buffer
	runE := commandregistry.FactoryConfigFlattenRunE(commandregistry.FactoryConfigFlattenBinding{
		Debug:   &debug,
		Verbose: func() bool { return true },
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return &diagnostic
		},
		Flatten: func(cfg configcli.FactoryConfigFlattenConfig) error {
			if cfg.Path != "./factory" || !cfg.Verbose || !cfg.Debug || cfg.Diagnostics != &diagnostic {
				t.Fatalf("flatten config = %+v", cfg)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "flatten"}
	if err := runE(cmd, []string{"./factory"}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestFactoryConfigExpandRunEMapsBindings(t *testing.T) {
	debug := true
	runE := commandregistry.FactoryConfigExpandRunE(commandregistry.FactoryConfigExpandBinding{
		Debug:   &debug,
		Verbose: func() bool { return true },
		Expand: func(cfg configcli.FactoryConfigExpandConfig) error {
			if cfg.Path != "./factory.json" || !cfg.Verbose || !cfg.Debug {
				t.Fatalf("expand config = %+v", cfg)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "expand"}
	if err := runE(cmd, []string{"./factory.json"}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestConfigInitRunEPropagatesHomeDirError(t *testing.T) {
	runE := commandregistry.ConfigInitRunE(commandregistry.ConfigInitBinding{
		HomeDir: func() (string, error) {
			return "", errors.New("home dir unavailable")
		},
		Init: func(configinitcmd.InitConfig) error {
			t.Fatal("Init must not run when HomeDir fails")
			return nil
		},
	})
	cmd := &cobra.Command{Use: "init"}
	if err := runE(cmd, nil); err == nil {
		t.Fatal("RunE() error = nil, want home dir failure")
	}
}

func TestInitRunEMapsBindings(t *testing.T) {
	dir := "factory"
	scaffoldType := "ralph"
	executor := "codex"
	debug := true
	runE := commandregistry.InitRunE(commandregistry.InitBinding{
		Dir:      &dir,
		Type:     &scaffoldType,
		Executor: &executor,
		Debug:    &debug,
		Verbose:  func() bool { return true },
		Init: func(cfg factorydefinitions.ScaffoldConfig) error {
			if cfg.Dir != dir || cfg.Type != scaffoldType || cfg.Executor != executor || !cfg.Debug || !cfg.Verbose {
				t.Fatalf("init config = %+v", cfg)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "init"}
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestFactoryQueryRunEMapsVerboseAndDebugBindings(t *testing.T) {
	verbose := true
	debug := true
	runE := commandregistry.FactoryQueryRunE(commandregistry.FactoryQueryBinding{
		Verbose: func() bool { return verbose },
		Debug:   &debug,
		Query: func(cfg factorycli.QueryConfig) error {
			if !cfg.Verbose || !cfg.Debug {
				t.Fatalf("query config = %+v, want verbose and debug", cfg)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "query"}
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestRunnableFactoryConfigInitCommandIDsFromGeneratedManifest(t *testing.T) {
	manifest, err := generated.FactoryConfigInitFamilyManifest()
	if err != nil {
		t.Fatalf("FactoryConfigInitFamilyManifest() error = %v", err)
	}
	ids, err := commandregistry.RunnableFactoryConfigInitCommandIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableFactoryConfigInitCommandIDs() error = %v", err)
	}
	want := []string{
		"you.config.init",
		"you.factory.config.expand",
		"you.factory.config.flatten",
		"you.factory.config.validate",
		"you.factory.create",
		"you.factory.delete",
		"you.factory.list",
		"you.factory.query",
		"you.factory.replace-current",
		"you.factory.update",
		"you.init",
	}
	if len(ids) != len(want) {
		t.Fatalf("runnable IDs = %#v, want %#v", ids, want)
	}
	for i, id := range want {
		if ids[i] != id {
			t.Fatalf("runnable IDs[%d] = %q, want %q", i, ids[i], id)
		}
	}
}

func TestVerifyFactoryConfigInitRunnableCoverageRejectsMissingHandler(t *testing.T) {
	manifest, err := generated.FactoryConfigInitFamilyManifest()
	if err != nil {
		t.Fatalf("FactoryConfigInitFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you.factory.query", noopRunE); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.VerifyFactoryConfigInitRunnableCoverage(manifest); err == nil {
		t.Fatal("VerifyFactoryConfigInitRunnableCoverage() missing handlers = nil, want error")
	}
}

func TestVerifyFactoryConfigInitRunnableCoverageAcceptsCompleteRegistry(t *testing.T) {
	manifest, err := generated.FactoryConfigInitFamilyManifest()
	if err != nil {
		t.Fatalf("FactoryConfigInitFamilyManifest() error = %v", err)
	}
	runnableIDs, err := commandregistry.RunnableFactoryConfigInitCommandIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableFactoryConfigInitCommandIDs() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	for _, commandID := range runnableIDs {
		if err := registry.Register(commandID, noopRunE); err != nil {
			t.Fatalf("Register(%q) error = %v", commandID, err)
		}
	}
	if err := registry.VerifyFactoryConfigInitRunnableCoverage(manifest); err != nil {
		t.Fatalf("VerifyFactoryConfigInitRunnableCoverage() error = %v", err)
	}
}
