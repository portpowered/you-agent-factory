# Customer Prose Standard And Enforcement Plan

## Status

Proposed.

## Problem statement

Authors and reviewers cannot consistently determine whether customer-facing
CLI and documentation prose is acceptable because the repository has no
normative writing standard, no product terminology register, and no prose-aware
quality gate.

## Customer ask

Create a durable writing and enforcement system, aligned with ASD-STE100, that
makes CLI help and customer documentation easier to understand and prevents new
prose regressions.

## Intended outcome

An author can run one repository command and receive actionable findings for
all deterministic rules that apply to changed public prose. A reviewer can use
one complementary checklist for semantic, vocabulary, and grammatical rules
that require human judgment. The system protects commands, code, identifiers,
and public contracts while it evaluates natural-language text.

## Parent plan

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\customer-prose-clarity-program.md`

## External references

- [ASD-STE100 Simplified Technical English, Issue 9](https://www.asd-ste100.org/assets/files/ASD-STE100_ISSUE9.pdf)
  is the controlled-language reference for the aligned repository profile.
- [Official ASD-STE100 frequently asked questions](https://www.asd-ste100.org/STE_faq.html)
  defines the role and limits of software, training, writing rules, and the
  controlled dictionary.

## Scope

### In scope

- A normative customer technical-writing standard.
- A reviewed product terminology register.
- A manual language-review checklist.
- A deterministic prose-check command.
- Markdown and CLI-manifest extraction.
- Plain-text fixture support for observable CLI output.
- A temporary, ratcheting baseline for existing violations.
- Local Make targets and required CI integration.
- Unit, fixture, integration, and drift tests.

### Out of scope

- Rewriting the current customer corpus in this plan.
- Adding a network dependency to normal generation, tests, or CLI execution.
- Redistributing the ASD-STE100 controlled dictionary.
- Claiming that a checker certifies ASD-STE100 compliance.
- Introducing a new product service, runtime dependency, or `pkg/wire`
  construction path for a repository maintenance command.
- Auto-rewriting prose in the first implementation.

## Canonical ownership

| Concern | Canonical owner | Notes |
| --- | --- | --- |
| Normative writing policy | `docs/internal/standards/writing/customer-technical-writing-standard.md` | Added to `docs/internal/standards/STANDARDS.md`. |
| Product vocabulary | `docs/internal/standards/writing/customer-technical-terms.yaml` | Product-specific terms and approved alternatives only. |
| Automated rule implementation | `cmd/prosecheck/` | Repository maintenance command with pure, directly tested analysis. |
| Existing-debt baseline | `docs/internal/baselines/customer-prose.json` | Temporary exact-finding baseline; deletion-only after acceptance. |
| CLI help source | `contracts/cli/commands.json` | Generated family projections remain derived. |
| Packaged documentation source | `docs/reference/*.md` | Embedded by the existing docs path. |
| CI entry points | `Makefile` and existing CI workflows | No second hidden enforcement path. |

## Normative writing profile

The standard must define at least these content classes.

### Labels

Labels include headings, table headers, command titles, and field names. They
must be concise and use approved terminology. They are not required to form a
complete sentence.

### Procedural prose

Procedural prose tells the customer to do a task. It must:

- use no more than 20 words in a sentence;
- contain one instruction per sentence unless actions must occur at the same
  time;
- use an imperative verb for the instruction;
- put a necessary condition before the instruction;
- identify the expected observable result; and
- use an ordered or vertical list when sequence matters.

### Descriptive prose

Descriptive prose explains a concept, state, or result. It must:

- use no more than 25 words in a sentence;
- introduce information gradually;
- keep one topic in each paragraph;
- use no more than six sentences in a paragraph;
- prefer active voice; and
- use stable key words and phrases to connect related information.

### Machine and technical text

The standard must explicitly distinguish natural-language prose from:

- code fences and inline code;
- command invocations and shell operators;
- JSON and YAML keys or values that are contract examples;
- API methods and routes;
- file paths, URLs, package paths, and identifiers;
- schema, event, status, and error-code literals;
- model and provider names;
- quoted external output; and
- generated artifacts.

These elements are protected during analysis. Natural-language comments inside
examples remain reviewable when the extractor can identify them safely.

## Product terminology register

Each term record should include:

- canonical spelling and capitalization;
- public definition;
- category such as product term, technical noun, technical verb, acronym,
  command, or protected literal;
- approved plural or verb form where applicable;
- discouraged or prohibited alternatives;
- customer-facing usage example;
- surfaces where the term is valid; and
- owning architecture or contract reference.

The initial register must cover at least:

- Factory;
- Factory Session;
- Current Factory;
- Work;
- Work Request;
- Worker;
- Worker Session;
- Workstation;
- Provider;
- Provider Session;
- Model;
- Recording;
- Factory Event; and
- customer-visible relationship and lifecycle names.

It must also identify internal terms such as `CPN`, place, transition, marking,
and token. Public prose may use these terms only in an explicitly internal or
implementation-focused context.

## Automated rule boundary

### Blocking rules

The first release must enforce deterministic rules for:

- procedural sentence length;
- descriptive sentence length;
- descriptive paragraph length;
- prohibited semicolons in natural-language prose;
- contractions;
- prohibited public terminology and approved replacements;
- canonical product capitalization;
- malformed or unknown suppression directives;
- stale baseline findings;
- new findings not present in the baseline; and
- parse failures that would cause prose to be silently skipped.

### Advisory rules

Rules with acceptable false positives may report warnings but must not alone
claim conformance:

- probable passive voice;
- multiple probable imperative actions in one sentence;
- repeated vague pronouns;
- nominalizations;
- unregistered candidate technical terms; and
- unusually dense headings, lists, or table cells.

### Human-review rules

A language and subject-matter reviewer must confirm:

- vocabulary is approved or valid as a technical noun or technical verb;
- each approved word uses the intended meaning and part of speech;
- each sentence communicates one subject or instruction;
- passive voice has a valid descriptive reason;
- paragraphs introduce one topic in logical order;
- simplification preserves technical and contract meaning;
- examples match the shipped CLI and public schemas; and
- warnings, destructive instructions, and failure recovery remain precise.

## Extraction and classification

The checker must parse structure instead of scanning raw text blindly.

### Markdown adapter

The adapter must emit source spans for headings, paragraphs, list items,
admonitions, table cells, and natural-language comments. It must preserve file
and line locations and exclude protected technical text.

Classification must be deterministic. The implementation must use explicit
document/block metadata or structural rules defined by the standard. It must
not make a blocking procedural-versus-descriptive decision from an opaque
language-model or probabilistic heuristic.

### CLI-manifest adapter

The adapter must inspect authored fields in `contracts/cli/commands.json`,
including:

- command titles;
- command descriptions;
- visible flag usage;
- authored example comments; and
- natural-language deprecation or replacement guidance.

Commands and literal example lines are protected. The adapter must report the
command ID and JSON source location with each finding.

### Observable-output fixture adapter

The adapter may inspect selected human-readable golden or baseline fixtures
when those fixtures are the canonical evidence for runtime CLI output. It must
not treat machine-readable JSON or NDJSON output as prose.

Direct runtime messages that are not in an extractable canonical source remain
subject to manual review and focused behavior tests until a later plan approves
a safe centralization strategy.

## Finding contract

Each finding must contain:

- stable rule ID;
- severity;
- source path;
- start line and, when known, column;
- content class;
- bounded offending excerpt;
- remediation guidance; and
- stable normalized fingerprint for baseline comparison.

Default terminal output must be concise and deterministic. A machine-readable
JSON mode may be added for CI, but it must not be required for an author to
understand a failure.

## Baseline policy

The initial audit will create exact fingerprints for accepted pre-existing
findings. The baseline must:

- contain no wildcard, file-wide, or directory-wide suppressions;
- fail when an entry is stale;
- fail when a changed excerpt no longer matches its fingerprint;
- fail on every new finding;
- permit only deletion during ordinary migration;
- require a reason, owner, and expiry for any exceptional addition; and
- reach zero for the accepted public scope before program completion.

Moving a violation or replacing it with another violation is not baseline
reduction.

## Suppression policy

Suppressions are for valid technical exceptions, not readability debt. A
suppression must name one rule, one bounded source span, and a reason. Unknown,
unbounded, nested, or stale suppressions fail the check. Generated files must
be excluded through source classification, not repeated inline suppressions.

## Work stories

### Story 1: Authors can apply one normative writing contract

#### Observable behavior

An author can determine how to write a command description, procedure,
explanation, product term, code example, and exception without relying on chat
history or reviewer preference.

#### Acceptance criteria

- The standard is indexed from `docs/internal/standards/STANDARDS.md`.
- It defines labels, procedures, descriptions, machine text, technical terms,
  automated rules, human-review rules, and suppression policy.
- It states the approved conformance claim and the conditions required before
  a stronger ASD-STE100 claim.
- It includes repository-specific good and bad examples.
- The terminology register validates as structured data and covers the initial
  public nouns.
- Architecture/data-model owners review the terminology definitions.

### Story 2: Maintainers receive exact findings for Markdown prose

#### Observable behavior

Running the prose check on a Markdown file returns deterministic file-and-line
findings while leaving code, commands, identifiers, and schemas untouched.

#### Acceptance criteria

- Valid Markdown prose produces no findings.
- Each supported violation produces the expected stable rule ID and source
  span.
- Fenced code, inline code, URLs, paths, JSON/YAML examples, and protected terms
  have direct false-positive fixtures.
- Mixed procedural and descriptive content is classified through documented,
  deterministic metadata or structure.
- Parse failures are blocking and identify the file.
- Unit and fixture tests cover Windows and Unix line endings.

### Story 3: Maintainers receive exact findings for CLI help

#### Observable behavior

Running the prose check on the authored CLI manifest identifies unclear command
and flag help by stable command or input ID without inspecting generated Go.

#### Acceptance criteria

- All visible authored command titles, descriptions, flag usage, and example
  comments are inspected.
- Literal commands, placeholders, flags, URLs, routes, error codes, and default
  values remain protected.
- Findings include the stable command/input ID and canonical JSON location.
- Hidden compatibility records follow an explicitly documented scope rule.
- Generated CLI files are not authoring inputs.
- CLI-manifest fixture tests cover valid, invalid, and excluded values.

### Story 4: Existing debt cannot grow

#### Observable behavior

A contributor can change compliant prose normally, but CI rejects every new or
moved violation and every stale baseline entry.

#### Acceptance criteria

- The initial full-scope report is reproducible.
- The baseline records exact normalized findings rather than aggregate counts.
- A new violation, changed violating excerpt, moved violation, stale entry, and
  expired exception each have a failing test.
- Deleting or correcting an existing violation requires deleting its stale
  baseline entry.
- The baseline report shows remaining findings by surface and rule.

### Story 5: Required checks run through standard repository entry points

#### Observable behavior

Authors and CI use documented Make targets to check changed prose and the full
accepted scope.

#### Acceptance criteria

- Focused and full-scope Make targets are documented.
- The appropriate required lint or verification lane invokes the blocking
  check.
- `docs-reference-check` and CLI-manifest workflows remain authoritative for
  their existing structural and generation responsibilities.
- The prose checker does not access the network.
- Check output is deterministic across repeated runs.
- The command and package comply with backend complexity and test standards.

## Verification

- Unit tests for tokenization, sentence/paragraph counting, terminology,
  classification, fingerprints, and suppressions.
- Table-driven fixtures for Markdown and CLI-manifest extraction.
- Integration tests for baseline comparison and repository configuration.
- `go test ./cmd/prosecheck/...` or the final focused package equivalent.
- `make docs-reference-smoke` after standard examples or packaged fixtures are
  introduced.
- `make cli-manifest-check` and `make cli-contract-smoke` for manifest adapter
  fixtures that use production contracts.
- `make verify-fast` and `make lint` before review.

## Project-level acceptance criteria

- The normative writing standard and terminology register are merged and
  discoverable from the standards index.
- Deterministic rules are enforced against Markdown and the authored CLI
  manifest with direct false-positive coverage.
- Human-only rules are not presented as mechanically certified.
- Existing findings are captured by exact temporary baseline entries, and no
  new violation can enter required CI.
- The checker has no runtime product dependency, network dependency, hidden
  global state, or generated authoring source.
- Required focused tests, `verify-fast`, and `lint` are terminal and passing.
- Delivery continues through blocking feedback, conflict resolution, terminal
  green CI, and actual PR merge.

## Work-story task packets

Each packet below is an independently reviewable task. An implementation may
split a packet further but must not combine it with unrelated cleanup.

### Task packet: normative customer-writing standard

# problem statement

Authors and reviewers have no normative rule set for customer-facing CLI and
documentation prose.

## customer ask

Define clear writing and product terminology rules aligned with ASD-STE100 so
customers can understand commands and technical documentation quickly.

## solution

Add an indexed customer technical-writing standard, structured product-term
register, and manual review checklist with explicit scope and conformance
language.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\customer-prose-standard-and-enforcement.md`

# changes

## package changes

- Add `docs/internal/standards/writing/customer-technical-writing-standard.md`.
- Add `docs/internal/standards/writing/customer-technical-terms.yaml`.
- Update `docs/internal/standards/STANDARDS.md`.

## contracts

- Define content classes, deterministic rules, human-review rules, protected
  technical text, terminology records, and suppression policy.

## services

- None.

## API changes

- None.

## tests

- Validate the structured terminology register and all referenced standard
  examples.

### Task packet: Markdown prose analysis

# problem statement

The current Markdown linter cannot detect customer-writing violations or
distinguish prose from protected technical text.

## customer ask

Give authors exact, trustworthy findings for customer-facing Markdown before
review.

## solution

Implement a deterministic Markdown prose analyzer with source spans, content
classification, protected-text handling, and focused rule tests.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\customer-prose-standard-and-enforcement.md`

# changes

## package changes

- Add the focused `cmd/prosecheck` maintenance command and pure analyzer files.
- Load the normative repository configuration and terminology register.

## contracts

- Define the finding, severity, content-class, source-span, and rule-ID values.

## services

- None. This is deterministic repository tooling, not a product service.

## API changes

- None.

## tests

- Add unit and Markdown fixture tests for every blocking rule and protected-text
  class.

### Task packet: CLI-manifest prose analysis

# problem statement

The canonical CLI manifest can preserve unclear command and flag help because
its current checks validate contract shape rather than writing quality.

## customer ask

Report customer-writing violations from the authored CLI source without
checking or editing generated projections.

## solution

Add a CLI-manifest adapter that extracts authored natural language, preserves
stable command/input identity, and excludes literal command syntax.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\customer-prose-standard-and-enforcement.md`

# changes

## package changes

- Extend `cmd/prosecheck` with canonical CLI-manifest extraction.
- Reuse existing CLI contract loading where it does not introduce runtime
  dependencies or duplicate validation policy.

## contracts

- Map findings to stable command, flag, argument, and documentation IDs.

## services

- None.

## API changes

- None.

## tests

- Add valid, invalid, protected-literal, hidden-record, and deterministic-order
  manifest fixtures.

### Task packet: prose baseline and CI gate

# problem statement

Blocking the complete existing corpus immediately would prevent ordinary work,
while a non-blocking report would allow new violations to accumulate.

## customer ask

Stop new customer-prose regressions immediately and remove existing debt in
measurable increments.

## solution

Add an exact-finding baseline, deletion-only ratchet, Make targets, and required
CI integration.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\customer-prose-standard-and-enforcement.md`

# changes

## package changes

- Add `docs/internal/baselines/customer-prose.json`.
- Add focused and full-scope Make targets.
- Integrate the blocking target into the appropriate existing verification
  workflow.

## contracts

- Define exact baseline fingerprint, reason, owner, expiry, and stale-entry
  behavior.

## services

- None.

## API changes

- None.

## tests

- Prove new, moved, changed, stale, wildcard, and expired findings fail while a
  corrected finding requires baseline deletion.
