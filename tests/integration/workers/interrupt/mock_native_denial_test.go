package interrupt_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const mockNativeAttemptFile = "WSV_MOCK_NATIVE_ATTEMPTS"

// The provider fixture is the already compiled test executable, with no shell,
// interpreter or forwarding path. Even an absolute resolved fixture path denies.
func TestMain(m *testing.M) {
	if marker := os.Getenv(mockNativeAttemptFile); marker != "" {
		file, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			os.Exit(98)
		}
		_, err = fmt.Fprintln(file, filepath.Base(os.Args[0]))
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			os.Exit(98)
		}
		os.Exit(97)
	}
	os.Exit(m.Run())
}

func mockDeniedEnvironment(t *testing.T, root string) ([]string, string) {
	t.Helper()
	home, bin := filepath.Join(root, "home"), filepath.Join(root, "denied-bin")
	for _, path := range []string{home, bin} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(root, "native-attempts")
	env := []string{
		"HOME=" + home, "USERPROFILE=" + home,
		"HOMEDRIVE=" + filepath.VolumeName(home), "HOMEPATH=" + home,
		"APPDATA=" + home, "LOCALAPPDATA=" + home,
		"XDG_CONFIG_HOME=" + home, "XDG_CACHE_HOME=" + home,
		"PATH=" + bin, "PATHEXT=.EXE", "YOU_NO_BROWSER_OPEN=1",
		mockNativeAttemptFile + "=" + marker,
	}
	// Preserve only OS runtime necessities, never operator configuration,
	// credentials, provider homes, inherited PATH or executable overrides.
	for _, key := range []string{"SystemRoot", "WINDIR", "TEMP", "TMP"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Windows forbids deleting any hard link to the currently running test
	// image. Copy once, then link the inactive copy for the denial aliases.
	if runtime.GOOS == "windows" {
		copy := filepath.Join(root, "denial-helper.exe")
		copyMockDenialFile(t, testBinary, copy)
		testBinary = copy
	}
	// Current installed-executable catalog plus protocol launchers. The only
	// selected route in this journey is Codex (fixed command "codex"); no
	// caller-authored command, ACP endpoint or script is admitted. Its lookup
	// cannot reach operator absolute paths: the entire PATH and home are private.
	for _, name := range strings.Fields("agy claude codex copilot cursor-agent droid gemini grok iflow kimi kiro-cli-chat mux node npx openclaw opencode pool qodercli qwen reasonix traecli uvx you zeroclaw pi") {
		path := filepath.Join(bin, name)
		if runtime.GOOS == "windows" {
			path += ".exe"
		}
		copyMockDenialExecutable(t, testBinary, path)
		command := exec.CommandContext(t.Context(), path)
		command.Env = env
		if err := command.Run(); err == nil {
			t.Fatalf("native denial preflight %s unexpectedly succeeded", name)
		} else {
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 97 {
				t.Fatalf("native denial preflight %s: %v", name, err)
			}
		}
	}
	if content, err := os.ReadFile(marker); err != nil || len(strings.Fields(string(content))) != 25 {
		t.Fatalf("denial preflight attempts=%q error=%v", content, err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	return env, marker
}

func copyMockDenialExecutable(t *testing.T, source, target string) {
	t.Helper()
	if err := os.Link(source, target); err == nil {
		return
	}
	copyMockDenialFile(t, source, target)
}

func copyMockDenialFile(t *testing.T, source, target string) {
	t.Helper()
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy denying executable: %v / %v", copyErr, closeErr)
	}
}

func assertNoMockNativeAttempts(t *testing.T, marker string) {
	t.Helper()
	content, err := os.ReadFile(marker)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("native attempts=%q error=%v; want zero attempts and zero native provider starts", content, err)
	}
}
