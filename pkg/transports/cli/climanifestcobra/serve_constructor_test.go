package climanifestcobra_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// TestNewServeCommandBuildsAcpLeafBoundToHandler proves the isolated family
// constructor projects exactly "you serve acp" as a runnable leaf bound to
// the injected handler's stable manifest ID, and "you serve" itself stays a
// non-runnable parent. Actually invoking the leaf's RunE requires the root
// "you" command's own PersistentPreRunE to resolve root inputs onto the
// command's context first (see bindings.go's "invocation has not resolved
// root inputs" guard); that full dispatch path is exercised end to end by
// root_serve_test.go's TestServeACPCommand_DispatchesToInjectedACPServerWithExactStreamsAndContext
// through the real CommandFactory.NewCommand root, not this isolated
// family-only constructor.
func TestNewServeCommandBuildsAcpLeafBoundToHandler(t *testing.T) {
	serve, err := climanifestcobra.NewServeCommand(noopResolvedHandler)
	if err != nil {
		t.Fatalf("NewServeCommand() error = %v", err)
	}
	if serve.Name() != "serve" {
		t.Fatalf("serve name = %q, want serve", serve.Name())
	}
	if serve.Runnable() {
		t.Fatal("you serve must remain non-runnable")
	}
	if len(serve.Commands()) != 1 {
		t.Fatalf("you serve child count = %d, want exactly 1 (acp)", len(serve.Commands()))
	}

	acp, _, err := serve.Find([]string{"acp"})
	if err != nil {
		t.Fatalf("Find(acp) error = %v", err)
	}
	if !acp.Runnable() {
		t.Fatal("you serve acp must be runnable")
	}
	if acp.RunE == nil {
		t.Fatal("you serve acp must attach a RunE")
	}
}

func TestNewServeCommandRejectsNilHandler(t *testing.T) {
	if _, err := climanifestcobra.NewServeCommand(nil); err == nil {
		t.Fatal("NewServeCommand(nil) error = nil, want handler-required rejection")
	}
}

// TestNewServeCommandFromManifestRejectsOutOfFamilyAcpRecord proves the
// production climanifestgen.AssertServeFamilyCommandID guard wired into
// NewServeCommandFromManifest actually rejects a manifest whose
// "you.serve.acp" entry does not carry a canonical serve-family command ID
// (for example, corrupted upstream generation that mapped a foreign
// command's record under that key). This is the behavioral replacement for
// a prior self-referential test that only checked the canonical ID constant
// list against itself.
func TestNewServeCommandFromManifestRejectsOutOfFamilyAcpRecord(t *testing.T) {
	manifest := mustServeManifestWithRoot(t)
	foreign := manifest.Commands["you.serve.acp"]
	foreign.ID = "you.mcp.serve"
	manifest.Commands["you.serve.acp"] = foreign

	_, err := climanifestcobra.NewServeCommandFromManifest(manifest, noopResolvedHandler)
	if err == nil {
		t.Fatal("NewServeCommandFromManifest() error = nil, want out-of-family acp record rejection")
	}
}

func TestNewServeCommandFromManifestRejectsOutOfFamilyParentRecord(t *testing.T) {
	manifest := mustServeManifestWithRoot(t)
	foreign := manifest.Commands["you.serve"]
	foreign.ID = "you.mcp"
	manifest.Commands["you.serve"] = foreign

	_, err := climanifestcobra.NewServeCommandFromManifest(manifest, noopResolvedHandler)
	if err == nil {
		t.Fatal("NewServeCommandFromManifest() error = nil, want out-of-family parent record rejection")
	}
}

func noopResolvedHandler(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}

// mustServeManifestWithRoot rebuilds the exact merged manifest
// climanifestcobra.NewServeCommand assembles internally (the serve family
// plus the "you" root record), so tests can mutate a copy of it directly
// through the same climanifestcobra.NewServeCommandFromManifest entrypoint.
func mustServeManifestWithRoot(t *testing.T) climanifest.Manifest {
	t.Helper()
	manifest, err := generated.ServeFamilyManifest()
	if err != nil {
		t.Fatalf("ServeFamilyManifest() error = %v", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		t.Fatalf("CommandByID(you) error = %v", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return manifest
}
