package factory_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

type rootOnlyPeer struct {
	err error
}

var _ factory.Service = (*rootOnlyPeer)(nil)

func (p *rootOnlyPeer) ControlPause(context.Context, factory.PauseRequest) (factory.PauseResult, error) {
	return factory.PauseResult{Outcome: factory.ControlOutcomeAccepted}, p.err
}
func (p *rootOnlyPeer) ControlResume(context.Context, factory.ResumeRequest) (factory.ResumeResult, error) {
	return factory.ResumeResult{Outcome: factory.ControlOutcomeAccepted}, p.err
}
func (p *rootOnlyPeer) ControlTerminate(context.Context, factory.TerminateRequest) (factory.TerminateResult, error) {
	return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, p.err
}
func (*rootOnlyPeer) ControlWaitToComplete(factory.WaitToCompleteRequest) factory.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factory.WaitToCompleteResult{Done: done}
}
func (p *rootOnlyPeer) ControlMoveWork(_ context.Context, req factory.MoveWorkRequest) (factory.MoveWorkResult, error) {
	return factory.MoveWorkResult{WorkID: req.WorkID, ToState: req.StateName}, p.err
}
func (p *rootOnlyPeer) Observe(context.Context, factory.ObserveRequest) (factory.ObserveResult, error) {
	return factory.ObserveResult{Observation: factory.Observation{Status: factory.ObservationStatusActive}}, p.err
}
func (p *rootOnlyPeer) CleanInvocationSnapshot(context.Context) (factory.CleanInvocationSnapshot, error) {
	return factory.CleanInvocationSnapshot{}, p.err
}
func (p *rootOnlyPeer) PlanDispatch(_ context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	return factory.PlanDispatchResult{Outcome: factory.DispatchPlanOutcomeAccepted, DispatchID: req.DispatchID}, p.err
}
func (p *rootOnlyPeer) InvokeWorker(_ context.Context, _ factory.InvokeWorkerRequest) (factory.InvokeWorkerResult, error) {
	return factory.InvokeWorkerResult{}, p.err
}

func (p *rootOnlyPeer) AcceptDispatchResult(_ context.Context, req factory.AcceptDispatchResultRequest) (factory.AcceptDispatchResultResult, error) {
	return factory.AcceptDispatchResultResult{Outcome: factory.DispatchPlanOutcomeRetired, DispatchID: req.DispatchID}, p.err
}
func TestRootService_RootOnlyPeerReachesEverySlice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var runtime factory.Service = &rootOnlyPeer{}

	assertRootControlSlice(t, ctx, runtime)
	assertRootObservationSlice(t, ctx, runtime)
	assertRootDispatchSlice(t, ctx, runtime)
}

func assertRootControlSlice(t *testing.T, ctx context.Context, runtime factory.Service) {
	t.Helper()
	if got, err := runtime.ControlPause(ctx, factory.PauseRequest{}); err != nil || got.Outcome != factory.ControlOutcomeAccepted {
		t.Fatalf("ControlPause() = (%#v, %v)", got, err)
	}
	if got, err := runtime.ControlMoveWork(ctx, factory.MoveWorkRequest{WorkID: "work-1", StateName: "done"}); err != nil || got.WorkID != "work-1" {
		t.Fatalf("ControlMoveWork() = (%#v, %v)", got, err)
	}
}

func assertRootObservationSlice(t *testing.T, ctx context.Context, runtime factory.Service) {
	t.Helper()
	if got, err := runtime.Observe(ctx, factory.ObserveRequest{}); err != nil || got.Observation.Status != factory.ObservationStatusActive {
		t.Fatalf("Observe() = (%#v, %v)", got, err)
	}
}

func assertRootDispatchSlice(t *testing.T, ctx context.Context, runtime factory.Service) {
	t.Helper()
	if got, err := runtime.PlanDispatch(ctx, factory.PlanDispatchRequest{DispatchID: "dispatch-1"}); err != nil || got.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch() = (%#v, %v)", got, err)
	}
	if got, err := runtime.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{DispatchID: "dispatch-1"}); err != nil || got.Outcome != factory.DispatchPlanOutcomeRetired {
		t.Fatalf("AcceptDispatchResult() = (%#v, %v)", got, err)
	}
}

func TestRootService_RootOnlyPeerReturnsTypedFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want error
		call func(factory.Service) error
	}{
		{"control", factory.ErrNotRunning, func(runtime factory.Service) error {
			_, err := runtime.ControlPause(context.Background(), factory.PauseRequest{})
			return err
		}},
		{"observation", factory.ErrInvalidObservationScope, func(runtime factory.Service) error {
			_, err := runtime.Observe(context.Background(), factory.ObserveRequest{})
			return err
		}},
		{"dispatch", factory.ErrUnknownDispatchCorrelation, func(runtime factory.Service) error {
			_, err := runtime.AcceptDispatchResult(context.Background(), factory.AcceptDispatchResultRequest{})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(&rootOnlyPeer{err: test.want}); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestNormalize_AcceptsAndTrimsCanonicalFields(t *testing.T) {
	t.Parallel()
	got, err := factory.NormalizeJavaScriptChild(map[string]any{
		"prompt":           "  review this  ",
		"label":            "  reviewer  ",
		"preset":           "  careful  ",
		"executorProvider": "  cursor-acp  ",
		"modelProvider":    "  codex  ",
		"model":            "  gpt-test  ",
		"reasoningEffort":  "  high  ",
		"resourceId":       "  reviewers  ",
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"answer": map[string]any{"type": "string"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	want := factory.JavaScriptChildSpec{
		Prompt: "review this", Label: "reviewer", Preset: "careful", ExecutorProvider: "cursor-acp",
		ModelProvider: "codex", Model: "gpt-test", ReasoningEffort: "high", ResourceID: "reviewers",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"answer": map[string]any{"type": "string"},
			},
		},
		Permissions: factory.JavaScriptChildPermissionDefault,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() = %#v, want %#v", got, want)
	}
}

func TestNormalize_ClonesValidSchemaWithoutSharingNestedState(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"answer": map[string]any{"type": "string"},
		},
	}
	got, err := factory.NormalizeJavaScriptChild(map[string]any{
		"prompt": "review",
		"schema": schema,
	})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if !reflect.DeepEqual(got.Schema, schema) {
		t.Fatalf("Normalize() schema = %#v, want %#v", got.Schema, schema)
	}

	gotProperties := got.Schema["properties"].(map[string]any)
	gotProperties["answer"].(map[string]any)["type"] = "number"
	if schema["properties"].(map[string]any)["answer"].(map[string]any)["type"] != "string" {
		t.Fatal("normalized schema mutation changed caller-owned schema")
	}
	schema["properties"].(map[string]any)["answer"].(map[string]any)["type"] = "boolean"
	if gotProperties["answer"].(map[string]any)["type"] != "number" {
		t.Fatal("caller schema mutation changed normalized schema")
	}
}

func TestNormalize_RejectsInvalidSchemasWithSafeDiagnostics(t *testing.T) {
	t.Parallel()
	const promptSecret = "prompt-secret"
	const schemaSecret = "schema-secret"
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{name: "non-object", value: schemaSecret, want: `agent.run() requires "schema" to be an object`},
		{name: "malformed-json-value", value: map[string]any{"type": func() {}}, want: `agent.run() requires "schema" to be a valid JSON Schema object`},
		{name: "invalid-schema-keyword", value: map[string]any{"type": "not-a-schema-type"}, want: `agent.run() requires "schema" to be a valid JSON Schema object`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := factory.NormalizeJavaScriptChild(map[string]any{
				"prompt": promptSecret,
				"schema": test.value,
			})
			if err == nil || err.Error() != test.want {
				t.Fatalf("Normalize() error = %v, want %q", err, test.want)
			}
			if strings.Contains(err.Error(), promptSecret) || strings.Contains(err.Error(), schemaSecret) {
				t.Fatalf("Normalize() error = %q, want safe schema diagnostic", err)
			}
		})
	}
}

func TestNormalize_AcceptsOnlyCanonicalPermissionValues(t *testing.T) {
	t.Parallel()

	for _, permission := range []factory.JavaScriptChildPermission{
		factory.JavaScriptChildPermissionDefault,
		factory.JavaScriptChildPermissionSkipPermissions,
	} {
		got, err := factory.NormalizeJavaScriptChild(map[string]any{
			"prompt":      "review",
			"permissions": string(permission),
		})
		if err != nil {
			t.Fatalf("Normalize(%q) error = %v", permission, err)
		}
		if got.Permissions != permission {
			t.Fatalf("Normalize(%q).Permissions = %q, want %q", permission, got.Permissions, permission)
		}
	}

	for _, value := range []any{"READ_ONLY", true, 1} {
		_, err := factory.NormalizeJavaScriptChild(map[string]any{
			"prompt":      "review",
			"permissions": value,
		})
		if err == nil || !strings.Contains(err.Error(), `"permissions"`) {
			t.Fatalf("Normalize(%#v) error = %v, want permissions diagnostic", value, err)
		}
	}
}

func TestNormalize_RejectsRetiredPermissionFieldWithReplacement(t *testing.T) {
	t.Parallel()

	retiredField := "skip" + "Permissions"
	_, err := factory.NormalizeJavaScriptChild(map[string]any{
		"prompt":     "review",
		retiredField: true,
	})
	if err == nil || !strings.Contains(err.Error(), `field "`+retiredField+`"`) ||
		!strings.Contains(err.Error(), `use "permissions"`) {
		t.Fatalf("Normalize() error = %v, want actionable permissions replacement", err)
	}
}

func TestNormalize_RejectsInvalidPermissionsValues(t *testing.T) {
	t.Parallel()

	for _, value := range []any{"READ_ONLY", true, 42} {
		_, err := factory.NormalizeJavaScriptChild(map[string]any{"prompt": "review", "permissions": value})
		if err == nil || !strings.Contains(err.Error(), `"permissions"`) {
			t.Fatalf("Normalize(%#v) error = %v, want field-specific permissions diagnostic", value, err)
		}
	}
}

func TestNormalize_RejectsInvalidSupportedFieldValues(t *testing.T) {
	t.Parallel()
	for _, field := range factory.JavaScriptChildSupportedFields() {
		t.Run(field, func(t *testing.T) {
			value := map[string]any{"prompt": "review", field: 42}
			_, err := factory.NormalizeJavaScriptChild(value)
			if err == nil || !strings.Contains(err.Error(), `"`+field+`"`) {
				t.Fatalf("Normalize() error = %v, want field-specific string error", err)
			}
		})
	}
}

func TestNormalize_RequiresUsablePrompt(t *testing.T) {
	t.Parallel()
	for _, value := range []map[string]any{{}, {"prompt": "   "}} {
		if _, err := factory.NormalizeJavaScriptChild(value); err == nil {
			t.Fatalf("Normalize(%#v) error = nil, want unusable prompt error", value)
		}
	}
}

func TestNormalize_RejectsUnsupportedFieldsWithoutExposingValues(t *testing.T) {
	t.Parallel()
	unsupported := []string{
		"writableRoots", "allowNetwork", "network", "allowDangerFullAccess", "dangerFullAccess",
		"outputSchema", "concurrency", "maxAgents", "duration", "timeout", "timeoutMs",
	}
	const secret = "secret-value-that-must-not-appear"
	for _, field := range unsupported {
		t.Run(field, func(t *testing.T) {
			_, err := factory.NormalizeJavaScriptChild(map[string]any{
				"prompt": "prompt-that-must-not-appear",
				field:    secret,
			})
			if err == nil {
				t.Fatal("Normalize() error = nil, want unsupported-field error")
			}
			want := `agent.run() does not support field "` + field + `"`
			if err.Error() != want {
				t.Fatalf("Normalize() error = %q, want %q", err, want)
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "prompt-that-must-not-appear") {
				t.Fatalf("Normalize() error = %q, want redacted diagnostic", err)
			}
		})
	}
}
