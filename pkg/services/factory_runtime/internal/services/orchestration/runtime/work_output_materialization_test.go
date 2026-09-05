package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

type countingMaterializationWorkService struct {
	work.Service
	calls atomic.Int32
}

func (service *countingMaterializationWorkService) MaterializeWorkerOutput(
	ctx context.Context,
	request work.MaterializeWorkerOutputRequest,
) (work.MaterializeWorkerOutputResult, error) {
	service.calls.Add(1)
	return service.Service.MaterializeWorkerOutput(ctx, request)
}

func TestAcceptWorkersResultMaterializesDetachedProposalOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	planner := dispatchplanningwire.New(func(context.Context, workers.WorkstationDispatchRequest) error {
		return nil
	}, nil)
	workService := &countingMaterializationWorkService{Service: testMaterializationService()}
	hook := newCanonicalDispatchPlanningResultHook(
		planner,
		buildSimpleNet(),
		buffers.NewTypedBuffer[workers.WorkResult](4),
		nil,
		workService,
		func() string { return "proposal-id" },
		"session-1",
	)
	dispatch := work.WorkDispatch{
		DispatchID:      "dispatch-proposal",
		TransitionID:    "t-process",
		WorkerType:      "mock",
		WorkstationName: "workstation-a",
		Execution: work.ExecutionMetadata{
			RequestID: "request-proposal",
			TraceID:   "trace-proposal",
			ReplayKey: "replay-proposal",
			WorkIDs:   []string{"source-work"},
		},
		InputTokens: workers.InputTokens(workers.Token{
			Color: workers.Color{
				WorkID:     "source-work",
				WorkTypeID: "task",
				DataType:   workers.DataTypeWork,
				TraceID:    "trace-proposal",
			},
		}),
	}
	if err := hook.SubmitDispatch(ctx, dispatch); err != nil {
		t.Fatalf("SubmitDispatch() error = %v", err)
	}
	intent, ok := planner.Intent(dispatch.DispatchID)
	if !ok {
		t.Fatalf("planner intent for %q is missing", dispatch.DispatchID)
	}
	request := intent.Action.Request
	proposedOutput := &workers.ProposedOutput{
		Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "materialized primary",
		}},
		ProposedWork: []workers.ProposedWork{{
			WorkTypeID: "task",
			Name:       "follow-up",
			State:      "done",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "follow-up body",
			}},
		}},
	}
	result := workers.WorkstationDispatchResult{
		DispatchID:      dispatch.DispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		Result: workers.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workers.OutcomeAccepted,
			Output:       "legacy output",
		},
		ProposedOutput: proposedOutput,
	}

	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			hook.acceptWorkersResult(ctx, request, result, nil)
		}()
	}
	wait.Wait()
	assertCanonicalProposal(t, workService, hook)
}

func assertCanonicalProposal(
	t *testing.T,
	workService *countingMaterializationWorkService,
	hook *dispatchPlanningResultHook,
) {
	t.Helper()
	if calls := workService.calls.Load(); calls != 1 {
		t.Fatalf("MaterializeWorkerOutput() calls = %d, want exactly one", calls)
	}
	if got := hook.resultBuffer.Len(); got != 1 {
		t.Fatalf("buffered canonical results = %d, want one", got)
	}
	canonical, ok := hook.resultBuffer.Read()
	if !ok {
		t.Fatal("buffered canonical result is missing")
	}
	if canonical.Output != "materialized primary" {
		t.Fatalf("canonical output = %q, want Work-materialized primary", canonical.Output)
	}
	if len(canonical.RecordedOutputWork) != 1 {
		t.Fatalf("canonical materialized Work = %#v, want one item", canonical.RecordedOutputWork)
	}
	if got := canonical.RecordedOutputWork[0].ID; got != "work-proposal-id" {
		t.Fatalf("canonical Work ID = %q, want Work-owned identity", got)
	}
}

func TestWorkMaterializationRejectionDoesNotPublishOutput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	planner := dispatchplanningwire.New(func(context.Context, workers.WorkstationDispatchRequest) error {
		return nil
	}, nil)
	workService := &countingMaterializationWorkService{Service: testMaterializationService()}
	hook := newCanonicalDispatchPlanningResultHook(
		planner,
		buildSimpleNet(),
		buffers.NewTypedBuffer[workers.WorkResult](4),
		nil,
		workService,
		func() string { return "proposal-rejected" },
		"session-rejected",
	)
	dispatch := work.WorkDispatch{
		DispatchID:      "dispatch-rejected",
		TransitionID:    "t-process",
		WorkerType:      "mock",
		WorkstationName: "workstation-a",
		Execution: work.ExecutionMetadata{
			RequestID: "request-rejected",
			TraceID:   "trace-rejected",
			ReplayKey: "replay-rejected",
			WorkIDs:   []string{"source-work"},
		},
		InputTokens: workers.InputTokens(workers.Token{
			Color: workers.Color{
				WorkID: "source-work", WorkTypeID: "task", DataType: workers.DataTypeWork,
				TraceID: "trace-rejected",
			},
		}),
	}
	if err := hook.SubmitDispatch(ctx, dispatch); err != nil {
		t.Fatalf("SubmitDispatch() error = %v", err)
	}
	intent, ok := planner.Intent(dispatch.DispatchID)
	if !ok {
		t.Fatalf("planner intent for %q is missing", dispatch.DispatchID)
	}
	result := workers.WorkstationDispatchResult{
		DispatchID:      dispatch.DispatchID,
		WorkstationName: intent.Action.Request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		Result: workers.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workers.OutcomeAccepted,
		},
		ProposedOutput: &workers.ProposedOutput{
			Primary: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "rejected output",
			}},
			ProposedWork: []workers.ProposedWork{{
				WorkTypeID: "task",
				Name:       "invalid-state",
				State:      "missing",
			}},
		},
	}
	hook.acceptWorkersResult(ctx, intent.Action.Request, result, nil)

	if calls := workService.calls.Load(); calls != 1 {
		t.Fatalf("MaterializeWorkerOutput() calls = %d, want exactly one", calls)
	}
	canonical, ok := hook.resultBuffer.Read()
	if !ok {
		t.Fatal("rejected canonical result is missing")
	}
	if canonical.Outcome != workers.OutcomeFailed ||
		!strings.Contains(canonical.Error, "worker output materialization") ||
		!strings.Contains(canonical.Error, work.ErrInvalidProposedWork.Error()) {
		t.Fatalf("canonical rejected result = %#v, want failed materialization result", canonical)
	}
	if canonical.Output != "" || len(canonical.OutputContent) != 0 || len(canonical.RecordedOutputWork) != 0 {
		t.Fatalf("rejected result published output: %#v", canonical)
	}
}

func TestCanceledAttemptDoesNotMaterializeCompletedOutput(t *testing.T) {
	ctx := context.Background()
	planner := dispatchplanningwire.New(func(context.Context, workers.WorkstationDispatchRequest) error {
		return nil
	}, nil)
	workService := &countingMaterializationWorkService{Service: testMaterializationService()}
	hook := newCanonicalDispatchPlanningResultHook(
		planner,
		buildSimpleNet(),
		buffers.NewTypedBuffer[workers.WorkResult](4),
		nil,
		workService,
		func() string { return "work-cancel-id" },
		"session-cancel",
	)
	dispatch := work.WorkDispatch{
		DispatchID: "dispatch-cancel-output", TransitionID: "t-process", WorkerType: "mock",
		WorkstationName: "workstation-a",
		Execution: work.ExecutionMetadata{
			RequestID: "request-cancel-output", TraceID: "trace-cancel-output", ReplayKey: "replay-cancel-output",
			WorkIDs: []string{"source-work"},
		},
		InputTokens: workers.InputTokens(workers.Token{Color: workers.Color{
			WorkID: "source-work", WorkTypeID: "task", DataType: workers.DataTypeWork,
			TraceID: "trace-cancel-output",
		}}),
	}
	if err := hook.SubmitDispatch(ctx, dispatch); err != nil {
		t.Fatalf("SubmitDispatch() error = %v", err)
	}
	intent, ok := planner.Intent(dispatch.DispatchID)
	if !ok {
		t.Fatalf("planner intent for %q is missing", dispatch.DispatchID)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	execute := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		close(started)
		<-release
		return workers.ExecuteResult{
			Correlation: request.Correlation,
			Outcome:     workers.ExecutionOutcomeAccepted,
			Output: workers.ProposedOutput{
				Primary:      []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "losing output"}},
				ProposedWork: []workers.ProposedWork{{WorkTypeID: "task", Name: "losing-follow-up"}},
			},
		}, nil
	})
	cfg := &runtimeConfig{
		executeService: execute,
		recordingID:    "runtime-cancel-output",
		eventHistory:   &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "generation-cancel-output"},
		clock:          testRuntimeClock{},
		newID:          func() string { return "attempt-cancel-output" },
		attempts:       newAttemptLifecycle(execute, func() string { return "attempt-cancel-output" }, 1),
		net:            buildSimpleNet(), workService: workService,
		workRequestIDs: func() string { return "work-cancel-id" },
	}
	callbackDone := make(chan struct{})
	if err := startThroughStatelessWorkers(ctx, cfg, intent.Action.Request, func(
		callbackCtx context.Context,
		request workers.WorkstationDispatchRequest,
		result workers.WorkstationDispatchResult,
		err error,
	) {
		hook.acceptWorkersResult(callbackCtx, request, result, err)
		close(callbackDone)
	}); err != nil {
		t.Fatalf("startThroughStatelessWorkers() error = %v", err)
	}
	<-started
	if _, err := cancelStatelessAttempt(ctx, cfg, workers.WorkstationDispatchCancelRequest{DispatchID: dispatch.DispatchID}); err != nil {
		t.Fatalf("cancelStatelessAttempt() error = %v", err)
	}
	close(release)
	<-callbackDone
	if calls := workService.calls.Load(); calls != 0 {
		t.Fatalf("MaterializeWorkerOutput() calls = %d, want zero for canceled output", calls)
	}
	canonical, ok := hook.resultBuffer.Read()
	if !ok {
		t.Fatal("canceled terminal result is missing")
	}
	if canonical.Output != "" || len(canonical.RecordedOutputWork) != 0 {
		t.Fatalf("canceled result carried downstream output: %#v", canonical)
	}
}

func TestValidStatesByTypeFromNetHandlesNilNet(t *testing.T) {
	t.Parallel()

	if got := validStatesByTypeFromNet(nil); got != nil {
		t.Fatalf("validStatesByTypeFromNet(nil) = %#v, want nil", got)
	}
}

func TestValidWorkTypesFromNetHandlesNilAndEmptyNet(t *testing.T) {
	t.Parallel()

	if got := validWorkTypesFromNet(nil); got != nil {
		t.Fatalf("validWorkTypesFromNet(nil) = %#v, want nil", got)
	}
	if got := validWorkTypesFromNet(&state.Net{}); got != nil {
		t.Fatalf("validWorkTypesFromNet(empty) = %#v, want nil", got)
	}
}

func TestPrimaryOutputTextSelectsFirstNonEmptySupportedPart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		parts []work.WorkContentPart
		want  string
	}{
		{name: "empty", want: ""},
		{
			name: "skips blank text and returns JSON",
			parts: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: "  "},
				{Type: work.WorkContentPartTypeJSON, JSON: json.RawMessage(`{"answer":42}`)},
			},
			want: `{"answer":42}`,
		},
		{
			name: "text",
			parts: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: "answer"},
				{Type: work.WorkContentPartTypeText, Text: "later"},
			},
			want: "answer",
		},
		{
			name:  "image URL",
			parts: []work.WorkContentPart{{Type: work.WorkContentPartTypeImage, URL: "file:///answer.png"}},
			want:  "file:///answer.png",
		},
		{
			name:  "image file fallback",
			parts: []work.WorkContentPart{{Type: work.WorkContentPartTypeImage, File: "answer.png"}},
			want:  "answer.png",
		},
		{
			name:  "audio URL",
			parts: []work.WorkContentPart{{Type: work.WorkContentPartTypeAudio, URL: "file:///answer.wav"}},
			want:  "file:///answer.wav",
		},
		{
			name:  "binary file fallback",
			parts: []work.WorkContentPart{{Type: work.WorkContentPartTypeBinary, File: "answer.bin"}},
			want:  "answer.bin",
		},
		{
			name:  "unsupported part",
			parts: []work.WorkContentPart{{Type: "unsupported", Text: "ignored"}},
			want:  "",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if got := primaryOutputText(test.parts); got != test.want {
				t.Fatalf("primaryOutputText() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRuntimeModelOperationContentTypeNormalizesSupportedKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		partType work.WorkContentPartType
		want     string
	}{
		{name: "text", partType: work.WorkContentPartTypeText, want: interfaces.ModelOperationContentTypeText},
		{name: "legacy text", partType: "TEXT", want: interfaces.ModelOperationContentTypeText},
		{name: "image", partType: work.WorkContentPartTypeImage, want: interfaces.ModelOperationContentTypeImage},
		{name: "legacy image", partType: "IMAGE", want: interfaces.ModelOperationContentTypeImage},
		{name: "audio", partType: work.WorkContentPartTypeAudio, want: interfaces.ModelOperationContentTypeAudio},
		{name: "JSON", partType: work.WorkContentPartTypeJSON, want: interfaces.ModelOperationContentTypeJSON},
		{name: "binary", partType: work.WorkContentPartTypeBinary, want: interfaces.ModelOperationContentTypeBinary},
		{name: "custom", partType: "custom", want: "custom"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeModelOperationContentType(work.WorkContentPart{Type: test.partType}); got != test.want {
				t.Fatalf("runtimeModelOperationContentType(%q) = %q, want %q", test.partType, got, test.want)
			}
		})
	}
}

func TestRuntimeModelOperationSelectorMatchingAndEmptiness(t *testing.T) {
	t.Parallel()

	part := work.WorkContentPart{
		Type:  work.WorkContentPartTypeText,
		Slot:  "prompt",
		Label: "question",
		Role:  "user",
	}
	matchCases := []struct {
		name     string
		selector *interfaces.ModelOperationBindingSelector
		want     bool
	}{
		{name: "nil", want: false},
		{name: "empty", selector: &interfaces.ModelOperationBindingSelector{}, want: true},
		{name: "slot match", selector: &interfaces.ModelOperationBindingSelector{Slot: " prompt "}, want: true},
		{name: "slot mismatch", selector: &interfaces.ModelOperationBindingSelector{Slot: "other"}, want: false},
		{name: "label match", selector: &interfaces.ModelOperationBindingSelector{Label: " question "}, want: true},
		{name: "label mismatch", selector: &interfaces.ModelOperationBindingSelector{Label: "other"}, want: false},
		{name: "type match", selector: &interfaces.ModelOperationBindingSelector{Type: interfaces.ModelOperationContentTypeText}, want: true},
		{name: "type mismatch", selector: &interfaces.ModelOperationBindingSelector{Type: interfaces.ModelOperationContentTypeJSON}, want: false},
		{name: "role match", selector: &interfaces.ModelOperationBindingSelector{Role: " user "}, want: true},
		{name: "role mismatch", selector: &interfaces.ModelOperationBindingSelector{Role: "assistant"}, want: false},
		{
			name:     "all fields match",
			selector: &interfaces.ModelOperationBindingSelector{Slot: "prompt", Label: "question", Type: interfaces.ModelOperationContentTypeText, Role: "user"},
			want:     true,
		},
	}
	for _, test := range matchCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeModelOperationSelectorMatches(part, test.selector); got != test.want {
				t.Fatalf("runtimeModelOperationSelectorMatches() = %v, want %v", got, test.want)
			}
		})
	}

	emptyCases := []struct {
		name     string
		selector *interfaces.ModelOperationBindingSelector
		want     bool
	}{
		{name: "nil", want: true},
		{name: "whitespace", selector: &interfaces.ModelOperationBindingSelector{Slot: " ", Label: "\t", Type: " ", Role: "\n"}, want: true},
		{name: "slot", selector: &interfaces.ModelOperationBindingSelector{Slot: "slot"}, want: false},
		{name: "label", selector: &interfaces.ModelOperationBindingSelector{Label: "label"}, want: false},
		{name: "type", selector: &interfaces.ModelOperationBindingSelector{Type: "TEXT"}, want: false},
		{name: "role", selector: &interfaces.ModelOperationBindingSelector{Role: "user"}, want: false},
	}
	for _, test := range emptyCases {
		test := test
		t.Run("empty/"+test.name, func(t *testing.T) {
			if got := runtimeModelOperationSelectorIsEmpty(test.selector); got != test.want {
				t.Fatalf("runtimeModelOperationSelectorIsEmpty() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestResolveRuntimeModelOperationBindingsUsesInputConfigDefaultAndOmittedSources(t *testing.T) {
	t.Parallel()

	configContent := []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, Text: "configured"}}
	defaultContent := []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "default"}}
	workstation := &interfaces.FactoryWorkstationConfig{
		Type:      interfaces.WorkstationTypeInference,
		Operation: "render",
		OperationBindings: []interfaces.ModelOperationBinding{
			{Slot: "prompt", Selector: &interfaces.ModelOperationBindingSelector{Label: "prompt", Type: interfaces.ModelOperationContentTypeText}},
			{Slot: "config", Selector: &interfaces.ModelOperationBindingSelector{Label: "missing"}, Config: configContent},
			{Slot: "default", Selector: &interfaces.ModelOperationBindingSelector{Label: "missing"}, DefaultContent: defaultContent},
			{Slot: "optional", Selector: &interfaces.ModelOperationBindingSelector{Label: "missing"}},
		},
	}
	worker := &interfaces.FactoryWorkerConfig{
		Operations: []interfaces.ModelOperation{{
			Name: "render",
			Inputs: []interfaces.ModelOperationSlot{
				{Name: "prompt", Required: true},
				{Name: "config"},
				{Name: "default"},
				{Name: "optional"},
				{Name: "implicit"},
			},
		}},
	}
	inputPrompt := work.WorkContentPart{Type: work.WorkContentPartTypeText, Slot: "prompt", Label: "prompt", Role: "user", Text: "input"}
	inputImplicit := work.WorkContentPart{Type: work.WorkContentPartTypeText, Slot: "implicit", Text: "implicit input"}
	resourcePrompt := work.WorkContentPart{Type: work.WorkContentPartTypeText, Slot: "prompt", Label: "prompt", Text: "resource must be ignored"}
	tokens := []workers.Token{
		{ID: "resource", Color: workers.Color{DataType: workers.DataTypeResource, Content: []work.WorkContentPart{resourcePrompt}}},
		{ID: "work", Color: workers.Color{DataType: workers.DataTypeWork, Content: []work.WorkContentPart{inputPrompt, inputImplicit}}},
	}

	got, err := resolveRuntimeModelOperationBindings(workstation, worker, tokens)
	if err != nil {
		t.Fatalf("resolveRuntimeModelOperationBindings() error = %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("resolved bindings = %#v, want five operation inputs", got)
	}
	wantSources := []workers.ModelOperationBindingSource{
		workers.ModelOperationBindingSourceInput,
		workers.ModelOperationBindingSourceConfig,
		workers.ModelOperationBindingSourceDefault,
		workers.ModelOperationBindingSourceOmitted,
		workers.ModelOperationBindingSourceInput,
	}
	for index, want := range wantSources {
		if got[index].Slot != worker.Operations[0].Inputs[index].Name || got[index].Source != want {
			t.Fatalf("resolved binding %d = %#v, want slot %q source %q", index, got[index], worker.Operations[0].Inputs[index].Name, want)
		}
	}
	if len(got[0].Content) != 1 || got[0].Content[0].Text != "input" {
		t.Fatalf("input binding = %#v, want input prompt content", got[0])
	}
	if len(got[1].Content) != 1 || got[1].Content[0].Text != "configured" {
		t.Fatalf("config binding = %#v, want configured content", got[1])
	}
	if len(got[2].Content) != 1 || got[2].Content[0].Text != "default" {
		t.Fatalf("default binding = %#v, want default content", got[2])
	}
	configContent[0].Text = "mutated"
	defaultContent[0].Text = "mutated"
	if got[1].Content[0].Text != "configured" || got[2].Content[0].Text != "default" {
		t.Fatalf("resolved configured/default content was not detached: %#v", got)
	}
}

func TestResolveRuntimeModelOperationBindingsRejectsMissingRequiredAndUnknownOperation(t *testing.T) {
	t.Parallel()

	worker := &interfaces.FactoryWorkerConfig{Operations: []interfaces.ModelOperation{{
		Name:   "known",
		Inputs: []interfaces.ModelOperationSlot{{Name: "required", Required: true}},
	}}}
	baseWorkstation := &interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeInference, Operation: "known"}
	if _, err := resolveRuntimeModelOperationBindings(baseWorkstation, worker, nil); err == nil || !strings.Contains(err.Error(), `required slot "required"`) {
		t.Fatalf("missing required input error = %v, want required-slot diagnostic", err)
	}
	if _, err := resolveRuntimeModelOperationBindings(
		&interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeInference, Operation: "unknown"}, worker, nil,
	); err == nil || !strings.Contains(err.Error(), `does not declare operation "unknown"`) {
		t.Fatalf("unknown operation error = %v, want declaration diagnostic", err)
	}

	noOpCases := []struct {
		name        string
		workstation *interfaces.FactoryWorkstationConfig
		worker      *interfaces.FactoryWorkerConfig
	}{
		{name: "nil workstation", worker: worker},
		{name: "nil worker", workstation: baseWorkstation},
		{name: "unsupported type", workstation: &interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeAgent, Operation: "known"}, worker: worker},
		{name: "empty operation", workstation: &interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeInference}, worker: worker},
	}
	for _, test := range noOpCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveRuntimeModelOperationBindings(test.workstation, test.worker, nil)
			if err != nil || got != nil {
				t.Fatalf("resolveRuntimeModelOperationBindings() = %#v, %v; want nil, nil", got, err)
			}
		})
	}
}

func TestRuntimeModelOperationByNameAndBindingResolution(t *testing.T) {
	t.Parallel()

	operation := interfaces.ModelOperation{Name: " render ", Inputs: []interfaces.ModelOperationSlot{{Name: "slot"}}}
	if got, ok := runtimeModelOperationByName([]interfaces.ModelOperation{operation}, "render"); !ok || got.Name != operation.Name {
		t.Fatalf("runtimeModelOperationByName() = %#v, %v; want matching operation", got, ok)
	}
	if _, ok := runtimeModelOperationByName([]interfaces.ModelOperation{operation}, "missing"); ok {
		t.Fatal("runtimeModelOperationByName() found an undeclared operation")
	}

	input := work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: "input"}
	config := work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: "config"}
	defaultValue := work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: "default"}
	cases := []struct {
		name    string
		binding interfaces.ModelOperationBinding
		want    workers.ModelOperationBindingSource
		text    string
	}{
		{name: "input", binding: interfaces.ModelOperationBinding{Selector: &interfaces.ModelOperationBindingSelector{Type: interfaces.ModelOperationContentTypeText}}, want: workers.ModelOperationBindingSourceInput, text: "input"},
		{name: "config", binding: interfaces.ModelOperationBinding{Selector: &interfaces.ModelOperationBindingSelector{Label: "missing"}, Config: []work.WorkContentPart{config}}, want: workers.ModelOperationBindingSourceConfig, text: "config"},
		{name: "default", binding: interfaces.ModelOperationBinding{Selector: &interfaces.ModelOperationBindingSelector{Label: "missing"}, DefaultContent: []work.WorkContentPart{defaultValue}}, want: workers.ModelOperationBindingSourceDefault, text: "default"},
		{name: "omitted", binding: interfaces.ModelOperationBinding{Selector: &interfaces.ModelOperationBindingSelector{Label: "missing"}}, want: workers.ModelOperationBindingSourceOmitted},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got := resolveRuntimeModelOperationBinding(test.binding, []workers.Token{{Color: workers.Color{Content: []work.WorkContentPart{input}}}})
			if got.Source != test.want || (test.text == "" && len(got.Content) != 0) || (test.text != "" && (len(got.Content) != 1 || got.Content[0].Text != test.text)) {
				t.Fatalf("resolved binding = %#v, want source %q and text %q", got, test.want, test.text)
			}
		})
	}
}

func TestRuntimeNonResourceTokensDropsCapacityInputs(t *testing.T) {
	t.Parallel()

	if got := runtimeNonResourceTokens(nil); got != nil {
		t.Fatalf("runtimeNonResourceTokens(nil) = %#v, want nil", got)
	}
	if got := runtimeNonResourceTokens([]workers.Token{}); got != nil {
		t.Fatalf("runtimeNonResourceTokens(empty) = %#v, want nil", got)
	}
	input := workers.Token{ID: "input", Color: workers.Color{DataType: workers.DataTypeWork}}
	resource := workers.Token{ID: "resource", Color: workers.Color{DataType: workers.DataTypeResource}}
	got := runtimeNonResourceTokens([]workers.Token{resource, input})
	if len(got) != 1 || got[0].ID != "input" {
		t.Fatalf("runtimeNonResourceTokens() = %#v, want only work token", got)
	}
}

func TestDetachedExecutionTickPrefersCurrentDispatchTick(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata work.ExecutionMetadata
		want     int
	}{
		{name: "current tick", metadata: work.ExecutionMetadata{CurrentTick: 7, DispatchCreatedTick: 3}, want: 7},
		{name: "created tick fallback", metadata: work.ExecutionMetadata{DispatchCreatedTick: 3}, want: 3},
		{name: "no tick", want: 0},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if got := detachedExecutionTick(test.metadata); got != test.want {
				t.Fatalf("detachedExecutionTick() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestAppendTranscriptEntryOmitsBlankAndBoundsLongContent(t *testing.T) {
	t.Parallel()

	var transcript []workers.AgentRunTranscriptEntry
	appendTranscriptEntry(&transcript, "system", "  ")
	if len(transcript) != 0 {
		t.Fatalf("blank transcript = %#v, want empty", transcript)
	}
	appendTranscriptEntry(&transcript, "user", "hello")
	long := strings.Repeat("x", detachedAgentRunTranscriptSummaryLimit+1)
	appendTranscriptEntry(&transcript, "assistant", long)
	if len(transcript) != 2 || transcript[0].Role != "user" || transcript[0].Summary != "hello" {
		t.Fatalf("transcript = %#v, want user entry followed by assistant entry", transcript)
	}
	wantLong := strings.Repeat("x", detachedAgentRunTranscriptSummaryLimit) + "..."
	if transcript[1].Role != "assistant" || transcript[1].Summary != wantLong {
		t.Fatalf("bounded transcript entry = %#v, want %d-character summary plus suffix", transcript[1], detachedAgentRunTranscriptSummaryLimit)
	}
}

func TestRecordDetachedAgentRunResponsePreservesSafeDiagnosticsAndTranscript(t *testing.T) {
	t.Parallel()

	ledger := &agentRunRecordingLedger{
		ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{},
	}
	cfg := &runtimeConfig{
		eventHistory: ledger,
		clock:        testRuntimeClock{},
		runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"agent": {Name: "agent", Type: interfaces.WorkstationTypeAgent},
			},
			Workers: map[string]*interfaces.FactoryWorkerConfig{},
		},
	}
	request := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{DispatchID: "dispatch-1"},
		Input: workers.ExecutionInput{Dispatch: work.WorkDispatch{
			Execution: work.ExecutionMetadata{CurrentTick: 9},
		}},
		Target: workers.ExecutionTarget{
			WorkstationName: "agent",
			Prompt: workers.PromptPolicy{
				SystemPrompt: "Reasoning effort: low",
				UserMessage:  "run the task",
			},
		},
	}
	result := workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
		Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "completed",
		}}},
		Diagnostics: &workers.SafeDiagnostics{Provider: &workers.SafeProviderDiagnostic{
			Provider: "codex",
		}},
		Metrics: workers.ExecutionMetrics{Duration: 1250 * time.Millisecond},
	}

	recordDetachedAgentRunResponse(cfg, request, result, nil)

	event := ledger.event
	if event.ID != "factory-event/agent-run-response/dispatch-1" ||
		event.DispatchID != "dispatch-1" ||
		event.Tick != 9 ||
		event.Payload.AgentRunID != "dispatch-1/agent-run/1" ||
		event.Payload.Outcome != string(workers.ExecutionOutcomeAccepted) ||
		event.Payload.DurationMillis != 1250 {
		t.Fatalf("recorded event = %#v, want detached agent-run identity and duration", event)
	}
	diagnostics, err := workers.SafeWorkDiagnosticsFromEventPayload(event.Payload.Diagnostics)
	if err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	if diagnostics.Provider == nil || diagnostics.Provider.Provider != "codex" {
		t.Fatalf("provider diagnostics = %#v, want codex", diagnostics.Provider)
	}
	if diagnostics.AgentRun == nil || len(diagnostics.AgentRun.Transcript) != 3 {
		t.Fatalf("agent-run diagnostics = %#v, want system/user/assistant transcript", diagnostics.AgentRun)
	}
	if diagnostics.AgentRun.Transcript[0].Summary != "Reasoning effort: low" ||
		diagnostics.AgentRun.Transcript[2].Summary != "completed" {
		t.Fatalf("transcript = %#v, want interpolated prompt and output", diagnostics.AgentRun.Transcript)
	}
}

func testMaterializationService() work.Service {
	return testRuntimeWorkService{}
}
