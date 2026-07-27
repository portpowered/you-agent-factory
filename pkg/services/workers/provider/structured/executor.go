// Package structured wires provider-owned response adapters into subprocess execution.
package structured

import (
	"context"
	"fmt"
	"strings"
	"sync"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/work"
	responseevents "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	claudeadapter "github.com/portpowered/infinite-you/pkg/services/workers/provider/claude"
	codexadapter "github.com/portpowered/infinite-you/pkg/services/workers/provider/codex"
	piadapter "github.com/portpowered/infinite-you/pkg/services/workers/provider/pi"
)

// Executor owns the registered structured provider adapters.
type Executor struct {
	registry *adapter.Registry
}

// NewExecutor constructs the production structured adapter registry.
func NewExecutor() *Executor {
	registry, err := adapter.NewRegistry(claudeadapter.NewAdapter(), codexadapter.NewResponseAdapter(), piadapter.NewAdapter())
	if err != nil {
		panic(fmt.Sprintf("register structured provider adapters: %v", err))
	}
	return &Executor{registry: registry}
}

// Supports reports whether provider has a registered structured adapter.
func (e *Executor) Supports(provider string) bool {
	if e == nil || e.registry == nil {
		return false
	}
	_, err := e.registry.Lookup(adapter.Identity(provider))
	return err == nil
}

// Execute runs the selected structured adapter and publishes correlated drafts
// while leaving terminal failure policy at the parent provider boundary.
func (e *Executor) Execute(
	ctx context.Context,
	req workerexecution.ProviderInferenceRequest,
	skipPermissions bool,
	contentMaterializer work.ContentMaterializer,
	runner workerprovider.CommandRunner,
	publisher workerprovider.InferenceProgressPublisher,
	logger logging.Logger,
) workerprovider.ResponseStreamExecutionResult {
	canonicalFailurePublished := false
	result, executeErr := adapter.Execute(ctx, e.registry, commandRunner{runner: runner, logger: logger}, adapter.ExecuteInput{
		Provider: adapter.Identity(req.ModelProvider),
		Command:  adapter.CommandContext{Request: req, SkipPermissions: skipPermissions, ContentMaterializer: contentMaterializer},
		Decoder:  adapter.DecoderContext{RunID: req.Dispatch.DispatchID, DispatchID: req.Dispatch.DispatchID},
		ObserveDraft: func(draft responseevents.Draft) {
			if draft.Kind == responseevents.KindError && draft.Phase == responseevents.PhaseFailed {
				canonicalFailurePublished = true
			}
			if publisher != nil {
				publisher(workerprovider.CanonicalDraftFragment(draft.DispatchID, draft))
			}
		},
	})
	output := workerprovider.ResponseStreamExecutionResult{
		Response: result.Response, Request: result.Request, Command: result.Command,
		CanonicalFailurePublished: canonicalFailurePublished, Err: executeErr,
	}
	if result.Failure != nil {
		output.FailureType = result.Failure.Type
		output.FailureMessage = result.Failure.Message
		output.FailureSession = result.Failure.ProviderSession
	}
	return output
}

type commandRunner struct {
	runner workerprovider.CommandRunner
	logger logging.Logger
}

func (r commandRunner) Run(
	ctx context.Context,
	req workerprocess.CommandRequest,
	observe func(adapter.Observation) error,
) (workerprocess.CommandResult, error) {
	switch r.runner.(type) {
	case workerprovider.InferenceProgressPublishingCommandRunner, *workerprovider.InferenceProgressPublishingCommandRunner,
		workerprocess.ExecCommandRunner, *workerprocess.ExecCommandRunner,
		workerprocess.StreamingAdaptedCommandRunner, *workerprocess.StreamingAdaptedCommandRunner:
		return runObservedCommand(ctx, req, observe, r.runner, r.logger)
	}
	runner := r.runner
	if runner == nil {
		return runObservedCommand(ctx, req, observe, r.runner, r.logger)
	}
	result, err := runner.Run(ctx, req)
	if len(result.Stdout) > 0 {
		err = observeResult(adapter.OutputStreamStdout, result.Stdout, observe, err)
	}
	if len(result.Stderr) > 0 {
		err = observeResult(adapter.OutputStreamStderr, result.Stderr, observe, err)
	}
	return result, err
}

func observeResult(stream adapter.OutputStream, chunk []byte, observe func(adapter.Observation) error, runErr error) error {
	observeErr := observe(adapter.Observation{Stream: stream, Chunk: chunk})
	if runErr != nil {
		return runErr
	}
	return observeErr
}

func runObservedCommand(
	ctx context.Context,
	req workerprocess.CommandRequest,
	observe func(adapter.Observation) error,
	baseRunner workerprovider.CommandRunner,
	logger logging.Logger,
) (workerprocess.CommandResult, error) {
	var mu sync.Mutex
	var observeErr error
	runner := workerprocess.StreamingExecCommandRunner{Runner: baseRunner, Logger: logger, Observer: func(stream string, chunk []byte) {
		mu.Lock()
		defer mu.Unlock()
		if observeErr == nil {
			observeErr = observe(adapter.Observation{Stream: adapter.OutputStream(strings.TrimSpace(stream)), Chunk: chunk})
		}
	}}
	result, err := runner.Run(ctx, req)
	mu.Lock()
	defer mu.Unlock()
	if err == nil {
		err = observeErr
	}
	return result, err
}

var _ workerprovider.ResponseStreamExecutor = (*Executor)(nil)
var _ adapter.StreamingCommandRunner = commandRunner{}
