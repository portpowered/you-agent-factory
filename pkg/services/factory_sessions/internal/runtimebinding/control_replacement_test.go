package runtimebinding

import (
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestTerminalControlRetryDoesNotReachReplacementRuntime(t *testing.T) {
	prior := &SessionState{}
	key := "CANCEL:cancel-request:turn-1"
	want := factorysessions.SessionControlResult{SessionID: "chat-target", Operation: factorysessions.SessionControlCancel, Status: factorysessions.LifecycleStatusSucceeded}
	if _, err := prior.ApplyControlOnce(key, func() (factorysessions.SessionControlResult, error) { return want, nil }); err != nil {
		t.Fatal(err)
	}
	if !prior.CanReplaceTerminatedSession() {
		t.Fatal("committed cancellation did not allow replacement")
	}
	replacement := &SessionState{}
	replacement.InheritTerminalControl(prior)
	reachedReplacement := false
	got, err := replacement.ApplyControlOnce(key, func() (factorysessions.SessionControlResult, error) {
		reachedReplacement = true
		return factorysessions.SessionControlResult{}, nil
	})
	if err != nil || reachedReplacement || got != want {
		t.Fatalf("retry result = %+v, reached replacement = %v, error = %v", got, reachedReplacement, err)
	}
}
