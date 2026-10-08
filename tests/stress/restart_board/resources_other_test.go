//go:build !windows && !linux

package restart_board_test

func processResources() string { return "CPU/RSS unavailable on this platform" }
