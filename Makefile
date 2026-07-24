BINARY_NAME := you
CMD_PATH    := ./cmd/factory/
BIN_DIR     := bin
GO          ?= go
INSTALL_DIR := $(or $(GOBIN),$(shell $(GO) env GOPATH)/bin)
NPM         ?= npm
BUN_BIN     := $(shell command -v bun 2>/dev/null)
BUN_INSTALL := $(BUN_BIN) install --frozen-lockfile
BUN_PACKAGE_DIRS := ui/packages/components ui
UI_SCRIPT   := $(if $(BUN_BIN),$(BUN_BIN) run,$(NPM) run)
UI_EXEC     := $(if $(BUN_BIN),$(BUN_BIN) x,$(NPM) exec)
UI_INSTALL  := $(if $(BUN_BIN),$(BUN_BIN) install --frozen-lockfile,$(NPM) install --no-package-lock)
FUNCTIONAL_DEFAULT_PACKAGES := ./tests/functional/...
FUNCTIONAL_DEFAULT_JOBS ?= 2
UNIT_DEFAULT_JOBS ?= 32
FUNCTIONAL_LONG_TAGS ?= functionallong
FUNCTIONAL_LONG_PACKAGES := ./tests/functional/...
STRESS_DEFAULT_PACKAGES := ./tests/stress/...
RELEASE_DEFAULT_PACKAGES := ./tests/release/...
MODEL_LONG_TEST_TIMEOUT ?= 20m
PR_INFERENCE_APPROVAL_REGRESSION ?= TestRealLocalInference_OMNIVOICEModelInvokeAndDirectAPIProduceAudio
SCRIPT_TIMEOUT_COMPANION_SMOKE_TEST := TestIntegrationSmoke_ScriptTimeoutCompanionRequeuesBeforeLaterCompletion
SCRIPT_TIMEOUT_COMPANION_SMOKE_COUNT ?= 100
SCRIPT_TIMEOUT_COMPANION_SMOKE_TIMEOUT ?= 120s
CRON_TIME_WORK_SMOKE_TEST := TestCronWorkstations_ServiceModeSmoke_SubmitsInternalTimeWorkExpiresRetriesDispatchesAndFiltersViews
CRON_TIME_WORK_SMOKE_COUNT ?= 10
CRON_TIME_WORK_SMOKE_TIMEOUT ?= 120s
CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TEST := TestCurrentFactoryActivationFixture_ActivatesSecondPersistedFactoryAndResolvesCurrentFactory
CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_COUNT ?= 1
CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TIMEOUT ?= 120s
CROSS_PROVIDER_PARITY_SMOKE_TEST := TestCrossProviderParity
CROSS_PROVIDER_PARITY_SMOKE_TIMEOUT ?= 120s
JAVASCRIPT_CONTRACT_SMOKE_TIMEOUT ?= 120s
CONFIG_CONTRACT_SMOKE_TIMEOUT ?= 120s
JAVASCRIPT_RUNTIME_REGRESSION_TESTS ?= ^(TestCallBehavior_WorkflowFinalInventoryMatchesExecution|TestCallBehavior_AgentRunInventoryMatchesExecution|TestRun_ProgressPrimitives_EmitsOrderedRuntimeRecords|TestRun_PolicyDeniedChildOperations_ReturnStableDiagnostics|TestCallBehavior_WorkflowResumeStateInventoryMatchesExecution)$$
RESPONSE_STREAM_STRESS_SMOKE_TEST := TestSessionResponseEventStore_Backpressure
RESPONSE_STREAM_STRESS_SMOKE_TIMEOUT ?= 120s
BUILT_CLI_ACCEPTANCE_PACKAGES := ./tests/functional/acceptance
BUILT_CLI_ACCEPTANCE_TIMEOUT ?= 300s

ifeq ($(OS),Windows_NT)
	BINARY_NAME := you.exe
endif

# Detect git worktree environment
# In a worktree, --git-common-dir points to the main repo's .git directory,
# while --git-dir points to the worktree's .git file. When they differ, we're
# in a worktree and must force a full rebuild to avoid stale build cache.
_GIT_COMMON_DIR := $(shell git rev-parse --git-common-dir 2>/dev/null)
_GIT_DIR := $(shell git rev-parse --git-dir 2>/dev/null)
IS_WORKTREE :=
ifneq ($(_GIT_COMMON_DIR),$(_GIT_DIR))
	IS_WORKTREE := 1
endif

# When in a worktree, add -a flag to force rebuild all packages
WORKTREE_FLAGS :=
ifdef IS_WORKTREE
	WORKTREE_FLAGS := -a
endif

GO_TEST_TIMEOUT ?= 300s
GO_COVERAGE_TIMEOUT ?= 10m
GO_COVERAGE_MIN ?= 75.9
GO_UNIT_COVERAGE_MIN ?= $(GO_COVERAGE_MIN)
GO_FUNCTIONAL_COVERAGE_MIN ?= 33.1
GO_UNIT_COVERAGE_MANIFEST ?= docs/internal/baselines/go-unit-coverage-package-minimums.json
GO_FUNCTIONAL_COVERAGE_MANIFEST ?= docs/internal/baselines/go-functional-coverage-package-minimums.json
GO_UNIT_COVERAGE_PROFILE ?=
GO_FUNCTIONAL_COVERAGE_PROFILE ?=
BACKEND_SIZE_ROOT ?= .
PACKAGE_MAINT_ROOT ?= .
PACKAGE_FILE_COUNT_ROOT ?= .
PACKAGE_BOUNDARY_ROOT ?= .
PACKAGE_STRUCTURE_ROOT ?= .
BACKEND_DEPENDENCY_GRAPH_DIR ?= .artifacts/backend-dependency-graph
BACKEND_DEPENDENCY_GRAPH_DOT ?= $(BACKEND_DEPENDENCY_GRAPH_DIR)/backend-dependency-graph.dot
BACKEND_DEPENDENCY_GRAPH_SVG ?= $(BACKEND_DEPENDENCY_GRAPH_DIR)/backend-dependency-graph.svg
COMPATIBILITY_ALIAS_CHECK_ROOT ?= .
RETIRED_SURFACE_CHECK_ROOT ?= .
LINT_TARGETS ?= ui-lint ui-deadcode vet backend-size pkg-maint pkg-file-count pkg-boundary pkg-structure package-target-manifest-check packaged-factory-source-check packaged-factory-catalog-check provider-catalog-check model-provider-package-check durable-runtime-construction-check logging-boundary-check compatibility-alias-check retired-surface-check ownership-inventory-check deadcode

define run_verification_step
	@printf '%s\n' "==> $(2) [make $(1)]"
	@$(MAKE) $(1) || { status=$$?; printf '%s\n' "FAIL: $(2) [make $(1)] failed. Rerun with: make $(1)"; exit $$status; }
endef

define run_timed_step
	@start=$$(date +%s); \
	if $(1); then \
		status=0; \
	else \
		status=$$?; \
	fi; \
	end=$$(date +%s); \
	elapsed=$$((end - start)); \
	printf '%s\n' "[ui-coverage] $(2) elapsed: $${elapsed}.00s"; \
	exit $$status
endef


.PHONY: default build install bundle-api
.PHONY: fmt vet deps deps-tidy clean init typecheck release lint

.PHONY: test test-full test-unit test-unit-fresh test-lane-audit test-maintenance test-integration test-contract test-stress test-release
.PHONY: test-functional test-functional-long test-backend-functional functional-boundary-check
.PHONY: test-ui-coverage-merge test-ui-browser-integration test-ui-durable-session-real-backend
.PHONY: test-unit-coverage test-functional-coverage test-backend-coverage test-coverage-go test-race
.PHONY: test-backend-verification test-built-cli-acceptance long-tests long-tests-managed-runtime long-tests-functional-runtime pr-inference-approval

.PHONY: verify-fast verify-pr verify-pr-inference verify-extended verify-build verify-lint verify-api
.PHONY: verify-build-contracts verify-tests run-concurrent-ui-verification-lanes run-sharded-ui-coverage verify test-ui-coverage

.PHONY: backend-dependency-graph

.PHONY: generate-api generate-go-api generate-go-server-api generate-go-client-api generate-ui-api generate-wire

.PHONY: wire-smoke api-smoke api-package-pack-smoke packaged-factory-package-smoke packaged-factory-package-script-test packaged-factory-package-pack-check packaged-factory-package-candidate-dry-run packaged-factory-package-consumer-smoke model-provider-package-smoke model-provider-reference-input-smoke
.PHONY: contracts-validate contracts-generate contracts-check contracts-smoke

.PHONY: cli-contract-smoke cli-manifest-generate cli-manifest-check
.PHONY: fnd-12-behavior-baselines fnd-12-cli-behavior-baselines fnd-12-http-behavior-baselines fnd-12-mcp-behavior-baselines fnd-12-replay-behavior-baselines fnd-12-visualization-behavior-baselines

.PHONY: mcp-contract-check mcp-contract-smoke mcp-discovery-generate mcp-discovery-check

.PHONY: docs-reference-check docs-reference-smoke

.PHONY: script-timeout-companion-smoke-100 cron-time-work-smoke current-factory-watcher-switch-smoke provider-parity-smoke javascript-contract-smoke config-contract-smoke
.PHONY: backend-size pkg-maint pkg-file-count pkg-boundary pkg-structure package-target-manifest-check packaged-factory-source-check packaged-factory-catalog-generate packaged-factory-catalog-check provider-catalog-generate provider-catalog-check model-provider-package-generate model-provider-package-check durable-runtime-construction-check logging-boundary-check ownership-inventory-check
.PHONY: response-stream-stress-smoke release-surface-smoke artifact-contract-closeout
.PHONY: compatibility-alias-check retired-surface-check readme-check deadcode dashboard-verify

.PHONY: ci ci-typecheck ci-verify-build-contracts ci-verify-tests

.PHONY: ui-deps ui-lint ui-build ui-test ui-integration-test ui-durable-session-real-backend-integration-test ui-test-coverage ui-replay-coverage-check ui-install-playwright
.PHONY: ui-test-storybook ui-components-typecheck ui-components-test ui-components-storybook ui-components-boundary ui-components-dependency-direction ui-components-verify ui-verify-fresh-npm-install
.PHONY: ui-public-package-release ui-public-package-publish-prepare
.PHONY: ui-storybook  ui-deadcode


default:
	$(MAKE) generate-api
	$(MAKE) build
	$(MAKE) test
	$(MAKE) lint

build:
	$(GO) build $(WORKTREE_FLAGS) -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_PATH)

install:
	$(GO) build $(WORKTREE_FLAGS) -o $(INSTALL_DIR)/$(BINARY_NAME) $(CMD_PATH)

bundle-api:
	node scripts/run-quiet-api-command.js bundle:rest ./api/openapi-main.yaml ./api/openapi.yaml

generate-api: bundle-api generate-go-api generate-ui-api

generate-go-api: generate-go-server-api generate-go-client-api

generate-go-server-api:
	$(GO) generate -run=server -tags=interfaces ./pkg/transports/http

generate-go-client-api:
	$(GO) generate -run=client -tags=interfaces ./pkg/transports/http

generate-ui-api:
	cd ui && node ./scripts/generate-openapi-types.mjs ../api/openapi.yaml src/api/generated/openapi.ts

generate-wire:
	$(GO) generate ./pkg/...

wire-smoke:
	$(MAKE) generate-wire
	$(MAKE) generate-wire
	node scripts/check-wire-gen-drift.js
	$(GO) test ./pkg/wire/... -count=1 -timeout $(GO_TEST_TIMEOUT)

api-smoke:
	node scripts/run-quiet-api-command.js validate:main ./api/openapi-main.yaml
	$(MAKE) generate-api
	$(MAKE) generate-api
	node scripts/check-api-generated-drift.js
	$(GO) test ./pkg/transports/http/contracttests -run TestOpenAPIContract_BundledFactoryEventSchemasRemainComplete -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./tests/functional/runtime_api -run TestGeneratedAPIIntegrationSmoke_OpenAPIGeneratedServerAndLiveRuntimeStayAligned -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(MAKE) provider-parity-smoke

api-package-pack-smoke:
	node --test scripts/api-package-contract.test.mjs scripts/api-package-pack.test.mjs scripts/api-package-candidate.test.mjs scripts/api-package-registry.test.mjs scripts/api-package-consumer.test.mjs scripts/api-package-pr-dry-run.test.mjs scripts/api-package-publish.test.mjs scripts/api-package-development-workflow.test.mjs

packaged-factory-package-smoke: packaged-factory-catalog-check packaged-factory-package-script-test

packaged-factory-package-script-test:
	node --test scripts/packaged-factories-package-pack.test.mjs scripts/packaged-factories-package-candidate.test.mjs scripts/packaged-factories-package-consumer.test.mjs scripts/packaged-factories-package-pr-dry-run.test.mjs scripts/packaged-factories-package-registry.test.mjs scripts/packaged-factories-package-publish.test.mjs scripts/packaged-factories-package-development-command.test.mjs

packaged-factory-package-pack-check: packaged-factory-catalog-check
	node -e "require('node:fs').rmSync('.artifacts/packaged-factories-local-pack', { recursive: true, force: true })"
	node scripts/packaged-factories-package-candidate.mjs --package-directory packages/packaged-factories --output-directory .artifacts/packaged-factories-local-pack --run-id 1 --source-commit $(shell git rev-parse HEAD)

packaged-factory-package-candidate-dry-run: packaged-factory-catalog-check
	node -e "require('node:fs').rmSync('.artifacts/packaged-factories-local-dry-run', { recursive: true, force: true })"
	node scripts/packaged-factories-package-pr-dry-run.mjs --event-name pull_request --prerequisite-result success --ref refs/pull/local/head --repository portpowered/you-agent-factory --run-id 1 --source-commit $(shell git rev-parse HEAD) --pull-request-head-sha $(shell git rev-parse HEAD) --package-directory packages/packaged-factories --output-directory .artifacts/packaged-factories-local-dry-run --workspace-directory .

packaged-factory-package-consumer-smoke: packaged-factory-package-candidate-dry-run

model-provider-package-smoke:
	node --test scripts/model-provider-package.test.mjs
	node scripts/model-provider-package.mjs smoke

model-provider-reference-input-smoke:
	cd ui && $(UI_SCRIPT) test:model-provider-reference-input

contracts-validate:
	$(GO) run ./cmd/contractsvalidate -root .

contracts-generate:
	$(GO) run ./cmd/contractsgenerate -root .

contracts-check:
	$(GO) run ./cmd/contractscheck -root .
	$(GO) run ./cmd/functionalscenarioproject -check contracts/functional-scenarios.json

contracts-smoke:
	$(GO) test ./internal/contract... ./cmd/contracts... -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(MAKE) contracts-validate
	$(MAKE) contracts-check
	$(MAKE) contracts-generate
	$(MAKE) contracts-check
	$(MAKE) contracts-generate
	$(MAKE) contracts-check

mcp-contract-check:
	$(GO) run ./cmd/mcpcontractcheck -root .

mcp-contract-smoke:
	$(MAKE) contracts-validate
	$(MAKE) mcp-discovery-check
	$(MAKE) mcp-contract-check
	$(GO) test ./pkg/services/factory_sessions/transports/mcp/... -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/transports/mcp/... -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/transports/cli/mcp -count=1 -timeout $(GO_TEST_TIMEOUT)

mcp-discovery-generate:
	$(GO) run ./cmd/mcpdiscoverygen -root .

mcp-discovery-check:
	$(GO) run ./cmd/mcpdiscoverygen -root . -check

cli-manifest-generate:
	$(GO) run ./cmd/climanifestgen -root .

cli-manifest-check:
	$(GO) run ./cmd/climanifestgen -root . -check
	$(GO) run ./cmd/clicontractsmoke -root .

cli-contract-smoke:
	$(GO) run ./cmd/clicontractsmoke -root .
	$(GO) test ./cmd/clicontractsmoke ./pkg/transports/cli/clicontract -count=1 -timeout $(GO_TEST_TIMEOUT)

# FND-12 captured public behavior baselines (see
# docs/internal/baselines/fnd-12-public-behavior-baseline-suite-map.md).
# Aggregator runs all five surface pairs; does not migrate packages or refresh
# PR #1262 CLI-manifest baselines. Pair with `make verify-fast` and `make lint`.
fnd-12-behavior-baselines:
	$(MAKE) fnd-12-cli-behavior-baselines
	$(MAKE) fnd-12-http-behavior-baselines
	$(MAKE) fnd-12-mcp-behavior-baselines
	$(MAKE) fnd-12-replay-behavior-baselines
	$(MAKE) fnd-12-visualization-behavior-baselines

# FND-12 captured public CLI success + typed-failure pair.
# Does not refresh or re-own PR #1262 CLI-manifest baselines.
fnd-12-cli-behavior-baselines:
	$(GO) test ./pkg/transports/cli/baseline -run '^Test(RootHelpBaseline_MatchesFixture|FailureBaseline_QuietInvalidTopologyWritesStructuredInvocationFailure)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

fnd-12-http-behavior-baselines:
	$(GO) test ./tests/functional/runtime_api -run '^TestGeneratedAPIIntegrationSmoke_(OpenAPIGeneratedServerAndLiveRuntimeStayAligned|SubmitWorkItemsRejectEmptyStructuredSubmission)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

fnd-12-mcp-behavior-baselines:
	$(GO) test ./pkg/transports/mcp/server -run '^Test(ServeStdioUsesSDKProtocolAndRegistersCatalog|SDKProtocolErrors)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

# FND-12 captured public replay success + typed-failure pair.
fnd-12-replay-behavior-baselines:
	$(GO) test ./pkg/services/recordings/replay -run '^TestSideEffects_(InferReturnsRecordedProviderResponse|UnmatchedRequestFailsClearly)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

# FND-12 captured visualization activation success + typed-failure pair.
fnd-12-visualization-behavior-baselines:
	$(GO) test ./pkg/services/factory_visualization -run '^Test(ServiceProjectsRetainedAndLiveFactoryEvents|NewRejectsMissingDependencies)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

docs-reference-check:
	$(GO) run ./cmd/markdown-linter docs/README.md docs/reference

docs-reference-smoke:
	$(MAKE) docs-reference-check
	$(GO) test ./pkg/transports/cli/docs/... -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/transports/cli -run TestDocsCommand_ -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./tests/functional/smoke -run TestDocsCommandSmoke_ -count=1 -timeout $(GO_TEST_TIMEOUT)

readme-check:
	$(GO) run ./cmd/readmecheck

test:
	$(MAKE) test-unit

test-full:
	$(GO) test ./... -timeout $(GO_TEST_TIMEOUT)

test-unit:
	$(GO) run ./cmd/unitlane -jobs $(UNIT_DEFAULT_JOBS) -timeout $(GO_TEST_TIMEOUT)

test-unit-fresh:
	$(GO) run ./cmd/unitlane -jobs $(UNIT_DEFAULT_JOBS) -count=1 -timeout $(GO_TEST_TIMEOUT)

test-lane-audit:
	$(GO) run ./cmd/testlanecheck

test-maintenance:
	$(MAKE) test-lane-audit
	$(GO) test -short -p=$(UNIT_DEFAULT_JOBS) ./cmd/... ./internal/... ./packages/model-providers ./packages/packaged-factories ./tests/functional/internal/... ./ui ./pkg/services/factory_runtime/exhaustiontests -count=1 -timeout $(GO_TEST_TIMEOUT)

test-integration:
	$(GO) test -short -p=$(UNIT_DEFAULT_JOBS) ./pkg/services/factory_definitions/loading/runtimetests ./pkg/services/factory_definitions/persistence/integrationtests ./pkg/services/factory_definitions/portableconfig/integrationtests ./pkg/services/factory_sessions/internal/execution/fixtures ./pkg/transports/http/servertests/... -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/services/factory_runtime/ingest -run '^TestFileWatcher_' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/platform/process -run '^TestExecCommandRunner_' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/services/workers/worktree -run '^TestPrepareFactoryGitWorktree_(CreatesWorktreeWhenMissing|ReusesExistingValidWorktree|UsesExistingWorktreesParent|ReturnsFailureWhenWorktreeAddFails|ReturnsFailureWhenPathExistsButIsNotWorktree)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/services/workers/provider -run '^TestScriptWrapProvider_CommandEnvironmentPreventsGitMergeEditorPrompt$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

test-contract:
	$(GO) test -short -p=$(UNIT_DEFAULT_JOBS) ./contracts ./pkg/services/factory_definitions/contracts/contracttests ./pkg/services/workers/provider/functionaltests ./pkg/services/workers/provider/paritytests ./pkg/transports/http/contracttests ./pkg/transports/cli/clicontract ./pkg/transports/cli/climanifestgen -count=1 -timeout $(GO_TEST_TIMEOUT)

test-functional:
	$(MAKE) functional-boundary-check
	$(GO) run ./cmd/functionallane -jobs $(FUNCTIONAL_DEFAULT_JOBS) -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test -short ./pkg/transports/cli/baseline ./pkg/transports/cli/cliinputs ./pkg/transports/cli/commandidentity -count=1 -timeout $(GO_TEST_TIMEOUT)

functional-boundary-check:
	$(GO) run ./cmd/functionalboundarycheck

test-stress:
	$(GO) test -short $(STRESS_DEFAULT_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)

test-release:
	$(GO) test -short $(RELEASE_DEFAULT_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)

test-functional-long:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) $(FUNCTIONAL_LONG_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)

test-built-cli-acceptance:
	$(MAKE) build
	$(GO) test $(BUILT_CLI_ACCEPTANCE_PACKAGES) -count=1 -timeout $(BUILT_CLI_ACCEPTANCE_TIMEOUT)

verify-fast:
	$(info Running fast verification tier: typecheck + MCP contract boundary + short UI/unit suite + short Go suite)
	$(call run_verification_step,typecheck,dashboard typecheck)
	$(call run_verification_step,mcp-contract-check,MCP contract boundary)
	$(call run_verification_step,ui-test,short UI/unit suite)
	$(call run_verification_step,test,short Go suite)

verify-pr:
	$(info Running pull-request verification tier: build contracts + required CI-equivalent test lanes)
	$(call run_verification_step,verify-build-contracts,build contracts and static verification)
	$(call run_verification_step,verify-tests,required CI-equivalent test lanes)

verify-pr-inference:
	$(info Running PR-gated inference approval lane: $(PR_INFERENCE_APPROVAL_REGRESSION))
	$(info Required: export INFINITE_YOU_RUN_OMNIVOICE_LONG_TESTS=1)
	$(info Runtime: omnivoice-llamacpp on PATH, or set INFINITE_YOU_OMNIVOICE_COMMAND to the executable)
	$(info Optional: INFINITE_YOU_OMNIVOICE_CACHE_DIR to reuse managed model cache (omit to use a temp cache))
	$(info Broader specialty sweep remains on make long-tests; this lane is merge-blocking PR inference approval only)
	$(call run_verification_step,pr-inference-approval,PR inference approval regression)

verify-extended:
	$(info Running extended verification tier: required PR verification + opt-in long and specialty suites)
	$(call run_verification_step,verify-pr,pull-request verification tier)
	$(call run_verification_step,long-tests,opt-in long and specialty suites)

test-ui-coverage:
	$(MAKE) ui-test-coverage
	@if [ -z "$$UI_COVERAGE_SHARD" ] && [ -z "$$UI_COVERAGE_MERGE" ]; then $(MAKE) ui-replay-coverage-check; fi

test-ui-coverage-merge:
	$(MAKE) ui-test-coverage-merge
	$(MAKE) ui-replay-coverage-check

test-ui-browser-integration:
	$(MAKE) ui-integration-test

test-ui-durable-session-real-backend:
	$(MAKE) ui-durable-session-real-backend-integration-test

test-backend-coverage:
	$(MAKE) test-unit-coverage

test-backend-verification:
	$(MAKE) test-unit-coverage
	$(MAKE) test-functional-coverage

test-backend-functional:
	$(MAKE) test-functional-coverage

long-tests:
	$(info Running opt-in long and specialty suites: managed runtime coverage + real local inference coverage)
	$(call run_verification_step,long-tests-managed-runtime,Managed Runtime specialty lane)
	$(call run_verification_step,long-tests-functional-runtime,Real Local Inference specialty lane)

long-tests-managed-runtime:
	$(GO) test ./pkg/services/models/local -run '^TestOmniVoiceLocalRuntime_' -count=1 -timeout $(GO_TEST_TIMEOUT)

pr-inference-approval:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/runtime_api -run '$(PR_INFERENCE_APPROVAL_REGRESSION)' -count=1 -timeout $(MODEL_LONG_TEST_TIMEOUT)

long-tests-functional-runtime:
	$(MAKE) pr-inference-approval

test-coverage-go:
	$(info make test-coverage-go is a compatibility alias for unit coverage; use make test-functional-coverage for the independent functional report.)
	$(MAKE) test-unit-coverage

test-unit-coverage:
	$(GO) run ./cmd/gocoveragecheck -suite unit -min $(GO_UNIT_COVERAGE_MIN) -package-manifest $(GO_UNIT_COVERAGE_MANIFEST) -timeout $(GO_COVERAGE_TIMEOUT) $(if $(GO_UNIT_COVERAGE_PROFILE),-profile $(GO_UNIT_COVERAGE_PROFILE),)

test-functional-coverage:
	$(GO) run ./cmd/gocoveragecheck -suite functional -min $(GO_FUNCTIONAL_COVERAGE_MIN) -package-manifest $(GO_FUNCTIONAL_COVERAGE_MANIFEST) -timeout $(GO_COVERAGE_TIMEOUT) $(if $(GO_FUNCTIONAL_COVERAGE_PROFILE),-profile $(GO_FUNCTIONAL_COVERAGE_PROFILE),)

script-timeout-companion-smoke-100:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/providers -run $(SCRIPT_TIMEOUT_COMPANION_SMOKE_TEST) -count=$(SCRIPT_TIMEOUT_COMPANION_SMOKE_COUNT) -timeout $(SCRIPT_TIMEOUT_COMPANION_SMOKE_TIMEOUT)

cron-time-work-smoke:
	$(GO) test ./tests/functional/runtime_api -run $(CRON_TIME_WORK_SMOKE_TEST) -count=$(CRON_TIME_WORK_SMOKE_COUNT) -timeout $(CRON_TIME_WORK_SMOKE_TIMEOUT)

current-factory-watcher-switch-smoke:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/bootstrap_portability -run $(CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TEST) -count=$(CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_COUNT) -timeout $(CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TIMEOUT)

provider-parity-smoke:
	$(GO) test ./pkg/services/workers/provider/paritytests -run $(CROSS_PROVIDER_PARITY_SMOKE_TEST) -count=1 -timeout $(CROSS_PROVIDER_PARITY_SMOKE_TIMEOUT)

javascript-contract-smoke:
	$(GO) run ./cmd/javascriptcontractsmoke -root .
	$(GO) run ./cmd/javascriptcontractsmoke -root .
	$(GO) test ./internal/javascriptcontractsmoke ./cmd/javascriptcontractsmoke -count=1 -timeout $(JAVASCRIPT_CONTRACT_SMOKE_TIMEOUT)
	$(GO) test ./contracts -run '^TestJavaScriptRuntimeBehaviorDoesNotLoadContractManifests$$' -count=1 -timeout $(JAVASCRIPT_CONTRACT_SMOKE_TIMEOUT)
	$(GO) test ./pkg/orchestrators/javascript/runtime -run '$(JAVASCRIPT_RUNTIME_REGRESSION_TESTS)' -count=1 -timeout $(JAVASCRIPT_CONTRACT_SMOKE_TIMEOUT)

config-contract-smoke:
	$(GO) run ./cmd/configcontractsmoke -root .
	$(GO) test ./internal/configcontractsmoke ./cmd/configcontractsmoke -count=1 -timeout $(CONFIG_CONTRACT_SMOKE_TIMEOUT)
	$(GO) test ./contracts -run '^TestRuntimePackage' -count=1 -timeout $(CONFIG_CONTRACT_SMOKE_TIMEOUT)

response-stream-stress-smoke:
	$(GO) test ./pkg/services/factory_sessions/internal/responseeventstore -run $(RESPONSE_STREAM_STRESS_SMOKE_TEST) -count=1 -timeout $(RESPONSE_STREAM_STRESS_SMOKE_TIMEOUT)

artifact-contract-closeout:
	$(GO) test ./internal/testutil -run TestArtifactContractInventory_ -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(MAKE) release-surface-smoke
	$(GO) test ./pkg/transports/http ./pkg/services/factory_definitions/loading/runtimetests ./pkg/services/factory_definitions/portableconfig/integrationtests ./pkg/services/recordings/replay ./pkg/platform/replay ./tests/adhoc ./tests/functional/bootstrap_portability ./tests/functional/runtime_api -run "Test(AutomatPortabilityFixture_|GeneratedAPIIntegrationSmoke_|LegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly)" -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/replay_contracts -run "Test(ReplayEventStreamArtifactSmoke_|WorkerPublicContractSmoke_)" -count=1 -timeout $(GO_TEST_TIMEOUT)

lint:
	$(MAKE) $(LINT_TARGETS)

backend-size:
	$(GO) run ./cmd/backendsizecheck -root $(BACKEND_SIZE_ROOT)

backend-dependency-graph:
	$(GO) run ./cmd/backenddependencygraph -root . -go $(GO) -output $(BACKEND_DEPENDENCY_GRAPH_DOT) -svg-output $(BACKEND_DEPENDENCY_GRAPH_SVG)

pkg-maint:
	$(GO) run ./cmd/pkgmaintcheck -root $(PACKAGE_MAINT_ROOT)

pkg-file-count:
	$(GO) run ./cmd/pkgfilecountcheck -root $(PACKAGE_FILE_COUNT_ROOT)

pkg-boundary:
	$(GO) run ./cmd/pkgboundarycheck -root $(PACKAGE_BOUNDARY_ROOT)
	$(GO) run ./cmd/ownershipboundarycheck

pkg-structure:
	$(GO) run ./cmd/pkgstructurecheck -root $(PACKAGE_STRUCTURE_ROOT)

package-target-manifest-check:
	$(GO) run ./cmd/packagetargetmanifestcheck -root .

packaged-factory-source-check:
	$(GO) run ./cmd/packagedfactorysourcecheck -root .

packaged-factory-catalog-generate:
	$(GO) run ./cmd/packagedfactorycataloggenerate -root .

packaged-factory-catalog-check:
	$(GO) run ./cmd/packagedfactorycatalogcheck -root .

provider-catalog-generate:
	$(GO) run ./cmd/providercataloggenerate -root .

provider-catalog-check:
	$(GO) run ./cmd/providercatalogcheck -root .

model-provider-package-generate:
	node scripts/model-provider-package.mjs generate

model-provider-package-check:
	node scripts/model-provider-package.mjs check

ownership-boundary-check:
	$(GO) run ./cmd/ownershipboundarycheck

ownership-inventory-check:
	$(GO) run ./cmd/ownershipinventorycheck

durable-runtime-construction-check:
	$(GO) run ./cmd/durableruntimeconstructioncheck -root .

logging-boundary-check:
	$(GO) run ./cmd/loggingboundarycheck -root .

compatibility-alias-check:
	$(GO) run ./cmd/compatibilityaliascheck -root $(COMPATIBILITY_ALIAS_CHECK_ROOT)

retired-surface-check:
	$(GO) run ./cmd/retiredsurfacecheck -root $(RETIRED_SURFACE_CHECK_ROOT)

deadcode:
	$(GO) run ./cmd/deadcodecheck

ui-deadcode:
	cd ui && $(UI_SCRIPT) deadcode

verify-build:
	$(MAKE) ui-build
	$(MAKE) build

verify-lint:
	$(MAKE) lint
	$(MAKE) ui-components-verify

verify-api:
	$(MAKE) contracts-smoke
	$(MAKE) api-smoke
	$(MAKE) response-stream-stress-smoke
	$(MAKE) api-package-pack-smoke
	$(MAKE) model-provider-package-smoke
	$(MAKE) model-provider-reference-input-smoke
	$(MAKE) wire-smoke

verify-build-contracts:
	$(MAKE) typecheck
	$(MAKE) verify-build
	$(MAKE) verify-lint
	$(MAKE) verify-api

run-sharded-ui-coverage:
	./scripts/ci/run-sharded-ui-coverage.sh

run-concurrent-ui-verification-lanes:
	./scripts/ci/run-concurrent-ui-verification-lanes.sh

verify-tests:
	$(info Running required CI-equivalent test lanes: maintenance + integration + contract + release surface + built-CLI S24 acceptance + concurrent UI coverage/browser integration + independent backend unit and functional coverage)
	$(call run_verification_step,test-maintenance,Backend Maintenance lane)
	$(call run_verification_step,test-integration,Backend Integration lane)
	$(call run_verification_step,test-contract,Backend Contract lane)
	$(call run_verification_step,release-surface-smoke,Release surface smoke lane)
	$(call run_verification_step,test-built-cli-acceptance,Built-CLI S24 acceptance lane)
	$(call run_verification_step,run-concurrent-ui-verification-lanes,Concurrent UI Coverage + UI Browser Integration lanes)
	$(call run_verification_step,test-unit-coverage,Backend Unit Coverage lane)
	$(call run_verification_step,test-functional-coverage,Backend Functional Coverage lane)

verify:
	$(info make verify is a compatibility alias for the canonical pull-request tier; prefer make verify-pr)
	$(MAKE) verify-pr

dashboard-verify:
	$(MAKE) ui-build
	$(MAKE) lint
	$(MAKE) test

typecheck:
	cd ui && $(UI_SCRIPT) tsc

ci: ci-typecheck ci-verify-build-contracts ci-verify-tests

ci-typecheck:
	$(MAKE) ui-deps
	$(MAKE) typecheck

ci-verify-build-contracts: ci-typecheck
	$(MAKE) verify-build
	$(MAKE) verify-lint
	$(MAKE) verify-api

ci-verify-tests: ci-verify-build-contracts
	$(MAKE) ui-install-playwright
	$(MAKE) test-maintenance
	$(MAKE) test-integration
	$(MAKE) test-contract
	$(MAKE) release-surface-smoke
	$(MAKE) test-built-cli-acceptance
	$(MAKE) run-concurrent-ui-verification-lanes
	$(MAKE) test-unit-coverage
	$(MAKE) test-functional-coverage

release:
	$(GO) run ./cmd/releaseprep -version $(VERSION)

release-surface-smoke:
	$(MAKE) ui-build
	$(MAKE) build
	$(MAKE) ui-install-playwright
	sh ./scripts/release/smoke-artifact.sh "$(CURDIR)/$(BIN_DIR)/$(BINARY_NAME)" "tests/release/testdata/cli_smoke_factory"

ui-deps:
	cd ui && $(UI_INSTALL)
	cd ui && $(UI_SCRIPT) prepare:packaged-factories

ui-verify-fresh-npm-install:
	cd ui && $(NPM) run verify:fresh-npm-install

ui-lint:
	cd ui && $(UI_SCRIPT) lint

test-race:
	$(GO) test ./... -race -timeout 30s -v

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

deps:
	$(GO) mod download

deps-tidy:
	$(GO) mod tidy

init:
ifeq ($(BUN_BIN),)
	$(error init requires bun on PATH; install Bun 1.3.12+ per ui/package.json packageManager and retry)
endif
	@set -e; \
	for dir in $(BUN_PACKAGE_DIRS); do \
		printf '%s\n' "==> bun install ($$dir)"; \
		(cd "$$dir" && $(BUN_INSTALL)) || { \
			printf '%s\n' "FAIL: bun install failed in $$dir. Rerun from repository root: cd $$dir && bun install --frozen-lockfile"; \
			exit 1; \
		}; \
	done

ui-build:
ifeq ($(BUN_BIN),)
	cd ui && $(NPM) exec tsc -b && $(NPM) exec vite build && node scripts/normalize-dist-output.mjs
else
	cd ui && $(UI_SCRIPT) build
endif

ui-test:
	cd ui && $(UI_SCRIPT) test:unit

ui-integration-test:
ifeq ($(BUN_BIN),)
	cd ui && $(NPM) run test:integration
else
	cd ui && $(UI_SCRIPT) test:integration
endif
	$(MAKE) ui-storybook
	$(MAKE) ui-test-storybook-browser-checks

ui-durable-session-real-backend-integration-test:
ifeq ($(BUN_BIN),)
	cd ui && $(NPM) run test:integration:durable-session-real-backend
else
	cd ui && $(UI_SCRIPT) test:integration:durable-session-real-backend
endif

ui-test-coverage:
	cd ui && $(UI_SCRIPT) test:coverage

ui-test-coverage-merge:
	cd ui && UI_COVERAGE_MERGE=1 $(UI_SCRIPT) test:coverage

ui-replay-coverage-check:
ifeq ($(BUN_BIN),)
	$(call run_timed_step,cd ui && $(NPM) exec tsx scripts/write-replay-coverage-report.ts --check,Replay coverage check)
else
	$(call run_timed_step,cd ui && $(UI_SCRIPT) replay:coverage:check,Replay coverage check)
endif

ui-install-playwright:
	cd ui && $(UI_EXEC) playwright install chromium

ui-storybook:
	cd ui && $(UI_SCRIPT) build-storybook

ui-test-storybook:
	cd ui && $(UI_SCRIPT) test-storybook

ui-test-storybook-browser-checks:
	cd ui && $(UI_SCRIPT) test-storybook:browser-checks

ui-components-typecheck:
	cd ui/packages/components && $(UI_SCRIPT) typecheck

ui-components-test:
	cd ui/packages/components && $(UI_SCRIPT) test:unit

ui-components-storybook:
	cd ui/packages/components && $(UI_SCRIPT) build-storybook

ui-components-boundary:
	cd ui/packages/components && $(UI_SCRIPT) check:package-boundary

ui-components-dependency-direction:
	cd ui/packages/components && $(UI_SCRIPT) check:package-dependency-direction

ui-components-verify:
	$(MAKE) ui-components-typecheck
	$(MAKE) ui-components-test
	$(MAKE) ui-components-storybook
	$(MAKE) ui-components-boundary
	$(MAKE) ui-components-dependency-direction

ui-public-package-release:
	cd ui && $(UI_SCRIPT) verify:public-packages

ui-public-package-publish-prepare:
	cd ui && $(UI_SCRIPT) publish:public-packages:prepare -- --version "$(PACKAGE_VERSION)" --output-directory "$(abspath $(or $(PACKAGE_OUTPUT),.artifacts/public-packages))"

clean:
	$(GO) clean ./...
	rm -rf $(BIN_DIR)
