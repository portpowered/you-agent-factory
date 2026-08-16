package claude

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

func TestClaudeCommandEnvironmentPreventsGitMergeEditorPrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("real Git integration")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}

	runner, err := platformprocess.NewExecCommandRunner(exec.Command, platformclock.Real{}, nil)
	if err != nil {
		t.Fatalf("NewExecCommandRunner() error = %v", err)
	}
	repoDir := t.TempDir()
	editorMarker := filepath.Join(t.TempDir(), "editor-invoked")
	editorScript := writeGitEditorMarkerScript(t, editorMarker)

	runGitCommand(t, runner, repoDir, "init", "-b", "main")
	runGitCommand(t, runner, repoDir, "config", "gc.auto", "0")
	runGitCommand(t, runner, repoDir, "config", "maintenance.auto", "false")
	runGitCommand(t, runner, repoDir, "config", "user.email", "agent-factory-test@example.com")
	runGitCommand(t, runner, repoDir, "config", "user.name", "Agent Factory Test")
	writeGitTestFile(t, repoDir, "base.txt", "base\n")
	runGitCommand(t, runner, repoDir, "add", "base.txt")
	runGitCommand(t, runner, repoDir, "commit", "-m", "base")

	runGitCommand(t, runner, repoDir, "checkout", "-b", "feature")
	writeGitTestFile(t, repoDir, "feature.txt", "feature\n")
	runGitCommand(t, runner, repoDir, "add", "feature.txt")
	runGitCommand(t, runner, repoDir, "commit", "-m", "feature")

	runGitCommand(t, runner, repoDir, "checkout", "main")
	writeGitTestFile(t, repoDir, "main.txt", "main\n")
	runGitCommand(t, runner, repoDir, "add", "main.txt")
	runGitCommand(t, runner, repoDir, "commit", "-m", "main")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, platformprocess.CommandRequest{
		Command: "git",
		Args:    []string{"merge", "--no-ff", "feature"},
		Env: isolatedGitEnvironment(buildCommandEnv(os.Environ(), map[string]string{
			"GIT_EDITOR":          editorScript,
			"GIT_SEQUENCE_EDITOR": editorScript,
			"GIT_MERGE_AUTOEDIT":  "yes",
			"EDITOR":              editorScript,
			"VISUAL":              editorScript,
		})),
		WorkDir: repoDir,
	})
	if err != nil {
		t.Fatalf("git merge returned system error: %v\nstdout:\n%s\nstderr:\n%s", err, result.Stdout, result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Fatalf("git merge exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if _, err := os.Stat(editorMarker); err == nil {
		t.Fatalf("git invoked editor at %s; Claude automation env should suppress merge editor prompts", editorMarker)
	} else if !os.IsNotExist(err) {
		t.Fatalf("checking editor marker: %v", err)
	}
}

func runGitCommand(t *testing.T, runner platformprocess.CommandRunner, dir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, platformprocess.CommandRequest{
		Command: "git",
		Args:    args,
		Env:     isolatedGitEnvironment(os.Environ()),
		WorkDir: dir,
	})
	if err != nil {
		t.Fatalf("git %s returned system error: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, result.Stdout, result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Fatalf("git %s exit code = %d\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), result.ExitCode, result.Stdout, result.Stderr)
	}
}

func writeGitTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func writeGitEditorMarkerScript(t *testing.T, markerPath string) string {
	t.Helper()
	scriptDir := t.TempDir()
	if runtime.GOOS == "windows" {
		script := filepath.Join(scriptDir, "editor.bat")
		content := "@echo off\r\necho invoked > %1\r\nexit /b 42\r\n"
		writeGitScript(t, script, content, 0o644)
		return script + " " + markerPath
	}

	script := filepath.Join(scriptDir, "editor.sh")
	content := "#!/bin/sh\nprintf invoked > \"$1\"\nexit 42\n"
	writeGitScript(t, script, content, 0o755)
	return script + " " + markerPath
}

func writeGitScript(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("writing editor script %s: %v", path, err)
	}
}

func isolatedGitEnvironment(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || inheritedGitRepositoryEnvironment[name] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

var inheritedGitRepositoryEnvironment = map[string]bool{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	"GIT_COMMON_DIR":                   true,
	"GIT_DIR":                          true,
	"GIT_INDEX_FILE":                   true,
	"GIT_OBJECT_DIRECTORY":             true,
	"GIT_PREFIX":                       true,
	"GIT_QUARANTINE_PATH":              true,
	"GIT_WORK_TREE":                    true,
}
