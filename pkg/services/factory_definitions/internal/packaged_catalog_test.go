package internal_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
)

func TestServiceListsAndResolvesDetachedDefinitionsInPublicNameOrder(t *testing.T) {
	service, err := factorydefinitionsinternal.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{
		{
			Name: "@you/review", Project: "builtin-review",
			JSON: []byte(`{"name":"review"}`), YAML: []byte("name: review\n"),
			Formats: []factorydefinitions.PackagedFactoryFormat{
				factorydefinitions.PackagedFactoryFormatJSON,
				factorydefinitions.PackagedFactoryFormatYAML,
			},
		},
		{
			Name: "@you/goal", Project: "builtin-goal",
			JSON: []byte(`{"name":"goal"}`), YAML: []byte("name: goal\n"),
			Formats: []factorydefinitions.PackagedFactoryFormat{
				factorydefinitions.PackagedFactoryFormatJSON,
				factorydefinitions.PackagedFactoryFormatYAML,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	listed, err := service.ListBuiltInPackagedFactories(
		t.Context(),
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	gotNames := []string{listed.Entries[0].Name, listed.Entries[1].Name}
	if !reflect.DeepEqual(gotNames, []string{"@you/goal", "@you/review"}) {
		t.Fatalf("listed names = %v", gotNames)
	}

	resolved, err := service.ResolveBuiltInPackagedFactory(
		t.Context(),
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: "@you/goal"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Definition.Project != "builtin-goal" ||
		!reflect.DeepEqual(resolved.Formats, []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
			factorydefinitions.PackagedFactoryFormatYAML,
		}) {
		t.Fatalf("resolved = %#v", resolved)
	}
	resolved.Definition.JSON[0] = 'x'
	resolved.Formats[0] = "changed"
	again, err := service.ResolveBuiltInPackagedFactory(
		t.Context(),
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: "@you/goal"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if again.Definition.JSON[0] == 'x' || again.Formats[0] != "JSON" {
		t.Fatal("resolve returned shared mutable catalog data")
	}
}

func TestServiceUnknownNameReportsStablePublicInventory(t *testing.T) {
	service, err := factorydefinitionsinternal.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{
		{Name: "@you/zeta", Formats: []factorydefinitions.PackagedFactoryFormat{factorydefinitions.PackagedFactoryFormatJSON}},
		{Name: "@you/alpha", Formats: []factorydefinitions.PackagedFactoryFormat{factorydefinitions.PackagedFactoryFormatJSON}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ResolveBuiltInPackagedFactory(
		t.Context(),
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: "@you/missing"},
	)
	if !errors.Is(err, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("resolve error = %v", err)
	}
	if !strings.Contains(err.Error(), "@you/alpha, @you/zeta") ||
		strings.Contains(err.Error(), "generated/") {
		t.Fatalf("resolve error = %q", err)
	}
}

func TestServiceHonorsCancellation(t *testing.T) {
	service, err := factorydefinitionsinternal.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{{
		Name: "@you/goal",
		Formats: []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.ListBuiltInPackagedFactories(
		ctx,
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("list error = %v", err)
	}
}
