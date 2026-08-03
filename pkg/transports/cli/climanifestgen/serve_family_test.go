package climanifestgen_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

func TestServeFamilyContainsOnlyCanonicalCommands(t *testing.T) {
	for _, id := range climanifestgen.ServeFamilyCommandIDs {
		if err := climanifestgen.AssertServeFamilyCommandID(id); err != nil {
			t.Fatalf("canonical serve command %q rejected: %v", id, err)
		}
	}
	if err := climanifestgen.AssertServeFamilyCommandID("you.mcp.serve"); err == nil {
		t.Fatal("MCP serve command must not be classified as canonical serve")
	}
}
