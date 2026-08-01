package root

import (
	"context"
	"sync"
	"testing"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestBuildProcessComposesAutomationsFilesystemWatcherInertly(t *testing.T) {
	t.Parallel()

	var (
		apiStarts     int
		submissionsMu sync.Mutex
		submissions   []work.FactorySubmissionRecord
	)
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			apiStarts++
			return nil
		},
		SubmissionRecorder: func(record work.FactorySubmissionRecord) {
			submissionsMu.Lock()
			submissions = append(submissions, record)
			submissionsMu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if apiStarts != 0 {
		t.Fatalf("BuildProcess() started API lifecycle %d times, want zero", apiStarts)
	}
	submissionsMu.Lock()
	defer submissionsMu.Unlock()
	if len(submissions) != 0 {
		t.Fatalf("BuildProcess() submitted %d Work records, want zero before runtime lifecycle", len(submissions))
	}
	if process == nil {
		t.Fatal("BuildProcess() returned nil process")
	}
}
