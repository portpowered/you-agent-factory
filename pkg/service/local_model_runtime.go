package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

type localModelCacheLayout struct {
	ModelName string
	CachePath string
	Revision  string
	Files     []string
}

type localModelLoadRequest struct {
	Resource  interfaces.ResourceConfig
	Worker    *interfaces.WorkerConfig
	ModelName string
	CachePath string
	Revision  string
	Files     []string
}

type localModelInvocationRequest struct {
	Resource interfaces.ResourceConfig
	Worker   *interfaces.WorkerConfig
	Request  interfaces.RunnerExecutionRequest
}

type localModelHandle interface {
	Invoke(context.Context, localModelInvocationRequest) (interfaces.InferenceResponse, error)
}

type localModelRuntime interface {
	Supports(resource interfaces.ResourceConfig, worker *interfaces.WorkerConfig) bool
	Load(context.Context, localModelLoadRequest) (localModelHandle, error)
}

type managedLocalModelManager struct {
	mu          sync.Mutex
	entries     map[string]*managedLocalModelEntry
	assetPuller modelAssetPuller
	runtime     localModelRuntime
}

type managedLocalModelEntry struct {
	mu     sync.Mutex
	handle localModelHandle
}

type localModelRunner struct {
	inner      workers.Runner
	manager    *managedLocalModelManager
	runtimeCfg interfaces.RuntimeConfigLookup
	factoryCfg *interfaces.FactoryConfig
	workerDef  *interfaces.WorkerConfig
}

func newManagedLocalModelManager(assetPuller modelAssetPuller, runtime localModelRuntime) *managedLocalModelManager {
	if assetPuller == nil || runtime == nil {
		return nil
	}
	return &managedLocalModelManager{
		entries:     make(map[string]*managedLocalModelEntry),
		assetPuller: assetPuller,
		runtime:     runtime,
	}
}

func (m *managedLocalModelManager) wrapRunner(
	inner workers.Runner,
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
) workers.Runner {
	if inner == nil || m == nil || runtimeCfg == nil || factoryCfg == nil || workerDef == nil {
		return inner
	}
	if workerDef.Type != interfaces.WorkerTypeModel || workerDef.ModelLocality != interfaces.ModelLocalityLocal {
		return inner
	}
	return &localModelRunner{
		inner:      inner,
		manager:    m,
		runtimeCfg: runtimeCfg,
		factoryCfg: factoryCfg,
		workerDef:  workerDef,
	}
}

func (r *localModelRunner) Execute(ctx context.Context, request interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error) {
	if r == nil || r.manager == nil {
		return r.inner.Execute(ctx, request)
	}
	response, handled, err := r.manager.execute(ctx, r.runtimeCfg, r.factoryCfg, r.workerDef, request)
	if !handled {
		return r.inner.Execute(ctx, request)
	}
	return response, err
}

func (m *managedLocalModelManager) execute(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
	request interfaces.RunnerExecutionRequest,
) (interfaces.InferenceResponse, bool, error) {
	resource, resourceKey, ok := localModelRuntimeResource(factoryCfg, workerDef)
	if !ok || !m.runtime.Supports(resource, workerDef) {
		return interfaces.InferenceResponse{}, false, nil
	}
	loaded, err := runtimeCfgForLocalModel(runtimeCfg)
	if err != nil {
		return interfaces.InferenceResponse{}, true, err
	}
	cacheLayout, err := m.assetPuller.ResolveModelCache(ctx, loaded, workerDef)
	if err != nil {
		return interfaces.InferenceResponse{}, true, err
	}
	loadWorker := factoryconfig.CloneWorkerConfig(*workerDef)
	handle, err := m.loadHandle(ctx, resourceKey, localModelLoadRequest{
		Resource:  resource,
		Worker:    &loadWorker,
		ModelName: cacheLayout.ModelName,
		CachePath: cacheLayout.CachePath,
		Revision:  cacheLayout.Revision,
		Files:     append([]string(nil), cacheLayout.Files...),
	})
	if err != nil {
		return interfaces.InferenceResponse{}, true, err
	}
	invokeWorker := factoryconfig.CloneWorkerConfig(*workerDef)
	response, err := handle.Invoke(ctx, localModelInvocationRequest{
		Resource: resource,
		Worker:   &invokeWorker,
		Request:  interfaces.CloneProviderInferenceRequest(request),
	})
	return response, true, err
}

func runtimeCfgForLocalModel(runtimeCfg interfaces.RuntimeConfigLookup) (*factoryconfig.LoadedFactoryConfig, error) {
	loaded, ok := runtimeCfg.(*factoryconfig.LoadedFactoryConfig)
	if !ok || loaded == nil {
		return nil, fmt.Errorf("loaded runtime config is required for local model execution")
	}
	return loaded, nil
}

func (m *managedLocalModelManager) loadHandle(ctx context.Context, key string, request localModelLoadRequest) (localModelHandle, error) {
	entry := m.entry(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.handle != nil {
		markModelExecutionLoadReused(ctx)
		return entry.handle, nil
	}
	markModelExecutionLoadRequested(ctx, time.Now())
	handle, err := m.runtime.Load(ctx, request)
	if err != nil {
		markModelExecutionLoadFinished(ctx, time.Now())
		return nil, err
	}
	markModelExecutionLoadFinished(ctx, time.Now())
	entry.handle = handle
	return handle, nil
}

func (m *managedLocalModelManager) entry(key string) *managedLocalModelEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[key]
	if ok {
		return entry
	}
	entry = &managedLocalModelEntry{}
	m.entries[key] = entry
	return entry
}

func localModelRuntimeResource(factoryCfg *interfaces.FactoryConfig, workerDef *interfaces.WorkerConfig) (interfaces.ResourceConfig, string, bool) {
	if factoryCfg == nil || workerDef == nil || workerDef.ModelLocality != interfaces.ModelLocalityLocal {
		return interfaces.ResourceConfig{}, "", false
	}
	if len(workerDef.Resources) == 0 {
		return interfaces.ResourceConfig{}, "", false
	}
	resourcesByName := make(map[string]interfaces.ResourceConfig, len(factoryCfg.Resources))
	for _, resource := range factoryCfg.Resources {
		resourcesByName[resource.Name] = resource
	}
	for _, requirement := range workerDef.Resources {
		resource, ok := resourcesByName[requirement.Name]
		if !ok || !isProcessScopedLocalModelResource(resource) {
			continue
		}
		if canonicalModelName(resource.Model) != canonicalModelName(workerDef.Model) {
			continue
		}
		key := localModelResourceKey(resource)
		if key == "" {
			continue
		}
		return resource, key, true
	}
	return interfaces.ResourceConfig{}, "", false
}

func canonicalBackendName(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

const (
	defaultOmniVoiceCommand       = "omnivoice-llamacpp"
	omniVoiceInvokeSubcommand     = "invoke"
	omniVoiceAudioContentType     = "audio/wav"
	omniVoiceModelSlotNameText    = "text"
	omniVoiceModelSlotNameAudio   = "audio"
	omniVoiceTokenizerNameSnippet = "tokenizer"
)

type omniVoiceLocalRuntime struct {
	runner workers.CommandRunner
}

type omniVoiceLocalHandle struct {
	runner        workers.CommandRunner
	command       string
	baseArgs      []string
	modelName     string
	cachePath     string
	revision      string
	modelPath     string
	tokenizerPath string
}

type omniVoiceInvocationPayload struct {
	Operation  string                                     `json:"operation"`
	ModelName  string                                     `json:"modelName"`
	Revision   string                                     `json:"revision,omitempty"`
	OutputFile string                                     `json:"outputFile"`
	Text       string                                     `json:"text"`
	Bindings   []interfaces.ResolvedModelOperationBinding `json:"bindings,omitempty"`
}

func newOmniVoiceLocalRuntime(runner workers.CommandRunner) localModelRuntime {
	if runner == nil {
		runner = workers.ExecCommandRunner{}
	}
	return &omniVoiceLocalRuntime{runner: runner}
}

func (r *omniVoiceLocalRuntime) Supports(resource interfaces.ResourceConfig, worker *interfaces.WorkerConfig) bool {
	if worker == nil {
		return false
	}
	return strings.TrimSpace(worker.ModelLocality) == interfaces.ModelLocalityLocal &&
		canonicalBackendName(resource.Backend) == "LLAMACPP" &&
		canonicalModelName(worker.Model) == canonicalModelName("OMNIVOICE_Q4_K_M")
}

func (r *omniVoiceLocalRuntime) Load(_ context.Context, request localModelLoadRequest) (localModelHandle, error) {
	if !r.Supports(request.Resource, request.Worker) {
		return nil, fmt.Errorf("unsupported local model runtime for model %q with backend %q", request.ModelName, request.Resource.Backend)
	}
	modelPath, tokenizerPath, err := omniVoiceCacheFiles(request.Files)
	if err != nil {
		return nil, err
	}
	command, args := omniVoiceCommandForWorker(request.Worker)
	return &omniVoiceLocalHandle{
		runner:        r.runner,
		command:       command,
		baseArgs:      args,
		modelName:     request.ModelName,
		cachePath:     request.CachePath,
		revision:      request.Revision,
		modelPath:     modelPath,
		tokenizerPath: tokenizerPath,
	}, nil
}

func (h *omniVoiceLocalHandle) Invoke(ctx context.Context, request localModelInvocationRequest) (interfaces.InferenceResponse, error) {
	if h == nil {
		return interfaces.InferenceResponse{}, fmt.Errorf("local model handle is required")
	}
	operation := strings.TrimSpace(request.Request.ModelOperation)
	if operation != "TTS" {
		return interfaces.InferenceResponse{}, fmt.Errorf("local OMNIVOICE runtime only supports TTS, got %q", operation)
	}
	text, err := omniVoiceBoundText(request.Request.ModelBindings)
	if err != nil {
		return interfaces.InferenceResponse{}, err
	}
	outputFile, err := omniVoiceOutputPath(h.cachePath)
	if err != nil {
		return interfaces.InferenceResponse{}, err
	}

	payload := omniVoiceInvocationPayload{
		Operation:  operation,
		ModelName:  h.modelName,
		Revision:   h.revision,
		OutputFile: outputFile,
		Text:       text,
		Bindings:   interfaces.CloneResolvedModelOperationBindings(request.Request.ModelBindings),
	}
	stdin, err := json.Marshal(payload)
	if err != nil {
		return interfaces.InferenceResponse{}, fmt.Errorf("encode local OMNIVOICE invocation payload: %w", err)
	}

	result, err := h.runner.Run(ctx, omniVoiceCommandRequest(request.Request, h, outputFile, stdin))
	if err != nil {
		return interfaces.InferenceResponse{}, fmt.Errorf("run local OMNIVOICE runtime: %w", err)
	}
	if result.ExitCode != 0 {
		return interfaces.InferenceResponse{}, fmt.Errorf("local OMNIVOICE runtime exited with code %d: %s", result.ExitCode, omniVoiceCombinedOutput(result))
	}

	content, err := omniVoiceResponseContent(strings.TrimSpace(string(result.Stdout)), outputFile)
	if err != nil {
		return interfaces.InferenceResponse{}, err
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return interfaces.InferenceResponse{}, fmt.Errorf("encode local OMNIVOICE response content: %w", err)
	}
	return interfaces.InferenceResponse{Content: string(encoded)}, nil
}

func omniVoiceCacheFiles(files []string) (string, string, error) {
	var modelPath string
	var tokenizerPath string
	for _, file := range files {
		trimmed := strings.TrimSpace(file)
		if trimmed == "" {
			continue
		}
		info, err := os.Stat(trimmed)
		if err != nil {
			return "", "", fmt.Errorf("inspect local model asset %q: %w", trimmed, err)
		}
		if info.IsDir() {
			return "", "", fmt.Errorf("local model asset %q is a directory", trimmed)
		}
		name := strings.ToLower(filepath.Base(trimmed))
		if strings.Contains(name, omniVoiceTokenizerNameSnippet) {
			tokenizerPath = trimmed
			continue
		}
		if modelPath == "" {
			modelPath = trimmed
		}
	}
	if modelPath == "" {
		return "", "", fmt.Errorf("local OMNIVOICE model asset is required")
	}
	if tokenizerPath == "" {
		return "", "", fmt.Errorf("local OMNIVOICE tokenizer asset is required")
	}
	return modelPath, tokenizerPath, nil
}

func omniVoiceCommandForWorker(worker *interfaces.WorkerConfig) (string, []string) {
	if worker == nil {
		return defaultOmniVoiceCommand, nil
	}
	command := strings.TrimSpace(worker.Command)
	if command == "" {
		command = defaultOmniVoiceCommand
	}
	return command, append([]string(nil), worker.Args...)
}

func omniVoiceBoundText(bindings []interfaces.ResolvedModelOperationBinding) (string, error) {
	for _, binding := range bindings {
		if !strings.EqualFold(strings.TrimSpace(binding.Slot), omniVoiceModelSlotNameText) {
			continue
		}
		var parts []string
		for _, content := range binding.Content {
			if content.Type.Normalized() != interfaces.WorkContentPartTypeText {
				continue
			}
			text := strings.TrimSpace(content.Text)
			if text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) == 0 {
			return "", fmt.Errorf("local OMNIVOICE runtime requires TEXT content for slot %q", omniVoiceModelSlotNameText)
		}
		return strings.Join(parts, "\n"), nil
	}
	return "", fmt.Errorf("local OMNIVOICE runtime requires resolved slot %q", omniVoiceModelSlotNameText)
}

func omniVoiceOutputPath(cachePath string) (string, error) {
	dir := strings.TrimSpace(cachePath)
	if dir == "" {
		dir = os.TempDir()
	}
	file, err := os.CreateTemp(dir, "omnivoice-*.wav")
	if err != nil {
		return "", fmt.Errorf("create local OMNIVOICE output file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close local OMNIVOICE output file: %w", err)
	}
	return file.Name(), nil
}

func omniVoiceCommandRequest(request interfaces.RunnerExecutionRequest, handle *omniVoiceLocalHandle, outputFile string, stdin []byte) workers.CommandRequest {
	dispatch := interfaces.CloneWorkDispatch(request.Dispatch)
	args := append([]string(nil), handle.baseArgs...)
	args = append(args,
		omniVoiceInvokeSubcommand,
		"--model", handle.modelPath,
		"--tokenizer", handle.tokenizerPath,
		"--output", outputFile,
	)
	return workers.CommandRequest{
		Command:                  handle.command,
		Args:                     args,
		Stdin:                    stdin,
		DispatchID:               dispatch.DispatchID,
		TransitionID:             dispatch.TransitionID,
		WorkerType:               dispatch.WorkerType,
		WorkstationName:          dispatch.WorkstationName,
		ProjectID:                dispatch.ProjectID,
		CurrentChainingTraceID:   dispatch.CurrentChainingTraceID,
		PreviousChainingTraceIDs: dispatch.PreviousChainingTraceIDs,
		Execution:                dispatch.Execution,
		InputTokens:              dispatch.InputTokens,
		InputBindings:            dispatch.InputBindings,
		WorkDir:                  strings.TrimSpace(request.WorkingDirectory),
	}
}

func omniVoiceCombinedOutput(result workers.CommandResult) string {
	parts := make([]string, 0, 2)
	if stdout := strings.TrimSpace(string(result.Stdout)); stdout != "" {
		parts = append(parts, "stdout: "+stdout)
	}
	if stderr := strings.TrimSpace(string(result.Stderr)); stderr != "" {
		parts = append(parts, "stderr: "+stderr)
	}
	if len(parts) == 0 {
		return "no command output"
	}
	return strings.Join(parts, "; ")
}

func omniVoiceResponseContent(stdout string, outputFile string) ([]interfaces.WorkContentPart, error) {
	content := make([]interfaces.WorkContentPart, 0, 1)
	if stdout != "" {
		var direct []interfaces.WorkContentPart
		if err := json.Unmarshal([]byte(stdout), &direct); err == nil {
			content = direct
		} else {
			var envelope struct {
				Content []interfaces.WorkContentPart `json:"content"`
			}
			if envelopeErr := json.Unmarshal([]byte(stdout), &envelope); envelopeErr != nil {
				return nil, fmt.Errorf("decode local OMNIVOICE response: %w", err)
			}
			content = envelope.Content
		}
	}
	if len(content) == 0 {
		content = []interfaces.WorkContentPart{{
			Type:        interfaces.WorkContentPartTypeAudio,
			Slot:        omniVoiceModelSlotNameAudio,
			File:        outputFile,
			ContentType: omniVoiceAudioContentType,
		}}
	}
	audioFound := false
	for i := range content {
		if content[i].Type.Normalized() != interfaces.WorkContentPartTypeAudio {
			continue
		}
		audioFound = true
		if strings.TrimSpace(content[i].File) == "" {
			content[i].File = outputFile
		}
		if strings.TrimSpace(content[i].ContentType) == "" {
			content[i].ContentType = omniVoiceAudioContentType
		}
		if strings.TrimSpace(content[i].Slot) == "" {
			content[i].Slot = omniVoiceModelSlotNameAudio
		}
	}
	if !audioFound {
		content = append(content, interfaces.WorkContentPart{
			Type:        interfaces.WorkContentPartTypeAudio,
			Slot:        omniVoiceModelSlotNameAudio,
			File:        outputFile,
			ContentType: omniVoiceAudioContentType,
		})
	}
	if _, err := os.Stat(outputFile); err != nil {
		return nil, fmt.Errorf("inspect local OMNIVOICE output file %q: %w", outputFile, err)
	}
	return content, nil
}

type localModelResourceReservation struct {
	key      string
	count    int
	capacity int
}

type localModelResourceLimiter struct {
	mu      sync.Mutex
	entries map[string]*localModelResourceLimiterEntry
}

type localModelResourceLimiterEntry struct {
	mu       sync.Mutex
	cond     *sync.Cond
	capacity int
	inUse    int
}

type localModelLimitedRunner struct {
	inner        workers.Runner
	limiter      *localModelResourceLimiter
	reservations []localModelResourceReservation
}

func newLocalModelResourceLimiter() *localModelResourceLimiter {
	return &localModelResourceLimiter{
		entries: make(map[string]*localModelResourceLimiterEntry),
	}
}

func newLocalModelResourceLimiterEntry(capacity int) *localModelResourceLimiterEntry {
	entry := &localModelResourceLimiterEntry{capacity: capacity}
	entry.cond = sync.NewCond(&entry.mu)
	return entry
}

func (l *localModelResourceLimiter) wrapRunner(
	inner workers.Runner,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
) workers.Runner {
	if inner == nil || l == nil || factoryCfg == nil || workerDef == nil {
		return inner
	}
	reservations := localModelResourceReservations(factoryCfg, workerDef)
	if len(reservations) == 0 {
		return inner
	}
	return &localModelLimitedRunner{
		inner:        inner,
		limiter:      l,
		reservations: reservations,
	}
}

func localModelResourceReservations(factoryCfg *interfaces.FactoryConfig, workerDef *interfaces.WorkerConfig) []localModelResourceReservation {
	if factoryCfg == nil || workerDef == nil || workerDef.ModelLocality != interfaces.ModelLocalityLocal {
		return nil
	}

	resourcesByName := make(map[string]interfaces.ResourceConfig, len(factoryCfg.Resources))
	for _, resource := range factoryCfg.Resources {
		resourcesByName[resource.Name] = resource
	}

	combined := make(map[string]localModelResourceReservation)
	order := make([]string, 0, len(workerDef.Resources))
	for _, requirement := range workerDef.Resources {
		resource, ok := resourcesByName[requirement.Name]
		if !ok || !isProcessScopedLocalModelResource(resource) || requirement.Capacity <= 0 {
			continue
		}
		key := localModelResourceKey(resource)
		if key == "" {
			continue
		}
		if existing, ok := combined[key]; ok {
			existing.count += requirement.Capacity
			combined[key] = existing
			continue
		}
		combined[key] = localModelResourceReservation{
			key:      key,
			count:    requirement.Capacity,
			capacity: resource.Capacity,
		}
		order = append(order, key)
	}

	if len(order) == 0 {
		return nil
	}
	out := make([]localModelResourceReservation, 0, len(order))
	for _, key := range order {
		out = append(out, combined[key])
	}
	return out
}

func isProcessScopedLocalModelResource(resource interfaces.ResourceConfig) bool {
	return resource.Type == interfaces.ResourceTypeModel &&
		strings.TrimSpace(resource.Model) != "" &&
		strings.TrimSpace(resource.Backend) != "" &&
		strings.TrimSpace(resource.LoadPolicy) != ""
}

func localModelResourceKey(resource interfaces.ResourceConfig) string {
	model := strings.ToUpper(strings.TrimSpace(resource.Model))
	backend := strings.ToUpper(strings.TrimSpace(resource.Backend))
	loadPolicy := strings.ToUpper(strings.TrimSpace(resource.LoadPolicy))
	if model == "" || backend == "" || loadPolicy == "" {
		return ""
	}
	return strings.Join([]string{model, backend, loadPolicy}, "|")
}

func (l *localModelResourceLimiter) acquire(ctx context.Context, reservations []localModelResourceReservation) error {
	if l == nil || len(reservations) == 0 {
		return nil
	}

	waitStartedAt := time.Now()
	markModelExecutionResourceWaitStarted(ctx, waitStartedAt)
	acquired := make([]localModelResourceReservation, 0, len(reservations))
	for _, reservation := range reservations {
		entry := l.entry(reservation.key, reservation.capacity)
		if err := entry.acquire(ctx, reservation.count); err != nil {
			markModelExecutionResourceWaitFinished(ctx, time.Now(), false)
			l.release(acquired)
			return err
		}
		acquired = append(acquired, reservation)
	}
	markModelExecutionResourceWaitFinished(ctx, time.Now(), true)
	return nil
}

func (l *localModelResourceLimiter) release(reservations []localModelResourceReservation) {
	if l == nil || len(reservations) == 0 {
		return
	}
	for i := len(reservations) - 1; i >= 0; i-- {
		reservation := reservations[i]
		entry := l.entry(reservation.key, reservation.capacity)
		entry.release(reservation.count)
	}
}

func (l *localModelResourceLimiter) entry(key string, capacity int) *localModelResourceLimiterEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if !ok {
		entry = newLocalModelResourceLimiterEntry(capacity)
		l.entries[key] = entry
		return entry
	}

	entry.mu.Lock()
	if capacity > 0 && (entry.capacity == 0 || capacity < entry.capacity) {
		entry.capacity = capacity
		entry.cond.Broadcast()
	}
	entry.mu.Unlock()
	return entry
}

func (e *localModelResourceLimiterEntry) acquire(ctx context.Context, count int) error {
	if e == nil || count <= 0 {
		return nil
	}

	stopBroadcast := context.AfterFunc(ctx, func() {
		e.mu.Lock()
		e.cond.Broadcast()
		e.mu.Unlock()
	})
	defer stopBroadcast()

	e.mu.Lock()
	defer e.mu.Unlock()

	for e.inUse+count > e.capacity {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("local model resource wait canceled: %w", err)
		}
		e.cond.Wait()
	}
	e.inUse += count
	return nil
}

func (e *localModelResourceLimiterEntry) release(count int) {
	if e == nil || count <= 0 {
		return
	}
	e.mu.Lock()
	e.inUse -= count
	if e.inUse < 0 {
		e.inUse = 0
	}
	e.mu.Unlock()
	e.cond.Broadcast()
}

func (r *localModelLimitedRunner) Execute(ctx context.Context, request interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error) {
	if err := r.limiter.acquire(ctx, r.reservations); err != nil {
		return interfaces.RunnerExecutionResult{}, err
	}
	defer r.limiter.release(r.reservations)
	return r.inner.Execute(ctx, request)
}
