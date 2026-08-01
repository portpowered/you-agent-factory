package wire

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

func TestPackagedFactoryCatalogConsumption_ListResolveInstallUsesPublishedCatalog(t *testing.T) {
	definitions, err := providePackagedFactoryDefinitions()
	if err != nil {
		t.Fatalf("providePackagedFactoryDefinitions() error = %v", err)
	}
	catalog, err := providePackagedFactoryCatalog(definitions)
	if err != nil {
		t.Fatalf("providePackagedFactoryCatalog() error = %v", err)
	}

	listed, err := catalog.ListBuiltInPackagedFactories(
		t.Context(),
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatalf("ListBuiltInPackagedFactories() error = %v", err)
	}
	assertPublishedPackagedFactoryNames(t, listed)

	resolved, err := catalog.ResolveBuiltInPackagedFactory(
		t.Context(),
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: "@you/goal"},
	)
	if err != nil {
		t.Fatalf("ResolveBuiltInPackagedFactory(@you/goal) error = %v", err)
	}
	if resolved.Definition.Name != "@you/goal" || len(resolved.Definition.JSON) == 0 {
		t.Fatalf("resolved goal = %#v, want published goal definition bytes", resolved.Definition)
	}

	_, err = catalog.ResolveBuiltInPackagedFactory(
		t.Context(),
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: "@you/missing"},
	)
	if !errors.Is(err, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("ResolveBuiltInPackagedFactory(@you/missing) error = %v", err)
	}
	if !strings.Contains(err.Error(), "@you/deep-research") ||
		!strings.Contains(err.Error(), "@you/tts") ||
		strings.Contains(err.Error(), "generated/") {
		t.Fatalf("unknown resolve error = %q, want stable public inventory", err.Error())
	}

	assertPublishedPackagedFactoryInstall(t, catalog, resolved.Definition.JSON)
}

func assertPublishedPackagedFactoryNames(t *testing.T, listed factorydefinitions.ListBuiltInPackagedFactoriesResult) {
	t.Helper()
	gotNames := make([]string, len(listed.Entries))
	for index, entry := range listed.Entries {
		gotNames[index] = entry.Name
	}
	wantNames := []string{
		"@you/classify",
		"@you/deep-research",
		"@you/full-flow",
		"@you/fusion",
		"@you/goal",
		"@you/loop",
		"@you/plan-execute",
		"@you/plan-parallel",
		"@you/quorum",
		"@you/review",
		"@you/spawn",
		"@you/subagent",
		"@you/tournament",
		"@you/tts",
	}
	if len(gotNames) != len(wantNames) {
		t.Fatalf("listed count = %d, want %d", len(gotNames), len(wantNames))
	}
	for index, wantName := range wantNames {
		if gotNames[index] != wantName {
			t.Fatalf("listed[%d] = %q, want %q", index, gotNames[index], wantName)
		}
	}
}

func assertPublishedPackagedFactoryInstall(t *testing.T, catalog factorydefinitions.PackagedFactoryCatalogOperations, resolvedJSON []byte) {
	t.Helper()
	var installed factorydefinitions.PackagedDefinition
	install := factorydefinitionswire.NewInstallPackagedFactoryOperation(
		catalog,
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				_ context.Context,
				params factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				installed = params.Definition
				return factorydefinitions.PackagedFactoryInstallResult{
					Name:       params.Definition.Name,
					FactoryDir: "/customer/factories/@you/goal",
					Outcome:    factorydefinitions.PackagedFactoryInstallCreated,
					Format:     params.Format,
				}, nil
			},
		},
	)
	result, err := install(
		t.Context(),
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/customer/factories",
			Name:    "@you/goal",
			Format:  factorydefinitions.PackagedFactoryFormatJSON,
		},
	)
	if err != nil {
		t.Fatalf("InstallPackagedFactory(@you/goal) error = %v", err)
	}
	if !bytes.Equal(installed.JSON, resolvedJSON) {
		t.Fatal("install received definition bytes that differ from catalog resolve")
	}
	if result.Definition.Name != "@you/goal" ||
		result.Outcome != factorydefinitions.PackagedFactoryInstallCreated {
		t.Fatalf("install result = %#v, want created goal install", result)
	}

	_, err = install(
		t.Context(),
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/customer/factories",
			Name:    "@you/missing",
		},
	)
	if !errors.Is(err, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("InstallPackagedFactory(@you/missing) error = %v", err)
	}
}
