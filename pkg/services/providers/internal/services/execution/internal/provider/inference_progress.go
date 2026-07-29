package provider

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/adapter"
)

const (
	ProgressFragmentKind         = workerexecution.ProgressFragmentKind
	ResponseFragmentKind         = workerexecution.ResponseFragmentKind
	CompletedFragmentKind        = workerexecution.CompletedFragmentKind
	FailedFragmentKind           = workerexecution.FailedFragmentKind
	NormalizedEventTypeUnknown   = "UNKNOWN"
	NormalizedEventTypeStarted   = "STARTED"
	NormalizedEventTypeProgress  = "PROGRESS"
	NormalizedEventTypeTextDelta = "TEXT_DELTA"
	NormalizedEventTypeFinalText = "FINAL_TEXT"
	NormalizedEventTypeFailed    = "FAILED"
	NormalizedEventTypeCanceled  = "CANCELED"
)

const (
	codexRetainedTextBytes       = 4096
	codexRetainedProgressBytes   = 1024
	codexMetadataRunnerIDKey     = "runner_id"
	codexMetadataWorkIDKey       = "work_id"
	codexMetadataWorkstationKey  = "workstation_name"
	codexMetadataTextBytesKey    = "text_bytes"
	codexMetadataTruncatedKey    = "payload_truncated"
	codexMetadataRawBytesKey     = "raw_bytes"
	codexMetadataRawSHA256Key    = "raw_sha256"
	codexMetadataDiagnosticKey   = "diagnostic_class"
	codexDiagnosticUnknownEvent  = "unknown_event"
	codexDiagnosticMalformedJSON = "malformed_json"
	codexDiagnosticIncompleteSSE = "incomplete_event_stream"
)

func isCodexCommand(command string) bool {
	base := filepath.Base(strings.ReplaceAll(strings.TrimSpace(command), `\`, "/"))
	extension := strings.ToLower(filepath.Ext(base))
	if extension == ".exe" || extension == ".cmd" || extension == ".bat" {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return strings.EqualFold(base, string(modelprovider.ProviderCodex))
}

// InferenceProgressFragment is the provider-boundary shape for transient internal
// session progress that must not enter canonical factory event history.
type InferenceProgressFragment = workerexecution.ProgressFragment

// CanonicalDraftFragment carries one provider-native canonical response draft
// to the session-owned publisher without flattening it into a legacy fragment.
func CanonicalDraftFragment(dispatchID string, draft any) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:     strings.TrimSpace(dispatchID),
		CanonicalDraft: draft,
	}
}

// InferenceProgressPublisher receives provider progress fragments for one live
// Factory Session internal response stream.
type InferenceProgressPublisher = workerexecution.ProgressPublisher

// ProgressFragment builds one ordered progress fragment for a dispatch.
func ProgressFragment(dispatchID string, providerSession *workerexecution.ProviderSessionMetadata, payload string) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               ProgressFragmentKind,
		Type:               NormalizedEventTypeProgress,
		Payload:            payload,
		ProviderSessionRef: workerexecution.CloneProviderSessionMetadata(providerSession),
	}
}

// ResponseFragment builds one ordered response fragment for a dispatch.
func ResponseFragment(dispatchID string, providerSession *workerexecution.ProviderSessionMetadata, payload string) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               ResponseFragmentKind,
		Type:               NormalizedEventTypeTextDelta,
		Payload:            payload,
		ProviderSessionRef: workerexecution.CloneProviderSessionMetadata(providerSession),
	}
}

// CompletedFragment builds one terminal completion marker for a dispatch.
func CompletedFragment(dispatchID string, providerSession *workerexecution.ProviderSessionMetadata) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               CompletedFragmentKind,
		ProviderSessionRef: workerexecution.CloneProviderSessionMetadata(providerSession),
	}
}

// FailedFragment builds one terminal failure marker for a dispatch.
func FailedFragment(dispatchID string, providerSession *workerexecution.ProviderSessionMetadata, payload string) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               FailedFragmentKind,
		Payload:            payload,
		ProviderSessionRef: workerexecution.CloneProviderSessionMetadata(providerSession),
	}
}

// InferenceProgressPublishingCommandRunner publishes internal response-stream
// fragments while provider subprocess stdout/stderr grow.
type InferenceProgressPublishingCommandRunner struct {
	Publisher InferenceProgressPublisher
	Logger    logging.Logger
	Runner    CommandRunner
}

// SupportsResponseStreaming reports that the runner observes subprocess output
// incrementally and can therefore consume native streaming protocols.
func (InferenceProgressPublishingCommandRunner) SupportsResponseStreaming() bool { return true }

// Run executes the provider subprocess and publishes incremental stdout/stderr
// fragments into the configured internal session response stream.
func (r InferenceProgressPublishingCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	if r.Publisher == nil {
		if r.Runner == nil {
			return CommandResult{}, errors.New("provider progress command runner is required")
		}
		return r.Runner.Run(ctx, req)
	}
	dispatchID := strings.TrimSpace(req.DispatchID)
	progressStream := newProgressStreamObserver(req, r.Publisher, r.Logger)
	observer := func(stream string, chunk []byte) {
		if len(chunk) == 0 {
			return
		}
		if progressStream != nil && progressStream.observe(ctx, stream, chunk) {
			return
		}
		payload := string(chunk)
		switch stream {
		case workerexecution.OutputStreamStdout:
			r.Publisher(ResponseFragment(dispatchID, nil, payload))
		case workerexecution.OutputStreamStderr:
			r.Publisher(ProgressFragment(dispatchID, nil, payload))
		}
	}
	result, err := workerexecution.StreamingExecCommandRunner{
		Observer: observer,
		Logger:   logging.EnsureLogger(r.Logger),
		Runner:   r.Runner,
	}.Run(ctx, req)
	if progressStream != nil {
		progressStream.flush(ctx, result, err)
	}
	return result, err
}

// NewInferenceProgressPublishingCommandRunner constructs a provider command
// runner that publishes internal response-stream fragments during subprocess IO.
func NewInferenceProgressPublishingCommandRunner(
	publisher InferenceProgressPublisher,
	logger logging.Logger,
) CommandRunner {
	return InferenceProgressPublishingCommandRunner{
		Publisher: publisher,
		Logger:    logger,
	}
}

// NewInferenceProgressPublishingCommandRunnerWithRunner wraps the already
// injected command edge with progress observation.
func NewInferenceProgressPublishingCommandRunnerWithRunner(
	runner CommandRunner,
	publisher InferenceProgressPublisher,
	logger logging.Logger,
) CommandRunner {
	return InferenceProgressPublishingCommandRunner{Runner: runner, Publisher: publisher, Logger: logger}
}

type progressStreamObserver interface {
	observe(ctx context.Context, stream string, chunk []byte) bool
	flush(ctx context.Context, result CommandResult, err error)
}

func progressStreamIdentity(command string) adapter.Identity {
	return adapter.NormalizeIdentity(adapter.Identity(command))
}

func newProgressStreamObserver(
	_ CommandRequest,
	_ InferenceProgressPublisher,
	_ logging.Logger,
) progressStreamObserver {
	return nil
}
