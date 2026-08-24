package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNewInvocationUsesFreshExternalDirectories(t *testing.T) {
	repoRoot := t.TempDir()
	root, err := makeExternalTempRoot(repoRoot)
	if err != nil {
		t.Fatalf("make external temp root: %v", err)
	}
	h := &Harness{repoRoot: repoRoot, root: root}
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Errorf("close harness: %v", err)
		}
	})

	first, err := h.NewInvocation()
	if err != nil {
		t.Fatalf("first invocation: %v", err)
	}
	second, err := h.NewInvocation()
	if err != nil {
		t.Fatalf("second invocation: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	t.Cleanup(func() { _ = second.Close() })

	firstEnv := first.Environment()
	secondEnv := second.Environment()
	for label, path := range map[string]string{
		"first working directory":  firstEnv.WorkingDirectory,
		"first home directory":     firstEnv.HomeDirectory,
		"first user profile":       firstEnv.UserProfile,
		"second working directory": secondEnv.WorkingDirectory,
		"second home directory":    secondEnv.HomeDirectory,
		"second user profile":      secondEnv.UserProfile,
	} {
		if pathWithin(repoRoot, path) {
			t.Errorf("%s %q is reachable below repository %q", label, path, repoRoot)
		}
	}
	if firstEnv.WorkingDirectory == secondEnv.WorkingDirectory || firstEnv.HomeDirectory == secondEnv.HomeDirectory || firstEnv.UserProfile == secondEnv.UserProfile {
		t.Fatal("invocations reused an isolated directory")
	}
}

func TestAuditTraceResolvesRelativeSourceRead(t *testing.T) {
	repoRoot := t.TempDir()
	workDir := t.TempDir()
	sourcePath := filepath.Join(repoRoot, "pkg", "internal.txt")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o700); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("source"), 0o600); err != nil {
		t.Fatalf("write source fixture: %v", err)
	}
	relativePath, err := filepath.Rel(workDir, sourcePath)
	if err != nil {
		t.Fatalf("relative source path: %v", err)
	}

	trace := fmt.Sprintf("123 openat(AT_FDCWD, %q, O_RDONLY) = 3<%s>\n", filepath.ToSlash(relativePath), filepath.ToSlash(sourcePath))
	violation, err := auditTrace(repoRoot, workDir, []byte(trace))
	if err != nil {
		t.Fatalf("audit trace: %v", err)
	}
	if violation == nil {
		t.Fatal("audit accepted a source-tree read")
	}
	canonicalSource, err := canonicalPath(sourcePath, "")
	if err != nil {
		t.Fatalf("canonical source path: %v", err)
	}
	if violation.Path != canonicalSource {
		t.Fatalf("violation path = %q, want %q", violation.Path, canonicalSource)
	}
	if !strings.Contains(violation.Error(), canonicalSource) {
		t.Fatalf("violation error %q does not name canonical source path %q", violation.Error(), canonicalSource)
	}
}

func TestAuditTraceFollowsDescendantRelativeReads(t *testing.T) {
	repoRoot := t.TempDir()
	workDir := t.TempDir()
	sourcePath := filepath.Join(repoRoot, "README.md")
	if err := os.WriteFile(sourcePath, []byte("source"), 0o600); err != nil {
		t.Fatalf("write source fixture: %v", err)
	}
	relativePath, err := filepath.Rel(workDir, sourcePath)
	if err != nil {
		t.Fatalf("relative source path: %v", err)
	}
	trace := fmt.Sprintf(
		"100 clone(child_stack, flags) = 101\n101 openat(AT_FDCWD, %q, O_RDONLY) = 3\n",
		filepath.ToSlash(relativePath),
	)
	violation, err := auditTrace(repoRoot, workDir, []byte(trace))
	if err != nil {
		t.Fatalf("audit trace: %v", err)
	}
	if violation == nil {
		t.Fatal("audit accepted a descendant source-tree read")
	}
}

func TestParseTraceRecordSupportsPidAndUnfinishedForms(t *testing.T) {
	unfinished, ok := parseTraceRecord(`[pid 123] openat(AT_FDCWD, "relative", O_RDONLY <unfinished ...>`)
	if !ok {
		t.Fatal("failed to parse unfinished strace record")
	}
	if unfinished.pid != 123 || unfinished.name != "openat" || !unfinished.unfinished {
		t.Fatalf("unfinished record = %#v", unfinished)
	}
	resumed, ok := parseTraceRecord(`[pid 123] <... openat resumed>) = 3`)
	if !ok {
		t.Fatal("failed to parse resumed strace record")
	}
	if resumed.pid != 123 || resumed.name != "openat" || !resumed.resumed || resumed.result != "3" {
		t.Fatalf("resumed record = %#v", resumed)
	}
}

func TestParseTraceRecordSupportsPaddedDecodedResumedReturn(t *testing.T) {
	resumed, ok := parseTraceRecord(`364   <... openat resumed>)             = 7</tmp/factory.json>`)
	if !ok {
		t.Fatal("failed to parse padded resumed strace record")
	}
	if resumed.pid != 364 || resumed.name != "openat" || !resumed.resumed || resumed.result != "7</tmp/factory.json>" {
		t.Fatalf("padded resumed record = %#v", resumed)
	}
	if number, ok := resultNumber(resumed.result); !ok || number != 7 {
		t.Fatalf("result number = (%d, %t), want (7, true)", number, ok)
	}
}

func TestAuditTracePairsPaddedResumedRecords(t *testing.T) {
	repoRoot := t.TempDir()
	workDir := t.TempDir()
	externalPath := filepath.Join(workDir, "factory.json")
	trace := fmt.Sprintf(
		"364 openat(AT_FDCWD<%s>, %q, O_RDONLY <unfinished ...>\n364 <... openat resumed>)             = 7<%s>\n",
		filepath.ToSlash(workDir),
		filepath.Base(externalPath),
		filepath.ToSlash(externalPath),
	)
	violation, err := auditTrace(repoRoot, workDir, []byte(trace))
	if err != nil {
		t.Fatalf("audit trace: %v", err)
	}
	if violation != nil {
		t.Fatalf("audit reported an external read: %v", violation)
	}
}

func TestAuditTraceRejectsUnpairedUnknownRecord(t *testing.T) {
	_, err := auditTrace(t.TempDir(), t.TempDir(), []byte("123 ???( <unfinished ...>\n"))
	if err == nil || !strings.Contains(err.Error(), "without a process exit") {
		t.Fatalf("audit error = %v, want an incomplete-trace error", err)
	}
}

func TestAuditLogFilesCollectsOnlyPerProcessOutputs(t *testing.T) {
	runtimeDir := t.TempDir()
	prefix := filepath.Join(runtimeDir, "file-access.strace")
	for _, name := range []string{"file-access.strace.12", "file-access.strace.3"} {
		if err := os.WriteFile(filepath.Join(runtimeDir, name), nil, 0o600); err != nil {
			t.Fatalf("write trace file: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "unrelated.log"), nil, 0o600); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}
	paths, err := auditLogFiles(prefix)
	if err != nil {
		t.Fatalf("audit log files: %v", err)
	}
	want := []string{filepath.Join(runtimeDir, "file-access.strace.12"), filepath.Join(runtimeDir, "file-access.strace.3")}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("audit log files = %#v, want %#v", paths, want)
	}
}

func TestCheckAuditAllowsEmptyPerProcessOutput(t *testing.T) {
	repoRoot := t.TempDir()
	workDir := t.TempDir()
	runtimeDir := t.TempDir()
	prefix := filepath.Join(runtimeDir, "file-access.strace")
	if err := os.WriteFile(prefix+".12", []byte("12 ???( <unfinished ...>\n12 +++ exited with 0 +++\n"), 0o600); err != nil {
		t.Fatalf("write empty trace: %v", err)
	}
	externalFile := filepath.Join(workDir, "factory.json")
	trace := fmt.Sprintf("3 openat(AT_FDCWD, %q, O_RDONLY) = 3<%s>\n", filepath.Base(externalFile), externalFile)
	if err := os.WriteFile(prefix+".3", []byte(trace), 0o600); err != nil {
		t.Fatalf("write file trace: %v", err)
	}
	i := &Invocation{
		harness: &Harness{repoRoot: repoRoot},
		env:     InvocationEnvironment{WorkingDirectory: workDir},
	}
	if err := i.checkAudit(prefix, Result{}); err != nil {
		t.Fatalf("check audit: %v", err)
	}
}

func TestAuditTraceResolvesDirectoryDescriptorReads(t *testing.T) {
	repoRoot := t.TempDir()
	workDir := t.TempDir()
	externalDir := filepath.Join(workDir, "external")
	if err := os.Mkdir(externalDir, 0o700); err != nil {
		t.Fatalf("create external directory: %v", err)
	}
	sourcePath := filepath.Join(repoRoot, "docs", "README.md")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o700); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("source"), 0o600); err != nil {
		t.Fatalf("write source fixture: %v", err)
	}
	relativePath, err := filepath.Rel(externalDir, sourcePath)
	if err != nil {
		t.Fatalf("relative source path: %v", err)
	}
	trace := fmt.Sprintf(
		"123 openat(AT_FDCWD, %q, O_RDONLY|O_DIRECTORY) = 3<%s>\n123 openat(3<%s>, %q, O_RDONLY) = 4\n",
		filepath.ToSlash(filepath.Join("external")),
		filepath.ToSlash(externalDir),
		filepath.ToSlash(externalDir),
		filepath.ToSlash(relativePath),
	)
	violation, err := auditTrace(repoRoot, workDir, []byte(trace))
	if err != nil {
		t.Fatalf("audit trace: %v", err)
	}
	if violation == nil {
		t.Fatal("audit accepted a directory-descriptor source-tree read")
	}
}

func TestIsolatedEnvironmentReplacesHomeAndWorkingDirectory(t *testing.T) {
	env := InvocationEnvironment{
		WorkingDirectory: t.TempDir(),
		HomeDirectory:    t.TempDir(),
		UserProfile:      t.TempDir(),
		RuntimeDirectory: t.TempDir(),
	}
	values := isolatedEnvironment(env)
	for key, want := range map[string]string{
		"HOME":        env.HomeDirectory,
		"USERPROFILE": env.UserProfile,
		"PWD":         env.WorkingDirectory,
		"TMPDIR":      env.RuntimeDirectory,
	} {
		if got := environmentValue(values, key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func environmentValue(environment []string, key string) string {
	prefix := key + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}
