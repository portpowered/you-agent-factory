package work

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestInvocationPolicyServiceRejectsUnsupportedSlices(t *testing.T) {
	t.Parallel()

	service := NewInvocationPolicyService()
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "admission",
			call: func() error {
				_, err := service.SubmitWorkRequestForSession(ctx, "session-1", WorkRequest{})
				return err
			},
			want: "does not support admission",
		},
		{
			name: "admission prep",
			call: func() error {
				_, err := service.PrepareWorkRequest(ctx, WorkRequestPreparation{})
				return err
			},
			want: "does not support admission prep",
		},
		{
			name: "state access move",
			call: func() error {
				_, err := service.MoveWorkForSession(ctx, "session-1", "work-1", "done", "move-1")
				return err
			},
			want: "does not support state access",
		},
		{
			name: "state access list",
			call: func() error {
				_, err := service.ListWork(ctx, "session-1", ListOptions{})
				return err
			},
			want: "does not support state access",
		},
		{
			name: "state access get",
			call: func() error {
				_, err := service.GetWork(ctx, "session-1", "work-1")
				return err
			},
			want: "does not support state access",
		},
		{
			name: "state access move and read",
			call: func() error {
				_, err := service.MoveWorkAndRead(ctx, "session-1", "work-1", "done", "move-1")
				return err
			},
			want: "does not support state access",
		},
		{
			name: "content stage",
			call: func() error {
				_, err := service.StageContent(ctx, StageContentRequest{})
				return err
			},
			want: "does not support content staging",
		},
		{
			name: "content prepare",
			call: func() error {
				_, err := service.PrepareContent(ctx, nil)
				return err
			},
			want: "does not support content staging",
		},
		{
			name: "content resolve",
			call: func() error {
				_, err := service.ResolveContent(ctx, "ref")
				return err
			},
			want: "does not support content staging",
		},
		{
			name: "content cleanup",
			call: func() error {
				return service.CleanupContent(ctx, "ref")
			},
			want: "does not support content staging",
		},
		{
			name: "content materialize",
			call: func() error {
				_, _, err := service.MaterializeContentURL(ctx, "file:///x")
				return err
			},
			want: "does not support content materialization",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.call(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestInvocationPolicyServicePrepareInvocationInputAcceptsPositionalText(t *testing.T) {
	t.Parallel()

	prepared, err := NewInvocationPolicyService().PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{
		Arguments: []string{"hello"},
	})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != "hello" {
		t.Fatalf("prepared = %#v, want positional hello text", prepared)
	}
}

func TestInvocationPolicyServiceResolvePrimaryResultAllowsNilContext(t *testing.T) {
	t.Parallel()

	_, err := NewInvocationPolicyService().ResolvePrimaryResult(nil, PrimaryResultSelectionInput{
		RequestID: "request-1",
		InvocationReturn: &InvocationReturnConfig{Policy: "NOT_A_POLICY"},
		WorldState: InvocationWorldState{
			WorkRequestsByID: map[string]InvocationWorkRequest{
				"request-1": {WorkItems: []FactoryWorkItem{{ID: "work-1"}}},
			},
		},
	})
	if !errors.Is(err, ErrUnsupportedReturnPolicy) {
		t.Fatalf("error = %v, want ErrUnsupportedReturnPolicy", err)
	}
}

func TestInvocationPolicyServicePrepareInvocationInputPropagatesCanceledContext(t *testing.T) {
	t.Parallel()

	service := NewInvocationPolicyService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.PrepareInvocationInput(ctx, InvocationInputPreparationRequest{
		Arguments: []string{"hello"},
	})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestInvocationPolicyServicePrepareInvocationInputWrapsTypedFailures(t *testing.T) {
	t.Parallel()

	service := NewInvocationPolicyService()
	_, err := service.PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{
		Arguments: []string{"   "},
	})
	if err == nil || !errors.Is(err, ErrInvalidInvocationInput) {
		t.Fatalf("error = %v, want ErrInvalidInvocationInput", err)
	}
}

func TestInvocationPolicyServiceResolvePrimaryResultHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	service := NewInvocationPolicyService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.ResolvePrimaryResult(ctx, PrimaryResultSelectionInput{})
	if err == nil {
		t.Fatal("ResolvePrimaryResult() = nil, want context error")
	}
}
