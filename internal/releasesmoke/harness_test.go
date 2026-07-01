package releasesmoke

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessEnvForIsolatedHome_ReplacesHomeVariables(t *testing.T) {
	t.Setenv("HOME", "/real/home")
	t.Setenv("USERPROFILE", "/real/home")
	t.Setenv("HOMEDRIVE", "C:")
	t.Setenv("HOMEPATH", "\\Users\\real")
	t.Setenv("PATH", "/bin:/usr/bin")

	isolatedHome := filepath.Join(t.TempDir(), "isolated-home")
	env := processEnvForIsolatedHome(isolatedHome)

	if got := envValue(env, "HOME"); got != isolatedHome {
		t.Fatalf("HOME = %q, want %q", got, isolatedHome)
	}
	if got := envValue(env, "USERPROFILE"); got != isolatedHome {
		t.Fatalf("USERPROFILE = %q, want %q", got, isolatedHome)
	}
	if got := envValue(env, "HOMEDRIVE"); got != filepath.VolumeName(isolatedHome) {
		t.Fatalf("HOMEDRIVE = %q, want %q", got, filepath.VolumeName(isolatedHome))
	}
	if got := envValue(env, "HOMEPATH"); got != string(os.PathSeparator) {
		t.Fatalf("HOMEPATH = %q, want %q", got, string(os.PathSeparator))
	}
	if got := envValue(env, "PATH"); got != "/bin:/usr/bin" {
		t.Fatalf("PATH = %q, want preserved PATH", got)
	}
	for _, entry := range env {
		if strings.HasPrefix(entry, "HOME=/real/home") || strings.HasPrefix(entry, "USERPROFILE=/real/home") {
			t.Fatalf("env still contains real home entry: %q", entry)
		}
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}
