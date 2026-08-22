package climanifestcobra_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// TestNewServeCommandBuildsAcpLeafBoundToHandler proves the isolated family
// constructor projects exactly "you server acp" as a runnable leaf bound to
// the injected handler's stable manifest ID, and "you server" stays a
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
	if serve.Name() != "server" {
		t.Fatalf("serve name = %q, want server", serve.Name())
	}
	if serve.Runnable() {
		t.Fatal("detached you server protocol family must remain non-runnable")
	}
	if len(serve.Commands()) != 1 {
		t.Fatalf("you server child count = %d, want exactly 1 (acp)", len(serve.Commands()))
	}

	acp, _, err := serve.Find([]string{"acp"})
	if err != nil {
		t.Fatalf("Find(acp) error = %v", err)
	}
	if !acp.Runnable() {
		t.Fatal("you server acp must be runnable")
	}
	if acp.RunE == nil {
		t.Fatal("you server acp must attach a RunE")
	}
}

func TestNewServeCommandRejectsNilHandler(t *testing.T) {
	if _, err := climanifestcobra.NewServeCommand(nil); err == nil {
		t.Fatal("NewServeCommand(nil) error = nil, want handler-required rejection")
	}
}

// TestNewServeCommandFromManifestRejectsOutOfFamilyAcpRecord proves
// NewServeCommandFromManifest rejects a manifest whose "you.server.acp" entry
// does not carry a canonical serve-family command ID (for example, corrupted
// upstream generation that mapped a foreign command's record under that
// key). NewServeCommandFromManifest rebuilds its working manifest keyed by
// the exact "you.server"/"you.server.acp" identities it looked the records up
// by, not by each record's own (here, mislabeled) ID field, so
// NewCommandTree's own shared map-key/record-id consistency check rejects
// the tree instead of silently projecting a mislabeled command.
func TestNewServeCommandFromManifestRejectsOutOfFamilyAcpRecord(t *testing.T) {
	manifest := mustServeManifestWithRoot(t)
	foreign := manifest.Commands["you.server.acp"]
	foreign.ID = "you.server.mcp"
	manifest.Commands["you.server.acp"] = foreign

	_, err := climanifestcobra.NewServeCommandFromManifest(manifest, noopResolvedHandler)
	if err == nil {
		t.Fatal("NewServeCommandFromManifest() error = nil, want out-of-family acp record rejection")
	}
}

func TestNewServeCommandFromManifestRejectsOutOfFamilyParentRecord(t *testing.T) {
	manifest := mustServeManifestWithRoot(t)
	foreign := manifest.Commands["you.server"]
	foreign.ID = "you.server.mcp"
	manifest.Commands["you.server"] = foreign

	_, err := climanifestcobra.NewServeCommandFromManifest(manifest, noopResolvedHandler)
	if err == nil {
		t.Fatal("NewServeCommandFromManifest() error = nil, want out-of-family parent record rejection")
	}
}

// TestNewServeCommandFromManifestRejectsMissingRootRecord proves the "you"
// root lookup itself surfaces a manifest error instead of panicking or
// silently building a rootless tree when a corrupted upstream manifest
// snapshot omits the root command record entirely.
func TestNewServeCommandFromManifestRejectsMissingRootRecord(t *testing.T) {
	manifest := mustServeManifestWithRoot(t)
	delete(manifest.Commands, "you")

	_, err := climanifestcobra.NewServeCommandFromManifest(manifest, noopResolvedHandler)
	if err == nil {
		t.Fatal("NewServeCommandFromManifest() error = nil, want missing root record rejection")
	}
}

// TestNewServeCommandFromManifestRejectsMissingParentRecord proves the
// "you.server" parent lookup surfaces a manifest error when a corrupted
// upstream manifest snapshot omits the serve family's own parent record.
func TestNewServeCommandFromManifestRejectsMissingParentRecord(t *testing.T) {
	manifest := mustServeManifestWithRoot(t)
	delete(manifest.Commands, "you.server")

	_, err := climanifestcobra.NewServeCommandFromManifest(manifest, noopResolvedHandler)
	if err == nil {
		t.Fatal("NewServeCommandFromManifest() error = nil, want missing parent record rejection")
	}
}

// TestNewServeCommandFromManifestRejectsMissingAcpRecord proves the
// "you.server.acp" leaf lookup surfaces a manifest error when a corrupted
// upstream manifest snapshot omits the acp leaf's own record.
func TestNewServeCommandFromManifestRejectsMissingAcpRecord(t *testing.T) {
	manifest := mustServeManifestWithRoot(t)
	delete(manifest.Commands, "you.server.acp")

	_, err := climanifestcobra.NewServeCommandFromManifest(manifest, noopResolvedHandler)
	if err == nil {
		t.Fatal("NewServeCommandFromManifest() error = nil, want missing acp record rejection")
	}
}

// TestNewServeCommandFromManifestPropagatesCommandTreeConstructionError
// proves a downstream NewCommandTree rejection (here, an unsupported
// manifest format version) surfaces through NewServeCommandFromManifest
// instead of being swallowed, once all three CommandByID lookups above have
// already succeeded.
func TestNewServeCommandFromManifestPropagatesCommandTreeConstructionError(t *testing.T) {
	manifest := mustServeManifestWithRoot(t)
	manifest.FormatVersion = ""

	_, err := climanifestcobra.NewServeCommandFromManifest(manifest, noopResolvedHandler)
	if err == nil {
		t.Fatal("NewServeCommandFromManifest() error = nil, want command tree construction rejection")
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
