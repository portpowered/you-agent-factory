# CLI Command Shape Standardization Plan

## Status

Proposed. This is a breaking public CLI contract migration. The repository is
currently its own primary consumer, so the plan deliberately prefers one
coherent command tree over compatibility aliases.

All old command literals in this plan are historical migration records. They
describe the source side of an intentional cutover; they are not active
customer invocations. Active guidance and the executable target tree use only
the target paths.

## Problem statement

The CLI has a strong manifest and drift-detection system, but it does not have
an enforced command-language standard. As a result, individually reasonable
commands have accumulated into an inconsistent whole:

- protocol hosts use both `you serve acp` and `you mcp serve`, while the HTTP
  host is the preferred noun command `you server`;
- resource families mix singular (`factory`, `session`, `work`) and plural
  (`models`, `providers`, `workers`, `worker-sessions`) names;
- `session dispatches` exposes a low-value dispatch inventory without a useful
  customer workflow and should not survive the breaking cleanup;
- provider configuration is exposed under `workers acp`, despite Providers
  owning provider identity, configuration, protocol, and adapter choice.

The existing CLI contract protects whatever shape is authored, but it cannot
say whether that shape is coherent. New commands can therefore pass contract
checks while widening the vocabulary and hierarchy drift.

## Customer ask

Give the `you` CLI one predictable grammar so a customer can infer a command
without memorizing family-specific exceptions. Breaking changes are accepted;
do not preserve old paths merely because they already exist.

## Intended outcome

A customer can predict that:

1. resource management uses the agreed public family noun, including singular
   `worker-session`;
2. workflow commands use one documented action family and consistent variants;
3. bare `you server` keeps its HTTP/dashboard behavior and protocol hosts use
   the predictable `you server acp|mcp` variants;
4. established `init`, `factory`, and `session` command paths stay stable
   except for `factory query` and the explicit removal of
   `session dispatches`;
5. existing arguments, flags, shorthands, defaults, and precedence remain
   stable while command paths change; and
6. the authored CLI manifest fails verification when a new command violates
   these rules.

## Governing sources

- `docs/internal/standards/code/planning-standards.md` governs the shape and
  delivery requirements of this plan.
- `docs/internal/standards/code/code-review-standards.md` and
  `docs/internal/standards/code/general-backend-standards.md` govern
  implementation, public-contract proof, generated artifacts, and functional
  testing.
- `docs/architecture/data-model.md` owns public resource vocabulary.
- `docs/architecture/architecture.md` and
  `docs/architecture/packaged-structure.md` own service and transport
  boundaries.
- `contracts/cli/commands.json` remains the authored public CLI contract.
- `docs/internal/development/plans/cli-and-reference-prose-migration.md`
  remains the prose migration plan. It should consume the final command names
  from this plan rather than independently renaming commands.

## Audit scope and method

The audit inspected the authored manifest, executable command-tree baseline,
root help baseline, generated family declarations, compatibility ledgers,
command construction and handler registration, packaged reference topics,
functional scenario contracts, and the CLI-related Make targets.

The current authored manifest contains 54 commands: 42 runnable commands and
12 non-runnable grouping commands. It has no command aliases, no deprecated
command records, and empty compatibility ledgers.

This is a command-path, noun, verb, and help-vocabulary audit. Argument and flag
renames, input remapping, output-mode changes, and runtime/API feature changes
are explicitly out of scope.

## Audit findings

| Priority | Finding | Evidence | Required correction |
| --- | --- | --- | --- |
| P0 | Hosting has no single grammar. | `you server`, `you serve acp`, and `you mcp serve` represent peer long-running hosts. | Keep bare `you server` as the HTTP/dashboard host and place protocol variants at `you server acp|mcp`. |
| P0 | Resource ownership is misleading. | `you workers acp add|delete` manages Provider integrations and calls Providers-owned behavior. | Move configuration to the singular `provider` family and remove the duplicate `workers` catalog. |
| P1 | Family nouns are not systematic. | `factory`, `session`, and `work` coexist with `models`, `providers`, `workers`, and `worker-sessions`. | Use the singular `worker-session`, `provider`, and `model` families selected by the breaking map. |
| P1 | Dispatch inventory has no useful customer job. | `you session dispatches <session-id>` exposes durable dispatch records but is not needed for the retained Session or Worker Session inspection workflows. | Remove the command without a successor. |
| P1 | Current Factory inspection uses a generic read verb. | `you factory query` shows the Current Factory. | Rename only that leaf to `you factory show`, preserving output, failures, and inputs. |
| P1 | Local rendering uses a broad verb without changing resource ownership. | `work visualize` reads a local batch and renders Mermaid; it does not inspect or mutate live Work. | Keep the behavior in `work`, rename the operation to `render`, and make its local-only behavior explicit. |
| P2 | The contract checker enforces equality, not style. | The manifest, generated families, and Cobra tree can agree on an inconsistent new command. | Add a pure CLI-style evaluator and run it from `cli-manifest-check` and `cli-contract-smoke`. |
| P2 | Public help still contains non-canonical language. | Root help defines the product as CPN-based and command titles vary in capitalization of Factory terms. | Apply canonical public vocabulary while each family moves; do not create a second prose-only rename. |

## Target CLI language standard

### 1. Command shapes

Every public command must use one of these shapes:

```text
you <workflow> [variant] [operand] [flags]
you <resource> <verb> [operand] [flags]
you <resource> <subresource> <verb> [operand] [flags]
```

The approved top-level workflow families are `docs`, `run`, and `submit`.
`server` is the one approved runnable host family: bare `you server` hosts the
HTTP API/dashboard and its `acp` and `mcp` children host protocol variants. A
new workflow or runnable noun family requires an explicit update to the CLI
style policy; it must not be introduced as an undocumented exception.

Resource and subresource tokens are lowercase kebab-case. New family names are
singular unless the target tree records an explicit exception. The approved
root resource families for this migration are `config`, `factory`, `model`,
`provider`, `session`, `work`, `worker-session`, and the established root
`init` command.

### 2. Verb lexicon

| Verb | Meaning | Do not substitute |
| --- | --- | --- |
| `list` | Return a collection, optionally filtered. | `query`, a plural collection noun |
| `show` | Return one resource in families that already use or explicitly migrate to `show`. | New unreviewed synonyms |
| `create` | Run an established resource or Factory Session creation operation. | New synonyms without an explicit decision |
| `update` | Replace or modify an existing persisted resource. | `set`, `edit` |
| `delete` | Run an established resource or Factory Session deletion operation. | New synonyms without an explicit decision |
| `pause` / `resume` | Change a pausable runtime's lifecycle. | `stop` / `start` |
| `add` | Add a member or integration where replacement semantics are explicit. | `create` when duplicate creation must fail |
| `move` | Move Work to an authored state. | `update`, `transition` |
| `render` | Convert a local source document to a presentation format without mutating runtime state. | `visualize`, `show` |
| `validate`, `flatten`, `expand`, `invoke`, `pull`, `read`, `stream`, `replace-current` | Established domain operations retained on their existing families. | New synonyms without an explicit plan decision |

### 3. Server grammar

`server` is a runnable noun family. The bare command keeps the established HTTP
API/dashboard behavior; child tokens select protocol hosts:

```text
you server
you server acp
you server mcp
```

Each command must state transport, process lifetime, stdout/stderr policy, and
shutdown behavior. Bare `server` owns the Factory API and embedded dashboard
listener; ACP and MCP remain stdio unless their contracts explicitly add
another transport. `server` is an intentional command-language exception and
must not be generalized into noun-only operations in other families.

### 4. Input stability boundary

This migration changes command paths, removes `session dispatches`, and updates
help vocabulary. It preserves the current public input contract of every
retained command:

- root-global `--server` remains the shared server address input with its
  current client and host behavior;
- root-global `--json` remains the shared structured-output switch and is
  inherited through commands that expose supported JSON behavior;
- `--session` remains the preferred Factory Session input name;
- Factory-root `--dir` remains unchanged;
- all `run` argument and flag names, shorthands, defaults, conflicts, and
  precedence remain unchanged;
- all model invocation inputs, including `model invoke --output`, remain
  unchanged;
- Worker Session commands retain `--work-id`, `--provider`, `--kind`, `--id`,
  `--output`, `--follow`, and `--replay-only` as applicable;
- Provider integration and Work submission inputs retain their current names
  and resolution behavior; and
- hidden compatibility inputs and existing `--port` behavior are not removed
  or redesigned by this project.

Renaming a command updates stable command and handler identifiers as required,
but it must not opportunistically rename an argument or flag. A separate plan
and explicit approval are required for any future input-contract migration.

### 5. Help and public vocabulary

- Titles begin with the approved verb and name the canonical public resource.
- Help describes customer-visible behavior before composition or package
  details.
- `Factory`, `Factory Session`, `Current Factory`, `Work`, `Work Request`,
  `Worker Session`, and `Provider Session` follow
  `docs/architecture/data-model.md`.
- Public root help must not define the product through CPN, token, place,
  transition, or marking terminology.
- Every renamed command gives one shortest invocation and one verification or
  follow-up command in its owned help or packaged topic.

## Target command tree

The target below is the complete naming decision for current public behavior.
It is not permission to add unrelated resource operations.

```text
you
├── config
├── docs
├── factory
│   ├── create
│   ├── delete
│   ├── list
│   ├── show
│   ├── replace-current
│   ├── update
│   └── config
│       ├── validate
│       ├── flatten
│       └── expand
├── init
├── session
│   ├── create
│   ├── delete
│   ├── list
│   ├── show
│   ├── pause
│   └── resume
├── work
│   ├── list
│   ├── show
│   ├── move
│   ├── watch
│   └── render
├── worker-session
│   ├── list
│   ├── show
│   ├── read
│   └── stream
├── provider
│   ├── list
│   ├── add
│   └── delete
├── model
│   ├── list
│   ├── show
│   ├── invoke
│   └── pull
├── run
├── submit
│   └── batch
└── server
    ├── acp
    └── mcp
```

`submit batch` uses Cobra's existing runnable-parent pattern: `submit`
continues to submit one Work item, while `submit batch` upserts a batch-shaped
Work Request. Its current positional, file, stdin, and inline JSON resolution
behavior remains unchanged.

`worker-session` retains the four existing `list`, `show`, `read`, and `stream`
operations. Provider integration configuration still moves to `provider`; it
does not move into `worker-session`.

## Breaking command map

| Current command | Target command | Disposition |
| --- | --- | --- |
| `you init` | unchanged | Preserve the established root initialization command. |
| `you factory query` | `you factory show` | Use the established single-resource read verb. |
| every other `you factory ...` command | unchanged | Preserve the rest of the Factory family, including `replace-current`. |
| `you session create|delete|list|show|pause|resume` | unchanged | Preserve the existing Factory Session family and verbs. |
| `you session dispatches` | removed | Delete the low-value command without a successor. |
| `you work visualize` | `you work render` | Keep the behavior under Work and use the precise local transformation verb. |
| `you worker-sessions list|show|read|stream` | `you worker-session list|show|read|stream` | Singularize the family and preserve all existing operations. |
| `you workers list` | `you provider list` | Remove the overlapping filtered Provider catalog. |
| `you workers acp add` | `you provider add` | Move provider configuration to Providers ownership. |
| `you workers acp delete` | `you provider delete` | Move provider configuration to Providers ownership. |
| `you providers list` | `you provider list` | Use the singular resource family. |
| `you models list|invoke|pull` | `you model list|invoke|pull` | Use the singular resource family. |
| `you models inspect` | `you model show` | Use the canonical single-resource read verb. |
| `you server` | unchanged | Preserve the preferred bare HTTP/dashboard host. |
| `you serve acp` | `you server acp` | Join the server family. |
| `you mcp serve` | `you server mcp` | Join the server family. |
| `you submit batch` | unchanged | Keep batch submission under the existing workflow family. |

Commands not listed in the map retain their path and input contract. Changed
families adopt only the command-path, help, and vocabulary rules in this plan.

## Compatibility and release policy

This migration intentionally does not add aliases or hidden executable copies
of old command paths. Each migrated path is removed from the canonical
manifest and production tree in the same change that adds its successor.

Before implementation begins, record the complete moves and removals in
`pkg/transports/cli/baseline/testdata/intentional_changes.md`. The CLI
compatibility manifests (`contracts/cli/deprecated-commands.json` and
`contracts/cli/deprecated.json`) are reserved for callable, separately
approved compatibility surfaces. A no-alias breaking cutover leaves those
manifests empty at final reconciliation; its historical move/removal evidence
lives in the intentional-change ledger and release notes. These records are
planning and migration evidence, not an instruction to keep compatibility
commands alive.

The release notes must contain the old-to-new command map. Old paths
must fail as unknown commands after cutover, and tests must prove their absence.
Generated files are regenerated from canonical sources and are never edited by
hand.

## Work stories

### Story 1: Contributors receive an enforceable CLI language contract

#### Observable behavior

An authored command with an unapproved plural resource family, unapproved read
synonym, or unapproved server shape fails the CLI contract check with a
path-specific correction.

#### Acceptance criteria

- A versioned CLI style policy records workflow families, resource tokens,
  approved verbs, the server exception, and explicitly allowed domain verbs.
- A pure evaluator reports deterministic findings for command path and
  help-vocabulary violations without constructing a second runtime graph.
- Table-driven tests cover every rule and at least one valid synthetic command
  for each approved shape.
- Existing production findings are captured as a temporary, explicit baseline;
  each later story removes only the findings it fixes.
- `make cli-manifest-check` and `make cli-contract-smoke` run the evaluator.
- The evaluator checks authored behavior and metadata, not repository file
  topology.

### Story 2: Customers find every host under one server family

#### Observable behavior

Customers use bare `you server` for HTTP/dashboard hosting and
`you server acp|mcp` for protocol hosting, and can infer the peer commands from
any one host's help.

#### Acceptance criteria

- Bare `you server` retains its existing HTTP API/dashboard behavior.
- `you serve acp` and `you mcp serve` are absent; `you server acp` and
  `you server mcp` preserve their behavior.
- Each surface preserves its existing transport, runtime composition,
  lifecycle, protocol frames, diagnostics channel, cancellation, and clean
  shutdown behavior.
- Bare HTTP hosting and both protocol children preserve their current global,
  inherited, and local inputs, including root `--server` and `--json` behavior.
- Root help and the server family give one three-row host router.
- ACP and MCP stdio smoke tests prove stdout remains protocol-only.
- HTTP host functional coverage uses `root.BuildProcess` and
  `Process.Execute`; no second application graph is introduced.
- Owned reference topics and generated CLI artifacts use only target paths.

### Story 3: Customers retain initialization and use factory show

#### Observable behavior

Customers continue to use root `init` and the existing `factory` family, with
only `factory query` becoming the clearer `factory show`.

#### Acceptance criteria

- `you init` remains a root command with its current behavior and inputs.
- `you factory show` preserves the output, typed failures, side effects, and
  inputs of `you factory query`.
- `you factory create|delete|list|replace-current|update` remain at their
  current paths.
- `you factory config validate|flatten|expand` remain at their current paths.
- Factory commands preserve their existing arguments, `--dir`, `--from`,
  `--server`, `--session`, `--json`, and compatibility inputs as applicable.
- Focused unit, HTTP mapping, CLI contract, and `root.BuildProcess` behavior
  tests prove these paths did not drift.

### Story 4: Customers no longer see the unused dispatch inventory

#### Observable behavior

Customers retain the established Factory Session commands without an unused
`session dispatches` branch or a replacement under `worker-session`.

#### Acceptance criteria

- `session create|delete|list|show|pause|resume` remain unchanged in the
  manifest, help, docs, scenarios, generated IDs, constructors, registries,
  and baselines.
- `session dispatches` is absent from the authored manifest, generated command
  families, executable tree, completion, help, docs, scenarios, and baselines.
- No `worker-session dispatches` successor is introduced.
- Dispatch services and HTTP/API behavior remain unchanged; this story removes
  only the public CLI command and its transport wiring.
- Focused CLI absence tests prove the retired path is unknown while ordinary
  Session and Worker Session commands remain functional.

### Story 5: Customers submit and inspect Work and Work Requests without noun drift

#### Observable behavior

Customers use `submit` for one Work item, `submit batch` for a batch-shaped Work
Request, and `work render` for a local dependency diagram without introducing
a separate Work Request command family.

#### Acceptance criteria

- Unary submission retains `--name`, `--payload`, `--work-type-name`,
  `--server`, and `--session` with their current requirements and precedence.
- `submit batch` retains its current positional, `--file`, stdin, inline JSON,
  `--dry-run`, `--server`, and `--session` behavior.
- Work Request submission preserves current dry-run, idempotency, output, and
  typed failure behavior.
- `work render` remains read-only, contacts no server, and preserves
  Mermaid and Markdown-Mermaid formats.
- Renaming `work visualize` to `work render` preserves its
  `<batch-file.json>`, `--format`, global flags, outputs, and local-only
  behavior.
- The old `work visualize` path is absent; `submit batch` remains canonical.
- Submission, inspection, watch, and render functional scenarios and packaged
  docs use target commands.

### Story 6: Customers use the singular worker-session family

#### Observable behavior

Customers use `worker-session list|show|read|stream` under one singular family
without learning new operations or a new input map.

#### Acceptance criteria

- `worker-session list` preserves required `--work-id` and optional `--session` and
  `--output` behavior.
- `worker-session show|read|stream` preserve the
  existing `--provider`, `--kind`, `--id`, `--session`, and `--output` inputs.
- `worker-session stream` also preserves `--follow` and `--replay-only`
  behavior and their current conflict rules.
- `show`, `read`, and `stream` preserve finite, replay-only, live,
  terminal, gap, and cancellation behavior as applicable.
- Root `--json` remains exposed through the normal inherited CLI contract;
  command-local `--output` behavior remains unchanged.
- Contract, projection, stream, CLI, and focused functional tests prove the
  renamed paths without API or service-contract changes.

### Story 7: Customers manage Providers and Models under their owning resources

#### Observable behavior

Customers use singular `provider` and `model` families; provider discovery and
operator-configured ACP integration management no longer appear under
`workers`.

#### Acceptance criteria

- `provider list` is the one complete catalog for built-in and
  operator-configured providers, including availability, source, protocol,
  and capabilities needed to replace `workers list`.
- `provider add` preserves `--name`, `--transport`, and `--argument`, including
  current validation and explicit replacement behavior.
- `provider delete` preserves required `--name`, cannot delete immutable
  built-in metadata, and reports the existing typed outcomes.
- `model show` replaces `models inspect`; list, invoke, and pull behavior is
  otherwise unchanged.
- `model invoke` and every one of its existing inputs remain unchanged.
- No Provider integration configuration remains under `worker-session`; that
  family remains reserved for Worker Session inspection.
- Provider/Model CLI service tests, generated manifests, docs, and relevant
  functional scenarios pass.

### Story 8: Every renamed command preserves the established input contract

#### Observable behavior

Customers can adopt the standardized command paths without relearning flags,
arguments, shorthands, defaults, input precedence, or output selection.

#### Acceptance criteria

- Root-global `--server` remains unchanged across client and host behavior.
- Root-global `--json` remains exposed through the inherited CLI contract and
  keeps current command-specific behavior.
- `--session` remains the preferred Factory Session input name.
- Factory-root `--dir` remains unchanged.
- Every `run` input remains byte-for-byte aligned with the pre-migration
  authored contract, including `--factory`, `--named` / `-a`, and `--output`.
- Every `model invoke` input remains unchanged.
- Worker, Provider, Work, submit, and server input maps remain unchanged when
  their command paths move.
- Input precedence, environment/config bindings, shell completion, diagnostic
  channels, exit codes, and sensitive-value handling remain explicit in the
  authored contract.
- The production style baseline is empty.

### Story 9: Customers and maintainers see only the standardized CLI

#### Observable behavior

Root help, command help, packaged docs, examples, completion, scenarios, and
published CLI metadata agree on the target tree, and old commands are absent.

#### Acceptance criteria

- Root help uses canonical Factory vocabulary and contains no unexplained CPN
  implementation terminology.
- Every renamed family updates its directly owned `docs/reference` topic and
  goal-to-command routing.
- README examples, functional scenario contracts, completion tests, baseline
  fixtures, published package metadata, and internal active plans that
  prescribe commands use the target paths.
- Historical proposals may retain old command literals only when explicitly
  labeled historical; active guidance must not route customers to them.
- `rg`-based audit output for each retired path is either empty or contains
  only an approved historical/migration record.
- The intentional-change ledger is reconciled with the final executable tree.
- Documentation smoke, CLI contract checks, generation checks, focused
  functional tests, `make verify-fast`, `make lint`, and the applicable PR
  verification tier pass.

## Dependency-aware delivery order

1. Land Story 1 so every subsequent family migration removes style findings.
2. Land Stories 2 through 7 as vertically sliced family changes. Stories 3
   through 7 may proceed independently after Story 1.
3. Apply Story 8 as the whole-tree proof that no argument or flag drifted while
   command paths moved.
4. Complete Story 9 after all target paths exist; it is the final whole-tree
   absence and routing proof, not a substitute for family-owned docs and tests.

Each merged story must leave the canonical manifest, generated family metadata,
executable tree, and changed documentation internally consistent. Temporary
style exceptions are allowed only through the explicit migration baseline and
must name the later story that removes them.

## Verification

Use the narrowest relevant proof in each story, then run the shared contract
and generated-artifact checks:

- `make cli-manifest-generate`
- `make cli-manifest-check`
- `make cli-contract-smoke`
- focused tests in `pkg/transports/cli/...` and the owning service transport
- focused `root.BuildProcess` plus `Process.Execute` functional tests for
  customer-visible cross-boundary behavior
- `make docs-reference-smoke` for changed packaged topics
- `make verify-fast`
- `make lint`
- `make verify-pr` before merge when the implementation reaches PR scope

Generation commands may update generated CLI artifacts, but implementation
must review those diffs and must not edit generated files directly.

## Project-level acceptance criteria

- The target command tree is the only visible executable tree.
- The CLI style evaluator reports zero unbaselined findings, and the temporary
  production style baseline is empty.
- All old command paths fail as unknown commands; no alias or hidden
  compatibility command remains.
- All current behavior has an explicit target path or an explicit retirement
  decision in this plan.
- Public resource names align with `docs/architecture/data-model.md`, and
  service ownership aligns with the repository package boundaries.
- CLI, HTTP, MCP, and ACP parity is preserved where shared contracts require
  it; no service or API contract changes are introduced.
- Authored manifests, generated families, published CLI metadata, completion,
  help, docs, scenarios, and behavioral baselines are current.
- Required CI is terminal and passing, all blocking review feedback is
  addressed, merge conflicts are resolved, and every implementation PR is
  actually merged. Opening a PR or reaching green CI without merge is not
  project completion.

## Out of scope

- Adding new Factory, Work, Worker, Provider, or Model capabilities merely to
  make every family symmetrical.
- Changing runtime scheduling, event semantics, replay semantics, or
  provider/model selection policy.
- Renaming, removing, relocating, or changing the semantics of any argument,
  flag, shorthand, default, source binding, precedence rule, or output selector
  on a retained command.
- Replacing root `--server` or `--json`, preferred `--session`, Factory-root
  `--dir`, any `run` input, any `model invoke` input, or the current Worker
  Session Provider Session identity inputs.
- Changing OpenAPI, service contracts, or JSON/NDJSON schemas.
- Rewriting all packaged documentation beyond directly affected command
  routes and required canonical terminology.
- Retaining compatibility aliases for hypothetical external consumers.
- Hand-editing generated artifacts.
