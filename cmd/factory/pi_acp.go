package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed pi_acp_bridge/dist/index.js
var piACPBridge []byte

//go:embed pi_acp_bridge/THIRD_PARTY_NOTICES.txt
var piACPNotices []byte

// runPiACP starts the repository-owned, self-contained Node bridge. The
// temporary .mjs file is needed because Node needs a script path while stdin
// remains the ACP transport.
func runPiACP(ctx context.Context, args []string) int {
	directory, err := os.MkdirTemp("", "you-pi-acp-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitFailure
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitFailure
	}
	defer removePiACPTemp(directory, root)
	script := filepath.Join(directory, "bridge.mjs")
	if err := root.WriteFile("bridge.mjs", piACPBridge, 0600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitFailure
	}
	if err := root.WriteFile("THIRD_PARTY_NOTICES.txt", piACPNotices, 0600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitFailure
	}
	command := exec.CommandContext(ctx, "node", append([]string{script}, args...)...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitFailure
	}
	return exitSuccess
}

func removePiACPTemp(directory string, root *os.Root) {
	// Remove only our two files, and only while the path still identifies the
	// directory we created. A replacement or unexpected child is left intact.
	owned, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return
	}
	current, err := os.Lstat(directory)
	if err != nil || !os.SameFile(owned, current) || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 {
		_ = root.Close()
		return
	}
	_ = root.Remove("bridge.mjs")
	_ = root.Remove("THIRD_PARTY_NOTICES.txt")
	if err := root.Close(); err != nil {
		return
	}
	_ = os.Remove(directory)
}
