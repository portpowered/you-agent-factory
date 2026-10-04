package analyzers

var transportProcessAndFilesystemSymbols = map[string]struct{}{
	"fmt.Print": {}, "fmt.Printf": {}, "fmt.Println": {},
	"net.Listen": {}, "net.ListenPacket": {},
	"net/http.Client": {}, "net/http.DefaultClient": {}, "net/http.DefaultTransport": {},
	"os.Args": {}, "os.Chdir": {}, "os.Create": {}, "os.CreateTemp": {}, "os.DirFS": {}, "os.File": {},
	"os.Environ": {}, "os.Exit": {}, "os.Getenv": {}, "os.Getwd": {}, "os.LookupEnv": {},
	"os.Mkdir": {}, "os.MkdirAll": {},
	"os.Open": {}, "os.OpenFile": {}, "os.ReadDir": {}, "os.ReadFile": {}, "os.Remove": {},
	"os.RemoveAll": {}, "os.Rename": {}, "os.Stat": {}, "os.Stderr": {}, "os.Stdin": {},
	"os.Stdout": {}, "os.TempDir": {}, "os.UserCacheDir": {}, "os.UserConfigDir": {}, "os.UserHomeDir": {},
	"os.WriteFile":    {},
	"os/exec.Command": {}, "os/exec.CommandContext": {},
	"time.Now": {}, "time.Since": {}, "time.Until": {},
	"github.com/google/uuid.New": {}, "github.com/google/uuid.NewString": {},
}

var transportLifecycleSymbols = map[string]struct{}{
	"context.Background": {},
	"context.WithCancel": {}, "context.WithCancelCause": {}, "context.WithDeadline": {},
	"context.WithDeadlineCause": {}, "context.WithTimeout": {}, "context.WithTimeoutCause": {},
	"net/http.Server":  {},
	"os/signal.Ignore": {}, "os/signal.Notify": {}, "os/signal.NotifyContext": {},
	"os/signal.Reset": {}, "os/signal.Stop": {},
	"time.After": {}, "time.AfterFunc": {}, "time.NewTicker": {}, "time.NewTimer": {},
	"time.Sleep": {}, "time.Tick": {},
}

var transportSynchronizationTypes = map[string]struct{}{
	"sync.Cond": {}, "sync.Map": {}, "sync.Mutex": {}, "sync.Once": {}, "sync.Pool": {},
	"sync.RWMutex": {}, "sync.WaitGroup": {},
}

var transportDomainPolicyPrefixes = []string{
	"Materialize", "Orchestrate", "Project", "Replay", "Schedule", "Validate",
}

var transportFactoryDefinitionValidationPolicySymbols = map[string]struct{}{
	"ExpectedWorkerBehaviorClassForWorkstation":    {},
	"CompatibleWorkerWorkstationBehavior":          {},
	"WorkerWorkstationBehaviorMismatchMessage":     {},
	"workerWorkstationCompatibilityTargetsFromAPI": {},
}

var transportFactoryDefinitionValidationPhaseMethods = map[string]struct{}{
	"ValidateBlockingLoad":                   {},
	"WorkerWorkstationBehaviorCompatibility": {},
	"WorkTypeHandlingBehavior":               {},
}

var retiredTransportServiceEntrypoints = map[string]struct{}{
	"BuildWorkflowSessionLiveResult":           {},
	"BuildWorkflowSessionResult":               {},
	"BuildWorkflowSessionResultUpdatedPayload": {},
	"LoadFactoryConfigFromConfigFile":          {},
	"ResolveFactoryRootFromConfigFile":         {},
	"ValidateFactoryForPromptRun":              {},
}

var transportFactorySessionNormalizationSymbols = map[string]struct{}{
	"NormalizeStartRequest":             {},
	"NormalizeControlRequest":           {},
	"NormalizeApproveRequest":           {},
	"NormalizeRetryDispatchRequest":     {},
	"NormalizeInterruptDispatchRequest": {},
	"NormalizeListSessionsRequest":      {},
	"NormalizeResultRequest":            {},
	"NormalizeEventReconnectRequest":    {},
	"NewExecutionValidationError":       {},
}

var transportFactorySessionPreparationMethods = map[string]struct{}{
	"PrepareStart":             {},
	"PrepareControl":           {},
	"PrepareApprove":           {},
	"PrepareRetryDispatch":     {},
	"PrepareInterruptDispatch": {},
	"PrepareListSessions":      {},
	"PrepareResult":            {},
	"PrepareEventReconnect":    {},
}

var transportWorkPreparationSymbols = map[string]struct{}{
	"NormalizeList":                            {},
	"ParseCanonicalWorkRequestJSON":            {},
	"ValidateCanonicalWorkRequestJSON":         {},
	"NewListRequestPreparation":                {},
	"NewFactoryRequestBatchPreparation":        {},
	"ResolveTextInput":                         {},
	"ResolveAPITextInputContent":               {},
	"NormalizeArguments":                       {},
	"NamedArgumentInputsFromAnyMap":            {},
	"NewInvocationInputPreparation":            {},
	"NewContentPreparation":                    {},
	"ResolveWorkRequestCurrentChainingTraceID": {},
}

var httpFactoryStatusProjectionSymbols = map[string]struct{}{
	"IsSystemTimeToken":             {},
	"SplitPlaceID":                  {},
	"StateCategoryForPlace":         {},
	"categorizeStatusTokens":        {},
	"countPublicStatusTokens":       {},
	"resourceTotalsFromTopology":    {},
	"statusFromEngineStateSnapshot": {},
	"statusStateCategory":           {},
}

var transportWorkersConfigLoadingSymbols = map[string]struct{}{
	"LoadMockWorkersConfig":      {},
	"NewMockWorkersConfigLoader": {},
	"NewEmptyMockWorkersConfig":  {},
	"ParseMockWorkersConfig":     {},
}

var transportWorkersFailurePolicySymbols = map[string]struct{}{
	"CanonicalProviderSessionProvider":       {},
	"NewProviderError":                       {},
	"NormalizeProviderExecutionError":        {},
	"SafeWorkDiagnosticsFromWorkDiagnostics": {},
	"WorkDiagnostics":                        {},
}

var transportModelsReadinessPolicySymbols = map[string]struct{}{
	"ErrFailed":                 {},
	"ErrLoading":                {},
	"ErrMissing":                {},
	"ErrUnsupported":            {},
	"InvocationError":           {},
	"InvocationErrorForRuntime": {},
	"IsInvocationBlocked":       {},
	"IsMissing":                 {},
	"ReadinessStateFromError":   {},
}

var transportPlatformSelectionPrefixes = []string{
	"Build", "Create", "Default", "Ensure", "New", "Open", "Provide", "Resolve",
}

const factorySessionMCPTransportPrefix = "pkg/transports/mcp/factorysession/"

var transportBehaviorKinds = []string{
	"external-effect", "lifecycle", "concurrency", "platform-selection", "domain-policy",
	"opaque-import", "factory-session-preparation", "work-content-normalization", "factory-status-projection",
	"validation-orchestration", "source-default-selection", "work-request-normalization", "service-normalization",
	"named-factory-path-policy", "workers-config-loading", "workers-failure-policy", "models-readiness-policy",
	"work-request-preparation", "alternate-service-entrypoint", "alternate-test-entrypoint", "service-forwarder", "mutable-function-seam",
}
