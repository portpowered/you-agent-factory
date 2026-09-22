//go:build windows && managed_process_integration

package asrlivecorrelation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

func awaitASRLiveCorrelationSignal(
	t *testing.T,
	ctx context.Context,
	controller *modelseffects.ASRLiveCorrelationController,
	kind modelseffects.ASRLiveCorrelationEventKind,
) modelseffects.ASRLiveCorrelationEvent {
	t.Helper()
	event, err := controller.WaitForSignal(ctx, kind)
	if err != nil {
		t.Fatalf("wait for ASR live-correlation signal %s: %v", kind, err)
	}
	return event
}

func awaitASRLiveCorrelationInvocation(
	t *testing.T,
	ctx context.Context,
	invocations <-chan asrLiveCorrelationInvocation,
) asrLiveCorrelationInvocation {
	t.Helper()
	select {
	case invocation := <-invocations:
		return invocation
	case <-ctx.Done():
		t.Fatalf("ASR invocation did not finish before its bounded ceiling: %v", ctx.Err())
		return asrLiveCorrelationInvocation{}
	}
}

func assertASRLiveCorrelationResponseFirst(
	t *testing.T,
	invocation asrLiveCorrelationInvocation,
) string {
	t.Helper()
	if invocation.err != nil || invocation.result.Status != models.ModelInvocationStatusCompleted ||
		invocation.result.LeaseDisposition != models.InvocationLeaseReleased || len(invocation.result.Outputs) != 2 {
		t.Fatalf("response-first Models result = %#v error=%v, want completed two-output result", invocation.result, invocation.err)
	}
	transcript, segments := readASRLiveCorrelationSemanticOutputs(t, invocation)
	if transcript == "" || len(segments) == 0 {
		t.Fatalf("response-first ASR semantic output is empty: transcriptBytes=%d segments=%d", len(transcript), len(segments))
	}
	assertASRLiveCorrelationSegmentsOrdered(t, segments)
	return transcript
}

func readASRLiveCorrelationSemanticOutputs(
	t *testing.T,
	invocation asrLiveCorrelationInvocation,
) (string, []models.ASRBackendSegment) {
	t.Helper()
	var transcript string
	var segments []models.ASRBackendSegment
	for _, content := range invocation.result.Content {
		switch content.Name {
		case "transcript":
			transcript = strings.TrimSpace(content.Content)
		case "segments":
			if err := json.Unmarshal([]byte(content.Content), &segments); err != nil {
				t.Fatalf("decode normalized ASR segments: %v", err)
			}
		}
	}
	return transcript, segments
}

func assertASRLiveCorrelationSegmentsOrdered(t *testing.T, segments []models.ASRBackendSegment) {
	t.Helper()
	previousStart, previousEnd := int64(-1), int64(-1)
	for index, segment := range segments {
		if invalidASRLiveCorrelationSegment(index, segment, previousStart, previousEnd) {
			t.Fatalf("ASR segment[%d] is not finite, nonnegative and ordered: %#v", index, segment)
		}
		previousStart, previousEnd = segment.Start, segment.End
	}
}

func invalidASRLiveCorrelationSegment(index int, segment models.ASRBackendSegment, previousStart, previousEnd int64) bool {
	return segment.Start < 0 || segment.End < segment.Start ||
		index > 0 && (segment.Start < previousStart || segment.Start < previousEnd)
}

func assertASRLiveCorrelationResponseFirstFailure(
	t *testing.T,
	invocation asrLiveCorrelationInvocation,
) {
	t.Helper()
	if invocation.err == nil || !errors.Is(invocation.err, models.ErrInferenceFailed) ||
		!errors.Is(invocation.err, models.ErrHostLeaseExpired) ||
		invocation.result.Status != models.ModelInvocationStatusFailed ||
		invocation.result.LeaseDisposition != models.InvocationLeaseExpired ||
		len(invocation.result.Content) != 0 || len(invocation.result.Outputs) != 0 {
		t.Fatalf("exit-first Models result = %#v error=%v, want typed lease failure with zero outputs", invocation.result, invocation.err)
	}
}

func stopASRLiveCorrelationScope(
	t *testing.T,
	ctx context.Context,
	service models.Service,
	scope models.RuntimeScopeRef,
) {
	t.Helper()
	if _, err := service.StopModelHost(ctx, models.StopModelHostRequest{Scope: scope, Name: models.BuiltInModelNameASR}); err != nil &&
		!errors.Is(err, models.ErrHostRuntimeNotReady) {
		t.Fatalf("stop ASR Runtime Host: %v", err)
	}
	if _, err := service.CloseRuntimeScope(ctx, models.CloseRuntimeScopeRequest{Scope: scope}); err != nil {
		t.Fatalf("close ASR Runtime Scope: %v", err)
	}
}

func assertASRLiveCorrelationCleanup(
	t *testing.T,
	manifest asrLiveCorrelationHarnessManifest,
	port int,
) {
	t.Helper()
	entries, err := os.ReadDir(manifest.StagingRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("ASR staged audio cleanup entries=%v err=%v", entries, err)
	}
	_, err = modelseffects.WindowsASRLiveCorrelationListenerPIDLookup(context.Background(), "127.0.0.1", port)
	if !errors.Is(err, modelseffects.ErrASRLiveCorrelationListenerAbsent) {
		t.Fatalf("owned ASR listener remains or could not be classified: %v", err)
	}
}

func (sink *asrLiveCorrelationRuntimeEvidenceSink) RecordRuntimeEvidence(record modelseffects.RuntimeEvidenceRecord) {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	copyRecord := record
	if record.ASRProtocol != nil {
		protocol := *record.ASRProtocol
		copyRecord.ASRProtocol = &protocol
	}
	sink.records = append(sink.records, copyRecord)
}

func (sink *asrLiveCorrelationRuntimeEvidenceSink) snapshot() []modelseffects.RuntimeEvidenceRecord {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return append([]modelseffects.RuntimeEvidenceRecord(nil), sink.records...)
}

func asrLiveCorrelationProtocolEvidence(
	t *testing.T,
	records []modelseffects.RuntimeEvidenceRecord,
) modelseffects.RuntimeASRProtocolObservation {
	t.Helper()
	for _, record := range records {
		if record.Kind == modelseffects.RuntimeEvidenceKindASRProtocol && record.ASRProtocol != nil {
			return *record.ASRProtocol
		}
	}
	t.Fatal("Models runtime did not emit the private ASR protocol observation")
	return modelseffects.RuntimeASRProtocolObservation{}
}

func writeASRLiveCorrelationEvidence(
	t *testing.T,
	manifest asrLiveCorrelationHarnessManifest,
	fixture asrLiveCorrelationFixture,
	endpoint string,
	events []modelseffects.ASRLiveCorrelationEvent,
	protocol modelseffects.RuntimeASRProtocolObservation,
	invocation asrLiveCorrelationInvocation,
	transcript string,
) {
	t.Helper()
	endpointEvent := findASRLiveCorrelationEvent(t, events, modelseffects.ASRLiveCorrelationEndpointObserved)
	terminalEvent := findASRLiveCorrelationEvent(t, events, modelseffects.ASRLiveCorrelationRPCTerminal)
	childEvent := findASRLiveCorrelationEvent(t, events, modelseffects.ASRLiveCorrelationChildWaited)
	application := modelseffects.ASRLiveCorrelationApplication{
		Outcome: "COMPLETED", OutputCount: len(invocation.result.Outputs),
	}
	if invocation.err != nil {
		application.Outcome = "FAILED"
		application.ErrorClasses = []string{"INFERENCE_FAILED", "HOST_LEASE_EXPIRED"}
		application.OutputCount = len(invocation.result.Outputs)
	}
	evidence := modelseffects.ASRLiveCorrelationEvidence{
		Schema: modelseffects.ASRLiveCorrelationEvidenceSchema,
		RunID:  manifest.RunID, Scenario: manifest.Scenario,
		SourceCommit: manifest.SourceCommit, SourceTree: manifest.SourceTree,
		GoToolchain:   runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH,
		ExecutableSHA: manifest.ExecutableSHA256, WAVSHA: manifest.WAVSHA256,
		ModelSHA: protocol.ModelIdentitySHA256, BackendSHA: protocol.BackendArtifactSHA256,
		CacheSHA: manifest.CacheManifestSHA256, Endpoint: endpointEvent.Endpoint,
		RPC: modelseffects.ASRLiveCorrelationRPC{
			Method: modelseffects.RuntimeASRPCMethodAudioTranscription, Status: "OK",
			TerminalSequence: terminalEvent.Sequence, ResponseDecoded: protocol.ResponseDecoded,
			RequestSemanticSHA256:  terminalEvent.RequestSemanticSHA256,
			ResponseSemanticSHA256: terminalEvent.ResponseSemanticSHA256,
		},
		Child: modelseffects.ASRLiveCorrelationChild{
			ProcessID: childEvent.ProcessID, WaitSequence: childEvent.Sequence,
			ExitTrigger: childEvent.ExitTrigger,
			ExitClass:   childEvent.ExitClass, ExitCodeKnown: childEvent.ExitCodeKnown,
			ExitCode: childEvent.ExitCode,
		},
		Application: application, RedactionPassed: true,
	}
	if err := modelseffects.ValidateASRLiveCorrelationEvidence(evidence); err != nil {
		t.Fatalf("live ASR evidence does not meet the additive schema: %v", err)
	}
	encoded, err := modelseffects.MarshalASRLiveCorrelationEvidence(evidence)
	if err != nil {
		t.Fatalf("serialize redacted live ASR evidence: %v", err)
	}
	endpointAddress := endpoint
	privateValues := [][]byte{
		[]byte(manifest.SourceRoot), []byte(manifest.ExecutablePath), []byte(manifest.WAVPath),
		[]byte(manifest.CacheRoot), []byte(manifest.CacheManifestPath), []byte(manifest.StateRoot),
		[]byte(manifest.StagingRoot), []byte(manifest.EvidenceOutput), []byte(endpointAddress),
		fixture.audio, []byte(transcript),
	}
	if invocation.err != nil {
		privateValues = append(privateValues, []byte(invocation.err.Error()))
	}
	for _, privateValue := range privateValues {
		if len(privateValue) != 0 && bytes.Contains(encoded, privateValue) {
			t.Fatalf("serialized ASR evidence contains a raw private value")
		}
	}
	if err := writeASRLiveCorrelationFileExclusive(manifest.EvidenceOutput, encoded); err != nil {
		t.Fatalf("write exclusive ASR evidence output: %v", err)
	}
	t.Logf("ASR_LIVE_CORRELATION scenario=%s request_sha256=%s response_sha256=%s endpoint_port=%d listener_pid=%d output_count=%d", manifest.Scenario, terminalEvent.RequestSemanticSHA256, terminalEvent.ResponseSemanticSHA256, endpointEvent.Endpoint.Port, endpointEvent.ProcessID, application.OutputCount)
}

func findASRLiveCorrelationEvent(
	t *testing.T,
	events []modelseffects.ASRLiveCorrelationEvent,
	kind modelseffects.ASRLiveCorrelationEventKind,
) modelseffects.ASRLiveCorrelationEvent {
	t.Helper()
	for _, event := range events {
		if event.Kind == kind {
			return event
		}
	}
	t.Fatalf("ASR live-correlation event %s is missing", kind)
	return modelseffects.ASRLiveCorrelationEvent{}
}

func writeASRLiveCorrelationFileExclusive(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("could not prepare private evidence directory")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("could not create a fresh private evidence file")
	}
	if _, err := file.Write(append(contents, '\n')); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return errors.New("could not write private evidence")
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return errors.New("could not sync private evidence")
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return errors.New("could not close private evidence")
	}
	return nil
}
