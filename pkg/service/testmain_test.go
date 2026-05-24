package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	rootDir, err := os.MkdirTemp("", "pkg-service-test-env-*")
	if err != nil {
		panic(err)
	}

	homeDir := filepath.Join(rootDir, "home")
	tempDir := filepath.Join(homeDir, "tmp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		panic(err)
	}

	for key, value := range map[string]string{
		"HOME":        homeDir,
		"USERPROFILE": homeDir,
		"HOMEDRIVE":   filepath.VolumeName(homeDir),
		"HOMEPATH":    homeDir,
		"TMPDIR":      tempDir,
		"TMP":         tempDir,
		"TEMP":        tempDir,
	} {
		if err := os.Setenv(key, value); err != nil {
			panic(err)
		}
	}

	code := m.Run()
	_ = os.RemoveAll(rootDir)
	os.Exit(code)
}
