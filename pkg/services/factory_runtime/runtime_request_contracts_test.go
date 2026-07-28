package factory

import (
	"strings"
	"testing"
)

func TestObservationScopeRequestPublishesRuntimeRootVocabulary(t *testing.T) {
	t.Parallel()

	request := ObservationScopeRequest(ObservationScopeProgress)
	if request.Scope != ObservationScopeProgress {
		t.Fatalf("scope = %q, want PROGRESS", request.Scope)
	}

	full := ObservationScopeRequest("")
	if full.Scope != "" {
		t.Fatalf("empty scope request = %#v, want zero scope for FULL default", full)
	}
}

func TestPauseControlRequestPublishesRuntimeRootVocabulary(t *testing.T) {
	t.Parallel()

	request := PauseControlRequest()
	if request != (PauseRequest{}) {
		t.Fatalf("pause request = %#v, want empty PauseRequest", request)
	}
}

func TestPlanDispatchRequestFromIntentPublishesRuntimeRootVocabulary(t *testing.T) {
	t.Parallel()

	request, err := PlanDispatchRequestFromIntent(PlanDispatchIntent{
		DispatchID:      "dispatch-runtime-root",
		CorrelationID:   "corr-runtime-root",
		WorkIDs:         []string{"work-runtime-root"},
		WorkstationName: "review",
		WorkerType:      "inference",
		ReplayKey:       "review/trace-1/work-runtime-root",
	})
	if err != nil {
		t.Fatalf("PlanDispatchRequestFromIntent: %v", err)
	}
	if request.DispatchID != "dispatch-runtime-root" {
		t.Fatalf("dispatch id = %q, want dispatch-runtime-root", request.DispatchID)
	}
	if request.CorrelationID != "corr-runtime-root" {
		t.Fatalf("correlation id = %q, want corr-runtime-root", request.CorrelationID)
	}
	if len(request.WorkIDs) != 1 || request.WorkIDs[0] != "work-runtime-root" {
		t.Fatalf("work ids = %#v, want [work-runtime-root]", request.WorkIDs)
	}
	if request.WorkstationName != "review" {
		t.Fatalf("workstation name = %q, want review", request.WorkstationName)
	}
	if request.WorkerType != "inference" {
		t.Fatalf("worker type = %q, want inference", request.WorkerType)
	}
	if request.ReplayKey != "review/trace-1/work-runtime-root" {
		t.Fatalf("replay key = %q, want review/trace-1/work-runtime-root", request.ReplayKey)
	}
}

func TestPlanDispatchRequestFromIntentRejectsMissingFields(t *testing.T) {
	t.Parallel()

	valid := PlanDispatchIntent{
		DispatchID:      "dispatch-runtime-root",
		CorrelationID:   "corr-runtime-root",
		WorkIDs:         []string{"work-runtime-root"},
		WorkstationName: "review",
		WorkerType:      "inference",
		ReplayKey:       "review/trace-1/work-runtime-root",
	}

	cases := []struct {
		name   string
		intent PlanDispatchIntent
		want   string
	}{
		{
			name:   "missing dispatch id",
			intent: withPlanDispatchIntent(valid, func(intent *PlanDispatchIntent) { intent.DispatchID = "" }),
			want:   "dispatch id is required",
		},
		{
			name:   "missing correlation id",
			intent: withPlanDispatchIntent(valid, func(intent *PlanDispatchIntent) { intent.CorrelationID = "" }),
			want:   "correlation id is required",
		},
		{
			name:   "missing workstation name",
			intent: withPlanDispatchIntent(valid, func(intent *PlanDispatchIntent) { intent.WorkstationName = "" }),
			want:   "workstation name is required",
		},
		{
			name:   "missing worker type",
			intent: withPlanDispatchIntent(valid, func(intent *PlanDispatchIntent) { intent.WorkerType = "" }),
			want:   "worker type is required",
		},
		{
			name:   "missing replay key",
			intent: withPlanDispatchIntent(valid, func(intent *PlanDispatchIntent) { intent.ReplayKey = "" }),
			want:   "replay key is required",
		},
		{
			name:   "missing work ids",
			intent: withPlanDispatchIntent(valid, func(intent *PlanDispatchIntent) { intent.WorkIDs = nil }),
			want:   "work ids must contain at least one Work identifier",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := PlanDispatchRequestFromIntent(tc.intent)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func withPlanDispatchIntent(
	intent PlanDispatchIntent,
	mutate func(*PlanDispatchIntent),
) PlanDispatchIntent {
	clone := intent
	clone.WorkIDs = append([]string(nil), intent.WorkIDs...)
	mutate(&clone)
	return clone
}
