package climanifestgen_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

func TestMCPFamilyContainsOnlyCanonicalCommands(t *testing.T) {
	for _, id := range climanifestgen.MCPFamilyCommandIDs {
		if err := climanifestgen.AssertMCPFamilyCommandID(id); err != nil {
			t.Fatalf("canonical MCP command %q rejected: %v", id, err)
		}
	}
	if err := climanifestgen.AssertMCPFamilyCommandID("you.workflow.validate"); err == nil {
		t.Fatal("removed workflow command must not be classified as canonical MCP")
	}
}
