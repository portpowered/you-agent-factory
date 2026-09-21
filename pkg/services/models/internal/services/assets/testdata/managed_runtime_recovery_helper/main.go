//go:build managed_process_integration

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	appwire "github.com/portpowered/infinite-you/pkg/wire"
)

const (
	ownerMode                 = "owner"
	retryMode                 = "retry"
	managedMetadataStageName  = ".managed-cache.json.partial"
	managedRecoveryStagePhase = "MANAGED_METADATA_STAGE_WRITTEN"
)

type recoveryConfiguration struct {
	CacheDirectory  string                    `json:"cacheDirectory"`
	SourceDirectory string                    `json:"sourceDirectory"`
	SignalPath      string                    `json:"signalPath"`
	ModelName       string                    `json:"modelName"`
	Artifacts       []models.AssetRequirement `json:"artifacts"`
}

type recoveryStageSignal struct {
	Phase             string `json:"phase"`
	PID               int    `json:"pid"`
	RevisionStagePath string `json:"revisionStagePath"`
	MetadataStagePath string `json:"metadataStagePath"`
}

type recoveryArtifactIdentity struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type recoveryObservation struct {
	Outcome         string                     `json:"outcome"`
	AssetReadiness  string                     `json:"assetReadiness"`
	AssetIntegrity  string                     `json:"assetIntegrity"`
	Artifacts       []recoveryArtifactIdentity `json:"artifacts"`
	CatalogStatus   string                     `json:"catalogStatus"`
	Readiness       string                     `json:"readiness"`
	Lifecycle       string                     `json:"lifecycle"`
	CachePath       string                     `json:"cachePath"`
	NetworkRequests int64                      `json:"networkRequests"`
}

type noNetworkAssetClient struct {
	requests atomic.Int64
}

func (client *noNetworkAssetClient) Do(*http.Request) (*http.Response, error) {
	client.requests.Add(1)
	return nil, errors.New("managed-runtime recovery helper prohibits network access")
}

func main() {
	mode := flag.String("mode", "", "owner or retry")
	configPath := flag.String("config", "", "absolute isolated recovery configuration path")
	flag.Parse()
	if err := run(*mode, *configPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(mode, configPath string) error {
	if mode != ownerMode && mode != retryMode {
		return fmt.Errorf("unsupported recovery helper mode %q", mode)
	}
	configuration, err := readConfiguration(configPath)
	if err != nil {
		return err
	}
	client := &noNetworkAssetClient{}
	writeAsset := managedStageWriter{mode: mode, configuration: configuration}
	service, err := appwire.NewModelsServiceForManagedProcessIntegration(serviceedges.Edges{
		ModelAssetHTTPClient: client,
		ModelAssetWriteFile:  writeAsset.write,
	})
	if err != nil {
		return fmt.Errorf("construct Models service: %w", err)
	}
	return prepareAndInspect(service, client, configuration, mode)
}

func readConfiguration(path string) (recoveryConfiguration, error) {
	if !filepath.IsAbs(path) {
		return recoveryConfiguration{}, errors.New("recovery configuration path must be absolute")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return recoveryConfiguration{}, fmt.Errorf("read recovery configuration: %w", err)
	}
	var configuration recoveryConfiguration
	if err := json.Unmarshal(body, &configuration); err != nil {
		return recoveryConfiguration{}, fmt.Errorf("decode recovery configuration: %w", err)
	}
	if err := configuration.validate(); err != nil {
		return recoveryConfiguration{}, err
	}
	return configuration, nil
}

func (configuration recoveryConfiguration) validate() error {
	for label, path := range map[string]string{
		"cache directory":  configuration.CacheDirectory,
		"source directory": configuration.SourceDirectory,
		"signal path":      configuration.SignalPath,
	} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("recovery %s must be absolute", label)
		}
	}
	if strings.TrimSpace(configuration.ModelName) == "" || len(configuration.Artifacts) < 2 {
		return errors.New("recovery configuration requires a model name and at least two artifacts")
	}
	return nil
}

func prepareAndInspect(
	service models.Service,
	client *noNetworkAssetClient,
	configuration recoveryConfiguration,
	mode string,
) error {
	ctx := context.Background()
	opened, err := service.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{
			CacheDirectory: configuration.CacheDirectory,
			OperatorModels: recoveryModelOverlay(configuration),
		},
	})
	if err != nil {
		return fmt.Errorf("open isolated Models runtime scope: %w", err)
	}
	defer closeModelsScope(service, opened.Scope)
	prepared, err := service.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{
		Scope:     opened.Scope,
		Name:      configuration.ModelName,
		Reference: models.ModelReference{NameOrURI: configuration.SourceDirectory},
		Offline:   mode == retryMode,
		Artifacts: configuration.Artifacts,
	})
	if err != nil {
		return fmt.Errorf("prepare managed model assets: %w", err)
	}
	return writeObservation(service, client, opened.Scope, configuration, prepared.Outcome)
}

func recoveryModelOverlay(configuration recoveryConfiguration) map[string]models.ModelOverlay {
	source := configuration.SourceDirectory
	backend := "localai-controlled-fixture"
	loadPolicy := models.LoadPolicyOnDemand
	return map[string]models.ModelOverlay{
		configuration.ModelName: {
			Source: &source, Backend: &backend, LoadPolicy: &loadPolicy,
			Operations: []string{models.OperationOMNI},
		},
	}
}

func closeModelsScope(service models.Service, scope models.RuntimeScopeRef) {
	_, _ = service.CloseRuntimeScope(context.Background(), models.CloseRuntimeScopeRequest{Scope: scope})
	if closer, ok := service.(interface{ Close(context.Context) error }); ok {
		_ = closer.Close(context.Background())
	}
}

func writeObservation(
	service models.Service,
	client *noNetworkAssetClient,
	scope models.RuntimeScopeRef,
	configuration recoveryConfiguration,
	outcome models.AssetPreparationOutcome,
) error {
	assets, err := service.InspectModelAssets(context.Background(), models.InspectModelAssetsRequest{
		Scope: scope, Name: configuration.ModelName, VerifyIntegrity: true,
	})
	if err != nil {
		return fmt.Errorf("inspect verified content-addressed inputs: %w", err)
	}
	catalog, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: scope, Name: configuration.ModelName,
	})
	if err != nil {
		return fmt.Errorf("inspect managed runtime readiness: %w", err)
	}
	observation := recoveryObservationFrom(assets.Asset, catalog.Model, outcome, client.requests.Load())
	encoded, err := json.Marshal(observation)
	if err != nil {
		return fmt.Errorf("encode recovery observation: %w", err)
	}
	_, err = os.Stdout.Write(append(encoded, '\n'))
	return err
}

func recoveryObservationFrom(
	assets models.AssetSnapshot,
	model models.Detail,
	outcome models.AssetPreparationOutcome,
	networkRequests int64,
) recoveryObservation {
	observation := recoveryObservation{
		Outcome: string(outcome), AssetReadiness: string(assets.Readiness),
		AssetIntegrity: string(assets.Integrity), CatalogStatus: string(model.Status),
		Readiness:       string(model.ManagedRuntime.ReadinessState),
		Lifecycle:       string(model.ManagedRuntime.LifecycleState),
		NetworkRequests: networkRequests,
	}
	if model.ManagedRuntime.CachePath != nil {
		observation.CachePath = *model.ManagedRuntime.CachePath
	}
	for _, artifact := range assets.Artifacts {
		observation.Artifacts = append(observation.Artifacts, recoveryArtifactIdentity{
			Name: artifact.Name, Bytes: artifact.Bytes, SHA256: artifact.SHA256,
		})
	}
	return observation
}

type managedStageWriter struct {
	mode          string
	configuration recoveryConfiguration
}

func (writer managedStageWriter) write(path string, body []byte, mode os.FileMode) error {
	managedMetadataStagePath := filepath.Join(
		writer.configuration.CacheDirectory,
		strings.ToUpper(strings.TrimSpace(writer.configuration.ModelName)),
		managedMetadataStageName,
	)
	if writer.mode != ownerMode || filepath.Clean(path) != filepath.Clean(managedMetadataStagePath) {
		return os.WriteFile(path, body, mode)
	}
	if err := os.WriteFile(path, body, mode); err != nil {
		return err
	}
	stagePath, err := completedRevisionStage(filepath.Dir(path), writer.configuration.Artifacts)
	if err != nil {
		return err
	}
	if err := writer.signalStage(path, stagePath); err != nil {
		return err
	}
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		return fmt.Errorf("await process-death observation: %w", err)
	}
	return errors.New("managed-stage owner was released before termination")
}

func (writer managedStageWriter) signalStage(metadataPath, revisionPath string) error {
	signal := recoveryStageSignal{
		Phase: managedRecoveryStagePhase, PID: os.Getpid(),
		RevisionStagePath: revisionPath, MetadataStagePath: metadataPath,
	}
	body, err := json.Marshal(signal)
	if err != nil {
		return fmt.Errorf("encode managed-stage signal: %w", err)
	}
	temporaryPath := writer.configuration.SignalPath + ".writing"
	if err := os.WriteFile(temporaryPath, body, 0o600); err != nil {
		return fmt.Errorf("publish managed-stage signal: %w", err)
	}
	if err := os.Rename(temporaryPath, writer.configuration.SignalPath); err != nil {
		return fmt.Errorf("publish managed-stage signal: %w", err)
	}
	return nil
}

func completedRevisionStage(root string, artifacts []models.AssetRequirement) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("inspect managed model root: %w", err)
	}
	var match string
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".partial") {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		if !containsExpectedArtifacts(candidate, artifacts) {
			continue
		}
		if match != "" {
			return "", errors.New("multiple completed managed revision stages were observed")
		}
		match = candidate
	}
	if match == "" {
		return "", errors.New("completed managed revision stage was not observed")
	}
	return match, nil
}

func containsExpectedArtifacts(stagePath string, artifacts []models.AssetRequirement) bool {
	for _, artifact := range artifacts {
		body, err := os.ReadFile(filepath.Join(stagePath, filepath.FromSlash(artifact.Name)))
		if err != nil || int64(len(body)) != artifact.Bytes || digest(body) != artifact.SHA256 {
			return false
		}
	}
	return true
}

func digest(body []byte) string {
	value := sha256.Sum256(body)
	return hex.EncodeToString(value[:])
}
