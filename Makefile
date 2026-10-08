BINARY_NAME := you
CMD_PATH    := ./cmd/factory/
BIN_DIR     := bin
GO          ?= go
PYTHON      ?= python
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
MANAGED_PROCESS_HELPER_SOURCE ?= ./tests/integration/models/testdata/managed_process_helper
MANAGED_PROCESS_HELPER_DIR ?= .artifacts/integration/models-managed-process
MANAGED_PROCESS_HELPER_DIGEST_FILE ?= $(MANAGED_PROCESS_HELPER_DIR)/managed_process_helper.sha256
MANAGED_PROCESS_INTEGRATION_PACKAGE ?= ./tests/integration/models/managed_process
ASR_LIVE_CORRELATION_HARNESS_DIR ?= .artifacts/integration/models-asr-live-correlation
# Keep the prebuilt helper under testdata so it is excluded from the unit package inventory.
ASR_LIVE_CORRELATION_HARNESS_SOURCE ?= ./pkg/services/models/internal/testdata/asr_live_correlation_harness
ASR_LIVE_CORRELATION_INTEGRATION_PACKAGE ?= ./tests/integration/models/asr_live_correlation

ifeq ($(OS),Windows_NT)
MANAGED_PROCESS_HELPER_DEFAULT := $(MANAGED_PROCESS_HELPER_DIR)/managed_process_helper.exe
else
MANAGED_PROCESS_HELPER_DEFAULT := $(MANAGED_PROCESS_HELPER_DIR)/managed_process_helper
endif
MANAGED_PROCESS_HELPER ?= $(MANAGED_PROCESS_HELPER_DEFAULT)

ifeq ($(OS),Windows_NT)
ASR_LIVE_CORRELATION_HARNESS_DEFAULT := $(ASR_LIVE_CORRELATION_HARNESS_DIR)/asr-live-correlation-harness.exe
else
ASR_LIVE_CORRELATION_HARNESS_DEFAULT := $(ASR_LIVE_CORRELATION_HARNESS_DIR)/asr-live-correlation-harness
endif
ASR_LIVE_CORRELATION_HARNESS ?= $(ASR_LIVE_CORRELATION_HARNESS_DEFAULT)
ASR_LIVE_CORRELATION_HARNESS_DIGEST_FILE ?= $(ASR_LIVE_CORRELATION_HARNESS_DIR)/asr-live-correlation-harness.sha256

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
FUNCTIONAL_MONOLITH ?= true
UNIT_DEFAULT_JOBS ?= $(GO_LANE_BUDGET)
UNIT_TIMING_OUTPUT ?=
UNIT_LATENCY_BUDGET ?= docs/internal/baselines/go-unit-lane-latency-budget.v1.json
UNIT_LATENCY_SAMPLES ?= .artifacts/unit-latency/run-1.v2.json,.artifacts/unit-latency/run-2.v2.json,.artifacts/unit-latency/run-3.v2.json
BASELINE_REGEN_ROOT ?= .
BASELINE_REGEN_DEADCODE_REPORT ?=
# The CLI inventory writers are update-mode tests. Keep their environment
# assignment valid for native Windows Make while retaining the POSIX form used
# by Linux CI and Windows shells backed by sh.exe.
ifeq ($(OS),Windows_NT)
ifneq (,$(or $(findstring /sh,$(SHELL)),$(findstring /bash,$(SHELL))))
BASELINE_REGEN_CLI_UPDATE_ENV := UPDATE_CLI_BASELINES=1
else
BASELINE_REGEN_CLI_UPDATE_ENV := set UPDATE_CLI_BASELINES=1 &&
endif
else
BASELINE_REGEN_CLI_UPDATE_ENV := UPDATE_CLI_BASELINES=1
endif
FUNCTIONAL_LONG_TAGS ?= functionallong
FUNCTIONAL_LONG_PACKAGES := ./tests/functional/...
FUNCTIONAL_LONG_COMPILE_PACKAGES := $(FUNCTIONAL_LONG_PACKAGES) ./pkg/services/models/internal/backendconformance
STRESS_DEFAULT_PACKAGES := ./tests/stress/...
STRESS_FIXTURE_PACKAGES := ./tests/stress/factory_session_snapshots ./pkg/services/models/internal/backends/localai/codecs/stresstests ./pkg/services/factory_visualization/internal/service/stresstests
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
GO_UNIT_COVERAGE_JOBS ?=
GO_FUNCTIONAL_COVERAGE_MANIFEST ?= docs/internal/baselines/go-functional-coverage-package-minimums.json
FUNCTIONAL_QUARANTINE ?= tests/functional/functional-quarantine.json
FUNCTIONAL_TEST_TIER ?= pr-short
FUNCTIONAL_TEST_TRIGGER ?= local
FUNCTIONAL_TEST_BUDGET ?= 35m
FUNCTIONAL_SHORT ?= true
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
FUNCTIONAL_RAW_FAILURE_DIR ?= $(FUNCTIONAL_TEST_VIZ_DIR)/raw-failures
FUNCTIONAL_RAW_FAILURE_MAX_BYTES ?= 536870912
FUNCTIONAL_TEST_VIZ_PACKAGES ?=
FUNCTIONAL_TEST_VIZ_COVER_PACKAGES ?=
FUNCTIONAL_COVERAGE_VERDICT_FILE ?= $(FUNCTIONAL_TEST_VIZ_DIR)/functional-coverage-verdict.txt
FUNCTIONAL_COVERAGE_BUILD_DIAGNOSTICS ?=
FUNCTIONAL_TEST_GO ?= $(GO)
# Hosted CI sets this optional handoff so an ordinary gocoveragecheck failure
# can be reported by its compact terminal verdict step. An unset path preserves
# the historical fail-fast target behavior.
FUNCTIONAL_GOCOVERAGE_EXIT_FILE ?=
BACKEND_DEPENDENCY_GRAPH_DIR ?= .artifacts/backend-dependency-graph
BACKEND_DEPENDENCY_GRAPH_DOT ?= $(BACKEND_DEPENDENCY_GRAPH_DIR)/backend-dependency-graph.dot
BACKEND_DEPENDENCY_GRAPH_SVG ?= $(BACKEND_DEPENDENCY_GRAPH_DIR)/backend-dependency-graph.svg
# Optional CI outputs can be defined but blank. Use the canonical lane budget
# for blank handoffs; report setup rejects invalid nonblank overrides.
LINT_JOBS ?= $(GO_LANE_BUDGET)
ifeq ($(strip $(LINT_JOBS)),)
override LINT_JOBS := $(GO_LANE_BUDGET)
endif
# Keep recursive Make behind an alias so make -n does not execute it.
LINT_MAKE ?= $(MAKE)
LINT_REPORT_FILE ?=
# Local `make lint` runs LINT_TARGETS_BASE, adds the UI gates only when ui/
# differs from the merge-base with origin/main (or has untracked files), and
# leaves the slow deadcode ratchet to CI. CI (CI set) or LINT_FULL=1 runs the
# complete inventory. Override LINT_TARGETS to select targets explicitly.
LINT_TARGETS_BASE := vet model-provider-package-check golangci docs-reference-check fmt-check contracts-check
LINT_TARGETS_UI := ui-lint ui-deadcode
LINT_TARGETS_CI_ONLY := deadcode
LINT_FULL ?=
LINT_UI_CHANGED = $(shell base=$$(git merge-base HEAD origin/main 2>/dev/null) && { git diff --quiet $$base -- ui && test -z "$$(git ls-files --others --exclude-standard ui)" || echo 1; } || echo 1)
LINT_TARGETS ?= $(if $(or $(strip $(CI)),$(strip $(LINT_FULL)),$(strip $(LINT_UI_CHANGED))),$(LINT_TARGETS_UI) )$(LINT_TARGETS_BASE)$(if $(or $(strip $(CI)),$(strip $(LINT_FULL))), $(LINT_TARGETS_CI_ONLY))

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


.PHONY: golangci golangci-lint-run lint-migration-smoke
.PHONY: default default-pipeline-banner build install bundle-api print-go-parallelism
.PHONY: fmt fmt-check vet deps deps-tidy clean init typecheck release lint

.PHONY: test test-full test-unit test-unit-fresh test-unit-latency-budget regenerate-shared-ci-baselines test-ci-workflows test-maintenance test-integration test-localai-runner-v2-component test-localai-runner-v2-prebuilt test-integration-models-managed-process build-integration-models-managed-process-helper test-integration-models-asr-live-correlation build-integration-models-asr-live-correlation-harness test-contract test-stress test-release
.PHONY: test-functional test-functional-fresh test-functional-long test-functional-long-compile test-backend-functional functional-test-viz
.PHONY: test-ui-browser-integration test-ui-storybook-integration test-ui-durable-session-real-backend test-ui-performance ui-component-test
.PHONY: test-unit-coverage test-functional-coverage coverage-help test-backend-coverage test-coverage-go test-race
.PHONY: test-backend-verification test-backend-conformance test-backend-conformance-live test-root-process-acceptance long-tests long-tests-managed-runtime
.PHONY: frontend-verification backend-verification ui-backend-integration

.PHONY: verify-fast verify-pr verify-extended verify-build verify-lint verify-api
.PHONY: verify-build-contracts verify-tests run-concurrent-ui-verification-lanes verify test-ui-coverage

.PHONY: backend-dependency-graph
.PHONY: architecture

.PHONY: generate-api generate-operator-config-schema operator-config-schema-check generate-go-api generate-go-server-api generate-go-client-api generate-ui-api generate-wire
.PHONY: interfaces-api-bundle interfaces-go interfaces-contracts interfaces-ui-openapi interfaces-ui-client interfaces-ui-emulator interfaces-ui interfaces-all

.PHONY: wire-smoke api-smoke api-package-pack-smoke api-package-verify packaged-factory-package-smoke packaged-factory-package-verify packaged-factory-package-script-test packaged-factory-package-pack-check packaged-factory-package-candidate-dry-run packaged-factory-package-consumer-smoke model-provider-package-smoke model-provider-package-verify model-provider-reference-input-smoke
.PHONY: public-release-package-smoke
.PHONY: contracts-validate contracts-generate contracts-check contracts-smoke

.PHONY: cli-contract-smoke cli-manifest-generate cli-manifest-check
.PHONY: fnd-12-behavior-baselines fnd-12-cli-behavior-baselines fnd-12-http-behavior-baselines fnd-12-mcp-behavior-baselines fnd-12-replay-behavior-baselines fnd-12-visualization-behavior-baselines

.PHONY: mcp-contract-check mcp-contract-smoke mcp-discovery-generate mcp-discovery-check

.PHONY: docs-reference-check docs-reference-smoke

.PHONY: script-timeout-companion-smoke-100 cron-time-work-smoke current-factory-watcher-switch-smoke javascript-contract-smoke config-contract-smoke
.PHONY: lint-full packaged-factory-catalog-generate provider-catalog-generate model-provider-package-generate model-provider-package-check test-functional-resumed-successor-artifact
.PHONY: response-stream-stress-smoke release-surface-smoke artifact-contract-closeout
.PHONY: readme-check deadcode dashboard-verify

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
# Before GNU Make 4.4, even a targeted .NOTPARALLEL serializes the whole
# invocation. The lint scheduler is a separate recursive invocation and must
# retain its explicit job budget on those versions too.
ifeq (,$(filter lint-observe-selected,$(MAKECMDGOALS)))
.NOTPARALLEL: default test
endif
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

generate-api: bundle-api generate-go-api generate-ui-api generate-operator-config-schema

generate-operator-config-schema: bundle-api
	$(PYTHON) scripts/generate-operator-config-schema.py

operator-config-schema-check:
	$(PYTHON) scripts/generate-operator-config-schema.py --check

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
	$(MAKE) operator-config-schema-check
	node scripts/check-api-generated-drift.js
	$(GO) test ./pkg/transports/http/contracttests -run TestOpenAPIContract_BundledFactoryEventSchemasRemainComplete -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./tests/functional/product/cli_rest_journeys -run '^TestRESTServerJourneys/TestGeneratedClientAndServerSchemaStayAligned$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

api-package-pack-smoke:
	node --test scripts/package-export-validation.test.mjs scripts/api-package-contract.test.mjs scripts/api-package-pack.test.mjs scripts/api-package-candidate.test.mjs scripts/api-package-registry.test.mjs scripts/api-package-consumer.test.mjs scripts/api-package-pr-dry-run.test.mjs scripts/api-package-publish.test.mjs scripts/api-package-development-workflow.test.mjs

api-package-verify: api-package-pack-smoke

packaged-factory-package-smoke: repository-lint-run packaged-factory-package-script-test

packaged-factory-package-verify: packaged-factory-package-smoke

packaged-factory-package-script-test:
	node --test scripts/packaged-factories-package-pack.test.mjs scripts/packaged-factories-package-candidate.test.mjs scripts/packaged-factories-package-consumer.test.mjs scripts/packaged-factories-package-pr-dry-run.test.mjs scripts/packaged-factories-package-registry.test.mjs scripts/packaged-factories-package-publish.test.mjs scripts/packaged-factories-package-development-command.test.mjs
	$(PYTHON) -B -m unittest discover -s packages/packaged-factories/factories/dub-video/scripts -p 'test_dub_*.py'

packaged-factory-package-pack-check: repository-lint-run
	node -e "require('node:fs').rmSync('.artifacts/packaged-factories-local-pack', { recursive: true, force: true })"
	node scripts/packaged-factories-package-candidate.mjs --package-directory packages/packaged-factories --output-directory .artifacts/packaged-factories-local-pack --run-id 1 --source-commit $(shell git rev-parse HEAD)

packaged-factory-package-candidate-dry-run: repository-lint-run
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
	$(GO) test ./pkg/services/worker_sessions/transports/mcp/... -count=1 -timeout $(GO_TEST_TIMEOUT)
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
	$(GO) test ./tests/functional/product/cli_rest_journeys -run '^TestRESTServerJourneys/TestGeneratedClientAndServerSchemaStayAligned$$' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./tests/functional/work/submission -run TestAPISubmitWorkRejectsEmptyStructuredSubmission -count=1 -timeout $(GO_TEST_TIMEOUT)

fnd-12-mcp-behavior-baselines:
	$(GO) test ./pkg/transports/mcp/server -run '^Test(ServeStdioUsesSDKProtocolAndRegistersCatalog|SDKProtocolErrors)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

# FND-12 captured public replay success + typed-failure pair.
fnd-12-replay-behavior-baselines:
	$(GO) test ./pkg/services/recordings/replay -run '^TestSideEffects_(InferReturnsRecordedProviderResponse|UnmatchedRequestFailsClearly)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

# FND-12 captured visualization activation success + typed-failure pair.
fnd-12-visualization-behavior-baselines:
	$(GO) test ./pkg/services/factory_visualization/internal/service -run '^Test(ServiceProjectsRetainedAndLiveFactoryEvents|NewRejectsMissingDependencies)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

DOCS_MARKDOWN_CACHE ?= .cache/docs-markdown-lint

docs-reference-check: SHELL := bash
docs-reference-check: export GOBIN := $(abspath $(DOCS_MARKDOWN_CACHE)/bin)
docs-reference-check: export PYTHONPATH := $(abspath $(DOCS_MARKDOWN_CACHE)/python)
docs-reference-check: export GOTOOLCHAIN := local
docs-reference-check: $(DOCS_MARKDOWN_CACHE)/ready
	@test -r .gomarklint-docs.json || { printf '%s\n' 'Cannot read .gomarklint-docs.json' >&2; exit 1; }
	$(PYTHON) -m check_jsonschema --schemafile .gomarklint-docs.schema.json .gomarklint-docs.json
	@set -eu; \
	config="$(abspath .gomarklint-docs.json)"; \
	tool="$(GOBIN)/gomarklint$(if $(filter Windows_NT,$(OS)),.exe,)"; \
	scratch=$$(mktemp -d); \
	trap 'rm -f "$$scratch/input.md" "$$scratch/paths"; rmdir "$$scratch"' EXIT; \
	find -H docs/README.md docs/reference \( -type f -o -type l \) -iname '*.md' -print0 > "$$scratch/paths"; \
	while IFS= read -r -d '' source; do \
	  printf 'Checking %s\n' "$$source"; \
	  iconv -f UTF-8 -t UTF-8 "$$source" > "$$scratch/input.md"; \
	  $(PYTHON) -m pymarkdown --disable-rules '*' --enable-rules MD047 --strict-config scan "$$scratch/input.md"; \
	  if [ -s "$$scratch/input.md" ]; then "$$tool" --config "$$config" "$$scratch/input.md"; fi; \
	done < "$$scratch/paths"

$(DOCS_MARKDOWN_CACHE)/ready: SHELL := bash
$(DOCS_MARKDOWN_CACHE)/ready: Makefile scripts/docs-markdown-lint-requirements.txt
	GOBIN="$(abspath $(DOCS_MARKDOWN_CACHE)/bin)" GOTOOLCHAIN=local $(GO) install -p 1 github.com/shinagawa-web/gomarklint/v3@v3.3.1
	$(PYTHON) -m pip install --disable-pip-version-check --no-deps --only-binary=:all: --upgrade --target "$(abspath $(DOCS_MARKDOWN_CACHE)/python)" -r scripts/docs-markdown-lint-requirements.txt
	@touch "$@"

docs-reference-smoke:
	$(MAKE) docs-reference-check
	$(GO) test ./pkg/transports/cli/docs/... -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/transports/cli -run TestDocsCommand_ -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./tests/functional/smoke -run TestDocsCommandSmoke_ -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./tests/functional/transport/cli/customer_commands -run '^TestCLICommandDocumentationJourneys$$/^TestInstalledDocumentationBehaviorThroughPublicProcess$$' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./tests/functional/factory/definitions -run '^TestFactoryValidationDocsCommandDescribesStaticGate$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

readme-check:
	$(GO) run ./cmd/readmecheck

test: test-unit test-ci-workflows

.PHONY: test-acp-sdk test-acp-provider-prebuilt
test-acp-sdk:
	$(GO) test -race ./third_party/acp-go-sdk/... ./pkg/services/providers/internal/services/acp/internal/service -count=1 -timeout $(GO_TEST_TIMEOUT)

test-acp-provider-prebuilt: export INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT := 1
test-acp-provider-prebuilt:
	$(GO) test ./tests/integration/providers/acp -run '^TestPrebuiltACPDeliveredResultsSurvivePeerExit$$' -count=1 -v -timeout $(GO_TEST_TIMEOUT)

test-ci-workflows:
	$(NODE) --test scripts/default-pipeline.test.mjs scripts/development-package-workflow.test.mjs scripts/verification-policy.test.mjs scripts/ci/backend-visualizations-workflow.test.mjs scripts/ci/lane-budget.test.mjs scripts/ci/unit-coverage-workflow.test.mjs scripts/ci/backend-lint-report.test.mjs scripts/ci/backend-lint-workflow.test.mjs scripts/ci/main-ci-churn-report.test.mjs scripts/ci/functional-coverage-comment.test.mjs scripts/ci/functional-coverage-verdict.test.mjs scripts/ci/flake-ledger-summary.test.mjs scripts/ci/unit-coverage-report.test.mjs scripts/ci/workflow-lint.test.mjs scripts/ci/functional-coverage-workflow.test.mjs scripts/ci/functional-coverage-supervisor.test.mjs scripts/ci/shared-baseline-regeneration-workflow.test.mjs scripts/ci/published-backend-conformance-workflow.test.mjs scripts/ci/backend-conformance-workflow.test.mjs scripts/localai-backend-artifact-workflow.test.mjs scripts/localai-llamacpp-independent-images-patch.test.mjs scripts/localai-llamacpp-json-schema-patch.test.mjs

test-full:
	$(GO) test ./... -timeout $(GO_TEST_TIMEOUT)

test-unit:
	$(GO) run ./cmd/unitlane -jobs $(UNIT_DEFAULT_JOBS) -timeout $(GO_TEST_TIMEOUT)

test-unit-fresh:
	$(GO) run ./cmd/unitlane -jobs $(UNIT_DEFAULT_JOBS) -count=1 -timeout $(GO_TEST_TIMEOUT) $(if $(UNIT_TIMING_OUTPUT),-timing-output "$(UNIT_TIMING_OUTPUT)" -timing-command "make test-unit-fresh UNIT_DEFAULT_JOBS=$(UNIT_DEFAULT_JOBS) UNIT_TIMING_OUTPUT=$(UNIT_TIMING_OUTPUT)" -computed-lane-budget $(GO_LANE_BUDGET),)

test-unit-latency-budget:
	$(GO) run ./cmd/unitlanebudget -budget "$(UNIT_LATENCY_BUDGET)" -samples "$(UNIT_LATENCY_SAMPLES)"

regenerate-shared-ci-baselines:
	cd "$(BASELINE_REGEN_ROOT)" && $(GO) run ./cmd/unitlanebudget -mode regenerate -skip-unit-latency -root . $(if $(strip $(BASELINE_REGEN_DEADCODE_REPORT)),-deadcode-report "$(BASELINE_REGEN_DEADCODE_REPORT)",)
	cd "$(BASELINE_REGEN_ROOT)" && $(BASELINE_REGEN_CLI_UPDATE_ENV) $(GO) test ./pkg/transports/cli/commandidentity -run "^TestWriteProductionInventoryBaseline$$" -count=1
	cd "$(BASELINE_REGEN_ROOT)" && $(BASELINE_REGEN_CLI_UPDATE_ENV) $(GO) test ./pkg/transports/cli/cliinputs -run "^TestWriteProductionInputsInventoryBaseline$$" -count=1
	cd "$(BASELINE_REGEN_ROOT)" && $(GO) run ./cmd/mcptoolinventorygen -root .

test-maintenance:
	$(GO) test -short -p=$(UNIT_DEFAULT_JOBS) ./cmd/... ./internal/... ./packages/model-providers ./packages/packaged-factories ./tests/functional/internal/... ./ui ./pkg/services/factory_runtime/internal/exhaustiontests -count=1 -timeout $(GO_TEST_TIMEOUT)

test-localai-runner-v2-component:
	$(GO) test ./tests/internal/localai/corpusv2/runner -count=1 -timeout $(GO_TEST_TIMEOUT)

test-localai-runner-v2-prebuilt: export YOU_OMNI_PREFLIGHT_REQUIRED := 1
test-localai-runner-v2-prebuilt: export INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT := 1
test-localai-runner-v2-prebuilt:
	$(GO) test ./tests/internal/localai/omni_media_probe -run '^TestProbeRunnerV2PrebuiltCLIHandoff$$' -count=1 -v -timeout $(GO_TEST_TIMEOUT)

test-integration:
	$(MAKE) test-wiring-integration
	$(GO) test -short -p=$(UNIT_DEFAULT_JOBS) ./pkg/services/factory_definitions/internal/services/compilation/runtimetests ./pkg/services/factory_definitions/internal/services/catalog/persistence/integrationtests ./pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig/integrationtests ./pkg/services/factory_sessions/internal/execution/fixtures ./pkg/transports/http/servertests/... ./tests/integration/factory/visualization/runtime_metrics ./tests/integration/models ./tests/integration/models/tts_clean_install ./tests/integration/models/platform_conformance ./tests/integration/models/model_invoke ./tests/integration/sessions/restart ./tests/integration/transport/acp/realclient ./tests/integration/transport/cli/process ./tests/integration/transport/server_binding ./tests/integration/workers/cancel ./tests/integration/workers/interrupt ./tests/integration/workers/recording_sidecar -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/services/automations/internal/services/filesystem_watchers/internal/service -run '^TestFileWatcher_' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/platform/process -run '^TestExecCommandRunner_' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/services/workers/internal/worktree -run '^TestPrepareFactoryGitWorktree_(CreatesWorktreeWhenMissing|ReusesExistingValidWorktree|UsesExistingWorktreesParent|ReturnsFailureWhenWorktreeAddFails|ReturnsFailureWhenPathExistsButIsNotWorktree)$$' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./pkg/services/providers/internal/services/execution/internal/adapters/claude -run '^TestClaudeCommandEnvironmentPreventsGitMergeEditorPrompt$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

ifeq ($(OS),Windows_NT)
	$(MAKE) test-integration-models-asr-live-correlation
endif

# The managed-process integration target is intentionally separate from the
# broad integration lane. It owns one helper build, records its immutable
# digest, and supplies both identities to the production boundary package.
build-integration-models-managed-process-helper:
ifeq ($(OS),Windows_NT)
	@powershell -NoProfile -ExecutionPolicy Bypass -Command "$$ErrorActionPreference = 'Stop'; $$artifactPath = [System.IO.Path]::GetFullPath('$(MANAGED_PROCESS_HELPER)'); $$artifactDir = Split-Path -Parent $$artifactPath; $$digestPath = [System.IO.Path]::GetFullPath('$(MANAGED_PROCESS_HELPER_DIGEST_FILE)'); New-Item -ItemType Directory -Path $$artifactDir -Force | Out-Null; New-Item -ItemType Directory -Path (Split-Path -Parent $$digestPath) -Force | Out-Null; $$env:GOFLAGS = '-p=4'; $$env:GOMAXPROCS = '4'; & '$(GO)' build $(GO_BUILD_FLAGS) $(GO_LOCAL_BUILD_FLAGS) -trimpath -o $$artifactPath '$(MANAGED_PROCESS_HELPER_SOURCE)'; if ($$LASTEXITCODE -ne 0) { exit $$LASTEXITCODE }; $$hashAlgorithm = [System.Security.Cryptography.SHA256]::Create(); try { $$sha256 = [System.BitConverter]::ToString($$hashAlgorithm.ComputeHash([System.IO.File]::ReadAllBytes($$artifactPath))).Replace('-', '').ToLowerInvariant() } finally { $$hashAlgorithm.Dispose() }; Set-Content -LiteralPath $$digestPath -Value $$sha256 -NoNewline; Write-Output ('managed process helper path=' + $$artifactPath); Write-Output ('managed process helper bytes=' + (Get-Item -LiteralPath $$artifactPath).Length); Write-Output ('managed process helper sha256=' + $$sha256)"
else
	@set -eu; \
	artifact_path="$(abspath $(MANAGED_PROCESS_HELPER))"; \
	digest_path="$(abspath $(MANAGED_PROCESS_HELPER_DIGEST_FILE))"; \
	mkdir -p "$$(dirname "$$artifact_path")" "$$(dirname "$$digest_path")"; \
	GOFLAGS=-p=4 GOMAXPROCS=4 $(GO) build $(GO_BUILD_FLAGS) $(GO_LOCAL_BUILD_FLAGS) -trimpath -o "$$artifact_path" "$(MANAGED_PROCESS_HELPER_SOURCE)"; \
	if command -v sha256sum >/dev/null 2>&1; then artifact_sha256="$$(sha256sum "$$artifact_path" | awk '{print $$1}')"; else artifact_sha256="$$(shasum -a 256 "$$artifact_path" | awk '{print $$1}')"; fi; \
	printf '%s' "$$artifact_sha256" > "$$digest_path"; \
	printf '%s\n' "managed process helper path=$$artifact_path" "managed process helper bytes=$$(wc -c < "$$artifact_path")" "managed process helper sha256=$$artifact_sha256"
endif

test-integration-models-managed-process: build-integration-models-managed-process-helper
ifeq ($(OS),Windows_NT)
	@powershell -NoProfile -ExecutionPolicy Bypass -Command "$$ErrorActionPreference = 'Stop'; $$artifactPath = [System.IO.Path]::GetFullPath('$(MANAGED_PROCESS_HELPER)'); $$digestPath = [System.IO.Path]::GetFullPath('$(MANAGED_PROCESS_HELPER_DIGEST_FILE)'); $$sha256 = (Get-Content -Raw -LiteralPath $$digestPath).Trim(); $$env:YOU_MODELS_MANAGED_PROCESS_HELPER = $$artifactPath; $$env:YOU_MODELS_MANAGED_PROCESS_HELPER_SHA256 = $$sha256; $$env:GOFLAGS = '-p=4'; $$env:GOMAXPROCS = '4'; & '$(GO)' test -tags=managed_process_integration -p=4 '$(MANAGED_PROCESS_INTEGRATION_PACKAGE)' -count=1 -timeout '$(GO_TEST_TIMEOUT)'; exit $$LASTEXITCODE"
else
	@set -eu; \
	artifact_path="$(abspath $(MANAGED_PROCESS_HELPER))"; \
	digest_path="$(abspath $(MANAGED_PROCESS_HELPER_DIGEST_FILE))"; \
	artifact_sha256="$$(tr -d '\r\n' < "$$digest_path")"; \
	printf '%s\n' "managed process integration helper path=$$artifact_path" "managed process integration helper sha256=$$artifact_sha256"; \
	YOU_MODELS_MANAGED_PROCESS_HELPER="$$artifact_path" YOU_MODELS_MANAGED_PROCESS_HELPER_SHA256="$$artifact_sha256" GOFLAGS=-p=4 GOMAXPROCS=4 $(GO) test -tags=managed_process_integration -p=4 "$(MANAGED_PROCESS_INTEGRATION_PACKAGE)" -count=1 -timeout "$(GO_TEST_TIMEOUT)"
endif

# The ASR correlation test package needs Models internal access, so it is
# compiled as one immutable helper artifact. The integration package under
# tests/integration owns its execution and the two serialized manifest calls.
ifeq ($(OS),Windows_NT)
build-integration-models-asr-live-correlation-harness:
	@powershell -NoProfile -ExecutionPolicy Bypass -Command "$$ErrorActionPreference = 'Stop'; $$artifactPath = [System.IO.Path]::GetFullPath('$(ASR_LIVE_CORRELATION_HARNESS)'); $$digestPath = [System.IO.Path]::GetFullPath('$(ASR_LIVE_CORRELATION_HARNESS_DIGEST_FILE)'); New-Item -ItemType Directory -Path (Split-Path -Parent $$artifactPath) -Force | Out-Null; New-Item -ItemType Directory -Path (Split-Path -Parent $$digestPath) -Force | Out-Null; if (Test-Path -LiteralPath $$artifactPath) { Set-ItemProperty -LiteralPath $$artifactPath -Name IsReadOnly -Value $$false }; $$env:GOFLAGS = '-p=4'; $$env:GOMAXPROCS = '4'; & '$(GO)' test -c -tags=managed_process_integration -trimpath -o $$artifactPath '$(ASR_LIVE_CORRELATION_HARNESS_SOURCE)'; if ($$LASTEXITCODE -ne 0) { exit $$LASTEXITCODE }; $$hashAlgorithm = [System.Security.Cryptography.SHA256]::Create(); try { $$artifactSHA = [System.BitConverter]::ToString($$hashAlgorithm.ComputeHash([System.IO.File]::ReadAllBytes($$artifactPath))).Replace('-', '').ToLowerInvariant() } finally { $$hashAlgorithm.Dispose() }; Set-Content -LiteralPath $$digestPath -Value $$artifactSHA -NoNewline; Set-ItemProperty -LiteralPath $$artifactPath -Name IsReadOnly -Value $$true; Write-Output ('ASR live-correlation harness sha256=' + $$artifactSHA)"

test-integration-models-asr-live-correlation:
	@powershell -NoProfile -ExecutionPolicy Bypass -Command "$$ErrorActionPreference = 'Stop'; $$manifestNames = @('INFINITE_YOU_ASR_LIVE_CORRELATION_RESPONSE_FIRST_MANIFEST','INFINITE_YOU_ASR_LIVE_CORRELATION_EXIT_FIRST_MANIFEST'); $$manifestPaths = @(); foreach ($$name in $$manifestNames) { $$path = [Environment]::GetEnvironmentVariable($$name); if ([string]::IsNullOrWhiteSpace($$path) -or -not [System.IO.Path]::IsPathRooted($$path) -or -not (Test-Path -LiteralPath $$path -PathType Leaf)) { throw ($$name + ' must name an existing absolute manifest file') }; $$manifestPaths += [System.IO.Path]::GetFullPath($$path) }; if ([System.StringComparer]::OrdinalIgnoreCase.Equals($$manifestPaths[0], $$manifestPaths[1])) { throw 'response-first and exit-first ASR manifests must be distinct files' }"
	@$(MAKE) build-integration-models-asr-live-correlation-harness
	@powershell -NoProfile -ExecutionPolicy Bypass -Command "$$ErrorActionPreference = 'Stop'; $$env:INFINITE_YOU_ASR_LIVE_CORRELATION_HARNESS = [System.IO.Path]::GetFullPath('$(ASR_LIVE_CORRELATION_HARNESS)'); $$env:INFINITE_YOU_ASR_LIVE_CORRELATION_HARNESS_SHA256 = (Get-Content -Raw -LiteralPath '$(ASR_LIVE_CORRELATION_HARNESS_DIGEST_FILE)').Trim(); $$env:GOFLAGS = '-p=4'; $$env:GOMAXPROCS = '4'; & '$(GO)' test -tags=managed_process_integration -p=4 '$(ASR_LIVE_CORRELATION_INTEGRATION_PACKAGE)' -count=1 -timeout 10m -v; exit $$LASTEXITCODE"
else
build-integration-models-asr-live-correlation-harness:
	@echo "ASR live-correlation harness is Windows-only" >&2
	@exit 1

test-integration-models-asr-live-correlation:
	@echo "ASR live-correlation integration is Windows-only" >&2
	@exit 1
endif

test-contract:
	$(GO) test -short -p=$(UNIT_DEFAULT_JOBS) ./contracts ./pkg/services/factory_definitions/internal/contracts/contracttests ./pkg/transports/http/contracttests ./pkg/transports/cli/baseline ./pkg/transports/cli/clicontract ./pkg/transports/cli/cliinputs ./pkg/transports/cli/climanifestgen ./pkg/transports/cli/commandidentity ./tests/contract/... -count=1 -timeout $(GO_TEST_TIMEOUT)

# Cache-aware developer feedback; use test-functional-fresh for an
# unconditional rerun.
test-functional:
	$(GO) run ./cmd/functionallane -jobs $(FUNCTIONAL_DEFAULT_JOBS) -timeout $(GO_TEST_TIMEOUT)

# CI-equivalent and flake-investigation path: force every package to execute.
test-functional-fresh:
	$(GO) run ./cmd/functionallane -jobs $(FUNCTIONAL_DEFAULT_JOBS) -count=1 -timeout $(GO_TEST_TIMEOUT)

# The resumed-successor witness consumes operator-staged immutable artifacts;
# its factoryartifact tag keeps the ordinary functional lane from reporting a
# false SKIP when those task-owned inputs are not present.
test-functional-resumed-successor-artifact:
	$(GO) test -tags=factoryartifact ./tests/functional/sessions/isolation_and_recovery -run '^TestResumedSuccessorResponseScopeAndWorkAdmission$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

# functional-test-viz is the single functional-report entrypoint. It runs the
# configured fresh functional coverage tier exactly once, renders and publishes
# the Markdown catalog when running in
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
		-raw-failure-dir "$(FUNCTIONAL_RAW_FAILURE_DIR)" \
		-raw-failure-max-bytes $(FUNCTIONAL_RAW_FAILURE_MAX_BYTES) \
		-test-packages "$(FUNCTIONAL_TEST_VIZ_PACKAGES)" \
		-cover-packages "$(FUNCTIONAL_TEST_VIZ_COVER_PACKAGES)" \
		-profile "$(FUNCTIONAL_TEST_VIZ_PROFILE)" \
		-verdict "$(FUNCTIONAL_COVERAGE_VERDICT_FILE)" \
		-exit-code-file "$(FUNCTIONAL_GOCOVERAGE_EXIT_FILE)" \
		-tier "$(FUNCTIONAL_TEST_TIER)" \
		-trigger "$(FUNCTIONAL_TEST_TRIGGER)" \
		-budget "$(FUNCTIONAL_TEST_BUDGET)" \
		-short=$(FUNCTIONAL_SHORT) \
		-quarantine "$(FUNCTIONAL_QUARANTINE)" \
		-jobs $(FUNCTIONAL_DEFAULT_JOBS) \
		-functional-monolith=$(FUNCTIONAL_MONOLITH) \
		-minimum $(GO_FUNCTIONAL_COVERAGE_MIN) \
		-package-manifest "$(GO_FUNCTIONAL_COVERAGE_MANIFEST)" \
		-package-floor-policy "$(GO_COVERAGE_FLOOR_POLICY)" \
		-test-timeout "$(GO_COVERAGE_TIMEOUT)" \
		$(if $(FUNCTIONAL_COVERAGE_BUILD_DIAGNOSTICS),-coverage-build-diagnostics "$(FUNCTIONAL_COVERAGE_BUILD_DIAGNOSTICS)",)

test-stress:
	$(GO) test -short $(STRESS_DEFAULT_PACKAGES) -skip '^TestL1' -count=1 -timeout $(GO_TEST_TIMEOUT)

.PHONY: test-worker-sessions-l1
test-worker-sessions-l1:
	$(GO) test ./tests/stress/worker_sessions -run '^TestL1' -count=1 -timeout 15m -v

.PHONY: test-stress-fixtures test-unit-monolith test-unit-monolith-prepare test-unit-monolith-prebuilt test-wiring-integration
test-stress-fixtures:
	$(GO) test -p=2 $(STRESS_FIXTURE_PACKAGES) -count=1 -timeout 5m -v

# Opt-in until implementation coverage and source-change rebuild profiles pass.
test-unit-monolith:
	$(GO) run ./cmd/unitlane -monolith -count=1

# Build once after edits; the execution target runs that explicit source snapshot.
test-unit-monolith-prepare:
	$(GO) build -o .artifacts/unitlane$(if $(filter Windows_NT,$(OS)),.exe,) ./cmd/unitlane
	.artifacts/unitlane$(if $(filter Windows_NT,$(OS)),.exe,) -monolith-prepare -jobs $(UNIT_DEFAULT_JOBS)

test-unit-monolith-prebuilt:
	.artifacts/unitlane$(if $(filter Windows_NT,$(OS)),.exe,) -monolith-prebuilt -count=1 $(if $(UNIT_TIMING_OUTPUT),-timing-output "$(UNIT_TIMING_OUTPUT)",)

test-wiring-integration:
	$(GO) run ./cmd/unitlane -wiring-integration -count=1 -jobs $(UNIT_DEFAULT_JOBS) -timeout $(GO_TEST_TIMEOUT)

ifeq ($(OS),Windows_NT)
ifneq (,$(or $(findstring /sh,$(SHELL)),$(findstring /bash,$(SHELL)),$(findstring sh.exe,$(SHELL)),$(findstring bash.exe,$(SHELL))))
test-release:
	@set -eu; \
	release_root="$$(mktemp -d "$${TMPDIR:-/tmp}/infinite-you-release.XXXXXX")"; \
	cleanup() { rm -rf "$$release_root"; }; \
	trap cleanup EXIT HUP INT TERM; \
	artifact_path="$$release_root/$(BINARY_NAME)"; \
	GOFLAGS=-p=4 GOMAXPROCS=4 $(GO) build $(GO_BUILD_FLAGS) $(GO_LOCAL_BUILD_FLAGS) -o "$$artifact_path" $(CMD_PATH); \
	chmod a-w "$$artifact_path"; \
	artifact_size="$$(wc -c < "$$artifact_path" | tr -d '[:space:]')"; \
	if command -v sha256sum >/dev/null 2>&1; then artifact_sha256="$$(sha256sum "$$artifact_path" | awk '{print $$1}')"; else artifact_sha256="$$(shasum -a 256 "$$artifact_path" | awk '{print $$1}')"; fi; \
	source_commit="$$(git rev-parse HEAD)"; \
	source_tree="$$(git rev-parse 'HEAD^{tree}')"; \
	tool_path="$$(command -v $(GO))"; \
	tool_version="$$($(GO) version)"; \
	goos="$$($(GO) env GOOS)"; \
	goarch="$$($(GO) env GOARCH)"; \
	descriptor_path="$$artifact_path"; descriptor_tool_path="$$tool_path"; \
	if command -v cygpath >/dev/null 2>&1; then descriptor_path="$$(cygpath -w "$$artifact_path")"; descriptor_tool_path="$$(cygpath -w "$$tool_path")"; fi; \
	printf '%s\n' "release prebuilt artifact:" "  path=$$descriptor_path" "  size=$$artifact_size" "  sha256=$$artifact_sha256" "  source_commit=$$source_commit" "  source_tree=$$source_tree" "  tool_path=$$descriptor_tool_path" "  tool_version=$$tool_version" "  goos=$$goos" "  goarch=$$goarch"; \
	unset INFINITE_YOU_RELEASE_LOCAL_GO_INSTALL_SMOKE INFINITE_YOU_RELEASE_PUBLIC_GO_INSTALL_SMOKE; \
	export INFINITE_YOU_RELEASE_PREBUILT_REQUIRED=1 INFINITE_YOU_RELEASE_PREBUILT_PATH="$$descriptor_path" INFINITE_YOU_RELEASE_PREBUILT_SHA256="$$artifact_sha256" INFINITE_YOU_RELEASE_PREBUILT_SIZE="$$artifact_size" INFINITE_YOU_RELEASE_PREBUILT_SOURCE_COMMIT="$$source_commit" INFINITE_YOU_RELEASE_PREBUILT_SOURCE_TREE="$$source_tree" INFINITE_YOU_RELEASE_PREBUILT_TOOL_PATH="$$descriptor_tool_path" INFINITE_YOU_RELEASE_PREBUILT_TOOL_VERSION="$$tool_version" INFINITE_YOU_RELEASE_PREBUILT_GOOS="$$goos" INFINITE_YOU_RELEASE_PREBUILT_GOARCH="$$goarch"; \
	$(GO) test -short -p=4 -parallel=4 $(RELEASE_DEFAULT_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)
else
test-release:
	@powershell -NoProfile -ExecutionPolicy Bypass -Command "$$ErrorActionPreference = 'Stop'; $$root = Join-Path ([System.IO.Path]::GetTempPath()) ('infinite-you-release-' + [System.Guid]::NewGuid().ToString('N')); New-Item -ItemType Directory -Path $$root -Force | Out-Null; $$status = 1; try { $$artifact = Join-Path $$root '$(BINARY_NAME)'; $$env:GOFLAGS = '-p=4'; $$env:GOMAXPROCS = '4'; & '$(GO)' build $(GO_BUILD_FLAGS) $(GO_LOCAL_BUILD_FLAGS) -o $$artifact $(CMD_PATH); if ($$LASTEXITCODE -ne 0) { exit $$LASTEXITCODE }; Set-ItemProperty -LiteralPath $$artifact -Name IsReadOnly -Value $$true; $$item = Get-Item -LiteralPath $$artifact; $$artifactSize = [string]$$item.Length; $$sha256 = [System.Security.Cryptography.SHA256]::Create(); try { $$artifactSHA256 = [System.BitConverter]::ToString($$sha256.ComputeHash([System.IO.File]::ReadAllBytes($$artifact))).Replace('-', '').ToLowerInvariant() } finally { $$sha256.Dispose() }; $$sourceCommit = (& git rev-parse HEAD).Trim(); $$sourceTree = (& git rev-parse 'HEAD^{tree}').Trim(); $$toolPath = (Get-Command '$(GO)' -ErrorAction Stop).Source; $$toolVersion = (& '$(GO)' version).Trim(); $$goos = (& '$(GO)' env GOOS).Trim(); $$goarch = (& '$(GO)' env GOARCH).Trim(); Write-Output 'release prebuilt artifact:'; Write-Output ('  path=' + $$artifact); Write-Output ('  size=' + $$artifactSize); Write-Output ('  sha256=' + $$artifactSHA256); Write-Output ('  source_commit=' + $$sourceCommit); Write-Output ('  source_tree=' + $$sourceTree); Write-Output ('  tool_path=' + $$toolPath); Write-Output ('  tool_version=' + $$toolVersion); Write-Output ('  goos=' + $$goos); Write-Output ('  goarch=' + $$goarch); Remove-Item Env:INFINITE_YOU_RELEASE_LOCAL_GO_INSTALL_SMOKE -ErrorAction SilentlyContinue; Remove-Item Env:INFINITE_YOU_RELEASE_PUBLIC_GO_INSTALL_SMOKE -ErrorAction SilentlyContinue; $$env:INFINITE_YOU_RELEASE_PREBUILT_REQUIRED = '1'; $$env:INFINITE_YOU_RELEASE_PREBUILT_PATH = $$artifact; $$env:INFINITE_YOU_RELEASE_PREBUILT_SHA256 = $$artifactSHA256; $$env:INFINITE_YOU_RELEASE_PREBUILT_SIZE = $$artifactSize; $$env:INFINITE_YOU_RELEASE_PREBUILT_SOURCE_COMMIT = $$sourceCommit; $$env:INFINITE_YOU_RELEASE_PREBUILT_SOURCE_TREE = $$sourceTree; $$env:INFINITE_YOU_RELEASE_PREBUILT_TOOL_PATH = $$toolPath; $$env:INFINITE_YOU_RELEASE_PREBUILT_TOOL_VERSION = $$toolVersion; $$env:INFINITE_YOU_RELEASE_PREBUILT_GOOS = $$goos; $$env:INFINITE_YOU_RELEASE_PREBUILT_GOARCH = $$goarch; & '$(GO)' test -short -p=4 -parallel=4 $(RELEASE_DEFAULT_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT); $$status = $$LASTEXITCODE } finally { if (Test-Path -LiteralPath $$root) { Remove-Item -LiteralPath $$root -Recurse -Force } }; exit $$status"
endif
else
test-release:
	@set -eu; \
	release_root="$$(mktemp -d "$${TMPDIR:-/tmp}/infinite-you-release.XXXXXX")"; \
	cleanup() { rm -rf "$$release_root"; }; \
	trap cleanup EXIT HUP INT TERM; \
	artifact_path="$$release_root/$(BINARY_NAME)"; \
	GOFLAGS=-p=4 GOMAXPROCS=4 $(GO) build $(GO_BUILD_FLAGS) $(GO_LOCAL_BUILD_FLAGS) -o "$$artifact_path" $(CMD_PATH); \
	chmod a-w "$$artifact_path"; \
	artifact_size="$$(wc -c < "$$artifact_path" | tr -d '[:space:]')"; \
	if command -v sha256sum >/dev/null 2>&1; then artifact_sha256="$$(sha256sum "$$artifact_path" | awk '{print $$1}')"; else artifact_sha256="$$(shasum -a 256 "$$artifact_path" | awk '{print $$1}')"; fi; \
	source_commit="$$(git rev-parse HEAD)"; \
	source_tree="$$(git rev-parse 'HEAD^{tree}')"; \
	tool_path="$$(command -v $(GO))"; \
	tool_version="$$($(GO) version)"; \
	goos="$$($(GO) env GOOS)"; \
	goarch="$$($(GO) env GOARCH)"; \
	descriptor_path="$$artifact_path"; descriptor_tool_path="$$tool_path"; \
	if command -v cygpath >/dev/null 2>&1; then descriptor_path="$$(cygpath -w "$$artifact_path")"; descriptor_tool_path="$$(cygpath -w "$$tool_path")"; fi; \
	printf '%s\n' "release prebuilt artifact:" "  path=$$descriptor_path" "  size=$$artifact_size" "  sha256=$$artifact_sha256" "  source_commit=$$source_commit" "  source_tree=$$source_tree" "  tool_path=$$descriptor_tool_path" "  tool_version=$$tool_version" "  goos=$$goos" "  goarch=$$goarch"; \
	unset INFINITE_YOU_RELEASE_LOCAL_GO_INSTALL_SMOKE INFINITE_YOU_RELEASE_PUBLIC_GO_INSTALL_SMOKE; \
	export INFINITE_YOU_RELEASE_PREBUILT_REQUIRED=1 INFINITE_YOU_RELEASE_PREBUILT_PATH="$$descriptor_path" INFINITE_YOU_RELEASE_PREBUILT_SHA256="$$artifact_sha256" INFINITE_YOU_RELEASE_PREBUILT_SIZE="$$artifact_size" INFINITE_YOU_RELEASE_PREBUILT_SOURCE_COMMIT="$$source_commit" INFINITE_YOU_RELEASE_PREBUILT_SOURCE_TREE="$$source_tree" INFINITE_YOU_RELEASE_PREBUILT_TOOL_PATH="$$descriptor_tool_path" INFINITE_YOU_RELEASE_PREBUILT_TOOL_VERSION="$$tool_version" INFINITE_YOU_RELEASE_PREBUILT_GOOS="$$goos" INFINITE_YOU_RELEASE_PREBUILT_GOARCH="$$goarch"; \
	$(GO) test -short -p=4 -parallel=4 $(RELEASE_DEFAULT_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)
endif

test-functional-long:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) $(FUNCTIONAL_LONG_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)

# Compile every functionallong-tagged package without running tests
# or starting any of the long-test runtime dependencies.
test-functional-long-compile:
	$(GO) vet -tags=$(FUNCTIONAL_LONG_TAGS) $(FUNCTIONAL_LONG_COMPILE_PACKAGES)

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

test-backend-conformance:
	$(GO) test -tags=backendconformance ./pkg/services/models/internal/backendconformance -count=1 -timeout $(GO_TEST_TIMEOUT)

test-backend-conformance-live:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./pkg/services/models/internal/backendconformance -run '^TestPublishedBackendArtifactLocations$$' -count=1 -timeout $(GO_TEST_TIMEOUT)

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
		$(if $(GO_UNIT_COVERAGE_JOBS),-jobs $(GO_UNIT_COVERAGE_JOBS),) \
		-profile "$(GO_UNIT_COVERAGE_PROFILE)" \
		-coverage-summary "$(GO_UNIT_COVERAGE_JSON_OUTPUT)" \
		-timing-summary "$(GO_UNIT_COVERAGE_TIMING_OUTPUT)" \
		-log "$(GO_UNIT_COVERAGE_LOG)"

# Instrumented coverage preserves Go test caching and the functional lane budget.
# Required Backend Lint owns static functional-boundary enforcement.
test-functional-coverage:
	@echo "Functional tier: name=$(FUNCTIONAL_TEST_TIER) trigger=$(FUNCTIONAL_TEST_TRIGGER) short=$(FUNCTIONAL_SHORT) budget=$(FUNCTIONAL_TEST_BUDGET) selection=subtractive quarantine=$(FUNCTIONAL_QUARANTINE)"
	@set +e; \
	$(GO) run ./cmd/gocoveragecheck -suite functional -functional-monolith=$(FUNCTIONAL_MONOLITH) -stream -jobs $(FUNCTIONAL_DEFAULT_JOBS) -min $(GO_FUNCTIONAL_COVERAGE_MIN) -package-manifest $(GO_FUNCTIONAL_COVERAGE_MANIFEST) -package-floor-policy $(GO_COVERAGE_FLOOR_POLICY) -functional-quarantine $(FUNCTIONAL_QUARANTINE) -timeout $(GO_COVERAGE_TIMEOUT) $(if $(filter false 0 no,$(FUNCTIONAL_SHORT)),-short=false,) $(if $(GO_FUNCTIONAL_COVERAGE_PROFILE),-profile $(GO_FUNCTIONAL_COVERAGE_PROFILE),) $(if $(GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT),-json-output $(GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT),) $(if $(GO_FUNCTIONAL_COVERAGE_TIMING_OUTPUT),-timing-output $(GO_FUNCTIONAL_COVERAGE_TIMING_OUTPUT),); \
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
	$(GO) test ./tests/functional/product/cli_rest_journeys -run '^TestRESTServerJourneys/TestGeneratedClientAndServerSchemaStayAligned$$' -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/replay_contracts -run "TestReplayEventStreamArtifactSmoke_" -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/workers/script -run "TestWorkerPublicContractSmoke_" -count=1 -timeout $(GO_TEST_TIMEOUT)

# Only lint recipes use Git sh on Windows; global budget arithmetic retains
# the caller's shell. Override LINT_SHELL for another installed POSIX shell.
ifeq ($(OS),Windows_NT)
ifneq (,$(or $(findstring /sh,$(SHELL)),$(findstring /bash,$(SHELL))))
LINT_SHELL ?= $(SHELL)
else
# Windows Make requires a shell executable path without spaces.
LINT_SHELL ?= $(subst \,/,$(shell for %%I in ("$(or $(ProgramW6432),$(ProgramFiles))\Git\bin\sh.exe") do @echo %%~sI))
endif
lint:
	@"$(LINT_MAKE)" --no-print-directory lint-run SHELL="$(LINT_SHELL)" LINT_JOBS="$(LINT_JOBS)"
else
lint: lint-run
endif

.PHONY: lint-run
lint-run:
	@run_dir=$$("$(NODE)" scripts/ci/backend-lint-report.mjs --begin-run --jobs "$(LINT_JOBS)" $(if $(LINT_REPORT_FILE),--report "$(LINT_REPORT_FILE)",) -- $(LINT_TARGETS)); \
	test -n "$$run_dir" || exit 1; \
	status=0; \
	"$(LINT_MAKE)" --no-print-directory --keep-going --jobs="$(LINT_JOBS)" --output-sync=target lint-observe-selected LINT_RUN_DIR="$$run_dir" || status=$$?; \
	"$(NODE)" scripts/ci/backend-lint-report.mjs --collect-run "$$run_dir" $(if $(LINT_REPORT_FILE),--report "$(LINT_REPORT_FILE)",) -- $(LINT_TARGETS) || status=1; \
	"$(NODE)" scripts/ci/backend-lint-report.mjs --remove-run "$$run_dir" || status=1; \
	exit $$status

.PHONY: lint-observe-selected $(addprefix lint-observe-,$(LINT_TARGETS))
lint-observe-selected: $(addprefix lint-observe-,$(LINT_TARGETS))
$(addprefix lint-observe-,$(LINT_TARGETS)): lint-observe-%:
	@"$(NODE)" scripts/ci/backend-lint-report.mjs --start-target "$(LINT_RUN_DIR)" --name "$*" || exit 1; \
	status=0; \
	TEMP="$(LINT_RUN_DIR)/$*.tmp" TMP="$(LINT_RUN_DIR)/$*.tmp" TMPDIR="$(LINT_RUN_DIR)/$*.tmp" \
	"$(LINT_MAKE)" --no-print-directory $(if $(filter Windows_NT,$(OS)),SHELL="$(LINT_SHELL)",) "$*" >"$(LINT_RUN_DIR)/$*.log" 2>&1 || status=$$?; \
	"$(NODE)" scripts/ci/backend-lint-report.mjs --record-target "$(LINT_RUN_DIR)" --name "$*" --exit-code "$$status" || exit 1; \
	exit $$status

lint-full:
	$(MAKE) lint LINT_FULL=1

backend-dependency-graph:
	$(GO) run ./cmd/backenddependencygraph -root . -go $(GO) -output $(BACKEND_DEPENDENCY_GRAPH_DOT) -svg-output $(BACKEND_DEPENDENCY_GRAPH_SVG)

# Generated GitHub-renderable architecture pages; coverage inputs are optional
# locally and required by the CI publisher after both measured lanes pass.
architecture:
	$(GO) run ./cmd/backendvisualizations -root . -go $(GO) -output-dir docs/architecture/visualizations $(if $(BACKEND_VIS_UNIT_SUMMARY),-unit-summary $(BACKEND_VIS_UNIT_SUMMARY),) $(if $(BACKEND_VIS_FUNCTIONAL_SUMMARY),-functional-summary $(BACKEND_VIS_FUNCTIONAL_SUMMARY),) $(if $(BACKEND_VIS_SOURCE_COMMIT),-source-commit $(BACKEND_VIS_SOURCE_COMMIT),) $(if $(BACKEND_VIS_REQUIRE_COVERAGE),-require-coverage,)


packaged-factory-catalog-generate:
	$(GO) run ./cmd/packagedfactorycataloggenerate -root .

provider-catalog-generate:
	$(GO) run ./cmd/providercataloggenerate -root .

model-provider-package-generate:
	node scripts/model-provider-package.mjs generate

model-provider-package-check:
	node scripts/model-provider-package.mjs check


# golangci-lint is pinned to an exact version and run with go run, like the
# deadcode tool. v2.11.4 is the newest release whose go.mod needs go 1.25.0.
# Ratchet: issues.new-from-merge-base in .golangci.yml, so origin/main must be
# fetched (Backend Lint checks out with fetch-depth 0).
GOLANGCI_LINT_VERSION ?= v2.11.4
GOLANGCI_LINT ?= $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOLANGCI_DIR ?= .artifacts/golangci
# Optional diagnostics use distinct profiles for the three authoritative scopes.
# Verbose logs include package loading and go/analysis pass timings.
GOLANGCI_PROFILE_DIR ?=
golangci_profile_flags = $(if $(strip $(GOLANGCI_PROFILE_DIR)),--verbose --cpu-profile-path "$(GOLANGCI_PROFILE_DIR)/$(1).cpu.pprof",)
ifeq ($(OS),Windows_NT)
GOLANGCI_REPOSITORY ?= $(GOLANGCI_DIR)/golangci-repository.exe
else
GOLANGCI_REPOSITORY ?= $(GOLANGCI_DIR)/golangci-repository
endif

# Repository source diagnostics run through the supported module plugin.
.PHONY: golangci-build repository-lint-run
ifeq ($(GOLANGCI_PREBUILT),1)
golangci-build:
	@test -f "$(GOLANGCI_REPOSITORY)"
	@test -f "$(GOLANGCI_DIR)/host-path.txt"
else
golangci-build:
	$(GO) test ./internal/lint/analyzers ./tools/golangcilintplugin
	$(PYTHON) scripts/build-golangci.py --destination "$(GOLANGCI_DIR)" -- $(GOLANGCI_LINT) custom --destination "$(GOLANGCI_DIR)"
endif

repository-lint-run: golangci-build
	@git merge-base HEAD origin/main
	@$(if $(strip $(GOLANGCI_PROFILE_DIR)),mkdir -p "$(GOLANGCI_PROFILE_DIR)",:)
	$(GOLANGCI_REPOSITORY) run $(call golangci_profile_flags,repository-default) --config .golangci-repository-default.yml ./...
	$(GOLANGCI_REPOSITORY) run $(call golangci_profile_flags,repository-tagged) --config .golangci-repository.yml --build-tags="$(REPOSITORY_LINT_TAGS)" ./...
# Compiler tag union shared with the compiler-owner analyzer.
REPOSITORY_LINT_TAGS ?= integration,functionallong,backendconformance,factoryartifact,managed_process_integration

# All shared diagnostics execute inside the supported golangci module plugin.
golangci: golangci-lint-run repository-lint-run

LINT_MIGRATION_COHORT ?= all

lint-migration-smoke: $(if $(filter markdown,$(LINT_MIGRATION_COHORT)),$(DOCS_MARKDOWN_CACHE)/ready,golangci-build)
	$(PYTHON) scripts/lint-migration-smoke.py "$(LINT_MIGRATION_COHORT)" --golangci "$(abspath $(GOLANGCI_REPOSITORY))"

golangci-lint-run: golangci-build
	@git merge-base HEAD origin/main
	@$(if $(strip $(GOLANGCI_PROFILE_DIR)),mkdir -p "$(GOLANGCI_PROFILE_DIR)",:)
	$(GOLANGCI_REPOSITORY) run $(call golangci_profile_flags,builtin) --config .golangci.yml ./...

deadcode: golangci-build
	$(PYTHON) scripts/deadcode-report.py --golangci-host-file "$(GOLANGCI_DIR)/host-path.txt" -- $(GO) run golang.org/x/tools/cmd/deadcode@v0.25.1

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
	$(GO) vet -trimpath ./cmd/... ./contracts/... ./docs/... ./internal/... ./packages/... ./pkg/... ./scripts/... ./tests/... ./ui/...

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
