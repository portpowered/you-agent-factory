package testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

const (
	defaultProviderErrorSmokeWorkerName = "worker-a"
	defaultProviderErrorSmokePromptBody = "Process the input task.\n"
)

// ProviderErrorSmokeHarness owns a copied script-wrap fixture configured for a
// requested provider/model pair plus the corresponding service harness.
type ProviderErrorSmokeHarness struct {
	Dir        string
	Provider   workers.ModelProvider
	Model      string
	WorkerName string

	providerRunner *ProviderCommandRunner
	serviceOptions []ServiceTestHarnessOption
}

// ProviderErrorSmokeWork captures the stable submission fields that provider-error
// smoke tests assert against.
type ProviderErrorSmokeWork struct {
	Name       string
	WorkTypeID string
	WorkID     string
	TraceID    string
	Payload    []byte
}

type providerErrorSmokeHarnessConfig struct {
	workerName     string
	promptBody     string
	serviceOptions []ServiceTestHarnessOption
}

// ProviderErrorSmokeHarnessOption customizes NewProviderErrorSmokeHarness.
type ProviderErrorSmokeHarnessOption func(*providerErrorSmokeHarnessConfig)

// WithProviderErrorSmokeServiceOptions forwards service harness options to the
// constructed ServiceTestHarness.
func WithProviderErrorSmokeServiceOptions(opts ...ServiceTestHarnessOption) ProviderErrorSmokeHarnessOption {
	return func(cfg *providerErrorSmokeHarnessConfig) {
		cfg.serviceOptions = append(cfg.serviceOptions, opts...)
	}
}

// NewProviderErrorSmokeHarness copies a real script-wrap fixture, rewrites the
// requested worker AGENTS.md with the provider/model pair under test, and
// returns a configured fixture helper. Build the service harness after writing
// any seed work so preseed sees the intended inputs.
func NewProviderErrorSmokeHarness(
	t *testing.T,
	fixtureDir string,
	provider workers.ModelProvider,
	model string,
	opts ...ProviderErrorSmokeHarnessOption,
) *ProviderErrorSmokeHarness {
	t.Helper()

	cfg := &providerErrorSmokeHarnessConfig{
		workerName: defaultProviderErrorSmokeWorkerName,
		promptBody: defaultProviderErrorSmokePromptBody,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	dir := CopyFixtureDir(t, fixtureDir)
	writeProviderErrorSmokeWorkerConfig(t, dir, cfg.workerName, provider, model, cfg.promptBody)

	providerRunner := NewProviderCommandRunner()
	serviceOptions := append([]ServiceTestHarnessOption{
		WithProviderCommandRunner(providerRunner),
	}, cfg.serviceOptions...)

	return &ProviderErrorSmokeHarness{
		Dir:            dir,
		Provider:       provider,
		Model:          model,
		WorkerName:     cfg.workerName,
		providerRunner: providerRunner,
		serviceOptions: serviceOptions,
	}
}

// BuildServiceHarness constructs the ServiceTestHarness for the rewritten
// fixture. Call this after any seed requests have been written into Dir.
func (h *ProviderErrorSmokeHarness) BuildServiceHarness(t *testing.T) *ServiceTestHarness {
	t.Helper()
	return NewServiceTestHarness(t, h.Dir, h.serviceOptions...)
}

// BuildRunningServiceHarness constructs the ServiceTestHarness, starts the real
// async run loop, and registers cleanup so provider-error smoke tests can focus
// on lane behavior rather than run-loop lifecycle plumbing.
func (h *ProviderErrorSmokeHarness) BuildRunningServiceHarness(
	t *testing.T,
	timeout time.Duration,
) *ServiceTestHarness {
	t.Helper()
	return buildRunningProviderErrorSmokeServiceHarness(t, h.BuildServiceHarness(t), timeout)
}

// QueueProviderResults appends ordered provider subprocess outcomes to the smoke harness.
func (h *ProviderErrorSmokeHarness) QueueProviderResults(results ...workers.CommandResult) {
	h.providerRunner.Queue(results...)
}

// ProviderRunner exposes the recorded provider subprocess seam for assertions.
func (h *ProviderErrorSmokeHarness) ProviderRunner() *ProviderCommandRunner {
	return h.providerRunner
}

// SeedWork writes a stable named smoke-test submission into the copied fixture
// so startup preseed preserves deterministic WorkID and TraceID values.
func (h *ProviderErrorSmokeHarness) SeedWork(t *testing.T, work ProviderErrorSmokeWork) {
	t.Helper()
	WriteSeedRequest(t, h.Dir, submitRequestFromProviderErrorSmokeWork(work))
}

func writeProviderErrorSmokeWorkerConfig(
	t *testing.T,
	dir string,
	workerName string,
	provider workers.ModelProvider,
	model string,
	promptBody string,
) {
	t.Helper()

	workerDir := filepath.Join(dir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("writeProviderErrorSmokeWorkerConfig: create %s: %v", workerDir, err)
	}

	path := filepath.Join(workerDir, "AGENTS.md")
	if err := os.WriteFile(path, []byte(providerErrorSmokeWorkerConfig(provider, model, promptBody)), 0o644); err != nil {
		t.Fatalf("writeProviderErrorSmokeWorkerConfig: write %s: %v", path, err)
	}
}

func writeProviderErrorSmokeWorkstationConfig(
	t *testing.T,
	dir string,
	workstationName string,
	promptBody string,
) {
	t.Helper()

	workstationDir := filepath.Join(dir, "workstations", workstationName)
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("writeProviderErrorSmokeWorkstationConfig: create %s: %v", workstationDir, err)
	}

	path := filepath.Join(workstationDir, "AGENTS.md")
	if err := os.WriteFile(path, []byte(providerErrorSmokeWorkstationConfig(promptBody)), 0o644); err != nil {
		t.Fatalf("writeProviderErrorSmokeWorkstationConfig: write %s: %v", path, err)
	}
}

func submitRequestFromProviderErrorSmokeWork(work ProviderErrorSmokeWork) interfaces.SubmitRequest {
	return interfaces.SubmitRequest{
		Name:       work.Name,
		WorkID:     work.WorkID,
		WorkTypeID: work.WorkTypeID,
		TraceID:    work.TraceID,
		Payload:    append([]byte(nil), work.Payload...),
	}
}

func providerErrorSmokeWorkerConfig(provider workers.ModelProvider, model string, promptBody string) string {
	return `---
type: MODEL_WORKER
model: ` + model + `
modelProvider: ` + string(provider) + `
stopToken: COMPLETE
---
` + promptBody
}

func providerErrorSmokeWorkstationConfig(promptBody string) string {
	return `---
type: MODEL_WORKSTATION
---
` + promptBody
}

func buildRunningProviderErrorSmokeServiceHarness(
	t *testing.T,
	serviceHarness *ServiceTestHarness,
	timeout time.Duration,
) *ServiceTestHarness {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	errCh := serviceHarness.RunInBackground(ctx)
	t.Cleanup(func() {
		cancel()
		if err := <-errCh; err != nil && err != context.Canceled {
			t.Fatalf("RunInBackground() error = %v", err)
		}
	})

	return serviceHarness
}
