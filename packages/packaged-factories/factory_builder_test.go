package packagedfactories_test

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
)

const factoryBuilderSlug = "factory-builder"

func TestFactoryBuilderPublishedInvocation(t *testing.T) {
	t.Parallel()

	published := packagedfactories.Published()
	manifestPayload, err := fs.ReadFile(published, manifestPath)
	if err != nil {
		t.Fatalf("read published manifest: %v", err)
	}
	manifest := decodeFactoryBuilderManifest(t, manifestPayload)
	entry := findFactoryBuilderManifestEntry(t, manifest)
	if entry.Project != "builtin-factory-builder" || entry.Slug != factoryBuilderSlug {
		t.Fatalf("Factory Builder catalog entry = %#v, want builtin factory-builder identity", entry)
	}
	if entry.Description.Value != "Creates and installs one validated graph or JavaScript Factory from a customer request." {
		t.Fatalf("Factory Builder description = %q", entry.Description.Value)
	}
	if len(entry.Examples) != 2 {
		t.Fatalf("Factory Builder examples = %#v, want graph and JavaScript examples", entry.Examples)
	}

	factoryPayload, err := fs.ReadFile(published, entry.JSON.Locator)
	if err != nil {
		t.Fatalf("read published Factory Builder JSON: %v", err)
	}
	var factoryBuilder factoryBuilderFactory
	if err := json.Unmarshal(factoryPayload, &factoryBuilder); err != nil {
		t.Fatalf("decode published Factory Builder JSON: %v", err)
	}
	if factoryBuilder.Name != factoryBuilderSlug || factoryBuilder.ID != "builtin-factory-builder" {
		t.Fatalf("published Factory Builder identity = %#v", factoryBuilder)
	}
	assertFactoryBuilderParameter(t, factoryBuilder.InvocationSignature.Parameters, "request", "to", true, nil)
	assertFactoryBuilderParameter(t, factoryBuilder.InvocationSignature.Parameters, "factoryName", "factory-name", true, nil)
	assertFactoryBuilderParameter(t, factoryBuilder.InvocationSignature.Parameters, "orchestrator", "orchestrator", false, []string{"graph", "javascript"})
	assertFactoryBuilderParameter(t, factoryBuilder.InvocationSignature.Parameters, "builderProvider", "builder-provider", false, nil)
	assertFactoryBuilderParameter(t, factoryBuilder.InvocationSignature.Parameters, "builderModel", "builder-model", false, nil)
	if factoryBuilder.WorkerToolPolicy("factory-builder") != "ENABLED" {
		t.Fatalf("Factory Builder tool policy = %q, want ENABLED", factoryBuilder.WorkerToolPolicy("factory-builder"))
	}

}

type factoryBuilderManifest struct {
	Factories []factoryBuilderManifestEntry `json:"factories"`
}

type factoryBuilderManifestEntry struct {
	Name        string                  `json:"name"`
	Project     string                  `json:"project"`
	Slug        string                  `json:"slug"`
	JSON        publicationArtifact     `json:"json"`
	Description factoryBuilderAsset     `json:"description"`
	Examples    []factoryBuilderExample `json:"examples"`
}

type factoryBuilderAsset struct {
	Value string `json:"value"`
}

type factoryBuilderExample struct {
	Name string `json:"name"`
}

type factoryBuilderFactory struct {
	Name                string `json:"name"`
	ID                  string `json:"id"`
	InvocationSignature struct {
		Parameters []factoryBuilderParameter `json:"parameters"`
	} `json:"invocationSignature"`
	Workers []struct {
		Name       string `json:"name"`
		AgentTools struct {
			Policy string `json:"policy"`
		} `json:"agentTools"`
	} `json:"workers"`
}

type factoryBuilderParameter struct {
	Name         string   `json:"name"`
	ExternalName string   `json:"externalName"`
	Required     bool     `json:"required"`
	Choices      []string `json:"choices"`
}

func decodeFactoryBuilderManifest(t *testing.T, payload []byte) factoryBuilderManifest {
	t.Helper()
	var manifest factoryBuilderManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("decode published manifest: %v", err)
	}
	return manifest
}

func findFactoryBuilderManifestEntry(t *testing.T, manifest factoryBuilderManifest) factoryBuilderManifestEntry {
	t.Helper()
	for _, entry := range manifest.Factories {
		if entry.Name == "@you/factory-builder" {
			return entry
		}
	}
	t.Fatal("published manifest does not contain @you/factory-builder")
	return factoryBuilderManifestEntry{}
}

func assertFactoryBuilderParameter(
	t *testing.T,
	parameters []factoryBuilderParameter,
	name, externalName string,
	required bool,
	choices []string,
) {
	t.Helper()
	for _, parameter := range parameters {
		if parameter.Name != name {
			continue
		}
		if parameter.ExternalName != externalName || parameter.Required != required {
			t.Fatalf("Factory Builder parameter %q = %#v", name, parameter)
		}
		if strings.Join(parameter.Choices, ",") != strings.Join(choices, ",") {
			t.Fatalf("Factory Builder parameter %q choices = %#v, want %#v", name, parameter.Choices, choices)
		}
		return
	}
	t.Fatalf("Factory Builder invocation is missing parameter %q", name)
}

func (factory factoryBuilderFactory) WorkerToolPolicy(name string) string {
	for _, worker := range factory.Workers {
		if worker.Name == name {
			return worker.AgentTools.Policy
		}
	}
	return ""
}
