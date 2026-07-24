package factorysessions_test

import (
	"context"
	"encoding/json"
	"errors"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	. "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionvalidation"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type discoveryDirectoryInspection struct {
	entries []fs.DirEntry
	err     error
}

func (d discoveryDirectoryInspection) Stat(string) (fs.FileInfo, error) {
	return discoveryFileInfo{}, nil
}

func (d discoveryDirectoryInspection) ReadDir(string) ([]fs.DirEntry, error) {
	return d.entries, d.err
}

type discoveryDirEntry struct {
	name  string
	isDir bool
}

type discoveryFileInfo struct{}

func (discoveryFileInfo) Name() string       { return "session-root" }
func (discoveryFileInfo) Size() int64        { return 0 }
func (discoveryFileInfo) Mode() fs.FileMode  { return fs.ModeDir }
func (discoveryFileInfo) ModTime() time.Time { return time.Time{} }
func (discoveryFileInfo) IsDir() bool        { return true }
func (discoveryFileInfo) Sys() any           { return nil }

func (e discoveryDirEntry) Name() string               { return e.name }
func (e discoveryDirEntry) IsDir() bool                { return e.isDir }
func (e discoveryDirEntry) Type() fs.FileMode          { return 0 }
func (e discoveryDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

func alwaysRunnableProbe(folderPath, factoryDir string, ref TargetRef) (Target, bool, *DiscoveryFailure) {
	return logicaltarget.Build(folderPath, factoryDir, ref, filepath.Base(factoryDir)), true, nil
}

func TestDiscoverTargets_ReturnsDefaultAndNamedTargets(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "beta"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	targets, err := logicaltarget.Discover(root, alwaysRunnableProbe, platformfilesystem.Local{}, os.UserHomeDir)
	if err != nil {
		t.Fatalf("DiscoverTargets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("len(targets) = %d, want 2", len(targets))
	}
	if targets[0].Ref.Kind != TargetKindDefault || targets[1].Ref != (TargetRef{Kind: TargetKindNamed, Name: "beta"}) {
		t.Fatalf("targets = %#v, want default then named beta", targets)
	}
}

func TestDiscoverTargets_RejectsFolderWithoutRunnableTargets(t *testing.T) {
	root := t.TempDir()
	_, err := logicaltarget.Discover(root, func(string, string, TargetRef) (Target, bool, *DiscoveryFailure) {
		return Target{}, false, nil
	}, platformfilesystem.Local{}, os.UserHomeDir)
	if err == nil {
		t.Fatal("DiscoverTargets(empty) error = nil, want not runnable")
	}
	reason, field, ok := sessionvalidation.ReasonFromError(err)
	if !ok || reason != ValidationReasonNotRunnable || field != "folderPath" {
		t.Fatalf("validation = (%q, %q, %v), want not_runnable folderPath", reason, field, ok)
	}
}

func TestDiscoverTargets_PreservesConfigLoadFailuresWhenNoRunnableTargetsRemain(t *testing.T) {
	root := t.TempDir()

	_, err := logicaltarget.Discover(root, func(_ string, factoryDir string, ref TargetRef) (Target, bool, *DiscoveryFailure) {
		return Target{}, false, &DiscoveryFailure{
			FactoryDir: factoryDir,
			Ref:        ref,
			Summary:    "unexpected end of JSON input",
		}
	}, platformfilesystem.Local{}, os.UserHomeDir)
	if err == nil {
		t.Fatal("DiscoverTargets(config load failed) error = nil, want structured failure")
	}
	reason, field, ok := sessionvalidation.ReasonFromError(err)
	if !ok || reason != ValidationReasonConfigLoadFailed || field != "folderPath" {
		t.Fatalf("validation = (%q, %q, %v), want config_load_failed folderPath", reason, field, ok)
	}
	var targetedErr interface {
		ErrorTargets() []factorydefinitions.ValidationTarget
	}
	if !errors.As(err, &targetedErr) {
		t.Fatalf("config load error %v did not expose structured targets", err)
	}
	targets := targetedErr.ErrorTargets()
	if len(targets) != 1 {
		t.Fatalf("config load error targets = %#v, want one target", targets)
	}
	if targets[0].Code != "factory.session.target.config_load_failed" {
		t.Fatalf("config load target code = %q, want factory.session.target.config_load_failed", targets[0].Code)
	}
	if targets[0].Subject.ID != "default" {
		t.Fatalf("config load target subject id = %q, want default", targets[0].Subject.ID)
	}
}

func TestDiscoverTargets_UsesInjectedDirectoryOrderingAndFiltering(t *testing.T) {
	root := t.TempDir()
	directories := discoveryDirectoryInspection{entries: []fs.DirEntry{
		discoveryDirEntry{name: "beta", isDir: true},
		discoveryDirEntry{name: "notes.txt"},
		discoveryDirEntry{name: "bad/name", isDir: true},
		discoveryDirEntry{name: "alpha", isDir: true},
	}}
	targets, err := logicaltarget.Discover(root, alwaysRunnableProbe, directories, os.UserHomeDir)
	if err != nil {
		t.Fatalf("DiscoverTargets: %v", err)
	}
	want := []TargetRef{
		{Kind: TargetKindDefault},
		{Kind: TargetKindNamed, Name: "alpha"},
		{Kind: TargetKindNamed, Name: "beta"},
	}
	if len(targets) != len(want) {
		t.Fatalf("targets = %#v, want refs %#v", targets, want)
	}
	for index := range want {
		if targets[index].Ref != want[index] {
			t.Fatalf("target[%d] ref = %#v, want %#v", index, targets[index].Ref, want[index])
		}
	}
}

func TestDiscoverTargets_FailsClosedAndClassifiesDirectoryErrors(t *testing.T) {
	root := t.TempDir()
	if _, err := logicaltarget.Discover(root, alwaysRunnableProbe, nil, os.UserHomeDir); err == nil {
		t.Fatal("DiscoverTargets(nil inspection) error = nil")
	}
	if _, err := logicaltarget.Discover(root, alwaysRunnableProbe, discoveryDirectoryInspection{err: fs.ErrPermission}, os.UserHomeDir); err == nil {
		t.Fatal("DiscoverTargets(permission) error = nil")
	} else if reason, field, ok := sessionvalidation.ReasonFromError(err); !ok || reason != ValidationReasonUnreadable || field != "folderPath" {
		t.Fatalf("permission validation = (%q, %q, %v)", reason, field, ok)
	}
	readErr := errors.New("directory unavailable")
	if _, err := logicaltarget.Discover(root, alwaysRunnableProbe, discoveryDirectoryInspection{err: readErr}, os.UserHomeDir); !errors.Is(err, readErr) {
		t.Fatalf("DiscoverTargets(generic read error) = %v, want wrapped %v", err, readErr)
	}
}

func TestSelectTarget_AutoSelectsSingleTarget(t *testing.T) {
	targets := []Target{{
		Ref:   TargetRef{Kind: TargetKindDefault},
		Label: "default",
	}}
	selected, err := logicaltarget.Select(targets, nil)
	if err != nil {
		t.Fatalf("SelectTarget: %v", err)
	}
	if selected == nil || selected.Ref.Kind != TargetKindDefault {
		t.Fatalf("selected = %#v, want default target", selected)
	}
}

func TestSelectTarget_ReturnsNilForAmbiguousFolder(t *testing.T) {
	targets := []Target{
		{Ref: TargetRef{Kind: TargetKindDefault}},
		{Ref: TargetRef{Kind: TargetKindNamed, Name: "beta"}},
	}
	selected, err := logicaltarget.Select(targets, nil)
	if err != nil {
		t.Fatalf("SelectTarget: %v", err)
	}
	if selected != nil {
		t.Fatalf("selected = %#v, want nil for multi-target picker", selected)
	}
}

func TestSelectTarget_RejectsMissingNamedTarget(t *testing.T) {
	targets := []Target{{Ref: TargetRef{Kind: TargetKindDefault}}}
	_, err := logicaltarget.Select(targets, &TargetRef{Kind: TargetKindNamed, Name: "missing"})
	if err == nil {
		t.Fatal("SelectTarget(missing) error = nil, want target_not_found")
	}
	reason, field, ok := sessionvalidation.ReasonFromError(err)
	if !ok || reason != ValidationReasonTargetNotFound || field != "target.name" {
		t.Fatalf("validation = (%q, %q, %v), want target_not_found target.name", reason, field, ok)
	}
}

func TestCloneTargets_ReturnsDefensiveCopy(t *testing.T) {
	original := []Target{{Ref: TargetRef{Kind: TargetKindDefault}, Label: "default"}}
	cloned := logicaltarget.Clone(original)
	original[0].Label = "mutated"
	if cloned[0].Label != "default" {
		t.Fatalf("cloned label = %q, want unchanged copy", cloned[0].Label)
	}
}

// --- merged from live_control_contract_characterization_test.go ---

// peerLiveControlFake exercises the published live-control root slice through
// the singular Service. It compiles against only the Sessions root package and
// never imports factory_sessions/internal or live-runtime registry/host types.
type peerLiveControlFake struct {
	*peerRootServiceFake
	openResults map[string]*OpenResult
	listed      []ReadProjection
	lifecycle   map[string]LifecycleStatus
	closed      map[string]bool
}

func newPeerLiveControlFake() *peerLiveControlFake {
	return &peerLiveControlFake{
		peerRootServiceFake: newPeerRootServiceFake(),
		openResults:         make(map[string]*OpenResult),
		lifecycle:           make(map[string]LifecycleStatus),
		closed:              make(map[string]bool),
	}
}

var _ Service = (*peerLiveControlFake)(nil)

func (fake *peerLiveControlFake) OpenFactorySession(
	_ context.Context,
	request LiveControlOpenRequest,
) (*LiveControlOpenResult, error) {
	if result, ok := fake.openResults[request.FolderPath]; ok {
		return result, nil
	}
	return nil, ErrSessionNotFound
}

func (fake *peerLiveControlFake) ListFactorySessions(context.Context) ([]LiveControlListItem, error) {
	out := make([]LiveControlListItem, len(fake.listed))
	copy(out, fake.listed)
	return out, nil
}

func (fake *peerLiveControlFake) GetFactorySession(
	_ context.Context,
	sessionID string,
) (LiveControlSnapshot, error) {
	if fake.closed[sessionID] {
		return LiveControlSnapshot{}, ErrSessionNotFound
	}
	if projection, ok := fake.sessions[sessionID]; ok {
		return projection, nil
	}
	return LiveControlSnapshot{}, ErrSessionNotFound
}

func (fake *peerLiveControlFake) PauseLiveFactorySession(
	_ context.Context,
	sessionID string,
	_ LiveControlRequest,
) (LiveControlResult, error) {
	return fake.applyLiveControl(sessionID, LifecycleControlPause)
}

func (fake *peerLiveControlFake) ResumeLiveFactorySession(
	_ context.Context,
	sessionID string,
	_ LiveControlRequest,
) (LiveControlResult, error) {
	return fake.applyLiveControl(sessionID, LifecycleControlResume)
}

func (fake *peerLiveControlFake) CloseFactorySession(_ context.Context, sessionID string) error {
	if _, ok := fake.sessions[sessionID]; !ok && !fake.closed[sessionID] {
		if _, opened := fake.lifecycle[sessionID]; !opened {
			return ErrSessionNotFound
		}
	}
	fake.closed[sessionID] = true
	delete(fake.sessions, sessionID)
	delete(fake.lifecycle, sessionID)
	return nil
}

func (fake *peerLiveControlFake) applyLiveControl(
	sessionID string,
	operation LifecycleControlKind,
) (LiveControlResult, error) {
	if fake.closed[sessionID] {
		return LiveControlResult{}, ErrSessionNotFound
	}
	status, ok := fake.lifecycle[sessionID]
	if !ok {
		return LiveControlResult{}, ErrSessionNotFound
	}
	// Peer fake hardcodes representative outcomes; it does not re-test nested
	// lifecycle evaluation algorithms.
	switch status {
	case LifecycleStatusSucceeded, LifecycleStatusFailed:
		return LiveControlResult{}, &LiveControlError{
			Operation: operation,
			Outcome:   LifecycleControlOutcomeTerminalSession,
			Status:    status,
			Message:   string(LifecycleControlOutcomeTerminalSession),
		}
	case LifecycleStatusPaused:
		if operation == LifecycleControlPause {
			return LiveControlResult{
				SessionID: sessionID,
				Operation: operation,
				Outcome:   LifecycleControlOutcomeNoOp,
				Status:    status,
			}, nil
		}
		if operation == LifecycleControlResume {
			fake.lifecycle[sessionID] = LifecycleStatusRunning
			return LiveControlResult{
				SessionID: sessionID,
				Operation: operation,
				Outcome:   LifecycleControlOutcomeAccepted,
				Status:    LifecycleStatusRunning,
			}, nil
		}
	case LifecycleStatusRunning:
		if operation == LifecycleControlPause {
			fake.lifecycle[sessionID] = LifecycleStatusPaused
			return LiveControlResult{
				SessionID: sessionID,
				Operation: operation,
				Outcome:   LifecycleControlOutcomeAccepted,
				Status:    LifecycleStatusPaused,
			}, nil
		}
		if operation == LifecycleControlResume {
			return LiveControlResult{
				SessionID: sessionID,
				Operation: operation,
				Outcome:   LifecycleControlOutcomeNoOp,
				Status:    status,
			}, nil
		}
	}
	return LiveControlResult{}, &LiveControlError{
		Operation: operation,
		Outcome:   LifecycleControlOutcomeInvalidState,
		Status:    status,
		Message:   string(LifecycleControlOutcomeInvalidState),
	}
}

func seedRunningLiveControlSession(
	fake *peerLiveControlFake,
	sessionID, folder string,
) LiveControlSnapshot {
	fake.openResults[folder] = &LiveControlOpenResult{
		SessionID:  sessionID,
		FolderPath: folder,
		Session: &ScopedLiveSessionSummary{
			ID:         sessionID,
			FolderPath: folder,
			IsDefault:  true,
		},
	}
	snapshot := LiveControlSnapshot{
		Context: ProjectionContext{
			FactorySessionID: sessionID,
			Session: &ScopedLiveSessionSummary{
				ID:         sessionID,
				FolderPath: folder,
				IsDefault:  true,
			},
		},
		Runtime: RuntimeProjection{Status: "RUNNING"},
	}
	fake.sessions[sessionID] = snapshot
	fake.lifecycle[sessionID] = LifecycleStatusRunning
	fake.listed = []LiveControlListItem{{
		Context:          snapshot.Context,
		Runtime:          snapshot.Runtime,
		RuntimeAvailable: true,
	}}
	return snapshot
}

func requireLiveOpenIdentity(
	t *testing.T,
	opened *LiveControlOpenResult,
	sessionID string,
) {
	t.Helper()
	if opened == nil || opened.SessionID != sessionID || opened.Session == nil || opened.Session.ID != sessionID {
		t.Fatalf("OpenFactorySession result = %#v, want stable session identity %q", opened, sessionID)
	}
}

func requireAcceptedPause(
	t *testing.T,
	paused LiveControlResult,
	sessionID string,
) {
	t.Helper()
	if paused.SessionID != sessionID ||
		paused.Outcome != LifecycleControlOutcomeAccepted ||
		paused.Status != LifecycleStatusPaused {
		t.Fatalf("PauseLiveFactorySession = %#v, want accepted pause", paused)
	}
}

func TestLiveControlRootContract_OpenListGetStableIdentity(t *testing.T) {
	t.Parallel()

	fake := newPeerLiveControlFake()
	sessionID := "live-session-alpha"
	folder := "/workspace/factories/demo"
	seedRunningLiveControlSession(fake, sessionID, folder)

	var service Service = fake
	ctx := context.Background()

	opened, err := service.OpenFactorySession(ctx, LiveControlOpenRequest{FolderPath: folder})
	if err != nil {
		t.Fatalf("OpenFactorySession: %v", err)
	}
	requireLiveOpenIdentity(t, opened, sessionID)

	listed, err := service.ListFactorySessions(ctx)
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if len(listed) != 1 || listed[0].Context.FactorySessionID != sessionID {
		t.Fatalf("ListFactorySessions = %#v, want one row for %q", listed, sessionID)
	}

	got, err := service.GetFactorySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if got.Context.FactorySessionID != sessionID || got.Runtime.Status != "RUNNING" {
		t.Fatalf("GetFactorySession = %#v, want live projection for %q", got, sessionID)
	}

	paused, err := service.PauseLiveFactorySession(ctx, sessionID, LiveControlRequest{Reason: "operator-pause"})
	if err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}
	requireAcceptedPause(t, paused, sessionID)
}

func TestLiveControlRootContract_TypedMissingAndLifecycleFailures(t *testing.T) {
	t.Parallel()

	fake := newPeerLiveControlFake()
	terminalID := "live-session-terminal"
	fake.lifecycle[terminalID] = LifecycleStatusSucceeded
	fake.sessions[terminalID] = LiveControlSnapshot{
		Context: ProjectionContext{FactorySessionID: terminalID},
		Runtime: RuntimeProjection{Status: "SUCCEEDED"},
	}

	var service Service = fake
	ctx := context.Background()

	_, err := service.GetFactorySession(ctx, "missing-live-session")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("GetFactorySession missing = %v, want ErrSessionNotFound", err)
	}

	_, err = service.PauseLiveFactorySession(ctx, "missing-live-session", LiveControlRequest{})
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("PauseLiveFactorySession missing = %v, want ErrSessionNotFound", err)
	}

	_, err = service.PauseLiveFactorySession(ctx, terminalID, LiveControlRequest{})
	var rejected *LiveControlError
	if !errors.As(err, &rejected) {
		t.Fatalf("PauseLiveFactorySession terminal = %v, want *LiveControlError", err)
	}
	if rejected.Outcome != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("PauseLiveFactorySession outcome = %q, want TERMINAL_SESSION", rejected.Outcome)
	}
	if errors.Is(err, ErrSessionNotFound) {
		t.Fatal("invalid lifecycle transition must stay distinct from ErrSessionNotFound")
	}

	if err := service.CloseFactorySession(ctx, "missing-live-session"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("CloseFactorySession missing = %v, want ErrSessionNotFound", err)
	}
}

// --- merged from response_stream_contract_characterization_test.go ---

// peerResponseStreamFake exercises the published response-stream root slice
// through the singular Service. It compiles against only the Sessions root
// package and never imports factory_sessions/internal or private response-stream
// store/manager types. It does not publish a nested stream interface for peer
// import.
type peerResponseStreamFake struct {
	*peerRootServiceFake
	eventsBySession map[string][]ResponseStreamEvent
	staleCursors    map[string]bool
	closedSessions  map[string]bool
}

func newPeerResponseStreamFake() *peerResponseStreamFake {
	return &peerResponseStreamFake{
		peerRootServiceFake: newPeerRootServiceFake(),
		eventsBySession:     make(map[string][]ResponseStreamEvent),
		staleCursors:        make(map[string]bool),
		closedSessions:      make(map[string]bool),
	}
}

var _ Service = (*peerResponseStreamFake)(nil)

func (fake *peerResponseStreamFake) SubscribeFactoryResponseEvents(
	_ context.Context,
	request ResponseStreamSubscriptionRequest,
) (*ResponseStreamCursor, error) {
	sessionID := request.SessionID
	if fake.closedSessions[sessionID] {
		return nil, ErrResponseStreamSubscriptionClosed
	}
	if _, ok := fake.eventsBySession[sessionID]; !ok {
		return nil, ErrSessionNotFound
	}
	if fake.staleCursors[sessionID] || request.AfterSequence < 0 {
		return nil, ErrResponseStreamStaleCursor
	}

	after := request.AfterSequence
	detached := false
	nextBatch := func() ([]ResponseStreamEvent, error) {
		if detached || fake.closedSessions[sessionID] {
			return nil, ErrResponseStreamSubscriptionClosed
		}
		retained := fake.eventsBySession[sessionID]
		out := make([]ResponseStreamEvent, 0, len(retained))
		for _, event := range retained {
			if event.Sequence > after {
				out = append(out, event)
			}
		}
		return out, nil
	}

	return &ResponseStreamCursor{
		NextEvents: func(context.Context) ([]ResponseStreamEvent, error) {
			return nextBatch()
		},
		DrainEvents: func() ([]ResponseStreamEvent, error) {
			return nextBatch()
		},
		DetachCursor: func() {
			detached = true
		},
	}, nil
}

func TestResponseStreamRootContract_SubscriberReceivesOnlyNewerEvents(t *testing.T) {
	t.Parallel()

	fake := newPeerResponseStreamFake()
	sessionID := "sess-response-alpha"
	dispatchID := "dispatch-1"
	fake.eventsBySession[sessionID] = []ResponseStreamEvent{
		{
			FactorySessionID: sessionID,
			DispatchID:       dispatchID,
			EventID:          "evt-1",
			Sequence:         1,
			Kind:             ResponseEventKindProgress,
			Phase:            ResponseEventPhaseUpdated,
			SchemaVersion:    ResponseEventSchemaVersionV1,
			Payload:          json.RawMessage(`{}`),
		},
		{
			FactorySessionID: sessionID,
			DispatchID:       dispatchID,
			EventID:          "evt-2",
			Sequence:         2,
			Kind:             ResponseEventKindMessage,
			Phase:            ResponseEventPhaseDelta,
			SchemaVersion:    ResponseEventSchemaVersionV1,
			Payload:          json.RawMessage(`{}`),
		},
		{
			FactorySessionID: sessionID,
			DispatchID:       dispatchID,
			EventID:          "evt-3",
			Sequence:         3,
			Kind:             ResponseStreamCompletionKind,
			Phase:            ResponseStreamCompletionPhase,
			SchemaVersion:    ResponseEventSchemaVersionV1,
			Payload:          json.RawMessage(`{}`),
		},
	}

	var service Service = fake
	cursor, err := service.SubscribeFactoryResponseEvents(context.Background(), ResponseStreamSubscriptionRequest{
		SessionID:     sessionID,
		AfterSequence: 1,
		DispatchID:    dispatchID,
	})
	if err != nil {
		t.Fatalf("SubscribeFactoryResponseEvents: %v", err)
	}
	if cursor == nil {
		t.Fatal("SubscribeFactoryResponseEvents returned nil cursor")
	}

	events, err := cursor.Drain()
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("Drain events = %#v, want only sequences > 1", events)
	}
	if events[0].Sequence != 2 || events[1].Sequence != 3 {
		t.Fatalf("Drain sequences = [%d %d], want [2 3]", events[0].Sequence, events[1].Sequence)
	}
	completion := events[1]
	if completion.Kind != ResponseStreamCompletionKind ||
		completion.Phase != ResponseStreamCompletionPhase {
		t.Fatalf("completion event = %#v, want published completion vocabulary", completion)
	}

	// Gap payload remains expressible on the published root slice without store types.
	gap := ResponseStreamGap{FirstAvailableSequence: 2, Reason: "compaction"}
	if gap.FirstAvailableSequence != 2 {
		t.Fatalf("ResponseStreamGap = %#v, want firstAvailableSequence=2", gap)
	}
}

func progressStreamEvent(sessionID, eventID string, sequence int64) ResponseStreamEvent {
	return ResponseStreamEvent{
		FactorySessionID: sessionID,
		EventID:          eventID,
		Sequence:         sequence,
		Kind:             ResponseEventKindProgress,
		Phase:            ResponseEventPhaseUpdated,
		SchemaVersion:    ResponseEventSchemaVersionV1,
		Payload:          json.RawMessage(`{}`),
	}
}

func seedGapStreamEvent(t *testing.T, fake *peerResponseStreamFake, sessionID string) {
	t.Helper()
	gapPayload, err := json.Marshal(ResponseStreamGap{
		FromSequence:           1,
		ToSequence:             3,
		FirstAvailableSequence: 4,
		Reason:                 "compaction",
	})
	if err != nil {
		t.Fatalf("marshal gap: %v", err)
	}
	fake.eventsBySession[sessionID] = []ResponseStreamEvent{{
		FactorySessionID: sessionID,
		EventID:          "evt-gap",
		Sequence:         4,
		Kind:             ResponseStreamKindGap,
		Phase:            ResponseEventPhaseUpdated,
		SchemaVersion:    ResponseEventSchemaVersionV1,
		Payload:          gapPayload,
	}}
}

func TestResponseStreamRootContract_TypedStaleCursorGapAndCancelFailures(t *testing.T) {
	t.Parallel()

	fake := newPeerResponseStreamFake()
	staleSession, gapSession, cancelSession := "sess-stale", "sess-gap", "sess-cancel"
	fake.eventsBySession[staleSession] = []ResponseStreamEvent{
		progressStreamEvent(staleSession, "evt-stale", 5),
	}
	fake.staleCursors[staleSession] = true
	seedGapStreamEvent(t, fake, gapSession)
	fake.eventsBySession[cancelSession] = []ResponseStreamEvent{
		progressStreamEvent(cancelSession, "evt-cancel", 1),
	}

	var service Service = fake
	_, err := service.SubscribeFactoryResponseEvents(context.Background(), ResponseStreamSubscriptionRequest{
		SessionID: staleSession, AfterSequence: 1,
	})
	if !errors.Is(err, ErrResponseStreamStaleCursor) {
		t.Fatalf("stale cursor = %v, want ErrResponseStreamStaleCursor", err)
	}
	if !errors.Is(err, ErrInvalidResponseEventCursor) {
		t.Fatalf("ErrResponseStreamStaleCursor must alias ErrInvalidResponseEventCursor, got %v", err)
	}

	cursor, err := service.SubscribeFactoryResponseEvents(context.Background(), ResponseStreamSubscriptionRequest{
		SessionID: gapSession, AfterSequence: 0,
	})
	if err != nil {
		t.Fatalf("gap SubscribeFactoryResponseEvents: %v", err)
	}
	events, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("gap Next: %v", err)
	}
	if len(events) != 1 || events[0].Kind != ResponseStreamKindGap {
		t.Fatalf("gap events = %#v, want one ResponseStreamKindGap outcome", events)
	}
	var gap ResponseStreamGap
	if err := json.Unmarshal(events[0].Payload, &gap); err != nil {
		t.Fatalf("unmarshal gap payload: %v", err)
	}
	if gap.FirstAvailableSequence != 4 || gap.FromSequence != 1 || gap.ToSequence != 3 {
		t.Fatalf("gap payload = %#v, want retention gap shape", gap)
	}

	cancelCursor, err := service.SubscribeFactoryResponseEvents(context.Background(), ResponseStreamSubscriptionRequest{
		SessionID: cancelSession, AfterSequence: 0,
	})
	if err != nil {
		t.Fatalf("cancel SubscribeFactoryResponseEvents: %v", err)
	}
	cancelCursor.Detach()
	_, err = cancelCursor.Next(context.Background())
	if !errors.Is(err, ErrResponseStreamSubscriptionClosed) {
		t.Fatalf("cancelled subscription = %v, want ErrResponseStreamSubscriptionClosed", err)
	}
	if !errors.Is(err, ErrResponseEventSubscriptionClosed) {
		t.Fatalf("ErrResponseStreamSubscriptionClosed must alias ErrResponseEventSubscriptionClosed, got %v", err)
	}
	if errors.Is(err, ErrResponseStreamStaleCursor) {
		t.Fatal("cancelled subscription must stay distinct from stale cursor")
	}
}

// --- merged from identity_contract_characterization_test.go ---

// peerIdentitySurfaceFake is a peer-owned stand-in for identity/target
// resolution. It compiles against only the published Factory Sessions root
// identity vocabulary and never imports factory_sessions/internal.
type peerIdentitySurfaceFake struct {
	bySignature map[string]ResolvedIdentity
	failures    map[string]error
}

func newPeerIdentitySurfaceFake() *peerIdentitySurfaceFake {
	return &peerIdentitySurfaceFake{
		bySignature: make(map[string]ResolvedIdentity),
		failures:    make(map[string]error),
	}
}

func identityRequestSignature(request IdentityNormalizeRequest) string {
	return "target|" + request.BackendScopeID + "|" + request.FolderPath + "|" +
		string(request.Target.Kind) + "|" + request.Target.Name
}

func identityProviderSignature(request IdentityNormalizeProviderRequest) string {
	boundary := request.Boundary
	return "provider|" + request.BackendScopeID + "|" + request.FolderPath + "|" +
		boundary.Provider + "|" + boundary.Kind + "|" + boundary.Boundary
}

func (fake *peerIdentitySurfaceFake) Normalize(
	request IdentityNormalizeRequest,
) (ResolvedIdentity, error) {
	signature := identityRequestSignature(request)
	if err, ok := fake.failures[signature]; ok {
		return ResolvedIdentity{}, err
	}
	if resolved, ok := fake.bySignature[signature]; ok {
		return resolved, nil
	}
	return ResolvedIdentity{}, ErrLogicalTargetNotFound
}

func (fake *peerIdentitySurfaceFake) NormalizeProvider(
	request IdentityNormalizeProviderRequest,
) (ResolvedIdentity, error) {
	signature := identityProviderSignature(request)
	if err, ok := fake.failures[signature]; ok {
		return ResolvedIdentity{}, err
	}
	if resolved, ok := fake.bySignature[signature]; ok {
		return resolved, nil
	}
	return ResolvedIdentity{}, ErrLogicalTargetNotFound
}

func defaultIdentityRequest(folder string, target TargetRef) IdentityNormalizeRequest {
	return IdentityNormalizeRequest{
		BackendScopeID: "backend-a",
		FolderPath:     folder,
		Target:         target,
	}
}

func seedDefaultResolvedIdentity(fake *peerIdentitySurfaceFake, folder, key string, requests ...IdentityNormalizeRequest) {
	resolved := ResolvedIdentity{
		Reference: CanonicalLogicalTargetReference{
			BackendScopeID: "backend-a",
			FolderPath:     folder,
			Kind:           LogicalTargetKindDefault,
		},
		LogicalSessionKeyID: key,
		RuntimeTarget: RuntimeLogicalTarget{
			FolderPath: folder,
			Kind:       string(LogicalTargetKindDefault),
		},
	}
	for _, request := range requests {
		fake.bySignature[identityRequestSignature(request)] = resolved
	}
}

func seedNamedResolvedIdentity(fake *peerIdentitySurfaceFake, request IdentityNormalizeRequest, key string) {
	fake.bySignature[identityRequestSignature(request)] = ResolvedIdentity{
		Reference: CanonicalLogicalTargetReference{
			BackendScopeID: request.BackendScopeID,
			FolderPath:     request.FolderPath,
			Kind:           LogicalTargetKindNamed,
			NamedTarget:    request.Target.Name,
		},
		LogicalSessionKeyID: key,
		RuntimeTarget: RuntimeLogicalTarget{
			FolderPath:  request.FolderPath,
			Kind:        string(LogicalTargetKindNamed),
			NamedTarget: ptr(request.Target.Name),
		},
	}
}

func seedProviderResolvedIdentity(fake *peerIdentitySurfaceFake, request IdentityNormalizeProviderRequest, key string) {
	boundary := request.Boundary
	fake.bySignature[identityProviderSignature(request)] = ResolvedIdentity{
		Reference: CanonicalLogicalTargetReference{
			BackendScopeID: request.BackendScopeID,
			FolderPath:     request.FolderPath,
			Kind:           LogicalTargetKindProvider,
			Provider: &LogicalTargetProviderBoundary{
				Provider: boundary.Provider,
				Kind:     boundary.Kind,
				Boundary: boundary.Boundary,
			},
		},
		LogicalSessionKeyID: key,
		RuntimeTarget: RuntimeLogicalTarget{
			FolderPath: request.FolderPath,
			Kind:       string(LogicalTargetKindProvider),
			ProviderBoundary: &RuntimeLogicalProviderBoundary{
				Provider: boundary.Provider,
				Kind:     boundary.Kind,
				Boundary: boundary.Boundary,
			},
		},
	}
}

func TestIdentityRootContract_EquivalentTargetsShareLogicalSessionKey(t *testing.T) {
	t.Parallel()

	fake := newPeerIdentitySurfaceFake()
	sharedKey := "lsk-equivalent-default"
	folder := "/workspace/factories/demo"
	explicitDefault := defaultIdentityRequest(folder, TargetRef{Kind: TargetKindDefault})
	equivalentEmptyKind := defaultIdentityRequest(folder, TargetRef{})
	seedDefaultResolvedIdentity(fake, folder, sharedKey, explicitDefault, equivalentEmptyKind)

	named := defaultIdentityRequest(folder, TargetRef{Kind: TargetKindNamed, Name: "beta"})
	seedNamedResolvedIdentity(fake, named, "lsk-named-beta")
	provider := IdentityNormalizeProviderRequest{
		BackendScopeID: "backend-a",
		FolderPath:     folder,
		Boundary: LogicalTargetProviderBoundary{
			Provider: "cursor",
			Kind:     "workspace",
			Boundary: "team-alpha",
		},
	}
	seedProviderResolvedIdentity(fake, provider, "lsk-provider-team-alpha")

	first, err := fake.Normalize(explicitDefault)
	if err != nil {
		t.Fatalf("Normalize(explicit default): %v", err)
	}
	second, err := fake.Normalize(equivalentEmptyKind)
	if err != nil {
		t.Fatalf("Normalize(equivalent empty kind): %v", err)
	}
	if first.LogicalSessionKeyID != second.LogicalSessionKeyID || first.LogicalSessionKeyID != sharedKey {
		t.Fatalf("equivalent keys = %q and %q, want %q", first.LogicalSessionKeyID, second.LogicalSessionKeyID, sharedKey)
	}

	namedResolved, err := fake.Normalize(named)
	if err != nil {
		t.Fatalf("Normalize(named): %v", err)
	}
	if namedResolved.LogicalSessionKeyID == sharedKey || namedResolved.Reference.NamedTarget != "beta" {
		t.Fatalf("named identity = %#v, want distinct named key", namedResolved)
	}

	providerResolved, err := fake.NormalizeProvider(provider)
	if err != nil {
		t.Fatalf("NormalizeProvider: %v", err)
	}
	if providerResolved.LogicalSessionKeyID == "" || providerResolved.Reference.Provider == nil {
		t.Fatalf("provider identity = %#v, want provider boundary result", providerResolved)
	}
}

func TestIdentityRootContract_TypedFailuresAreDistinct(t *testing.T) {
	t.Parallel()

	fake := newPeerIdentitySurfaceFake()
	malformed := IdentityNormalizeRequest{
		BackendScopeID: "backend-a",
		FolderPath:     "/workspace/factories/demo",
		Target: TargetRef{
			Kind: TargetKindDefault,
			Name: "beta",
		},
	}
	ambiguous := IdentityNormalizeRequest{
		BackendScopeID: "backend-a",
		FolderPath:     "/workspace/factories/demo",
		Target:         TargetRef{Kind: "unsupported"},
	}
	missing := IdentityNormalizeRequest{
		BackendScopeID: "backend-a",
		FolderPath:     "/workspace/factories/missing",
		Target:         TargetRef{Kind: TargetKindNamed, Name: "gone"},
	}

	fake.failures[identityRequestSignature(malformed)] = ErrLogicalTargetInvalid
	fake.failures[identityRequestSignature(ambiguous)] = ErrLogicalTargetAmbiguous

	cases := []struct {
		name    string
		request IdentityNormalizeRequest
		want    error
	}{
		{name: "malformed", request: malformed, want: ErrLogicalTargetInvalid},
		{name: "ambiguous", request: ambiguous, want: ErrLogicalTargetAmbiguous},
		{name: "not_found", request: missing, want: ErrLogicalTargetNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fake.Normalize(tc.request)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Normalize error = %v, want %v", err, tc.want)
			}
		})
	}

	if errors.Is(ErrLogicalTargetInvalid, ErrLogicalTargetAmbiguous) ||
		errors.Is(ErrLogicalTargetInvalid, ErrLogicalTargetNotFound) ||
		errors.Is(ErrLogicalTargetAmbiguous, ErrLogicalTargetNotFound) {
		t.Fatal("identity typed failures must remain distinguishable sentinels")
	}
}

func ptr(value string) *string { return &value }
