package automations_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

func TestServiceCancellation_FakeReconcileReturnsTypedCancelledObservation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var svc automations.Service = &fakeRootService{ready: true}
	result, err := svc.Reconcile(ctx, automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{{
			AutomationID: "auto-cancel",
			SourceID:     "source-cancel",
			Kind:         "schedule",
			State:        automations.DesiredLifecycleRunning,
		}},
	})

	assertTypedAutomationsError(
		t, "Reconcile", err, automations.ErrorCodeCancelled, context.Canceled,
	)
	if len(result.Outcomes) != 1 {
		t.Fatalf("Reconcile() outcomes len = %d, want 1", len(result.Outcomes))
	}
	outcome := result.Outcomes[0]
	if outcome.Observed != automations.ObservedLifecycleCancelled ||
		outcome.Convergence != automations.ConvergenceStatusCancelled {
		t.Fatalf("Reconcile() outcome = %+v, want cancelled observation", outcome)
	}
	if outcome.Convergence == automations.ConvergenceStatusConverged {
		t.Fatal("cancelled Reconcile() reported converged")
	}
}

func TestServiceCancellation_FakeLifecycleDoesNotConvergeOrMutateSource(t *testing.T) {
	t.Parallel()

	identity := automations.SourceIdentity{
		AutomationID: "auto-cancel",
		SourceID:     "source-cancel",
	}
	fake := &fakeRootService{
		ready: true,
		sources: map[string]automations.SourceObservation{
			sourceKey(identity): {
				Identity:   identity,
				InstanceID: "instance:auto-cancel:source-cancel",
				State:      automations.ObservedLifecycleRunning,
				Cursor:     "cursor-before-cancel",
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := fake.StopSource(ctx, automations.StopSourceRequest{Identity: identity})

	assertTypedAutomationsError(
		t, "StopSource", err, automations.ErrorCodeCancelled, context.Canceled,
	)
	if result.Outcome.Observation.State != automations.ObservedLifecycleCancelled ||
		result.Outcome.Convergence != automations.ConvergenceStatusCancelled ||
		result.Outcome.Idempotent {
		t.Fatalf("StopSource() outcome = %+v, want non-idempotent cancellation", result.Outcome)
	}
	status, statusErr := fake.SourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: identity,
	})
	if statusErr != nil {
		t.Fatalf("SourceStatus() unexpected error: %v", statusErr)
	}
	if status.Observation.State != automations.ObservedLifecycleRunning ||
		status.Observation.Cursor != "cursor-before-cancel" {
		t.Fatalf("SourceStatus() observation = %+v, want unchanged running source", status.Observation)
	}
}

func TestGeneratedWorkRequestOutcomes_FakeAcceptedRejectedAndDuplicate(t *testing.T) {
	t.Parallel()

	fake := &fakeRootService{ready: true}
	acceptedRequest := generatedRequest("request-accepted", []byte(`{"work":"one"}`), "memory://one")
	accepted := fake.admitGeneratedWorkRequest(acceptedRequest, "")
	assertWorkRequestOutcome(t, accepted, automations.WorkRequestAdmissionAccepted, "")

	rejectedRequest := generatedRequest("request-rejected", nil, "")
	rejected := fake.admitGeneratedWorkRequest(
		rejectedRequest,
		automations.WorkRequestRejectedInvalidPayload,
	)
	assertWorkRequestOutcome(
		t, rejected, automations.WorkRequestAdmissionRejected,
		automations.WorkRequestRejectedInvalidPayload,
	)

	duplicateRequest := generatedRequest(
		"request-duplicate",
		[]byte(`{"work":"one"}`),
		"memory://one",
	)
	duplicate := fake.admitGeneratedWorkRequest(duplicateRequest, "")
	assertWorkRequestOutcome(t, duplicate, automations.WorkRequestAdmissionDuplicate, "")
	if duplicate.OriginalRequestID != acceptedRequest.Identity.RequestID {
		t.Fatalf(
			"duplicate OriginalRequestID = %q, want %q",
			duplicate.OriginalRequestID,
			acceptedRequest.Identity.RequestID,
		)
	}
	if fake.emissionCount != 1 {
		t.Fatalf("logical emission count = %d, want 1", fake.emissionCount)
	}

	var svc automations.Service = fake
	reconciled, err := svc.Reconcile(context.Background(), generatedWorkReconcileRequest())
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}
	if len(reconciled.GeneratedWorkRequests) != 3 {
		t.Fatalf(
			"Reconcile() generated outcomes len = %d, want 3",
			len(reconciled.GeneratedWorkRequests),
		)
	}
	assertWorkRequestOutcome(
		t,
		reconciled.GeneratedWorkRequests[0],
		automations.WorkRequestAdmissionAccepted,
		"",
	)
	assertWorkRequestOutcome(
		t,
		reconciled.GeneratedWorkRequests[1],
		automations.WorkRequestAdmissionRejected,
		automations.WorkRequestRejectedInvalidPayload,
	)
	assertWorkRequestOutcome(
		t,
		reconciled.GeneratedWorkRequests[2],
		automations.WorkRequestAdmissionDuplicate,
		"",
	)
}

func TestGeneratedWorkRequestOutcome_FakeReturnsDetachedPayload(t *testing.T) {
	t.Parallel()

	fake := &fakeRootService{ready: true}
	payload := []byte(`{"work":"detached"}`)
	request := generatedRequest("request-detached", payload, "memory://detached")
	fake.admitGeneratedWorkRequest(request, "")
	payload[0] = '!'

	var svc automations.Service = fake
	result, err := svc.Reconcile(context.Background(), generatedWorkReconcileRequest())
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}
	outcome := result.GeneratedWorkRequests[0]
	if !bytes.Equal(outcome.Request.Payload, []byte(`{"work":"detached"}`)) {
		t.Fatalf("outcome payload = %q, want detached copy", outcome.Request.Payload)
	}
	outcome.Request.Payload[0] = '!'
	repeated, err := svc.Reconcile(context.Background(), generatedWorkReconcileRequest())
	if err != nil {
		t.Fatalf("repeated Reconcile() unexpected error: %v", err)
	}
	if !bytes.Equal(
		repeated.GeneratedWorkRequests[0].Request.Payload,
		[]byte(`{"work":"detached"}`),
	) {
		t.Fatalf(
			"repeated outcome payload = %q, want service-owned value unchanged",
			repeated.GeneratedWorkRequests[0].Request.Payload,
		)
	}
}

func (f *fakeRootService) admitGeneratedWorkRequest(
	request automations.GeneratedWorkRequest,
	rejection automations.WorkRequestRejectionReason,
) automations.GeneratedWorkRequestOutcome {
	detached := request
	detached.Payload = append([]byte(nil), request.Payload...)
	outcome := automations.GeneratedWorkRequestOutcome{Request: detached}
	if rejection != "" {
		outcome.Status = automations.WorkRequestAdmissionRejected
		outcome.RejectionReason = rejection
		f.workRequestOutcomes = append(f.workRequestOutcomes, outcome)
		return outcome
	}

	if f.admittedRequests == nil {
		f.admittedRequests = make(map[string]automations.GeneratedWorkRequest)
	}
	key := generatedRequestKey(request)
	if original, exists := f.admittedRequests[key]; exists {
		outcome.Status = automations.WorkRequestAdmissionDuplicate
		outcome.OriginalRequestID = original.Identity.RequestID
		f.workRequestOutcomes = append(f.workRequestOutcomes, outcome)
		return outcome
	}
	f.admittedRequests[key] = detached
	f.emissionCount++
	outcome.Status = automations.WorkRequestAdmissionAccepted
	f.workRequestOutcomes = append(f.workRequestOutcomes, outcome)
	return outcome
}

func cloneWorkRequestOutcomes(
	outcomes []automations.GeneratedWorkRequestOutcome,
) []automations.GeneratedWorkRequestOutcome {
	cloned := make([]automations.GeneratedWorkRequestOutcome, len(outcomes))
	copy(cloned, outcomes)
	for i := range cloned {
		cloned[i].Request.Payload = append([]byte(nil), outcomes[i].Request.Payload...)
	}
	return cloned
}

func generatedWorkReconcileRequest() automations.ReconcileRequest {
	return automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{{
			AutomationID: "auto-work",
			SourceID:     "source-work",
			Kind:         "event-stream",
			State:        automations.DesiredLifecycleRunning,
		}},
	}
}

func generatedRequest(
	requestID string,
	payload []byte,
	reference string,
) automations.GeneratedWorkRequest {
	return automations.GeneratedWorkRequest{
		Identity: automations.GeneratedWorkRequestIdentity{
			AutomationID: "auto-work",
			SourceID:     "source-work",
			RequestID:    requestID,
		},
		Payload:          payload,
		PayloadReference: reference,
	}
}

func generatedRequestKey(request automations.GeneratedWorkRequest) string {
	return request.Identity.AutomationID + "\x00" +
		request.Identity.SourceID + "\x00" +
		string(request.Payload) + "\x00" +
		request.PayloadReference
}

func assertWorkRequestOutcome(
	t *testing.T,
	outcome automations.GeneratedWorkRequestOutcome,
	status automations.WorkRequestAdmissionStatus,
	rejection automations.WorkRequestRejectionReason,
) {
	t.Helper()
	if outcome.Status != status || outcome.RejectionReason != rejection {
		t.Fatalf(
			"outcome status/rejection = %q/%q, want %q/%q",
			outcome.Status,
			outcome.RejectionReason,
			status,
			rejection,
		)
	}
}
