package providerparity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/agy"
)

// TerminalResult is the neutral terminal outcome produced by one parity fixture run.
type TerminalResult struct {
	Outcome      adapter.CommandOutcome
	Response     workerexecution.InferenceResponse
	Capabilities adapter.Capabilities
	Drafts       []factorysessions.ResponseEventDraft
}

// RunTerminal executes one fixture through the adapter-neutral harness and returns
// its terminal invocation outcome without live provider credentials.
func RunTerminal(ctx context.Context, fixture Fixture) (TerminalResult, error) {
	transcript, err := ReadTranscript(fixture.TranscriptFile)
	if err != nil {
		return TerminalResult{}, err
	}
	if err := ValidateSanitized(transcript); err != nil {
		return TerminalResult{}, fmt.Errorf("fixture %q: %w", fixture.ID, err)
	}
	if fixture.Provider != adapter.Identity(modelprovider.ProviderAntigravity) {
		return TerminalResult{}, fmt.Errorf("unsupported parity provider %q", fixture.Provider)
	}
	if fixture.AgyFinalOnly {
		return runAgyTerminal(ctx, fixture, transcript)
	}
	return TerminalResult{}, fmt.Errorf("unsupported parity fixture %q", fixture.ID)
}

func runAgyTerminal(_ context.Context, fixture Fixture, transcript []byte) (TerminalResult, error) {
	response, capabilities, drafts, err := agy.ParityTerminal(
		"run-parity-"+fixture.ID,
		fixture.Request.Dispatch.DispatchID,
		transcript,
	)
	if err != nil {
		return TerminalResult{}, err
	}
	return TerminalResult{
		Outcome:      adapter.CommandOutcomeCompleted,
		Response:     response,
		Capabilities: capabilities,
		Drafts:       drafts,
	}, nil
}

// ReadTranscript loads one fixture transcript from the package testdata directory.
func ReadTranscript(relPath string) ([]byte, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("resolve provider parity package path")
	}
	path := filepath.Join(filepath.Dir(file), relPath)
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read transcript %s: %w", relPath, err)
	}
	return contents, nil
}

type transcriptRunner struct {
	transcript []byte
	chunks     []adapter.Observation
}

func newTranscriptRunner(transcript []byte) *transcriptRunner {
	if len(transcript) == 0 {
		return &transcriptRunner{}
	}
	middle := len(transcript) / 2
	if middle == 0 {
		middle = len(transcript)
	}
	return &transcriptRunner{
		transcript: transcript,
		chunks: []adapter.Observation{
			{Stream: adapter.OutputStreamStdout, Chunk: transcript[:middle]},
			{Stream: adapter.OutputStreamStdout, Chunk: transcript[middle:]},
		},
	}
}

func (r *transcriptRunner) Run(
	_ context.Context,
	_ workers.CommandRequest,
	observe func(adapter.Observation) error,
) (workers.CommandResult, error) {
	for _, chunk := range r.chunks {
		if err := observe(chunk); err != nil {
			return workers.CommandResult{}, err
		}
	}
	return workers.CommandResult{Stdout: r.transcript}, nil
}

func adapterIdentityForProvider(provider modelprovider.Provider) adapter.Identity {
	return adapter.Identity(provider)
}
