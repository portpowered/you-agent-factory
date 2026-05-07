# Factory Config Schema Inventory And Enum Policy

This artifact records the high-value public factory-config resources and
direct-authoring fields in scope for the schema-description and enum-contract
cleanup. It separates fields that must stay free-form or reference-backed from
fields that later stories should tighten into named enum schemas, and it fixes
the canonical public enum casing policy for this slice before OpenAPI,
generated-model, and config-boundary work begins.

## Change

- PRD, design, or issue: `prd.json` for Agent Factory Config Schema
  Descriptions And Enum Contracts
- Owner: Agent Factory maintainers
- Packages or subsystems: `api/openapi.yaml`, `pkg/api/generated`,
  `pkg/config`, `pkg/replay`, `ui/src/api/generated`, docs/examples
- Canonical package responsibility artifact: `api/openapi.yaml` owns the
  public factory-config contract; `pkg/config` owns generated-to-runtime
  mapping plus boundary validation; generated Go and UI models are derived
  outputs.
- Canonical package interaction artifact: generated `pkg/api/generated.Factory`
  remains the shared public contract used by config loading, replay, API
  events, and generated UI consumers; runtime packages translate at explicit
  mapper boundaries instead of round-tripping through loose JSON.

## Trigger Check

- [x] Shared configuration shape
- [x] Inter-package contract or payload
- [x] API or generated schema
- [x] Package-local model another package must interpret

## Boundary Rules

- This inventory is scoped to the public factory-config contract rooted at
  `Factory`. Separate submit, status, and runtime event contracts are out of
  scope unless they embed or serialize the generated `Factory` payload.
- Public enum contracts use one canonical style: uppercase values, with
  underscore separators for multiword values.
- OpenAPI named component schemas own public enum names, descriptions, and
  canonical example values. Generated Go/UI models and config loading must
  preserve those same canonical values.
- Caller-owned identifiers, templates, file paths, command arguments,
  durations, schedule expressions, and free-form map keys do not become enums
  just because they are strings.

## High-Value Resource Inventory

| Schema | High-value direct-authoring fields in scope | Contract classification |
| --- | --- | --- |
| `Factory` | `project`, `metadata`, `inputTypes`, `workTypes`, `resources`, `workers`, `workstations` | Root authored graph. `metadata` stays free-form; nested resource arrays recurse into the schema-specific rules below. |
| `InputType` | `name`, `type` | `name` stays free-form. `type` must reference named enum `InputKind`. |
| `WorkType` | `name`, `states` | `name` stays free-form. `states` recurse into `WorkState`. |
| `WorkState` | `name`, `type` | `name` stays free-form. `type` already references named enum `WorkStateType` and should stay enum-backed. |
| `Resource` | `name`, `capacity` | `name` stays free-form. `capacity` stays numeric. |
| `Worker` | `name`, `type`, `model`, `modelProvider`, `sessionId`, `provider`, `command`, `args`, `resources`, `concurrency`, `timeout`, `stopToken`, `skipPermissions`, `body` | `type`, `modelProvider`, and `provider` must become named enums. The remaining fields stay free-form, numeric, boolean, or reference-backed. |
| `Workstation` | `id`, `name`, `kind`, `type`, `worker`, `promptFile`, `outputSchema`, `timeout`, `limits`, `body`, `promptTemplate`, `cron`, `inputs`, `outputs`, `onRejection`, `onFailure`, `resourceUsage`, `guards`, `stopWords`, `runtimeStopWords`, `workingDirectory`, `worktree`, `env` | `kind` and `type` must be enum-backed. `worker` remains a reference to `Worker.name`. Templates, file paths, durations, stop-word arrays, and `env` stay free-form. |
| `WorkstationIO` | `workType`, `state`, `guards` | `workType` and `state` remain reference-backed names; `guards[*].type` must be enum-backed through `InputGuardType`. |
| `InputGuard` | `type`, `parentInput`, `spawnedBy` | `type` must reference named enum `InputGuardType`. `parentInput` and `spawnedBy` stay reference-backed. |
| `WorkstationGuard` | `type`, `workstation`, `maxVisits` | `type` must reference named enum `WorkstationGuardType`. `workstation` stays reference-backed and `maxVisits` stays numeric. |
| `WorkstationCron` | `schedule`, `triggerAtStart`, `jitter`, `expiryWindow` | Schedule and durations stay free-form. `triggerAtStart` stays boolean. |
| `WorkstationLimits` | `maxRetries`, `maxExecutionTime` | Retries stay numeric and durations stay free-form. |

## Enum-Backed Public Fields To Tighten

| Field path | Current public contract | Canonical enum target for this slice | Owning evidence |
| --- | --- | --- | --- |
| `Factory.inputTypes[].type` | Named enum `InputKind` with current value `default` | Named enum `InputKind` with canonical value `DEFAULT` | `interfaces.InputKindDefault`, `config.ruleInputTypes` |
| `Factory.workTypes[].states[].type` | Named enum `WorkStateType` already using uppercase values | Keep `INITIAL`, `PROCESSING`, `TERMINAL`, `FAILED` | `interfaces.StateType*`, generated `WorkStateType` |
| `Factory.workers[].type` | Loose string in OpenAPI and generated models | Named enum `WorkerType` with `MODEL_WORKER`, `SCRIPT_WORKER` | `interfaces.WorkerTypeModel`, `interfaces.WorkerTypeScript` |
| `Factory.workers[].modelProvider` | Loose string; runtime currently uses lowercase provider selectors | Named enum `WorkerModelProvider` with canonical public values such as `CLAUDE`, `CODEX` | `workers.ModelProviderClaude`, `workers.ModelProviderCodex` |
| `Factory.workers[].provider` | Loose string; frontmatter examples already show uppercase provider identifiers | Named enum `WorkerProvider` with canonical public values such as `LOCAL_CLAUDE` | `config.LoadWorkerConfig`, `config/agents_config_test.go` |
| `Factory.workstations[].kind` | Named enum `WorkstationKind` with current values `standard`, `repeater`, `cron` | Named enum `WorkstationKind` with `STANDARD`, `REPEATER`, `CRON` | `interfaces.WorkstationKind*`, `config.ruleWorkstationKind` |
| `Factory.workstations[].type` | Loose string; examples/runtime already use uppercase workstation types | Named enum `WorkstationType` with `MODEL_WORKSTATION`, `LOGICAL_MOVE` | `interfaces.WorkstationTypeModel`, `interfaces.WorkstationTypeLogical` |
| `Factory.workstations[].guards[].type` | Named enum `WorkstationGuardType` with current value `visit_count` | Named enum `WorkstationGuardType` with `VISIT_COUNT` | `interfaces.GuardTypeVisitCount`, `config.ruleGuards` |
| `Factory.workstations[].inputs[].guards[].type` | Named enum `InputGuardType` with current values `all_children_complete`, `any_child_failed` | Named enum `InputGuardType` with `ALL_CHILDREN_COMPLETE`, `ANY_CHILD_FAILED` | `interfaces.GuardTypeAllChildrenComplete`, `interfaces.GuardTypeAnyChildFailed`, `config.rulePerInputGuards` |

## Fields That Intentionally Stay Free-Form Or Reference-Backed

| Field family | Why it stays non-enum |
| --- | --- |
| `metadata`, `env` map keys and values | Caller-owned extension data. The schema owns the map property name, not nested customer keys. |
| Resource, worker, workstation, work-type, and state names | These are authored identifiers that reference other declared resources. The contract should document them, not enumerate them globally. |
| `model`, `sessionId`, `stopToken`, `command`, `args`, `promptFile`, `promptTemplate`, `outputSchema`, `workingDirectory`, `worktree`, `body` | These are provider-specific names, templates, paths, or command payloads whose value sets are intentionally open. |
| `timeout`, `maxExecutionTime`, `jitter`, `expiryWindow`, `schedule` | These are duration or cron-expression inputs validated by parsers, not closed enums. |
| `resources`, `resourceUsage[].name`, `worker`, `workType`, `state`, `parentInput`, `spawnedBy`, `workstation` | These fields point to other authored declarations inside the same factory. Their validity comes from cross-reference checks, not a global enum registry. |

## Representative Canonical Payload

The snippet below is the target canonical payload shape for later stories. It
uses the uppercase enum policy even where the current OpenAPI still exposes
lowercase values or loose strings.

```json
{
  "inputTypes": [
    {
      "name": "brief",
      "type": "DEFAULT"
    }
  ],
  "workTypes": [
    {
      "name": "story",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "inReview", "type": "PROCESSING" },
        { "name": "failed", "type": "FAILED" },
        { "name": "complete", "type": "TERMINAL" }
      ]
    }
  ],
  "workers": [
    {
      "name": "reviewer",
      "type": "MODEL_WORKER",
      "provider": "LOCAL_CLAUDE",
      "modelProvider": "CLAUDE",
      "model": "claude-sonnet-4-20250514"
    },
    {
      "name": "review-loop-breaker",
      "type": "MODEL_WORKER",
      "provider": "LOCAL_CLAUDE",
      "modelProvider": "CLAUDE",
      "model": "claude-sonnet-4-20250514"
    }
  ],
  "workstations": [
    {
      "name": "review-story",
      "kind": "STANDARD",
      "type": "MODEL_WORKSTATION",
      "worker": "reviewer",
      "inputs": [
        { "workType": "story", "state": "init" }
      ],
      "outputs": [
        { "workType": "story", "state": "inReview" }
      ]
    },
    {
      "name": "review-loop-breaker",
      "type": "LOGICAL_MOVE",
      "worker": "review-loop-breaker",
      "inputs": [
        { "workType": "story", "state": "inReview" }
      ],
      "outputs": [
        { "workType": "story", "state": "failed" }
      ],
      "guards": [
        {
          "type": "VISIT_COUNT",
          "workstation": "review-story",
          "maxVisits": 3
        }
      ]
    }
  ]
}
```

The loop-breaker example keeps a `worker` reference because the current public
schema still requires it on `Workstation`, even though the runtime treats
`LOGICAL_MOVE` as a pass-through workstation type.

## Inventory Conclusions

- The contract already has named enum schemas for `InputKind`,
  `WorkStateType`, `WorkstationKind`, `WorkstationGuardType`, and
  `InputGuardType`, but some of those enums still publish lowercase canonical
  values.
- `Worker.type`, `Worker.modelProvider`, `Worker.provider`, and
  `Workstation.type` are the main high-value public fields still exposed as
  loose strings even though runtime code already treats them as constrained
  concepts.
- Later stories should add descriptions and enum references only to the
  customer-authored contract fields in this inventory. They should not convert
  authored identifiers, durations, schedules, file paths, templates, or free-
  form maps into global enums.

## Verification

```bash
make docs-check
cd libraries/agent-factory && go test ./pkg/api -count=1
cd libraries/agent-factory && go test ./pkg/config -count=1
```
