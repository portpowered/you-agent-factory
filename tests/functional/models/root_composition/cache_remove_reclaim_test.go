package root_composition_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestModelsPublicOptInRemoveReclaimsUnsharedASRThroughCLIAndHTTP(t *testing.T) {
	t.Parallel()

	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	localHome := functionalTempDir(t)
	localCache := filepath.Join(localHome, "model-cache")
	localFixture := writeDefaultASRRemovalFixture(t, localCache)
	localEnvironment := isolatedModelEnvironment(localHome, localCache)
	localEvidence := measureASRReclamationEvidence(t, localFixture)

	localInputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "remove", models.BuiltInModelNameASR, "--reclaim-unused-cache",
	})
	localInputs.Input.Env = localEnvironment
	localInputs.Input.WorkingDirectory = factoryDir
	if err := functionalSharedDefaultProcess(t).Execute(localInputs.Input); err != nil {
		t.Fatalf("Process.Execute(local opt-in ASR remove) error = %v\nstdout=%s\nstderr=%s",
			err, localInputs.Stdout(), localInputs.Stderr())
	}
	var localResponse factoryapi.ModelRemoveResponse
	if err := json.Unmarshal([]byte(localInputs.Stdout()), &localResponse); err != nil {
		t.Fatalf("decode local opt-in remove response: %v\nstdout=%s", err, localInputs.Stdout())
	}
	assertASRReclaimResponse(t, localResponse, localFixture, localEvidence)
	assertASRReclaimEffects(t, localFixture)

	localRepeat := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "remove", models.BuiltInModelNameASR, "--reclaim-unused-cache",
	})
	localRepeat.Input.Env = localEnvironment
	localRepeat.Input.WorkingDirectory = factoryDir
	localRepeatErr := functionalSharedDefaultProcess(t).Execute(localRepeat.Input)
	assertASRReclaimRepeatNotFound(t, "local", localRepeatErr, localRepeat.Stdout(), localRepeat.Stderr())

	serverHome := functionalTempDir(t)
	serverCache := filepath.Join(serverHome, "model-cache")
	serverFixture := writeDefaultASRRemovalFixture(t, serverCache)
	serverEnvironment := isolatedModelEnvironment(serverHome, serverCache)
	serverEvidence := measureASRReclamationEvidence(t, serverFixture)
	if serverEvidence.modelBytes != localEvidence.modelBytes || serverEvidence.backendBytes != localEvidence.backendBytes {
		t.Fatalf("equivalent fixture snapshot bytes differ: local=%#v server=%#v", localEvidence, serverEvidence)
	}
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, Env: serverEnvironment, WaitForServiceModeRuntime: true,
		ServerReadyTimeout: 60 * time.Second,
	})
	defer server.Close(t)
	serverURL := server.URL()

	serverInputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", serverURL, "--json", "models", "remove", models.BuiltInModelNameASR,
		"--reclaim-unused-cache",
	})
	serverInputs.Input.Env = serverEnvironment
	serverInputs.Input.WorkingDirectory = factoryDir
	if err := server.Execute(t, serverInputs.Input); err != nil {
		t.Fatalf("Process.Execute(configured-server opt-in ASR remove) error = %v\nstdout=%s\nstderr=%s",
			err, serverInputs.Stdout(), serverInputs.Stderr())
	}
	var serverResponse factoryapi.ModelRemoveResponse
	if err := json.Unmarshal([]byte(serverInputs.Stdout()), &serverResponse); err != nil {
		t.Fatalf("decode configured-server opt-in remove response: %v\nstdout=%s", err, serverInputs.Stdout())
	}
	assertASRReclaimResponse(t, serverResponse, serverFixture, serverEvidence)
	assertASRReclaimEffects(t, serverFixture)
	if localResponse.ModelName != serverResponse.ModelName || localResponse.Revision != serverResponse.Revision ||
		localResponse.Outcome != serverResponse.Outcome || localResponse.BytesRemoved != serverResponse.BytesRemoved ||
		int64Value(localResponse.ReclaimedCacheBytes) != int64Value(serverResponse.ReclaimedCacheBytes) ||
		int64Value(localResponse.RetainedSharedCacheBytes) != int64Value(serverResponse.RetainedSharedCacheBytes) {
		t.Fatalf("normalized local/configured-server responses differ: local=%#v server=%#v", localResponse, serverResponse)
	}

	serverRepeat := support.FakeInputs(t.Context(), []string{
		"you", "--server", serverURL, "--json", "models", "remove", models.BuiltInModelNameASR,
		"--reclaim-unused-cache",
	})
	serverRepeat.Input.Env = serverEnvironment
	serverRepeat.Input.WorkingDirectory = factoryDir
	serverRepeatErr := server.Execute(t, serverRepeat.Input)
	assertASRReclaimRepeatNotFound(t, "configured-server", serverRepeatErr, serverRepeat.Stdout(), serverRepeat.Stderr())
}

type asrReclamationEvidence struct {
	modelBytes   int64
	backendBytes int64
}

func measureASRReclamationEvidence(t *testing.T, fixture defaultASRRemovalFixture) asrReclamationEvidence {
	t.Helper()
	evidence := asrReclamationEvidence{
		modelBytes:   story003RegularFileBytes(t, filepath.Dir(fixture.modelCASPath)),
		backendBytes: story003RegularFileBytes(t, filepath.Dir(fixture.backendPath)),
	}
	if evidence.modelBytes <= 0 || evidence.backendBytes <= 0 {
		t.Fatalf("ASR fixture cache measurements must be positive: %#v", evidence)
	}
	return evidence
}

func assertASRReclaimResponse(
	t *testing.T,
	response factoryapi.ModelRemoveResponse,
	fixture defaultASRRemovalFixture,
	evidence asrReclamationEvidence,
) {
	t.Helper()
	if response.ModelName != "ASR" || response.Revision != fixture.revision ||
		response.CachePath != fixture.revisionPath || response.BytesRemoved != fixture.revisionBytes ||
		response.Outcome != factoryapi.REMOVED || int64Value(response.ReclaimedCacheBytes) != evidence.modelBytes+evidence.backendBytes ||
		int64Value(response.RetainedSharedCacheBytes) != 0 {
		t.Fatalf("ASR opt-in remove response = %#v, want revision bytes %d and separate reclaimed bytes %d + %d",
			response, fixture.revisionBytes, evidence.modelBytes, evidence.backendBytes)
	}
	if response.ReclaimedCacheBytes == nil || response.RetainedSharedCacheBytes == nil {
		t.Fatalf("opt-in ASR response omitted cache byte fields: %#v", response)
	}
}

func assertASRReclaimEffects(t *testing.T, fixture defaultASRRemovalFixture) {
	t.Helper()
	for _, path := range []string{
		fixture.revisionPath,
		filepath.Dir(fixture.modelCASPath),
		filepath.Dir(fixture.backendPath),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("reclaimed ASR path %q stat error = %v, want not-exist", path, err)
		}
	}
	if body, err := os.ReadFile(fixture.siblingPath); err != nil || string(body) != "another managed revision" {
		t.Fatalf("ASR sibling revision changed: body=%q error=%v", body, err)
	}
}

func assertASRReclaimRepeatNotFound(t *testing.T, label string, err error, stdout, stderr string) {
	t.Helper()
	if err == nil || !errors.Is(err, modelscli.ErrModelCacheNotFound) {
		t.Fatalf("%s repeated opt-in remove error = %v, want typed cache-not-found", label, err)
	}
	if stdout != "" {
		t.Fatalf("%s repeated opt-in remove stdout = %q, want empty", label, stdout)
	}
	diagnostic := decodeFirstDiagnostic(t, stderr)
	if diagnostic.Code != factoryapi.ErrorResponseCodeMODELCACHENOTFOUND || diagnostic.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("%s repeated opt-in remove diagnostic = %#v, want MODEL_CACHE_NOT_FOUND/NOT_FOUND", label, diagnostic)
	}
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
