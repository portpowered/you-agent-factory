package provider

import (
	"context"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
)

const (
	ProgressFragmentKind  = "PROGRESS_FRAGMENT"
	ResponseFragmentKind  = "RESPONSE_FRAGMENT"
)

// InferenceProgressFragment is the provider-boundary shape for transient internal
// session progress that must not enter canonical factory event history.
type InferenceProgressFragment struct {
	DispatchID         string
	Kind               string
	Payload            string
	ProviderSessionRef *interfaces.ProviderSessionMetadata
}

// InferenceProgressPublisher receives provider progress fragments for one live
// Factory Session internal response stream.
type InferenceProgressPublisher func(fragment InferenceProgressFragment)

// ProgressFragment builds one ordered progress fragment for a dispatch.
func ProgressFragment(dispatchID string, providerSession *interfaces.ProviderSessionMetadata, payload string) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               ProgressFragmentKind,
		Payload:            payload,
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(providerSession),
	}
}

// ResponseFragment builds one ordered response fragment for a dispatch.
func ResponseFragment(dispatchID string, providerSession *interfaces.ProviderSessionMetadata, payload string) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               ResponseFragmentKind,
		Payload:            payload,
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(providerSession),
	}
}

// InferenceProgressPublishingCommandRunner publishes internal response-stream
// fragments while provider subprocess stdout/stderr grow.
type InferenceProgressPublishingCommandRunner struct {
	Publisher InferenceProgressPublisher
	Logger    logging.Logger
}

// Run executes the provider subprocess and publishes incremental stdout/stderr
// fragments into the configured internal session response stream.
func (r InferenceProgressPublishingCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	if r.Publisher == nil {
		return workerprocess.ExecCommandRunner{}.Run(ctx, req)
	}
	dispatchID := strings.TrimSpace(req.DispatchID)
	return workerprocess.StreamingExecCommandRunner{
		Observer: func(stream string, chunk []byte) {
			if len(chunk) == 0 {
				return
			}
			payload := string(chunk)
			switch stream {
			case workerprocess.OutputStreamStdout:
				r.Publisher(ResponseFragment(dispatchID, nil, payload))
			case workerprocess.OutputStreamStderr:
				r.Publisher(ProgressFragment(dispatchID, nil, payload))
			}
		},
		Logger: logging.EnsureLogger(r.Logger),
	}.Run(ctx, req)
}

// NewInferenceProgressPublishingCommandRunner constructs a provider command
// runner that publishes internal response-stream fragments during subprocess IO.
func NewInferenceProgressPublishingCommandRunner(
	publisher InferenceProgressPublisher,
	logger logging.Logger,
) CommandRunner {
	if publisher == nil {
		return workerprocess.ExecCommandRunner{}
	}
	return InferenceProgressPublishingCommandRunner{
		Publisher: publisher,
		Logger:    logger,
	}
}
