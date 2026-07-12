package cli

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// Hermetic S02 failure-baseline fixtures for root CLI one-shot model invocation
// when no factory API server is listening on the configured loopback endpoint.
// Packaged @you/goal one-shot runs embed their own API server, so the no-server
// transport contract is locked through models client commands that share the
// same /models HTTP surface goal inference depends on.

const goalFailureBaselineUnreachableServer = "http://127.0.0.1:1"

func TestFailureBaseline_NoServer_ModelsInvokeCommandReportsUnreachableEndpoint(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS",
		"--text", "hello world",
		"--output", outputPath,
		"--server", goalFailureBaselineUnreachableServer,
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models/OMNIVOICE_Q4_K_M/invocations"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestFailureBaseline_NoServer_ModelsListCommandReportsUnreachableEndpoint(t *testing.T) {
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"models", "list",
		"--server", goalFailureBaselineUnreachableServer,
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}
