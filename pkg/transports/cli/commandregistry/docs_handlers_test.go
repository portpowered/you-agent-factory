package commandregistry_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/spf13/cobra"
)

func TestDocsRunEPrintsPackagedIndexWithoutTopic(t *testing.T) {
	runE := commandregistry.DocsRunE(commandregistry.DocsBinding{BinaryName: "you"})
	cmd := &cobra.Command{Use: "docs"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if !strings.Contains(out.String(), "you docs") {
		t.Fatalf("stdout = %q, want packaged docs index guidance", out.String())
	}
}

func TestDocsRunEDefaultsBinaryNameWhenUnset(t *testing.T) {
	runE := commandregistry.DocsRunE(commandregistry.DocsBinding{})
	cmd := &cobra.Command{Use: "docs"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if !strings.Contains(out.String(), "you docs") {
		t.Fatalf("stdout = %q, want default binary name in index", out.String())
	}
}

func TestDocsRunEPrintsTopicMarkdownAndDiagnostics(t *testing.T) {
	var diagnostic bytes.Buffer
	runE := commandregistry.DocsRunE(commandregistry.DocsBinding{
		DiagnosticsWriter: func(*cobra.Command) io.Writer { return &diagnostic },
		Verbose:           func() bool { return true },
	})
	cmd := &cobra.Command{Use: "docs"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runE(cmd, []string{"models"}); err != nil {
		t.Fatalf("RunE(models) error = %v", err)
	}
	if !strings.Contains(out.String(), "you models") {
		t.Fatalf("stdout = %q, want packaged models topic markdown", out.String())
	}
	if !strings.Contains(diagnostic.String(), "docs request topic=models") {
		t.Fatalf("diagnostics = %q, want topic request log", diagnostic.String())
	}
}

func TestDocsRunEPropagatesUnsupportedTopicError(t *testing.T) {
	runE := commandregistry.DocsRunE(commandregistry.DocsBinding{
		DiagnosticsWriter: func(*cobra.Command) io.Writer { return io.Discard },
		Verbose:           func() bool { return false },
	})
	cmd := &cobra.Command{Use: "docs"}
	cmd.SetOut(io.Discard)
	if err := runE(cmd, []string{"not-a-real-topic"}); err == nil {
		t.Fatal("RunE(unsupported topic) error = nil, want failure")
	}
}
