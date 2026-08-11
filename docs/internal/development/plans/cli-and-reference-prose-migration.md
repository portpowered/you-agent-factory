# CLI And Packaged Reference Prose Migration Plan

## Status

Proposed. Depends on the accepted customer-writing standard and blocking
changed-prose check.

## Problem statement

Customers cannot reliably move from a goal to the correct CLI command and
detailed guide because command help and packaged reference topics are dense,
inconsistently structured, and sometimes expose internal implementation
vocabulary.

## Customer ask

Rewrite the CLI and its packaged documentation so customers can rapidly
understand the product, decompose goals into Work, submit that Work, verify the
result, and find deeper guidance.

## Intended outcome

Every visible command answers three questions in order:

1. What customer task does this command perform?
2. What is the shortest correct invocation?
3. How does the customer verify the result or get detailed help?

Every packaged topic has one clear purpose, uses the canonical public data
model, and presents task instructions before edge cases and exhaustive
reference material.

## Parent plan

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\customer-prose-clarity-program.md`

## Dependency plan

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\customer-prose-standard-and-enforcement.md`

## Scope

### In scope

- `contracts/cli/commands.json` authored help.
- Authored handwritten help that cannot yet come from the command manifest.
- Signature-aware Factory invocation help.
- `docs/reference/*.md` packaged topics and their index.
- Root help and focused human output fixtures affected by a journey.
- Required generated CLI family artifacts.
- Existing documentation, contract, and public CLI tests.
- Direct customer task-study evidence.

### Out of scope

- Renaming commands or flags solely to make prose easier to write.
- Changing JSON/NDJSON output, public schemas, exit codes, or compatibility
  aliases.
- Moving runtime policy into documentation.
- Rewriting every topic in one PR.
- Hand-editing generated CLI files.
- Updating unrelated internal documentation.

## Customer information architecture

### Root and command help

Visible help should use this order when applicable:

1. One-sentence purpose.
2. Recommended first command.
3. Required prerequisite or target.
4. Concise alternatives.
5. Verification command.
6. Route to `you docs <topic>` for detailed contracts and edge cases.

Long help must not duplicate an entire packaged guide. It must contain enough
information to select and run the command safely.

### Packaged task guides

Task-oriented topics should use this order when applicable:

1. Purpose and audience.
2. Prerequisites.
3. Choose a task or shape.
4. Minimal working procedure.
5. Expected result and verification.
6. Relevant failure recovery.
7. Contract details and advanced variants.
8. Related canonical topics.

Reference-oriented topics may lead with a compact field or behavior table but
must still include a shortest working path.

### Examples

Every example must:

- state the customer goal;
- use public vocabulary and canonical fields;
- contain only the fields needed for that behavior unless labeled complete;
- show the exact command;
- show or describe the expected result; and
- give the next verification command.

Examples must not use repository-local paths as if they are installed-product
paths unless the context explicitly identifies them as repository examples.

## Canonical terminology

All migrated prose must follow `docs/architecture/data-model.md` and the new
customer technical-term register. In particular:

- Use `Factory`, `Factory Session`, `Current Factory`, `Work`, and `Work
  Request` for customer behavior.
- Use `Worker Session` only for one supervised worker execution context.
- Do not describe the public product primarily through CPN, place, marking,
  transition, or token vocabulary.
- Preserve literal schema and event names when the customer must type or parse
  them.
- Explain a literal once when its form differs from normal customer prose.

## Migration unit

One implementation task may change:

- one complete customer journey across tightly coupled help and topics;
- one command family with its directly owned help topic; or
- one topic or no more than approximately 2,000 prose words from a larger
  topic.

An implementation task must not combine several unrelated topics merely
because they are Markdown files. A partial migration must remove only the
baseline findings that it actually fixes.

## Work stories

### Story 1: Customers can select the first command from root help

#### Observable behavior

Running `you` or `you --help` tells a customer how to start, submit, inspect,
and find detailed documentation without exposing internal orchestration
implementation as the product definition.

#### Primary sources

- `contracts/cli/commands.json` records for `you` and `you.docs`.
- `docs/reference/agents.md` orientation and topic router.
- Root help fixture and related tests.

#### Acceptance criteria

- The first screen identifies `run`, `server`, `submit`, `work`, and `docs` by
  customer goal.
- The root description uses customer-facing Factory vocabulary and contains no
  unexplained CPN terminology.
- A recommended command is visible for starting the Current Factory.
- A recommended command is visible for submitting to a running Factory
  Session.
- A verification command is visible for submitted Work.
- Root help remains side-effect-free and exits successfully.
- `you docs agents` contains a concise goal-to-command router before detailed
  contracts.
- Prose checks, root help baselines, CLI manifest checks, and docs smoke pass.

### Story 2: Customers can choose a Work decomposition shape

#### Observable behavior

A customer can determine whether a goal needs independent Work,
dependency-ordered Work, or parent-child Work and can create the corresponding
valid batch.

#### Primary sources

- `docs/reference/agents.md`.
- `docs/reference/batch-inputs.md`.
- `docs/reference/relationships.md`.
- Relevant `submit batch` command help in `contracts/cli/commands.json`.

#### Acceptance criteria

- One compact decision table distinguishes independent, `DEPENDS_ON`, and
  `PARENT_CHILD` Work.
- Each shape has one minimal valid `FACTORY_REQUEST_BATCH` example.
- Each batch uses a stable, non-empty `requestId` and canonical camelCase
  fields.
- The guidance distinguishes sequencing from containment.
- The guidance states when a retry must reuse the same `requestId`.
- Each procedure ends with `you submit batch` and a public verification path.
- Existing relationship and batch contract tests confirm that the examples are
  valid.
- The affected baseline findings are removed.

### Story 3: Customers can submit and verify Work without duplicate ingress

#### Observable behavior

A customer can choose unary or batch submission, target the intended Factory
Session, and verify acceptance without confusing startup input, watched files,
HTTP ingress, or repeated unary submission.

#### Primary sources

- `you submit` and `you submit batch` help.
- `docs/reference/agents.md`.
- `docs/reference/work.md`.
- `docs/reference/sessions.md` routing sections.

#### Acceptance criteria

- The guidance gives one rule for choosing unary versus batch submission.
- It clearly distinguishes a running Factory Session from local Factory files.
- It explains default and explicit `--server` and `--session` targets once and
  routes detailed ownership to `sessions`.
- It identifies duplicate behavior for unary submit and idempotent behavior for
  batch upsert.
- Human success output routes to `you work show` when a Work ID exists and to a
  narrowed list command otherwise.
- JSON output remains machine-readable and unchanged unless a separate contract
  change is approved.
- Focused `root.BuildProcess` functional coverage proves one submit-and-verify
  path through the CLI.

### Story 4: Customers can select and verify a run shape

#### Observable behavior

A customer can distinguish Current Factory, explicit Factory, named Factory,
continuous, hosted, replay, and mock-worker runs before starting execution.

#### Primary sources

- `you run` and `you server` help.
- `docs/reference/run.md`.
- `docs/reference/operations.md`.
- `docs/reference/record-replay.md`.
- `docs/reference/mock-workers.md`.

#### Acceptance criteria

- A compact task table maps each supported goal to one canonical invocation.
- Prerequisites and lifecycle effects are stated before each invocation.
- Current Factory selection is explained without internal implementation
  vocabulary.
- Continuous and hosted modes state who owns the listener and when it stops.
- Recording defaults and artifact sensitivity remain accurate.
- Each run procedure states its terminal or verification behavior.
- Existing run help, recording, invocation, and terminal-policy tests pass.

### Story 5: Customers can author and validate a Factory

#### Observable behavior

A customer can create the minimum Factory structure, validate it, run it, and
understand the public roles of Work, Workers, and Workstations.

#### Primary sources

- `docs/reference/authoring-factories.md`.
- `docs/reference/config.md`.
- `docs/reference/factory-validation.md`.
- Factory/config/init command-family help.

#### Acceptance criteria

- The shortest valid authoring path appears before comprehensive schema detail.
- Each public noun has one consistent definition and role.
- Validation is a required step immediately before first execution.
- The minimum example uses canonical public fields and passes the validation
  command in a focused fixture or smoke test.
- Split and single-file layouts are clearly distinguished.
- Failure guidance names observable validation output and the next correction
  action.

### Story 6: Customers can select execution components

#### Observable behavior

A customer can distinguish Worker, Workstation, Provider, Model, resource, and
prompt-template responsibilities and route to the correct command or guide.

#### Primary sources

- Worker, Worker Session, Provider, and Model command families.
- `docs/reference/workers.md`.
- `docs/reference/workstations.md`.
- `docs/reference/providers.md`.
- `docs/reference/models.md`.
- `docs/reference/resources.md`.
- `docs/reference/templates.md`.

#### Acceptance criteria

- Each topic starts with its owned customer question and points to adjacent
  owners without duplicating their complete contract.
- `Worker` and `Worker Session` are not used interchangeably.
- `Worker` and `Workstation` configuration responsibilities are distinct.
- Provider and Model selection guidance preserves capability and readiness
  caveats.
- Every shown inspection command identifies its expected result.
- Existing provider, model, worker, and docs tests remain passing.

### Story 7: Customers can host and integrate the CLI

#### Observable behavior

A customer can distinguish HTTP/dashboard hosting, MCP hosting, ACP-agent
hosting, and JavaScript workflow execution and can select the correct entry
point.

#### Primary sources

- `serve`, `mcp`, and ACP command-family help.
- `docs/reference/mcp.md`.
- `docs/reference/serve-acp.md`.
- `docs/reference/javascript-workflows.md`.
- Applicable provider integration sections.

#### Acceptance criteria

- One router distinguishes the supported host and protocol shapes.
- Each procedure states transport, process lifetime, prerequisite, and first
  verification action.
- Stdio requirements and stdout/stderr policy remain explicit where applicable.
- JavaScript workflow execution routes through canonical Factory and Factory
  Session surfaces.
- Protocol smoke and documentation tests pass.

### Story 8: Every remaining packaged topic conforms to the accepted profile

#### Observable behavior

Customers receive consistent purpose, procedure, result, recovery, and routing
language in every packaged topic not completed by Stories 1–7.

#### Execution rule

Create one task per topic or per approximately 2,000-word section of a larger
topic. Use current customer exposure and baseline severity to order the queue.

#### Acceptance criteria for each task

- The topic has one declared purpose and canonical concept owner.
- The first working path appears before exhaustive detail.
- Procedures state prerequisites, actions, expected results, and verification.
- Descriptive sections use one topic per paragraph.
- Cross-references use runnable `you docs <topic>` syntax in packaged output.
- Code and schema examples remain byte-accurate where contract accuracy
  requires it.
- Automated findings for the migrated scope are zero.
- A language reviewer and subject-matter reviewer record approval.
- `make docs-reference-smoke` passes.

## Topic migration queue

The queue is dependency-aware. A topic may move earlier when active product
work changes it, but the implementing task must still satisfy the per-topic
acceptance criteria.

### Priority A: orientation and Work decomposition

- `agents.md`
- `authoring-factories.md`
- `batch-inputs.md`
- `relationships.md`
- `work.md`
- `guards.md`

### Priority B: execution and operation

- `run.md`
- `sessions.md`
- `operations.md`
- `record-replay.md`
- `mock-workers.md`
- `factory-validation.md`

### Priority C: Factory components

- `config.md`
- `workstations.md`
- `workers.md`
- `resources.md`
- `templates.md`
- `authoring-agents-md.md`

### Priority D: providers and integrations

- `providers.md`
- `models.md`
- `mcp.md`
- `serve-acp.md`
- `javascript-workflows.md`
- `orchestrators.md`

### Priority E: catalogs and supporting reference

- `packaged-factories.md`
- `references.md`
- `harnesses.md`
- `the-zen-of-flow.md`
- `README.md`

Before implementation, confirm whether topics not currently listed in the
packaged index are intentional public topics, maintainer-only pages, or stale
content. Do not silently package or delete them as part of prose rewriting.

## CLI family migration procedure

For each command-family task:

1. Capture the current public command tree, help, examples, flags, and required
   facts.
2. Identify the customer task and canonical detailed topic.
3. Rewrite authored fields in `contracts/cli/commands.json`.
4. Update only the minimum handwritten help that cannot be projected from the
   manifest.
5. Run `make cli-manifest-generate`.
6. Review generated changes but do not edit them directly.
7. Update behavioral goldens only after the new output passes subject-matter
   and language review.
8. Run CLI manifest, contract, focused behavior, prose, and documentation
   checks.
9. Remove only the corrected baseline entries.

## Documentation migration procedure

For each topic task:

1. Record the audience, customer task, canonical concept owner, and required
   facts.
2. Capture runnable examples and current verification commands.
3. Reorder content into the accepted topic pattern before sentence-level
   rewriting.
4. Rewrite one bounded section at a time without changing code or schema
   literals.
5. Validate examples through existing contracts or focused smoke fixtures.
6. Run the prose checker and remove corrected baseline entries.
7. Obtain language and subject-matter review.

## Verification

- Prose check for every changed canonical source.
- `make docs-reference-smoke` for packaged topics.
- `make cli-manifest-generate` and `make cli-manifest-check` for command help.
- `make cli-contract-smoke` for CLI contract behavior.
- Focused Cobra/root help and signature-aware help tests.
- Focused `root.BuildProcess` functional tests for high-value customer journeys.
- Contract validation for JSON/YAML examples.
- Pre-change and post-change customer task study.
- `make verify-fast`, `make lint`, and applicable PR verification before merge.

## Project-level acceptance criteria

- Every visible command family and packaged topic in scope has zero automated
  findings and recorded manual review.
- Root help and `you docs agents` route customers to start, submit, decompose,
  verify, and find detailed help.
- Independent, `DEPENDS_ON`, and `PARENT_CHILD` decomposition examples validate
  against the shipped Work Request contract.
- No migration changes JSON/NDJSON shape, exit codes, command names, flags,
  compatibility aliases, or runtime behavior without separate approval.
- All generated CLI artifacts are current.
- The CLI/reference portion of the temporary baseline is empty.
- The post-change customer task study meets the program targets.
- Required CI is terminal and passing, all blocking review feedback is
  addressed, conflicts are resolved, and each PR is merged.

## Work-story task packets

### Task packet: root orientation

# problem statement

Root help leads with internal implementation terminology and does not give the
shortest goal-to-command route for common customer tasks.

## customer ask

Let a new customer select how to start, submit, inspect, or open detailed help
from the first CLI screen.

## solution

Rewrite root and docs command help with customer-facing Factory vocabulary and
align the top of the packaged agent-orientation guide to the same router.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\cli-and-reference-prose-migration.md`

# changes

## package changes

- Update `you` and `you.docs` authored records in
  `contracts/cli/commands.json`.
- Update the orientation and topic-router sections of
  `docs/reference/agents.md`.
- Regenerate CLI family projections and update reviewed root help fixtures.

## contracts

- Preserve command identity, flags, aliases, exits, and output-channel policy.

## services

- None.

## API changes

- None.

## tests

- Update root help behavior/golden tests, CLI manifest checks, prose checks,
  and packaged docs smoke.

### Task packet: Work decomposition

# problem statement

Customers must assemble guidance from several topics to choose between
independent, dependency-ordered, and parent-child Work.

## customer ask

Create a valid decomposition and submit it without guessing relationship
semantics or retry behavior.

## solution

Add one decision path and three minimal validated batch examples across the
canonical agent, batch-input, relationship, and submit-batch surfaces.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\cli-and-reference-prose-migration.md`

# changes

## package changes

- Update bounded sections in `docs/reference/agents.md`,
  `docs/reference/batch-inputs.md`, and `docs/reference/relationships.md`.
- Update `you submit batch` authored help in `contracts/cli/commands.json`.
- Regenerate affected CLI family artifacts.

## contracts

- Preserve `FACTORY_REQUEST_BATCH`, `requestId`, Work fields, and relationship
  semantics.

## services

- None.

## API changes

- None.

## tests

- Validate all three example shapes, run CLI manifest/prose/docs checks, and
  add or update one public batch-submit smoke path.

### Task packet: submit and verify

# problem statement

Customers can confuse startup Work, unary submission, batch upsert, watched
files, and API ingress, which can cause failed targeting or duplicate Work.

## customer ask

Choose the correct CLI ingress, target one Factory Session, and verify the
accepted Work.

## solution

Align submit command help and the canonical Work/session documentation around
one unary-versus-batch decision and one submit-to-verify loop.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\cli-and-reference-prose-migration.md`

# changes

## package changes

- Update `you.submit`, `you.submit.batch`, and relevant Work command help.
- Update bounded submission and routing sections in `agents.md`, `work.md`, and
  `sessions.md`.
- Regenerate CLI family artifacts.

## contracts

- Preserve unary duplicate behavior, batch idempotency, server/session routing,
  human output facts, and JSON output shape.

## services

- None unless a separately discovered documentation defect requires an
  approved runtime behavior fix.

## API changes

- None.

## tests

- Update CLI help/contract tests and add one `root.BuildProcess`
  submit-and-verify functional scenario using public CLI observations.

### Task packet: one command-family migration

# problem statement

One visible command family does not conform to the accepted customer-writing
standard or route customers to its canonical detailed guide.

## customer ask

Understand and use the selected command family without reading unrelated
implementation details.

## solution

Rewrite the selected family as one customer-behavior slice, regenerate its
derived artifacts, and prove help and runtime contracts remain aligned.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\cli-and-reference-prose-migration.md`

# changes

## package changes

- Update only the selected authored command-family records and directly owned
  handwritten help.
- Update the directly owned topic when routing facts must change.
- Regenerate CLI family projections.

## contracts

- Preserve command identity, inputs, aliases, channels, exits, and generated
  manifest shape.

## services

- None.

## API changes

- None.

## tests

- Run family-focused help tests, CLI manifest/contract checks, prose checks,
  and the owned topic smoke tests.

### Task packet: one packaged-topic migration

# problem statement

One packaged topic does not let customers complete its owned task through
clear, progressively disclosed instructions.

## customer ask

Use the topic to complete one defined task and verify the result without
searching overlapping guides.

## solution

Restructure and rewrite one topic or one bounded large-topic section, validate
its examples, and remove its corrected prose baseline findings.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\cli-and-reference-prose-migration.md`

# changes

## package changes

- Update one canonical `docs/reference` topic or one approximately 2,000-word
  section.
- Update the topic index only when ownership or routing changes.

## contracts

- Preserve literal commands, configuration fields, schemas, routes, and
  customer-visible behavior.

## services

- None.

## API changes

- None.

## tests

- Validate runnable examples, run prose and docs-reference smoke checks, and
  update focused documentation assertions only when required facts change.
