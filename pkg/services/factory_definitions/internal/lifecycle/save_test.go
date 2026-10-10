package lifecycle_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
)

// saveHost controls every peer operation; Lifecycle alone chooses the target,
// stamps the version, sequences activation and owns the rollback decision.
type saveHost struct {
	lifecycle.Host
	t         *testing.T
	ctx       context.Context
	session   *definitions.DefinitionSession
	name      string
	current   *definitions.FactorySnapshot
	version   definitions.FactoryVersion
	prepared  *definitions.PreparedFactoryLayoutPayload
	validated *definitions.FactorySnapshot
	events    []string
	fail      string
	cause     error
	existing  bool
	locked    bool
}

func (h *saveHost) event(stage string) error {
	h.events = append(h.events, stage)
	if stage == h.fail {
		return h.cause
	}
	return nil
}
func (h *saveHost) PersistRootDir() string { return "fallback" }
func (h *saveHost) RequireSession(id string) (*definitions.DefinitionSession, error) {
	if id != h.session.ID {
		h.t.Fatal("wrong session")
	}
	return h.session, h.event("session")
}
func (h *saveHost) SessionFactoryPersistRoot(session *definitions.DefinitionSession) string {
	if session != h.session {
		h.t.Fatal("wrong persistence session")
	}
	return "root"
}
func (h *saveHost) GetCurrentFactorySnapshotForSession(ctx context.Context, id string) (*definitions.FactorySnapshot, error) {
	if ctx != h.ctx || id != h.session.ID {
		h.t.Fatal("wrong read context")
	}
	stage := "current"
	if h.locked {
		stage = "readback"
	}
	return h.current, h.event(stage)
}
func (h *saveHost) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *definitions.FactorySnapshot) error {
	if ctx != h.ctx {
		h.t.Fatal("wrong validation context")
	}
	h.validated = snapshot
	return h.event("validate")
}
func (h *saveHost) ResolveExistingFactoryDir(root, name string) (string, error) {
	if root != "root" || name != h.name {
		h.t.Fatalf("lookup = %q, %q", root, name)
	}
	if !h.existing {
		return "", definitions.ErrNamedFactoryNotFound
	}
	return filepath.Join(root, name), nil
}
func (*saveHost) WorkstationLoader() definitions.WorkstationLoader { return nil }
func (h *saveHost) LoadFactory(dir string, loader definitions.WorkstationLoader) (definitions.MutableLoadedFactorySource, error) {
	if dir != h.target() || loader != nil || !h.locked {
		h.t.Fatal("wrong version source")
	}
	return versionSource{config: &definitions.FactoryConfig{Version: &h.version}}, h.event("version")
}
func (h *saveHost) target() string {
	if h.name == definitions.DefaultCurrentFactoryName {
		return "root"
	}
	return filepath.Join("root", h.name)
}
func (h *saveHost) PrepareFactoryLayoutPayload(segment string, payload []byte) (*definitions.PreparedFactoryLayoutPayload, error) {
	var input map[string]any
	if err := json.Unmarshal(payload, &input); err != nil {
		h.t.Fatal(err)
	}
	if !h.locked || segment != h.name || input["name"] != h.name || input["body"] != "replacement" || input["version"] != nil {
		h.t.Fatalf("preparation = %q, %#v, locked %t", segment, input, h.locked)
	}
	return h.prepared, h.event("prepare")
}
func (h *saveHost) checkPrepared(prepared *definitions.PreparedFactoryLayoutPayload) {
	if !h.locked || prepared.Config != h.prepared.Config {
		h.t.Fatal("wrong prepared payload")
	}
	var decoded struct {
		Version struct {
			Logical  string
			Physical time.Time
		}
	}
	if err := json.Unmarshal(prepared.Canonical, &decoded); err != nil {
		h.t.Fatal(err)
	}
	wantLogical := "2"
	if !h.existing {
		wantLogical = "1"
	}
	if decoded.Version.Logical != wantLogical || !decoded.Version.Physical.Equal(time.Unix(200, 0)) {
		h.t.Fatalf("persisted version = %#v", decoded.Version)
	}
}
func (h *saveHost) ReplaceFactoryLayoutAtDir(dir string, prepared *definitions.PreparedFactoryLayoutPayload) (*definitions.FactorySplitLayoutReplaceResult, error) {
	if dir != h.target() {
		h.t.Fatalf("replace target = %q", dir)
	}
	h.checkPrepared(prepared)
	return &definitions.FactorySplitLayoutReplaceResult{
		Restore: func() {
			if !h.locked {
				h.t.Fatal("restore outside lock")
			}
			_ = h.event("restore")
		},
		DiscardBackup: func() {
			if !h.locked {
				h.t.Fatal("discard outside lock")
			}
			_ = h.event("discard")
		},
	}, h.event("replace")
}
func (h *saveHost) PersistNamedFactoryWithPrepared(root, name string, prepared *definitions.PreparedFactoryLayoutPayload) (string, error) {
	if root != "root" || name != h.name {
		h.t.Fatal("wrong create target")
	}
	h.checkPrepared(prepared)
	err := h.event("persist")
	if err == nil {
		h.existing = true
	}
	return h.target(), err
}
func (h *saveHost) WriteCurrentFactoryPointer(root, name string) error {
	if !h.locked || root != "root" || name != h.name {
		h.t.Fatal("wrong pointer target")
	}
	return h.event("pointer")
}
func (h *saveHost) SessionRuntimeConfig(id string) (definitions.LoadedFactorySource, error) {
	if id != h.session.ID || !h.locked {
		h.t.Fatal("wrong runtime session")
	}
	return readSource{versionSource: versionSource{config: h.prepared.Config}, dir: h.target()}, h.event("runtime")
}
func (h *saveHost) PreparePortableFactoryConfig(dir string, config *definitions.FactoryConfig, inline bool) (*definitions.FactoryConfig, error) {
	if dir != h.target() || config != h.prepared.Config || inline {
		h.t.Fatal("upsert response must be thin and retain source")
	}
	return config, nil
}
func (h *saveHost) CaptureFactorySnapshot(dir string, config *definitions.FactoryConfig, lookup definitions.RuntimeDefinitionLookup, base string, metadata map[string]string) (*definitions.FactorySnapshot, error) {
	if dir != h.target() || base != dir || config != h.prepared.Config || lookup == nil || metadata != nil {
		h.t.Fatal("wrong capture source")
	}
	return h.current, nil
}

type saveGateway struct {
	definitions.DefinitionActivationGateway
	host *saveHost
}

func (g saveGateway) WithActivationLock(fn func() error) error {
	g.host.locked = true
	defer func() { g.host.locked = false }()
	return fn()
}
func (g saveGateway) SaveNow() time.Time { return time.Unix(200, 0) }
func (g saveGateway) RequireIdleRuntimeForSession(ctx context.Context, id string) error {
	if ctx != g.host.ctx || id != g.host.session.ID || !g.host.locked {
		g.host.t.Fatal("wrong idle gate")
	}
	return g.host.event("idle")
}
func (g saveGateway) ActivateSessionEditableFactory(ctx context.Context, session *definitions.DefinitionSession, id, root, dir, name, display string) error {
	h := g.host
	if !h.locked || ctx != h.ctx || session != h.session || id != session.ID || root != "root" || dir != h.target() || name != h.name || display != name {
		h.t.Fatal("activation lost selected identity")
	}
	return h.event("activate")
}
func newSaveFixture(t *testing.T, name string, existing bool) (*saveHost, *lifecycle.Service, definitions.EditableFactory) {
	t.Helper()
	snapshot, err := definitions.NewFactorySnapshot(map[string]any{"name": name, "body": "replacement"})
	if err != nil {
		t.Fatal(err)
	}
	host := &saveHost{t: t, ctx: t.Context(), session: &definitions.DefinitionSession{ID: "selected", FolderPath: "root"}, name: name, current: snapshot,
		version: definitions.FactoryVersion{Logical: 1, Physical: time.Unix(100, 0)}, existing: existing,
		prepared: &definitions.PreparedFactoryLayoutPayload{Config: &definitions.FactoryConfig{Name: name}, Canonical: []byte(`{"name":"` + name + `"}`)}}
	disabled := definitions.UnimplementedService{}
	service := lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(host, saveGateway{host: host}, disabled, disabled, disabled, disabled, disabled, disabled, nil, disabled.ListEffectiveFactories, disabled)
	if len(host.events) != 0 {
		t.Fatal("construction performed effects")
	}
	return host, service, definitions.EditableFactory{Name: name, Snapshot: snapshot, Version: &definitions.FactoryVersion{Logical: 2, Physical: time.Unix(101, 0)}}
}

func TestReplaceCurrentSaveSequencesSelectedLayoutAndRollback(t *testing.T) {
	t.Parallel()
	for _, name := range []string{definitions.DefaultCurrentFactoryName, "alpha"} {
		for _, failure := range []string{"", "validate", "idle", "version", "prepare", "replace", "activate", "readback"} {
			t.Run(name+"/"+failure, func(t *testing.T) {
				t.Parallel()
				h, service, request := newSaveFixture(t, name, true)
				h.fail, h.cause = failure, errors.New("selected dependency failed")
				// Replace-current uses the selected current name, not the submitted name.
				request.Snapshot, _ = request.Snapshot.WithName("submitted")
				got, err := service.SaveReplaceCurrentSnapshotForSession(h.ctx, h.session.ID, request)
				want := []string{"session", "current", "validate", "idle", "version", "prepare", "replace", "activate", "discard", "readback"}
				if failure != "" {
					for i, stage := range want {
						if stage == failure {
							want = want[:i+1]
							break
						}
					}
					if failure == "activate" {
						want = append(want, "restore")
					}
					if !errors.Is(err, h.cause) || got != nil {
						t.Fatalf("failure = %v, %v", got, err)
					}
				} else {
					if err != nil || got == h.current || !reflect.DeepEqual(got, h.current) {
						t.Fatalf("readback = %v, %v", got, err)
					}
				}
				if !reflect.DeepEqual(h.events, want) || h.locked {
					t.Fatalf("effects = %v, want %v", h.events, want)
				}
				var identity struct{ Name string }
				if err := h.validated.Decode(&identity); err != nil || identity.Name != name {
					t.Fatalf("sanitized identity = %q, %v", identity.Name, err)
				}
			})
		}
	}
}

func TestNamedUpsertSaveCreatesOrReplacesAndActivatesChosenTarget(t *testing.T) {
	t.Parallel()
	for _, existing := range []bool{false, true} {
		for _, failure := range []string{"", "prepare", "write", "pointer", "activate", "runtime"} {
			t.Run(map[bool]string{false: "create", true: "replace"}[existing]+"/"+failure, func(t *testing.T) {
				t.Parallel()
				h, service, request := newSaveFixture(t, "imported-target", existing)
				h.fail, h.cause = failure, errors.New("upsert dependency failed")
				write := "persist"
				if existing {
					write = "replace"
				}
				if failure == "write" {
					h.fail = write
				}
				got, err := service.SaveUpsertNamedSnapshotAndActivateForSession(h.ctx, h.session.ID, request)
				want := []string{"validate", "session", "idle"}
				if existing {
					want = append(want, "version")
				}
				want = append(want, "prepare", write, "pointer", "activate", "runtime")
				if failure != "" {
					for i, stage := range want {
						if stage == h.fail {
							want = want[:i+1]
							break
						}
					}
					if !errors.Is(err, h.cause) || got.Snapshot != nil {
						t.Fatalf("failure = %#v, %v", got, err)
					}
				} else {
					want = append(want, "version")
					if err != nil || got.Name != h.name || got.Version == nil || got.Version.Logical != h.version.Logical || !got.Version.Physical.Equal(h.version.Physical) {
						t.Fatalf("upsert = %#v, %v", got, err)
					}
					var decoded map[string]any
					if err := got.Snapshot.Decode(&decoded); err != nil || !reflect.DeepEqual(decoded, map[string]any{"name": h.name, "body": "replacement"}) {
						t.Fatalf("upsert payload = %#v, %v", decoded, err)
					}
				}
				if !reflect.DeepEqual(h.events, want) || h.locked {
					t.Fatalf("effects = %v, want %v", h.events, want)
				}
			})
		}
	}
}

func TestSaveRejectsInvalidNamesAndRetainsTopologyError(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"bad/name", definitions.DefaultCurrentFactoryName} {
		h, service, request := newSaveFixture(t, name, true)
		if err := service.ValidateUpsertNamedFactoryRequest(h.ctx, name, request.Snapshot); !errors.Is(err, definitions.ErrInvalidNamedFactoryName) || len(h.events) != 0 {
			t.Fatalf("invalid upsert = %v, effects %v", err, h.events)
		}
	}
	h, service, request := newSaveFixture(t, "bad/name", true)
	if _, err := service.SaveReplaceCurrentSnapshotForSession(h.ctx, h.session.ID, request); !errors.Is(err, definitions.ErrInvalidNamedFactoryName) || !reflect.DeepEqual(h.events, []string{"session", "current"}) {
		t.Fatalf("invalid current = %v, effects %v", err, h.events)
	}
	h, service, request = newSaveFixture(t, "alpha", true)
	h.fail, h.cause = "validate", &definitions.ValidationTopologyError{}
	if err := service.ValidateEditableFactoryTopology(h.ctx, request.Snapshot); err != h.cause || h.validated != request.Snapshot {
		t.Fatalf("topology error/request identity lost: %v", err)
	}
}
