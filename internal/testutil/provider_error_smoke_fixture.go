package testutil

import (
	"os"
	"path/filepath"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
)

// ProviderErrorSmokeWork captures stable public submission fields used by
// provider-boundary fixture tests.
type ProviderErrorSmokeWork struct {
	Name       string
	WorkTypeID string
	WorkID     string
	TraceID    string
	Payload    []byte
}

func writeProviderErrorSmokeWorkerConfig(
	t *testing.T,
	dir string,
	workerName string,
	provider modelprovider.Provider,
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

func submitRequestFromProviderErrorSmokeWork(work ProviderErrorSmokeWork) workdomain.SubmitRequest {
	return workdomain.SubmitRequest{
		Name:       work.Name,
		WorkID:     work.WorkID,
		WorkTypeID: work.WorkTypeID,
		TraceID:    work.TraceID,
		Payload:    append([]byte(nil), work.Payload...),
	}
}

func providerErrorSmokeWorkerConfig(
	provider modelprovider.Provider,
	model string,
	promptBody string,
) string {
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
