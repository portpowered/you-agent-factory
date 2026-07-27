# Functional tests

## Domain summaries

### transport

- Customer scenarios: 1
- Packages: `process` (1)
- Subsections: `cli` (1)

### workers

- Customer scenarios: 1
- Packages: `openai` (1)
- Subsections: `inference` (1)

### orchestration

- Customer scenarios: 0

### workstations

- Customer scenarios: 0

### work

- Customer scenarios: 0

### sessions

- Customer scenarios: 0

### factory

- Customer scenarios: 1
- Packages: `definitions` (1)
- Subsections: `definitions` (1)

### provider_sessions

- Customer scenarios: 0

### events

- Customer scenarios: 0

### models

- Customer scenarios: 0

### guards / resources

- Customer scenarios: 1
- Packages: `policy` (1)
- Subsections: `policy` (1)

### observability / product / resilience

- Customer scenarios: 0

## Test catalog

### transport

- **TestHelp** — verifies help output
  - Source: `transport/cli/process/help_test.go:12`
  - Domain: `transport`
  - Package: `process`
  - Labels: short


### workers

- **TestInvoke** — verifies provider invoke replay
  - Source: `workers/inference/openai/invoke_long_test.go:40`
  - Domain: `workers`
  - Package: `openai`
  - Labels: long-only, golden-backed
  - Golden provenance:
    - Provider: `openai`
    - Case: `invoke`
    - Fidelity class: `partial-stream`
    - Golden id: `openai-invoke`
    - Manifest: `testdata/goldens/openai/invoke/manifest.json`


### orchestration

- _No customer scenarios._

### workstations

- _No customer scenarios._

### work

- _No customer scenarios._

### sessions

- _No customer scenarios._

### factory

- **TestLoad** — (undocumented)
  - Source: `factory/definitions/load_test.go:7`
  - Domain: `factory`
  - Package: `definitions`
  - Labels: short, undocumented


### provider_sessions

- _No customer scenarios._

### events

- _No customer scenarios._

### models

- _No customer scenarios._

### guards / resources

- **TestEnforce** — verifies guard enforcement
  - Source: `guards/policy/enforce_test.go:15`
  - Domain: `guards`
  - Package: `policy`
  - Labels: short


### observability / product / resilience

- _No customer scenarios._


### Other domains

- **TestSession** — (undocumented)
  - Source: `runtime_api/session_test.go:5`
  - Domain: `runtime_api`
  - Package: `runtime_api`
  - Labels: short, deprecated, undocumented



## Harness verification

- **TestHelper** — (undocumented)
  - Source: `internal/support/helpers_test.go:3`
  - Domain: `internal`
  - Package: `support`
  - Labels: short


## Documentation debt

### Undocumented customer tests

- `factory/definitions/load_test.go::TestLoad`
- `runtime_api/session_test.go::TestSession`

### Deprecated tests

- `runtime_api/session_test.go::TestSession`

## Package coverage

- Covered statements: 8
- Measurable statements: 10
- Coverage percent: 80.0%

| Package | Covered | Measurable | Coverage % | Floor | Measurement exception |
| --- | ---: | ---: | ---: | ---: | --- |
| `github.com/portpowered/infinite-you/pkg/config` | 3 | 3 | 100.0 | 66.66 | — |
| `github.com/portpowered/infinite-you/pkg/service` | 0 | 0 | 0.0 | — | measurement: no measurable statements (owner=backend-quality; deadline=2027-07-15; removalGate=profile reports measurable statements) |
