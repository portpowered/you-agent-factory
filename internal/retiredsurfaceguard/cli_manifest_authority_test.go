package retiredsurfaceguard_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/retiredsurfaceguard"
)

func TestScanCLIManifestAuthoritySourceViolations_PassesCanonicalTransportBoundaries(t *testing.T) {
	root := writeCLIManifestAuthorityFixture(t, map[string]string{
		"pkg/transports/cli/root_work.go": `package cli
func executeWork(inputID string) string { return inputID }
`,
	})

	violations, err := retiredsurfaceguard.ScanCLIManifestAuthoritySourceViolations(root)
	if err != nil {
		t.Fatalf("ScanCLIManifestAuthoritySourceViolations() error = %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
}

func TestScanCLIManifestAuthoritySourceViolations_RejectsSecondaryPublicCLIShape(t *testing.T) {
	root := writeCLIManifestAuthorityFixture(t, map[string]string{
		"pkg/transports/cli/root_submit_batch.go": `package cli
import "github.com/spf13/cobra"
type SessionFamilyBindings struct { FlagUsages map[string]string }
func newSubmitCommand() *cobra.Command {
	var name string
	cmd := &cobra.Command{Use: "submit", Short: "secondary help"}
	cmd.Flags().StringVar(&name, "name", "", "secondary usage")
	return cmd
}
`,
	})

	violations, err := retiredsurfaceguard.ScanCLIManifestAuthoritySourceViolations(root)
	if err != nil {
		t.Fatalf("ScanCLIManifestAuthoritySourceViolations() error = %v", err)
	}
	output := formatViolations(violations)
	for _, want := range []string{
		"remove handwritten CLI-shape function newSubmitCommand",
		"remove CLI-shape mirror SessionFamilyBindings",
		"remove handwritten cobra.Command metadata",
		"remove direct Cobra/pflag public input registration",
		"contracts/cli/commands.json",
		"stable input ID",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("violations = %q, want actionable diagnostic containing %q", output, want)
		}
	}
}

func writeCLIManifestAuthorityFixture(t *testing.T, overrides map[string]string) string {
	t.Helper()
	root := t.TempDir()
	paths := []string{
		"pkg/transports/cli/root_work.go",
		"pkg/transports/cli/root_workflow.go",
		"pkg/transports/cli/root_submit_batch.go",
		"pkg/transports/cli/root_factory.go",
		"pkg/transports/cli/commandregistry/representative_handlers.go",
		"pkg/transports/cli/climanifestcobra/run_submit_constructor.go",
	}
	for _, relative := range paths {
		content := "package cli\n"
		if override, ok := overrides[relative]; ok {
			content = override
		}
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", relative, err)
		}
	}
	return root
}
