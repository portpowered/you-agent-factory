package mcpcontractcheck_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/mcpcontractcheck"
	"github.com/portpowered/infinite-you/internal/testutil"
)

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
