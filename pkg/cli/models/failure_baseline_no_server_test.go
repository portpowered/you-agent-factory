package models

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// Hermetic S02 failure-baseline fixtures for one-shot model invocation when no
// factory API server is reachable on the configured loopback endpoint.

const failureBaselineUnreachableServer = "http://127.0.0.1:1"

func TestFailureBaseline_NoServer_ModelsInvokeJSONReportsUnreachableEndpoint(t *testing.T) {
	var out bytes.Buffer
	err := Invoke(InvokeConfig{
		ModelName: "OMNIVOICE_Q4_K_M",
		Operation: "TTS",
		Text:      "hello world",
		Server:    failureBaselineUnreachableServer,
		JSON:      true,
		Output:    &out,
	})
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models/OMNIVOICE_Q4_K_M/invocations"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on transport failure", out.String())
	}
}

func TestFailureBaseline_NoServer_ModelsListReportsUnreachableEndpoint(t *testing.T) {
	err := List(ListConfig{
		Server: failureBaselineUnreachableServer,
		Output: io.Discard,
	})
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestFailureBaseline_NoServer_ModelsInspectReportsUnreachableEndpoint(t *testing.T) {
	err := Inspect(InspectConfig{
		ModelName: "OMNIVOICE_Q4_K_M",
		Server:    failureBaselineUnreachableServer,
		Output:    io.Discard,
	})
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models/OMNIVOICE_Q4_K_M"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}
