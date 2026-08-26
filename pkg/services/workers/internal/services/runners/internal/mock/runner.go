// Package mock implements the Workers-parent-private mock Runner strategy.
// It is a testing-feature path only and must not be wired into production
// Workers construction as a bypass around agent, script, or inference.
package mock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
	mockworkerbehavior "github.com/portpowered/infinite-you/pkg/services/workers/internal/mockworker"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

const Identity = "mock"

// Config is the immutable mock definition captured by one Runner.
type Config struct {
	WorkersConfig *workers.MockWorkersConfig
}

// Dependencies are optional effects for mock script passthrough. Production
// Workers construction must leave this registry unregistered.
type Dependencies struct {
	Next  workerprocess.CommandRunner
	Files mockworkerbehavior.GateFileSystem
}

type runner struct {
	config *workers.MockWorkersConfig
	next   workerprocess.CommandRunner
	files  mockworkerbehavior.GateFileSystem
}

var _ workers.Runner = (*runner)(nil)

// New validates and snapshots one mock Strategy. Construction is inert.
func New(config Config, dependencies Dependencies) (workers.Runner, error) {
	if config.WorkersConfig == nil {
		return nil, errors.New("construct mock runner: mock workers config is required")
	}
	return &runner{config: config.WorkersConfig.Clone(), next: dependencies.Next, files: dependencies.Files}, nil
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
				Diagnostics: &workers.WorkDiagnostics{Metadata: map[string]string{
					workers.ProviderResponseMetadataCompletionEvidence: "provider_response",
				}},
			}, nil
		}
		return acceptResult(), nil
	}
	recordMockWorkerUsage(ctx, request.Correlation, entry.Usage)
	if entry.GateConfig != nil {
		if err := mockworkerbehavior.WaitForGate(ctx, *entry.GateConfig, r.files); err != nil {
			return workers.RunnerExecutionResult{}, err
		}
	}
	switch entry.RunType {
	case workers.MockWorkerRunTypeReject:
		return workerexecution.ApplyMockWorkerUsageDiagnostics(
				workers.RunnerExecutionResult{}, entry.Usage,
			), workers.NewProviderError(
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
		commandResult, err := r.next.Run(ctx, workerprocess.CommandRequest{
			Command:         script.Command,
			Args:            append([]string(nil), script.Args...),
			WorkerType:      request.WorkerType,
			WorkstationName: request.WorkstationType,
			WorkDir:         script.WorkingDirectory,
		})
		if err != nil {
			return workers.RunnerExecutionResult{}, err
		}
		if commandResult.ExitCode != 0 {
			return workers.RunnerExecutionResult{}, workers.NewProviderError(
				workers.WorkFailureTypeInternalServerError,
				"mock script command failed",
				nil,
			)
		}
		runnerResult := workers.RunnerExecutionResult{
			Content: strings.TrimSpace(string(commandResult.Stdout)),
			Diagnostics: &workers.WorkDiagnostics{
				Command: &workers.CommandDiagnostic{
					Command:  script.Command,
					Args:     append([]string(nil), script.Args...),
					Stdout:   string(commandResult.Stdout),
					Stderr:   string(commandResult.Stderr),
					ExitCode: commandResult.ExitCode,
				},
				Metadata: map[string]string{
					workers.ProviderResponseMetadataCompletionEvidence: "provider_response",
				},
			},
		}
		return workerexecution.ApplyMockWorkerUsageDiagnostics(runnerResult, entry.Usage), nil
	default:
		return workerexecution.ApplyMockWorkerUsageDiagnostics(acceptResult(), entry.Usage), nil
	}
}

func recordMockWorkerUsage(
	ctx context.Context,
	correlation workers.ExecutionCorrelation,
	usage *workers.MockWorkerUsageConfig,
) {
	if capture := workerexecution.MockWorkerUsageCaptureFromContext(ctx); capture != nil {
		capture.Record(usage)
		return
	}
	workerexecution.PublishMockWorkerUsage(ctx, correlation, usage)
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
		if !mockWorkInputSelectorsMatch(candidate.WorkInputs, mockRequestInputs(request)) {
			continue
		}
		return candidate, true
	}
	return workers.MockWorkerConfig{}, false
}

func mockRequestInputs(request workers.RunnerExecutionRequest) []any {
	inputs := make([]any, 0, len(request.InputTokens)+len(request.Dispatch.InputTokens))
	inputs = append(inputs, request.InputTokens...)
	inputs = append(inputs, request.Dispatch.InputTokens...)
	return inputs
}

type mockInputToken struct {
	ID         string            `json:"id"`
	State      string            `json:"state"`
	WorkID     string            `json:"workId"`
	WorkTypeID string            `json:"workTypeId"`
	TraceID    string            `json:"traceId"`
	Tags       map[string]string `json:"tags"`
	Payload    []byte            `json:"payload"`
	Color      struct {
		WorkID     string            `json:"work_id"`
		WorkTypeID string            `json:"work_type_id"`
		DataType   string            `json:"data_type"`
		TraceID    string            `json:"trace_id"`
		Tags       map[string]string `json:"tags"`
		Payload    []byte            `json:"payload"`
	} `json:"color"`
}

func mockWorkInputSelectorsMatch(selectors []workers.MockWorkInputSelector, raw []any) bool {
	for _, selector := range selectors {
		matched := false
		for _, item := range raw {
			if input, ok := item.(workers.WorkInput); ok && mockSelectorMatchesWorkInput(selector, input) {
				matched = true
				break
			}
			encoded, err := json.Marshal(item)
			if err != nil {
				continue
			}
			var token mockInputToken
			if json.Unmarshal(encoded, &token) == nil && mockSelectorMatchesToken(selector, token) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func mockSelectorMatchesWorkInput(selector workers.MockWorkInputSelector, input workers.WorkInput) bool {
	return (selector.WorkID == "" || selector.WorkID == input.WorkID) &&
		(selector.WorkType == "" || selector.WorkType == input.WorkTypeID) &&
		(selector.State == "" || selector.State == input.State) &&
		(selector.InputName == "" || contains(input.InputNames, selector.InputName)) &&
		(selector.TraceID == "" || selector.TraceID == input.Lineage.TraceID) &&
		(selector.Channel == "" || selector.Channel == input.Tags["channel"]) &&
		(selector.PayloadHash == "" || selector.PayloadHash == mockPayloadHash(workInputPayload(input)))
}

func mockSelectorMatchesToken(selector workers.MockWorkInputSelector, token mockInputToken) bool {
	workID := firstNonEmpty(token.Color.WorkID, token.WorkID)
	workTypeID := firstNonEmpty(token.Color.WorkTypeID, token.WorkTypeID)
	traceID := firstNonEmpty(token.Color.TraceID, token.TraceID)
	tags := token.Color.Tags
	if tags == nil {
		tags = token.Tags
	}
	payload := token.Color.Payload
	if len(payload) == 0 {
		payload = token.Payload
	}
	return (selector.WorkID == "" || selector.WorkID == workID) &&
		(selector.WorkType == "" || selector.WorkType == workTypeID) &&
		(selector.State == "" || selector.State == token.State) &&
		selector.InputName == "" &&
		(selector.TraceID == "" || selector.TraceID == traceID) &&
		(selector.Channel == "" || selector.Channel == tags["channel"]) &&
		(selector.PayloadHash == "" || selector.PayloadHash == mockPayloadHash(payload))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func workInputPayload(input workers.WorkInput) []byte {
	for _, part := range input.Content {
		if part.Type.Normalized() == "text" {
			return []byte(part.Text)
		}
	}
	return nil
}

func mockPayloadHash(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func acceptResult() workers.RunnerExecutionResult {
	return workers.RunnerExecutionResult{
		Content: "mock worker accepted",
		Diagnostics: &workers.WorkDiagnostics{
			Metadata: map[string]string{
				"mock": "accept",
				workers.ProviderResponseMetadataCompletionEvidence: "provider_response",
			},
		},
	}
}
