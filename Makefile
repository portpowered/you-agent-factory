BINARY_NAME := you
CMD_PATH    := ./cmd/factory/
BIN_DIR     := bin
GO          ?= go
INSTALL_DIR = $(or $(GOBIN),$(shell $(GO) env GOPATH)/bin)
NPM         ?= npm
NODE        ?= node
ifeq ($(OS),Windows_NT)
BUN_BIN     := $(shell where.exe bun >/dev/null 2>&1 && echo bun)
else
BUN_BIN     := $(shell command -v bun 2>/dev/null)
endif
BUN_INSTALL := $(BUN_BIN) install
BUN_PACKAGE_DIRS := ui/packages/components ui
UI_SCRIPT   := $(if $(BUN_BIN),$(BUN_BIN) run,$(NPM) run)
UI_EXEC     := $(if $(BUN_BIN),$(BUN_BIN) x,$(NPM) exec)
UI_INSTALL  := $(if $(BUN_BIN),$(BUN_BIN) install,$(NPM) install --no-package-lock)
FUNCTIONAL_DEFAULT_PACKAGES := ./tests/functional/...

# Keep the default Go work claimed by each factory lane bounded when several
# lanes share one host. GO_LANE_BUDGET is max(2, logical CPUs /
# YOU_EXPECTED_CONCURRENT_LANES). Both inputs are overridable for controlled
# probes and CI-specific capacity, while invalid detected values safely select
# two jobs.
YOU_EXPECTED_CONCURRENT_LANES ?= 4
ifeq ($(OS),Windows_NT)
YOU_LOGICAL_CPUS ?= $(strip $(NUMBER_OF_PROCESSORS))
else
YOU_LOGICAL_CPUS ?= $(strip $(shell getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.logicalcpu 2>/dev/null || nproc 2>/dev/null))
endif

define decimal_remainder
$(strip $(subst 0,,$(subst 1,,$(subst 2,,$(subst 3,,$(subst 4,,$(subst 5,,$(subst 6,,$(subst 7,,$(subst 8,,$(subst 9,,$(1))))))))))))
endef

define nonzero_decimal
$(strip $(subst 0,,$(1)))
endef

define positive_decimal
$(if $(strip $(1)),$(if $(call decimal_remainder,$(1)),,$(if $(call nonzero_decimal,$(1)),$(strip $(1)),)),)
endef

ifeq ($(OS),Windows_NT)
ifneq (,$(or $(findstring /sh,$(SHELL)),$(findstring /bash,$(SHELL))))
# GNU Make uses the effective SHELL for $(shell), so use POSIX arithmetic when
# Windows Make is running through sh.exe or bash.exe. Calling cmd.exe here
# makes MSYS shells return the cmd banner and prompt instead of the result.
define compute_go_lane_budget
$(strip $(shell budget=$$(expr $(1) / $(2) 2>/dev/null); if test "$$budget" -ge 2 2>/dev/null; then printf '%s' "$$budget"; elif test "$$budget" = 0 || test "$$budget" = 1; then printf '2'; fi))
endef
else
# The native Windows Make shell is cmd.exe when no POSIX shell is selected.
# Keep the command's raw stdout for the validation guard below.
define compute_go_lane_budget
$(strip $(shell cmd.exe /d /v:on /c "set /a result=$(1)/$(2) >nul & if !result! LSS 2 (echo 2) else (echo !result!)"))
endef
endif
else
define compute_go_lane_budget
$(strip $(shell budget=$$(expr $(1) / $(2) 2>/dev/null); if test "$$budget" -ge 2 2>/dev/null; then printf '%s' "$$budget"; elif test "$$budget" = 0 || test "$$budget" = 1; then printf '2'; fi))
endef
endif

ifndef GO_LANE_BUDGET
ifneq ($(call positive_decimal,$(YOU_LOGICAL_CPUS)),)
ifneq ($(call positive_decimal,$(YOU_EXPECTED_CONCURRENT_LANES)),)
GO_LANE_BUDGET_COMPUTED ?= $(call compute_go_lane_budget,$(YOU_LOGICAL_CPUS),$(YOU_EXPECTED_CONCURRENT_LANES))
ifneq ($(call positive_decimal,$(GO_LANE_BUDGET_COMPUTED)),)
GO_LANE_BUDGET := $(call positive_decimal,$(GO_LANE_BUDGET_COMPUTED))
else
$(warning GO_LANE_BUDGET received invalid computed value '$(GO_LANE_BUDGET_COMPUTED)'; using 2)
GO_LANE_BUDGET := 2
endif
else
GO_LANE_BUDGET := 2
endif
else
GO_LANE_BUDGET := 2
endif
endif

ifdef GO_LANE_BUDGET
GO_LANE_BUDGET_VALIDATED := $(call positive_decimal,$(GO_LANE_BUDGET))
ifneq ($(GO_LANE_BUDGET_VALIDATED),)
override GO_LANE_BUDGET := $(GO_LANE_BUDGET_VALIDATED)
else
$(warning GO_LANE_BUDGET received invalid override '$(GO_LANE_BUDGET)'; using 2)
override GO_LANE_BUDGET := 2
endif
endif

FUNCTIONAL_DEFAULT_JOBS ?= $(GO_LANE_BUDGET)
UNIT_DEFAULT_JOBS ?= $(GO_LANE_BUDGET)
FUNCTIONAL_LONG_TAGS ?= functionallong
FUNCTIONAL_LONG_PACKAGES := ./tests/functional/...
STRESS_DEFAULT_PACKAGES := ./tests/stress/...
RELEASE_DEFAULT_PACKAGES := ./tests/release/...
SCRIPT_TIMEOUT_COMPANION_SMOKE_TEST := TestProviderCancellationTerminatesCompanionProcesses
SCRIPT_TIMEOUT_COMPANION_SMOKE_COUNT ?= 100
SCRIPT_TIMEOUT_COMPANION_SMOKE_TIMEOUT ?= 120s
CRON_TIME_WORK_SMOKE_TEST := TestCronFiresAtInjectedTimeWithoutWallClockSleep
CRON_TIME_WORK_SMOKE_COUNT ?= 10
CRON_TIME_WORK_SMOKE_TIMEOUT ?= 120s
CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TEST := TestCurrentFactoryActivationSwitchesPersistedFactories
CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_COUNT ?= 1
CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TIMEOUT ?= 120s
JAVASCRIPT_CONTRACT_SMOKE_TIMEOUT ?= 120s
CONFIG_CONTRACT_SMOKE_TIMEOUT ?= 120s
JAVASCRIPT_RUNTIME_REGRESSION_TESTS ?= ^(TestCallBehavior_WorkflowFinalInventoryMatchesExecution|TestCallBehavior_AgentRunInventoryMatchesExecution|TestRun_ProgressPrimitives_EmitsOrderedRuntimeRecords|TestRun_PolicyDeniedChildOperations_ReturnStableDiagnostics|TestCallBehavior_WorkflowResumeStateInventoryMatchesExecution)$$
RESPONSE_STREAM_STRESS_SMOKE_TEST := TestSessionResponseEventStore_Backpressure
RESPONSE_STREAM_STRESS_SMOKE_TIMEOUT ?= 120s
ROOT_PROCESS_ACCEPTANCE_PACKAGES := ./tests/functional/acceptance ./tests/functional/recordings/process
ROOT_PROCESS_ACCEPTANCE_TIMEOUT ?= 300s

ifeq ($(OS),Windows_NT)
	BINARY_NAME := you.exe
endif

# Go's build cache is content-addressed and already includes the inputs that
# determine a package's compiled output. Local CLI builds do not consume Go's
# repository VCS metadata, so skip that probe by default. Keep this option
# separate from GO_BUILD_FLAGS so callers can restore stamping explicitly with
# GO_LOCAL_BUILD_FLAGS=-buildvcs=true without changing other build flags.
GO_BUILD_FLAGS ?=
GO_LOCAL_BUILD_FLAGS ?= -buildvcs=false

GO_TEST_TIMEOUT ?= 300s
GO_COVERAGE_TIMEOUT ?= 10m
GO_COVERAGE_MIN ?= 75.9
GO_COVERAGE_FLOOR_POLICY ?= blocking
GO_UNIT_COVERAGE_MIN ?= $(GO_COVERAGE_MIN)
GO_FUNCTIONAL_COVERAGE_MIN ?= 33.1
GO_UNIT_COVERAGE_MANIFEST ?= docs/internal/baselines/go-unit-coverage-package-minimums.json
GO_FUNCTIONAL_COVERAGE_MANIFEST ?= docs/internal/baselines/go-functional-coverage-package-minimums.json
FUNCTIONAL_QUARANTINE ?= tests/functional/functional-quarantine.json
FUNCTIONAL_TEST_TIER ?= pr-short
FUNCTIONAL_TEST_TRIGGER ?= local
FUNCTIONAL_TEST_BUDGET ?= 35m
FUNCTIONAL_SHORT ?= true
CHANGED_TEST_STABILITY_BASE ?=
CHANGED_TEST_STABILITY_HEAD ?= HEAD
CHANGED_TEST_STABILITY_ATTEMPTS ?= 3
CHANGED_TEST_STABILITY_BUDGET ?= 15m
CHANGED_TEST_STABILITY_JOBS ?= 4
UNIT_COVERAGE_DIR ?= .artifacts/unit-coverage
GO_UNIT_COVERAGE_PROFILE ?= $(UNIT_COVERAGE_DIR)/coverage.out
GO_UNIT_COVERAGE_JSON_OUTPUT ?= $(UNIT_COVERAGE_DIR)/coverage-summary.json
GO_UNIT_COVERAGE_TIMING_OUTPUT ?= $(UNIT_COVERAGE_DIR)/unit-timing-summary.json
GO_UNIT_COVERAGE_LOG ?= $(UNIT_COVERAGE_DIR)/command.log
UNIT_COVERAGE_GO ?= $(GO)
GO_FUNCTIONAL_COVERAGE_PROFILE ?=
GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT ?=
GO_FUNCTIONAL_COVERAGE_TIMING_OUTPUT ?=
FUNCTIONAL_TEST_VIZ_DIR ?= .artifacts/functional-test-viz
FUNCTIONAL_TEST_VIZ_PROFILE ?= $(FUNCTIONAL_TEST_VIZ_DIR)/coverage.out
FUNCTIONAL_TEST_VIZ_JSON ?= $(FUNCTIONAL_TEST_VIZ_DIR)/coverage-summary.json
FUNCTIONAL_TEST_VIZ_TIMING ?= $(FUNCTIONAL_TEST_VIZ_DIR)/functional-timing-summary.json
FUNCTIONAL_TEST_VIZ_MARKDOWN ?= $(FUNCTIONAL_TEST_VIZ_DIR)/functional-tests.md
FUNCTIONAL_TEST_VIZ_LOG ?= $(FUNCTIONAL_TEST_VIZ_DIR)/command.log
FUNCTIONAL_COVERAGE_VERDICT_FILE ?= $(FUNCTIONAL_TEST_VIZ_DIR)/functional-coverage-verdict.txt
FUNCTIONAL_TEST_GO ?= $(GO)
# Hosted CI sets this optional handoff so an ordinary gocoveragecheck failure
# can be reported by its compact terminal verdict step. An unset path preserves
# the historical fail-fast target behavior.
FUNCTIONAL_GOCOVERAGE_EXIT_FILE ?=
BACKEND_SIZE_ROOT ?= .
PACKAGE_MAINT_ROOT ?= .
PACKAGE_FILE_COUNT_ROOT ?= .
PACKAGE_BOUNDARY_ROOT ?= .
PACKAGE_BOUNDARY_ALL ?= 0
PACKAGE_BOUNDARY_BASE_REF ?=
PACKAGE_STRUCTURE_ROOT ?= .
BACKEND_DEPENDENCY_GRAPH_DIR ?= .artifacts/backend-dependency-graph
BACKEND_DEPENDENCY_GRAPH_DOT ?= $(BACKEND_DEPENDENCY_GRAPH_DIR)/backend-dependency-graph.dot
BACKEND_DEPENDENCY_GRAPH_SVG ?= $(BACKEND_DEPENDENCY_GRAPH_DIR)/backend-dependency-graph.svg
COMPATIBILITY_ALIAS_CHECK_ROOT ?= .
RETIRED_SURFACE_CHECK_ROOT ?= .
LINT_CHECKER_CACHE_DIR ?= .cache/lint-checkers
# Set LINT_CHECKER_FALLBACK=1 to use the original go run path for one proof.
LINT_CHECKER_FALLBACK ?= 0
LINT_CHECKER_DRIVER_PACKAGE := ./cmd/lintcheck
LINT_CHECKER_DRIVER ?=
LINT_LANE_PACKAGE := ./cmd/lintlane
LINT_JOBS ?= $(GO_LANE_BUDGET)
# Keep the recursive command available to lintlane without spelling the
# special $(MAKE) variable in this recipe; GNU Make executes such recipes
# during -n so recursive builds can receive the dry-run flag.
LINT_MAKE ?= $(MAKE)
LINT_REPORT_FILE ?=
LINT_TARGETS ?= ui-lint ui-deadcode vet backend-size pkg-maint pkg-file-count pkg-boundary pkg-structure service-cycle-check package-target-manifest-check packaged-factory-source-check packaged-factory-consumption-check packaged-factory-catalog-check provider-catalog-check model-provider-package-check durable-runtime-construction-check logging-boundary-check compatibility-alias-check retired-surface-check ownership-inventory-check deadcode fmt-check contracts-check

define run_lint_checker
$(if $(LINT_CHECKER_DRIVER),"$(LINT_CHECKER_DRIVER)",$(GO) run $(LINT_CHECKER_DRIVER_PACKAGE)) -cache-dir "$(LINT_CHECKER_CACHE_DIR)" -go "$(GO)" $(if $(filter 1 true yes,$(LINT_CHECKER_FALLBACK)),-fallback,) -package "$(1)" -- $(2)
endef

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


.PHONY: default default-pipeline-banner build install bundle-api print-go-parallelism
.PHONY: fmt fmt-check vet deps deps-tidy clean init typecheck release lint

.PHONY: test test-full test-unit test-unit-fresh test-ci-workflows test-lane-audit test-maintenance test-integration test-contract test-stress test-release
.PHONY: test-functional test-functional-fresh test-functional-long test-functional-long-compile test-backend-functional functional-boundary-check functional-test-viz
.PHONY: test-ui-browser-integration test-ui-storybook-integration test-ui-durable-session-real-backend test-ui-performance ui-component-test
.PHONY: test-unit-coverage test-functional-coverage coverage-help test-backend-coverage test-coverage-go test-race
.PHONY: test-backend-verification test-root-process-acceptance long-tests long-tests-managed-runtime
.PHONY: frontend-verification backend-verification ui-backend-integration

.PHONY: verify-fast verify-pr verify-extended verify-build verify-lint verify-api
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

.PHONY: script-timeout-companion-smoke-100 cron-time-work-smoke current-factory-watcher-switch-smoke javascript-contract-smoke config-contract-smoke
.PHONY: backend-size pkg-maint pkg-file-count pkg-boundary pkg-structure service-cycle-check package-target-manifest-check packaged-factory-source-check packaged-factory-consumption-check packaged-factory-catalog-generate packaged-factory-catalog-check provider-catalog-generate provider-catalog-check model-provider-package-generate model-provider-package-check durable-runtime-construction-check logging-boundary-check ownership-inventory-check
.PHONY: response-stream-stress-smoke release-surface-smoke artifact-contract-closeout
.PHONY: compatibility-alias-check retired-surface-check readme-check deadcode dashboard-verify

.PHONY: ci ci-typecheck ci-verify-build-contracts ci-verify-tests

.PHONY: ui-deps ui-lint ui-build ui-test ui-performance-test ui-integration-test ui-storybook-integration-test ui-durable-session-real-backend-integration-test ui-test-coverage ui-replay-coverage-check ui-install-playwright
.PHONY: ui-test-storybook ui-components-typecheck ui-components-test ui-components-storybook ui-components-boundary ui-components-dependency-direction ui-components-verify ui-verify-fresh-npm-install
.PHONY: ui-public-package-release ui-public-package-publish-prepare
.PHONY: ui-storybook  ui-deadcode
.PHONY: ui-package-client-build ui-package-components-build ui-package-emulator-build ui-package-replay-build ui-package-visualizers-build ui-packages-build ui-dashboard-build ui-build-all build-all


# Keep the default pipeline as an ordinary ordered dependency graph. Recipe-
# level $(MAKE) invocations are special to GNU Make and execute during -n so
# that recursive builds can receive the dry-run flag; on the measured Windows
# Make implementation that still launched real work. These aggregators remain
# serialized to preserve the old stop-on-failure behavior even with -j.
.NOTPARALLEL: default test
# Bare `make` runs the complete generation, frontend, build, test, and lint
# pipeline. Use `make build` when only the Go binary is needed.
default: default-pipeline-banner generate-api ui-deps ui-build build test lint

default-pipeline-banner:
	@echo "Bare make runs: generate-api, ui-deps, ui-build, build, test, lint."
	@echo "For only the Go binary, run: make build."

# Print the derived values without starting a toolchain process. This is useful
# for controlled host-capacity probes, for example:
#   make -s print-go-parallelism YOU_LOGICAL_CPUS=24 YOU_EXPECTED_CONCURRENT_LANES=4
print-go-parallelism:
	@echo "YOU_LOGICAL_CPUS=$(YOU_LOGICAL_CPUS) YOU_EXPECTED_CONCURRENT_LANES=$(YOU_EXPECTED_CONCURRENT_LANES)"
	@echo "GO_LANE_BUDGET=$(GO_LANE_BUDGET)"
	@echo "UNIT_DEFAULT_JOBS=$(UNIT_DEFAULT_JOBS)"
	@echo "FUNCTIONAL_DEFAULT_JOBS=$(FUNCTIONAL_DEFAULT_JOBS)"
	@echo "LINT_JOBS=$(LINT_JOBS)"
	@echo "UNITLANE_DEFAULT_JOBS=$(GO_LANE_BUDGET)"

# Local pre-push guidance: run both independent Go coverage reports before
# pushing. A build or typecheck does not substitute for either completed run.
coverage-help:
	$(info Local pre-push checks: run both independent Go coverage reports before pushing.)
	$(info   make test-unit-coverage       backend unit/package coverage report)
	$(info   make test-functional-coverage independent functional coverage report)
	$(info A build or typecheck alone does not replace either completed coverage run.)

build:
	$(GO) build $(GO_BUILD_FLAGS) $(GO_LOCAL_BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_PATH)

install:
	$(GO) build $(GO_BUILD_FLAGS) $(GO_LOCAL_BUILD_FLAGS) -o $(INSTALL_DIR)/$(BINARY_NAME) $(CMD_PATH)

bundle-api:
	$(NODE) scripts/run-quiet-api-command.js bundle:rest ./api/openapi-main.yaml ./api/openapi.yaml

generate-api: bundle-api generate-go-api generate-ui-api

generate-go-api: generate-go-server-api generate-go-client-api

generate-go-server-api:
	$(GO) generate -run=server -tags=interfaces ./pkg/transports/http

generate-go-client-api:
	$(GO) generate -run=client -tags=interfaces ./pkg/transports/http

generate-ui-api:
	cd ui && $(NODE) ./scripts/generate-openapi-types.mjs ../api/openapi.yaml src/api/generated/openapi.ts

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
	$(GO) test ./tests/functional/factory/definitions -run '^TestFactoryValidationDocsCommandDescribesStaticGate$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

readme-check:
	$(GO) run ./cmd/readmecheck

test: test-unit test-ci-workflows

test-ci-workflows:
	$(NODE) --test scripts/default-pipeline.test.mjs scripts/development-package-workflow.test.mjs scripts/verification-policy.test.mjs scripts/ci/lane-budget.test.mjs scripts/ci/backend-integration-workflow.test.mjs scripts/ci/backend-lint-report.test.mjs scripts/ci/backend-lint-workflow.test.mjs scripts/ci/functional-coverage-comment.test.mjs scripts/ci/functional-coverage-verdict.test.mjs scripts/ci/unit-coverage-report.test.mjs scripts/ci/workflow-lint.test.mjs scripts/localai-backend-artifact-workflow.test.mjs

test-full:
	$(GO) test ./... -timeout $(GO_TEST_TIMEOUT)

test-unit:
	$(GO) run ./cmd/unitlane -jobs $(UNIT_DEFAULT_JOBS) -timeout $(GO_TEST_TIMEOUT)

test-unit-fresh:
	$(GO) run ./cmd/unitlane -jobs $(UNIT_DEFAULT_JOBS) -count=1 -timeout $(GO_TEST_TIMEOUT)

# Merge-base-aware changed-test flake prevention. The caller must provide the
# pull-request base ref/SHA; the command resolves its merge-base with head,
# then gives every selected top-level Go test 3 isolated attempts within one
# 15-minute total budget.
test-changed-test-stability:
	$(GO) run ./cmd/teststability -base "$(CHANGED_TEST_STABILITY_BASE)" -head "$(CHANGED_TEST_STABILITY_HEAD)" -attempts $(CHANGED_TEST_STABILITY_ATTEMPTS) -budget $(CHANGED_TEST_STABILITY_BUDGET) -jobs $(CHANGED_TEST_STABILITY_JOBS)

test-lane-audit:
	$(GO) run ./cmd/testlanecheck

test-maintenance:
	$(MAKE) test-lane-audit
	$(GO) test -short -p=$(UNIT_DEFAULT_JOBS) ./cmd/... ./internal/... ./packages/model-providers ./packages/packaged-factories ./tests/functional/internal/... ./ui ./pkg/services/factory_runtime/internal/exhaustiontests -count=1 -timeout $(GO_TEST_TIMEOUT)

test-integration:
	$(GO) test -short ./tests/integration -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test -short ./tests/integration/harness -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test -short -p=$(UNIT_DEFAULT_JOBS) ./pkg/services/factory_definitions/internal/services/compilation/runtimetests ./pkg/services/factory_definitions/internal/services/catalog/persistence/integrationtests ./pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig/integrationtests ./pkg/services/factory_sessions/internal/execution/fixtures ./pkg/transports/http/servertests/... ./tests/integration/factory/visualization/runtime_metrics ./tests/integration/transport/cli/process ./tests/integration/transport/server_binding -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/services/automations/internal/services/filesystem_watchers/internal/service -run '^TestFileWatcher_' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/platform/process -run '^TestExecCommandRunner_' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/services/workers/internal/worktree -run '^TestPrepareFactoryGitWorktree_(CreatesWorktreeWhenMissing|ReusesExistingValidWorktree|UsesExistingWorktreesParent|ReturnsFailureWhenWorktreeAddFails|ReturnsFailureWhenPathExistsButIsNotWorktree)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/services/providers/internal/services/execution/internal/adapters/claude -run '^TestClaudeCommandEnvironmentPreventsGitMergeEditorPrompt$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

test-contract:
	$(GO) test -short -p=$(UNIT_DEFAULT_JOBS) ./contracts ./pkg/services/factory_definitions/internal/contracts/contracttests ./pkg/transports/http/contracttests ./pkg/transports/cli/baseline ./pkg/transports/cli/clicontract ./pkg/transports/cli/cliinputs ./pkg/transports/cli/climanifestgen ./pkg/transports/cli/commandidentity -count=1 -timeout $(GO_TEST_TIMEOUT)

# Cache-aware developer feedback; use test-functional-fresh for an
# unconditional rerun.
test-functional:
	$(MAKE) functional-boundary-check
	$(GO) run ./cmd/functionallane -jobs $(FUNCTIONAL_DEFAULT_JOBS) -timeout $(GO_TEST_TIMEOUT)

# CI-equivalent and flake-investigation path: force every package to execute.
test-functional-fresh:
	$(MAKE) functional-boundary-check
	$(GO) run ./cmd/functionallane -jobs $(FUNCTIONAL_DEFAULT_JOBS) -count=1 -timeout $(GO_TEST_TIMEOUT)

functional-boundary-check:
	$(GO) run ./cmd/functionalboundarycheck

# functional-test-viz is the single functional-report entrypoint. It runs the
# configured fresh functional coverage tier exactly once (including its
# boundary check), renders and publishes the Markdown catalog when running in
# GitHub Actions, retains the complete command stream in command.log, and prints
# only pkg/ coverage plus functional-package latencies to the terminal. Artifacts
# land under .artifacts/functional-test-viz/.
#
# Fail-closed composition: boundary, suite, coverage-floor, metadata/inventory,
# console-summary, or Markdown rendering failures exit non-zero. The target
# never deletes the artifact root on failure, so already-written diagnostics
# remain inspectable.
# gocoveragecheck writes -json-output after a completed measurement even when a
# floor fails. Without FUNCTIONAL_GOCOVERAGE_EXIT_FILE, Make stops before
# Markdown so the failure stays non-zero; hosted CI sets that path to hand an
# ordinary exit-1 outcome to its compact verdict step.
functional-test-viz:
	@$(GO) run ./cmd/functionaltestviz \
		-run-suite \
		-go "$(FUNCTIONAL_TEST_GO)" \
		-root . \
		-coverage-summary "$(FUNCTIONAL_TEST_VIZ_JSON)" \
		-timing-summary "$(FUNCTIONAL_TEST_VIZ_TIMING)" \
		-output "$(FUNCTIONAL_TEST_VIZ_MARKDOWN)" \
		-log "$(FUNCTIONAL_TEST_VIZ_LOG)" \
		-profile "$(FUNCTIONAL_TEST_VIZ_PROFILE)" \
		-verdict "$(FUNCTIONAL_COVERAGE_VERDICT_FILE)" \
		-exit-code-file "$(FUNCTIONAL_GOCOVERAGE_EXIT_FILE)" \
		-tier "$(FUNCTIONAL_TEST_TIER)" \
		-trigger "$(FUNCTIONAL_TEST_TRIGGER)" \
		-budget "$(FUNCTIONAL_TEST_BUDGET)" \
		-short=$(FUNCTIONAL_SHORT) \
		-quarantine "$(FUNCTIONAL_QUARANTINE)" \
		-jobs $(FUNCTIONAL_DEFAULT_JOBS) \
		-minimum $(GO_FUNCTIONAL_COVERAGE_MIN) \
		-package-manifest "$(GO_FUNCTIONAL_COVERAGE_MANIFEST)" \
		-package-floor-policy "$(GO_COVERAGE_FLOOR_POLICY)" \
		-test-timeout "$(GO_COVERAGE_TIMEOUT)"

test-stress:
	$(GO) test -short $(STRESS_DEFAULT_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)

test-release:
	$(GO) test -short $(RELEASE_DEFAULT_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)

test-functional-long:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) $(FUNCTIONAL_LONG_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)

# Compile every functionallong-tagged functional package without running tests
# or starting any of the long-test runtime dependencies.
test-functional-long-compile:
	$(GO) vet -tags=$(FUNCTIONAL_LONG_TAGS) $(FUNCTIONAL_LONG_PACKAGES)

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

# Focused classifier lanes. Each target owns only its product verification
# scope and composes the existing checks so broader verification entry points
# retain their current behavior.
frontend-verification:
	$(MAKE) typecheck
	$(MAKE) ui-lint
	$(MAKE) ui-component-test
	$(MAKE) test-ui-coverage
	$(MAKE) test-ui-browser-integration
	$(MAKE) test-ui-storybook-integration
	$(MAKE) ui-public-package-release

backend-verification:
	$(MAKE) build
	$(MAKE) test-backend-verification

# This lane is intentionally narrower than the general UI browser lane: it
# runs the browser coverage that starts and calls the real backend, without
# Storybook-only checks.
ui-backend-integration:
	$(MAKE) ui-durable-session-real-backend-integration-test

ACP_BASELINE_DIR       ?= docs/internal/projects/acp-program/baselines
ACP_BASELINE_ARTIFACTS ?= .artifacts/acp-baseline

.PHONY: acp-baseline-self acp-baseline-capture acp-baseline-compare acp-baseline-check

# Capture our own `you server acp` through the shared scenario scripts. Needs no
# external agent and no provider credentials, so it is safe anywhere.
acp-baseline-self:
	$(GO) build -o $(ACP_BASELINE_ARTIFACTS)/you ./cmd/factory
	$(GO) run ./cmd/acpbaseline capture -agent '$(ACP_BASELINE_ARTIFACTS)/you server acp' \
		-name you -out $(ACP_BASELINE_ARTIFACTS) -publish $(ACP_BASELINE_DIR)/you/$(shell date -u +%Y-%m-%d)

# Capture a third-party ACP agent. Requires that agent installed and
# authenticated; exits 3 with instructions when it is not. Raw transcripts hold
# full prompt and response content and stay under .artifacts (gitignored).
#   make acp-baseline-capture ACP_AGENT='cursor-agent acp' ACP_BASELINE_NAME=cursor-agent
acp-baseline-capture:
	$(GO) run ./cmd/acpbaseline capture -agent '$(ACP_AGENT)' -name '$(ACP_BASELINE_NAME)' \
		-out $(ACP_BASELINE_ARTIFACTS) -publish $(ACP_BASELINE_DIR)/$(ACP_BASELINE_NAME)/$(shell date -u +%Y-%m-%d)

acp-baseline-compare:
	$(GO) run ./cmd/acpbaseline compare \
		$(foreach m,$(wildcard $(ACP_BASELINE_DIR)/*/*/capability-matrix.json),-matrix $(m)) \
		-out $(ACP_BASELINE_DIR)/comparison-matrix.md

# Commit guard: committed baselines are digested, secret-free, and in budget.
acp-baseline-check:
	$(GO) run ./cmd/acpbaseline verify -dir $(ACP_BASELINE_DIR)

long-tests:
	$(info Running opt-in long and specialty suites: UI performance + managed runtime coverage)
	$(call run_verification_step,test-ui-performance,UI Performance specialty lane)
	$(call run_verification_step,long-tests-managed-runtime,Managed Runtime specialty lane)

long-tests-managed-runtime:
	$(GO) test ./pkg/services/models/internal/local -run '^TestOmniVoiceLocalRuntime_' -count=1 -timeout $(GO_TEST_TIMEOUT)

test-coverage-go:
	$(info make test-coverage-go is a compatibility alias for unit coverage; use make test-functional-coverage for the independent functional report.)
	$(MAKE) test-unit-coverage

# test-unit-coverage is the single unit coverage entrypoint. Its Go runner owns
# artifact cleanup, retains the complete checker stream in command.log, and
# prints only pkg/ coverage plus package latencies. gocoveragecheck's exit code
# remains the sole pass/fail signal. Floor policy defaults to blocking for local
# callers; hosted CI sets GO_COVERAGE_FLOOR_POLICY=advisory for both lanes.
test-unit-coverage:
	@$(GO) run ./cmd/unitcoverage \
		-go "$(UNIT_COVERAGE_GO)" \
		-root . \
		-minimum $(GO_UNIT_COVERAGE_MIN) \
		-package-manifest "$(GO_UNIT_COVERAGE_MANIFEST)" \
		-package-floor-policy "$(GO_COVERAGE_FLOOR_POLICY)" \
		-test-timeout "$(GO_COVERAGE_TIMEOUT)" \
		-profile "$(GO_UNIT_COVERAGE_PROFILE)" \
		-coverage-summary "$(GO_UNIT_COVERAGE_JSON_OUTPUT)" \
		-timing-summary "$(GO_UNIT_COVERAGE_TIMING_OUTPUT)" \
		-log "$(GO_UNIT_COVERAGE_LOG)"

# test-functional-coverage always runs functional-boundary-check first so the
# required CI Backend Functional Coverage lane (and any local/alias caller of
# this target) cannot succeed without a successful boundary check. gocoveragecheck
# forces -count=1 for its instrumented run, so this target remains fresh even
# though the ordinary developer lane is cache-aware. Boundary failures exit
# non-zero before gocoveragecheck starts.
test-functional-coverage:
	$(MAKE) functional-boundary-check
	@echo "Functional tier: name=$(FUNCTIONAL_TEST_TIER) trigger=$(FUNCTIONAL_TEST_TRIGGER) short=$(FUNCTIONAL_SHORT) budget=$(FUNCTIONAL_TEST_BUDGET) selection=subtractive quarantine=$(FUNCTIONAL_QUARANTINE)"
	@set +e; \
	$(GO) run ./cmd/gocoveragecheck -suite functional -stream -jobs $(FUNCTIONAL_DEFAULT_JOBS) -min $(GO_FUNCTIONAL_COVERAGE_MIN) -package-manifest $(GO_FUNCTIONAL_COVERAGE_MANIFEST) -package-floor-policy $(GO_COVERAGE_FLOOR_POLICY) -functional-quarantine $(FUNCTIONAL_QUARANTINE) -timeout $(GO_COVERAGE_TIMEOUT) $(if $(filter false 0 no,$(FUNCTIONAL_SHORT)),-short=false,) $(if $(GO_FUNCTIONAL_COVERAGE_PROFILE),-profile $(GO_FUNCTIONAL_COVERAGE_PROFILE),) $(if $(GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT),-json-output $(GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT),) $(if $(GO_FUNCTIONAL_COVERAGE_TIMING_OUTPUT),-timing-output $(GO_FUNCTIONAL_COVERAGE_TIMING_OUTPUT),); \
	status=$$?; \
	if [ -n "$(FUNCTIONAL_GOCOVERAGE_EXIT_FILE)" ]; then \
		printf '%s\n' "$$status" > "$(FUNCTIONAL_GOCOVERAGE_EXIT_FILE)"; \
		if [ "$$status" -eq 1 ]; then exit 0; fi; \
	fi; \
	exit "$$status"

script-timeout-companion-smoke-100:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/workers/inference -run $(SCRIPT_TIMEOUT_COMPANION_SMOKE_TEST) -count=$(SCRIPT_TIMEOUT_COMPANION_SMOKE_COUNT) -timeout $(SCRIPT_TIMEOUT_COMPANION_SMOKE_TIMEOUT)

cron-time-work-smoke:
	$(GO) test ./tests/functional/workstations/cron -run $(CRON_TIME_WORK_SMOKE_TEST) -count=$(CRON_TIME_WORK_SMOKE_COUNT) -timeout $(CRON_TIME_WORK_SMOKE_TIMEOUT)

current-factory-watcher-switch-smoke:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/factory/current -run $(CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TEST) -count=$(CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_COUNT) -timeout $(CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TIMEOUT)

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
	$(GO) run $(LINT_LANE_PACKAGE) -make "$(LINT_MAKE)" -jobs "$(LINT_JOBS)" -go "$(GO)" -cache-dir "$(LINT_CHECKER_CACHE_DIR)" $(if $(LINT_REPORT_FILE),-report-file "$(LINT_REPORT_FILE)",) $(if $(LINT_CHECKER_DRIVER),-checker-driver "$(LINT_CHECKER_DRIVER)",-checker-package "$(LINT_CHECKER_DRIVER_PACKAGE)") -- $(LINT_TARGETS)

backend-size:
	$(call run_lint_checker,./cmd/backendsizecheck,-root "$(BACKEND_SIZE_ROOT)")

backend-dependency-graph:
	$(GO) run ./cmd/backenddependencygraph -root . -go $(GO) -output $(BACKEND_DEPENDENCY_GRAPH_DOT) -svg-output $(BACKEND_DEPENDENCY_GRAPH_SVG)

pkg-maint:
	$(call run_lint_checker,./cmd/pkgmaintcheck,-root "$(PACKAGE_MAINT_ROOT)")

pkg-file-count:
	$(call run_lint_checker,./cmd/pkgfilecountcheck,-root "$(PACKAGE_FILE_COUNT_ROOT)")

pkg-boundary:
	$(call run_lint_checker,./cmd/pkgboundarycheck,-root "$(PACKAGE_BOUNDARY_ROOT)" $(if $(strip $(PACKAGE_BOUNDARY_BASE_REF)),-base-ref "$(PACKAGE_BOUNDARY_BASE_REF)",) $(if $(filter 1 true yes,$(PACKAGE_BOUNDARY_ALL)),--all,))
	$(call run_lint_checker,./cmd/ownershipboundarycheck,)

pkg-structure:
	$(call run_lint_checker,./cmd/pkgstructurecheck,-root "$(PACKAGE_STRUCTURE_ROOT)")

service-cycle-check:
	$(call run_lint_checker,./cmd/servicecyclecheck,-root ".")

package-target-manifest-check:
	$(call run_lint_checker,./cmd/packagetargetmanifestcheck,-root ".")

packaged-factory-source-check:
	$(call run_lint_checker,./cmd/packagedfactorysourcecheck,-root ".")

packaged-factory-consumption-check:
	$(call run_lint_checker,./cmd/packagedfactoryconsumptioncheck,-root ".")

packaged-factory-catalog-generate:
	$(GO) run ./cmd/packagedfactorycataloggenerate -root .

packaged-factory-catalog-check:
	$(call run_lint_checker,./cmd/packagedfactorycatalogcheck,-root ".")

provider-catalog-generate:
	$(GO) run ./cmd/providercataloggenerate -root .

provider-catalog-check:
	$(call run_lint_checker,./cmd/providercatalogcheck,-root ".")

model-provider-package-generate:
	node scripts/model-provider-package.mjs generate

model-provider-package-check:
	node scripts/model-provider-package.mjs check

ownership-boundary-check:
	$(call run_lint_checker,./cmd/ownershipboundarycheck,)

ownership-inventory-check:
	$(call run_lint_checker,./cmd/ownershipinventorycheck,)

durable-runtime-construction-check:
	$(call run_lint_checker,./cmd/durableruntimeconstructioncheck,-root ".")

logging-boundary-check:
	$(call run_lint_checker,./cmd/loggingboundarycheck,-root ".")

compatibility-alias-check:
	$(call run_lint_checker,./cmd/compatibilityaliascheck,-root "$(COMPATIBILITY_ALIAS_CHECK_ROOT)")

retired-surface-check:
	$(call run_lint_checker,./cmd/retiredsurfacecheck,-root "$(RETIRED_SURFACE_CHECK_ROOT)")

deadcode:
	$(call run_lint_checker,./cmd/deadcodecheck,)

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
	$(MAKE) test-functional-long-compile
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
	$(MAKE) test-functional-long-compile
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

fmt-check:
	@set -e; \
	paths_file="$${TMPDIR:-.}/you-gofmt-check-$$.paths"; \
	trap 'rm -f "$$paths_file"' 0 1 2 3 15; \
	git ls-files -z --cached -- 'cmd/**/*.go' 'pkg/**/*.go' 'tests/**/*.go' > "$$paths_file"; \
	violations="$$(xargs -0 $(GO)fmt -l < "$$paths_file")"; \
	if test -n "$$violations"; then \
		printf '%s\n' "$$violations"; \
		exit 1; \
	fi

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
