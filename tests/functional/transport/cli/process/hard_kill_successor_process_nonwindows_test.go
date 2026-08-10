//go:build !windows

package process_test

func suspendHardKillProcess(int) (func(), error) {
	return func() {}, nil
}
