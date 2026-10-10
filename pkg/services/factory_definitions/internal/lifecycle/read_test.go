package lifecycle_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
)

type activationGateway struct {
	definitions.DefinitionActivationGateway
	t           *testing.T
	session     *definitions.DefinitionSession
	idleError   error
	swapError   error
	locked      bool
	swappedName string
	swapCalls   int
}

func (g *activationGateway) RunSessionID() string { return g.session.ID }
func (g *activationGateway) SessionForActivation(id string) *definitions.DefinitionSession {
	if id != g.session.ID || !g.locked {
		g.t.Fatal("activation did not retain session identity and lock")
	}
	return g.session
}
func (g *activationGateway) WithActivationLock(fn func() error) error {
	g.locked = true
	defer func() { g.locked = false }()
	return fn()
}
func (g *activationGateway) NamedFactoryActivationPaths(session *definitions.DefinitionSession) (string, string) {
	if session != g.session {
		g.t.Fatal("activation selected a different session")
	}
	return "root", "folder"
}
func (g *activationGateway) RequireIdleBeforeNamedFactoryActivation(_ context.Context, id string, session *definitions.DefinitionSession) error {
	if id != g.session.ID || session != g.session || !g.locked {
		g.t.Fatal("idle gate lost selected session or lock")
	}
	return g.idleError
}
func (g *activationGateway) SwapPersistedNamedFactoryRuntime(_ context.Context, id string, session *definitions.DefinitionSession, root, folder, dir, name string) error {
	if !g.locked || id != g.session.ID || session != g.session || root != "root" || folder != "folder" || dir != filepath.Join("root", "alpha") {
		g.t.Fatalf("swap lost activation context: %q, %v, %q, %q, %q", id, session, root, folder, dir)
	}
	g.swapCalls++
	g.swappedName = name
	return g.swapError
}

type activationHost struct {
	lifecycle.Host
	t           *testing.T
	lookupError error
	lookupCalls int
}

func (h *activationHost) ResolveExistingFactoryDir(root, name string) (string, error) {
	h.lookupCalls++
	if root != "root" || name != "alpha" {
		h.t.Fatalf("lookup arguments = %q, %q", root, name)
	}
	return filepath.Join(root, name), h.lookupError
}

func TestNamedActivationGatesLookupAndSwapAndRetainsSelectedSession(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"success", "busy", "lookup", "swap"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			cause := errors.New("activation dependency rejected")
			host := &activationHost{t: t}
			gateway := &activationGateway{t: t, session: &definitions.DefinitionSession{ID: "session-alpha"}}
			wantError, wantLookup, wantSwap := error(nil), 1, 1
			switch stage {
			case "busy":
				gateway.idleError, wantError, wantLookup, wantSwap = cause, cause, 0, 0
			case "lookup":
				host.lookupError, wantError, wantSwap = cause, cause, 0
			case "swap":
				gateway.swapError, wantError = cause, cause
			}
			disabled := definitions.UnimplementedService{}
			service := lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(host, gateway, disabled, disabled, disabled, disabled, disabled, disabled, nil, disabled.ListEffectiveFactories, disabled)
			if host.lookupCalls != 0 || gateway.swapCalls != 0 {
				t.Fatal("construction performed activation effects")
			}
			err := service.ActivateNamedFactory(t.Context(), "alpha")
			if err != wantError || host.lookupCalls != wantLookup || gateway.swapCalls != wantSwap || gateway.locked {
				t.Fatalf("activation = %v, lookup %d, swap %d, lock %t; want %v, %d, %d, false", err, host.lookupCalls, gateway.swapCalls, gateway.locked, wantError, wantLookup, wantSwap)
			}
			if wantSwap != 0 && gateway.swappedName != "alpha" {
				t.Fatalf("swapped name = %q", gateway.swappedName)
			}
			if stage == "busy" {
				gateway.idleError = nil
				if err := service.ActivateNamedFactory(t.Context(), "alpha"); err != nil || gateway.swapCalls != 1 || gateway.swappedName != "alpha" {
					t.Fatalf("activation after idle = %v, swap %d, name %q", err, gateway.swapCalls, gateway.swappedName)
				}
			}
		})
	}
}

type readHost struct {
	*versionHost
	root                                                   string
	pointerError, sessionError, runtimeError, captureError error
}

func (h readHost) PersistRootDir() string                           { return h.root }
func (h readHost) ReadCurrentFactoryPointer(string) (string, error) { return "alpha", h.pointerError }
func (h readHost) RequireSession(string) (*definitions.DefinitionSession, error) {
	return &definitions.DefinitionSession{FolderPath: h.root}, h.sessionError
}
func (h readHost) SessionRuntimeConfig(string) (definitions.LoadedFactorySource, error) {
	return h.source, h.runtimeError
}
func (h readHost) SessionFactoryPersistRoot(*definitions.DefinitionSession) string { return h.root }
func (h readHost) CurrentRuntimeConfig() definitions.LoadedFactorySource           { return h.source }
func (h readHost) PreparePortableFactoryConfig(_ string, config *definitions.FactoryConfig, _ bool) (*definitions.FactoryConfig, error) {
	return config, nil
}
func (h readHost) CaptureFactorySnapshot(string, *definitions.FactoryConfig, definitions.RuntimeDefinitionLookup, string, map[string]string) (*definitions.FactorySnapshot, error) {
	if h.captureError != nil {
		return nil, h.captureError
	}
	return definitions.NewFactorySnapshot(map[string]any{"name": "loaded", "project": "preserved"})
}

type readSource struct {
	versionSource
	dir string
}

func (s readSource) FactoryDir() string { return s.dir }

func newReadHost() readHost {
	return readHost{root: "root", versionHost: &versionHost{source: readSource{
		versionSource: versionSource{config: &definitions.FactoryConfig{Version: &definitions.FactoryVersion{Logical: 7, Physical: time.Unix(100, 0).UTC()}}},
		dir:           filepath.Join("root", "alpha"),
	}}}
}

func TestCurrentSessionReadReturnsNamedSnapshotAndDurableVersion(t *testing.T) {
	t.Parallel()
	host := newReadHost()
	got, err := injectedLifecycle(host, nil).GetCurrentFactoryForSession(context.Background(), "session")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "alpha" || got.Version == nil || *got.Version != *host.source.FactoryConfig().Version {
		t.Fatalf("editable factory = %#v", got)
	}
	assertReadSnapshot(t, got.Snapshot, "alpha")
}

func assertReadSnapshot(t *testing.T, snapshot *definitions.FactorySnapshot, name string) {
	t.Helper()
	var result struct{ Name, Project string }
	if snapshot == nil {
		t.Fatal("missing snapshot")
	}
	if err := snapshot.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Name != name || result.Project != "preserved" {
		t.Fatalf("snapshot = %#v", result)
	}
}

func TestCurrentNamedReadPreservesSourceAndFailureCauses(t *testing.T) {
	t.Parallel()
	cause := errors.New("read dependency unavailable")
	for _, stage := range []string{"success", "pointer", "lookup", "load", "capture", "missing pointer"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			host := newReadHost()
			wantError := cause
			switch stage {
			case "success":
				wantError = nil
			case "pointer":
				host.pointerError = cause
			case "lookup":
				host.resolveError = cause
			case "load":
				host.loadError = cause
			case "capture":
				host.captureError = cause
			case "missing pointer":
				host.pointerError = fs.ErrNotExist
				wantError = definitions.ErrCurrentFactoryNotFound
			}
			got, err := injectedLifecycle(host, nil).GetCurrentNamedFactory(context.Background())
			if wantError != nil {
				if got != nil || !errors.Is(err, wantError) {
					t.Fatalf("read = %#v, %v; want cause %v", got, err, wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertReadSnapshot(t, got, "alpha")
		})
	}
}

func TestCurrentSessionReadRetainsFailuresWithoutFabricatingSnapshot(t *testing.T) {
	t.Parallel()
	cause := errors.New("session read unavailable")
	for _, stage := range []string{"session", "runtime", "capture", "version"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			host := newReadHost()
			switch stage {
			case "session":
				host.sessionError = cause
			case "runtime":
				host.runtimeError = cause
			case "capture":
				host.captureError = cause
			case "version":
				host.loadError = cause
			}
			got, err := injectedLifecycle(host, nil).GetCurrentFactoryForSession(context.Background(), "session")
			if !errors.Is(err, cause) || got.Snapshot != nil || got.Version != nil {
				t.Fatalf("read = %#v, %v", got, err)
			}
		})
	}
}

func TestMissingPointerUsesOnlyRuntimeAtThePersistRoot(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"same root", "different root", "no runtime"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			host := newReadHost()
			host.pointerError = fs.ErrNotExist
			switch stage {
			case "same root":
				host.source = readSource{versionSource: versionSource{config: &definitions.FactoryConfig{}}, dir: host.root}
			case "no runtime":
				host.source = nil
			}
			got, err := injectedLifecycle(host, nil).GetCurrentNamedFactory(t.Context())
			if stage != "same root" {
				if got != nil || !errors.Is(err, definitions.ErrCurrentFactoryNotFound) {
					t.Fatalf("missing pointer read = %#v, %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertReadSnapshot(t, got, definitions.DefaultCurrentFactoryName)
		})
	}
}

func TestNamedReadAndActivationPreserveMissingDefinitionClassification(t *testing.T) {
	t.Parallel()
	host := newReadHost()
	host.resolveError = definitions.ErrNamedFactoryNotFound
	if got, err := injectedLifecycle(host, nil).GetCurrentNamedFactory(t.Context()); got != nil || !errors.Is(err, definitions.ErrNamedFactoryNotFound) {
		t.Fatalf("missing named read = %#v, %v", got, err)
	}
	gateway := &activationGateway{t: t, session: &definitions.DefinitionSession{ID: "selected"}}
	activation := &activationHost{t: t, lookupError: definitions.ErrNamedFactoryNotFound}
	disabled := definitions.UnimplementedService{}
	service := lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(activation, gateway, disabled, disabled, disabled, disabled, disabled, disabled, nil, disabled.ListEffectiveFactories, disabled)
	if err := service.ActivateNamedFactory(t.Context(), "alpha"); !errors.Is(err, definitions.ErrNamedFactoryNotFound) || gateway.swapCalls != 0 {
		t.Fatalf("missing named activation = %v, swaps %d", err, gateway.swapCalls)
	}
}

func TestService_NilReceiverReturnsRequiredErrors(t *testing.T) {
	t.Parallel()

	var svc *lifecycle.Service
	if _, err := svc.GetCurrentNamedFactory(context.Background()); err == nil {
		t.Fatal("GetCurrentNamedFactory: expected error for nil service")
	}
	if _, err := svc.GetCurrentFactoryForSession(context.Background(), "session"); err == nil {
		t.Fatal("GetCurrentFactoryForSession: expected error for nil service")
	}
	if _, err := svc.CurrentFactoryDefinitionVersionAtRoot("root", "alpha"); err == nil {
		t.Fatal("CurrentFactoryDefinitionVersionAtRoot: expected error for nil service")
	}
	if _, err := svc.SerializeNamedFactory("alpha", nil, true); err == nil {
		t.Fatal("SerializeNamedFactory: expected error for nil service")
	}
	if _, err := svc.PrepareEditableFactoryPersistView("alpha", nil); err == nil {
		t.Fatal("PrepareEditableFactoryPersistView: expected error for nil service")
	}
	if _, err := svc.PersistPayloadFromView(nil, definitions.FactoryVersion{}); err == nil {
		t.Fatal("PersistPayloadFromView: expected error for nil service")
	}
	if _, err := svc.PreparePersistedFactoryPayload("alpha", nil, definitions.FactoryVersion{}); err == nil {
		t.Fatal("PreparePersistedFactoryPayload: expected error for nil service")
	}
	if err := svc.ValidateEditableFactoryTopology(context.Background(), nil); err == nil {
		t.Fatal("ValidateEditableFactoryTopology: expected error for nil service")
	}
	if _, err := svc.SaveReplaceCurrentSnapshotForSession(context.Background(), "session", definitions.EditableFactory{}); err == nil {
		t.Fatal("SaveReplaceCurrentForSession: expected error for nil service")
	}
	if _, err := svc.SaveUpsertNamedSnapshotAndActivateForSession(context.Background(), "session", definitions.EditableFactory{}); err == nil {
		t.Fatal("SaveUpsertNamedAndActivateForSession: expected error for nil service")
	}
	if err := svc.ActivateNamedFactory(context.Background(), "alpha"); err == nil {
		t.Fatal("ActivateNamedFactory: expected error for nil service")
	}
}

type persistHost struct {
	lifecycle.Host
	t     *testing.T
	view  *definitions.PreparedFactoryLayoutPayload
	err   error
	calls int
}

func (h *persistHost) PrepareFactoryLayoutPayload(segment string, payload []byte) (*definitions.PreparedFactoryLayoutPayload, error) {
	h.calls++
	var input map[string]any
	if err := json.Unmarshal(payload, &input); err != nil {
		h.t.Fatal(err)
	}
	if segment != "alpha" || input["name"] != "alpha" || input["body"] != "editable body" {
		h.t.Fatalf("preparation input = %q, %#v", segment, input)
	}
	if _, exists := input["version"]; exists {
		h.t.Fatal("editable preparation retained concurrency metadata")
	}
	return h.view, h.err
}
func (*persistHost) PersistNamedFactoryWithPrepared(string, string, *definitions.PreparedFactoryLayoutPayload) (string, error) {
	panic("unexpected persistence")
}
func (*persistHost) WriteCurrentFactoryPointer(string, string) error {
	panic("unexpected pointer write")
}

func TestPersistViewDelegationStripsVersionAndPreservesSelectedResult(t *testing.T) {
	t.Parallel()
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			t.Parallel()
			view := &definitions.PreparedFactoryLayoutPayload{Config: &definitions.FactoryConfig{Name: "alpha"}, Canonical: []byte(`{"name":"alpha"}`)}
			host := &persistHost{t: t, view: view}
			if failure {
				host.err = errors.New("preparation denied")
			}
			service := injectedLifecycle(host, nil)
			if host.calls != 0 {
				t.Fatal("construction prepared a payload")
			}
			snapshot, err := definitions.NewFactorySnapshot(map[string]any{"name": "alpha", "body": "editable body", "version": map[string]any{"logical": "1"}})
			if err != nil {
				t.Fatal(err)
			}
			got, err := service.PrepareEditableFactoryPersistView("alpha", snapshot)
			if got != view || err != host.err || host.calls != 1 {
				t.Fatalf("prepare = %p, %v, calls %d", got, err, host.calls)
			}
			stamped, err := service.PreparePersistedFactoryPayload("alpha", snapshot, definitions.FactoryVersion{Logical: 9, Physical: time.Unix(100, 0)})
			if host.calls != 2 || err != host.err {
				t.Fatalf("stamped prepare = %v, calls %d", err, host.calls)
			}
			if failure {
				if stamped != nil {
					t.Fatal("preparation failure returned persisted payload")
				}
			} else if stamped.Config != view.Config || !strings.Contains(string(stamped.Canonical), `"logical":"9"`) {
				t.Fatalf("stamped payload lost selected config/version: %#v", stamped)
			}
			if _, err := service.PreparePersistedFactoryPayload("alpha", nil, definitions.FactoryVersion{}); err == nil || !strings.Contains(err.Error(), "editable factory snapshot is required") || host.calls != 2 {
				t.Fatalf("missing snapshot = %v, calls %d", err, host.calls)
			}
		})
	}
}

func TestPersistPayloadStampsUTCVersionWithoutMutatingPreparedView(t *testing.T) {
	t.Parallel()
	config := &definitions.FactoryConfig{Name: "alpha"}
	canonical := []byte(`{"name":"alpha","resourceManifest":{"bundledFiles":[]},"version":{"logical":"1"}}`)
	view := &definitions.PreparedFactoryLayoutPayload{Config: config, Canonical: canonical}
	physical := time.Date(2026, 6, 8, 8, 0, 0, 0, time.FixedZone("local", 3600))
	service := injectedLifecycle(nil, nil)
	got, err := service.PersistPayloadFromView(view, definitions.FactoryVersion{Logical: 9, Physical: physical})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(got.Canonical, &decoded); err != nil {
		t.Fatal(err)
	}
	wantVersion := map[string]any{"logical": "9", "physical": physical.UTC().Format(time.RFC3339Nano)}
	if got.Config != config || !reflect.DeepEqual(decoded["version"], wantVersion) || decoded["name"] != "alpha" || decoded["resourceManifest"] == nil {
		t.Fatalf("stamped payload = %#v, config %p", decoded, got.Config)
	}
	if string(view.Canonical) != string(canonical) || strings.Contains(string(view.Canonical), `"logical":"9"`) {
		t.Fatal("stamping mutated original view")
	}
	if _, err := service.PersistPayloadFromView(nil, definitions.FactoryVersion{}); err == nil || !strings.Contains(err.Error(), "persist factory view is required") {
		t.Fatalf("nil view = %v", err)
	}
	_, err = service.PersistPayloadFromView(&definitions.PreparedFactoryLayoutPayload{Canonical: []byte(`{`)}, definitions.FactoryVersion{})
	var syntax *json.SyntaxError
	if !errors.As(err, &syntax) {
		t.Fatalf("invalid canonical error = %v", err)
	}
}

type serializationHost struct {
	lifecycle.Host
	t                          *testing.T
	source                     definitions.LoadedFactorySource
	portable                   *definitions.FactoryConfig
	prepareError, captureError error
	inline                     bool
	prepareCalls, captureCalls int
}

func (h *serializationHost) PreparePortableFactoryConfig(dir string, config *definitions.FactoryConfig, inline bool) (*definitions.FactoryConfig, error) {
	h.prepareCalls++
	if dir != h.source.FactoryDir() || config != h.source.FactoryConfig() {
		h.t.Fatal("portable preparation lost source identity")
	}
	h.inline = inline
	return h.portable, h.prepareError
}
func (h *serializationHost) CaptureFactorySnapshot(dir string, config *definitions.FactoryConfig, lookup definitions.RuntimeDefinitionLookup, base string, metadata map[string]string) (*definitions.FactorySnapshot, error) {
	h.captureCalls++
	if dir != h.source.FactoryDir() || base != dir || lookup != h.source || config != h.portable || metadata != nil {
		h.t.Fatal("capture lost prepared source identity")
	}
	if h.captureError != nil {
		return nil, h.captureError
	}
	return definitions.NewFactorySnapshot(map[string]any{"name": "loaded", "project": "preserved"})
}
func TestNamedSerializationRetainsPreparedSourceAndRejectsFailedPreparation(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"inline", "disk", "upsert"} {
		for _, failure := range []string{"success", "prepare", "capture"} {
			t.Run(mode+"/"+failure, func(t *testing.T) {
				t.Parallel()
				source := readSource{versionSource: versionSource{config: &definitions.FactoryConfig{Name: "loaded"}}, dir: "selected"}
				host := &serializationHost{t: t, source: &source, portable: &definitions.FactoryConfig{Name: "portable"}}
				if mode == "disk" {
					host.portable = source.config
				}
				cause := errors.New("serialization dependency failed")
				if failure == "prepare" {
					host.prepareError = cause
				}
				if failure == "capture" {
					host.captureError = cause
				}
				service := injectedLifecycle(host, nil)
				if host.prepareCalls != 0 || host.captureCalls != 0 {
					t.Fatal("construction serialized a source")
				}
				var snapshot *definitions.FactorySnapshot
				var err error
				if mode == "upsert" {
					snapshot, err = service.SerializeNamedFactoryUpsertResponse("alpha", &source)
				} else {
					snapshot, err = service.SerializeNamedFactory("alpha", &source, mode == "inline")
				}
				wantPrepare, wantCapture := 1, 1
				if mode == "disk" {
					wantPrepare = 0
				}
				if failure == "prepare" && mode != "disk" {
					wantCapture = 0
				}
				if host.prepareCalls != wantPrepare || host.captureCalls != wantCapture || host.inline != (mode == "inline") {
					t.Fatalf("calls = %d/%d, inline %t", host.prepareCalls, host.captureCalls, host.inline)
				}
				if failure == "capture" || (failure == "prepare" && mode != "disk") {
					if !errors.Is(err, cause) || snapshot != nil {
						t.Fatalf("serialization failure = %v, snapshot %v", err, snapshot)
					}
				} else if err != nil {
					t.Fatal(err)
				} else {
					assertReadSnapshot(t, snapshot, "alpha")
				}
			})
		}
	}
}
