package contractstaging_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestGeneratedManifestAndConfigurationSchemasEnforceTheirPromisedContracts(t *testing.T) {
	t.Parallel()

	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	artifacts := testArtifactsForRepository(t, repositoryRoot)

	manifestPayload := artifacts["packages/api/generated/manifest.json"]
	var manifest map[string]any
	if err := json.Unmarshal(manifestPayload, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if _, ok := manifest["sourceCommit"]; ok {
		t.Fatalf("development manifest contains commit-derived provenance: %#v", manifest["sourceCommit"])
	}

	manifestSchema := compileArtifactSchema(t, artifacts["packages/api/generated/joined/contracts/manifest.schema.json"])
	assertArtifactValid(t, manifestSchema, artifacts["packages/api/generated/manifest.json"], true)

	cliSchema := compileCLIArtifactSchema(t, artifacts)
	assertArtifactValid(t, cliSchema, artifacts["packages/api/generated/cli/commands.json"], true)

	cases := []struct {
		path    string
		valid   string
		invalid string
	}{
		{
			path:    "packages/api/generated/schemas/you-config.schema.json",
			valid:   `{"backendScopeID":"local-example","defaults":{},"workerPresets":[]}`,
			invalid: `{"unexpected":true}`,
		},
		{
			path:    "packages/api/generated/schemas/factory.schema.json",
			valid:   `{"name":"example","invocationSignature":{"parameters":[]}}`,
			invalid: `{}`,
		},
		{
			path:    "packages/api/generated/schemas/mock-workers.schema.json",
			valid:   `{"mockWorkers":[{"runType":"script","scriptConfig":{"command":"echo"}}]}`,
			invalid: `{"mockWorkers":[{"runType":"script"}]}`,
		},
	}
	for _, test := range cases {
		t.Run(filepath.Base(test.path), func(t *testing.T) {
			schema := compileArtifactSchema(t, artifacts[test.path])
			assertArtifactValid(t, schema, []byte(test.valid), true)
			assertArtifactValid(t, schema, []byte(test.invalid), false)
		})
	}
}

func compileCLIArtifactSchema(t *testing.T, artifacts map[string][]byte) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	resources := []struct {
		id   string
		path string
	}{
		{
			id:   "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json",
			path: "packages/api/generated/joined/contracts/common/documentation.schema.json",
		},
		{
			id:   "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json",
			path: "packages/api/generated/joined/contracts/common/deprecations.schema.json",
		},
		{
			id:   "https://schemas.portpowered.com/you/contracts/cli/command-manifest.schema.json",
			path: "packages/api/generated/cli/command-manifest.schema.json",
		},
	}
	for _, resource := range resources {
		var document any
		if err := json.Unmarshal(artifacts[resource.path], &document); err != nil {
			t.Fatalf("decode %s: %v", resource.path, err)
		}
		if err := compiler.AddResource(resource.id, document); err != nil {
			t.Fatalf("add %s: %v", resource.path, err)
		}
	}
	schema, err := compiler.Compile(resources[len(resources)-1].id)
	if err != nil {
		t.Fatalf("compile published CLI schema: %v", err)
	}
	return schema
}

func compileArtifactSchema(t *testing.T, payload []byte) *jsonschema.Schema {
	t.Helper()
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("artifact.json", document); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	schema, err := compiler.Compile("artifact.json")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return schema
}

func assertArtifactValid(t *testing.T, schema *jsonschema.Schema, payload []byte, valid bool) {
	t.Helper()
	var instance any
	if err := json.Unmarshal(payload, &instance); err != nil {
		t.Fatalf("decode instance: %v", err)
	}
	err := schema.Validate(instance)
	if valid && err != nil {
		t.Fatalf("valid instance rejected: %v", err)
	}
	if !valid && err == nil {
		t.Fatal("invalid instance accepted")
	}
}
