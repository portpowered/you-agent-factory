package recordings

// ReplayInputCapability is a narrow, Recordings-owned capability for
// classifying and loading one historical replay input selected by
// filesystem path into either a portable JavaScript Factory Session
// recording or a legacy embedded-Factory replay artifact. It composes
// Recordings' existing portable-recording decode/validate behavior and
// legacy replay-artifact loading behavior, so a caller that only needs to
// classify and load a replay input does not have to combine a raw file
// reader, an aliased decoder/validator, and a legacy loader itself.
type ReplayInputCapability interface {
	// LoadReplayInput reads the file at the selected path, classifies it as
	// a portable JavaScript Factory Session recording or a legacy
	// embedded-Factory replay artifact, and returns the decoded/validated
	// result for exactly one of those two families.
	LoadReplayInput(LoadReplayInputRequest) (LoadReplayInputResult, error)
}

// LoadReplayInputRequest selects one historical replay input by filesystem
// path.
type LoadReplayInputRequest struct {
	Path string
}

// LoadReplayInputResult contains exactly one of Portable or Legacy,
// depending on which replay input family the selected path contained.
type LoadReplayInputResult struct {
	Portable *PortableRecording
	Legacy   *ReplayArtifact
}

// ReplayInputDiagnosticCode identifies one ReplayInputCapability structured
// validation failure. Values mirror the existing portable-recording
// diagnostic vocabulary so a directly owned ReplayInputCapability consumer
// observes the same failure areas without depending on
// recordings/internal/contracts.
type ReplayInputDiagnosticCode string

const (
	ReplayInputDiagnosticMalformed          ReplayInputDiagnosticCode = "MALFORMED_RECORDING_CONTRACT"
	ReplayInputDiagnosticUnsupportedVersion ReplayInputDiagnosticCode = "UNSUPPORTED_REPLAY_COMPATIBILITY_VERSION"
	ReplayInputDiagnosticInvalidIdentity    ReplayInputDiagnosticCode = "INVALID_RECORDING_IDENTITY"
	ReplayInputDiagnosticInvalidDigest      ReplayInputDiagnosticCode = "INVALID_RECORDING_DIGEST"
	ReplayInputDiagnosticInvalidSummary     ReplayInputDiagnosticCode = "INVALID_RECORDING_SUMMARY"
)

// ReplayInputDiagnostic reports one structured, directly owned portable
// replay-input validation failure area. It is populated whenever the
// selected path classifies as a portable JavaScript Factory Session
// recording but fails decode or validation.
type ReplayInputDiagnostic struct {
	Code              ReplayInputDiagnosticCode
	Area              string
	Path              string
	Message           string
	SupportedVersions []string
}

// ReplayInputErrorKind distinguishes typed ReplayInputCapability outcomes so
// callers can branch on classification (read, portable, or legacy) without
// depending on recordings/internal/contracts sentinel errors.
type ReplayInputErrorKind string

const (
	// ReplayInputErrorRead reports a failure to read the selected path
	// before either replay input family could be classified.
	ReplayInputErrorRead ReplayInputErrorKind = "READ_FAILED"
	// ReplayInputErrorPortable reports a portable JavaScript Factory
	// Session recording that failed decode or validation.
	ReplayInputErrorPortable ReplayInputErrorKind = "INVALID_PORTABLE_RECORDING"
	// ReplayInputErrorLegacy reports a legacy embedded-Factory replay
	// artifact that failed to load.
	ReplayInputErrorLegacy ReplayInputErrorKind = "LEGACY_LOAD_FAILED"
)

// ReplayInputError is a typed, directly owned ReplayInputCapability failure.
// Diagnostic is populated only for ReplayInputErrorPortable; callers branch
// on Kind or unwrap Cause for standard errors.Is/errors.As matching.
type ReplayInputError struct {
	Kind       ReplayInputErrorKind
	Diagnostic *ReplayInputDiagnostic
	Message    string
	Cause      error
}

func (e *ReplayInputError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *ReplayInputError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
