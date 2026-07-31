// Package mock implements the Workers-parent-private mock Runner strategy.
// It is a testing-feature path only and must not be wired into production
// Workers construction as a bypass around agent, script, or inference.
package mock

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const Identity = "mock"

// Config is the immutable mock definition captured by one Runner.
type Config struct {
	WorkersConfig *workers.MockWorkersConfig
}

// Dependencies are optional effects for mock script passthrough. Production
// Workers construction must leave this registry unregistered.
type Dependencies struct {
	Next workers.CommandRunner
}

type runner struct {
	config *workers.MockWorkersConfig
	next   workers.CommandRunner
}

var _ workers.Runner = (*runner)(nil)

// New validates and snapshots one mock Strategy. Construction is inert.
func New(config Config, dependencies Dependencies) (workers.Runner, error) {
	if config.WorkersConfig == nil {
		return nil, errors.New("construct mock runner: mock workers config is required")
	}
	snapshot := *config.WorkersConfig
	snapshot.MockWorkers = append(
		[]workers.MockWorkerConfig(nil),
		config.WorkersConfig.MockWorkers...,
	)
	return &runner{config: &snapshot, next: dependencies.Next}, nil
}

// Execute evaluates one request-scoped mock decision without retaining caller
// mutable state after return.
func (r *runner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	request = workers.CloneProviderInferenceRequest(request)
	if strings.TrimSpace(request.RunnerID) == "" {
		return workers.RunnerExecutionResult{}, workers.NewProviderError(
			workers.WorkFailureTypePermanentBadRequest,
			"mock runner request is invalid",
			nil,
		)
	}
	entry, matched := r.match(request)
	if !matched {
		if r.config.UnmatchedDispatchPolicy.PassthroughUnmatched() {
			return workers.RunnerExecutionResult{
				Content: "mock unmatched passthrough",
			}, nil
		}
		return acceptResult(), nil
	}
	switch entry.RunType {
	case workers.MockWorkerRunTypeReject:
		return workers.RunnerExecutionResult{}, workers.NewProviderError(
			workers.WorkFailureTypeInternalServerError,
			"mock worker rejected the dispatch",
			nil,
		)
	case workers.MockWorkerRunTypeScript:
		if entry.ScriptConfig == nil {
			return workers.RunnerExecutionResult{}, workers.NewProviderError(
				workers.WorkFailureTypePermanentBadRequest,
				"mock scriptConfig is required",
				nil,
			)
		}
		if r.next == nil {
			return workers.RunnerExecutionResult{}, workers.NewProviderError(
				workers.WorkFailureTypeInternalServerError,
				"mock script requires an injected command runner",
				nil,
			)
		}
		script := entry.ScriptConfig
		result, err := r.next.Run(ctx, workers.CommandRequest{
			Command:         script.Command,
			Args:            append([]string(nil), script.Args...),
			WorkerType:      request.WorkerType,
			WorkstationName: request.WorkstationType,
			WorkDir:         script.WorkingDirectory,
		})
		if err != nil {
			return workers.RunnerExecutionResult{}, err
		}
		if result.ExitCode != 0 {
			return workers.RunnerExecutionResult{}, workers.NewProviderError(
				workers.WorkFailureTypeInternalServerError,
				"mock script command failed",
				nil,
			)
		}
		return workers.RunnerExecutionResult{
			Content: strings.TrimSpace(string(result.Stdout)),
			Diagnostics: &workers.WorkDiagnostics{
				Command: &workers.CommandDiagnostic{
					Command:  script.Command,
					Args:     append([]string(nil), script.Args...),
					Stdout:   string(result.Stdout),
					Stderr:   string(result.Stderr),
					ExitCode: result.ExitCode,
				},
			},
		}, nil
	default:
		return acceptResult(), nil
	}
}

func (r *runner) match(
	request workers.RunnerExecutionRequest,
) (workers.MockWorkerConfig, bool) {
	for _, candidate := range r.config.MockWorkers {
		if candidate.WorkerName != "" &&
			candidate.WorkerName != request.WorkerType {
			continue
		}
		if candidate.WorkstationName != "" &&
			candidate.WorkstationName != request.WorkstationType {
			continue
		}
		return candidate, true
	}
	return workers.MockWorkerConfig{}, false
}

func acceptResult() workers.RunnerExecutionResult {
	return workers.RunnerExecutionResult{
		Content: "mock worker accepted",
		Diagnostics: &workers.WorkDiagnostics{
			Metadata: map[string]string{"mock": "accept"},
		},
	}
}
