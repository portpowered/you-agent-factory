//go:build !windows && !linux && !darwin

package agypty

import "context"

func runPlatformSession(context.Context, *platformSession) (SessionResult, error) {
	return SessionResult{}, ErrUnsupportedPlatform
}

func sessionProcessRunning(int) bool {
	return false
}

func terminateSessionTestProcess(int) {}
