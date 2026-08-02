BINARY_NAME := you
CMD_PATH    := ./cmd/factory/
BIN_DIR     := bin
GO          ?= go
INSTALL_DIR := $(or $(GOBIN),$(shell $(GO) env GOPATH)/bin)
NPM         ?= npm
ifeq ($(OS),Windows_NT)
BUN_BIN     := $(shell where.exe bun >NUL 2>NUL && echo bun)
else
BUN_BIN     := $(shell command -v bun 2>/dev/null)
endif
BUN_INSTALL := $(BUN_BIN) install
BUN_PACKAGE_DIRS := ui/packages/components ui
UI_SCRIPT   := $(if $(BUN_BIN),$(BUN_BIN) run,$(NPM) run)
UI_EXEC     := $(if $(BUN_BIN),$(BUN_BIN) x,$(NPM) exec)
UI_INSTALL  := $(if $(BUN_BIN),$(BUN_BIN) install,$(NPM) install --no-package-lock)
FUNCTIONAL_DEFAULT_PACKAGES := ./tests/functional/...
FUNCTIONAL_DEFAULT_JOBS ?= 8
UNIT_DEFAULT_JOBS ?= 32
FUNCTIONAL_LONG_TAGS ?= functionallong
FUNCTIONAL_LONG_PACKAGES := ./tests/functional/...
STRESS_DEFAULT_PACKAGES := ./tests/stress/...
RELEASE_DEFAULT_PACKAGES := ./tests/release/...
MODEL_LONG_TEST_TIMEOUT ?= 20m
PR_INFERENCE_APPROVAL_REGRESSION ?= TestRealLocalInference_OMNIVOICEModelInvokeAndDirectAPIProduceAudio
SCRIPT_TIMEOUT_COMPANION_SMOKE_TEST := TestProviderCancellationTerminatesCompanionProcesses
SCRIPT_TIMEOUT_COMPANION_SMOKE_COUNT ?= 100
SCRIPT_TIMEOUT_COMPANION_SMOKE_TIMEOUT ?= 120s
CRON_TIME_WORK_SMOKE_TEST := TestCronFiresAtInjectedTimeWithoutWallClockSleep
CRON_TIME_WORK_SMOKE_COUNT ?= 10
CRON_TIME_WORK_SMOKE_TIMEOUT ?= 120s
CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TEST := TestCurrentFactoryActivationSwitchesPersistedFactories
CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_COUNT ?= 1
CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TIMEOUT ?= 120s
CROSS_PROVIDER_PARITY_SMOKE_TEST := TestCrossProviderParity
CROSS_PROVIDER_PARITY_SMOKE_TIMEOUT ?= 120s
JAVASCRIPT_CONTRACT_SMOKE_TIMEOUT ?= 120s
CONFIG_CONTRACT_SMOKE_TIMEOUT ?= 120s
JAVASCRIPT_RUNTIME_REGRESSION_TESTS ?= ^(TestCallBehavior_WorkflowFinalInventoryMatchesExecution|TestCallBehavior_AgentRunInventoryMatchesExecution|TestRun_ProgressPrimitives_EmitsOrderedRuntimeRecords|TestRun_PolicyDeniedChildOperations_ReturnStableDiagnostics|TestCallBehavior_WorkflowResumeStateInventoryMatchesExecution)$$
RESPONSE_STREAM_STRESS_SMOKE_TEST := TestSessionResponseEventStore_Backpressure
RESPONSE_STREAM_STRESS_SMOKE_TIMEOUT ?= 120s
ROOT_PROCESS_ACCEPTANCE_PACKAGES := ./tests/functional/acceptance ./tests/functional/transport/cli/process
ROOT_PROCESS_ACCEPTANCE_TIMEOUT ?= 300s

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
GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT ?=
FUNCTIONAL_TEST_VIZ_DIR ?= .artifacts/functional-test-viz
FUNCTIONAL_TEST_VIZ_PROFILE ?= $(FUNCTIONAL_TEST_VIZ_DIR)/coverage.out
FUNCTIONAL_TEST_VIZ_JSON ?= $(FUNCTIONAL_TEST_VIZ_DIR)/coverage-summary.json
FUNCTIONAL_TEST_VIZ_MARKDOWN ?= $(FUNCTIONAL_TEST_VIZ_DIR)/functional-tests.md
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
LINT_TARGETS ?= ui-lint ui-deadcode vet backend-size pkg-maint pkg-file-count pkg-boundary pkg-structure package-target-manifest-check packaged-factory-source-check packaged-factory-consumption-check packaged-factory-catalog-check provider-catalog-check model-provider-package-check durable-runtime-construction-check logging-boundary-check compatibility-alias-check retired-surface-check ownership-inventory-check deadcode

define run_verification_step
	@printf '%s\n' "==> $(2) [make $(1)]"
	@$(MAKE) $(1) || { status=$$?; printf '%s\n' "FAIL: $(2) [make $(1)] failed. Rerun with: make $(1)"; exit $$status; }
endef

define ensure_directory
	@mkdir -p $(1)
endef

ifeq ($(OS),Windows_NT)
define run_verification_step
	@echo Running $(2) [make $(1)]
	@$(MAKE) $(1) || (echo FAIL: $(2) [make $(1)] failed. Rerun with: make $(1) & exit /b 1)
endef

define ensure_directory
	@if not exist "$(subst /,\,$(1))" mkdir "$(subst /,\,$(1))"
endef
endif

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
.PHONY: test-functional test-functional-long test-backend-functional functional-boundary-check functional-test-viz
.PHONY: test-ui-browser-integration test-ui-storybook-integration test-ui-durable-session-real-backend test-ui-performance ui-component-test
.PHONY: test-unit-coverage test-functional-coverage test-backend-coverage test-coverage-go test-race
.PHONY: test-backend-verification test-root-process-acceptance long-tests long-tests-managed-runtime long-tests-functional-runtime pr-inference-approval

.PHONY: verify-fast verify-pr verify-pr-inference verify-extended verify-build verify-lint verify-api
.PHONY: verify-build-contracts verify-tests run-concurrent-ui-verification-lanes verify test-ui-coverage

.PHONY: backend-dependency-graph

.PHONY: generate-api generate-go-api generate-go-server-api generate-go-client-api generate-ui-api generate-wire
.PHONY: interfaces-api-bundle interfaces-go interfaces-contracts interfaces-ui-openapi interfaces-ui-client interfaces-ui-emulator interfaces-ui interfaces-all

.PHONY: wire-smoke api-smoke api-package-pack-smoke api-package-verify packaged-factory-package-smoke packaged-factory-package-verify packaged-factory-package-script-test packaged-factory-package-pack-check packaged-factory-package-candidate-dry-run packaged-factory-package-consumer-smoke model-provider-package-smoke model-provider-package-verify model-provider-reference-input-smoke
.PHONY: public-release-package-smoke
.PHONY: contracts-validate contracts-generate contracts-check contracts-smoke

.PHONY: cli-contract-smoke cli-manifest-generate cli-manifest-check
.PHONY: fnd-12-behavior-baselines fnd-12-cli-behavior-baselines fnd-12-http-behavior-baselines fnd-12-mcp-behavior-baselines fnd-12-replay-behavior-baselines fnd-12-visualization-behavior-baselines

.PHONY: mcp-contract-check mcp-contract-smoke mcp-discovery-generate mcp-discovery-check

.PHONY: docs-reference-check docs-reference-smoke

.PHONY: script-timeout-companion-smoke-100 cron-time-work-smoke current-factory-watcher-switch-smoke provider-parity-smoke javascript-contract-smoke config-contract-smoke
.PHONY: backend-size pkg-maint pkg-file-count pkg-boundary pkg-structure package-target-manifest-check packaged-factory-source-check packaged-factory-consumption-check packaged-factory-catalog-generate packaged-factory-catalog-check provider-catalog-generate provider-catalog-check model-provider-package-generate model-provider-package-check durable-runtime-construction-check logging-boundary-check ownership-inventory-check
.PHONY: response-stream-stress-smoke release-surface-smoke artifact-contract-closeout
.PHONY: compatibility-alias-check retired-surface-check readme-check deadcode dashboard-verify

.PHONY: ci ci-typecheck ci-verify-build-contracts ci-verify-tests

.PHONY: ui-deps ui-lint ui-build ui-test ui-performance-test ui-integration-test ui-storybook-integration-test ui-durable-session-real-backend-integration-test ui-test-coverage ui-replay-coverage-check ui-install-playwright
.PHONY: ui-test-storybook ui-components-typecheck ui-components-test ui-components-storybook ui-components-boundary ui-components-dependency-direction ui-components-verify ui-verify-fresh-npm-install
.PHONY: ui-public-package-release ui-public-package-publish-prepare
.PHONY: ui-storybook  ui-deadcode
.PHONY: ui-package-client-build ui-package-components-build ui-package-emulator-build ui-package-replay-build ui-package-visualizers-build ui-packages-build ui-dashboard-build ui-build-all build-all


default:
	$(MAKE) generate-api
	$(MAKE) ui-deps
	$(MAKE) ui-build
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

# Interface generation is split by consumer so callers can refresh only UI
# artifacts or all generated interfaces without relying on prerequisite order.
interfaces-api-bundle:
	$(MAKE) bundle-api

interfaces-go: interfaces-api-bundle
	$(MAKE) generate-go-api

interfaces-contracts:
	$(MAKE) contracts-generate

interfaces-ui-openapi: interfaces-api-bundle ui-deps
	$(MAKE) generate-ui-api

interfaces-ui-client: interfaces-contracts interfaces-ui-openapi
	cd ui/packages/client && $(UI_SCRIPT) generate

interfaces-ui-emulator: ui-deps
	cd ui/packages/factory-emulator && $(UI_SCRIPT) generate

# Regenerates the dashboard and UI-package interface artifacts without emitting
# the generated Go HTTP server/client interfaces.
interfaces-ui: interfaces-ui-client interfaces-ui-emulator

# Regenerates every public interface artifact: bundled OpenAPI, Go HTTP
# interfaces, contract schemas, and UI-facing generated artifacts.
interfaces-all: interfaces-go interfaces-ui

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
	$(GO) test ./tests/functional/transport/http/server -run TestGeneratedClientAndServerSchemaStayAligned -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(MAKE) provider-parity-smoke

api-package-pack-smoke:
	node --test scripts/package-export-validation.test.mjs scripts/api-package-contract.test.mjs scripts/api-package-pack.test.mjs scripts/api-package-candidate.test.mjs scripts/api-package-registry.test.mjs scripts/api-package-consumer.test.mjs scripts/api-package-pr-dry-run.test.mjs scripts/api-package-publish.test.mjs scripts/api-package-development-workflow.test.mjs

api-package-verify: api-package-pack-smoke

packaged-factory-package-smoke: packaged-factory-catalog-check packaged-factory-package-script-test

packaged-factory-package-verify: packaged-factory-package-smoke

packaged-factory-package-script-test:
	node --test scripts/packaged-factories-package-pack.test.mjs scripts/packaged-factories-package-candidate.test.mjs scripts/packaged-factories-package-consumer.test.mjs scripts/packaged-factories-package-pr-dry-run.test.mjs scripts/packaged-factories-package-registry.test.mjs scripts/packaged-factories-package-publish.test.mjs scripts/packaged-factories-package-development-command.test.mjs

packaged-factory-package-pack-check: packaged-factory-catalog-check
	node -e "require('node:fs').rmSync('.artifacts/packaged-factories-local-pack', { recursive: true, force: true })"
	node scripts/packaged-factories-package-candidate.mjs --package-directory packages/packaged-factories --output-directory .artifacts/packaged-factories-local-pack --run-id 1 --source-commit $(shell git rev-parse HEAD)

packaged-factory-package-candidate-dry-run: packaged-factory-catalog-check
	node -e "require('node:fs').rmSync('.artifacts/packaged-factories-local-dry-run', { recursive: true, force: true })"
	node scripts/packaged-factories-package-pr-dry-run.mjs --event-name pull_request --prerequisite-result success --ref refs/pull/local/head --repository portpowered/you-agent-factory --run-id 1 --source-commit $(shell git rev-parse HEAD) --pull-request-head-sha $(shell git rev-parse HEAD) --package-directory packages/packaged-factories --output-directory .artifacts/packaged-factories-local-dry-run --workspace-directory .

packaged-factory-package-consumer-smoke: packaged-factory-package-candidate-dry-run

public-release-package-smoke:
	node --test scripts/public-package-set.test.mjs scripts/public-release-package-publish.test.mjs scripts/public-release-package-candidate.test.mjs

model-provider-package-smoke:
	node --test scripts/model-provider-package.test.mjs
	node scripts/model-provider-package.mjs smoke

model-provider-package-verify: model-provider-package-smoke

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
	$(GO) test ./tests/functional/transport/http/server -run TestGeneratedClientAndServerSchemaStayAligned -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./tests/functional/work/submission -run TestAPISubmitWorkRejectsEmptyStructuredSubmission -count=1 -timeout $(GO_TEST_TIMEOUT)

fnd-12-mcp-behavior-baselines:
	$(GO) test ./pkg/transports/mcp/server -run '^Test(ServeStdioUsesSDKProtocolAndRegistersCatalog|SDKProtocolErrors)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

# FND-12 captured public replay success + typed-failure pair.
fnd-12-replay-behavior-baselines:
	$(GO) test ./pkg/services/recordings/replay -run '^TestSideEffects_(InferReturnsRecordedProviderResponse|UnmatchedRequestFailsClearly)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

# FND-12 captured visualization activation success + typed-failure pair.
fnd-12-visualization-behavior-baselines:
	$(GO) test ./pkg/services/factory_visualization/internal/service -run '^Test(ServiceProjectsRetainedAndLiveFactoryEvents|NewRejectsMissingDependencies)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

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
	$(GO) test -short -p=$(UNIT_DEFAULT_JOBS) ./cmd/... ./internal/... ./packages/model-providers ./packages/packaged-factories ./tests/functional/internal/... ./ui ./pkg/services/factory_runtime/internal/exhaustiontests -count=1 -timeout $(GO_TEST_TIMEOUT)

test-integration:
	$(GO) test -short -p=$(UNIT_DEFAULT_JOBS) ./pkg/services/factory_definitions/internal/services/compilation/runtimetests ./pkg/services/factory_definitions/internal/services/catalog/persistence/integrationtests ./pkg/services/factory_definitions/portableconfig/integrationtests ./pkg/services/factory_sessions/internal/execution/fixtures ./pkg/transports/http/servertests/... -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/services/automations/internal/services/filesystem_watchers/internal/service -run '^TestFileWatcher_' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/platform/process -run '^TestExecCommandRunner_' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/services/workers/worktree -run '^TestPrepareFactoryGitWorktree_(CreatesWorktreeWhenMissing|ReusesExistingValidWorktree|UsesExistingWorktreesParent|ReturnsFailureWhenWorktreeAddFails|ReturnsFailureWhenPathExistsButIsNotWorktree)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/services/providers/internal/services/execution/internal/provider -run '^TestScriptWrapProvider_CommandEnvironmentPreventsGitMergeEditorPrompt$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

test-contract:
	$(GO) test -short -p=$(UNIT_DEFAULT_JOBS) ./contracts ./pkg/services/factory_definitions/contracts/contracttests ./pkg/services/providers/internal/services/execution/internal/provider/functionaltests ./pkg/services/providers/internal/services/execution/internal/provider/paritytests ./pkg/transports/http/contracttests ./pkg/transports/cli/baseline ./pkg/transports/cli/clicontract ./pkg/transports/cli/cliinputs ./pkg/transports/cli/climanifestgen ./pkg/transports/cli/commandidentity -count=1 -timeout $(GO_TEST_TIMEOUT)

test-functional:
	$(MAKE) functional-boundary-check
	$(GO) run ./cmd/functionallane -jobs $(FUNCTIONAL_DEFAULT_JOBS) -count=1 -timeout $(GO_TEST_TIMEOUT)

functional-boundary-check:
	$(GO) run ./cmd/functionalboundarycheck

# functional-test-viz runs the boundary check, then the required short functional
# coverage lane exactly once (profile + gocoveragecheck -json-output), then the
# FND-004 Markdown catalog generator. Artifacts land under
# .artifacts/functional-test-viz/.
#
# Fail-closed composition: each recipe line must succeed before the next runs.
# Boundary, suite, coverage-floor, metadata/inventory, or Markdown rendering
# failures exit non-zero. The target never deletes the artifact root on failure,
# so already-written diagnostics (for example coverage.out / coverage-summary.json
# after a floor fail, or those files before a render fail) remain inspectable.
# gocoveragecheck writes -json-output after a completed measurement even when a
# floor fails; Make then stops before Markdown so the failure stays non-zero.
functional-test-viz:
	$(MAKE) functional-boundary-check
	$(call ensure_directory,$(FUNCTIONAL_TEST_VIZ_DIR))
	$(MAKE) test-functional-coverage \
		GO_FUNCTIONAL_COVERAGE_PROFILE=$(FUNCTIONAL_TEST_VIZ_PROFILE) \
		GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT=$(FUNCTIONAL_TEST_VIZ_JSON)
	$(GO) run ./cmd/functionaltestviz \
		-coverage-summary $(FUNCTIONAL_TEST_VIZ_JSON) \
		-output $(FUNCTIONAL_TEST_VIZ_MARKDOWN)

test-stress:
	$(GO) test -short $(STRESS_DEFAULT_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)

test-release:
	$(GO) test -short $(RELEASE_DEFAULT_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)

test-functional-long:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) $(FUNCTIONAL_LONG_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)

test-root-process-acceptance:
	$(GO) test $(ROOT_PROCESS_ACCEPTANCE_PACKAGES) -count=1 -timeout $(ROOT_PROCESS_ACCEPTANCE_TIMEOUT)

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
	$(MAKE) ui-replay-coverage-check

test-ui-browser-integration:
	$(MAKE) ui-integration-test

test-ui-storybook-integration:
	$(MAKE) ui-storybook-integration-test

test-ui-durable-session-real-backend:
	$(MAKE) ui-durable-session-real-backend-integration-test

test-ui-performance:
	$(MAKE) ui-performance-test

test-backend-coverage:
	$(MAKE) test-unit-coverage

test-backend-verification:
	$(MAKE) test-unit-coverage
	$(MAKE) test-functional-coverage

test-backend-functional:
	$(MAKE) test-functional-coverage

long-tests:
	$(info Running opt-in long and specialty suites: UI performance + managed runtime coverage + real local inference coverage)
	$(call run_verification_step,test-ui-performance,UI Performance specialty lane)
	$(call run_verification_step,long-tests-managed-runtime,Managed Runtime specialty lane)
	$(call run_verification_step,long-tests-functional-runtime,Real Local Inference specialty lane)

long-tests-managed-runtime:
	$(GO) test ./pkg/services/models/internal/local -run '^TestOmniVoiceLocalRuntime_' -count=1 -timeout $(GO_TEST_TIMEOUT)

pr-inference-approval:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/runtime_api -run '$(PR_INFERENCE_APPROVAL_REGRESSION)' -count=1 -timeout $(MODEL_LONG_TEST_TIMEOUT)

long-tests-functional-runtime:
	$(MAKE) pr-inference-approval

test-coverage-go:
	$(info make test-coverage-go is a compatibility alias for unit coverage; use make test-functional-coverage for the independent functional report.)
	$(MAKE) test-unit-coverage

test-unit-coverage:
	$(GO) run ./cmd/gocoveragecheck -suite unit -min $(GO_UNIT_COVERAGE_MIN) -package-manifest $(GO_UNIT_COVERAGE_MANIFEST) -timeout $(GO_COVERAGE_TIMEOUT) $(if $(GO_UNIT_COVERAGE_PROFILE),-profile $(GO_UNIT_COVERAGE_PROFILE),)

# test-functional-coverage always runs functional-boundary-check first so the
# required CI Backend Functional Coverage lane (and any local/alias caller of
# this target) cannot succeed without a successful boundary check. Boundary
# failures exit non-zero before gocoveragecheck starts.
test-functional-coverage:
	$(MAKE) functional-boundary-check
	$(GO) run ./cmd/gocoveragecheck -suite functional -jobs $(FUNCTIONAL_DEFAULT_JOBS) -min $(GO_FUNCTIONAL_COVERAGE_MIN) -package-manifest $(GO_FUNCTIONAL_COVERAGE_MANIFEST) -timeout $(GO_COVERAGE_TIMEOUT) $(if $(GO_FUNCTIONAL_COVERAGE_PROFILE),-profile $(GO_FUNCTIONAL_COVERAGE_PROFILE),) $(if $(GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT),-json-output $(GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT),)

script-timeout-companion-smoke-100:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/workers/inference -run $(SCRIPT_TIMEOUT_COMPANION_SMOKE_TEST) -count=$(SCRIPT_TIMEOUT_COMPANION_SMOKE_COUNT) -timeout $(SCRIPT_TIMEOUT_COMPANION_SMOKE_TIMEOUT)

cron-time-work-smoke:
	$(GO) test ./tests/functional/workstations/cron -run $(CRON_TIME_WORK_SMOKE_TEST) -count=$(CRON_TIME_WORK_SMOKE_COUNT) -timeout $(CRON_TIME_WORK_SMOKE_TIMEOUT)

current-factory-watcher-switch-smoke:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/factory/current -run $(CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TEST) -count=$(CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_COUNT) -timeout $(CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TIMEOUT)

provider-parity-smoke:
	$(GO) test ./pkg/services/providers/internal/services/execution/internal/provider/paritytests -run $(CROSS_PROVIDER_PARITY_SMOKE_TEST) -count=1 -timeout $(CROSS_PROVIDER_PARITY_SMOKE_TIMEOUT)

javascript-contract-smoke:
	$(GO) run ./cmd/javascriptcontractsmoke -root .
	$(GO) run ./cmd/javascriptcontractsmoke -root .
	$(GO) test ./internal/javascriptcontractsmoke ./cmd/javascriptcontractsmoke -count=1 -timeout $(JAVASCRIPT_CONTRACT_SMOKE_TIMEOUT)
	$(GO) test ./contracts -run '^TestJavaScriptRuntimeBehaviorDoesNotLoadContractManifests$$' -count=1 -timeout $(JAVASCRIPT_CONTRACT_SMOKE_TIMEOUT)
	$(GO) test ./pkg/services/factory_runtime/internal/services/orchestration/javascript/runtime -run '$(JAVASCRIPT_RUNTIME_REGRESSION_TESTS)' -count=1 -timeout $(JAVASCRIPT_CONTRACT_SMOKE_TIMEOUT)

config-contract-smoke:
	$(GO) run ./cmd/configcontractsmoke -root .
	$(GO) test ./internal/configcontractsmoke ./cmd/configcontractsmoke -count=1 -timeout $(CONFIG_CONTRACT_SMOKE_TIMEOUT)
	$(GO) test ./contracts -run '^TestRuntimePackage' -count=1 -timeout $(CONFIG_CONTRACT_SMOKE_TIMEOUT)

response-stream-stress-smoke:
	$(GO) test ./pkg/services/factory_sessions/internal/responseeventstore -run $(RESPONSE_STREAM_STRESS_SMOKE_TEST) -count=1 -timeout $(RESPONSE_STREAM_STRESS_SMOKE_TIMEOUT)

artifact-contract-closeout:
	$(GO) test ./internal/testutil -run TestArtifactContractInventory_ -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(MAKE) release-surface-smoke
	$(GO) test ./pkg/transports/http ./pkg/services/factory_definitions/internal/services/compilation/runtimetests ./pkg/services/factory_definitions/portableconfig/integrationtests ./pkg/services/recordings/replay ./pkg/platform/replay ./tests/adhoc ./tests/functional/bootstrap_portability ./tests/functional/runtime_api -run "Test(AutomatPortabilityFixture_|GeneratedAPIIntegrationSmoke_)" -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./tests/functional/work/submission -run TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./tests/functional/transport/http/server -run TestGeneratedClientAndServerSchemaStayAligned -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/replay_contracts -run "TestReplayEventStreamArtifactSmoke_" -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/workers/script -run "TestWorkerPublicContractSmoke_" -count=1 -timeout $(GO_TEST_TIMEOUT)

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

packaged-factory-consumption-check:
	$(GO) run ./cmd/packagedfactoryconsumptioncheck -root .

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

run-concurrent-ui-verification-lanes:
	./scripts/ci/run-concurrent-ui-verification-lanes.sh

verify-tests:
	$(info Running required CI-equivalent test lanes: maintenance + integration + contract + release surface + root-process S24 acceptance + concurrent UI coverage/browser integration + Storybook + UI backend integration + independent backend unit and functional coverage)
	$(call run_verification_step,test-maintenance,Backend Maintenance lane)
	$(call run_verification_step,test-integration,Backend Integration lane)
	$(call run_verification_step,test-contract,Backend Contract lane)
	$(call run_verification_step,release-surface-smoke,Release surface smoke lane)
	$(call run_verification_step,test-root-process-acceptance,Root-process S24 acceptance lane)
	$(call run_verification_step,run-concurrent-ui-verification-lanes,Concurrent UI Coverage + UI Browser Integration lanes)
	$(call run_verification_step,test-ui-storybook-integration,UI Storybook Integration lane)
	$(call run_verification_step,test-ui-durable-session-real-backend,UI Backend Integration lane)
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
	$(MAKE) test-root-process-acceptance
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
	cd ui && $(NPM) run build
else
	cd ui && $(UI_SCRIPT) build
endif

# Build each publishable UI package once, in dependency order. The dashboard
# consumes package source during its Vite build, while package dist/ artifacts
# are required for local package consumers and release checks.
ui-package-client-build: interfaces-ui-client
	cd ui/packages/client && $(UI_SCRIPT) build

ui-package-components-build: ui-deps
	cd ui/packages/components && $(UI_SCRIPT) build

ui-package-emulator-build: interfaces-ui-emulator ui-package-client-build
	cd ui/packages/factory-emulator && $(UI_SCRIPT) build

ui-package-replay-build: ui-package-client-build
	cd ui/packages/factory-replay && $(UI_SCRIPT) build

ui-package-visualizers-build: ui-package-client-build ui-package-components-build ui-package-replay-build
	cd ui/packages/factory-visualizers && $(UI_SCRIPT) build:current

ui-packages-build: ui-package-client-build ui-package-components-build ui-package-emulator-build ui-package-replay-build ui-package-visualizers-build

ui-dashboard-build: interfaces-ui
	$(MAKE) ui-build

# Rebuilds every UI artifact: generated contracts, package dist/ outputs, and
# the dashboard's distributable Vite bundle.
ui-build-all: ui-packages-build ui-dashboard-build

# Produces the complete application from canonical interface sources before
# compiling both the dashboard and the Go CLI.
build-all: interfaces-all ui-build-all
	$(MAKE) build

ui-test:
	cd ui && $(UI_SCRIPT) test:unit

ui-component-test:
	cd ui && $(UI_SCRIPT) test:component

ui-performance-test:
	cd ui && $(UI_SCRIPT) test:performance

ui-integration-test:
ifeq ($(BUN_BIN),)
	cd ui && $(NPM) run test:integration
else
	cd ui && $(UI_SCRIPT) test:integration
endif

ui-storybook-integration-test:
	$(MAKE) ui-storybook
	$(MAKE) ui-test-storybook-browser-checks

ui-durable-session-real-backend-integration-test:
	$(GO) test ./tests/functional/internal/support/cmd/browser_api_harness
ifeq ($(BUN_BIN),)
	cd ui && $(NPM) run test:integration:durable-session-real-backend
else
	cd ui && $(UI_SCRIPT) test:integration:durable-session-real-backend
endif

ui-test-coverage:
	cd ui && $(UI_SCRIPT) test:coverage

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
