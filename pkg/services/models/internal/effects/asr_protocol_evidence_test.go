package effects

import (
	"context"
	"testing"
)

func TestRuntimeASRProtocolEvidenceAcceptsOnlyAllowlistedFacts(t *testing.T) {
	t.Parallel()

	valid := validASRProtocolEvidence()
	if normalized, ok := normalizeRuntimeEvidenceRecord(valid); !ok ||
		normalized.ASRProtocol == nil || normalized.ASRProtocol.RPCStatus != "OK" {
		t.Fatalf("valid ASR protocol evidence = %#v, accepted %t", normalized, ok)
	}
	tests := []struct {
		name   string
		mutate func(*RuntimeEvidenceRecord)
	}{
		{
			name: "unlisted phase",
			mutate: func(record *RuntimeEvidenceRecord) {
				record.ASRProtocol.Phase = "PATH=C:\\private\\backend"
			},
		},
		{
			name: "unlisted status",
			mutate: func(record *RuntimeEvidenceRecord) {
				record.ASRProtocol.RPCStatus = "Unavailable token=secret"
			},
		},
		{
			name: "invalid digest",
			mutate: func(record *RuntimeEvidenceRecord) {
				record.ASRProtocol.RequestSHA256 = "raw-path=C:\\private\\backend"
			},
		},
		{
			name: "raw top-level cause",
			mutate: func(record *RuntimeEvidenceRecord) {
				record.CauseMessage = "raw backend output"
			},
		},
		{
			name: "ASR facts on a standard record",
			mutate: func(record *RuntimeEvidenceRecord) {
				record.Kind = RuntimeEvidenceKindStage
			},
		},
		{
			name: "destination match without comparison",
			mutate: func(record *RuntimeEvidenceRecord) {
				record.ASRProtocol.DestinationCompared = false
			},
		},
		{
			name: "response without OK status",
			mutate: func(record *RuntimeEvidenceRecord) {
				record.ASRProtocol.RPCStatus = "UNAVAILABLE"
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := cloneASRProtocolEvidence(valid)
			test.mutate(&candidate)
			if normalized, ok := normalizeRuntimeEvidenceRecord(candidate); ok {
				t.Fatalf("unsafe ASR protocol evidence accepted: %#v", normalized)
			}
		})
	}
}

func TestRuntimeObservationContextClonesResolvedConfiguration(t *testing.T) {
	t.Parallel()

	sink := &runtimeEvidenceRecords{}
	configuration := ResolvedHostConfiguration{
		ModelName: "asr", ModelFiles: []string{"model-a.bin"},
		BackendFiles: []string{"backend-a.exe"},
	}
	ctx := WithRuntimeObservation(context.Background(), sink, configuration)
	configuration.ModelFiles[0] = "caller-mutated.bin"
	recorder, observed, ok := RuntimeObservationFromContext(ctx)
	if !ok || recorder == nil || observed.ModelFiles[0] != "model-a.bin" ||
		observed.BackendFiles[0] != "backend-a.exe" {
		t.Fatalf("runtime observation = recorder:%T config:%#v present:%t", recorder, observed, ok)
	}
	observed.ModelFiles[0] = "observer-mutated.bin"
	_, observedAgain, ok := RuntimeObservationFromContext(ctx)
	if !ok || observedAgain.ModelFiles[0] != "model-a.bin" {
		t.Fatalf("runtime observation config after caller mutation = %#v, want detached files", observedAgain)
	}
}

func validASRProtocolEvidence() RuntimeEvidenceRecord {
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	return RuntimeEvidenceRecord{
		Kind: RuntimeEvidenceKindASRProtocol, Stage: RuntimeStageInvoke,
		Outcome: RuntimeEvidenceOutcomeCompleted,
		ASRProtocol: &RuntimeASRProtocolObservation{
			Phase: RuntimeASRPhaseComplete, SelectedBackend: "LOCALAI_WHISPER",
			RPCMethod:        RuntimeASRPCMethodAudioTranscription,
			SelectedPlatform: "WINDOWS_AMD64", ModelIdentitySHA256: digest,
			ModelFileCount: 1, ModelFileNamesSHA256: digest,
			BackendArtifactSHA256: digest, BackendArtifactBytes: 10,
			BackendFileCount: 1, BackendFileNamesSHA256: digest,
			StagedPathSHA256: digest, AudioBytes: 4, AudioSHA256: digest,
			RequestBytes: 8, RequestSHA256: digest, RequestSemanticSHA256: digest,
			RequestDestinationSHA256: digest, DestinationCompared: true,
			DestinationMatchesStaged: true, RPCStatus: "OK",
			ResponseReceived: true, ResponseBytes: 5, ResponseSHA256: digest,
			ResponseDecoded: true, TranscriptTextBytes: 3, SegmentCount: 1,
		},
	}
}

func cloneASRProtocolEvidence(record RuntimeEvidenceRecord) RuntimeEvidenceRecord {
	cloned := record
	if record.ASRProtocol != nil {
		observation := *record.ASRProtocol
		cloned.ASRProtocol = &observation
	}
	return cloned
}
