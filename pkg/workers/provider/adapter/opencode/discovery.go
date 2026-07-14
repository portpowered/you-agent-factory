package opencode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
)

const maxVersionBytes = 4096

// ExecutableIdentifier resolves and fingerprints executable contents so an
// in-place upgrade cannot reuse a stale capability decision.
type ExecutableIdentifier struct{}

func (ExecutableIdentifier) Identify(ctx context.Context, executable string) (Installation, error) {
	if err := ctx.Err(); err != nil {
		return Installation{}, err
	}
	resolved, err := exec.LookPath(strings.TrimSpace(executable))
	if err != nil {
		return Installation{}, err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return Installation{}, err
	}
	if evaluated, evaluateErr := filepath.EvalSymlinks(resolved); evaluateErr == nil {
		resolved = evaluated
	}
	file, err := os.Open(resolved)
	if err != nil {
		return Installation{}, err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return Installation{}, err
	}
	return Installation{Executable: resolved, Fingerprint: hex.EncodeToString(hasher.Sum(nil))}, nil
}

// CommandDiscoverer negotiates without customer input. It reads a bounded
// version record, then relies only on the exit status of a structured-format
// help invocation; human help text is never parsed or retained.
type CommandDiscoverer struct {
	Runner workerprocess.CommandRunner
}

// NewDefaultResolver constructs the production resolver around the shared
// subprocess runner and executable-content identity.
func NewDefaultResolver(runner workerprocess.CommandRunner) (*Resolver, error) {
	return NewResolver(ResolverOptions{
		Identifier: ExecutableIdentifier{},
		Discoverer: CommandDiscoverer{Runner: runner},
	})
}

func (d CommandDiscoverer) Discover(ctx context.Context, installation Installation) (Decision, error) {
	if d.Runner == nil {
		return Decision{}, errors.New("opencode capability discovery requires a command runner")
	}
	versionResult, err := d.Runner.Run(ctx, workerprocess.CommandRequest{
		Command: installation.Executable, Args: []string{"--version"},
	})
	if err != nil {
		return Decision{}, fmt.Errorf("read opencode version: %w", err)
	}
	if versionResult.ExitCode != 0 {
		return Decision{}, fmt.Errorf("read opencode version: exit code %d", versionResult.ExitCode)
	}
	versionBytes := versionResult.Stdout
	if len(versionBytes) == 0 {
		versionBytes = versionResult.Stderr
	}
	if len(versionBytes) == 0 || len(versionBytes) > maxVersionBytes {
		return Decision{}, errors.New("read opencode version: invalid bounded output")
	}
	version := strings.TrimSpace(string(versionBytes))
	if version == "" {
		return Decision{}, errors.New("read opencode version: empty output")
	}

	probe, err := d.Runner.Run(ctx, workerprocess.CommandRequest{
		Command: installation.Executable, Args: []string{"run", "--format", "json", "--help"},
	})
	if err != nil {
		return Decision{}, fmt.Errorf("probe opencode structured output: %w", err)
	}
	mode := ModeStructured
	if probe.ExitCode != 0 {
		mode = ModeFinalOnly
	}
	return Decision{Installation: installation, Version: version, Mode: mode}, nil
}
