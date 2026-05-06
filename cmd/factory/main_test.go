package main

import (
	"os"
	"testing"
)

func TestMainHelpExecutesWithoutError(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"infinite-you", "--help"}
	t.Cleanup(func() {
		os.Args = originalArgs
	})

	main()
}
