package wire

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

type inertProcessLauncher struct{}

func (inertProcessLauncher) Start(
	context.Context,
	modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	panic("process launcher called during readiness inspection")
}

type inertHostClock struct{}

func (inertHostClock) Now() time.Time {
	return time.Unix(0, 0)
}

func (inertHostClock) NewTimer(time.Duration) modelseffects.HostTimer {
	panic("host timer created during readiness inspection")
}

type inertCommandRunner struct{}

func (inertCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	panic("runtime command called during readiness inspection")
}

type recordingHTTPDoer struct {
	name  string
	calls int
}

func (doer *recordingHTTPDoer) Do(*http.Request) (*http.Response, error) {
	doer.calls++
	panic(doer.name + " client invoked during inert construction")
}

type recordingProcessLauncher struct{ starts int }

func (launcher *recordingProcessLauncher) Start(
	context.Context,
	modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	launcher.starts++
	panic("process launcher invoked during inert construction")
}

type recordingHostClock struct {
	nowCalls   int
	timerCalls int
}

func (clock *recordingHostClock) Now() time.Time {
	clock.nowCalls++
	panic("host clock invoked during inert construction")
}

func (clock *recordingHostClock) NewTimer(time.Duration) modelseffects.HostTimer {
	clock.timerCalls++
	panic("host timer created during inert construction")
}

type recordingCommandRunner struct{ calls int }

func (runner *recordingCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.calls++
	panic("runtime command runner invoked during inert construction")
}

type recordingAssetMkdirAll struct{ calls int }

func (effect *recordingAssetMkdirAll) mkdirAll(string, os.FileMode) error {
	effect.calls++
	panic("asset mkdir invoked during inert construction")
}

type recordingAssetStat struct{ calls int }

func (effect *recordingAssetStat) stat(string) (os.FileInfo, error) {
	effect.calls++
	panic("asset stat invoked during inert construction")
}

type recordingAssetHome struct{ calls int }

func (effect *recordingAssetHome) home() (string, error) {
	effect.calls++
	panic("asset home invoked during inert construction")
}

type recordingAssetWriteFile struct{ calls int }

func (effect *recordingAssetWriteFile) write(string, []byte, os.FileMode) error {
	effect.calls++
	panic("asset write invoked during inert construction")
}

type recordingAssetRename struct{ calls int }

func (effect *recordingAssetRename) rename(string, string) error {
	effect.calls++
	panic("asset rename invoked during inert construction")
}

type recordingAssetRemove struct{ calls int }

func (effect *recordingAssetRemove) remove(string) error {
	effect.calls++
	panic("asset remove invoked during inert construction")
}

type recordingAssetReadFile struct{ calls int }

func (effect *recordingAssetReadFile) read(string) ([]byte, error) {
	effect.calls++
	panic("asset read file invoked during inert construction")
}

type recordingAssetReadDir struct{ calls int }

func (effect *recordingAssetReadDir) readDir(string) ([]os.DirEntry, error) {
	effect.calls++
	panic("asset read dir invoked during inert construction")
}

type recordingAssetCreate struct{ calls int }

func (effect *recordingAssetCreate) create(string) (io.WriteCloser, error) {
	effect.calls++
	panic("asset create invoked during inert construction")
}

type recordingAssetOpen struct{ calls int }

func (effect *recordingAssetOpen) open(string) (io.ReadCloser, error) {
	effect.calls++
	panic("asset open invoked during inert construction")
}

type recordingRuntimeInspect struct{ calls int }

func (effect *recordingRuntimeInspect) inspect(string) (os.FileInfo, error) {
	effect.calls++
	panic("runtime inspect invoked during inert construction")
}

type recordingRuntimeTempDir struct{ calls int }

func (effect *recordingRuntimeTempDir) tempDir() string {
	effect.calls++
	panic("runtime temp dir invoked during inert construction")
}

type recordingRuntimeTempFile struct{ calls int }

func (effect *recordingRuntimeTempFile) create(string, string) (modelseffects.RuntimeTempFile, error) {
	effect.calls++
	panic("runtime temp file invoked during inert construction")
}

type recordingProcessClock struct{ calls int }

func (clock *recordingProcessClock) now() time.Time {
	clock.calls++
	panic("process clock invoked during inert construction")
}
