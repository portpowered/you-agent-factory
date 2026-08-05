package systeminitialization_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	systeminitializationwire "github.com/portpowered/infinite-you/pkg/services/system_initialization/wire"
)

type definitionsPackagingRecorder struct {
	definitions []factorydefinitions.PackagedDefinition
	installs    []factorydefinitions.InstallPackagedFactoryRequest
	installErr  error
}

func (packaging *definitionsPackagingRecorder) ListBuiltInPackagedFactories(
	context.Context,
	factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	entries := make([]factorydefinitions.BuiltInPackagedFactoryEntry, len(packaging.definitions))
	for index, definition := range packaging.definitions {
		entries[index] = factorydefinitions.BuiltInPackagedFactoryEntry{
			Name: definition.Name, Project: definition.Project,
			Formats: append([]factorydefinitions.PackagedFactoryFormat(nil), definition.Formats...),
		}
	}
	return factorydefinitions.ListBuiltInPackagedFactoriesResult{Entries: entries}, nil
}

func (packaging *definitionsPackagingRecorder) ResolveBuiltInPackagedFactory(
	_ context.Context,
	request factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
	for _, definition := range packaging.definitions {
		if definition.Name == request.Name {
			return factorydefinitions.ResolveBuiltInPackagedFactoryResult{
				Definition: definition,
				Formats:    append([]factorydefinitions.PackagedFactoryFormat(nil), definition.Formats...),
			}, nil
		}
	}
	return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, factorydefinitions.ErrUnknownPackagedFactoryIdentity
}

func (packaging *definitionsPackagingRecorder) InstallPackagedFactory(
	_ context.Context,
	request factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
	packaging.installs = append(packaging.installs, request)
	if packaging.installErr != nil {
		return factorydefinitions.InstallPackagedFactoryResult{}, packaging.installErr
	}

	factoryDir := filepath.Join(request.RootDir, strings.TrimPrefix(request.Name, "@you/"))
	outcome := factorydefinitions.PackagedFactoryInstallCreated
	if len(packaging.installs) > 1 {
		outcome = factorydefinitions.PackagedFactoryInstallSkipped
	} else {
		if err := os.MkdirAll(factoryDir, 0o755); err != nil {
			return factorydefinitions.InstallPackagedFactoryResult{}, err
		}
		if err := os.WriteFile(filepath.Join(factoryDir, "customer-owned.txt"), []byte("bootstrap-created\n"), 0o600); err != nil {
			return factorydefinitions.InstallPackagedFactoryResult{}, err
		}
	}
	return factorydefinitions.InstallPackagedFactoryResult{
		Definition: factorydefinitions.DistributedFactoryDefinitionFacts{
			Name: request.Name, FactoryDir: factoryDir,
		},
		Outcome: outcome,
		Format:  request.Format,
	}, nil
}

func newDefinitionsRootService(
	t *testing.T,
	settings systeminitializationwire.OperatorSettings,
	packaging factorydefinitions.Packaging,
) systeminitialization.Service {
	t.Helper()

	service, err := systeminitializationwire.NewService(
		settings,
		packaging,
		os.Stat,
		localMigrationFileSystem{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

// TestInitializeDefinitionsRootBoundary_CreatedThenSkippedPreservesCustomerFactoryContent
// proves Bootstrap uses the focused Packaging capability to retain its
// customer-visible create/skip behavior without rewriting customer-owned
// Factory files.
func TestInitializeDefinitionsRootBoundary_CreatedThenSkippedPreservesCustomerFactoryContent(t *testing.T) {
	t.Parallel()

	packaging := &definitionsPackagingRecorder{definitions: []factorydefinitions.PackagedDefinition{{
		Name:    "@you/goal",
		JSON:    []byte(`{}`),
		Formats: []factorydefinitions.PackagedFactoryFormat{factorydefinitions.PackagedFactoryFormatJSON},
	}}}
	service := newDefinitionsRootService(t, &routingOperatorSettings{}, packaging)

	homeDir := t.TempDir()
	first, err := service.Initialize(context.Background(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("first Initialize() error = %v", err)
	}
	if len(first.PackagedFactories) != 1 ||
		first.PackagedFactories[0].Name != "@you/goal" ||
		first.PackagedFactories[0].Outcome != systeminitialization.PackagedFactoryCreated {
		t.Fatalf("first packaged factories = %#v, want one created @you/goal", first.PackagedFactories)
	}

	factoryMarker := filepath.Join(first.PackagedFactories[0].FactoryDir, "customer-owned.txt")
	customerContent := []byte("customer-edited\n")
	if err := os.WriteFile(factoryMarker, customerContent, 0o600); err != nil {
		t.Fatal(err)
	}

	second, err := service.Initialize(context.Background(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("second Initialize() error = %v", err)
	}
	if len(second.PackagedFactories) != 1 ||
		second.PackagedFactories[0].Outcome != systeminitialization.PackagedFactorySkipped {
		t.Fatalf("second packaged factories = %#v, want one skipped @you/goal", second.PackagedFactories)
	}

	after, err := os.ReadFile(factoryMarker)
	if err != nil {
		t.Fatalf("read factory marker after repeat = %v", err)
	}
	if string(after) != string(customerContent) {
		t.Fatalf("factory content rewritten on repeat: before %q after %q", customerContent, after)
	}
	if len(packaging.installs) != 2 ||
		packaging.installs[0].Format != factorydefinitions.PackagedFactoryFormatJSON ||
		packaging.installs[1].Format != factorydefinitions.PackagedFactoryFormatJSON {
		t.Fatalf("focused packaging requests = %#v, want ordered JSON installs", packaging.installs)
	}
}

// TestInitializeDefinitionsRootBoundary_PartialFailurePreservesPackageClassification
// proves a Packaging failure remains inspectable through Bootstrap's existing
// partial-failure result instead of being converted to an untyped error.
func TestInitializeDefinitionsRootBoundary_PartialFailurePreservesPackageClassification(t *testing.T) {
	t.Parallel()

	installErr := factorydefinitions.NewPackagedFactoryInputError(
		factorydefinitions.PackagedFactoryErrorIntegrity,
		"@you/goal",
		factorydefinitions.PackagedFactoryFormatJSON,
		"prompts/build.md",
		errors.New("digest differs"),
	)
	service := newDefinitionsRootService(t, &routingOperatorSettings{}, &definitionsPackagingRecorder{
		definitions: []factorydefinitions.PackagedDefinition{{
			Name:    "@you/goal",
			Formats: []factorydefinitions.PackagedFactoryFormat{factorydefinitions.PackagedFactoryFormatJSON},
		}},
		installErr: installErr,
	})

	_, err := service.Initialize(context.Background(), systeminitialization.Request{HomeDir: t.TempDir()})
	if !errors.Is(err, systeminitialization.ErrInitializePartialFailure) {
		t.Fatalf("Initialize() error = %v, want ErrInitializePartialFailure", err)
	}
	if !errors.Is(err, factorydefinitions.ErrPackagedFactoryIntegrity) {
		t.Fatalf("Initialize() error = %v, want package integrity classification", err)
	}
	var partialFailure systeminitialization.InitializePartialFailure
	if !errors.As(err, &partialFailure) {
		t.Fatalf("Initialize() error = %T(%v), want InitializePartialFailure", err, err)
	}
	if len(partialFailure.Facts) != 3 ||
		partialFailure.Facts[2].Step != systeminitialization.InitializeStepPackagedFactories ||
		partialFailure.Facts[2].Outcome != systeminitialization.RollbackStepUnresolved {
		t.Fatalf("Initialize() rollback facts = %#v", partialFailure.Facts)
	}
}
