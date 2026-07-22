package opencode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
)

const maxVersionBytes = 4096

// ExecutableIdentifier resolves and fingerprints executable contents so an
// in-place upgrade cannot reuse a stale capability decision.
type ExecutableIdentifier struct {
	ResolveSymlinks workers.ResolveExecutableSymlinks
	Locator         platformprocess.ExecutableLocator
	Files           platformfilesystem.ReadOpener
}

func (i ExecutableIdentifier) Identify(ctx context.Context, executable string) (Installation, error) {
	if err := ctx.Err(); err != nil {
		return Installation{}, err
	}
	if i.Locator == nil {
		return Installation{}, errors.New("opencode executable locator is required")
	}
	if i.Files == nil {
		return Installation{}, errors.New("opencode executable file reader is required")
	}
	resolved, err := i.Locator.LookPath(strings.TrimSpace(executable))
	if err != nil {
		return Installation{}, err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return Installation{}, err
	}
	if i.ResolveSymlinks == nil {
		return Installation{}, errors.New("opencode executable symlink resolver is required")
	}
	if evaluated, evaluateErr := i.ResolveSymlinks(resolved); evaluateErr == nil {
		resolved = evaluated
	}
	file, err := i.Files.Open(resolved)
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
func NewDefaultResolver(
	runner workerprocess.CommandRunner,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	locator platformprocess.ExecutableLocator,
	files platformfilesystem.ReadOpener,
) (*Resolver, error) {
	if resolveSymlinks == nil {
		return nil, errors.New("opencode executable symlink resolver is required")
	}
	if locator == nil {
		return nil, errors.New("opencode executable locator is required")
	}
	if files == nil {
		return nil, errors.New("opencode executable file reader is required")
	}
	return NewResolver(ExecutableIdentifier{
		ResolveSymlinks: resolveSymlinks,
		Locator:         locator,
		Files:           files,
	}, CommandDiscoverer{Runner: runner}, 0, 0)
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
