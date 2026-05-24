package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	homeDir, err := os.MkdirTemp("", "pkg-service-test-home-*")
	if err != nil {
		panic(err)
	}

	if err := os.Setenv("HOME", homeDir); err != nil {
		panic(err)
	}
	if err := os.Setenv("USERPROFILE", homeDir); err != nil {
		panic(err)
	}
	if err := os.Setenv("HOMEDRIVE", filepath.VolumeName(homeDir)); err != nil {
		panic(err)
	}
	if err := os.Setenv("HOMEPATH", homeDir); err != nil {
		panic(err)
	}

	code := m.Run()
	_ = os.RemoveAll(homeDir)
	os.Exit(code)
}
