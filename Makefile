BINARY_NAME := agent-factory
CMD_PATH    := ./cmd/factory/
BIN_DIR     := bin
BUN         ?= bun
GO          ?= go
INSTALL_DIR := $(or $(GOBIN),$(shell $(GO) env GOPATH)/bin)
FUNCTIONAL_DEFAULT_PACKAGES := ./tests/functional/...
FUNCTIONAL_LONG_TAGS ?= functionallong
FUNCTIONAL_LONG_PACKAGES := ./tests/functional/...
SCRIPT_TIMEOUT_COMPANION_SMOKE_TEST := TestIntegrationSmoke_ScriptTimeoutCompanionRequeuesBeforeLaterCompletion
SCRIPT_TIMEOUT_COMPANION_SMOKE_COUNT ?= 100
SCRIPT_TIMEOUT_COMPANION_SMOKE_TIMEOUT ?= 120s
CRON_TIME_WORK_SMOKE_TEST := TestCronWorkstations_ServiceModeSmoke_SubmitsInternalTimeWorkExpiresRetriesDispatchesAndFiltersViews
CRON_TIME_WORK_SMOKE_COUNT ?= 10
CRON_TIME_WORK_SMOKE_TIMEOUT ?= 120s
CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TEST := TestCurrentFactoryActivationFixture_ActivatesSecondPersistedFactoryAndResolvesCurrentFactory
CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_COUNT ?= 1
CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TIMEOUT ?= 120s

ifeq ($(OS),Windows_NT)
	BINARY_NAME := agent-factory.exe
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

.PHONY: default build intall bundle-api generate-api generate-go-api generate-ui-api api-smoke docs-reference-check docs-reference-smoke test test-full test-functional test-functional-long functional-layout-contract script-timeout-companion-smoke-100 cron-time-work-smoke current-factory-watcher-switch-smoke release-surface-smoke artifact-contract-closeout lint deadcode  test-race fmt vet deps deps-tidy dashboard-verify ui-deps ui-build ui-test ui-storybook ui-test-storybook clean

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

generate-go-api:
	$(GO) generate -tags=interfaces ./pkg/api

generate-ui-api:
	cd ui && $(BUN) run generate-api

api-smoke:
	node scripts/run-quiet-api-command.js validate:main ./api/openapi-main.yaml
	$(MAKE) generate-api
	$(MAKE) generate-api
	git diff --exit-code -- api/openapi.yaml pkg/api/generated/server.gen.go ui/src/api/generated/openapi.ts
	$(GO) test ./pkg/api -run TestOpenAPIContract_BundledFactoryEventSchemasRemainComplete -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(GO) test ./tests/functional/runtime_api -run TestGeneratedAPIIntegrationSmoke_OpenAPIGeneratedServerAndLiveRuntimeStayAligned -count=1 -timeout $(GO_TEST_TIMEOUT)

docs-reference-check:
	$(GO) run ../markdown-linter/cmd/markdown-linter docs/README.md docs/reference

docs-reference-smoke:
	$(MAKE) docs-reference-check
	$(GO) test ./tests/functional/smoke -run TestDocsCommandSmoke_ -count=1 -timeout $(GO_TEST_TIMEOUT)

test:
	$(GO) test -short ./... -timeout $(GO_TEST_TIMEOUT)

test-full:
	$(GO) test ./... -timeout $(GO_TEST_TIMEOUT)

test-functional:
	$(GO) test -short $(FUNCTIONAL_DEFAULT_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)

test-functional-long:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) $(FUNCTIONAL_LONG_PACKAGES) -count=1 -timeout $(GO_TEST_TIMEOUT)

functional-layout-contract:
	$(GO) test ./internal/contractguard -run TestFunctionalLayoutContractGuard_ -count=1 -timeout $(GO_TEST_TIMEOUT)

script-timeout-companion-smoke-100:
	$(GO) test ./tests/functional/providers -run $(SCRIPT_TIMEOUT_COMPANION_SMOKE_TEST) -count=$(SCRIPT_TIMEOUT_COMPANION_SMOKE_COUNT) -timeout $(SCRIPT_TIMEOUT_COMPANION_SMOKE_TIMEOUT)

cron-time-work-smoke:
	$(GO) test ./tests/functional/runtime_api -run $(CRON_TIME_WORK_SMOKE_TEST) -count=$(CRON_TIME_WORK_SMOKE_COUNT) -timeout $(CRON_TIME_WORK_SMOKE_TIMEOUT)

current-factory-watcher-switch-smoke:
	$(GO) test -tags=$(FUNCTIONAL_LONG_TAGS) ./tests/functional/bootstrap_portability -run $(CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TEST) -count=$(CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_COUNT) -timeout $(CURRENT_FACTORY_WATCHER_SWITCH_SMOKE_TIMEOUT)

artifact-contract-closeout:
	$(GO) test ./pkg/testutil -run TestArtifactContractInventory_ -count=1 -timeout $(GO_TEST_TIMEOUT)
	$(MAKE) release-surface-smoke
	$(GO) test ./pkg/api ./pkg/config ./pkg/replay ./tests/adhoc ./tests/functional_test -count=1 -timeout $(GO_TEST_TIMEOUT)

lint:
	$(GO) vet ./...
	$(MAKE) deadcode
	$(MAKE) functional-layout-contract

deadcode:
	$(GO) run ./cmd/deadcodecheck

dashboard-verify:
	$(MAKE) ui-build
	$(MAKE) lint
	$(MAKE) test

ui-deps:
	cd ui && $(BUN) install --frozen-lockfile

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

ui-build:
	cd ui && $(BUN) run build

ui-test:
	cd ui && $(BUN) run test

ui-storybook:
	cd ui && $(BUN) run build-storybook

ui-test-storybook:
	cd ui && $(BUN) run test-storybook

clean:
	$(GO) clean ./...
	rm -rf $(BIN_DIR)
