# Public Documentation Prose Migration Plan

## Status

Proposed. Begins after the writing standard, prose gate, and highest-priority
CLI journeys are merged.

## Problem statement

Customer-facing documentation outside the packaged CLI reference uses mixed
structures, terminology, and levels of detail. The repository does not yet
classify which documents are public customer guidance, which source owns each
concept, or which writing-quality gate applies to each public surface.

## Customer ask

Apply the same clear technical-writing standard to the broader customer
documentation without rewriting maintainer-only material or creating duplicate
sources of truth.

## Intended outcome

Every public document has a named audience, task, canonical concept owner, and
verification path. Customers encounter the same terminology and task sequence
whether they begin in the root README, a public guide, an example, a package
README, or `you docs`.

## Parent plan

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\customer-prose-clarity-program.md`

## Dependencies

- `customer-prose-standard-and-enforcement.md` is merged and blocking changed
  prose.
- Root orientation and Work decomposition stories from
  `cli-and-reference-prose-migration.md` are merged.
- Canonical public terminology is approved.

## Scope

### Candidate public surfaces

- Root `README.md` and public routing material.
- `docs/README.md`.
- Customer guides and concept pages under `docs/` that are not packaged
  `you docs` topics.
- Customer-facing README files under `examples/`.
- Publishable package README files under `packages/`.
- Factory-local customer examples and overview documentation that ship as
  examples or packaged content.

### Explicitly excluded by default

- `docs/internal/**` standards, baselines, development plans, process notes,
  and maintainer inventories.
- Source comments and test failure strings.
- Generated documentation and generated contract artifacts.
- Third-party or quoted text.
- Historical design records not linked from public customer routes.

An excluded document can enter scope only through a reviewed classification
change. A file path alone must not determine whether a document is public.

## Public-document classification

The initial inventory must classify each candidate document as one of:

| Class | Meaning | Required action |
| --- | --- | --- |
| Public router | Directs customers to canonical task or reference owners. | Keep concise; remove duplicated contracts. |
| Public task guide | Teaches one customer task from prerequisite through verification. | Migrate to the task-guide pattern. |
| Public concept reference | Defines one customer concept or contract. | Migrate to the descriptive/reference pattern. |
| Public example guide | Explains a runnable example and its expected result. | Validate the example and migrate its instructions. |
| Publishable package guide | Documents an installed package surface. | Align terminology and validate package commands. |
| Maintainer-only | Supports repository development rather than product use. | Exclude from the public prose gate. |
| Generated | Derived from a canonical source. | Exclude from authoring; verify drift only. |
| Stale or duplicate candidate | Has no unique public responsibility. | Route, archive, or delete only through a separate reviewed documentation change. |

The classification must record audience, owned task or concept, canonical
source, public entry points, and responsible package or documentation owner.
It must not become a test that freezes every file in the repository.

## Canonical-source rules

- `docs/reference/` remains the canonical source for packaged `you docs`
  topics.
- OpenAPI fragments remain canonical for public HTTP schemas.
- `contracts/cli/commands.json` remains canonical for static CLI command
  metadata.
- `docs/architecture/data-model.md` remains canonical for public resource
  vocabulary unless a later architecture decision replaces it.
- Root README material routes to detailed owners and does not duplicate long
  task guides.
- Example guides may explain their specific Factory names and files but must
  route general product contracts to their canonical owner.
- Package README files own package installation and consumption behavior, not
  the complete product model.

## Prioritization model

Score candidate migration work by:

1. Customer exposure: linked from root, release, package, or CLI entry points.
2. Task frequency: used during setup, Work submission, verification, or common
   operation.
3. Error cost: unclear guidance can cause duplicate Work, wrong targets,
   destructive action, insecure configuration, or failed deployment.
4. Terminology drift: conflicts with the public data model or packaged help.
5. Content duplication: repeats a contract maintained elsewhere.
6. Change activity: likely to receive new customer-facing edits.

High exposure and high error cost move first. Word count alone does not set
priority.

## Migration unit

One implementation story covers:

- one public router;
- one complete task guide;
- one concept-reference section;
- one runnable example and its README; or
- no more than approximately 2,000 prose words from a larger document.

A story can update a small set of inbound links when that is necessary to make
the migrated document the clear canonical route. It must not absorb unrelated
content cleanup.

## Work stories

### Story 1: Maintainers can identify the public customer corpus

#### Observable behavior

An author can determine whether a document is subject to the customer-writing
standard and which source owns its task or concept.

#### Acceptance criteria

- Every candidate entry point has one reviewed classification.
- The inventory records audience, task/concept, owner, canonical source, and
  public inbound routes.
- Generated and internal files are explicitly excluded without broad path-only
  assumptions.
- The inventory identifies duplicate candidates but does not delete or move
  them.
- The prose checker can consume the accepted public scope deterministically.
- Adding an unclassified document to a known public entry point fails or emits
  a blocking classification finding.

### Story 2: Customers receive a concise repository entry point

#### Observable behavior

A customer can use the root README to identify the product, install or build
the CLI, run the shortest start path, and open the correct detailed guide.

#### Acceptance criteria

- The root README states the customer problem and product outcome before
  architecture detail.
- Setup and first-run procedures use tested commands and expected results.
- The README routes decomposition, submission, operation, authoring, and
  integration detail to canonical owners.
- Long duplicated contracts are removed or replaced by concise routing.
- `make readme-check` and all linked-example smoke checks pass.
- The migrated README has zero prose findings and recorded language and
  subject-matter review.

### Story 3: Customers can run each promoted example

#### Observable behavior

A customer can select a promoted example, understand its purpose, run its
documented commands, and verify the expected result.

#### Acceptance criteria for each example

- The README defines the example-specific goal and prerequisites.
- Product-wide contracts route to packaged or public canonical references.
- Commands use paths valid from the stated working directory.
- Input files validate against current schemas.
- A smoke test proves the shortest example path when external-provider behavior
  can be replaced through approved edges.
- Provider-dependent or platform-dependent steps state the limitation and safe
  alternative.
- The example guide has zero prose findings.

### Story 4: Package consumers receive focused installation and consumption guidance

#### Observable behavior

A package consumer can install or resolve the package, use its supported public
surface, and identify unsupported internal paths.

#### Acceptance criteria for each package

- The README states the package audience and supported exports.
- Installation and consumption examples match the packaged artifact.
- Generated paths are described as generated and not as authoring sources.
- Unsupported internal paths and version assumptions remain explicit.
- Package verification or tarball-consumer tests validate shown commands and
  exports.
- The package guide uses the common product terminology and has zero prose
  findings.

### Story 5: Public concept and architecture pages use one customer model

#### Observable behavior

A customer reading public concept material encounters the same Factory, Factory
Session, and Work model used by the CLI and API.

#### Acceptance criteria

- Each migrated page identifies whether it is customer-facing or maintainer
  architecture guidance.
- Customer-facing sections lead with public resources and observable behavior.
- Internal Petri-net terms appear only in explicitly internal explanations.
- Duplicate definitions route to `data-model.md` or another approved canonical
  owner.
- API and CLI examples remain contract-accurate.
- The migrated scope has zero prose findings and recorded reviews.

### Story 6: The public documentation baseline reaches zero

#### Observable behavior

Required CI rejects every deterministic customer-prose violation in the full
classified public corpus without consulting a temporary debt baseline.

#### Acceptance criteria

- Every classified public document has been migrated, reclassified with
  evidence, or removed through a separate approved change.
- The public-document baseline contains no findings.
- Full-scope prose checking is blocking in required CI.
- Stale classifications, suppressions, and inbound public links fail focused
  checks.
- The post-migration customer task study meets the program targets.

## Verification

- Prose check for the classified public scope.
- `make readme-check` for root README changes.
- `make docs-reference-smoke` when packaged routes or topics change.
- Schema/contract validation for example inputs.
- Focused package verification for publishable package guides.
- Focused `root.BuildProcess` functional scenarios for promoted CLI examples.
- Link and source-owner checks without adding broad runtime filesystem scans.
- `make verify-fast`, `make lint`, and applicable PR verification.

## Project-level acceptance criteria

- The public documentation corpus is explicitly classified by audience,
  responsibility, and canonical owner.
- Root and public router documents remain concise and do not duplicate long
  canonical guides.
- Promoted example commands and inputs are validated through the appropriate
  public contract or smoke surface.
- Public concepts use the same customer vocabulary as CLI help, packaged docs,
  OpenAPI, and `data-model.md`.
- Generated and maintainer-only documents are not treated as public authoring
  sources.
- The classified public corpus has zero temporary baseline findings.
- Required checks and the post-change comprehension study pass.
- Required CI is terminal and passing, blocking feedback and conflicts are
  resolved, and every implementation PR is merged.

## Work-story task packets

### Task packet: classify the public documentation corpus

# problem statement

The repository cannot enforce customer-writing rules broadly because it has no
reviewed boundary between public, maintainer-only, generated, and duplicate
documentation.

## customer ask

Apply the writing standard to all customer documentation without rewriting
internal or generated material.

## solution

Create a reviewed public-document classification that records audience, task or
concept, canonical owner, entry points, and enforcement scope.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\public-documentation-prose-migration.md`

# changes

## package changes

- Add the bounded public-document classification under
  `docs/internal/standards/writing/` or the approved standards-owned location.
- Teach the prose checker to consume the accepted scope.

## contracts

- Define classification values, required ownership fields, and stale-entry
  behavior.

## services

- None.

## API changes

- None.

## tests

- Add classification validation, public-entry-point coverage, generated-file
  exclusion, and stale-entry fixtures.

### Task packet: root public routing

# problem statement

The repository entry point can force customers through duplicated detail before
they reach the first supported CLI task.

## customer ask

Understand the product, complete the shortest setup/start path, and find the
canonical guide for the next task.

## solution

Rewrite the root README as a concise public router with verified installation,
first-run, result, and next-step guidance.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\public-documentation-prose-migration.md`

# changes

## package changes

- Update the root `README.md` and only the inbound links necessary to preserve
  canonical routing.

## contracts

- Preserve shipped commands, public names, install paths, and linked canonical
  owners.

## services

- None.

## API changes

- None.

## tests

- Run `make readme-check`, prose checks, link checks, and the shortest supported
  first-run smoke path.

### Task packet: one public guide or example migration

# problem statement

One classified public guide or example does not provide a clear task,
canonical route, runnable procedure, and verification result.

## customer ask

Complete the guide's owned customer task without relying on duplicated or
internal-only documentation.

## solution

Migrate one guide, example, package README, or bounded large-document section
to the customer-writing standard and validate its public commands and inputs.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\public-documentation-prose-migration.md`

# changes

## package changes

- Update one classified public surface and the minimum canonical-routing links.
- Remove only the baseline findings corrected by the task.

## contracts

- Preserve literal CLI, API, schema, package, and Factory behavior.

## services

- None unless a separately approved behavior defect is discovered.

## API changes

- None.

## tests

- Run prose checks and the surface-owned README, package, schema, example, or
  functional verification.
