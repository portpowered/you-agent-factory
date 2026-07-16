package climanifestparity_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/baseline"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
)

func TestProductionManifestHelpIdentityParity_ModelsFamily(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	root := cli.NewRootCommand()
	for _, commandID := range []string{
		"you.models",
		"you.models.list",
		"you.models.inspect",
		"you.models.invoke",
		"you.models.pull",
	} {
		t.Run(commandID, func(t *testing.T) {
			record, err := manifest.CommandByID(commandID)
			if err != nil {
				t.Fatalf("CommandByID(%q) error = %v", commandID, err)
			}

			cmd, err := climanifestparity.FindCommandByPath(root, record.Path)
			if err != nil {
				t.Fatalf("FindCommandByPath(%q) error = %v", record.Path, err)
			}

			mismatches := climanifestparity.CompareModelsHelpIdentity(manifest, record, root, cmd)
			if len(mismatches) == 0 {
				return
			}
			t.Fatalf("contract vs live models help/identity drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
		})
	}
}

func TestProductionManifestHelpIdentityParity_RootAndSessionShow(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	root := cli.NewRootCommand()
	cases := []string{"you", "you.session.show"}
	for _, commandID := range cases {
		t.Run(commandID, func(t *testing.T) {
			record, err := manifest.CommandByID(commandID)
			if err != nil {
				t.Fatalf("CommandByID(%q) error = %v", commandID, err)
			}

			cmd, err := climanifestparity.FindCommandByPath(root, record.Path)
			if err != nil {
				t.Fatalf("FindCommandByPath(%q) error = %v", record.Path, err)
			}

			helpOutput, err := baseline.CaptureHelpOutput(root, climanifestparity.HelpArgsForPath(record.Path))
			if err != nil {
				t.Fatalf("CaptureHelpOutput(%q) error = %v", record.Path, err)
			}

			mismatches := climanifestparity.CompareHelpIdentity(manifest, record, cmd, helpOutput)
			if len(mismatches) == 0 {
				return
			}
			t.Fatalf("contract vs live help/identity drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
		})
	}
}

func TestProductionManifestHelpIdentityParity_DocsFamily(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	root := cli.NewRootCommand()
	record, err := manifest.CommandByID("you.docs")
	if err != nil {
		t.Fatalf("CommandByID(you.docs) error = %v", err)
	}

	cmd, err := climanifestparity.FindCommandByPath(root, record.Path)
	if err != nil {
		t.Fatalf("FindCommandByPath(%q) error = %v", record.Path, err)
	}

	mismatches := climanifestparity.CompareDocsHelpIdentity(manifest, record, root, cmd)
	if len(mismatches) == 0 {
		return
	}
	t.Fatalf("contract vs live docs help/identity drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
}
