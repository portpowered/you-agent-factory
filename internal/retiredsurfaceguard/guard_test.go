package retiredsurfaceguard_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/retiredsurfaceguard"
)

func TestScanCLIReintroductionViolations_PassesOnCanonicalInventory(t *testing.T) {
	inventory := retiredsurfaceguard.CLIInventory{
		Commands: []retiredsurfaceguard.CLICommandRecord{
			{Path: "you factory config validate"},
			{Path: "you factory create"},
			{Path: "you config init"},
		},
	}
	if violations := retiredsurfaceguard.ScanCLIReintroductionViolations(inventory); len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
}

func TestScanCLIReintroductionViolations_FailsOnDeliberateReintroduction(t *testing.T) {
	inventory := retiredsurfaceguard.CLIInventory{
		Commands: []retiredsurfaceguard.CLICommandRecord{
			{Path: "you factory save"},
			{Path: "you config validate", Lifecycle: "deprecated", DeprecatedMessage: "you factory config validate is the canonical replacement"},
		},
	}
	violations := retiredsurfaceguard.ScanCLIReintroductionViolations(inventory)
	if len(violations) == 0 {
		t.Fatal("expected deliberate CLI reintroduction violations")
	}
	output := formatViolations(violations)
	for _, want := range []string{
		"you factory save",
		"you config validate",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("violations = %q, want substring %q", output, want)
		}
	}
}

func TestScanCLIReintroductionViolations_FailsOnAliasReintroduction(t *testing.T) {
	inventory := retiredsurfaceguard.CLIInventory{
		Commands: []retiredsurfaceguard.CLICommandRecord{
			{Path: "you factory config", Aliases: []string{"save"}},
		},
	}
	violations := retiredsurfaceguard.ScanCLIReintroductionViolations(inventory)
	if len(violations) == 0 {
		t.Fatal("expected alias reintroduction violation")
	}
	if got := formatViolations(violations); !strings.Contains(got, "you factory save") {
		t.Fatalf("violations = %q, want retired path you factory save", got)
	}
}

func TestScanDocsReintroductionViolations_PassesOnCanonicalRegistry(t *testing.T) {
	registry := retiredsurfaceguard.DocsRegistry{
		SupportedTopics:   []string{"agents", "config"},
		SupportedCommands: []string{"agents", "config", "batch-work"},
		IndexEntries: []retiredsurfaceguard.DocsTopicEntry{
			{Name: "batch-inputs", Aliases: []string{"batch-work"}},
		},
	}
	if violations := retiredsurfaceguard.ScanDocsReintroductionViolations(registry); len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
}

func TestScanDocsReintroductionViolations_FailsOnDeliberateReintroduction(t *testing.T) {
	registry := retiredsurfaceguard.DocsRegistry{
		SupportedTopics:   []string{"packaged-goal", "agents"},
		SupportedCommands: []string{"packaged-goal", "mcp-hosts"},
		IndexEntries: []retiredsurfaceguard.DocsTopicEntry{
			{Name: "agents"},
			{Name: "mcp", Aliases: []string{"mcp-hosts"}},
		},
	}
	violations := retiredsurfaceguard.ScanDocsReintroductionViolations(registry)
	if len(violations) == 0 {
		t.Fatal("expected deliberate docs reintroduction violations")
	}
	output := formatViolations(violations)
	for _, want := range []string{"packaged-goal", "mcp-hosts"} {
		if !strings.Contains(output, want) {
			t.Fatalf("violations = %q, want substring %q", output, want)
		}
	}
}

func TestScanCLIManifestAuthoritySourceViolations_FailsOnHandwrittenRegistration(t *testing.T) {
	root := t.TempDir()
	writeManifestAuthorityFixture(t, root, "pkg/transports/cli/root_work.go", `package cli

func newRunCommand() {}
`)
	writeManifestAuthorityFixture(t, root, "pkg/transports/cli/climanifestcobra/run_submit_constructor.go", `package climanifestcobra

func bind(name string) {
	switch name {
	case "with-server":
	}
}
`)

	violations, err := retiredsurfaceguard.ScanCLIManifestAuthoritySourceViolations(root)
	if err != nil {
		t.Fatalf("ScanCLIManifestAuthoritySourceViolations: %v", err)
	}
	output := formatViolations(violations)
	for _, want := range []string{"newRunCommand", "with-server"} {
		if !strings.Contains(output, want) {
			t.Fatalf("violations = %q, want substring %q", output, want)
		}
	}
}

func TestScanEncodedPathReintroductionViolations_PassesOnHierarchicalMapper(t *testing.T) {
	mapper := func(factoriesRoot, name string) (string, error) {
		return factoriesRoot + "/@you/goal", nil
	}
	if violations := retiredsurfaceguard.ScanEncodedPathReintroductionViolations(
		mapper,
		t.TempDir(),
		retiredsurfaceguard.SettledScopedNamedFactoryPaths(),
	); len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
}

func TestScanEncodedPathReintroductionViolations_FailsOnEncodedMapper(t *testing.T) {
	mapper := func(factoriesRoot, name string) (string, error) {
		return factoriesRoot + "/@you%2Fgoal", nil
	}
	violations := retiredsurfaceguard.ScanEncodedPathReintroductionViolations(
		mapper,
		t.TempDir(),
		retiredsurfaceguard.SettledScopedNamedFactoryPaths(),
	)
	if len(violations) == 0 {
		t.Fatal("expected encoded mapper violations")
	}
	if got := formatViolations(violations); !strings.Contains(got, "percent-encoded scoped leaf names") {
		t.Fatalf("violations = %q, want percent-encoded mapping failure", got)
	}
}

func writeManifestAuthorityFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", relative, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", relative, err)
	}
}

func formatViolations(violations []retiredsurfaceguard.Violation) string {
	var builder strings.Builder
	for _, violation := range violations {
		builder.WriteString(retiredsurfaceguard.FormatViolation(violation))
		builder.WriteByte('\n')
	}
	return builder.String()
}
