//go:build functionallong

package runtime_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type batchBoundarySummary struct {
	RequestID string
	Source    string
	Works     []batchBoundaryWork
	Relations []batchBoundaryRelation
}

type batchBoundaryWork struct {
	Name         string
	WorkID       string
	WorkTypeName string
	State        string
	TraceID      string
}

type batchBoundaryRelation struct {
	Type           string
	SourceWorkName string
	TargetWorkName string
	RequiredState  string
}

func runBoundaryBatchSmokeThroughWatchedFile(t *testing.T, batchJSON []byte, requestID string) batchBoundarySummary {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "factory_request_batch"))
	testutil.WriteSeedFile(t, dir, "task", batchJSON)

	host := startRootRunBatchBoundaryHost(t, dir)
	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)
	waitForGeneratedWorkIDsComplete(t, host.Endpoint(), []string{
		"work-boundary-parent",
		"work-boundary-prerequisite",
		"work-boundary-child",
	}, 10*time.Second)

	return loadBatchBoundarySummary(t, stream, requestID)
}

func runBoundaryBatchSmokeThroughHTTP(t *testing.T, batchJSON []byte, requestID string) batchBoundarySummary {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "factory_request_batch"))
	host := startRootRunBatchBoundaryHost(t, dir)
	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)

	req, err := http.NewRequest(http.MethodPut, support.DefaultSessionWorkURL(host.Endpoint(), "/work-requests/"+requestID), bytes.NewReader(batchJSON))
	if err != nil {
		t.Fatalf("build PUT /work-requests: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /work-requests: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var body bytes.Buffer
		if _, copyErr := body.ReadFrom(resp.Body); copyErr != nil {
			t.Fatalf("read PUT /work-requests failure body: %v", copyErr)
		}
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", resp.StatusCode, body.String())
	}

	var out factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode PUT /work-requests response: %v", err)
	}
	if out.RequestId != requestID {
		t.Fatalf("PUT /work-requests request_id = %q, want %q", out.RequestId, requestID)
	}

	waitForGeneratedWorkIDsComplete(t, host.Endpoint(), []string{
		"work-boundary-parent",
		"work-boundary-prerequisite",
		"work-boundary-child",
	}, 10*time.Second)

	return loadBatchBoundarySummary(t, stream, requestID)
}

func startRootRunBatchBoundaryHost(t *testing.T, factoryRoot string) *support.RootRunFunctionalHost {
	t.Helper()

	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot: factoryRoot,
		SystemRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})
	return host
}

func loadBatchBoundarySummary(t *testing.T, stream *factoryEventHTTPStream, requestID string) batchBoundarySummary {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		event := stream.next(time.Until(deadline))
		if event.Type != factoryapi.FactoryEventTypeWorkRequest || support.StringPointerValue(event.Context.RequestId) != requestID {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode WORK_REQUEST payload: %v", err)
		}

		summary := batchBoundarySummary{
			RequestID: requestID,
			Source:    support.StringPointerValue(payload.Source),
		}
		for _, work := range support.FactoryWorksValue(payload.Works) {
			summary.Works = append(summary.Works, batchBoundaryWork{
				Name:         work.Name,
				WorkID:       support.StringPointerValue(work.WorkId),
				WorkTypeName: support.StringPointerValue(work.WorkTypeName),
				State:        generatedWorkStateName(work.State),
				TraceID:      support.StringPointerValue(work.TraceId),
			})
		}
		for _, relation := range support.FactoryRelationsValue(payload.Relations) {
			summary.Relations = append(summary.Relations, batchBoundaryRelation{
				Type:           string(relation.Type),
				SourceWorkName: relation.SourceWorkName,
				TargetWorkName: relation.TargetWorkName,
				RequiredState:  support.StringPointerValue(relation.RequiredState),
			})
		}

		sort.Slice(summary.Works, func(i, j int) bool {
			return summary.Works[i].Name < summary.Works[j].Name
		})
		sort.Slice(summary.Relations, func(i, j int) bool {
			left := summary.Relations[i]
			right := summary.Relations[j]
			if left.Type != right.Type {
				return left.Type < right.Type
			}
			if left.SourceWorkName != right.SourceWorkName {
				return left.SourceWorkName < right.SourceWorkName
			}
			return left.TargetWorkName < right.TargetWorkName
		})

		return summary
	}

	t.Fatalf("canonical Factory Session SSE missing WORK_REQUEST event for %q", requestID)
	return batchBoundarySummary{}
}
