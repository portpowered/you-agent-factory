# PRD: Factory Service Core and Coordinator Split with Go Wire

## Introduction

The current backend service layer combines three different responsibilities inside `FactoryService`:

1. building and wiring the runtime graph from root configuration
2. exposing application-facing APIs used by CLI and HTTP layers
3. coordinating session lifecycle, runtime replacement, sidecars, and cross-service orchestration

This creates coupling between composition, capability exposure, and orchestration. As a result, APIs that should depend on a narrower service, such as model operations, currently route through the primary factory service because it owns session/runtime lookup and cross-service state.

This initiative will separate those responsibilities into explicit components while keeping existing transport contracts stable during the migration. The new architecture must use Go Wire as the composition root mechanism and must expose one primary entrypoint for constructing the runtime graph.

## Goals

- Separate composition, API exposure, and orchestration into distinct backend components.
- Introduce a single Go Wire-driven composition root for backend runtime assembly.
- Extract a dedicated `ModelService` API that no longer depends on the full `FactoryService` surface.
- Extract a dedicated `Coordinator` that owns runtime/session lifecycle orchestration.
- Keep HTTP and CLI contracts stable during the first implementation phase.
- Preserve current multi-session behavior, session-owned runtime state, and runtime replacement semantics.

## User Stories

### US-001: Build a single Go Wire composition root
**Description:** As a backend maintainer, I want one Wire-built runtime entrypoint so that service assembly is explicit, testable, and not spread across ad hoc constructors.

**Acceptance Criteria:**
- [ ] Introduce one primary runtime composition entrypoint built via Go Wire.
- [ ] The entrypoint returns a single root object that exposes the runtime graph needed by transports.
- [ ] Existing CLI and HTTP composition paths can construct the runtime graph through the Wire entrypoint.
- [ ] Manual dependency wiring in the service composition path is removed or reduced to compatibility adapters only.
- [ ] `go test` passes for the service, compose, and affected CLI packages.

### US-002: Introduce a core runtime graph object
**Description:** As a backend maintainer, I want a core object that owns normalized runtime dependencies so that composition concerns are separated from orchestration and transport-facing APIs.

**Acceptance Criteria:**
- [ ] Introduce a core runtime graph type such as `FactoryCore`.
- [ ] The core owns normalized policy, registries, and constructed sub-services.
- [ ] The core does not absorb orchestration logic that belongs in a coordinator.
- [ ] The core exposes explicit accessors or interfaces for sub-services used by transports.
- [ ] The core can be constructed without starting the runtime loop.
- [ ] `go test` passes for the service and compose packages.

### US-003: Extract a dedicated model service
**Description:** As an API maintainer, I want model-related handlers to call a model-specific service so that model operations do not depend on the full primary factory service.

**Acceptance Criteria:**
- [ ] Introduce a `ModelService` with a clear interface for list/get/pull/invoke operations.
- [ ] `ModelService` depends on explicit collaborators such as runtime/session resolution, model assets, and invocation policy.
- [ ] CLI and HTTP model-facing paths call `ModelService` directly or through a thin compatibility adapter.
- [ ] `FactoryService` no longer contains the authoritative implementation of model operations.
- [ ] Existing model API behavior and response shapes remain unchanged.
- [ ] `go test` passes for service and model-related tests.

### US-004: Extract a dedicated coordinator
**Description:** As a backend maintainer, I want a coordinator that owns runtime and session orchestration so that lifecycle behavior is isolated from composition and transport concerns.

**Acceptance Criteria:**
- [ ] Introduce a `FactoryCoordinator` or equivalent orchestration component.
- [ ] The coordinator owns active session tracking, default session behavior, runtime start/stop, replacement, and sidecar orchestration.
- [ ] Poller orchestration and service-mode runtime orchestration move under the coordinator or its dedicated subcomponents.
- [ ] Session registry interactions occur through coordinator-owned paths rather than being spread across unrelated APIs.
- [ ] Runtime/session lifecycle tests continue to pass without behavior regressions.

### US-005: Keep `FactoryService` as a migration compatibility layer
**Description:** As a maintainer, I want compatibility during the migration so that we can move callers incrementally without breaking existing transports.

**Acceptance Criteria:**
- [ ] `FactoryService` remains available during migration as a thin facade or adapter layer.
- [ ] New business logic is added to extracted services or the coordinator, not to `FactoryService`.
- [ ] `FactoryService` delegates to core services rather than owning the canonical implementation.
- [ ] Compatibility delegation is documented in code comments where the indirection is not obvious.
- [ ] Existing transport entrypoints continue to compile and behave the same.

### US-006: Preserve transport contracts during phase one
**Description:** As a caller of the CLI or HTTP API, I want the backend architecture to improve without requiring immediate transport contract changes.

**Acceptance Criteria:**
- [ ] No HTTP API request/response contract changes are required in the first phase.
- [ ] No CLI command contract changes are required in the first phase.
- [ ] Existing compose entrypoints continue to return the current externally expected service/facade object or a compatible wrapper.
- [ ] Regression coverage confirms transport-level behavior remains stable.

## Functional Requirements

1. FR-1: The backend must provide a single Go Wire composition entrypoint that assembles the runtime graph from root configuration.
2. FR-2: The composition entrypoint must construct a single core runtime object that owns normalized policy and explicit collaborators.
3. FR-3: The runtime core must expose sub-services through explicit fields, methods, or interfaces rather than requiring callers to depend on a monolithic service object.
4. FR-4: The runtime core must include a dedicated coordinator responsible for session lifecycle and runtime orchestration.
5. FR-5: The coordinator must own session registry access, default-session behavior, runtime replacement, service-mode runtime start/stop, and sidecar coordination.
6. FR-6: Model-facing backend operations must be implemented by a dedicated `ModelService`, not by the monolithic primary service object.
7. FR-7: `ModelService` must depend on explicit collaborators for runtime resolution, model asset access, and invocation policy.
8. FR-8: The first migration phase must preserve existing CLI and HTTP transport contracts.
9. FR-9: `FactoryService` may remain as a compatibility facade during migration, but it must delegate to extracted components instead of being the canonical owner of new logic.
10. FR-10: The service layer must preserve session-owned runtime state and must not regress the current session isolation model.
11. FR-11: The architecture must preserve the existing ability to host multiple sessions concurrently with separate runtime state and working directories.
12. FR-12: The runtime graph must remain constructible in tests with explicit collaborator substitution.
13. FR-13: The design must support incremental migration so that extracted services can move behind current transports before transports are rewritten to call them directly.

## Non-Goals

- No HTTP API surface redesign in the first phase.
- No CLI command redesign in the first phase.
- No OpenAPI schema changes required purely for this refactor.
- No requirement to move packages across major repo boundaries in the first phase.
- No runtime behavior redesign unrelated to service ownership boundaries.
- No persistence model rewrite beyond what is required to preserve current session-owned runtime state.

## Design Considerations

- The composition root must remain singular from the caller’s perspective. One runtime entrypoint is preferred over multiple competing constructors.
- The returned root object should not become a renamed god object. It should expose smaller services and the coordinator rather than reabsorbing their logic.
- Compatibility adapters should be explicit and temporary, not the place where new logic accumulates.
- Naming should reflect roles clearly. For example:
  - `FactoryCore` for the runtime graph root
  - `FactoryCoordinator` for orchestration
  - `ModelService`, `SessionService`, `FactoryDefinitionService` for service APIs

## Technical Considerations

- Use Go Wire as the required composition root mechanism.
- Reuse the recent session-state split and normalized service policy work rather than reintroducing service-global mutable runtime state.
- Keep extracted services dependent on narrow interfaces such as runtime/session resolvers instead of depending on the full core object where not necessary.
- Favor constructor inputs that are immutable or normalized before runtime use.
- Keep `FactoryServiceConfig` as a public build-time DTO until a later phase, but allow the core to consume a normalized internal build policy.
- Ensure tests can still substitute collaborators such as model assets, command runners, workstation loaders, persistence services, and runtime builders.
- Avoid moving all logic in one step. Migrate behavior in slices:
  1. establish core and Wire root
  2. extract `ModelService`
  3. extract `Coordinator`
  4. shrink `FactoryService` into compatibility-only role
- Preserve existing functional behavior around:
  - session switching
  - named factory activation
  - service-mode startup
  - poller lifecycle
  - model invocation and model asset management

## Success Metrics

- New backend composition is built from one Go Wire entrypoint.
- Model-facing handlers no longer need to call the monolithic `FactoryService` implementation directly.
- Coordinator-owned lifecycle code is isolated from composition and transport-specific concerns.
- `FactoryService` shrinks into a compatibility adapter with materially less direct business logic.
- Existing service, session, model, compose, and transport tests continue to pass.
- Developers can identify where to add new model, orchestration, or composition logic without ambiguity.

## Open Questions

- Should the long-term public root returned by Wire remain a compatibility facade, or should transports eventually depend on `FactoryCore` directly?
- Which additional extracted services should follow `ModelService` and `Coordinator` in the second phase: session service, factory definition service, or work service?
- Should the coordinator live under `pkg/service` or a more specific package once the boundary is stable?
- How much of the current compose/provider helper surface should remain public after the Wire root is established?
- At what point should the public `FactoryServiceConfig` DTO be paired with or replaced by an explicit normalized build input type?
