package controlplane_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/factorysessions/dataplane"
)

type openControlHost struct {
	targets     []factorysessions.Target
	discoverErr error
	scaffoldErr error
}

func (h *openControlHost) DiscoverTargets(_ string) ([]factorysessions.Target, error) {
	if h.discoverErr != nil {
		return nil, h.discoverErr
	}
	return h.targets, nil
}

func (h *openControlHost) InitializeFactoryScaffold(_ string) error {
	return h.scaffoldErr
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

func TestOpenFromFolder_ValidateOnlyNotRunnableReturnsInitNewFactoryHint(t *testing.T) {
	t.Parallel()

	host := &openControlHost{
		discoverErr: factorysessions.NewValidationError(
			factorysessions.ValidationReasonNotRunnable,
			"folderPath",
			errors.New("no runnable targets"),
		),
	}
	opener := dataplane.NewLiveOpener(&liveOpenHost{err: errors.New("open should not run")})

	result, err := controlplane.OpenFromFolder(
		context.Background(),
		host,
		opener,
		t.TempDir(),
		nil,
		true,
		false,
	)
	if err != nil {
		t.Fatalf("OpenFromFolder: %v", err)
	}
	if !result.InitsNewFactory {
		t.Fatal("InitsNewFactory = false, want true")
	}
}

func TestOpenFromFolder_PropagatesTargetSelectionError(t *testing.T) {
	t.Parallel()

	host := &openControlHost{
		targets: []factorysessions.Target{
			{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}},
			{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"}},
		},
	}
	opener := dataplane.NewLiveOpener(&liveOpenHost{})

	_, err := controlplane.OpenFromFolder(
		context.Background(),
		host,
		opener,
		"/tmp",
		&factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "missing"},
		false,
		false,
	)
	if err == nil {
		t.Fatal("OpenFromFolder = nil, want target not found")
	}
	reason, _, ok := factorysessions.ValidationReasonFromError(err)
	if !ok || reason != factorysessions.ValidationReasonTargetNotFound {
		t.Fatalf("validation reason = %q ok=%v, want target_not_found", reason, ok)
	}
}
