package wire

import (
	"context"
	"io"
	"sync/atomic"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestOpeningPresentationOwnerScopesDynamicCollaboratorsAndClosesThem(t *testing.T) {
	owner := NewOpeningPresentationOwner()
	var appObserved, directObserved atomic.Int32
	appID, err := owner.RegisterApplication(factorysessions.ApplicationOpeningScope{
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) { appObserved.Add(1) },
	})
	if err != nil {
		t.Fatalf("RegisterApplication: %v", err)
	}
	directID, err := owner.RegisterDirectJavaScript(factorysessions.DirectJavaScriptRunScope{
		Output: io.Discard,
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			directObserved.Add(1)
		},
	})
	if err != nil {
		t.Fatalf("RegisterDirectJavaScript: %v", err)
	}
	stdioID, err := owner.RegisterStdio(factorysessions.StdioOpeningScope{Input: nilReader{}, Output: io.Discard})
	if err != nil {
		t.Fatalf("RegisterStdio: %v", err)
	}

	owner.ObserveHost(appID, factorysessions.RuntimeHostBinding{Port: 1})
	owner.ObserveHost(directID, factorysessions.RuntimeHostBinding{Port: 2})
	if appObserved.Load() != 1 || directObserved.Load() != 1 {
		t.Fatalf("host observations = app:%d direct:%d", appObserved.Load(), directObserved.Load())
	}
	if _, ok := owner.Application(appID); !ok {
		t.Fatal("application scope was not retained")
	}
	if _, ok := owner.DirectJavaScript(directID); !ok {
		t.Fatal("direct JavaScript scope was not retained")
	}
	if _, ok := owner.Stdio(stdioID); !ok {
		t.Fatal("stdio scope was not retained")
	}

	owner.Close(appID)
	owner.Close(directID)
	owner.Close(stdioID)
	if _, ok := owner.Application(appID); ok {
		t.Fatal("application scope survived Close")
	}
	if _, ok := owner.DirectJavaScript(directID); ok {
		t.Fatal("direct JavaScript scope survived Close")
	}
	if _, ok := owner.Stdio(stdioID); ok {
		t.Fatal("stdio scope survived Close")
	}
}

func TestOpeningPresentationOwnerGatesApplicationCompletionOnHostBinding(t *testing.T) {
	var completed atomic.Int32
	started := make(chan struct{}, 1)
	owner := NewOpeningPresentationOwner()
	id, err := owner.RegisterApplication(factorysessions.ApplicationOpeningScope{
		Completion: func(context.Context) error {
			started <- struct{}{}
			completed.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("RegisterApplication: %v", err)
	}
	scope, ok := owner.Application(id)
	if !ok {
		t.Fatal("application scope was not registered")
	}
	result := make(chan error, 1)
	go func() { result <- scope.Completion(context.Background()) }()
	select {
	case <-started:
		t.Fatal("completion ran before host binding")
	default:
	}
	owner.ObserveHost(id, factorysessions.RuntimeHostBinding{Port: 7437})
	if err := <-result; err != nil {
		t.Fatalf("completion: %v", err)
	}
	if completed.Load() != 1 {
		t.Fatalf("completion calls = %d, want 1", completed.Load())
	}
}

type nilReader struct{}

func (nilReader) Read([]byte) (int, error) { return 0, io.EOF }
