package service_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	settingsdocument "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
)

type constructionDocument struct{ settingsdocument.Service }
type constructionResolution struct{ resolution.Service }

func rootTestIDGenerator() string { return "00000000-0000-4000-8000-000000000001" }

type forwardingDocument struct {
	settingsdocument.Service
	load  func(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error)
	apply func(operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error)
}

func (d forwardingDocument) LoadDocument(r operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
	return d.load(r)
}

func (d forwardingDocument) ApplyDocumentUpdate(r operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
	return d.apply(r)
}

type forwardingResolution struct {
	resolve func(operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error)
}

func (r forwardingResolution) ResolveEffective(request operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
	return r.resolve(request)
}

func newControlledRoot(t *testing.T, document settingsdocument.Service, resolution resolution.Service) operatorsettings.Service {
	t.Helper()
	root, err := operatorservice.New(document, resolution, rootTestFileSystem{}, rootTestCreateTemporaryFile,
		rootTestConfigDecoder, rootTestConfigEncoder, rootTestIDGenerator, logging.NoopLogger{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRootForwardsRequestsResultsAndTypedFailures(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("failure=%v", fail), func(t *testing.T) {
			t.Parallel()
			loadRequest := operatorsettings.LoadDocumentRequest{Path: "selected-config", RequireExisting: true}
			model := "selected-model"
			applyRequest := operatorsettings.ApplyDocumentUpdateRequest{Path: "selected-config", ExpectedBackendScope: "scope", ProviderModel: operatorsettings.DocumentProviderModelUpdate{Model: &model}}
			resolveRequest := operatorsettings.ResolveEffectiveRequest{ConfigPath: "selected-config"}
			loadResult := operatorsettings.LoadDocumentResult{Path: "selected-config", Found: true}
			applyResult := operatorsettings.ApplyDocumentUpdateResult{Path: "selected-config"}
			resolveResult := operatorsettings.ResolveEffectiveResult{Selection: operatorsettings.EffectiveSelection{WorkerModel: model}}
			var documentError, resolutionError error
			if fail {
				documentError = operatorsettings.DocumentFailure{Kind: operatorsettings.DocumentFailureKindConflict, Message: "selected failure"}
				resolutionError = operatorsettings.ResolutionFailure{Kind: operatorsettings.ResolutionFailureKindConflict}
				loadResult = operatorsettings.LoadDocumentResult{}
			}
			root := newControlledRoot(t, forwardingDocument{
				load: func(r operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
					if !reflect.DeepEqual(r, loadRequest) {
						t.Errorf("load request = %#v", r)
					}
					return loadResult, documentError
				},
				apply: func(r operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
					if !reflect.DeepEqual(r, applyRequest) {
						t.Errorf("apply request = %#v", r)
					}
					return applyResult, documentError
				},
			}, forwardingResolution{resolve: func(r operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
				if !reflect.DeepEqual(r, resolveRequest) {
					t.Errorf("resolve request = %#v", r)
				}
				return resolveResult, resolutionError
			}})
			loaded, err := root.LoadDocument(loadRequest)
			if !reflect.DeepEqual(loaded, loadResult) || !errors.Is(err, documentError) {
				t.Fatalf("load = %#v, %v", loaded, err)
			}
			applied, err := root.ApplyDocumentUpdate(applyRequest)
			if !reflect.DeepEqual(applied, applyResult) || !errors.Is(err, documentError) {
				t.Fatalf("apply = %#v, %v", applied, err)
			}
			resolved, err := root.ResolveEffective(resolveRequest)
			if !reflect.DeepEqual(resolved, resolveResult) || !errors.Is(err, resolutionError) {
				t.Fatalf("resolve = %#v, %v", resolved, err)
			}
		})
	}
}

func TestRootMutationRejectsInvalidContextAndPreservesPersistFailure(t *testing.T) {
	t.Parallel()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"nil", nil, nil}, {"canceled", canceled, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newControlledRoot(t, &constructionDocument{}, &constructionResolution{})
			_, err := root.ConfigureACPIntegrationAdd(tc.ctx, "config", operatorsettings.ACPIntegration{})
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("mutation error = %v", err)
			}
		})
	}
	failure := errors.New("persist failure")
	document := &profileDocument{path: "config", persistError: failure}
	root := newControlledRoot(t, document, &constructionResolution{})
	_, err := root.UpdateACPAgentProfile(context.Background(), "config", operatorsettings.DefaultACPAgentProfile())
	if !errors.Is(err, failure) || document.published {
		t.Fatalf("persist = %v, published=%v", err, document.published)
	}
}

// profileDocument supplies only the document operations exercised by the root.
// Each instance owns its state, destination and controlled failures.
type profileDocument struct {
	settingsdocument.Service
	path         string
	document     operatorsettings.Document
	loadError    error
	persistError error
	entered      chan<- struct{}
	release      <-chan struct{}
	published    bool
}

func (d *profileDocument) LoadDocument(request operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
	if request.Path != d.path {
		return operatorsettings.LoadDocumentResult{}, fmt.Errorf("unexpected load destination: %s", request.Path)
	}
	if d.entered != nil {
		d.entered <- struct{}{}
		<-d.release
		d.entered = nil
	}
	return operatorsettings.LoadDocumentResult{Path: d.path, Document: d.document}, d.loadError
}

func (d *profileDocument) PersistDocument(_ context.Context, request operatorsettings.PersistDocumentRequest) error {
	if request.Path != d.path {
		return fmt.Errorf("unexpected persist destination: %s", request.Path)
	}
	if d.persistError != nil {
		return d.persistError
	}
	d.document = request.Document
	d.published = true
	return nil
}

type profileOperationResult struct {
	updated  operatorsettings.ACPAgentProfile
	resolved operatorsettings.ACPAgentProfile
	err      error
}

func exerciseSelectedProfileLogger(root operatorsettings.Service, path string, update bool) profileOperationResult {
	var result profileOperationResult
	if update {
		result.updated, result.err = root.UpdateACPAgentProfile(context.Background(), path, operatorsettings.ACPAgentProfile{
			DefaultTarget: " factory:@you/reviewer ", AllowedTargets: []string{" factory:@you/reviewer "},
		})
		if result.err != nil {
			return result
		}
	}
	result.resolved, result.err = root.ResolveACPAgentProfile(path)
	return result
}

func TestSelectedProfileLoggersPreserveOutcomesAndIsolateConcurrentEffects(t *testing.T) {
	t.Parallel()
	secret := errors.New("credential-and-raw-config-sentinel")
	for _, tc := range []struct {
		name         string
		loadError    error
		persistError error
		update       bool
		reason       string
	}{
		{name: "update and readback", update: true},
		{name: "decode failure", loadError: operatorsettings.DocumentFailure{Kind: operatorsettings.DocumentFailureKindMalformed, Message: secret.Error()}, reason: "document_malformed"},
		{name: "read failure", loadError: fmt.Errorf("read operator config: %w", secret), reason: "operation_failed"},
		{name: "write failure", persistError: fmt.Errorf("persist operator config: %w", secret), update: true, reason: "operation_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			entered := make(chan struct{}, 3)
			release := make(chan struct{})
			// Always release blocked peers if an assertion or failure ceiling fires.
			defer close(release)
			spy := &spyLogger{}
			peerSpy := &spyLogger{}
			documents := make([]*profileDocument, 3)
			results := make([]chan profileOperationResult, 3)
			for index, logger := range []logging.Logger{spy, peerSpy, logging.NoopLogger{}} {
				document := &profileDocument{path: filepath.Join(t.TempDir(), "config.json"), loadError: tc.loadError, persistError: tc.persistError, entered: entered, release: release}
				documents[index] = document
				root, err := operatorservice.New(document, &constructionResolution{}, rootTestFileSystem{}, rootTestCreateTemporaryFile, rootTestConfigDecoder, rootTestConfigEncoder, rootTestIDGenerator, logger, nil)
				if err != nil {
					t.Fatalf("New = %v", err)
				}
				results[index] = make(chan profileOperationResult, 1)
				go func() { results[index] <- exerciseSelectedProfileLogger(root, document.path, tc.update) }()
			}
			awaitProfileEntry(t, entered)
			awaitProfileEntry(t, entered)
			awaitProfileEntry(t, entered)
			// All three independently selected effects have reached the document boundary.
			release <- struct{}{}
			release <- struct{}{}
			release <- struct{}{}
			captureResult := awaitProfileResult(t, results[0])
			peerResult := awaitProfileResult(t, results[1])
			quietResult := awaitProfileResult(t, results[2])
			if !reflect.DeepEqual(captureResult.updated, quietResult.updated) || !reflect.DeepEqual(captureResult.resolved, quietResult.resolved) {
				t.Fatalf("selected logger altered results: capture=%#v quiet=%#v", captureResult, quietResult)
			}
			assertSelectedProfileOutcome(t, tc.loadError, tc.persistError, tc.update, documents, captureResult, peerResult, quietResult)
			operation := "resolve_acp_agent_profile"
			if tc.update {
				operation = "update_acp_agent_profile"
			}
			want := []string{"operator_settings." + operation + ".started", "operator_settings." + operation + ".failed"}
			if tc.reason == "" {
				want = []string{"operator_settings.update_acp_agent_profile.started", "operator_settings.update_acp_agent_profile.finished", "operator_settings.resolve_acp_agent_profile.started", "operator_settings.resolve_acp_agent_profile.finished"}
				if !containsKeyValue(spy, "allowed_target_count", 1) {
					t.Fatal("missing safe target count")
				}
			} else if !containsKeyValue(spy, "reason", tc.reason) {
				t.Fatalf("missing failure classification %q", tc.reason)
			}
			if !reflect.DeepEqual(peerResult.updated, quietResult.updated) || !reflect.DeepEqual(peerResult.resolved, quietResult.resolved) {
				t.Fatalf("peer logger altered result: %#v", peerResult)
			}
			if !reflect.DeepEqual(peerSpy.messages(), want) {
				t.Fatalf("peer records = %v, want %v", peerSpy.messages(), want)
			}
			if !reflect.DeepEqual(spy.messages(), want) {
				t.Fatalf("capture records = %v, want %v; quiet peer must emit none", spy.messages(), want)
			}
			for _, capture := range []*spyLogger{spy, peerSpy} {
				assertNoSensitiveValuesLogged(t, capture, secret.Error(), "factory:@you/reviewer", documents[0].path, documents[1].path, documents[2].path)
			}
		})
	}
}

func assertSelectedProfileOutcome(t *testing.T, loadError, persistError error, update bool, documents []*profileDocument, results ...profileOperationResult) {
	t.Helper()
	wantError := loadError
	if wantError == nil {
		wantError = persistError
	}
	for index, result := range results {
		if wantError != nil {
			if !errors.Is(result.err, wantError) {
				t.Fatalf("instance %d error = %v, want original error %v", index, result.err, wantError)
			}
			var expected, actual operatorsettings.DocumentFailure
			if errors.As(wantError, &expected) && (!errors.As(result.err, &actual) || actual.Kind != expected.Kind) {
				t.Fatalf("instance %d lost typed failure", index)
			}
			if documents[index].published || documents[index].document.Workers.ACP.AgentProfile != nil {
				t.Fatalf("instance %d published failed update", index)
			}
		} else {
			if result.err != nil {
				t.Fatalf("instance %d error = %v", index, result.err)
			}
			want := operatorsettings.ACPAgentProfile{DefaultTarget: "factory:@you/reviewer", AllowedTargets: []string{"factory:@you/reviewer"}}
			if !reflect.DeepEqual(result.resolved, want) || !reflect.DeepEqual(result.updated, want) || documents[index].published != update {
				t.Fatalf("instance %d update/readback = %#v, published=%v", index, result, documents[index].published)
			}
		}
	}
}

func awaitProfileEntry(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		t.Fatal("independent instance did not reach document boundary")
	}
}

func awaitProfileResult(t *testing.T, result <-chan profileOperationResult) profileOperationResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(30 * time.Second):
		t.Fatal("profile operation did not complete")
		return profileOperationResult{}
	}
}
