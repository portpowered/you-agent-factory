package rawfailurewitness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	witnessEnabledEnv   = "FUNCTIONAL_RAW_FAILURE_WITNESS"
	witnessDirectoryEnv = "FUNCTIONAL_RAW_FAILURE_RENDEZVOUS"
	witnessTimeoutEnv   = "FUNCTIONAL_RAW_FAILURE_RENDEZVOUS_TIMEOUT"
	witnessTimeout      = 90 * time.Second
	maximumWait         = 2 * time.Minute
)

type event struct {
	sequence int
	role     string
	line     string
}

var events = []event{
	{sequence: 1, role: "rawfailure", line: "Factory Event timeline: sequence=1 kind=work.accepted"},
	{sequence: 2, role: "rawfailurepeer", line: "Factory Event timeline: sequence=2 kind=worker.completed"},
	{sequence: 3, role: "rawfailure", line: "Factory Event timeline: sequence=3 kind=work.dispatched"},
	{sequence: 4, role: "rawfailurepeer", line: "Factory Event timeline: sequence=4 kind=worker.started"},
	{sequence: 5, role: "rawfailure", line: "Factory Event timeline: sequence=5 kind=worker.output"},
	{sequence: 6, role: "rawfailurepeer", line: "Factory Event timeline: sequence=6 kind=work.completed"},
}

// Run emits alternating package output only after both real go test package
// processes reach the same per-run directory and the capture process confirms
// it has observed each line.
func Run(t *testing.T, role, assertion string) {
	t.Helper()
	directory := strings.TrimSpace(os.Getenv(witnessDirectoryEnv))
	if directory == "" {
		t.Fatalf("%s is required when %s=1", witnessDirectoryEnv, witnessEnabledEnv)
	}
	timeout, err := rendezvousTimeout(os.Getenv(witnessTimeoutEnv))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("create raw failure rendezvous: %v", err)
	}
	if err := writeOnce(filepath.Join(directory, role+"-ready"), []byte("ready\n")); err != nil {
		t.Fatalf("publish %s readiness: %v", role, err)
	}
	deadline := time.Now().Add(timeout)
	for _, peer := range []string{"rawfailure", "rawfailurepeer"} {
		if err := waitForFile(filepath.Join(directory, peer+"-ready"), deadline); err != nil {
			t.Fatalf("wait for both raw failure packages: %v", err)
		}
	}

	for _, item := range events {
		if item.role != role {
			if err := waitForFile(capturedEventPath(directory, item.sequence), deadline); err != nil {
				t.Fatalf("wait for interleaved package event %d: %v", item.sequence, err)
			}
			continue
		}
		if item.sequence > 1 {
			previous := capturedEventPath(directory, item.sequence-1)
			if err := waitForFile(previous, deadline); err != nil {
				t.Fatalf("wait for preceding package event %d: %v", item.sequence-1, err)
			}
		}
		t.Log(item.line)
		if err := waitForFile(capturedEventPath(directory, item.sequence), deadline); err != nil {
			t.Fatalf("wait for capture to observe package event %d: %v", item.sequence, err)
		}
	}
	t.Errorf("%s", assertion)
}

func rendezvousTimeout(value string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return witnessTimeout, nil
	}
	timeout, err := time.ParseDuration(value)
	if err != nil || timeout <= 0 || timeout > maximumWait {
		return 0, fmt.Errorf("%s must be greater than 0s and at most %s", witnessTimeoutEnv, maximumWait)
	}
	return timeout, nil
}

func waitForFile(path string, deadline time.Time) error {
	for {
		_, err := os.Stat(path)
		if err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect rendezvous marker %s: %w", path, err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func writeOnce(path string, value []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(value); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func capturedEventPath(directory string, sequence int) string {
	return filepath.Join(directory, fmt.Sprintf("captured-event-%02d-complete", sequence))
}
