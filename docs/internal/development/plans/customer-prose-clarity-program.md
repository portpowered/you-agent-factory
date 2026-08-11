# Customer Prose Clarity Program

## Status

Proposed.

## Problem statement

Customers must read dense, inconsistent CLI help and documentation before they
can select a command, decompose a goal into Work, or verify the result. The
repository has canonical CLI and packaged-reference sources, but it has no
normative customer-writing standard and no quality gate that checks the clarity
of their prose.

## Customer ask

Make CLI prose and customer documentation clear, consistent, and suitable for
customers who must quickly:

- understand the purpose of the `you` CLI;
- choose the correct command for a task;
- decompose a goal into independent, ordered, or parent-child Work;
- submit and verify that Work; and
- move from concise command help to the correct detailed guide.

Use a repository writing standard aligned with ASD-STE100 Simplified Technical
English. Prevent the prose from returning to its current state after the first
rewrite.

## Intended outcome

The CLI and its documentation use one customer-facing vocabulary and one
progressive-disclosure pattern. A customer can start from `you`, select a task,
run a correct command, and verify its effect without first learning internal
Petri-net terminology or searching several overlapping guides.

Authors receive an actionable, deterministic conformance report before review.
Reviewers receive a separate checklist for rules that require linguistic and
technical judgment. Required CI blocks new violations and the migration removes
all temporary baseline entries from the accepted public scope.

## Planning basis

This program follows
`docs/internal/standards/code/planning-standards.md`:

- stories describe customer-visible comprehension and task-completion behavior;
- enabling stories are limited to the standard and enforcement needed to make
  that behavior durable;
- implementation work is divided by customer journey or one bounded document,
  not by a broad file-type rewrite;
- acceptance criteria identify direct CLI, documentation, contract, and
  comprehension evidence; and
- delivery continues through required CI, blocking review feedback, conflict
  resolution, and actual PR merge.

## Current-state findings

### Canonical sources

| Surface | Canonical source | Current scale | Existing evidence |
| --- | --- | ---: | --- |
| Static CLI command help | `contracts/cli/commands.json` | 54 commands, 301 flag records, 23 argument records, approximately 6,300 prose words | CLI manifest generation/check, CLI contract smoke, root help baselines |
| Packaged CLI reference | `docs/reference/*.md` | 29 Markdown files, approximately 65,000 words | Markdown structure check, embedding tests, CLI docs smoke |
| Handwritten or dynamic CLI help | `pkg/transports/cli/**` and service-owned CLI adapters | Bounded exceptions outside the command manifest | Focused command tests and observable-output tests |
| Human success, warning, and failure prose | CLI transport and service-owned CLI adapters | Distributed | Focused unit, contract, and functional tests |
| Broader public documentation | Root and public `docs/`, `examples/`, and package guides | Not yet classified | README and specialty smoke checks |

The scale figures are planning snapshots, not permanent inventory assertions.
The implementation must recalculate them when it establishes the baseline.

### Principal behavior gaps

1. Root help describes a `CPN-based workflow factory`, although public surfaces
   are required to hide the internal Petri-net implementation.
2. Long descriptions mix orientation, operation, architecture, and exception
   details before the customer receives a recommended first action.
3. Work-decomposition guidance exists across `agents`, `batch-inputs`,
   `relationships`, `work`, and command help, but the shortest route from a goal
   to a valid batch is not consistently visible.
4. Similar concepts use inconsistent capitalization, actor names, and verbs.
5. The current Markdown checker validates encoding, final newlines, and code
   fences but does not enforce customer-writing rules.
6. Existing golden and substring tests can preserve required facts while also
   preserving unclear sentence structures.

## Standard posture

The repository will initially claim that in-scope prose conforms to the You
customer technical-writing standard, which is aligned with ASD-STE100 Issue 9.
It must not make an unqualified ASD-STE100 certification claim unless the
organization separately establishes the necessary trained review, source
access, and legal approval.

The standard will use applicable ASD-STE100 principles, including:

- controlled and consistent terminology;
- short procedural and descriptive sentences;
- one instruction per procedural sentence;
- active voice unless a documented descriptive exception applies;
- gradual presentation of information;
- one topic per paragraph;
- bounded paragraph length; and
- vertical lists for complex information.

The repository will maintain only its product-specific technical terms and
approved alternatives. It will not copy or redistribute the ASD-STE100
controlled dictionary without explicit legal approval.

## Scope

### First release scope

- Visible root and subcommand help.
- Visible flag descriptions and authored help examples.
- Signature-aware Factory invocation help.
- Packaged `you docs` topics.
- Human CLI success, warning, and failure messages touched by a migration
  story.
- Root/customer documentation that directly routes customers into the CLI.

### Later public-documentation scope

- Public guides outside `docs/reference/`.
- Customer-facing example README files.
- Publishable package README files.
- Public architecture or concept pages that a customer must read to use the
  product.

### Out of scope

- Rewriting source comments, test failure strings, internal development plans,
  or maintainer-only architecture notes merely to increase an inventory count.
- Changing command names, flags, request schemas, error codes, JSON keys, or
  runtime behavior unless a separate behavior story approves that change.
- Replacing the public product vocabulary with ASD aerospace terminology.
- Treating a readability grade or an automated checker as proof of full
  controlled-language conformance.
- Copying the ASD-STE100 dictionary into source control.
- Combining unrelated documentation corrections, API changes, or runtime
  refactors with a prose-migration story.

## Workstreams and child plans

| Workstream | Plan | Primary outcome |
| --- | --- | --- |
| Normative rules and enforcement | `customer-prose-standard-and-enforcement.md` | Authors receive one standard, terminology register, checker, baseline, and CI gate. |
| CLI and packaged-reference migration | `cli-and-reference-prose-migration.md` | Customers can discover, decompose, submit, inspect, and operate through concise CLI and packaged help. |
| Broader public-documentation migration | `public-documentation-prose-migration.md` | Remaining customer documentation moves to the same standard without rewriting internal material. |

## Customer journeys

The program prioritizes these observable journeys:

1. **Discover:** Run `you` and identify the correct next command.
2. **Start:** Start the Current Factory or an explicitly selected Factory.
3. **Decompose:** Represent a goal as independent Work, dependency-ordered Work,
   or parent-child Work.
4. **Submit:** Choose unary or batch submission and use the correct target.
5. **Verify:** Confirm that submitted Work exists and inspect its state.
6. **Operate:** Keep a Factory Session alive, diagnose failure, and recover.
7. **Author:** Create and validate a Factory without relying on internal runtime
   vocabulary.
8. **Select execution:** Choose Workers, Workstations, Providers, Models, and
   related integrations.

## Program sequencing

### Stage 1: Establish the contract

Approve the customer-writing standard, product terminology register, public
scope, conformance claim, and manual review checklist. Capture pre-change
customer task measurements.

### Stage 2: Add enforceable feedback

Implement the prose checker, adapters, fixtures, baseline, and changed-prose
gate. The checker becomes blocking before broad rewriting begins. Existing
violations remain visible and cannot increase.

### Stage 3: Correct the highest-value journeys

Rewrite root orientation and the Work decomposition, submission, and
verification journeys. Remove internal implementation vocabulary. Validate the
commands and examples through the public CLI boundary.

### Stage 4: Migrate command families and packaged topics

Process one command family or one bounded topic slice at a time. Remove the
corresponding baseline findings after automated and manual review.

### Stage 5: Expand to broader public documentation

Classify the remaining customer corpus, route duplicate content to canonical
owners, and migrate it in task-oriented slices.

### Stage 6: Close the baseline and validate comprehension

Remove all temporary baseline entries in the accepted public scope, enable
full-corpus blocking, rerun the customer task study, and publish maintainer
guidance for future additions.

## Measurement plan

### Pre-change and post-change task study

Use the same script with at least eight representative readers who did not
author the affected prose. Give each participant these tasks without coaching:

1. Identify how to start the Current Factory.
2. Identify how to submit one Work item to a running Factory Session.
3. Create a three-item batch in which one item depends on another.
4. Represent one parent Work item with two child Work items.
5. Identify how to verify a submitted Work item.
6. Find the guide for Worker or Provider selection.

Record:

- whether the participant selected the correct first command;
- whether the submitted batch shape and relationships are valid;
- time to the first correct command;
- task completion without assistance; and
- each point at which terminology or routing caused hesitation.

Release targets:

- at least 80 percent of all tasks complete without assistance;
- at least 90 percent correct first-command selection;
- median time to the correct first command below 60 seconds; and
- no repeated critical ambiguity that causes an unsafe, destructive, or
  duplicate-submission path.

The study is direct evidence for comprehension. Sentence counts and readability
metrics are supporting evidence only.

### Repository quality measures

- New or changed in-scope prose has zero unbaselined blocking findings.
- Baseline finding count decreases in every migration PR.
- Every migrated surface has recorded human language-review evidence.
- Every runnable example has a smoke test or a documented reason it cannot run
  in CI.
- All canonical and generated CLI artifacts remain synchronized.

## Cross-cutting acceptance criteria

- A customer can move from root help to the correct detailed guide without
  encountering internal Petri-net vocabulary as the primary product model.
- The decomposition path gives correct independent, `DEPENDS_ON`, and
  `PARENT_CHILD` examples and identifies the verification command.
- The normative standard defines procedural and descriptive prose, technical
  terminology, exclusions, suppression policy, examples, and human-review
  responsibilities.
- Automated findings identify an exact source location, stable rule ID,
  offending text, and a remediation hint.
- Code fences, inline code, command invocations, API routes, identifiers, JSON
  and YAML keys, error codes, and proper product names are not corrupted by
  prose rewriting or false-positive auto-fixes.
- CLI human prose changes do not change JSON output, exit behavior, flags,
  request shapes, or compatibility aliases.
- Generated CLI artifacts are regenerated from canonical sources and are never
  hand-edited.
- `docs-reference-smoke`, CLI manifest checks, CLI contract smoke, focused
  behavior tests, `verify-fast`, and `lint` are terminal and passing where
  applicable.
- The accepted public scope has no temporary prose baseline when the program is
  declared complete.
- The post-change task study meets the release targets.
- Required CI is terminal and passing, all blocking review feedback is
  addressed, merge conflicts are resolved, and every delivery PR is actually
  merged.

## Risks and controls

| Risk | Control |
| --- | --- |
| Simplification changes technical meaning | Require a subject-matter reviewer and preserve contract-focused tests. |
| A checker is mistaken for full STE conformance | Separate deterministic checks from human language review and constrain the public claim. |
| The vocabulary rule rejects valid product terms | Maintain a reviewed technical-term register with categories and approved usage. |
| Code and contract tokens are rewritten | Parse Markdown and manifest structure; exclude code, identifiers, and machine contracts explicitly. |
| One large rewrite becomes unreviewable | Limit a task to one customer journey, one command family, or approximately 2,000 prose words. |
| A baseline becomes permanent debt | Require every migration PR to reduce it and make a zero baseline a program acceptance criterion. |
| Golden tests preserve unclear wording | Test required facts and customer outcomes in addition to selected stable output fixtures. |
| Documentation duplicates canonical contracts | Identify one concept owner and replace duplicate detail with task-oriented routing. |

## Delivery loop

Each child plan must deliver through focused implementation, generation where
required, automated and manual review, terminal green CI, resolution of all
blocking feedback and conflicts, and actual PR merge. Opening a PR, generating
artifacts, obtaining approval, or reaching green CI without merge is not
completion.
