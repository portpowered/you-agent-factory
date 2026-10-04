package testhome

import (
	"os"
	"path/filepath"
	"testing"
)

// homeEnvironmentKeys are the variables Go and the factory consult to locate
// the user profile, config, cache, and data directories on Linux and Windows.
var homeEnvironmentKeys = []string{
	"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME",
	"APPDATA", "LOCALAPPDATA",
}

// IsolateHome points every home-resolving environment variable at a fresh
// per-test directory so the test never reads or writes the real user profile
// (for example ~/.you-agent-factory). It returns the isolated home directory.
// Tests that call it cannot use t.Parallel because it uses t.Setenv.
func IsolateHome(t testing.TB) string {
	t.Helper()
	home := t.TempDir()
	for _, key := range homeEnvironmentKeys {
		t.Setenv(key, filepath.Join(home, key))
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// IsolateHomeMain points the home-resolving environment at a temp directory
// for a whole package. Call it first in TestMain and defer or invoke the
// returned cleanup after m.Run (os.Exit skips defers):
//
//	func TestMain(m *testing.M) {
//		cleanup := testhome.IsolateHomeMain()
//		code := m.Run()
//		cleanup()
//		os.Exit(code)
//	}
func IsolateHomeMain() (cleanup func()) {
	home, err := os.MkdirTemp("", "you-test-home-")
	if err != nil {
		panic("isolate test home: " + err.Error())
	}
	for _, key := range homeEnvironmentKeys {
		_ = os.Setenv(key, filepath.Join(home, key))
	}
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("USERPROFILE", home)
	return func() { _ = os.RemoveAll(home) }
}
