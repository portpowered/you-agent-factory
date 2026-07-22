package dataplane_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/dataplane"
)

func TestNewLiveOpener_OpenForTarget_DelegatesToHost(t *testing.T) {
	t.Parallel()

	host := &liveOpenHost{sessionID: "sess-open"}
	opener := dataplane.NewLiveOpener(host)

	sessionID, err := opener.OpenForTarget(context.Background(), factorysessions.Target{
		Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	})
	if err != nil {
		t.Fatalf("OpenForTarget: %v", err)
	}
	if sessionID != "sess-open" {
		t.Fatalf("session id = %q, want sess-open", sessionID)
	}
}

func TestNewLiveOpener_OpenForTarget_RequiresHost(t *testing.T) {
	t.Parallel()

	var opener *dataplane.LiveOpener
	_, err := opener.OpenForTarget(context.Background(), factorysessions.Target{})
	if err == nil {
		t.Fatal("OpenForTarget = nil, want host required error")
	}

	opener = dataplane.NewLiveOpener(nil)
	_, err = opener.OpenForTarget(context.Background(), factorysessions.Target{})
	if err == nil {
		t.Fatal("OpenForTarget with nil host = nil, want host required error")
	}
}

type liveOpenHost struct {
	sessionID string
	err       error
}

func (h *liveOpenHost) OpenLiveSessionForTarget(_ context.Context, _ factorysessions.Target) (string, error) {
	if h.err != nil {
		return "", h.err
	}
	return h.sessionID, nil
}

func TestLiveOpener_OpenForTarget_PropagatesHostError(t *testing.T) {
	t.Parallel()

	host := &liveOpenHost{err: errors.New("open failed")}
	opener := dataplane.NewLiveOpener(host)

	_, err := opener.OpenForTarget(context.Background(), factorysessions.Target{})
	if err == nil || err.Error() != "open failed" {
		t.Fatalf("OpenForTarget error = %v, want open failed", err)
	}
}
