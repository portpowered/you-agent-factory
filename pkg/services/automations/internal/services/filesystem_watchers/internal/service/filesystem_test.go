package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
)

type localInputFiles struct{}

func (localInputFiles) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (localInputFiles) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (localInputFiles) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }

type pendingInputFiles struct {
	localInputFiles
	reads   int
	readyAt int
	err     error
}

func (files *pendingInputFiles) ReadFile(string) ([]byte, error) {
	files.reads++
	if files.err != nil {
		return nil, files.err
	}
	if files.reads >= files.readyAt {
		return []byte("ready"), nil
	}
	return nil, nil
}

func TestReadFileWithRetry_UsesControlledClockAndPreservesReadOutcomes(t *testing.T) {
	readErr := errors.New("read failed")
	for _, tc := range []struct {
		name    string
		readyAt int
		waits   int
		reads   int
		want    string
		err     error
	}{
		{name: "content arrives after empty read", readyAt: 2, waits: 1, reads: 2, want: "ready"},
		{name: "empty reads exhaust bounded retries", readyAt: 4, waits: 2, reads: 3},
		{name: "read failure returns immediately", reads: 1, err: readErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clock := clockwork.NewFakeClock()
			files := &pendingInputFiles{readyAt: tc.readyAt, err: tc.err}
			fw := &watcher{clock: clock, files: files}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			done := make(chan struct{})
			var content []byte
			var err error
			go func() {
				content, err = fw.readFileWithRetry("input.md", 3, 50*time.Millisecond)
				close(done)
			}()
			for range tc.waits {
				waitForFakeClockWaiters(t, clock, 1)
				clock.Advance(50 * time.Millisecond)
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("file retry did not complete after controlled advances")
			}
			if string(content) != tc.want || !errors.Is(err, tc.err) || files.reads != tc.reads {
				t.Fatalf("content/error/reads = %q/%v/%d, want %q/%v/%d", content, err, files.reads, tc.want, tc.err, tc.reads)
			}
		})
	}
}
