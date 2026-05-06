package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
)

func TestMainRoutesThroughCommandMain(t *testing.T) {
	originalCommandMain := commandMain
	originalExitFunc := exitFunc
	originalStdout := stdout
	originalStderr := stderr
	originalArgs := os.Args
	t.Cleanup(func() {
		commandMain = originalCommandMain
		exitFunc = originalExitFunc
		stdout = originalStdout
		stderr = originalStderr
		os.Args = originalArgs
	})

	var gotArgs []string
	var gotStdout io.Writer
	var gotStderr io.Writer
	var exitCode int
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	commandMain = func(args []string, stdout io.Writer, stderr io.Writer) int {
		gotArgs = append([]string(nil), args...)
		gotStdout = stdout
		gotStderr = stderr
		return 17
	}
	exitFunc = func(code int) {
		exitCode = code
	}
	stdout = out
	stderr = errOut
	os.Args = []string{"releasetagcheck", "-tag", "v1.2.3"}

	main()

	if exitCode != 17 {
		t.Fatalf("main() exit code = %d, want 17", exitCode)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "-tag" || gotArgs[1] != "v1.2.3" {
		t.Fatalf("main() args = %v, want [-tag v1.2.3]", gotArgs)
	}
	if gotStdout != out {
		t.Fatalf("main() stdout writer mismatch")
	}
	if gotStderr != errOut {
		t.Fatalf("main() stderr writer mismatch")
	}
}

func TestRunExplicitTagSuccess(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-tag", "v1.2.3"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if got := stdout.String(); got != "release_tag=v1.2.3\n" {
		t.Fatalf("run() stdout = %q, want %q", got, "release_tag=v1.2.3\n")
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}
}

func TestRunExplicitTagValidationFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-tag", "v1.2"}, &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "release tag \"v1.2\" must match vMAJOR.MINOR.PATCH\n" {
		t.Fatalf("run() stderr = %q", got)
	}
}

func TestRunRejectsBothTagInputs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-tag", "v1.2.3", "-points-at", "HEAD"}, &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "use either -tag or -points-at, not both\n" {
		t.Fatalf("run() stderr = %q", got)
	}
}

func TestRunRequiresOneTagInput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(nil, &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "provide -tag or -points-at\n" {
		t.Fatalf("run() stderr = %q", got)
	}
}

func TestRunPointsAtSuccess(t *testing.T) {
	originalListGitTagsPointingAt := listGitTagsPointingAt
	t.Cleanup(func() {
		listGitTagsPointingAt = originalListGitTagsPointingAt
	})

	var gotRevision string
	listGitTagsPointingAt = func(_ context.Context, revision string) ([]string, error) {
		gotRevision = revision
		return []string{"not-a-release-tag", "v1.2.3"}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-points-at", "HEAD"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if gotRevision != "HEAD" {
		t.Fatalf("lookup revision = %q, want %q", gotRevision, "HEAD")
	}
	if got := stdout.String(); got != "release_tag=v1.2.3\n" {
		t.Fatalf("run() stdout = %q, want %q", got, "release_tag=v1.2.3\n")
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}
}

func TestRunPointsAtFailsWithoutSemverTag(t *testing.T) {
	originalListGitTagsPointingAt := listGitTagsPointingAt
	t.Cleanup(func() {
		listGitTagsPointingAt = originalListGitTagsPointingAt
	})

	listGitTagsPointingAt = func(_ context.Context, revision string) ([]string, error) {
		return []string{"build-123"}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-points-at", "HEAD"}, &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "expected exactly one semver release tag for HEAD, found []\n" {
		t.Fatalf("run() stderr = %q", got)
	}
}

func TestRunPointsAtFailsWithMultipleSemverTags(t *testing.T) {
	originalListGitTagsPointingAt := listGitTagsPointingAt
	t.Cleanup(func() {
		listGitTagsPointingAt = originalListGitTagsPointingAt
	})

	listGitTagsPointingAt = func(_ context.Context, revision string) ([]string, error) {
		return []string{"v1.2.4", "v1.2.3"}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-points-at", "HEAD"}, &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "expected exactly one semver release tag for HEAD, found [\"v1.2.3\" \"v1.2.4\"]\n" {
		t.Fatalf("run() stderr = %q", got)
	}
}

func TestRunPointsAtSurfacesGitFailure(t *testing.T) {
	originalListGitTagsPointingAt := listGitTagsPointingAt
	t.Cleanup(func() {
		listGitTagsPointingAt = originalListGitTagsPointingAt
	})

	listGitTagsPointingAt = func(_ context.Context, revision string) ([]string, error) {
		return nil, errors.New("list tags pointing at HEAD: exit status 1\nfatal: bad object HEAD")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-points-at", "HEAD"}, &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "list tags pointing at HEAD: exit status 1\nfatal: bad object HEAD\n" {
		t.Fatalf("run() stderr = %q", got)
	}
}
