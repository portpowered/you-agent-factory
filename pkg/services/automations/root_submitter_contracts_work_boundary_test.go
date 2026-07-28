package automations_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// peerAutomationsWorkSubmitter is a peer-shaped consumer that wires Automations
// root submitter contracts using only Work root types. It must not import Work
// implementation packages to admit or assert Work Requests.
type peerAutomationsWorkSubmitter struct {
	workRequestSubmitter automations.WorkRequestSubmitter
	hostedWorkSubmitter  automations.HostedWorkSubmitter
	watcherConfig        automations.FilesystemWatcherConfig
}

func (p peerAutomationsWorkSubmitter) admitThroughWorkRequestSubmitter(
	ctx context.Context,
	request work.WorkRequest,
) error {
	return p.workRequestSubmitter(ctx, request)
}

func (p peerAutomationsWorkSubmitter) admitThroughHostedWorkSubmitter(
	ctx context.Context,
	request work.WorkRequest,
) error {
	return p.hostedWorkSubmitter(ctx, request)
}

func (p peerAutomationsWorkSubmitter) admitThroughFilesystemWatcherConfig(
	ctx context.Context,
	request work.WorkRequest,
) error {
	if p.watcherConfig.Submitter == nil {
		return nil
	}
	return p.watcherConfig.Submitter(ctx, request)
}

func newPeerAutomationsWorkSubmitter(
	capture *work.WorkRequest,
) peerAutomationsWorkSubmitter {
	submit := func(_ context.Context, request work.WorkRequest) error {
		*capture = request
		return nil
	}
	return peerAutomationsWorkSubmitter{
		workRequestSubmitter: automations.WorkRequestSubmitter(submit),
		hostedWorkSubmitter:  automations.HostedWorkSubmitter(submit),
		watcherConfig: automations.FilesystemWatcherConfig{
			WorkRequestIDs: func() string { return "peer-root-submitter-id" },
			Submitter:      automations.WorkRequestSubmitter(submit),
		},
	}
}

func sampleWorkRootRequestForPeerSubmitter() work.WorkRequest {
	return work.WorkRequestFromSubmitRequests([]work.SubmitRequest{{
		RequestID:  "peer-root-submitter-request",
		WorkTypeID: "story",
		Name:       "peer-root-submitter-work",
		TargetState: "draft",
		Payload:    []byte(`{"source":"automations-root-submitter-contract"}`),
		Tags: map[string]string{
			"source": "automations-root-submitter-contract",
		},
	}})
}

// TestRootSubmitterContracts_ReferenceOnlyWorkRootTypes proves Automations root
// submitter and watcher config contracts expose only Work root types on the
// peer surface.
func TestRootSubmitterContracts_ReferenceOnlyWorkRootTypes(t *testing.T) {
	t.Parallel()

	contractSamples := []any{
		automations.WorkRequestSubmitter(nil),
		automations.HostedWorkSubmitter(nil),
		automations.FilesystemWatcherConfig{},
	}
	for _, sample := range contractSamples {
		assertAutomationsContractUsesOnlyWorkRootTypes(t, reflect.TypeOf(sample))
	}
}

func assertAutomationsContractUsesOnlyWorkRootTypes(t *testing.T, typ reflect.Type) {
	t.Helper()
	seen := make(map[reflect.Type]struct{})
	var walk func(reflect.Type)
	walk = func(current reflect.Type) {
		if current == nil {
			return
		}
		if _, ok := seen[current]; ok {
			return
		}
		seen[current] = struct{}{}

		switch current.Kind() {
		case reflect.Pointer:
			walk(current.Elem())
		case reflect.Func:
			if current.NumIn() > 0 {
				for i := 0; i < current.NumIn(); i++ {
					walk(current.In(i))
				}
			}
			for i := 0; i < current.NumOut(); i++ {
				walk(current.Out(i))
			}
		case reflect.Struct:
			for i := 0; i < current.NumField(); i++ {
				walk(current.Field(i).Type)
			}
		case reflect.Slice, reflect.Array, reflect.Chan, reflect.Map:
			walk(current.Elem())
			if current.Kind() == reflect.Map {
				walk(current.Key())
			}
		default:
			if current.PkgPath() != "" && isForbiddenWorkPackagePath(current.PkgPath()) {
				t.Fatalf(
					"Automations root contract type %s references forbidden Work package %s; want only %s",
					current.String(),
					current.PkgPath(),
					workRootImportPath,
				)
			}
		}
	}
	walk(typ)
}

func isForbiddenWorkPackagePath(pkgPath string) bool {
	if pkgPath == workRootImportPath {
		return false
	}
	return forbiddenWorkImport(pkgPath)
}

// TestPeerWiresRootSubmitterContracts_AdmitsWorkRootRequest proves peers can
// wire Automations root submitter contracts and admit Work Requests constructed
// only through Work root helpers without importing Work implementation packages.
func TestPeerWiresRootSubmitterContracts_AdmitsWorkRootRequest(t *testing.T) {
	t.Parallel()

	request := sampleWorkRootRequestForPeerSubmitter()
	ctx := context.Background()

	t.Run("WorkRequestSubmitter", func(t *testing.T) {
		t.Parallel()
		var admitted work.WorkRequest
		peer := newPeerAutomationsWorkSubmitter(&admitted)
		if err := peer.admitThroughWorkRequestSubmitter(ctx, request); err != nil {
			t.Fatalf("admitThroughWorkRequestSubmitter() error = %v", err)
		}
		assertPeerAdmittedWorkRootRequest(t, admitted, request)
	})

	t.Run("HostedWorkSubmitter", func(t *testing.T) {
		t.Parallel()
		var admitted work.WorkRequest
		peer := newPeerAutomationsWorkSubmitter(&admitted)
		if err := peer.admitThroughHostedWorkSubmitter(ctx, request); err != nil {
			t.Fatalf("admitThroughHostedWorkSubmitter() error = %v", err)
		}
		assertPeerAdmittedWorkRootRequest(t, admitted, request)
	})

	t.Run("FilesystemWatcherConfig", func(t *testing.T) {
		t.Parallel()
		var admitted work.WorkRequest
		peer := newPeerAutomationsWorkSubmitter(&admitted)
		if err := peer.admitThroughFilesystemWatcherConfig(ctx, request); err != nil {
			t.Fatalf("admitThroughFilesystemWatcherConfig() error = %v", err)
		}
		assertPeerAdmittedWorkRootRequest(t, admitted, request)
	})
}

func assertPeerAdmittedWorkRootRequest(
	t *testing.T,
	admitted work.WorkRequest,
	expected work.WorkRequest,
) {
	t.Helper()

	if admitted.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("admitted type = %q, want %q", admitted.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if admitted.RequestID != expected.RequestID {
		t.Fatalf("admitted request ID = %q, want %q", admitted.RequestID, expected.RequestID)
	}
	if len(admitted.Works) != 1 {
		t.Fatalf("admitted works len = %d, want 1", len(admitted.Works))
	}
	got := admitted.Works[0]
	want := expected.Works[0]
	if got.Name != want.Name {
		t.Fatalf("admitted work name = %q, want %q", got.Name, want.Name)
	}
	if got.WorkTypeID != want.WorkTypeID {
		t.Fatalf("admitted work type = %q, want %q", got.WorkTypeID, want.WorkTypeID)
	}
	if got.State != want.State {
		t.Fatalf("admitted work state = %q, want %q", got.State, want.State)
	}
	payload, ok := got.Payload.([]byte)
	if !ok {
		t.Fatalf("admitted payload type = %T, want []byte", got.Payload)
	}
	wantPayload, ok := want.Payload.([]byte)
	if !ok {
		t.Fatalf("expected payload type = %T, want []byte", want.Payload)
	}
	if string(payload) != string(wantPayload) {
		t.Fatalf("admitted payload = %q, want %q", payload, wantPayload)
	}
	if got.Tags["source"] != want.Tags["source"] {
		t.Fatalf("admitted tags = %#v, want %#v", got.Tags, want.Tags)
	}
}
