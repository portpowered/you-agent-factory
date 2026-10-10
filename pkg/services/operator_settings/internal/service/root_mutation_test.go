package service_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsdocument "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
)

type gatedMutationDocument struct {
	settingsdocument.Service
	mu       sync.Mutex
	document operatorsettings.Document
	loads    int
	entered  chan struct{}
	release  chan struct{}
}

func (d *gatedMutationDocument) LoadDocument(r operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
	d.mu.Lock()
	d.loads++
	first := d.loads == 1
	snapshot := d.document.Clone()
	d.mu.Unlock()
	if first {
		close(d.entered)
		<-d.release
	}
	return operatorsettings.LoadDocumentResult{Path: r.Path, Document: snapshot}, nil
}

func (d *gatedMutationDocument) PersistDocument(_ context.Context, r operatorsettings.PersistDocumentRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.document = r.Document.Clone()
	return nil
}

func TestRootMutationsSerializeReadModifyPublishAndPeerRootProgresses(t *testing.T) {
	t.Parallel()
	document := &gatedMutationDocument{entered: make(chan struct{}), release: make(chan struct{})}
	root := newControlledRoot(t, document, &constructionResolution{})
	results := make(chan error, 2)
	go func() {
		_, err := root.ConfigureACPIntegrationAdd(context.Background(), "config", operatorsettings.ACPIntegration{ID: "first", Name: "first", Transport: "stdio", Command: "first"})
		results <- err
	}()
	// The first mutation has captured its document and holds the root's write lock.
	awaitMutationSignal(t, document.entered)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(document.release) }) }
	defer release()
	started := make(chan struct{})
	go func() {
		close(started)
		_, err := root.ConfigureACPIntegrationAdd(context.Background(), "config", operatorsettings.ACPIntegration{ID: "second", Name: "second", Transport: "stdio", Command: "second"})
		results <- err
	}()
	awaitMutationSignal(t, started)
	peerDocument := &profileDocument{path: "peer-config"}
	peer := newControlledRoot(t, peerDocument, &constructionResolution{})
	peerResult := make(chan error, 1)
	go func() {
		_, err := peer.UpdateACPAgentProfile(context.Background(), "peer-config", operatorsettings.DefaultACPAgentProfile())
		peerResult <- err
	}()
	awaitMutationResult(t, peerResult)
	if !peerDocument.published {
		t.Fatal("independent root did not publish while first root was held")
	}
	release()
	awaitMutationResult(t, results)
	awaitMutationResult(t, results)
	loaded, err := root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: "config"})
	if err != nil || len(loaded.Document.Workers.ACP.Integrations) != 2 || loaded.Document.Workers.ACP.Integrations[0].Name != "first" || loaded.Document.Workers.ACP.Integrations[1].Name != "second" {
		t.Fatalf("serialized document = %#v, %v", loaded.Document, err)
	}
}

// Timers are failure ceilings only; channel observations drive every test phase.
func awaitMutationSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(30 * time.Second):
		t.Fatal("mutation did not reach controlled boundary")
	}
}

func awaitMutationResult(t *testing.T, results <-chan error) {
	t.Helper()
	select {
	case err := <-results:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("mutation did not complete")
	}
}

func TestRootOptionalIntegrationDefaultsAndValidation(t *testing.T) {
	t.Parallel()
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "authored empty"}[empty], func(t *testing.T) {
			t.Parallel()
			document := &profileDocument{path: "config"}
			if empty {
				document.document.Workers.ACP.Integrations = []operatorsettings.ACPIntegration{}
			}
			root := newControlledRoot(t, document, &constructionResolution{})
			integration := operatorsettings.ACPIntegration{ID: "entry", Name: "entry", Transport: "stdio", Command: "agent"}
			got, err := root.EnsurePackagedACPIntegrations(context.Background(), "config", []operatorsettings.ACPIntegration{integration})
			// Existing document-to-config conversion treats an empty list as absent.
			// Preserve that policy in this required-dependency change.
			want := 1
			if err != nil || len(got.Workers.ACP.Integrations) != want {
				t.Fatalf("defaults = %#v, %v", got, err)
			}
			if _, err := root.ConfigureACPIntegrationDelete(context.Background(), "config", "missing"); !errors.Is(err, operatorsettings.ErrACPIntegrationNotFound) {
				t.Fatalf("missing integration = %v", err)
			}
			document.published = false
			if _, err := root.UpdatePriceTable(context.Background(), "config", operatorsettings.PriceTable{Currency: "invalid"}); !errors.Is(err, operatorsettings.ErrPriceTableInvalid) || document.published {
				t.Fatalf("invalid price = %v, published=%v", err, document.published)
			}
		})
	}
}

func TestRootProfileUpdatesSerializeCompleteCandidates(t *testing.T) {
	t.Parallel()
	document := &gatedMutationDocument{entered: make(chan struct{}), release: make(chan struct{})}
	root := newControlledRoot(t, document, &constructionResolution{})
	first := operatorsettings.ACPAgentProfile{DefaultTarget: "factory:@you/first", AllowedTargets: []string{"factory:@you/first"}}
	second := operatorsettings.ACPAgentProfile{DefaultTarget: "factory:@you/second", AllowedTargets: []string{"factory:@you/second", "factory:@you/factory-builder"}}
	results := make(chan error, 2)
	go func() { _, err := root.UpdateACPAgentProfile(context.Background(), "config", first); results <- err }()
	awaitMutationSignal(t, document.entered)
	var once sync.Once
	release := func() { once.Do(func() { close(document.release) }) }
	defer release()
	started := make(chan struct{})
	go func() {
		close(started)
		_, err := root.UpdateACPAgentProfile(context.Background(), "config", second)
		results <- err
	}()
	awaitMutationSignal(t, started)
	release()
	awaitMutationResult(t, results)
	awaitMutationResult(t, results)
	loaded, err := root.ResolveACPAgentProfile("config")
	if err != nil || !reflect.DeepEqual(loaded, second) {
		t.Fatalf("final profile = %#v, %v, want entire second candidate %#v", loaded, err, second)
	}
}
