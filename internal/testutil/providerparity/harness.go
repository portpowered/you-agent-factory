package providerparity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	opencodeadapter "github.com/portpowered/infinite-you/pkg/workers/provider/adapter/opencode"
	"github.com/portpowered/infinite-you/pkg/workers/provider/agy"
	"github.com/portpowered/infinite-you/pkg/workers/provider/claude"
	"github.com/portpowered/infinite-you/pkg/workers/provider/codex"
)

// TerminalResult is the neutral terminal outcome produced by one parity fixture run.
type TerminalResult struct {
	Outcome      adapter.CommandOutcome
	Response     workerexecution.InferenceResponse
	Capabilities adapter.Capabilities
	Drafts       []responseevents.Draft
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
	if fixture.AgyFinalOnly {
		return runAgyTerminal(ctx, fixture, transcript)
	}
	providerAdapter, err := adapterForFixture(fixture)
	if err != nil {
		return TerminalResult{}, err
	}
	registry, err := adapter.NewRegistry(providerAdapter)
	if err != nil {
		return TerminalResult{}, fmt.Errorf("registry fixture %q: %w", fixture.ID, err)
	}
	runner := newTranscriptRunner(transcript)
	result, err := adapter.Execute(ctx, registry, runner, adapter.ExecuteInput{
		Provider: fixture.Provider,
		Command:  adapter.CommandContext{Request: fixture.Request},
		Decoder: adapter.DecoderContext{
			RunID:      "run-parity-" + fixture.ID,
			DispatchID: fixture.Request.Dispatch.DispatchID,
		},
	})
	if err != nil {
		return TerminalResult{}, fmt.Errorf("execute fixture %q: %w", fixture.ID, err)
	}
	return TerminalResult{
		Outcome:      result.Outcome,
		Response:     result.Response,
		Capabilities: result.Capabilities,
		Drafts:       result.Drafts,
	}, nil
}

func runAgyTerminal(_ context.Context, fixture Fixture, transcript []byte) (TerminalResult, error) {
	factoryRoot, err := os.MkdirTemp("", "parity-agy-*")
	if err != nil {
		return TerminalResult{}, fmt.Errorf("agy temp dir: %w", err)
	}
	defer os.RemoveAll(factoryRoot)

	providerAdapter := agy.NewAdapter(factoryRoot, agy.WithExecutable("agy"))
	reported, err := providerAdapter.Capabilities(context.Background(), adapter.CapabilityContext{Request: fixture.Request})
	if err != nil {
		return TerminalResult{}, fmt.Errorf("agy capabilities: %w", err)
	}
	parsed, err := providerAdapter.ParseFinal(context.Background(), adapter.FinalParseContext{
		RunID:         "run-parity-" + fixture.ID,
		DispatchID:    fixture.Request.Dispatch.DispatchID,
		CommandResult: workerprocess.CommandResult{Stdout: transcript},
		FlushReason:   adapter.FlushReasonCompleted,
	})
	if err != nil {
		return TerminalResult{}, fmt.Errorf("agy parse final: %w", err)
	}
	return TerminalResult{
		Outcome:      adapter.CommandOutcomeCompleted,
		Response:     parsed.Response,
		Capabilities: reported.Capabilities,
		Drafts:       parsed.Drafts,
	}, nil
}

func adapterForFixture(fixture Fixture) (adapter.Adapter, error) {
	switch fixture.Provider {
	case adapter.Identity(modelprovider.Claude):
		return claude.NewAdapter(), nil
	case adapter.Identity(modelprovider.Codex):
		return codex.NewResponseAdapter(), nil
	case adapter.Identity(modelprovider.OpenCode):
		return openCodeAdapterForFixture(fixture)
	default:
		return nil, fmt.Errorf("unsupported parity provider %q", fixture.Provider)
	}
}

func openCodeAdapterForFixture(fixture Fixture) (adapter.Adapter, error) {
	mode := opencodeadapter.ModeStructured
	if fixture.FidelityClass == FidelityFinalOnly {
		mode = opencodeadapter.ModeFinalOnly
	}
	negotiated, err := opencodeadapter.NewNegotiatedAdapter(opencodeadapter.Decision{
		Installation: opencodeadapter.Installation{
			Executable:  "/parity/opencode",
			Fingerprint: "parity-fixture",
		},
		Version: "1.2.3",
		Mode:    mode,
	}, nil)
	if err != nil {
		return nil, err
	}
	return negotiated, nil
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
	if middle < 1 {
		middle = 1
	}
	return &transcriptRunner{
		transcript: transcript,
		chunks: []adapter.Observation{
			{Stream: adapter.OutputStreamStdout, Chunk: transcript[:minInt(7, len(transcript))]},
			{Stream: adapter.OutputStreamStdout, Chunk: transcript[minInt(7, len(transcript)):middle]},
			{Stream: adapter.OutputStreamStdout, Chunk: transcript[middle:]},
		},
	}
}

func (r *transcriptRunner) Run(ctx context.Context, _ workerprocess.CommandRequest, observe func(adapter.Observation) error) (workerprocess.CommandResult, error) {
	for _, chunk := range r.chunks {
		if err := ctx.Err(); err != nil {
			return workerprocess.CommandResult{}, err
		}
		if len(chunk.Chunk) == 0 {
			continue
		}
		if err := observe(chunk); err != nil {
			return workerprocess.CommandResult{}, err
		}
	}
	return workerprocess.CommandResult{Stdout: r.transcript}, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ adapter.StreamingCommandRunner = (*transcriptRunner)(nil)
