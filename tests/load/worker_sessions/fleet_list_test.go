package workersessions_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workerhttp "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Dedicated performance workload: 200 associated sessions, one fixture setup,
// no subprocesses or paid providers. Run with -benchtime=1s -p 1 -timeout=2m.
// The controlled reader models finite native-file parsing, not live Codex cost.
func BenchmarkFleetList(b *testing.B) {
	service, reader := fleetFixture(b)
	for _, size := range []int{10, 50, 200} {
		b.Run(fmt.Sprintf("rows-%d", size), func(b *testing.B) {
			reader.opens = 0
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				page, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{MaxResults: size})
				if err != nil || len(page.Observations) != size {
					b.Fatalf("page rows=%d error=%v", len(page.Observations), err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(reader.opens)/float64(b.N), "native-opens/op")
		})
	}
}

// Include attribution and public JSON serialization without substituting
// their cost for a measurement of production Work storage on the live daemon.
func BenchmarkFleetListHTTP(b *testing.B) {
	service, reader := fleetFixture(b)
	works := &fixtureWorkReader{}
	adapter := workerhttp.NewAdapter(service, works)
	limit := 50
	var serialization time.Duration
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		page, err := adapter.ListTopLevelWorkerSessions(context.Background(), "", nil, &limit, nil)
		if err != nil || len(page.Sessions) != limit {
			b.Fatalf("HTTP page rows=%d error=%v", len(page.Sessions), err)
		}
		started := time.Now()
		if _, err := json.Marshal(page); err != nil {
			b.Fatal(err)
		}
		serialization += time.Since(started)
	}
	b.StopTimer()
	b.ReportMetric(float64(reader.opens)/float64(b.N), "native-opens/op")
	b.ReportMetric(float64(works.calls)/float64(b.N), "work-reads/op")
	b.ReportMetric(float64(works.elapsed.Nanoseconds())/float64(b.N), "work-ns/op")
	b.ReportMetric(float64(serialization.Nanoseconds())/float64(b.N), "serialization-ns/op")
}

type fixtureWorkReader struct {
	work.Service
	calls   int
	elapsed time.Duration
}

func (r *fixtureWorkReader) GetWork(_ context.Context, _, id string) (work.ReadModel, error) {
	started := time.Now()
	r.calls++
	result := work.ReadModel{Name: id}
	r.elapsed += time.Since(started)
	return result, nil
}

type fixtureExecution struct{ workers.Service }

func (fixtureExecution) Execute(_ context.Context, req workers.ExecuteRequest) (workers.ExecuteResult, error) {
	ref := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: req.Correlation.DispatchID}
	continuation := ref.ContinuationRef()
	return workers.ExecuteResult{Correlation: req.Correlation, Outcome: workers.ExecutionOutcomeAccepted, Continuation: continuation.ClonePtr()}, nil
}

type fixtureReader struct {
	providersessions.Service
	path  string
	opens int
}

func (r *fixtureReader) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	r.opens++
	file, err := os.Open(r.path)
	if err != nil {
		return providersessions.ProjectResult{}, err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		var event map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return providersessions.ProjectResult{}, err
		}
		count++
	}
	return providersessions.ProjectResult{Detail: providersessions.Detail{Parse: providersessions.ParseSummary{EventCount: count}}}, scanner.Err()
}

func fleetFixture(b *testing.B) (workersessions.Service, *fixtureReader) {
	b.Helper()
	path := filepath.Join(b.TempDir(), "controlled-native.jsonl")
	if err := os.WriteFile(path, []byte(strings.Repeat("{\"type\":\"event\",\"text\":\"controlled fixture\"}\n", 2048)), 0600); err != nil {
		b.Fatal(err)
	}
	reader := &fixtureReader{path: path}
	eventStore, err := eventswire.NewService()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if closer, ok := eventStore.(interface{ Close(context.Context) error }); ok {
			_ = closer.Close(context.Background())
		}
	})
	service, err := wire.NewService(fixtureExecution{}, eventStore, logging.NoopLogger{}, platformclock.Real{}, platformclock.Real{}, reader, nil)
	if err != nil {
		b.Fatal(err)
	}
	for i := range 200 {
		id := fmt.Sprintf("worker-%03d", i)
		_, err := service.InvokeSession(context.Background(), workersessions.InvokeSessionRequest{ID: id, Execution: workers.WorkstationDispatchRequest{
			WorkstationName: "fixture", Execution: workers.WorkstationExecutionRequest{Dispatch: work.WorkDispatch{DispatchID: id, WorkstationName: "fixture", Execution: work.ExecutionMetadata{WorkIDs: []string{"work-" + id}}}},
		}})
		if err != nil {
			b.Fatal(err)
		}
	}
	b.Cleanup(func() {
		if closer, ok := service.(interface{ Close(context.Context) error }); ok {
			_ = closer.Close(context.Background())
		}
	})
	return service, reader
}
