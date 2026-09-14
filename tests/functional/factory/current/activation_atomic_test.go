package current

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestSharedCurrentFactoryActivationProvenanceAtomic proves the public GET
// boundary sees either the old or new complete tuple while a replacement save
// is in flight. The save and reads share one root-built server and real local
// Factory layouts.
func TestSharedCurrentFactoryActivationProvenanceAtomic(t *testing.T) {
	if testing.Short() {
		t.Skip("slow activation provenance concurrency witness")
	}
	t.Parallel()

	fixture := startSharedCurrentFactoryAPI(t)
	fixture.requireServerRunning(t)
	session := fixture.openSession(t, "alpha-activation-atomic")
	before := getCurrentFactoryForSession(t, session.serverURL, session.id)
	if before.Activation == nil || before.Version == nil {
		t.Fatalf("initial tuple = %#v, want activation and version", before)
	}
	newVersion := advancedFactoryVersion(t, before.Version)
	body := saveFactoryForSessionRequestBody(
		functionalNamedFactoryBody("alpha-activation-atomic", "atomic-updated", newVersion),
	)

	const readCount = 24
	start := make(chan struct{})
	readResults := make(chan atomicFactoryReadResult, readCount)
	putResult := make(chan atomicFactoryReadResult, 1)
	var waitGroup sync.WaitGroup
	for index := 0; index < readCount; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			readResults <- readFactoryTuple(session.serverURL, session.id)
		}()
	}
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		<-start
		putResult <- putFactoryTuple(session.serverURL, session.id, body)
	}()
	close(start)
	waitGroup.Wait()
	close(readResults)

	put := <-putResult
	if put.err != nil {
		t.Fatalf("concurrent activation PUT: %v", put.err)
	}
	if put.value.Activation == nil || put.value.Version == nil {
		t.Fatalf("saved tuple = %#v, want activation and version", put.value)
	}
	if put.value.Activation.ActivationId == before.Activation.ActivationId {
		t.Fatalf("saved activation ID = %q, want a single new ID after success", put.value.Activation.ActivationId)
	}

	for result := range readResults {
		if result.err != nil {
			t.Fatalf("concurrent activation GET: %v", result.err)
		}
		assertCompleteActivationTuple(t, result.value, before, put.value)
	}
}

type atomicFactoryReadResult struct {
	value factoryapi.Factory
	err   error
}

func readFactoryTuple(serverURL, sessionID string) atomicFactoryReadResult {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sessionFactoryURL(serverURL, sessionID), nil)
	if err != nil {
		return atomicFactoryReadResult{err: err}
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return atomicFactoryReadResult{err: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		return atomicFactoryReadResult{err: fmt.Errorf("GET status=%d body=%s", response.StatusCode, payload)}
	}
	var value factoryapi.Factory
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return atomicFactoryReadResult{err: err}
	}
	return atomicFactoryReadResult{value: value}
}

func putFactoryTuple(serverURL, sessionID, body string) atomicFactoryReadResult {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, sessionFactoryURL(serverURL, sessionID), bytes.NewBufferString(body))
	if err != nil {
		return atomicFactoryReadResult{err: err}
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return atomicFactoryReadResult{err: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		return atomicFactoryReadResult{err: fmt.Errorf("PUT status=%d body=%s", response.StatusCode, payload)}
	}
	var value factoryapi.Factory
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return atomicFactoryReadResult{err: err}
	}
	return atomicFactoryReadResult{value: value}
}

func assertCompleteActivationTuple(
	t *testing.T,
	got factoryapi.Factory,
	before factoryapi.Factory,
	after factoryapi.Factory,
) {
	t.Helper()
	if got.Activation == nil || got.Version == nil || got.WorkTypes == nil || len(*got.WorkTypes) != 1 {
		t.Fatalf("concurrent result = %#v, want complete Factory tuple", got)
	}
	if got.Name != before.Name || got.Activation.LoadedSourceDigest == "" {
		t.Fatalf("concurrent result identity = %#v, want named Factory %q with digest", got, before.Name)
	}
	switch (*got.WorkTypes)[0].Name {
	case (*before.WorkTypes)[0].Name:
		if got.Activation.ActivationId != before.Activation.ActivationId || !sameVersion(got.Version, before.Version) {
			t.Fatalf("old concurrent tuple = %#v, want activation/version from before=%#v", got, before)
		}
	case (*after.WorkTypes)[0].Name:
		if got.Activation.ActivationId != after.Activation.ActivationId || !sameVersion(got.Version, after.Version) {
			t.Fatalf("new concurrent tuple = %#v, want activation/version from after=%#v", got, after)
		}
	default:
		t.Fatalf("concurrent result work type = %q, want old %q or new %q", (*got.WorkTypes)[0].Name, (*before.WorkTypes)[0].Name, (*after.WorkTypes)[0].Name)
	}
}

func sameVersion(left, right *factoryapi.HybridLogicalTimestamp) bool {
	return left != nil && right != nil && left.Logical == right.Logical && left.Physical.Equal(right.Physical)
}
