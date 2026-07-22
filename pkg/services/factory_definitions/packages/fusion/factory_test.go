package fusion

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestBuiltInFactoryJSON_LoadsRunnablePackagedFusionFactory(t *testing.T) {
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Name != PackagedFactoryName {
		t.Fatalf("factory name = %q, want %s", cfg.Name, PackagedFactoryName)
	}
	if cfg.Project != PackagedFactoryProject {
		t.Fatalf("factory project = %q, want %s", cfg.Project, PackagedFactoryProject)
	}
	if cfg.InvocationSignature == nil {
		t.Fatal("InvocationSignature = nil, want packaged signature")
	}
	if len(cfg.Workers) != 2 {
		t.Fatalf("workers = %#v, want two fusion workers", cfg.Workers)
	}
	if len(cfg.Workstations) != 2 {
		t.Fatalf("workstations = %#v, want two fusion workstations", cfg.Workstations)
	}

	for _, target := range factoryvalidation.Validate(cfg).Targets {
		if strings.HasPrefix(target.Code, "factory.invocationSignature.") {
			t.Fatalf("validation target = %#v, want valid packaged fusion signature", target)
		}
	}
}

func TestMaterializedPackagedFusionFactory_PreservesInvocationSignatureAndInterpolationFields(t *testing.T) {
	globalRoot := t.TempDir()
	if _, err := factorydefinitioncomposition.PersistNamedFactory(globalRoot, PackagedFactoryName, BuiltInFactoryJSON, factoryvalidation.New(nil)); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	resolution, err := factorydefinitioncomposition.NamedFactoryCatalog().ResolveNamedFactoryAcrossRoots(t.TempDir(), globalRoot, PackagedFactoryName)
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots: %v", err)
	}
	if resolution.Name != PackagedFactoryName {
		t.Fatalf("resolution name = %q, want %s", resolution.Name, PackagedFactoryName)
	}

	loaded, err := factorydefinitioncomposition.LoadDirectory(
		resolution.FactoryDir,
		nil,
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(%q): %v", resolution.FactoryDir, err)
	}
	cfg := loaded.FactoryConfig()
	if cfg.Project != PackagedFactoryProject {
		t.Fatalf("materialized project = %q, want %s", cfg.Project, PackagedFactoryProject)
	}
	if cfg.InvocationSignature == nil {
		t.Fatal("materialized InvocationSignature = nil, want preserved signature")
	}
	if got := cfg.Workers[0].ModelProvider; got != "${firstProvider}" {
		t.Fatalf("workers[0].modelProvider = %q, want ${firstProvider}", got)
	}
	if got := cfg.Workers[1].Model; got != "${secondModel}" {
		t.Fatalf("workers[1].model = %q, want ${secondModel}", got)
	}
}

func TestMaterializedPackagedFusionFactory_EditAndRereadRetainsInvocationSignature(t *testing.T) {
	original, err := factorymapping.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	factoryDir, err := factorydefinitioncomposition.PersistNamedFactory(t.TempDir(), PackagedFactoryName, BuiltInFactoryJSON, factoryvalidation.New(nil))
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	factoryJSONPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	factoryJSON, err := os.ReadFile(factoryJSONPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var factoryDoc map[string]any
	if err := json.Unmarshal(factoryJSON, &factoryDoc); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	factoryDoc["id"] = "customer-fusion"
	editedJSON, err := json.MarshalIndent(factoryDoc, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(edited factory.json): %v", err)
	}
	if err := os.WriteFile(factoryJSONPath, editedJSON, 0o644); err != nil {
		t.Fatalf("WriteFile(edited factory.json): %v", err)
	}

	loaded, err := factorydefinitioncomposition.LoadDirectory(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(%q): %v", factoryDir, err)
	}
	if got := loaded.FactoryConfig().Project; got != "customer-fusion" {
		t.Fatalf("edited project = %q, want customer-fusion from factory id", got)
	}
	if !reflect.DeepEqual(loaded.FactoryConfig().InvocationSignature, original.InvocationSignature) {
		t.Fatalf("invocationSignature changed after edit/reload = %#v, want %#v", loaded.FactoryConfig().InvocationSignature, original.InvocationSignature)
	}
}

func TestBuiltInFusionFactory_NormalizesMixedPositionalAndNamedArgumentsThroughSharedPath(t *testing.T) {
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	got, err := work.NormalizeArguments(work.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Draft a release summary"},
		NamedArgs: []work.NamedArgumentInput{
			{Key: "first-provider", Values: []string{"CLAUDE"}},
			{Key: "second-model", Values: []string{"gpt-5"}},
			{Key: "output", Values: []string{"fusion-summary.md"}},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments: %v", err)
	}

	assertArgumentValues(t, got.Arguments, "input", []string{"Draft a release summary"})
	assertArgumentValues(t, got.Arguments, "firstProvider", []string{"CLAUDE"})
	assertArgumentValues(t, got.Arguments, "secondModel", []string{"gpt-5"})
	assertArgumentValues(t, got.Arguments, "output", []string{"fusion-summary.md"})
	assertArgumentValues(t, got.Arguments, "firstEffort", []string{"medium"})
	assertArgumentValues(t, got.Arguments, "secondEffort", []string{"medium"})
}

func TestIsPackagedFactory_MatchesBuiltInFusionIdentity(t *testing.T) {
	if !IsPackagedFactory(&interfaces.FactoryConfig{Name: PackagedFactoryName}) {
		t.Fatal("expected packaged factory name match")
	}
	if !IsPackagedFactory(&interfaces.FactoryConfig{Project: PackagedFactoryProject}) {
		t.Fatal("expected packaged factory project match")
	}
	if IsPackagedFactory(&interfaces.FactoryConfig{Name: "customer-fusion"}) {
		t.Fatal("unexpected packaged factory match for unrelated factory")
	}
	if IsPackagedFactory(nil) {
		t.Fatal("expected nil factory config not to match")
	}
}

func assertArgumentValues(t *testing.T, arguments map[string]work.NormalizedArgument, name string, want []string) {
	t.Helper()
	got, ok := arguments[name]
	if !ok {
		t.Fatalf("argument %q missing from %#v", name, arguments)
	}
	if !reflect.DeepEqual(got.Values, want) {
		t.Fatalf("argument %q values = %#v, want %#v", name, got.Values, want)
	}
}
