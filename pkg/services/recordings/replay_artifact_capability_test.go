package recordings_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

// Construct only the root adapter; nil embedded interfaces reject unexpected
// calls. Owner policy and public composed journeys have separate witnesses.
func replayArtifactAdapter(artifacts recordingswire.ArtifactsExportOwner, replay recordingswire.ReplayOwner) recordings.RecordingReplayArtifacts {
	return recordingswire.NewService(nil, nil, nil, artifacts, replay, nil, nil, platformclock.Real{}, logging.NoopLogger{}, nil, nil, nil, nil, nil).(recordings.RecordingReplayArtifacts)
}

type artifactAdapterOwner struct {
	recordingswire.ArtifactsExportOwner
	request any
	result  any
	err     error
	ctx     context.Context
}
type replayAdapterOwner struct {
	recordingswire.ReplayOwner
	request recordings.LoadReplayRecordingRequest
	result  recordings.LoadReplayRecordingResult
	err     error
}

func (owner *replayAdapterOwner) LoadReplayRecording(request recordings.LoadReplayRecordingRequest) (recordings.LoadReplayRecordingResult, error) {
	owner.request = request
	return owner.result, owner.err
}

func (owner *artifactAdapterOwner) BuildPortableArtifact(request recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error) {
	owner.request = request
	if owner.result == nil {
		return recordings.BuildPortableArtifactResult{}, owner.err
	}
	return owner.result.(recordings.BuildPortableArtifactResult), owner.err
}

func (owner *artifactAdapterOwner) ValidatePortableArtifact(request recordings.ValidatePortableArtifactRequest) (recordings.ValidatePortableArtifactResult, error) {
	owner.request = request
	if owner.result == nil {
		return recordings.ValidatePortableArtifactResult{}, owner.err
	}
	return owner.result.(recordings.ValidatePortableArtifactResult), owner.err
}

func (owner *artifactAdapterOwner) EncodePortableArtifact(request recordings.EncodePortableArtifactRequest) (recordings.EncodePortableArtifactResult, error) {
	owner.request = request
	if owner.result == nil {
		return recordings.EncodePortableArtifactResult{}, owner.err
	}
	return owner.result.(recordings.EncodePortableArtifactResult), owner.err
}

func (owner *artifactAdapterOwner) DecodePortableArtifact(request recordings.DecodePortableArtifactRequest) (recordings.DecodePortableArtifactResult, error) {
	owner.request = request
	if owner.result == nil {
		return recordings.DecodePortableArtifactResult{}, owner.err
	}
	return owner.result.(recordings.DecodePortableArtifactResult), owner.err
}

func (owner *artifactAdapterOwner) SummarizePortableArtifact(request recordings.SummarizePortableArtifactRequest) (recordings.SummarizePortableArtifactResult, error) {
	owner.request = request
	if owner.result == nil {
		return recordings.SummarizePortableArtifactResult{}, owner.err
	}
	return owner.result.(recordings.SummarizePortableArtifactResult), owner.err
}

func (owner *artifactAdapterOwner) ExportPortableArtifact(ctx context.Context, request recordings.ExportPortableArtifactRequest) (recordings.ExportPortableArtifactResult, error) {
	owner.request = request
	owner.ctx = ctx
	if owner.result == nil {
		return recordings.ExportPortableArtifactResult{}, owner.err
	}
	return owner.result.(recordings.ExportPortableArtifactResult), owner.err
}

func (owner *artifactAdapterOwner) ReadPortableArtifact(ctx context.Context, request recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error) {
	owner.request = request
	owner.ctx = ctx
	if owner.result == nil {
		return recordings.ReadPortableArtifactResult{}, owner.err
	}
	return owner.result.(recordings.ReadPortableArtifactResult), owner.err
}

func adapterArtifactFacts() (recordings.PortableArtifact, recordings.ArtifactEnvelope) {
	at := time.Unix(1700000000, 0).UTC()
	native := recordings.PortableArtifact{
		SchemaVersion: recordings.PortableArtifactSchemaV1,
		Summary:       recordings.PortableArtifactSummary{RecordingID: "recording", Reference: "artifact", Scope: recordings.CanonicalEventScope{FactorySessionID: "session"}, State: recordings.RecordingFinalized, EventCount: 1, Available: true, Failures: []recordings.RecordingFailure{{Code: "failed", Message: "detail", RecordedAt: at}}},
		Events:        []recordings.CanonicalEvent{{ID: "event", Sequence: 7, FactoryTick: 9, Scope: recordings.CanonicalEventScope{FactorySessionID: "session"}, Cursor: recordings.CanonicalEventCursor{StreamGenerationID: "generation", Sequence: 7}, RecordedAt: at, Kind: "WORK_REQUEST", Payload: "{}", SourceContext: "source"}},
		Integrity:     recordings.PortableArtifactIntegrity{Algorithm: "sha256", Digest: "digest"},
	}
	public := recordings.ArtifactEnvelope{
		SchemaVersion: recordings.ArtifactSchemaV1,
		Summary:       recordings.ArtifactSummary{RecordingID: "recording", Reference: "artifact", Scope: recordings.ReplayScope{FactorySessionID: "session"}, State: recordings.ArtifactStateFinalized, EventCount: 1, Available: true, Failures: []recordings.ArtifactFailure{{Code: "failed", Message: "detail", RecordedAt: at}}},
		Events:        []recordings.ReplayEvent{{ID: "event", Sequence: 7, FactoryTick: 9, Scope: recordings.ReplayScope{FactorySessionID: "session"}, Cursor: recordings.ReplayEventCursor{StreamGenerationID: "generation", Sequence: 7}, RecordedAt: at, Kind: "WORK_REQUEST", Payload: "{}", SourceContext: "source"}},
		Integrity:     recordings.ArtifactIntegrity{Algorithm: "sha256", Digest: "digest"},
	}
	return native, public
}
func TestRecordingReplayArtifactsMapsRequestsAndResults(t *testing.T) {
	t.Parallel()
	native, public := adapterArtifactFacts()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cases := []struct {
		name                  string
		request, result, want any
		call                  func(recordings.RecordingReplayArtifacts) (any, error)
	}{
		{"Build", recordings.BuildPortableArtifactRequest{RecordingID: "recording"}, recordings.BuildPortableArtifactResult{Artifact: native}, recordings.BuildArtifactResult{Artifact: public}, func(cap recordings.RecordingReplayArtifacts) (any, error) {
			return cap.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: "recording"})
		}},
		{"Validate", recordings.ValidatePortableArtifactRequest{Artifact: native}, recordings.ValidatePortableArtifactResult{Summary: native.Summary}, recordings.ValidateArtifactResult{Summary: public.Summary}, func(cap recordings.RecordingReplayArtifacts) (any, error) {
			return cap.ValidateArtifact(recordings.ValidateArtifactRequest{Artifact: public})
		}},
		{"Encode", recordings.EncodePortableArtifactRequest{Artifact: native}, recordings.EncodePortableArtifactResult{Payload: []byte("encoded")}, recordings.EncodeArtifactResult{Payload: []byte("encoded")}, func(cap recordings.RecordingReplayArtifacts) (any, error) {
			return cap.EncodeArtifact(recordings.EncodeArtifactRequest{Artifact: public})
		}},
		{"Decode", recordings.DecodePortableArtifactRequest{Payload: []byte("encoded")}, recordings.DecodePortableArtifactResult{Artifact: native, IgnoredJSONPaths: []string{"$.future"}}, recordings.DecodeArtifactResult{Artifact: public, IgnoredJSONPaths: []string{"$.future"}}, func(cap recordings.RecordingReplayArtifacts) (any, error) {
			return cap.DecodeArtifact(recordings.DecodeArtifactRequest{Payload: []byte("encoded")})
		}},
		{"Summarize", recordings.SummarizePortableArtifactRequest{Artifact: native}, recordings.SummarizePortableArtifactResult{Summary: native.Summary}, recordings.SummarizeArtifactResult{Summary: public.Summary}, func(cap recordings.RecordingReplayArtifacts) (any, error) {
			return cap.SummarizeArtifact(recordings.SummarizeArtifactRequest{Artifact: public})
		}},
		{"Export", recordings.ExportPortableArtifactRequest{RecordingID: "recording"}, recordings.ExportPortableArtifactResult{Reference: "artifact", Artifact: native}, recordings.ExportArtifactResult{Reference: "artifact", Artifact: public}, func(cap recordings.RecordingReplayArtifacts) (any, error) {
			return cap.ExportArtifact(ctx, recordings.ExportArtifactRequest{RecordingID: "recording"})
		}},
		{"Read", recordings.ReadPortableArtifactRequest{RecordingID: "recording", Reference: "artifact"}, recordings.ReadPortableArtifactResult{Artifact: native, IgnoredJSONPaths: []string{"$.future"}}, recordings.ReadArtifactResult{Artifact: public, IgnoredJSONPaths: []string{"$.future"}}, func(cap recordings.RecordingReplayArtifacts) (any, error) {
			return cap.ReadArtifact(ctx, recordings.ReadArtifactRequest{RecordingID: "recording", Reference: "artifact"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			owner := &artifactAdapterOwner{result: tc.result}
			cap := replayArtifactAdapter(owner, nil)
			got, err := tc.call(cap)
			if err != nil || !reflect.DeepEqual(got, tc.want) || !reflect.DeepEqual(owner.request, tc.request) {
				t.Fatalf("result/request = (%#v,%v)/%#v, want %#v/%#v", got, err, owner.request, tc.want, tc.request)
			}
			if (tc.name == "Export" || tc.name == "Read") && owner.ctx != ctx {
				t.Fatal("context replaced")
			}
			cause := errors.New("owner failed")
			owner.err = cause
			_, err = tc.call(cap)
			var typed *recordings.ReplayArtifactError
			if !errors.Is(err, cause) || !errors.As(err, &typed) {
				t.Fatalf("typed cause lost: %v", err)
			}
		})
	}
}
func TestRecordingReplayArtifactsLoadMapsDetachedFacts(t *testing.T) {
	t.Parallel()
	native, public := adapterArtifactFacts()
	owner := &replayAdapterOwner{result: recordings.LoadReplayRecordingResult{Recording: recordings.ReplayRecordingFacts{RecordingID: "recording", Scope: native.Summary.Scope, Events: native.Events}}}
	cap := replayArtifactAdapter(nil, owner)
	got, err := cap.LoadReplay(recordings.LoadReplayRequest{RecordingID: "recording"})
	want := recordings.LoadReplayResult{Replay: recordings.ReplayFacts{RecordingID: "recording", Scope: public.Summary.Scope, Events: public.Events}}
	if err != nil || !reflect.DeepEqual(got, want) || owner.request.RecordingID != "recording" {
		t.Fatalf("LoadReplay = %#v, %v; request %#v", got, err, owner.request)
	}
	got.Replay.Events[0].ID = "mutated"
	if native.Events[0].ID != "event" {
		t.Fatal("returned event aliases owner facts")
	}
}
func TestRecordingReplayArtifactsTranslatesOwnerClassifications(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cause error
		kind  recordings.ReplayArtifactErrorKind
	}{
		{recordings.ErrReplayRecordingNotFound, recordings.ReplayArtifactErrorNotFound},
		{recordings.ErrReplayRecordingNotFinalized, recordings.ReplayArtifactErrorNotFinalized},
		{recordings.ErrCorruptReplayInput, recordings.ReplayArtifactErrorCorruptInput},
		{recordings.ErrPortableArtifactUnavailable, recordings.ReplayArtifactErrorUnavailable},
		{recordings.ErrUnsupportedPortableArtifactSchema, recordings.ReplayArtifactErrorUnsupportedSchema},
		{recordings.ErrInvalidPortableArtifactIntegrity, recordings.ReplayArtifactErrorInvalidIntegrity},
		{recordings.ErrInvalidPortableArtifactOrder, recordings.ReplayArtifactErrorInvalidOrder},
		{recordings.ErrPortableArtifactExportFailed, recordings.ReplayArtifactErrorExportFailed},
		{recordings.ErrForeignPortableArtifact, recordings.ReplayArtifactErrorForeign},
		{recordings.ErrPortableArtifactCancelled, recordings.ReplayArtifactErrorCancelled},
		{recordings.ErrInvalidPortableArtifact, recordings.ReplayArtifactErrorInvalid},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			t.Parallel()
			owner := &replayAdapterOwner{err: tc.cause}
			_, err := replayArtifactAdapter(nil, owner).LoadReplay(recordings.LoadReplayRequest{RecordingID: "recording"})
			var typed *recordings.ReplayArtifactError
			if !errors.Is(err, tc.cause) || !errors.As(err, &typed) || typed.Kind != tc.kind || typed.Diagnostic.Code == "" || typed.Diagnostic.Area == "" || typed.Diagnostic.Message == "" {
				t.Fatalf("classification/cause lost: %v", err)
			}
		})
	}
}

// narrowReplayArtifactsFake implements only recordings.RecordingReplayArtifacts,
// proving peers can fake the capability without implementing recording
// lifecycle, event streaming, projection query, or runtime execution
// behavior from the broader recordings.Service surface.
type narrowReplayArtifactsFake struct {
	replay   recordings.ReplayFacts
	artifact recordings.ArtifactEnvelope
}

var _ recordings.RecordingReplayArtifacts = (*narrowReplayArtifactsFake)(nil)

func (fake *narrowReplayArtifactsFake) LoadReplay(
	request recordings.LoadReplayRequest,
) (recordings.LoadReplayResult, error) {
	if request.RecordingID != fake.replay.RecordingID {
		return recordings.LoadReplayResult{}, recordings.ErrReplayRecordingNotFound
	}
	return recordings.LoadReplayResult{Replay: fake.replay}, nil
}

func (fake *narrowReplayArtifactsFake) BuildArtifact(
	recordings.BuildArtifactRequest,
) (recordings.BuildArtifactResult, error) {
	return recordings.BuildArtifactResult{Artifact: fake.artifact}, nil
}

func (fake *narrowReplayArtifactsFake) ValidateArtifact(
	recordings.ValidateArtifactRequest,
) (recordings.ValidateArtifactResult, error) {
	return recordings.ValidateArtifactResult{Summary: fake.artifact.Summary}, nil
}

func (fake *narrowReplayArtifactsFake) EncodeArtifact(
	recordings.EncodeArtifactRequest,
) (recordings.EncodeArtifactResult, error) {
	return recordings.EncodeArtifactResult{Payload: []byte("encoded")}, nil
}

func (fake *narrowReplayArtifactsFake) DecodeArtifact(
	recordings.DecodeArtifactRequest,
) (recordings.DecodeArtifactResult, error) {
	return recordings.DecodeArtifactResult{Artifact: fake.artifact}, nil
}

func (fake *narrowReplayArtifactsFake) SummarizeArtifact(
	recordings.SummarizeArtifactRequest,
) (recordings.SummarizeArtifactResult, error) {
	return recordings.SummarizeArtifactResult{Summary: fake.artifact.Summary}, nil
}

func (fake *narrowReplayArtifactsFake) ExportArtifact(
	context.Context, recordings.ExportArtifactRequest,
) (recordings.ExportArtifactResult, error) {
	return recordings.ExportArtifactResult{Reference: fake.artifact.Summary.Reference, Artifact: fake.artifact}, nil
}

func (fake *narrowReplayArtifactsFake) ReadArtifact(
	context.Context, recordings.ReadArtifactRequest,
) (recordings.ReadArtifactResult, error) {
	return recordings.ReadArtifactResult{Artifact: fake.artifact}, nil
}

func TestRecordingReplayArtifacts_NarrowFakeConsumption(t *testing.T) {
	t.Parallel()
	fake := &narrowReplayArtifactsFake{
		replay: recordings.ReplayFacts{
			RecordingID: "narrow-fake",
			Events:      []recordings.ReplayEvent{{ID: "narrow-fake-event"}},
		},
		artifact: recordings.ArtifactEnvelope{
			SchemaVersion: recordings.ArtifactSchemaV1,
			Summary:       recordings.ArtifactSummary{RecordingID: "narrow-fake", EventCount: 1, Available: true},
		},
	}
	var replayArtifacts recordings.RecordingReplayArtifacts = fake

	loaded, err := replayArtifacts.LoadReplay(recordings.LoadReplayRequest{RecordingID: "narrow-fake"})
	if err != nil {
		t.Fatalf("LoadReplay() error = %v", err)
	}
	if len(loaded.Replay.Events) != 1 || loaded.Replay.Events[0].ID != "narrow-fake-event" {
		t.Fatalf("LoadReplay() = %#v", loaded.Replay)
	}

	built, err := replayArtifacts.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: "narrow-fake"})
	if err != nil {
		t.Fatalf("BuildArtifact() error = %v", err)
	}
	exported, err := replayArtifacts.ExportArtifact(context.Background(), recordings.ExportArtifactRequest{
		RecordingID: "narrow-fake",
	})
	if err != nil {
		t.Fatalf("ExportArtifact() error = %v", err)
	}
	if !reflect.DeepEqual(exported.Artifact.Summary, built.Artifact.Summary) {
		t.Fatalf("ExportArtifact() Summary = %#v, want %#v", exported.Artifact.Summary, built.Artifact.Summary)
	}
}

func TestRecordingReplayArtifactsDetachArtifactResultsAndRequests(t *testing.T) {
	t.Parallel()
	native, public := adapterArtifactFacts()
	owner := &artifactAdapterOwner{result: recordings.BuildPortableArtifactResult{Artifact: native}}
	cap := replayArtifactAdapter(owner, nil)
	built, err := cap.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: "recording"})
	if err != nil {
		t.Fatal(err)
	}
	built.Artifact.Events[0].Payload = "mutated"
	built.Artifact.Summary.Failures[0].Code = "mutated"
	if native.Events[0].Payload != "{}" || native.Summary.Failures[0].Code != "failed" {
		t.Fatal("artifact result aliases owner facts")
	}
	owner.result = recordings.ValidatePortableArtifactResult{Summary: native.Summary}
	if _, err := cap.ValidateArtifact(recordings.ValidateArtifactRequest{Artifact: public}); err != nil {
		t.Fatal(err)
	}
	request := owner.request.(recordings.ValidatePortableArtifactRequest)
	request.Artifact.Events[0].Payload = "changed by owner"
	request.Artifact.Summary.Failures[0].Code = "changed by owner"
	if public.Events[0].Payload != "{}" || public.Summary.Failures[0].Code != "failed" {
		t.Fatal("artifact request aliases caller facts")
	}
}

func TestRecordingReplayArtifactsKeepsJoinedCancellationCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("operator stopped export")
	owner := &artifactAdapterOwner{err: errors.Join(recordings.ErrPortableArtifactCancelled, context.Canceled, cause)}
	_, err := replayArtifactAdapter(owner, nil).ExportArtifact(context.Background(), recordings.ExportArtifactRequest{RecordingID: "recording"})
	var typed *recordings.ReplayArtifactError
	if !errors.As(err, &typed) || typed.Kind != recordings.ReplayArtifactErrorCancelled || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("joined cancellation cause lost: %v", err)
	}
}
