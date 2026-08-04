package inputcontract

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// TestNewServeCommandProjectsAcpLeafFromRepresentativeManifest proves the
// independently injected "you serve" family constructor projects the real
// generated manifest into exactly "you serve acp" bound to the injected
// handler, mirroring TestNewCommandTreeAcceptsRepresentativeGeneratedFlagRecords's
// use of the production generated manifest above.
func TestNewServeCommandProjectsAcpLeafFromRepresentativeManifest(t *testing.T) {
	serve, err := climanifestcobra.NewServeCommand(noopResolvedServeHandler)
	if err != nil {
		t.Fatalf("NewServeCommand() error = %v", err)
	}
	if serve.Name() != "serve" || serve.Runnable() {
		t.Fatalf("serve command = (name %q, runnable %t), want (serve, non-runnable)", serve.Name(), serve.Runnable())
	}
	acp, _, err := serve.Find([]string{"acp"})
	if err != nil {
		t.Fatalf("Find(acp) error = %v", err)
	}
	if !acp.Runnable() || acp.RunE == nil {
		t.Fatal("you serve acp must be a runnable command bound to the injected handler")
	}
}

// TestNewServeCommandRejectsMissingHandler proves the constructor refuses to
// build a leaf with no dispatch target, matching every other generic
// constructor's guard against handler-less runnable commands exercised
// above.
func TestNewServeCommandRejectsMissingHandler(t *testing.T) {
	if _, err := climanifestcobra.NewServeCommand(nil); err == nil {
		t.Fatal("NewServeCommand(nil) error = nil, want handler-required rejection")
	}
}

// TestNewServeCommandFromManifestRejectsCorruptedServeFamilyManifest proves
// the constructor's lookups and downstream NewCommandTree validation reject
// a corrupted upstream serve family manifest before ever returning a tree,
// the same manifest-corruption contract TestNewCommandTreeRejectsInvalidManifestBeforeReturningTree
// and TestNewCommandTreeRejectsInvalidHandlerBindingsBeforeExecution establish
// for the generic constructor above.
func TestNewServeCommandFromManifestRejectsCorruptedServeFamilyManifest(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*climanifest.Manifest)
	}{
		{
			name: "missing root record",
			mutate: func(manifest *climanifest.Manifest) {
				delete(manifest.Commands, "you")
			},
		},
		{
			name: "missing parent record",
			mutate: func(manifest *climanifest.Manifest) {
				delete(manifest.Commands, "you.serve")
			},
		},
		{
			name: "missing acp record",
			mutate: func(manifest *climanifest.Manifest) {
				delete(manifest.Commands, "you.serve.acp")
			},
		},
		{
			name: "out-of-family parent record",
			mutate: func(manifest *climanifest.Manifest) {
				record := manifest.Commands["you.serve"]
				record.ID = "you.mcp"
				manifest.Commands["you.serve"] = record
			},
		},
		{
			name: "out-of-family acp record",
			mutate: func(manifest *climanifest.Manifest) {
				record := manifest.Commands["you.serve.acp"]
				record.ID = "you.mcp.serve"
				manifest.Commands["you.serve.acp"] = record
			},
		},
		{
			name: "unsupported manifest format version",
			mutate: func(manifest *climanifest.Manifest) {
				manifest.FormatVersion = ""
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := mustServeFamilyManifestWithRoot(t)
			test.mutate(&manifest)
			if _, err := climanifestcobra.NewServeCommandFromManifest(manifest, noopResolvedServeHandler); err == nil {
				t.Fatalf("NewServeCommandFromManifest() error = nil, want rejection for %s", test.name)
			}
		})
	}
}

func noopResolvedServeHandler(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}

func mustServeFamilyManifestWithRoot(t *testing.T) climanifest.Manifest {
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
