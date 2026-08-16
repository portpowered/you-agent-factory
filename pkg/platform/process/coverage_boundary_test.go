package process

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

type coverageProcessClock struct {
	times []time.Time
	next  int
}

func (clock *coverageProcessClock) Now() time.Time {
	if clock.next >= len(clock.times) {
		return clock.times[len(clock.times)-1]
	}
	value := clock.times[clock.next]
	clock.next++
	return value
}

func (clock *coverageProcessClock) After(time.Duration) <-chan time.Time {
	return make(chan time.Time)
}

func TestLoggingCommandRunnerAndStatusProjection(t *testing.T) {
	logger := &recordingCommandLogger{}
	clock := &coverageProcessClock{times: []time.Time{time.Unix(1, 0), time.Unix(2, 0)}}
	runner := CommandRunnerWithLogging(
		fixedCommandRunnerWithError{result: CommandResult{Stdout: []byte("ok")}},
		logger,
		clock,
	)
	result, err := runner.Run(context.Background(), CommandRequest{Command: "tool", Args: []string{"--version"}})
	if err != nil || string(result.Stdout) != "ok" {
		t.Fatalf("logged success = %#v, %v", result, err)
	}
	if len(logger.infos) != 2 || len(logger.verboses) != 2 {
		t.Fatalf("logged success entries = info %d/verbose %d, want two each", len(logger.infos), len(logger.verboses))
	}

	failedLogger := &recordingCommandLogger{}
	failed, err := (&LoggingCommandRunner{
		Runner: fixedCommandRunnerWithError{
			result: CommandResult{ExitCode: 7, Stderr: []byte("failed")},
			err:    errors.New("runner failed"),
		},
		Logger: failedLogger,
		Clock:  &coverageProcessClock{times: []time.Time{time.Unix(1, 0), time.Unix(2, 0)}},
	}).Run(context.Background(), CommandRequest{Command: "tool"})
	if err == nil || failed.ExitCode != 7 || len(failedLogger.warns) != 0 {
		t.Fatalf("logged failure = %#v, %v, warns=%d", failed, err, len(failedLogger.warns))
	}
	if len(failedLogger.infos) != 2 {
		t.Fatalf("logged failure info entries = %d, want request and completion", len(failedLogger.infos))
	}

	if got := commandResultStatus(context.Background(), CommandResult{}, context.DeadlineExceeded); got != "timed_out" {
		t.Fatalf("commandResultStatus(deadline) = %q", got)
	}
	deadline, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if got := commandResultStatus(deadline, CommandResult{}, errors.New("failed")); got != "timed_out" {
		t.Fatalf("commandResultStatus(expired context) = %q", got)
	}
	if got := commandResultStatus(context.Background(), CommandResult{ExitCode: 1}, nil); got != "failed" {
		t.Fatalf("commandResultStatus(exit) = %q", got)
	}
}

func TestCommandRunnerWithLoggingPreservesExecRunnerLogger(t *testing.T) {
	logger := &recordingCommandLogger{}
	clock := &coverageProcessClock{times: []time.Time{time.Unix(1, 0)}}
	execRunner := ExecCommandRunner{}
	if got := execCommandRunnerWithLogger(execRunner, logger).(ExecCommandRunner).Logger; got != logger {
		t.Fatal("execCommandRunnerWithLogger(value) did not inject logger")
	}
	if got := execCommandRunnerWithLogger(&execRunner, logger).(*ExecCommandRunner).Logger; got != logger {
		t.Fatal("execCommandRunnerWithLogger(pointer) did not inject logger")
	}
	if got := execCommandRunnerWithLogger(fixedCommandRunnerWithError{}, logger); got == nil {
		t.Fatal("execCommandRunnerWithLogger(default) returned nil")
	}
	if got := CommandRunnerWithLogging(execRunner, logger, clock); got == nil {
		t.Fatal("CommandRunnerWithLogging() returned nil")
	}
}

func TestCommandEnvironmentAndExecutableEffects(t *testing.T) {
	if got := CommandEnvEntriesFromMap(nil); got != nil {
		t.Fatalf("CommandEnvEntriesFromMap(nil) = %#v, want nil", got)
	}
	entries := CommandEnvEntriesFromMap(map[string]string{"Z": "last", "A": "first"})
	if len(entries) != 2 || entries[0].Name != "A" || entries[1].Name != "Z" {
		t.Fatalf("sorted environment entries = %#v", entries)
	}
	merged := MergeCommandEnv(
		[]string{"A=base", "invalid", "B=base"},
		[]CommandEnvEntry{{Name: "B", Value: "overlay"}, {Name: "", Value: "ignored"}},
		[]CommandEnvEntry{{Name: "C", Value: "added"}},
	)
	if len(merged) != 3 || merged[0] != "A=base" || merged[1] != "B=overlay" || merged[2] != "C=added" {
		t.Fatalf("MergeCommandEnv() = %#v", merged)
	}
	if _, err := (HostExecutableLocator{}).LookPath(os.Args[0]); err != nil {
		t.Fatalf("HostExecutableLocator.LookPath(%q) = %v", os.Args[0], err)
	}
}

func TestCommandCleanupLogsGracefulTermination(t *testing.T) {
	logger := &recordingCommandLogger{}
	cleanup := newCommandProcessCleanupContext(logger, CommandRequest{
		Command: "tool",
		WorkDir: "/work",
	}, commandProcessCleanupReasonCancel)
	cleanup.logGraceful(42)
	if len(logger.verboses) != 1 || logger.verboses[0].fields["status"] != "graceful" ||
		logger.verboses[0].fields["process_group_id"] != 42 {
		t.Fatalf("graceful cleanup log = %#v, want graceful status and process group", logger.verboses)
	}
}
