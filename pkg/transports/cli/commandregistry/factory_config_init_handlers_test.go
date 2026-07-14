package commandregistry_test

import (
	"bytes"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	configinitcmd "github.com/portpowered/infinite-you/pkg/transports/cli/configinit"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	initcmd "github.com/portpowered/infinite-you/pkg/transports/cli/init"
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
		JSON: func() bool { return true },
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
		Init: func(initcmd.InitConfig) error {
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
