package service

import (
	"os"
	"path/filepath"
)

func init() {
	testHome := filepath.Join(os.TempDir(), "infinite-you-pkg-service-test-home")
	if err := os.MkdirAll(testHome, 0o755); err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", testHome); err != nil {
		panic(err)
	}
}
