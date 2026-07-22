package controlplane_test

import (
	"context"
	"errors"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
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

func (h *openControlHost) SelectTarget(targets []factorysessions.Target, ref *factorysessions.TargetRef) (*factorysessions.Target, error) {
	return factorysessions.SelectTarget(targets, ref)
}

func (h *openControlHost) InitializeFactoryScaffold(_ string) error {
	return h.scaffoldErr
}

func (h *openControlHost) ValidateInitNewFactoryNestedDir(folder string) error {
	return factorysessions.ValidateInitNewFactoryNestedDir(folder, platformfilesystem.Local{})
}

func (h *openControlHost) ResolveSessionFolder(folder string) (string, error) {
	return factorysessions.ResolveSessionFolder(folder, func() (string, error) { return "", errors.New("unused home") }, platformfilesystem.Local{})
}

type liveOpenHost struct {
	sessionID string
	err       error
}

func (h *liveOpenHost) OpenForTarget(_ context.Context, _ factorysessions.Target) (string, error) {
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
	opener := &liveOpenHost{err: errors.New("open should not run")}

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
	opener := &liveOpenHost{}

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

func TestOpenFromFolder_OpensSelectedTarget(t *testing.T) {
	t.Parallel()

	host := &openControlHost{
		targets: []factorysessions.Target{{
			Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			FactoryDir: "/tmp/factory",
		}},
	}
	opener := &liveOpenHost{sessionID: "sess-opened"}

	result, err := controlplane.OpenFromFolder(
		context.Background(),
		host,
		opener,
		"/tmp",
		nil,
		false,
		false,
	)
	if err != nil {
		t.Fatalf("OpenFromFolder: %v", err)
	}
	if result.SessionID != "sess-opened" {
		t.Fatalf("session id = %q, want sess-opened", result.SessionID)
	}
}

type initNewFactoryHost struct {
	discoverCalls int
	targets       []factorysessions.Target
	scaffoldErr   error
}

func (h *initNewFactoryHost) DiscoverTargets(_ string) ([]factorysessions.Target, error) {
	h.discoverCalls++
	if h.discoverCalls == 1 {
		return nil, factorysessions.NewValidationError(
			factorysessions.ValidationReasonNotRunnable,
			"folderPath",
			errors.New("no runnable targets"),
		)
	}
	return h.targets, nil
}

func (h *initNewFactoryHost) SelectTarget(targets []factorysessions.Target, ref *factorysessions.TargetRef) (*factorysessions.Target, error) {
	return factorysessions.SelectTarget(targets, ref)
}

func (h *initNewFactoryHost) InitializeFactoryScaffold(_ string) error {
	return h.scaffoldErr
}

func (h *initNewFactoryHost) ValidateInitNewFactoryNestedDir(folder string) error {
	return factorysessions.ValidateInitNewFactoryNestedDir(folder, platformfilesystem.Local{})
}

func (h *initNewFactoryHost) ResolveSessionFolder(folder string) (string, error) {
	return factorysessions.ResolveSessionFolder(folder, func() (string, error) { return "", errors.New("unused home") }, platformfilesystem.Local{})
}

func TestOpenFromFolder_InitNewFactoryScaffoldsAndOpens(t *testing.T) {
	t.Parallel()

	host := &initNewFactoryHost{
		targets: []factorysessions.Target{{
			Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			FactoryDir: "/tmp/factory",
		}},
	}
	opener := &liveOpenHost{sessionID: "sess-init"}

	result, err := controlplane.OpenFromFolder(
		context.Background(),
		host,
		opener,
		t.TempDir(),
		nil,
		false,
		true,
	)
	if err != nil {
		t.Fatalf("OpenFromFolder: %v", err)
	}
	if result.SessionID != "sess-init" {
		t.Fatalf("session id = %q, want sess-init", result.SessionID)
	}
	if host.discoverCalls < 2 {
		t.Fatalf("discover calls = %d, want at least 2", host.discoverCalls)
	}
}

func TestGetLiveFactorySession_ReturnsNotFoundForMissingSession(t *testing.T) {
	t.Parallel()

	host := &readTestHost{sessions: map[string]*factorysessions.LiveSession{}}

	_, err := controlplane.GetLiveFactorySession(context.Background(), host, "missing")
	if err == nil {
		t.Fatal("GetLiveFactorySession = nil, want not found")
	}
}
