package mcpcontractcheck_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/mcpcontractcheck"
	"github.com/portpowered/infinite-you/internal/testutil"
)

func TestPublicToolInventoryIncludesCompleteUnion(t *testing.T) {
	t.Parallel()
	inputs, err := mcpcontractcheck.LoadInputs(testutil.MustRepoRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := mcpcontractcheck.ProjectToolInventory(inputs.Discovery, inputs.Registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Tools) != len(inputs.Discovery) {
		t.Fatalf("inventory count = %d, discovery = %d", len(inventory.Tools), len(inputs.Discovery))
	}
	workers := 0
	for _, tool := range inventory.Tools {
		if strings.HasPrefix(tool.Name, "you.worker_session.") {
			workers++
			if !tool.HandlerRegistered {
				t.Fatalf("unregistered Worker tool %s", tool.Name)
			}
		}
	}
	if workers != 3 {
		t.Fatalf("Worker tool count = %d, want 3", workers)
	}
	artifacts, err := mcpcontractcheck.GenerateInventoryArtifacts()
	if err != nil {
		t.Fatal(err)
	}
	var encoded struct {
		Tools []struct {
			Name              string
			HandlerRegistered bool
		}
	}
	if err := json.Unmarshal(artifacts[0].Payload, &encoded); err != nil {
		t.Fatal(err)
	}
	if len(encoded.Tools) != len(inventory.Tools) {
		t.Fatal("generator omits discovered tools")
	}
	for i, tool := range encoded.Tools {
		if tool.Name != inventory.Tools[i].Name || !tool.HandlerRegistered {
			t.Fatalf("generated identity mismatch: %#v", tool)
		}
	}
}

func TestPublicToolInventoryFailsClosedOnRegistryDrift(t *testing.T) {
	t.Parallel()
	for _, mutation := range []string{"missing", "duplicate", "extra", "wrong handler", "duplicate discovery", "invalid schema"} {
		t.Run(mutation, func(t *testing.T) {
			t.Parallel()
			inputs, err := mcpcontractcheck.LoadInputs(testutil.MustRepoRoot(t))
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "missing":
				inputs.Registry = inputs.Registry[1:]
			case "duplicate":
				inputs.Registry = append(inputs.Registry, inputs.Registry[0])
			case "extra":
				inputs.Registry = append(inputs.Registry, mcpcontractcheck.HandlerBinding{ToolID: "mcp.tool.you.extra", HandlerID: "mcp.handler.you.extra"})
			case "wrong handler":
				inputs.Registry[0].HandlerID = "mcp.handler.you.wrong"
			case "duplicate discovery":
				inputs.Discovery = append(inputs.Discovery, inputs.Discovery[0])
			case "invalid schema":
				inputs.Discovery[0].InputSchema = "object"
			}
			if _, err := mcpcontractcheck.ProjectToolInventory(inputs.Discovery, inputs.Registry); err == nil {
				t.Fatal("invalid inventory was accepted")
			}
		})
	}
}

func TestValidateCleanExplicitBoundaryInputs(t *testing.T) {
	t.Parallel()

	schema := map[string]any{"type": "object", "additionalProperties": false}
	inputs := mcpcontractcheck.Inputs{
		Catalog: []mcpcontractcheck.ToolRecord{{
			ID: "mcp.tool.you.factory_session.list", Name: "you.factory_session.list",
			Description: "List sessions.", InputSchema: schema,
			HandlerID: "mcp.handler.you.factory_session.list",
		}},
		Discovery: []mcpcontractcheck.ToolRecord{{
			ID: "mcp.tool.you.factory_session.list", Name: "you.factory_session.list",
			Description: "List sessions.", InputSchema: schema,
		}},
		Registry: []mcpcontractcheck.HandlerBinding{{
			ToolID: "mcp.tool.you.factory_session.list", HandlerID: "mcp.handler.you.factory_session.list",
		}},
		Aliases: []mcpcontractcheck.AliasBinding{{
			ID: "mcp.alias.you.workflow.status", Name: "you.workflow.status", CanonicalToolID: "mcp.tool.you.factory_session.list",
		}},
		RuntimeAliases: []mcpcontractcheck.RuntimeAliasBinding{{
			Name: "you.workflow.status", CanonicalName: "you.factory_session.list",
		}},
		Resources:          []mcpcontractcheck.ResourceRecord{{ID: "mcp.resource.providers.catalog", URI: "you://providers/catalog", Name: "provider-catalog", Description: "Provider catalog", MIMEType: "application/json", Handler: "mcp.handler.providers.catalog"}},
		GeneratedResources: []mcpcontractcheck.ResourceRecord{{ID: "mcp.resource.providers.catalog", URI: "you://providers/catalog", Name: "provider-catalog", Description: "Provider catalog", MIMEType: "application/json", Handler: "mcp.handler.providers.catalog"}},
		Skills:             []mcpcontractcheck.SkillRecord{{ID: "mcp.skill.subagent", URI: "skill://subagent/SKILL.md", Frontmatter: map[string]any{"name": "subagent", "description": "Configure subagents"}, ResourceURIs: []string{"skill://subagent/SKILL.md"}}},
		GeneratedSkills:    []mcpcontractcheck.SkillRecord{{ID: "mcp.skill.subagent", URI: "skill://subagent/SKILL.md", Frontmatter: map[string]any{"name": "subagent", "description": "Configure subagents"}, ResourceURIs: []string{"skill://subagent/SKILL.md"}}},
	}

	if diagnostics := mcpcontractcheck.Validate(inputs); len(diagnostics) != 0 {
		t.Fatalf("Validate() diagnostics = %+v, want none", diagnostics)
	}
}

func TestCompleteRegistryUnionRejectsWorkerBindingDrift(t *testing.T) {
	t.Parallel()
	root := testutil.MustRepoRoot(t)
	for _, mutation := range []string{"missing", "duplicate", "extra"} {
		t.Run(mutation, func(t *testing.T) {
			t.Parallel()
			inputs, err := mcpcontractcheck.LoadInputs(root)
			if err != nil {
				t.Fatal(err)
			}
			index := -1
			for i, binding := range inputs.Registry {
				if binding.ToolID == "mcp.tool.you.worker_session.read" {
					index = i
				}
			}
			if index < 0 {
				t.Fatal("complete union omitted the Worker Session registry")
			}
			switch mutation {
			case "missing":
				inputs.Registry = append(inputs.Registry[:index], inputs.Registry[index+1:]...)
			case "duplicate":
				inputs.Registry = append(inputs.Registry, inputs.Registry[index])
			case "extra":
				inputs.Registry = append(inputs.Registry, mcpcontractcheck.HandlerBinding{ToolID: "mcp.tool.you.worker_session.extra", HandlerID: "mcp.handler.you.worker_session.extra"})
			}
			diagnostics := mcpcontractcheck.Validate(inputs)
			if len(diagnostics) == 0 || !strings.Contains(diagnosticText(diagnostics), "worker_session.") {
				t.Fatalf("%s binding was accepted: %v", mutation, diagnostics)
			}
		})
	}
}

func TestValidateDetectsResourceAndSkillDescriptorDrift(t *testing.T) {
	inputs := mcpcontractcheck.Inputs{
		Resources:          []mcpcontractcheck.ResourceRecord{{ID: "resource-a", URI: "you://a", Name: "a", Description: "a", MIMEType: "application/json", Handler: "handler-a"}},
		GeneratedResources: []mcpcontractcheck.ResourceRecord{},
		Skills:             []mcpcontractcheck.SkillRecord{{ID: "skill-a", URI: "skill://a/SKILL.md", Frontmatter: map[string]any{"name": "a", "description": "a"}, ResourceURIs: []string{"skill://a/SKILL.md"}}},
		GeneratedSkills:    []mcpcontractcheck.SkillRecord{},
	}
	diagnostics := mcpcontractcheck.Validate(inputs)
	if len(diagnostics) != 2 || diagnostics[0].Code != "mcp.resource.missing" || diagnostics[1].Code != "mcp.skill.missing" {
		t.Fatalf("Validate() diagnostics = %#v, want one resource and one skill missing diagnostic", diagnostics)
	}
}

func TestCheckCleanRepositoryParity(t *testing.T) {
	t.Parallel()

	root := testutil.MustRepoRoot(t)
	first, err := mcpcontractcheck.Check(root)
	if err != nil {
		t.Fatalf("first Check() error = %v", err)
	}
	second, err := mcpcontractcheck.Check(root)
	if err != nil {
		t.Fatalf("second Check() error = %v", err)
	}
	if len(first) != 0 || len(second) != 0 {
		t.Fatalf("Check() diagnostics = first %+v, second %+v; want none", first, second)
	}
}
