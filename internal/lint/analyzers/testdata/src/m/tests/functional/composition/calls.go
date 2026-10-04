package composition
func AssertReplaySucceeds() {}
func BuildRuntimeScope() {}
func GetEngineStateSnapshot() {}
func NewProviderErrorSmokeHarness() {}
func NewReplayHarness() {}
func NewRuntimeBuilder() {}
func NewRuntimeBuilderWithModelsFactory() {}
func NewRuntimeFactory() {}
func NewServiceTestHarness() {}
func SetCustomExecutor() {}
func StartFunctionalServerWithConfig() {}
func WithRuntimeScheduler() {}
func WithWorkerExecutor() {}
func startFunctionalServerWithConfig() {}
func startReplayFunctionalServerWithConfig() {}
func calls() {
	AssertReplaySucceeds() // want `functional scenarios must use the customer process boundary`
	BuildRuntimeScope() // want `functional scenarios must use the customer process boundary`
	GetEngineStateSnapshot() // want `functional scenarios must use the customer process boundary`
	NewProviderErrorSmokeHarness() // want `functional scenarios must use the customer process boundary`
	NewReplayHarness() // want `functional scenarios must use the customer process boundary`
	NewRuntimeBuilder() // want `functional scenarios must use the customer process boundary`
	NewRuntimeBuilderWithModelsFactory() // want `functional scenarios must use the customer process boundary`
	NewRuntimeFactory() // want `functional scenarios must use the customer process boundary`
	NewServiceTestHarness() // want `functional scenarios must use the customer process boundary`
	SetCustomExecutor() // want `functional scenarios must use the customer process boundary`
	StartFunctionalServerWithConfig() // want `functional scenarios must use the customer process boundary`
	WithRuntimeScheduler() // want `functional scenarios must use the customer process boundary`
	WithWorkerExecutor() // want `functional scenarios must use the customer process boundary`
	startFunctionalServerWithConfig() // want `functional scenarios must use the customer process boundary`
	startReplayFunctionalServerWithConfig() // want `functional scenarios must use the customer process boundary`
}
func Allowed() {}
