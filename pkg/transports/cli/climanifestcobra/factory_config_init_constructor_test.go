package climanifestcobra_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

func TestNewFactoryConfigInitFamilyComponentsBuildsContractedPaths(t *testing.T) {
	components, registry := mustFactoryConfigInitFamilyComponents(t)

	factory := components.Factory
	if factory.Name() != "factory" {
		t.Fatalf("factory name = %q, want factory", factory.Name())
	}
	if factory.RunE == nil {
		t.Fatal("you factory group parent must wire unknown-subcommand guard RunE")
	}

	factoryConfig, err := climanifestparity.FindCommandByPath(factory, "factory config")
	if err != nil {
		t.Fatalf("FindCommandByPath(factory config) error = %v", err)
	}
	if factoryConfig.Name() != "config" {
		t.Fatalf("factory config name = %q, want config", factoryConfig.Name())
	}

	query, err := climanifestparity.FindCommandByPath(factory, "factory query")
	if err != nil {
		t.Fatalf("FindCommandByPath(factory query) error = %v", err)
	}
	if !query.Runnable() || query.RunE == nil {
		t.Fatal("you factory query must attach handwritten RunE")
	}
	if _, err := registry.Lookup("you.factory.query"); err != nil {
		t.Fatalf("Lookup(you.factory.query) error = %v", err)
	}

	if components.Config.Name() != "config" {
		t.Fatalf("config name = %q, want config", components.Config.Name())
	}
	if _, err := climanifestparity.FindCommandByPath(components.Config, "config init"); err != nil {
		t.Fatalf("missing config init: %v", err)
	}

	if components.Init.Name() != "init" {
		t.Fatalf("init name = %q, want init", components.Init.Name())
	}
	if !components.Init.Runnable() || components.Init.RunE == nil {
		t.Fatal("you init must attach handwritten RunE")
	}
}

func TestNewFactoryConfigInitFamilyComponentsRejectsMissingHandler(t *testing.T) {
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you.factory.query", noopRunE); err != nil {
		t.Fatalf("Register(you.factory.query) error = %v", err)
	}
	if _, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(registry, testFactoryConfigInitBindings()); err == nil {
		t.Fatal("NewFactoryConfigInitFamilyComponents() missing handlers = nil, want error")
	}
}

func TestNewFactoryConfigInitFamilyComponentsExposesOnlyFactoryConfigInitFamily(t *testing.T) {
	components, _ := mustFactoryConfigInitFamilyComponents(t)
	for _, id := range climanifestgen.FactoryConfigInitFamilyCommandIDs {
		path := factoryConfigInitPathForID(id)
		root := factoryConfigInitRootForID(components, id)
		if _, err := climanifestparity.FindCommandByPath(root, path); err != nil {
			t.Fatalf("path for %q missing: %v", id, err)
		}
	}
}

func mustFactoryConfigInitFamilyComponents(t *testing.T) (climanifestcobra.FactoryConfigInitFamilyComponents, *commandregistry.Registry) {
	t.Helper()
	registry, err := commandregistry.NewFactoryConfigInitRegistry(factoryConfigInitNoopHandlers())
	if err != nil {
		t.Fatalf("NewFactoryConfigInitRegistry() error = %v", err)
	}
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(registry, testFactoryConfigInitBindings())
	if err != nil {
		t.Fatalf("NewFactoryConfigInitFamilyComponents() error = %v", err)
	}
	return components, registry
}

func testFactoryConfigInitBindings() climanifestcobra.FactoryConfigInitFlagBindings {
	listDir := "factory"
	createDir := "factory"
	updateDir := "factory"
	deleteDir := "factory"
	createFrom := ""
	createSetCurrent := false
	updateFrom := ""
	replaceSessionID := ""
	initDir := "factory"
	initType := "default"
	initExecutor := "codex"
	return climanifestcobra.FactoryConfigInitFlagBindings{
		FactoryListDir:          &listDir,
		FactoryCreateDir:        &createDir,
		FactoryUpdateDir:        &updateDir,
		FactoryDeleteDir:        &deleteDir,
		FactoryCreateFrom:       &createFrom,
		FactoryCreateSetCurrent: &createSetCurrent,
		FactoryUpdateFrom:       &updateFrom,
		FactoryReplaceSessionID: &replaceSessionID,
		InitDir:                 &initDir,
		InitType:                &initType,
		InitExecutor:            &initExecutor,
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

func factoryConfigInitPathForID(commandID string) string {
	manifest, err := generated.FactoryConfigInitFamilyManifest()
	if err != nil {
		panic(err)
	}
	record, err := manifest.CommandByID(commandID)
	if err != nil {
		panic(err)
	}
	parts := strings.Split(record.Path, " ")
	if len(parts) < 2 {
		return record.Path
	}
	return strings.Join(parts[1:], " ")
}

func factoryConfigInitRootForID(components climanifestcobra.FactoryConfigInitFamilyComponents, commandID string) *cobra.Command {
	switch {
	case strings.HasPrefix(commandID, "you.factory"):
		return components.Factory
	case strings.HasPrefix(commandID, "you.config"):
		return components.Config
	case commandID == "you.init":
		return components.Init
	default:
		panic("unexpected factory/config/init command id: " + commandID)
	}
}
