package commandregistry_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestDocsResolvedRunEPrintsPackagedIndexWithoutTopic(t *testing.T) {
	runE := commandregistry.DocsResolvedRunE(commandregistry.DocsBinding{BinaryName: "you"})
	cmd := &cobra.Command{Use: "docs"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runE(cmd, resolvedDocsInputs(t, ""), resolvedinput.Inputs{}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if !strings.Contains(out.String(), "you docs") {
		t.Fatalf("stdout = %q, want packaged docs index guidance", out.String())
	}
}

func TestDocsResolvedRunEDefaultsBinaryNameWhenUnset(t *testing.T) {
	runE := commandregistry.DocsResolvedRunE(commandregistry.DocsBinding{})
	cmd := &cobra.Command{Use: "docs"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runE(cmd, resolvedDocsInputs(t, ""), resolvedinput.Inputs{}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if !strings.Contains(out.String(), "you docs") {
		t.Fatalf("stdout = %q, want default binary name in index", out.String())
	}
}

func TestDocsResolvedRunEPrintsTopicMarkdownAndDiagnostics(t *testing.T) {
	var diagnostic bytes.Buffer
	runE := commandregistry.DocsResolvedRunE(commandregistry.DocsBinding{
		DiagnosticsWriter: func(*cobra.Command) io.Writer { return &diagnostic },
		Verbose:           func() bool { return true },
	})
	cmd := &cobra.Command{Use: "docs"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	inputs := resolvedDocsInputs(t, "models")
	state, found := inputs.State("you.docs.arg.0")
	if !found || state.Provenance != resolvedinput.SourcePositionalArgument ||
		!state.Changed || state.Default {
		t.Fatalf("topic state = %#v, %t; want explicit positional provenance", state, found)
	}
	if err := runE(cmd, inputs, resolvedinput.Inputs{}); err != nil {
		t.Fatalf("RunE(models) error = %v", err)
	}
	if !strings.Contains(out.String(), "you models") {
		t.Fatalf("stdout = %q, want packaged models topic markdown", out.String())
	}
	if !strings.Contains(diagnostic.String(), "docs request topic=models") {
		t.Fatalf("diagnostics = %q, want topic request log", diagnostic.String())
	}
}

func TestDocsResolvedRunEPropagatesUnsupportedTopicError(t *testing.T) {
	runE := commandregistry.DocsResolvedRunE(commandregistry.DocsBinding{
		DiagnosticsWriter: func(*cobra.Command) io.Writer { return io.Discard },
		Verbose:           func() bool { return false },
	})
	cmd := &cobra.Command{Use: "docs"}
	cmd.SetOut(io.Discard)
	if err := runE(cmd, resolvedDocsInputs(t, "not-a-real-topic"), resolvedinput.Inputs{}); err == nil {
		t.Fatal("RunE(unsupported topic) error = nil, want failure")
	}
}

func resolvedDocsInputs(t *testing.T, topic string) resolvedinput.Inputs {
	t.Helper()
	definition := resolvedinput.Definition{
		ID: "you.docs.arg.0", Kind: resolvedinput.ValueKindString,
		Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument},
	}
	var candidates []resolvedinput.Candidate
	if topic != "" {
		candidates = append(candidates, resolvedinput.Candidate{
			InputID: definition.ID, Source: resolvedinput.SourcePositionalArgument,
			Value: resolvedinput.StringValue(topic),
		})
	}
	inputs, err := resolvedinput.Resolve([]resolvedinput.Definition{definition}, candidates)
	if err != nil {
		t.Fatalf("resolve docs inputs: %v", err)
	}
	return inputs
}
