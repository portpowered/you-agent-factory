//go:build windows && managed_process_integration

package effects

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestASRLiveCorrelationEvidenceValidatesAndRoundTripsAllowlistedFacts(t *testing.T) {
	t.Parallel()

	evidence := validASRLiveCorrelationEvidence(ASRLiveCorrelationScenarioResponseFirst)
	encoded, err := MarshalASRLiveCorrelationEvidence(evidence)
	if err != nil {
		t.Fatalf("marshal valid evidence: %v", err)
	}
	decoded, err := UnmarshalASRLiveCorrelationEvidence(encoded)
	if err != nil || !reflect.DeepEqual(decoded, evidence) {
		t.Fatalf("evidence round trip = %#v, error = %v; want %#v", decoded, err, evidence)
	}

	failed := validASRLiveCorrelationEvidence(ASRLiveCorrelationScenarioExitFirst)
	failed.RPC.TerminalSequence = 3
	failed.Child.WaitSequence = 4
	failed.Application = ASRLiveCorrelationApplication{
		Outcome: "FAILED", ErrorClasses: []string{"INFERENCE_FAILED", "HOST_LEASE_EXPIRED"},
		OutputCount: 0,
	}
	if err := ValidateASRLiveCorrelationEvidence(failed); err != nil {
		t.Fatalf("valid exit-first evidence rejected: %v", err)
	}
}

func TestASRLiveCorrelationEvidenceRejectsMalformedOwnershipOrderAndRawFacts(t *testing.T) {
	t.Parallel()

	valid := validASRLiveCorrelationEvidence(ASRLiveCorrelationScenarioResponseFirst)
	tests := []struct {
		name   string
		mutate func(*ASRLiveCorrelationEvidence)
	}{
		{name: "raw run identity", mutate: func(e *ASRLiveCorrelationEvidence) { e.RunID = "token=private-sentinel" }},
		{name: "unknown scenario", mutate: func(e *ASRLiveCorrelationEvidence) { e.Scenario = "response_first path=C:\\private\\backend" }},
		{name: "malformed endpoint port", mutate: func(e *ASRLiveCorrelationEvidence) { e.Endpoint.Port = 7437 }},
		{name: "unowned listener pid", mutate: func(e *ASRLiveCorrelationEvidence) { e.Endpoint.ListenerProcessID++ }},
		{name: "endpoint digest mismatch", mutate: func(e *ASRLiveCorrelationEvidence) { e.Endpoint.IdentitySHA256 = strings.Repeat("B", 64) }},
		{name: "missing semantic response digest", mutate: func(e *ASRLiveCorrelationEvidence) { e.RPC.ResponseSemanticSHA256 = "" }},
		{name: "child wait before terminal", mutate: func(e *ASRLiveCorrelationEvidence) { e.Child.WaitSequence = e.RPC.TerminalSequence }},
		{name: "missing exit trigger", mutate: func(e *ASRLiveCorrelationEvidence) { e.Child.ExitTrigger = "" }},
		{name: "invalid exit trigger", mutate: func(e *ASRLiveCorrelationEvidence) { e.Child.ExitTrigger = "BACKEND_EXIT" }},
		{name: "unbounded child error", mutate: func(e *ASRLiveCorrelationEvidence) { e.Child.ExitClass = "backend failure: C:\\private" }},
		{name: "partial failed result", mutate: func(e *ASRLiveCorrelationEvidence) {
			e.Application.Outcome = "FAILED"
			e.Application.ErrorClasses = []string{"INFERENCE_FAILED", "HOST_LEASE_EXPIRED"}
			e.Application.OutputCount = 1
		}},
		{name: "unlisted error class", mutate: func(e *ASRLiveCorrelationEvidence) {
			e.Application.ErrorClasses = []string{"backend says token=secret"}
		}},
		{name: "unclean owned resources", mutate: func(e *ASRLiveCorrelationEvidence) { e.Cleanup.OwnedListenersRemaining = 1 }},
		{name: "redaction not scanned", mutate: func(e *ASRLiveCorrelationEvidence) { e.RedactionPassed = false }},
		{name: "unexpected toolchain path", mutate: func(e *ASRLiveCorrelationEvidence) { e.GoToolchain = "go1.26.8 path=C:\\private" }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			candidate.Application.ErrorClasses = append([]string(nil), valid.Application.ErrorClasses...)
			test.mutate(&candidate)
			if err := ValidateASRLiveCorrelationEvidence(candidate); err == nil {
				t.Fatalf("invalid evidence accepted: %#v", candidate)
			}
			if _, err := MarshalASRLiveCorrelationEvidence(candidate); err == nil {
				t.Fatal("invalid evidence serialized")
			}
		})
	}

	validJSON, err := MarshalASRLiveCorrelationEvidence(valid)
	if err != nil {
		t.Fatal(err)
	}
	withUnknownField := bytes.Replace(validJSON, []byte(`"schema":`), []byte(`"unsafe":"payload marker","schema":`), 1)
	if _, err := UnmarshalASRLiveCorrelationEvidence(withUnknownField); err == nil {
		t.Fatal("unknown raw payload field accepted")
	}
	withTrailingValue := append(append([]byte(nil), validJSON...), []byte(` {"raw":"backend error marker"}`)...)
	if _, err := UnmarshalASRLiveCorrelationEvidence(withTrailingValue); err == nil {
		t.Fatal("trailing raw value accepted")
	}
}

func TestASRLiveCorrelationEvidenceAndSignalsOmitRawSentinels(t *testing.T) {
	t.Parallel()

	const (
		secret  = "token=secret-sentinel"
		path    = `C:\private\backend.exe`
		payload = "private transcript sentinel"
		backend = "unrestricted backend diagnostic sentinel"
	)
	controller, err := NewASRLiveCorrelationController(func(context.Context, string, int) (int, error) {
		return 8123, errors.New(secret + " " + path + " " + backend)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.RecordManagedChildStarted(8123, "grpc://127.0.0.1:49152"); err != nil {
		t.Fatal(err)
	}
	if err := controller.ObserveEndpoint(context.Background(), "grpc://127.0.0.1:49152"); !errors.Is(err, ErrASRLiveCorrelationOwnership) {
		t.Fatalf("failed PID lookup = %v, want generic ownership rejection", err)
	} else if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), path) || strings.Contains(err.Error(), backend) {
		t.Fatalf("PID lookup error leaked raw details: %v", err)
	}
	if err := controller.RecordRequestSemanticSHA256(asrCorrelationTestDigest(payload)); err != nil {
		t.Fatal(err)
	}
	evidence := validASRLiveCorrelationEvidence(ASRLiveCorrelationScenarioResponseFirst)
	encoded, err := MarshalASRLiveCorrelationEvidence(evidence)
	if err != nil {
		t.Fatal(err)
	}
	signals, err := jsonMarshalASRLiveCorrelationSignals(controller.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{secret, path, payload, backend, "grpc://127.0.0.1:49152"} {
		if bytes.Contains(encoded, []byte(raw)) || bytes.Contains(signals, []byte(raw)) {
			t.Fatalf("redacted evidence contains %q: evidence=%s signals=%s", raw, encoded, signals)
		}
	}
}

func validASRLiveCorrelationEvidence(scenario string) ASRLiveCorrelationEvidence {
	digest := strings.Repeat("a", 64)
	endpoint, _ := ASRLiveCorrelationEndpointFromAddress("grpc://127.0.0.1:49152", 8123, 49152)
	return ASRLiveCorrelationEvidence{
		Schema: ASRLiveCorrelationEvidenceSchema, RunID: "cycle-163-asr-live-endpoint-correlation-001",
		Scenario: scenario, SourceCommit: strings.Repeat("1", 40), SourceTree: strings.Repeat("2", 40),
		GoToolchain: "go1.26.8 windows/amd64", ExecutableSHA: digest, WAVSHA: digest,
		ModelSHA: digest, BackendSHA: digest, CacheSHA: digest, Endpoint: endpoint,
		RPC: ASRLiveCorrelationRPC{
			Method: "AUDIO_TRANSCRIPTION", Status: "OK", TerminalSequence: 3,
			ResponseDecoded: true, RequestSemanticSHA256: digest, ResponseSemanticSHA256: digest,
		},
		Child: ASRLiveCorrelationChild{
			ProcessID: 8123, WaitSequence: 5, ExitTrigger: ASRLiveCorrelationExitHarnessRequested,
			ExitClass: "NONZERO_EXIT", ExitCodeKnown: true, ExitCode: 1,
		},
		Application:     ASRLiveCorrelationApplication{Outcome: "COMPLETED", OutputCount: 2},
		RedactionPassed: true,
	}
}

func jsonMarshalASRLiveCorrelationSignals(events []ASRLiveCorrelationEvent) ([]byte, error) {
	return json.Marshal(events)
}
