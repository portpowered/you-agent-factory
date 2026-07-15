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
	}

	if diagnostics := mcpcontractcheck.Validate(inputs); len(diagnostics) != 0 {
		t.Fatalf("Validate() diagnostics = %+v, want none", diagnostics)
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
