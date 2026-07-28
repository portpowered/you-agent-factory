package controlplane_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionvalidation"
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
	return logicaltarget.Select(targets, ref)
}

func (h *openControlHost) InitializeFactoryScaffold(_ string) error {
	return h.scaffoldErr
}

func (h *openControlHost) ValidateInitNewFactoryNestedDir(folder string) error {
	return logicaltarget.ValidateInitNewFactoryNestedDir(folder, platformfilesystem.Local{})
}

func (h *openControlHost) ResolveSessionFolder(folder string) (string, error) {
	return logicaltarget.ResolveSessionFolder(folder, func() (string, error) { return "", errors.New("unused home") }, platformfilesystem.Local{})
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
		discoverErr: sessionvalidation.New(
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
	reason, _, ok := sessionvalidation.ReasonFromError(err)
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
	discoverCalls      int
	initialDiscoverErr error
	targets            []factorysessions.Target
	scaffoldErr        error
}

func (h *initNewFactoryHost) DiscoverTargets(_ string) ([]factorysessions.Target, error) {
	h.discoverCalls++
	if h.discoverCalls == 1 {
		if h.initialDiscoverErr != nil {
			return nil, h.initialDiscoverErr
		}
		return nil, sessionvalidation.New(
			factorysessions.ValidationReasonNotRunnable,
			"folderPath",
			errors.New("no runnable targets"),
		)
	}
	return h.targets, nil
}

func (h *initNewFactoryHost) SelectTarget(targets []factorysessions.Target, ref *factorysessions.TargetRef) (*factorysessions.Target, error) {
	return logicaltarget.Select(targets, ref)
}

func (h *initNewFactoryHost) InitializeFactoryScaffold(_ string) error {
	return h.scaffoldErr
}

func (h *initNewFactoryHost) ValidateInitNewFactoryNestedDir(folder string) error {
	return logicaltarget.ValidateInitNewFactoryNestedDir(folder, platformfilesystem.Local{})
}

func (h *initNewFactoryHost) ResolveSessionFolder(folder string) (string, error) {
	return logicaltarget.ResolveSessionFolder(folder, func() (string, error) { return "", errors.New("unused home") }, platformfilesystem.Local{})
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

func TestOpenFromFolder_InitNewFactoryAcceptsMissingNestedFactory(t *testing.T) {
	t.Parallel()

	host := &initNewFactoryHost{
		initialDiscoverErr: sessionvalidation.New(
			factorysessions.ValidationReasonConfigLoadFailed,
			"folderPath",
			errors.New("nested Factory configuration is missing"),
		),
		targets: []factorysessions.Target{{
			Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			FactoryDir: "/tmp/factory",
		}},
	}

	result, err := controlplane.OpenFromFolder(
		context.Background(),
		host,
		&liveOpenHost{sessionID: "sess-init"},
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
}

func TestOpenFromFolder_InitNewFactoryPropagatesResolveFolderError(t *testing.T) {
	t.Parallel()

	host := &resolveFolderErrorHost{}

	_, err := controlplane.OpenFromFolder(
		context.Background(),
		host,
		&liveOpenHost{sessionID: "sess-init"},
		t.TempDir(),
		nil,
		false,
		true,
	)
	if err == nil || err.Error() != "resolve failed" {
		t.Fatalf("OpenFromFolder error = %v, want resolve failed", err)
	}
}

func TestOpenFromFolder_InitNewFactoryPropagatesUnrecoverableDiscoveryError(t *testing.T) {
	t.Parallel()

	discoverErr := sessionvalidation.New(
		factorysessions.ValidationReasonTargetNotFound,
		"target.name",
		errors.New("missing target"),
	)
	host := &initNewFactoryHost{initialDiscoverErr: discoverErr}

	_, err := controlplane.OpenFromFolder(
		context.Background(),
		host,
		&liveOpenHost{sessionID: "sess-init"},
		t.TempDir(),
		nil,
		false,
		true,
	)
	if !errors.Is(err, discoverErr) {
		t.Fatalf("OpenFromFolder error = %v, want %v", err, discoverErr)
	}
}

func TestOpenFromFolder_InitNewFactoryRequiresLiveOpenerAfterScaffold(t *testing.T) {
	t.Parallel()

	host := &initNewFactoryHost{
		targets: []factorysessions.Target{{
			Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			FactoryDir: "/tmp/factory",
		}},
	}

	_, err := controlplane.OpenFromFolder(
		context.Background(),
		host,
		nil,
		t.TempDir(),
		nil,
		false,
		true,
	)
	if err == nil {
		t.Fatal("OpenFromFolder = nil, want live opener required")
	}
	if !containsSubstring(err.Error(), "live session dataplane opener is required") {
		t.Fatalf("OpenFromFolder error = %q, want live opener required", err)
	}
}

type resolveFolderErrorHost struct {
	openControlHost
}

func (h *resolveFolderErrorHost) ResolveSessionFolder(_ string) (string, error) {
	return "", errors.New("resolve failed")
}

func TestOpenFromFolder_IdempotentInitNewFactoryPropagatesSelectTargetError(t *testing.T) {
	t.Parallel()

	_, err := controlplane.OpenFromFolder(
		context.Background(),
		&openControlHost{targets: nil},
		&liveOpenHost{sessionID: "sess-reinit"},
		t.TempDir(),
		nil,
		false,
		true,
	)
	if err == nil {
		t.Fatal("OpenFromFolder = nil, want select target error")
	}
}

func TestOpenFromFolder_IdempotentInitNewFactoryRequiresRunnableTarget(t *testing.T) {
	t.Parallel()

	host := &openControlHost{
		targets: []factorysessions.Target{
			{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "alpha"}, FactoryDir: "/tmp/alpha"},
			{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"}, FactoryDir: "/tmp/beta"},
		},
	}

	_, err := controlplane.OpenFromFolder(
		context.Background(),
		host,
		&liveOpenHost{sessionID: "sess-reinit"},
		t.TempDir(),
		nil,
		false,
		true,
	)
	if err == nil {
		t.Fatal("OpenFromFolder = nil, want runnable target error")
	}
	if !containsSubstring(err.Error(), "did not resolve to a runnable target") {
		t.Fatalf("OpenFromFolder error = %q, want runnable target resolution failure", err)
	}
}

func TestOpenFromFolder_IdempotentInitNewFactoryPropagatesScaffoldError(t *testing.T) {
	t.Parallel()

	host := &openControlHost{
		targets: []factorysessions.Target{{
			Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			FactoryDir: "/tmp/factory",
		}},
		scaffoldErr: errors.New("scaffold failed"),
	}

	_, err := controlplane.OpenFromFolder(
		context.Background(),
		host,
		&liveOpenHost{sessionID: "sess-reinit"},
		t.TempDir(),
		nil,
		false,
		true,
	)
	if err == nil || err.Error() != "scaffold failed" {
		t.Fatalf("OpenFromFolder error = %v, want scaffold failed", err)
	}
}

func TestOpenFromFolder_IdempotentInitNewFactoryRequiresLiveOpener(t *testing.T) {
	t.Parallel()

	host := &openControlHost{
		targets: []factorysessions.Target{{
			Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			FactoryDir: "/tmp/factory",
		}},
	}

	_, err := controlplane.OpenFromFolder(
		context.Background(),
		host,
		nil,
		t.TempDir(),
		nil,
		false,
		true,
	)
	if err == nil {
		t.Fatal("OpenFromFolder = nil, want live opener required")
	}
	if !containsSubstring(err.Error(), "live session dataplane opener is required") {
		t.Fatalf("OpenFromFolder error = %q, want live opener required", err)
	}
}

func TestOpenFromFolder_IdempotentInitNewFactoryPropagatesOpenForTargetError(t *testing.T) {
	t.Parallel()

	host := &openControlHost{
		targets: []factorysessions.Target{{
			Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			FactoryDir: "/tmp/factory",
		}},
	}
	openErr := errors.New("open failed")

	_, err := controlplane.OpenFromFolder(
		context.Background(),
		host,
		&liveOpenHost{err: openErr},
		t.TempDir(),
		nil,
		false,
		true,
	)
	if !errors.Is(err, openErr) {
		t.Fatalf("OpenFromFolder error = %v, want %v", err, openErr)
	}
}

func containsSubstring(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func TestOpenFromFolder_InitNewFactoryReinitializesExistingRunnableTargetIdempotently(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	factoryDir := filepath.Join(workspaceDir, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("mkdir factory dir: %v", err)
	}

	target := factorysessions.Target{
		Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "factory"},
		FactoryDir: factoryDir,
	}
	host := &openControlHost{
		targets: []factorysessions.Target{target},
	}
	var scaffoldDirs []string
	hostWithScaffoldTracking := &idempotentReinitHost{
		openControlHost: host,
		scaffoldDirs:    &scaffoldDirs,
	}

	result, err := controlplane.OpenFromFolder(
		context.Background(),
		hostWithScaffoldTracking,
		&liveOpenHost{sessionID: "sess-reinit"},
		workspaceDir,
		nil,
		false,
		true,
	)
	if err != nil {
		t.Fatalf("OpenFromFolder: %v", err)
	}
	if result.SessionID != "sess-reinit" {
		t.Fatalf("session id = %q, want sess-reinit", result.SessionID)
	}
	if len(scaffoldDirs) != 1 || scaffoldDirs[0] != target.FactoryDir {
		t.Fatalf("scaffold dirs = %v, want one idempotent call for %q", scaffoldDirs, target.FactoryDir)
	}
}

type idempotentReinitHost struct {
	*openControlHost
	scaffoldDirs *[]string
}

func (h *idempotentReinitHost) InitializeFactoryScaffold(factoryDir string) error {
	*h.scaffoldDirs = append(*h.scaffoldDirs, factoryDir)
	return nil
}

func TestGetLiveFactorySession_ReturnsNotFoundForMissingSession(t *testing.T) {
	t.Parallel()

	host := &readTestHost{sessions: map[string]*livesession.LiveSession{}}

	_, err := controlplane.GetLiveFactorySession(context.Background(), host, "missing")
	if err == nil {
		t.Fatal("GetLiveFactorySession = nil, want not found")
	}
}
