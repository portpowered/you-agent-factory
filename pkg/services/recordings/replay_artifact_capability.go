package recordings

import (
	"context"
	"time"
)

// RecordingReplayArtifacts is a narrow, Recordings-owned capability for peers
// that only need to load a finalized recording's detached canonical replay
// facts and build, validate, encode, decode, summarize, export, and read its
// portable artifact envelope. It deliberately excludes recording lifecycle,
// event streaming, projection query, and runtime execution behavior so peers
// can fake it without implementing the rest of Service.
//
// Every identity, replay-fact, artifact, request, result, diagnostic, and
// typed-error type used by this capability is defined directly in this file
// rather than aliased from recordings/internal/contracts.
type RecordingReplayArtifacts interface {
	// LoadReplay selects one finalized recording's detached canonical replay
	// facts by opaque recording identity.
	LoadReplay(LoadReplayRequest) (LoadReplayResult, error)
	// BuildArtifact builds one portable artifact envelope for a finalized
	// recording selected by opaque identity.
	BuildArtifact(BuildArtifactRequest) (BuildArtifactResult, error)
	// ValidateArtifact validates one detached portable artifact envelope.
	ValidateArtifact(ValidateArtifactRequest) (ValidateArtifactResult, error)
	// EncodeArtifact validates a detached portable artifact envelope and
	// returns its completed portable bytes.
	EncodeArtifact(EncodeArtifactRequest) (EncodeArtifactResult, error)
	// DecodeArtifact decodes and validates one portable artifact payload.
	DecodeArtifact(DecodeArtifactRequest) (DecodeArtifactResult, error)
	// SummarizeArtifact returns a detached summary for one portable artifact
	// envelope.
	SummarizeArtifact(SummarizeArtifactRequest) (SummarizeArtifactResult, error)
	// ExportArtifact closes one finalized recording and atomically publishes
	// its completed portable artifact to the recording's existing public
	// reference.
	ExportArtifact(context.Context, ExportArtifactRequest) (ExportArtifactResult, error)
	// ReadArtifact reads and validates one published portable artifact from
	// its public reference.
	ReadArtifact(context.Context, ReadArtifactRequest) (ReadArtifactResult, error)
}

// ReplayRecordingID is the Recordings-owned identity of one recording,
// published for peers that only consume RecordingReplayArtifacts.
type ReplayRecordingID string

// ReplayScope identifies the Factory Session whose canonical history contains
// a recording's events. An empty FactorySessionID represents factory-wide
// scope.
type ReplayScope struct {
	FactorySessionID string
}

// ReplayEventCursor is a portable position in global canonical order.
// StreamGenerationID distinguishes histories whose numeric sequences may
// overlap.
type ReplayEventCursor struct {
	StreamGenerationID string
	Sequence           int64
}

// ReplayEvent is a detached, Recordings-owned canonical fact. Payload is
// immutable JSON text rather than a shared byte slice, and no
// implementation, datastore, runtime, or transport handle can cross this
// boundary.
type ReplayEvent struct {
	ID            string
	Sequence      int64
	FactoryTick   int
	Scope         ReplayScope
	Cursor        ReplayEventCursor
	RecordedAt    time.Time
	Kind          string
	Payload       string
	SourceContext string
}

// ReplayFacts is a detached selection of one recording's canonical facts.
// Events is an independent slice and contains no decoder, store, or runtime
// handle.
type ReplayFacts struct {
	RecordingID ReplayRecordingID
	Scope       ReplayScope
	Events      []ReplayEvent
}

// LoadReplayRequest selects one finalized recording by its opaque identity.
type LoadReplayRequest struct {
	RecordingID ReplayRecordingID
}

// LoadReplayResult returns detached canonical facts for replay.
type LoadReplayResult struct {
	Replay ReplayFacts
}

// ArtifactReference is an opaque published portable artifact reference. It
// does not grant filesystem authority or expose a writer, transaction, or
// temporary storage location.
type ArtifactReference string

// ArtifactState identifies the observable lifecycle state of the recording a
// portable artifact was built from.
type ArtifactState string

const (
	ArtifactStateActive    ArtifactState = "ACTIVE"
	ArtifactStateFinalized ArtifactState = "FINALIZED"
	ArtifactStateFailed    ArtifactState = "FAILED"
)

// ArtifactFailure is a detached failure fact accumulated by the recording a
// portable artifact was built from.
type ArtifactFailure struct {
	Code       string
	Message    string
	RecordedAt time.Time
}

// ArtifactIntegrity contains the completed artifact digest. Digest is
// computed over the artifact with this Digest field empty.
type ArtifactIntegrity struct {
	Algorithm string
	Digest    string
}

// ArtifactSummary contains the detached facts a receiver needs to inspect a
// portable artifact without Recordings storage access.
type ArtifactSummary struct {
	RecordingID ReplayRecordingID
	Reference   ArtifactReference
	Scope       ReplayScope
	State       ArtifactState
	EventCount  int
	FirstCursor *ReplayEventCursor
	LastCursor  *ReplayEventCursor
	Failures    []ArtifactFailure
	Available   bool
}

// ArtifactSchemaVersion identifies the detached portable artifact document
// contract.
type ArtifactSchemaVersion string

// ArtifactSchemaV1 is the first RecordingReplayArtifacts-published portable
// artifact schema, backed by Recordings' existing PortableArtifactSchemaV1
// document contract.
const ArtifactSchemaV1 ArtifactSchemaVersion = "recordings.portable-artifact.v1"

// ArtifactEnvelope is a detached, self-validating Recordings document. Events
// remain canonical Recordings facts and preserve their assigned order.
type ArtifactEnvelope struct {
	SchemaVersion ArtifactSchemaVersion
	Summary       ArtifactSummary
	Events        []ReplayEvent
	Integrity     ArtifactIntegrity
}

// BuildArtifactRequest selects a finalized recording by opaque identity.
type BuildArtifactRequest struct {
	RecordingID ReplayRecordingID
}

// BuildArtifactResult returns a detached completed artifact envelope.
type BuildArtifactResult struct {
	Artifact ArtifactEnvelope
}

// ValidateArtifactRequest validates one detached artifact envelope.
type ValidateArtifactRequest struct {
	Artifact ArtifactEnvelope
}

// ValidateArtifactResult returns its detached summary on success.
type ValidateArtifactResult struct {
	Summary ArtifactSummary
}

// EncodeArtifactRequest requests completed portable bytes.
type EncodeArtifactRequest struct {
	Artifact ArtifactEnvelope
}

// EncodeArtifactResult contains completed bytes, never an open writer, file,
// transaction, or temporary path.
type EncodeArtifactResult struct {
	Payload []byte
}

// DecodeArtifactRequest imports completed portable bytes.
type DecodeArtifactRequest struct {
	Payload []byte
}

// DecodeArtifactResult contains the validated detached artifact envelope.
type DecodeArtifactResult struct {
	Artifact ArtifactEnvelope
}

// SummarizeArtifactRequest inspects one detached artifact envelope.
type SummarizeArtifactRequest struct {
	Artifact ArtifactEnvelope
}

// SummarizeArtifactResult contains a detached summary copy.
type SummarizeArtifactResult struct {
	Summary ArtifactSummary
}

// ExportArtifactRequest closes one finalized recording and publishes its
// completed portable artifact to the recording's public reference.
type ExportArtifactRequest struct {
	RecordingID ReplayRecordingID
}

// ExportArtifactResult reports the published public reference and the
// detached artifact envelope that was exported.
type ExportArtifactResult struct {
	Reference ArtifactReference
	Artifact  ArtifactEnvelope
}

// ReadArtifactRequest reads one published portable artifact for the selected
// recording's export scope.
type ReadArtifactRequest struct {
	RecordingID ReplayRecordingID
	Reference   ArtifactReference
}

// ReadArtifactResult contains the validated detached artifact envelope read
// from the public destination.
type ReadArtifactResult struct {
	Artifact ArtifactEnvelope
}

// ReplayArtifactErrorKind distinguishes typed RecordingReplayArtifacts
// outcomes so peers can branch without depending on
// recordings/internal/contracts sentinel errors.
type ReplayArtifactErrorKind string

const (
	ReplayArtifactErrorNotFound          ReplayArtifactErrorKind = "NOT_FOUND"
	ReplayArtifactErrorNotFinalized      ReplayArtifactErrorKind = "NOT_FINALIZED"
	ReplayArtifactErrorCorruptInput      ReplayArtifactErrorKind = "CORRUPT_INPUT"
	ReplayArtifactErrorUnavailable       ReplayArtifactErrorKind = "UNAVAILABLE"
	ReplayArtifactErrorUnsupportedSchema ReplayArtifactErrorKind = "UNSUPPORTED_SCHEMA"
	ReplayArtifactErrorInvalidIntegrity  ReplayArtifactErrorKind = "INVALID_INTEGRITY"
	ReplayArtifactErrorInvalidOrder      ReplayArtifactErrorKind = "INVALID_ORDER"
	ReplayArtifactErrorInvalid           ReplayArtifactErrorKind = "INVALID"
	ReplayArtifactErrorExportFailed      ReplayArtifactErrorKind = "EXPORT_FAILED"
	ReplayArtifactErrorForeign           ReplayArtifactErrorKind = "FOREIGN"
	ReplayArtifactErrorCancelled         ReplayArtifactErrorKind = "CANCELLED"
)

// ReplayArtifactError is a typed RecordingReplayArtifacts failure peers can
// branch on via Kind or unwrap via Cause for standard errors.Is/errors.As
// matching.
type ReplayArtifactError struct {
	Kind    ReplayArtifactErrorKind
	Message string
	Cause   error
}

func (e *ReplayArtifactError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *ReplayArtifactError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
