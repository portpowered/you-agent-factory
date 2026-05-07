# Package Dependency Graph

---
author: ralph agent
last modified: 2026, march, 20
doc-id: AGF-DEV-002
---

This document describes the Go package dependency graph for the `agent-factory` library. Use it to understand the import structure, validate that no cycles are introduced, and orient yourself when adding new packages.

## Regenerating the Graph

Run from the `libraries/agent-factory/` directory:

```bash
go list -f '{{.ImportPath}} -> {{join .Imports " "}}' ./... | grep portpowered
```

Or for a focused view of inter-package imports only:

```bash
go list -f '{{.ImportPath}}: {{join .Imports ", "}}' ./... \
  | grep portpowered \
  | sed 's|github.com/portpowered/infinite-you/||g'
```

## Package Overview

```
agent-factory/
├── cmd/factory/           # Binary entry point (imports pkg/cli)
├── pkg/
│   ├── interfaces/        # Shared constants only — no imports from this module
│   ├── logging/           # Logger type — no imports from this module
│   ├── petri/             # Petri net primitives (imports interfaces)
│   ├── store/             # Persistence store (imports interfaces, petri)
│   ├── factory/context/   # FactoryContext type (imports interfaces)
│   ├── factory/state/     # Net/marking types (imports petri)
│   │   ├── loader/        # Net loader from disk (imports state, validation, store, petri)
│   │   └── validation/    # Net validation (imports state, petri)
│   ├── factory/scheduler/ # Transition selection strategies (imports state, petri)
│   ├── workers/           # WorkerExecutor implementations (imports factory/context, interfaces, logging, petri)
│   ├── factory/           # Factory interface + options (imports scheduler, state, logging, petri, store, workers)
│   ├── factory/subsystems/# Engine subsystems: dispatcher, collector, etc (imports factory, factory/context, scheduler, state, logging, petri, store, workers)
│   ├── factory/engine/    # CPN execution engine (imports factory, scheduler, state, subsystems, logging, petri)
│   ├── factory/runtime/   # Factory runtime/loop (imports factory, engine, scheduler, state, subsystems, logging, petri, workers)
│   ├── listeners/         # File-system event listeners (imports factory, interfaces)
│   ├── config/            # Koanf-based config (imports mux, koanf)
│   ├── service/           # Service wiring (imports config, factory, runtime, state, loader, validation, interfaces, listeners, logging, petri, store, workers)
│   ├── api/               # HTTP API handlers (imports factory, state/validation, petri, service, store)
│   ├── cli/               # CLI router plus command packages
│   │   ├── config/        # Config portability commands
│   │   ├── dashboard/     # CLI dashboard formatting and dashboard read models
│   │   ├── default/       # No-argument default flow configuration
│   │   ├── init/          # Factory scaffold command
│   │   ├── run/           # Runtime execution command
│   │   └── submit/        # Work submission command
│   ├── testutil/          # Test harness (imports factory, engine, scheduler, state, subsystems, petri, workers)
│   └── workers/           # (see above)
└── tests/
    ├── functional/        # Functional tests using testutil
    ├── stress/            # Stress/concurrency tests
    └── adhoc/             # Ad-hoc manual tests
```

## Dependency Layers (leaf → root)

The package tree is strictly layered. Lower layers must not import higher layers.

```
Layer 0 (leaf): interfaces, logging
Layer 1:        petri           (→ interfaces)
Layer 2:        store           (→ interfaces, petri)
                factory/context (→ interfaces)
Layer 3:        factory/state   (→ petri)
Layer 4:        factory/state/validation (→ state, petri)
                factory/state/loader    (→ state, validation, store, petri)
                factory/scheduler       (→ state, petri)
Layer 5:        workers         (→ factory/context, interfaces, logging, petri)
Layer 6:        factory         (→ scheduler, state, logging, petri, store, workers)
Layer 7:        factory/subsystems (→ factory, factory/context, scheduler, state, logging, petri, store, workers)
                factory/engine     (→ factory, scheduler, state, subsystems, logging, petri)
Layer 8:        factory/runtime    (→ factory, engine, scheduler, state, subsystems, logging, petri, workers)
                listeners          (→ factory, interfaces)
Layer 9:        service         (→ config, factory, runtime, state, loader, validation, interfaces, listeners, logging, petri, store, workers)
Layer 10:       api             (→ factory, state/validation, petri, service, store)
                cli/dashboard   (→ factory/state, petri, workers)
                cli/run         (→ api, cli/dashboard, cli/init, config, factory, interfaces, logging, petri, service)
                cli/config      (→ config, interfaces, workers)
                cli/init        (→ interfaces)
                cli/submit      (→ api/generated)
                cli/default     (→ cli/run, logging)
Layer 11:       cli             (→ cli/config, cli/dashboard, cli/default, cli/init, cli/run, cli/submit)
                cmd/factory     (→ cli)
```

## Known Historical Cycles (Now Resolved)

| Cycle | Resolution |
|-------|-----------|
| `pkg/workers` (test) → `pkg/factory` → `pkg/workers` | `FactoryContext` moved from `pkg/factory` to `pkg/factory/context`; test imports updated |

## Rules to Prevent Future Cycles

1. **Never import a higher layer from a lower layer.** `workers` must not import `factory`. `factory` must not import `factory/engine` or `factory/subsystems`.
2. **Shared types that multiple packages need belong in a leaf package** (`interfaces`, `logging`, or a new leaf). If adding a type to `factory` would cause a cycle with `workers`, add it to `factory/context` instead.
3. **Test files obey the same rules.** A test in `pkg/foo` that imports `pkg/bar` — and `pkg/bar` imports `pkg/foo` — creates a cycle even if the production code does not.
4. **Verify with:** `go build ./...` — import cycles are compile errors.
