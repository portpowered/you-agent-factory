package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type CacheLayout struct {
	ModelName string
	CachePath string
	Revision  string
	Files     []string
}

type LoadRequest struct {
	Resource        models.RuntimeResource
	Worker          *models.RuntimeWorker
	ModelName       string
	CachePath       string
	Revision        string
	Files           []string
	ServingEndpoint string
}

type InvocationRequest struct {
	Resource models.RuntimeResource
	Worker   *models.RuntimeWorker
	Request  ModelInvocation
}

type ModelInvocation struct {
	Dispatch         work.WorkDispatch
	ModelOperation   string
	ModelBindings    []modelinference.ResolvedModelOperationBinding
	WorkingDirectory string
}

type InvocationResponse struct {
	Content string
}

func cloneModelInvocation(request ModelInvocation) ModelInvocation {
	clone := request
	clone.Dispatch = work.CloneWorkDispatch(request.Dispatch)
	clone.ModelBindings = cloneModelBindings(request.ModelBindings)
	return clone
}

func cloneModelBindings(values []modelinference.ResolvedModelOperationBinding) []modelinference.ResolvedModelOperationBinding {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]modelinference.ResolvedModelOperationBinding, len(values))
	for index, value := range values {
		cloned[index] = value
		cloned[index].Content = work.CloneWorkContentParts(value.Content)
	}
	return cloned
}

type Handle interface {
	Invoke(context.Context, InvocationRequest) (InvocationResponse, error)
}

type Runtime interface {
	Supports(resource models.RuntimeResource, worker *models.RuntimeWorker) bool
	Load(context.Context, LoadRequest) (Handle, error)
}

type Hooks = modelseffects.LocalRuntimeHooks

type Manager struct {
	mu          sync.Mutex
	entries     map[string]*managedLocalModelEntry
	assetPuller AssetPuller
	runtime     Runtime
	hooks       Hooks
	now         func() time.Time
}

// ErrInvalidDependencies classifies managed local-runtime construction failures.
var ErrInvalidDependencies = errors.New("managed local runtime dependencies are invalid")

// NewManagedRuntime constructs managed local invocation behavior after
// synchronously validating its required collaborators. It does not resolve
// assets, load a runtime, or start lifecycle work during construction.
func NewManagedRuntime(assetPuller AssetPuller, runtime Runtime, hooks Hooks, now func() time.Time) (*Manager, error) {
	if isNilDependency(assetPuller) {
		return nil, missingDependencyError("asset puller and cache resolver")
	}
	if isNilDependency(runtime) {
		return nil, missingDependencyError("local invocation runtime")
	}
	if now == nil {
		return nil, missingDependencyError("clock")
	}
	return newManager(assetPuller, runtime, hooks, now), nil
}

type managedLocalModelEntry struct {
	mu     sync.Mutex
	handle Handle
}

func newManager(assetPuller AssetPuller, runtime Runtime, hooks Hooks, now func() time.Time) *Manager {
	return &Manager{
		entries:     make(map[string]*managedLocalModelEntry),
		assetPuller: assetPuller,
		runtime:     runtime,
		hooks:       hooks,
		now:         now,
	}
}

func missingDependencyError(name string) error {
	return fmt.Errorf("%w: %s is required", ErrInvalidDependencies, name)
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// Invoke resolves, loads, and invokes a managed local model. handled is false
// when the supplied worker is not owned by this runtime.
func (m *Manager) Invoke(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	factoryCfg *models.RuntimeConfig,
	workerDef *models.RuntimeWorker,
	request ModelInvocation,
) (InvocationResponse, bool, error) {
	resource, resourceKey, ok := RuntimeResource(factoryCfg, workerDef)
	if !ok || !m.runtime.Supports(resource, workerDef) {
		return InvocationResponse{}, false, nil
	}
	loaded, err := runtimeCfgForLocalModel(runtimeCfg)
	if err != nil {
		return InvocationResponse{}, true, err
	}
	if _, err := EnsureManagedRuntimeReadyForInvocation(
		loaded, workerDef.Model, m.assetPuller, DefaultManagedRuntimeSourceResolver(),
	); err != nil {
		return InvocationResponse{}, true, err
	}
	cacheLayout, err := m.assetPuller.ResolveModelCache(ctx, loaded, workerDef)
	if err != nil {
		return InvocationResponse{}, true, err
	}
	loadWorker := workerDef.Clone()
	handle, err := m.loadHandle(ctx, resourceKey, LoadRequest{
		Resource:  resource,
		Worker:    &loadWorker,
		ModelName: cacheLayout.ModelName,
		CachePath: cacheLayout.CachePath,
		Revision:  cacheLayout.Revision,
		Files:     append([]string(nil), cacheLayout.Files...),
	})
	if err != nil {
		return InvocationResponse{}, true, err
	}
	invokeWorker := workerDef.Clone()
	response, err := handle.Invoke(ctx, InvocationRequest{
		Resource: resource,
		Worker:   &invokeWorker,
		Request:  cloneModelInvocation(request),
	})
	return response, true, err
}

func runtimeCfgForLocalModel(runtimeCfg *models.RuntimeConfig) (*models.RuntimeConfig, error) {
	if runtimeCfg == nil {
		return nil, fmt.Errorf("loaded runtime config is required for local model execution")
	}
	return runtimeCfg, nil
}

func (m *Manager) loadHandle(ctx context.Context, key string, request LoadRequest) (Handle, error) {
	entry := m.entry(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.handle != nil {
		if m.hooks.MarkLoadReused != nil {
			m.hooks.MarkLoadReused(ctx)
		}
		return entry.handle, nil
	}
	if m.hooks.MarkLoadRequested != nil {
		m.hooks.MarkLoadRequested(ctx, m.now())
	}
	handle, err := m.runtime.Load(ctx, request)
	if err != nil {
		if m.hooks.MarkLoadFinished != nil {
			m.hooks.MarkLoadFinished(ctx, m.now())
		}
		return nil, err
	}
	if m.hooks.MarkLoadFinished != nil {
		m.hooks.MarkLoadFinished(ctx, m.now())
	}
	entry.handle = handle
	return handle, nil
}

func (m *Manager) entry(key string) *managedLocalModelEntry {
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

func RuntimeResource(factoryCfg *models.RuntimeConfig, workerDef *models.RuntimeWorker) (models.RuntimeResource, string, bool) {
	if factoryCfg == nil || workerDef == nil || workerDef.ModelLocality != models.RuntimeModelLocalityLocal {
		return models.RuntimeResource{}, "", false
	}
	if len(workerDef.Resources) == 0 {
		return models.RuntimeResource{}, "", false
	}
	resourcesByName := make(map[string]models.RuntimeResource, len(factoryCfg.Resources))
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
	return models.RuntimeResource{}, "", false
}

func CanonicalBackendName(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

const (
	DefaultOmniVoiceCommand       = "omnivoice-llamacpp"
	omniVoiceInvokeSubcommand     = "invoke"
	OmniVoiceAudioContentType     = "audio/wav"
	omniVoiceModelSlotNameText    = "text"
	omniVoiceModelSlotNameAudio   = "audio"
	omniVoiceTokenizerNameSnippet = "tokenizer"
	supervisedInvokePath          = "/invoke"
)

type omniVoiceLocalRuntime struct {
	runner         platformprocess.CommandRunner
	client         HTTPDoer
	inspectFile    InspectFile
	tempDirectory  TempDirectory
	createTempFile CreateTempFile
}

// HTTPDoer is the exact external HTTP effect used to invoke a supervised local
// model runtime.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// TempFile is a local-runtime temporary output handle.
type TempFile interface {
	Close() error
	Name() string
}

type InspectFile func(string) (os.FileInfo, error)
type TempDirectory func() string
type CreateTempFile func(string, string) (TempFile, error)

type omniVoiceLocalHandle struct {
	runner        platformprocess.CommandRunner
	command       string
	baseArgs      []string
	modelName     string
	cachePath     string
	revision      string
	modelPath     string
	tokenizerPath string
	inspectFile   InspectFile
	tempDirectory TempDirectory
	createTemp    CreateTempFile
}

type OmniVoiceInvocationPayload struct {
	Operation  string                                         `json:"operation"`
	ModelName  string                                         `json:"modelName"`
	Revision   string                                         `json:"revision,omitempty"`
	OutputFile string                                         `json:"outputFile"`
	Text       string                                         `json:"text"`
	Bindings   []modelinference.ResolvedModelOperationBinding `json:"bindings,omitempty"`
}

// NewOmniVoiceRuntime constructs the local runtime with the exact
// process and HTTP effects supplied by the composition root.
func NewOmniVoiceRuntime(
	runner platformprocess.CommandRunner,
	client HTTPDoer,
	inspectFile InspectFile,
	tempDirectory TempDirectory,
	createTempFile CreateTempFile,
) (Runtime, error) {
	if runner == nil {
		return nil, missingDependencyError("local runtime command runner")
	}
	if client == nil {
		return nil, missingDependencyError("local runtime HTTP client")
	}
	if inspectFile == nil {
		return nil, missingDependencyError("local runtime file inspector")
	}
	if tempDirectory == nil {
		return nil, missingDependencyError("local runtime temporary directory resolver")
	}
	if createTempFile == nil {
		return nil, missingDependencyError("local runtime temporary file creator")
	}
	return &omniVoiceLocalRuntime{
		runner: runner, client: client, inspectFile: inspectFile,
		tempDirectory: tempDirectory, createTempFile: createTempFile,
	}, nil
}

func (r *omniVoiceLocalRuntime) Supports(resource models.RuntimeResource, worker *models.RuntimeWorker) bool {
	if worker == nil {
		return false
	}
	return strings.TrimSpace(worker.ModelLocality) == models.RuntimeModelLocalityLocal &&
		CanonicalBackendName(resource.Backend) == "LLAMACPP" &&
		canonicalModelName(worker.Model) == canonicalModelName("OMNIVOICE_Q4_K_M")
}

func (r *omniVoiceLocalRuntime) Load(_ context.Context, request LoadRequest) (Handle, error) {
	if !r.Supports(request.Resource, request.Worker) {
		return nil, fmt.Errorf("unsupported local model runtime for model %q with backend %q", request.ModelName, request.Resource.Backend)
	}
	modelPath, tokenizerPath, err := omniVoiceCacheFiles(r.inspectFile, request.Files)
	if err != nil {
		return nil, err
	}
	if endpoint := strings.TrimSpace(request.ServingEndpoint); endpoint != "" {
		return &omniVoiceSupervisedHandle{
			client:          r.client,
			servingEndpoint: endpoint,
			modelName:       request.ModelName,
			cachePath:       request.CachePath,
			revision:        request.Revision,
			modelPath:       modelPath,
			tokenizerPath:   tokenizerPath,
			inspectFile:     r.inspectFile,
			tempDirectory:   r.tempDirectory,
			createTemp:      r.createTempFile,
		}, nil
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
		inspectFile:   r.inspectFile,
		tempDirectory: r.tempDirectory,
		createTemp:    r.createTempFile,
	}, nil
}

func (h *omniVoiceLocalHandle) Invoke(ctx context.Context, request InvocationRequest) (InvocationResponse, error) {
	if h == nil {
		return InvocationResponse{}, fmt.Errorf("local model handle is required")
	}
	operation := strings.TrimSpace(request.Request.ModelOperation)
	if operation != "TTS" {
		return InvocationResponse{}, fmt.Errorf("local OMNIVOICE runtime only supports TTS, got %q", operation)
	}
	text, err := omniVoiceBoundText(request.Request.ModelBindings)
	if err != nil {
		return InvocationResponse{}, err
	}
	outputFile, err := omniVoiceOutputPath(h.tempDirectory, h.createTemp, h.cachePath)
	if err != nil {
		return InvocationResponse{}, err
	}

	payload := OmniVoiceInvocationPayload{
		Operation:  operation,
		ModelName:  h.modelName,
		Revision:   h.revision,
		OutputFile: outputFile,
		Text:       text,
		Bindings:   cloneModelBindings(request.Request.ModelBindings),
	}
	stdin, err := json.Marshal(payload)
	if err != nil {
		return InvocationResponse{}, fmt.Errorf("encode local OMNIVOICE invocation payload: %w", err)
	}

	result, err := h.runner.Run(ctx, omniVoiceCommandRequest(request.Request, h, outputFile, stdin))
	if err != nil {
		return InvocationResponse{}, fmt.Errorf("run local OMNIVOICE runtime: %w", err)
	}
	if result.ExitCode != 0 {
		return InvocationResponse{}, fmt.Errorf("local OMNIVOICE runtime exited with code %d: %s", result.ExitCode, omniVoiceCombinedOutput(result))
	}

	content, err := omniVoiceResponseContent(h.inspectFile, strings.TrimSpace(string(result.Stdout)), outputFile)
	if err != nil {
		return InvocationResponse{}, err
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return InvocationResponse{}, fmt.Errorf("encode local OMNIVOICE response content: %w", err)
	}
	return InvocationResponse{Content: string(encoded)}, nil
}

type omniVoiceSupervisedHandle struct {
	client          HTTPDoer
	servingEndpoint string
	modelName       string
	cachePath       string
	revision        string
	modelPath       string
	tokenizerPath   string
	inspectFile     InspectFile
	tempDirectory   TempDirectory
	createTemp      CreateTempFile
}

func (h *omniVoiceSupervisedHandle) Invoke(ctx context.Context, request InvocationRequest) (InvocationResponse, error) {
	if h == nil {
		return InvocationResponse{}, fmt.Errorf("supervised local model handle is required")
	}
	operation := strings.TrimSpace(request.Request.ModelOperation)
	if operation != "TTS" {
		return InvocationResponse{}, fmt.Errorf("local OMNIVOICE runtime only supports TTS, got %q", operation)
	}
	text, err := omniVoiceBoundText(request.Request.ModelBindings)
	if err != nil {
		return InvocationResponse{}, err
	}
	outputFile, err := omniVoiceOutputPath(h.tempDirectory, h.createTemp, h.cachePath)
	if err != nil {
		return InvocationResponse{}, err
	}

	payload := OmniVoiceInvocationPayload{
		Operation:  operation,
		ModelName:  h.modelName,
		Revision:   h.revision,
		OutputFile: outputFile,
		Text:       text,
		Bindings:   cloneModelBindings(request.Request.ModelBindings),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return InvocationResponse{}, fmt.Errorf("encode local OMNIVOICE invocation payload: %w", err)
	}

	invokeURL := SupervisedInvokeURL(h.servingEndpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, invokeURL, strings.NewReader(string(body)))
	if err != nil {
		return InvocationResponse{}, fmt.Errorf("build supervised OMNIVOICE invocation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return InvocationResponse{}, fmt.Errorf("invoke supervised OMNIVOICE runtime at %q: %w", invokeURL, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return InvocationResponse{}, fmt.Errorf("supervised OMNIVOICE runtime at %q returned status %d", invokeURL, resp.StatusCode)
	}
	stdout, err := io.ReadAll(resp.Body)
	if err != nil {
		return InvocationResponse{}, fmt.Errorf("read supervised OMNIVOICE response: %w", err)
	}
	content, err := omniVoiceResponseContent(h.inspectFile, strings.TrimSpace(string(stdout)), outputFile)
	if err != nil {
		return InvocationResponse{}, err
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return InvocationResponse{}, fmt.Errorf("encode local OMNIVOICE response content: %w", err)
	}
	return InvocationResponse{Content: string(encoded)}, nil
}

// SupervisedInvokeURL derives the supervised model-server invoke URL from a health or serving endpoint.
func SupervisedInvokeURL(servingEndpoint string) string {
	trimmed := strings.TrimSpace(servingEndpoint)
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(strings.TrimRight(trimmed, "/"), supervisedInvokePath) {
		return trimmed
	}
	if idx := strings.LastIndex(trimmed, "/health"); idx >= 0 {
		return trimmed[:idx] + supervisedInvokePath
	}
	return strings.TrimRight(trimmed, "/") + supervisedInvokePath
}

func omniVoiceCacheFiles(inspectFile InspectFile, files []string) (string, string, error) {
	var modelPath string
	var tokenizerPath string
	for _, file := range files {
		trimmed := strings.TrimSpace(file)
		if trimmed == "" {
			continue
		}
		info, err := inspectFile(trimmed)
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

func omniVoiceCommandForWorker(worker *models.RuntimeWorker) (string, []string) {
	if worker == nil {
		return DefaultOmniVoiceCommand, nil
	}
	command := strings.TrimSpace(worker.Command)
	if command == "" {
		command = DefaultOmniVoiceCommand
	}
	return command, append([]string(nil), worker.Args...)
}

func omniVoiceBoundText(bindings []modelinference.ResolvedModelOperationBinding) (string, error) {
	for _, binding := range bindings {
		if !strings.EqualFold(strings.TrimSpace(binding.Slot), omniVoiceModelSlotNameText) {
			continue
		}
		var parts []string
		for _, content := range binding.Content {
			if content.Type.Normalized() != work.WorkContentPartTypeText {
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

func omniVoiceOutputPath(tempDirectory TempDirectory, createTemp CreateTempFile, cachePath string) (string, error) {
	dir := strings.TrimSpace(cachePath)
	if dir == "" {
		dir = tempDirectory()
	}
	file, err := createTemp(dir, "omnivoice-*.wav")
	if err != nil {
		return "", fmt.Errorf("create local OMNIVOICE output file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close local OMNIVOICE output file: %w", err)
	}
	return file.Name(), nil
}

func omniVoiceCommandRequest(request ModelInvocation, handle *omniVoiceLocalHandle, outputFile string, stdin []byte) platformprocess.CommandRequest {
	args := append([]string(nil), handle.baseArgs...)
	args = append(args,
		omniVoiceInvokeSubcommand,
		"--model", handle.modelPath,
		"--tokenizer", handle.tokenizerPath,
		"--output", outputFile,
	)
	return platformprocess.CommandRequest{
		Command: handle.command,
		Args:    args,
		Stdin:   stdin,
		WorkDir: strings.TrimSpace(request.WorkingDirectory),
	}
}

func omniVoiceCombinedOutput(result platformprocess.CommandResult) string {
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

func omniVoiceResponseContent(inspectFile InspectFile, stdout string, outputFile string) ([]work.WorkContentPart, error) {
	content := make([]work.WorkContentPart, 0, 1)
	if stdout != "" {
		var direct []work.WorkContentPart
		if err := json.Unmarshal([]byte(stdout), &direct); err == nil {
			content = direct
		} else {
			var envelope struct {
				Content []work.WorkContentPart `json:"content"`
			}
			if envelopeErr := json.Unmarshal([]byte(stdout), &envelope); envelopeErr != nil {
				return nil, fmt.Errorf("decode local OMNIVOICE response: %w", err)
			}
			content = envelope.Content
		}
	}
	if len(content) == 0 {
		content = []work.WorkContentPart{{
			Type:        work.WorkContentPartTypeAudio,
			Slot:        omniVoiceModelSlotNameAudio,
			File:        outputFile,
			ContentType: OmniVoiceAudioContentType,
		}}
	}
	audioFound := false
	for i := range content {
		if content[i].Type.Normalized() != work.WorkContentPartTypeAudio {
			continue
		}
		audioFound = true
		if strings.TrimSpace(content[i].File) == "" {
			content[i].File = outputFile
		}
		if strings.TrimSpace(content[i].ContentType) == "" {
			content[i].ContentType = OmniVoiceAudioContentType
		}
		if strings.TrimSpace(content[i].Slot) == "" {
			content[i].Slot = omniVoiceModelSlotNameAudio
		}
	}
	if !audioFound {
		content = append(content, work.WorkContentPart{
			Type:        work.WorkContentPartTypeAudio,
			Slot:        omniVoiceModelSlotNameAudio,
			File:        outputFile,
			ContentType: OmniVoiceAudioContentType,
		})
	}
	if _, err := inspectFile(outputFile); err != nil {
		return nil, fmt.Errorf("inspect local OMNIVOICE output file %q: %w", outputFile, err)
	}
	return content, nil
}

type localModelResourceReservation struct {
	key      string
	count    int
	capacity int
}

type ResourceLimiter struct {
	mu      sync.Mutex
	entries map[string]*ResourceLimiterEntry
	hooks   Hooks
	now     func() time.Time
}

type ResourceLimiterEntry struct {
	mu       sync.Mutex
	cond     *sync.Cond
	capacity int
	inUse    int
}

func NewResourceLimiter(hooks Hooks, now func() time.Time) (*ResourceLimiter, error) {
	if now == nil {
		return nil, missingDependencyError("resource limiter clock")
	}
	return &ResourceLimiter{
		entries: make(map[string]*ResourceLimiterEntry),
		hooks:   hooks,
		now:     now,
	}, nil
}

func newLocalModelResourceLimiterEntry(capacity int) *ResourceLimiterEntry {
	entry := &ResourceLimiterEntry{capacity: capacity}
	entry.cond = sync.NewCond(&entry.mu)
	return entry
}

// Acquire reserves local model capacity for a worker and returns an idempotent
// release function. A nil release means the worker has no local reservations.
func (l *ResourceLimiter) Acquire(
	ctx context.Context,
	factoryCfg *models.RuntimeConfig,
	workerDef *models.RuntimeWorker,
) (func(), error) {
	if l == nil || factoryCfg == nil || workerDef == nil {
		return nil, nil
	}
	reservations := localModelResourceReservations(factoryCfg, workerDef)
	if len(reservations) == 0 {
		return nil, nil
	}
	if err := l.acquire(ctx, reservations); err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() { l.release(reservations) })
	}, nil
}

func localModelResourceReservations(factoryCfg *models.RuntimeConfig, workerDef *models.RuntimeWorker) []localModelResourceReservation {
	if factoryCfg == nil || workerDef == nil || workerDef.ModelLocality != models.RuntimeModelLocalityLocal {
		return nil
	}

	resourcesByName := make(map[string]models.RuntimeResource, len(factoryCfg.Resources))
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

func isProcessScopedLocalModelResource(resource models.RuntimeResource) bool {
	return resource.Type == models.RuntimeResourceTypeModel &&
		strings.TrimSpace(resource.Model) != "" &&
		strings.TrimSpace(resource.Backend) != "" &&
		strings.TrimSpace(resource.LoadPolicy) != ""
}

func localModelResourceKey(resource models.RuntimeResource) string {
	model := strings.ToUpper(strings.TrimSpace(resource.Model))
	backend := strings.ToUpper(strings.TrimSpace(resource.Backend))
	loadPolicy := strings.ToUpper(strings.TrimSpace(resource.LoadPolicy))
	if model == "" || backend == "" || loadPolicy == "" {
		return ""
	}
	return strings.Join([]string{model, backend, loadPolicy}, "|")
}

func (l *ResourceLimiter) acquire(ctx context.Context, reservations []localModelResourceReservation) error {
	if l == nil || len(reservations) == 0 {
		return nil
	}

	waitStartedAt := l.now()
	if l.hooks.MarkResourceWaitStarted != nil {
		l.hooks.MarkResourceWaitStarted(ctx, waitStartedAt)
	}
	acquired := make([]localModelResourceReservation, 0, len(reservations))
	for _, reservation := range reservations {
		entry := l.entry(reservation.key, reservation.capacity)
		if err := entry.acquire(ctx, reservation.count); err != nil {
			if l.hooks.MarkResourceWaitFinished != nil {
				l.hooks.MarkResourceWaitFinished(ctx, l.now(), false)
			}
			l.release(acquired)
			return err
		}
		acquired = append(acquired, reservation)
	}
	if l.hooks.MarkResourceWaitFinished != nil {
		l.hooks.MarkResourceWaitFinished(ctx, l.now(), true)
	}
	return nil
}

func (l *ResourceLimiter) release(reservations []localModelResourceReservation) {
	if l == nil || len(reservations) == 0 {
		return
	}
	for i := len(reservations) - 1; i >= 0; i-- {
		reservation := reservations[i]
		entry := l.entry(reservation.key, reservation.capacity)
		entry.release(reservation.count)
	}
}

func (l *ResourceLimiter) entry(key string, capacity int) *ResourceLimiterEntry {
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

func (e *ResourceLimiterEntry) acquire(ctx context.Context, count int) error {
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

func (e *ResourceLimiterEntry) release(count int) {
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
