package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const piMinimumVersion = "0.81.0"

const piUpgradeAction = "pi-acp requires Pi 0.81.0 or newer; run npm install -g @earendil-works/pi-coding-agent@latest, then retry"

var errPiVersionProbe = errors.New("Pi version probe failed")

// piPreflight rejects Pi releases that lack the agent_settled event consumed
// by pi-acp. This runs before the ACP daemon is started, so an incompatible Pi
// cannot leave session/prompt waiting for an event it will never emit.
func piPreflight(
	ctx context.Context,
	newCommand platformprocess.CommandFactory,
	locator platformprocess.ExecutableLocator,
	cwd string,
	environment []string,
	scheduler platformclock.TimerSource,
) error {
	if err := ctx.Err(); err != nil {
		return nativeFailure(err)
	}
	if newCommand == nil {
		return piVersionFailure()
	}
	if locator != nil {
		if _, err := locator.LookPath("pi"); err != nil {
			return piMissingExecutableFailure()
		}
	}
	output, err := probePiVersion(ctx, newCommand, cwd, environment, scheduler)
	if ctx.Err() != nil {
		return nativeFailure(ctx.Err())
	}
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return piMissingExecutableFailure()
		}
		return piVersionFailure()
	}
	installed, ok := parsePiVersion(output)
	if !ok {
		return piVersionFailure()
	}
	minimum, _ := parsePiVersion(piMinimumVersion)
	if installed.lessThan(minimum) {
		return piVersionFailure()
	}
	return nil
}

func piMissingExecutableFailure() error {
	return providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindDependency,
		Message: "Pi executable is unavailable; install @earendil-works/pi-coding-agent and retry",
		Diagnostics: &providers.ExecuteDiagnostics{Metadata: map[string]string{
			"work-failure-type": "missing_executable",
		}},
	}
}

func piVersionFailure() error {
	return providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindMisconfigured,
		Message: piUpgradeAction,
	}
}

type piVersion struct{ major, minor, patch int }

func parsePiVersion(output string) (piVersion, bool) {
	value := strings.TrimSpace(output)
	value = strings.TrimPrefix(value, "pi ")
	value = strings.TrimPrefix(value, "v")
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return piVersion{}, false
	}
	fields := [3]int{}
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return piVersion{}, false
		}
		fields[index] = number
	}
	return piVersion{major: fields[0], minor: fields[1], patch: fields[2]}, true
}

func (version piVersion) lessThan(other piVersion) bool {
	if version.major != other.major {
		return version.major < other.major
	}
	if version.minor != other.minor {
		return version.minor < other.minor
	}
	return version.patch < other.patch
}

func probePiVersion(
	ctx context.Context,
	newCommand platformprocess.CommandFactory,
	cwd string,
	environment []string,
	scheduler platformclock.TimerSource,
) (string, error) {
	command := newCommand("pi", "--version")
	if command == nil {
		return "", errPiVersionProbe
	}
	command.Dir = cwd
	command.Env = append([]string(nil), environment...)
	command.WaitDelay = 2 * time.Second
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = io.Discard
	platformprocess.ConfigureSubprocessTree(command)
	if err := command.Start(); err != nil {
		return "", err
	}
	tree, err := platformprocess.AttachSubprocessTree(command)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return "", errPiVersionProbe
	}
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	timer := scheduler.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case err := <-finished:
		platformprocess.CloseSubprocessTree(command, tree)
		if err != nil {
			return "", errPiVersionProbe
		}
		return output.String(), nil
	case <-ctx.Done():
	case <-timer.C():
	}
	_ = platformprocess.TerminateSubprocessTree(command, tree)
	<-finished
	platformprocess.CloseSubprocessTree(command, tree)
	return "", errPiVersionProbe
}
