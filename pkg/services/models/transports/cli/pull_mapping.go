package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/models"
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	// A pull can take hours, but a fifteen-second heartbeat keeps a waiting
	// operator informed without turning diagnostics into a transfer log.
	modelPullProgressInterval = 15 * time.Second
)

type synchronizedWriter struct {
	mu     sync.Mutex
	output io.Writer
}

func newSynchronizedWriter(output io.Writer) io.Writer {
	if output == nil {
		return nil
	}
	return &synchronizedWriter{output: output}
}

func (writer *synchronizedWriter) Write(payload []byte) (int, error) {
	if writer == nil || writer.output == nil {
		return len(payload), nil
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.output.Write(payload)
}

func startPullProgress(
	ctx context.Context,
	modelName string,
	output io.Writer,
	interval time.Duration,
	now func() time.Time,
) func() {
	if output == nil || interval <= 0 || now == nil {
		return func() {}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	started := now()
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				_, _ = fmt.Fprintf(
					output,
					"models pull progress modelName=%q elapsed=%s\n",
					strings.TrimSpace(modelName), now().Sub(started).Round(time.Millisecond),
				)
			case <-ctx.Done():
				return
			case <-stop:
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

func pullResultToGenerated(result models.PullResult) factoryapi.ModelPullResponse {
	files := make([]factoryapi.ModelPullDownloadedFile, 0, len(result.DownloadedFiles))
	for _, file := range result.DownloadedFiles {
		current := factoryapi.ModelPullDownloadedFile{Path: file.Path, Bytes: file.Bytes}
		if sha := file.SHA256; sha != "" {
			current.Sha256 = &sha
		}
		files = append(files, current)
	}
	managedPull := managedRuntimePullResultToGenerated(result, files)
	response := factoryapi.ModelPullResponse{
		ModelName: result.ModelName, ProviderLocality: factoryapi.WorkerModelLocality(result.ProviderLocality),
		Outcome: modelPullOutcomeFromManagedRuntime(managedPull.PullOutcome), CachePath: result.CachePath,
		Revision: result.Revision, DownloadedFiles: files,
		ManagedRuntimePull: managedPull,
	}
	return response
}

func modelPullOutcomeFromManagedRuntime(outcome factoryapi.ManagedRuntimePullOutcome) factoryapi.ModelPullOutcome {
	switch outcome {
	case factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY:
		return factoryapi.ModelPullOutcomePULLED
	case factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT,
		factoryapi.ManagedRuntimePullOutcomeALREADYREADY:
		return factoryapi.ModelPullOutcomeALREADYPRESENT
	case factoryapi.ManagedRuntimePullOutcomeSTILLLOADING,
		factoryapi.ManagedRuntimePullOutcomeTIMEDOUT,
		factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED,
		factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME:
		return factoryapi.ModelPullOutcomeFAILED
	default:
		return factoryapi.ModelPullOutcomeFAILED
	}
}

func managedRuntimePullResultToGenerated(result models.PullResult, files []factoryapi.ModelPullDownloadedFile) factoryapi.ManagedRuntimePullResult {
	pull := factoryapi.ManagedRuntimePullResult{
		Identity:       result.ModelName,
		PullOutcome:    managedRuntimePullOutcome(result),
		ReadinessState: managedRuntimePullReadiness(result),
	}
	if cachePath := strings.TrimSpace(result.CachePath); cachePath != "" {
		pull.CachePath = &cachePath
	}
	if revision := strings.TrimSpace(result.Revision); revision != "" {
		pull.Revision = &revision
	}
	if len(files) > 0 {
		copied := append([]factoryapi.ModelPullDownloadedFile(nil), files...)
		pull.DownloadedFiles = &copied
	}
	if diagnostics := managedRuntimePullSourceDiagnostics(result); diagnostics != nil {
		pull.SourceDiagnostics = diagnostics
	}
	if diagnostics := managedRuntimePullDiagnostics(result); diagnostics != nil {
		pull.PullDiagnostics = diagnostics
	}
	return pull
}

func managedRuntimePullDiagnostics(result models.PullResult) *factoryapi.ManagedRuntimePullDiagnostics {
	diagnostics := result.PullDiagnostics.Normalize()
	if !diagnostics.HasDetails() {
		return nil
	}
	generated := factoryapi.ManagedRuntimePullDiagnostics{}
	if value := diagnostics.ModelName; value != "" {
		generated.ModelName = &value
	}
	if value := diagnostics.ResolvedRepository; value != "" {
		generated.ResolvedRepository = &value
	}
	if value := diagnostics.Revision; value != "" {
		generated.Revision = &value
	}
	if value := diagnostics.File; value != "" {
		generated.File = &value
	}
	if value := diagnostics.Operation; value != "" {
		generated.Operation = &value
	}
	if value := diagnostics.RequestURL; value != "" {
		generated.RequestUrl = &value
	}
	if diagnostics.UpstreamStatusCode != 0 {
		value := int32(diagnostics.UpstreamStatusCode)
		generated.UpstreamStatusCode = &value
	}
	return &generated
}

func managedRuntimePullOutcome(result models.PullResult) factoryapi.ManagedRuntimePullOutcome {
	if outcome := strings.TrimSpace(result.ManagedPullOutcome); outcome != "" {
		return factoryapi.ManagedRuntimePullOutcome(outcome)
	}
	switch factoryapi.ModelPullOutcome(strings.TrimSpace(strings.ToUpper(result.Outcome))) {
	case factoryapi.ModelPullOutcomePULLED:
		return factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY
	case factoryapi.ModelPullOutcomeALREADYPRESENT:
		return factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT
	default:
		return factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME
	}
}

func managedRuntimePullReadiness(result models.PullResult) factoryapi.ManagedRuntimeReadinessState {
	if readiness := strings.TrimSpace(result.ReadinessState); readiness != "" {
		return factoryapi.ManagedRuntimeReadinessState(readiness)
	}
	return factoryapi.ManagedRuntimeReadinessStateREADY
}

func managedRuntimePullSourceDiagnostics(result models.PullResult) *factoryapi.ManagedRuntimeSourceDiagnostics {
	sourceKind := strings.TrimSpace(result.SourceKind)
	sourceID := strings.TrimSpace(result.SourceID)
	resolverNotes := strings.TrimSpace(result.ResolverNotes)
	if sourceKind == "" && sourceID == "" && resolverNotes == "" {
		return nil
	}
	diagnostics := factoryapi.ManagedRuntimeSourceDiagnostics{}
	if sourceKind != "" {
		diagnostics.SourceKind = &sourceKind
	}
	if sourceID != "" {
		diagnostics.SourceId = &sourceID
	}
	if resolverNotes != "" {
		diagnostics.ResolverNotes = &resolverNotes
	}
	return &diagnostics
}

func renderList(response factoryapi.ListModelsResponse, output io.Writer) error {
	if _, err := fmt.Fprintln(output, "NAME\tREADINESS\tLIFECYCLE\tLOCALITY\tOPERATIONS\tMODALITIES\tRESOURCES\tCACHE SIZE"); err != nil {
		return err
	}
	results := append([]factoryapi.ModelSummary(nil), response.Results...)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	for _, model := range results {
		if _, err := fmt.Fprintf(
			output,
			"%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
			model.Name,
			managedRuntimeReadiness(model.ManagedRuntime),
			managedRuntimeLifecycle(model.ManagedRuntime),
			model.ProviderLocality,
			modelOperationNames(model.Operations),
			modelModalities(model.Modalities),
			len(model.Resources),
			managedRuntimeCacheSize(model.ManagedRuntime),
		); err != nil {
			return err
		}
	}
	return nil
}

func managedRuntimeCacheSize(runtime factoryapi.ManagedRuntime) string {
	if runtime.CacheBytes == nil || *runtime.CacheBytes < 0 {
		return "NOT_INSTALLED"
	}
	return fmt.Sprintf("%s (%d bytes)", humanByteSize(*runtime.CacheBytes), *runtime.CacheBytes)
}

func managedRuntimeRevision(runtime factoryapi.ManagedRuntime) string {
	if runtime.Revision == nil || strings.TrimSpace(*runtime.Revision) == "" {
		return "NOT_INSTALLED"
	}
	return *runtime.Revision
}

func managedRuntimeCachePath(runtime factoryapi.ManagedRuntime) string {
	if runtime.CachePath == nil || strings.TrimSpace(*runtime.CachePath) == "" {
		return "NOT_INSTALLED"
	}
	return *runtime.CachePath
}

func humanByteSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	for _, unit := range units {
		value /= 1024
		if value < 1024 || unit == units[len(units)-1] {
			return fmt.Sprintf("%.2f %s", value, unit)
		}
	}
	return fmt.Sprintf("%d B", bytes)
}

func renderPull(response factoryapi.ModelPullResponse, output io.Writer) error {
	if _, err := fmt.Fprintf(
		output,
		"MODEL\tPULL OUTCOME\tREADINESS\tLIFECYCLE\tREVISION\tCACHE PATH\n%s\t%s\t%s\t%s\t%s\t%s\n",
		response.ModelName,
		response.ManagedRuntimePull.PullOutcome,
		response.ManagedRuntimePull.ReadinessState,
		managedRuntimeLifecycleFromPull(response),
		response.Revision,
		response.CachePath,
	); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, "FILES"); err != nil {
		return err
	}
	files := append([]factoryapi.ModelPullDownloadedFile(nil), response.DownloadedFiles...)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	for _, file := range files {
		if _, err := fmt.Fprintf(output, "%s\t%d\n", file.Path, file.Bytes); err != nil {
			return err
		}
	}
	return nil
}

func renderModel(model factoryapi.ModelDetail, output io.Writer) error {
	if _, err := fmt.Fprintf(output, "Name:\t%s\n", model.Name); err != nil {
		return err
	}
	for _, row := range []struct {
		label string
		value string
	}{
		{label: "Readiness", value: managedRuntimeReadiness(model.ManagedRuntime)},
		{label: "Lifecycle", value: managedRuntimeLifecycle(model.ManagedRuntime)},
		{label: "Locality", value: string(model.ProviderLocality)},
		{label: "Revision", value: managedRuntimeRevision(model.ManagedRuntime)},
		{label: "Cache Size", value: managedRuntimeCacheSize(model.ManagedRuntime)},
		{label: "Cache Path", value: managedRuntimeCachePath(model.ManagedRuntime)},
		{label: "Operations", value: modelOperationNames(model.Operations)},
		{label: "Modalities", value: modelModalities(model.Modalities)},
	} {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", row.label, row.value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(output, "Resources:\t%d\n", len(model.Resources)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, "Capabilities:"); err != nil {
		return err
	}
	for _, capability := range model.Capabilities {
		if _, err := fmt.Fprintf(
			output,
			"- %s\t%s\t%s\n",
			capability.Worker,
			capability.ProviderLocality,
			modelOperationNames(capability.Operations),
		); err != nil {
			return err
		}
	}
	diagnostics := managedRuntimeDiagnosticsMap(model)
	if len(diagnostics) > 0 {
		keys := make([]string, 0, len(diagnostics))
		for key := range diagnostics {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if _, err := fmt.Fprintln(output, "Diagnostics:"); err != nil {
			return err
		}
		for _, key := range keys {
			if _, err := fmt.Fprintf(output, "- %s=%s\n", key, diagnostics[key]); err != nil {
				return err
			}
		}
	}
	return nil
}

func managedRuntimeReadiness(runtime factoryapi.ManagedRuntime) string {
	if strings.TrimSpace(string(runtime.ReadinessState)) == "" {
		return "UNKNOWN"
	}
	return string(runtime.ReadinessState)
}

func managedRuntimeLifecycle(runtime factoryapi.ManagedRuntime) string {
	if strings.TrimSpace(string(runtime.LifecycleState)) == "" {
		return "UNKNOWN"
	}
	return string(runtime.LifecycleState)
}

func managedRuntimeLifecycleFromPull(response factoryapi.ModelPullResponse) string {
	switch response.ManagedRuntimePull.PullOutcome {
	case factoryapi.ManagedRuntimePullOutcomeSTILLLOADING:
		return string(factoryapi.ManagedRuntimeLifecycleStateINSTALLING)
	case factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY,
		factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT,
		factoryapi.ManagedRuntimePullOutcomeALREADYREADY:
		return string(factoryapi.ManagedRuntimeLifecycleStateINSTALLED)
	default:
		return "UNKNOWN"
	}
}

func managedRuntimeDiagnosticsMap(model factoryapi.ModelDetail) factoryapi.StringMap {
	if model.ManagedRuntime.Diagnostics != nil && len(*model.ManagedRuntime.Diagnostics) > 0 {
		return *model.ManagedRuntime.Diagnostics
	}
	return model.Diagnostics
}

func modelOperationNames(operations []factoryapi.ModelInvocationOperation) string {
	names := make([]string, 0, len(operations))
	for _, operation := range operations {
		names = append(names, operation.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func modelModalities(modalities []factoryapi.ModelInvocationContentType) string {
	values := make([]string, 0, len(modalities))
	for _, modality := range modalities {
		values = append(values, string(modality))
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

func managedRuntimePullResponseError(statusCode int, body []byte) error {
	if statusCode != http.StatusUnprocessableEntity && statusCode != http.StatusGatewayTimeout {
		return nil
	}
	var response factoryapi.ModelPullResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil
	}
	if !isManagedRuntimePullFailure(response.ManagedRuntimePull.PullOutcome) {
		return nil
	}
	return &managedRuntimePullFailure{
		Outcome:     response.ManagedRuntimePull.PullOutcome,
		Readiness:   response.ManagedRuntimePull.ReadinessState,
		Diagnostics: managedRuntimePullDiagnosticsFromGenerated(response.ManagedRuntimePull.PullDiagnostics),
	}
}

func isManagedRuntimePullFailure(outcome factoryapi.ManagedRuntimePullOutcome) bool {
	switch outcome {
	case "":
		return false
	case factoryapi.ManagedRuntimePullOutcomeALREADYREADY,
		factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY,
		factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT:
		return false
	default:
		return true
	}
}

func managedRuntimePullDiagnosticsFromGenerated(
	input *factoryapi.ManagedRuntimePullDiagnostics,
) error {
	if input == nil {
		return nil
	}
	diagnostics := models.PullDiagnostics{}
	if input.ModelName != nil {
		diagnostics.ModelName = *input.ModelName
	}
	if input.ResolvedRepository != nil {
		diagnostics.ResolvedRepository = *input.ResolvedRepository
	}
	if input.Revision != nil {
		diagnostics.Revision = *input.Revision
	}
	if input.File != nil {
		diagnostics.File = *input.File
	}
	if input.Operation != nil {
		diagnostics.Operation = *input.Operation
	}
	if input.RequestUrl != nil {
		diagnostics.RequestURL = *input.RequestUrl
	}
	if input.UpstreamStatusCode != nil {
		diagnostics.UpstreamStatusCode = int(*input.UpstreamStatusCode)
	}
	return pullsupport.NewPullDiagnosticsError(diagnostics, nil)
}

type managedRuntimePullFailure struct {
	Outcome     factoryapi.ManagedRuntimePullOutcome
	Readiness   factoryapi.ManagedRuntimeReadinessState
	Diagnostics error
}

func (failure *managedRuntimePullFailure) Error() string {
	if failure == nil {
		return ""
	}
	return fmt.Sprintf(
		"managed runtime pull failed (pullOutcome=%s readinessState=%s)",
		failure.Outcome,
		failure.Readiness,
	)
}

func (failure *managedRuntimePullFailure) CLIErrorCode() string {
	return managedRuntimePullFailureCode
}

func (failure *managedRuntimePullFailure) CLIErrorFamily() factoryapi.ErrorFamily {
	return factoryapi.ErrorFamilyBadRequest
}

func (failure *managedRuntimePullFailure) CLIErrorMessage() string {
	return failure.Error()
}

func (failure *managedRuntimePullFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Diagnostics
}

func modelsRequestError(statusCode int, body []byte, response ...*http.Response) error {
	var httpResponse *http.Response
	if len(response) > 0 {
		httpResponse = response[0]
	}
	var errResp factoryapi.ErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Message != "" {
		switch errResp.Code {
		case factoryapi.ErrorResponseCodeMODELCACHENOTFOUND:
			displayMessage := fmt.Sprintf("%s: %s", ErrModelCacheNotFound, errResp.Message)
			if httpResponse != nil {
				return clihttp.NewAPIErrorFromResponse(httpResponse, errResp, displayMessage, ErrModelCacheNotFound)
			}
			return clihttp.NewAPIError(statusCode, errResp, displayMessage, ErrModelCacheNotFound)
		case factoryapi.ErrorResponseCodeMODELCACHEINUSE:
			displayMessage := fmt.Sprintf("%s: %s", ErrModelCacheInUse, errResp.Message)
			if httpResponse != nil {
				return clihttp.NewAPIErrorFromResponse(httpResponse, errResp, displayMessage, ErrModelCacheInUse)
			}
			return clihttp.NewAPIError(statusCode, errResp, displayMessage, ErrModelCacheInUse)
		}
		if statusCode == http.StatusNotFound && errResp.Code == factoryapi.ErrorResponseCodeNOTFOUND {
			if httpResponse != nil {
				return clihttp.NewAPIErrorFromResponse(httpResponse, errResp, fmt.Sprintf("%s: %s", ErrModelNotFound, errResp.Message), ErrModelNotFound)
			}
			return clihttp.NewAPIError(statusCode, errResp, fmt.Sprintf("%s: %s", ErrModelNotFound, errResp.Message), ErrModelNotFound)
		}
		if httpResponse != nil {
			return clihttp.NewAPIErrorFromResponse(httpResponse, errResp, fmt.Sprintf("models request failed (%d): %s", statusCode, errResp.Message), nil)
		}
		return clihttp.NewAPIError(statusCode, errResp, fmt.Sprintf("models request failed (%d): %s", statusCode, errResp.Message), nil)
	}
	return clihttp.WithHTTPResponse(
		httpResponse,
		fmt.Errorf("models request failed (%d): response body was not a structured API error", statusCode),
	)
}

func modelsEndpoint(server, path string) (url.URL, error) {
	endpointURL, err := cliserver.RequestURL(server, path)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse models endpoint: %w", err)
	}
	return *endpoint, nil
}

func mustGeneratedTextContentPart(text string) factoryapi.WorkContentPart {
	var part factoryapi.WorkContentPart
	slot := "text"
	_ = part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeTextUpper,
		Text: text,
		Slot: &slot,
	})
	return part
}
