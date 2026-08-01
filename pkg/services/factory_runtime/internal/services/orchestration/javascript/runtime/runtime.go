package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/javascript/validation"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtimecontract"
)

// Run executes one simple JavaScript workflow source with explicit inputs and hooks.
func Run(ctx context.Context, req Request, hooks Hooks) (Outcome, error) {
	if err := ctx.Err(); err != nil {
		return contextTerminationOutcome(err), nil
	}
	if outcome := preExecutionOutcome(req); outcome != nil {
		return *outcome, nil
	}
	policy := req.Policy
	if policy.Mode == "" {
		policy = workflowpolicy.DefaultEffectivePolicy()
	}

	vm := goja.New()
	records := newRecordCollector()
	records.onRecord = hooks.OnRecord
	sessionID := strings.TrimSpace(req.SessionID)
	childExecutor := childExecutorForRequest(sessionID, records, hooks, req.Resume, policy)
	globals := &runtimeGlobals{
		vm:             vm,
		policy:         policy,
		sessionID:      sessionID,
		ctx:            ctx,
		records:        records,
		childExecutor:  childExecutor,
		agents:         req.Agents,
		workerSettings: req.WorkerSettings,
		onArtifact:     hooks.OnArtifact,
	}
	globals.bindResumeCheckpointState(req.Resume)
	if err := globals.bindWorkflowAPI(); err != nil {
		return Outcome{}, err
	}

	argsValue, err := argsValueForRequest(vm, req.Args)
	if err != nil {
		return invalidArgsFailure(err), nil
	}
	if err := validateRequestArgs(req.Args, req.ArgsSchema); err != nil {
		return invalidArgsFailure(err), nil
	}
	globals.bindArgs(argsValue)
	globals.bindMeta(metaFromRequest(req.Metadata))

	interrupt := make(chan struct{}, 1)
	go watchContext(ctx, vm, interrupt)

	value, runErr := vm.RunString(wrapWorkflowSource(req.Source))
	close(interrupt)
	if ctxErr := ctx.Err(); ctxErr != nil {
		outcome := contextTerminationOutcome(ctxErr)
		outcome.Records = records.list()
		return outcome, nil
	}
	if runErr != nil {
		outcome := scriptErrorOutcome(vm, runErr)
		outcome.Records = records.list()
		return outcome, nil
	}
	globals.captureReturn(value)

	terminal, ok := globals.terminalValue()
	if !ok {
		return Outcome{
			OK: false,
			Failure: Failure{
				Code:    CodeUnresolvedFinal,
				Message: "workflow completed without a returned or final value",
			},
		}, nil
	}

	typed, err := typedValueFromGoja(vm, terminal)
	if err != nil {
		outcome := Outcome{
			OK: false,
			Failure: Failure{
				Code:    CodeScriptError,
				Message: err.Error(),
			},
		}
		outcome.Records = records.list()
		return outcome, nil
	}
	if validation := workflowresult.ValidateTypedValue(typed); validation.HasIssues() {
		outcome := invalidResultFailure(validation)
		outcome.Records = records.list()
		return outcome, nil
	}
	if hooks.OnResult != nil {
		if err := hooks.OnResult(typed); err != nil {
			return Outcome{}, err
		}
	}
	return Outcome{OK: true, Value: typed, Records: records.list()}, nil
}

func preExecutionOutcome(req Request) *Outcome {
	if strings.TrimSpace(req.Source) == "" {
		return &Outcome{OK: false, Failure: Failure{
			Code:    CodePreExecutionInvalid,
			Message: "workflow source is required",
		}}
	}
	if issue := validatePreExecution(req); issue != nil {
		outcome := preExecutionFailure(req, *issue)
		return &outcome
	}
	return nil
}

func validateRequestArgs(rawArgs, schema json.RawMessage) error {
	args, err := argsMapForRequest(rawArgs)
	if err != nil {
		return err
	}
	return workflowvalidation.ValidateArgs(schema, args)
}

func argsMapForRequest(raw json.RawMessage) (map[string]any, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil || args == nil {
		return nil, fmt.Errorf("workflow args must be a JSON object")
	}
	return args, nil
}

func argsValueForRequest(vm *goja.Runtime, raw json.RawMessage) (goja.Value, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return vm.ToValue(map[string]any{}), nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("workflow args must be JSON-compatible")
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("workflow args must be JSON-compatible: %w", err)
	}
	return vm.ToValue(decoded), nil
}

func wrapWorkflowSource(source string) string {
	return "(function(){\n" + source + "\n})()"
}

func childExecutorForRun(sessionID string, records *recordCollector, hooks Hooks, policy workflowpolicy.EffectivePolicy) ChildExecutor {
	if hooks.NewChildExecutor != nil {
		return hooks.NewChildExecutor(sessionID, childRecordSinkFromCollector(records), policy)
	}
	return NewFakeChildExecutor(sessionID, childRecordSinkFromCollector(records))
}

func childExecutorForRequest(sessionID string, records *recordCollector, hooks Hooks, resume *ResumeContext, policy workflowpolicy.EffectivePolicy) ChildExecutor {
	childExecutor := childExecutorForRun(sessionID, records, hooks, policy)
	if resume != nil {
		childExecutor = NewResumingChildExecutor(childExecutor, *resume)
	}
	return childExecutor
}

func watchContext(ctx context.Context, vm *goja.Runtime, done <-chan struct{}) {
	if ctx == nil {
		return
	}
	select {
	case <-ctx.Done():
		vm.Interrupt(ctx.Err().Error())
	case <-done:
	}
}

func contextTerminationOutcome(err error) Outcome {
	if errors.Is(err, context.DeadlineExceeded) {
		return timeoutOutcome(err)
	}
	return canceledOutcome(err)
}

func canceledOutcome(err error) Outcome {
	message := "workflow runtime canceled"
	if err != nil && !errors.Is(err, context.Canceled) {
		message = err.Error()
	}
	return Outcome{
		OK: false,
		Failure: Failure{
			Code:    CodeCanceled,
			Message: message,
		},
	}
}

func timeoutOutcome(err error) Outcome {
	message := "workflow runtime timed out"
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		message = err.Error()
	}
	return Outcome{
		OK: false,
		Failure: Failure{
			Code:    CodeTimeout,
			Message: message,
		},
	}
}

func scriptErrorOutcome(vm *goja.Runtime, err error) Outcome {
	message := err.Error()
	var interrupted *goja.InterruptedError
	if errors.As(err, &interrupted) {
		if strings.Contains(strings.ToLower(message), "context canceled") {
			return canceledOutcome(context.Canceled)
		}
		if strings.Contains(strings.ToLower(message), "deadline exceeded") {
			return timeoutOutcome(context.DeadlineExceeded)
		}
	}
	var exception *goja.Exception
	if errors.As(err, &exception) {
		if extracted := javascriptExceptionMessage(vm, exception); extracted != "" {
			message = extracted
		}
	}
	return Outcome{
		OK: false,
		Failure: Failure{
			Code:    CodeScriptError,
			Message: message,
		},
	}
}

func javascriptExceptionMessage(vm *goja.Runtime, exception *goja.Exception) string {
	value := exception.Value()
	if value == nil {
		return ""
	}
	if obj := value.ToObject(vm); obj != nil {
		if msg := obj.Get("message"); msg != nil && !goja.IsUndefined(msg) {
			if exported := msg.Export(); exported != nil {
				return fmt.Sprint(exported)
			}
		}
	}
	if exported := value.Export(); exported != nil {
		return fmt.Sprint(exported)
	}
	return ""
}

// RunWithTimeout executes one workflow with a bounded timeout derived from the context deadline.
func RunWithTimeout(parent context.Context, timeout time.Duration, req Request, hooks Hooks) (Outcome, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return Run(ctx, req, hooks)
}
