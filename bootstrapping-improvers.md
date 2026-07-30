# Packaged Factory Bootstrapping and Improvement Plan

## Problem

The first-party packaged Factories are shipped, but we do not yet have enough
live evidence that each one reliably achieves its intended customer outcome.
Unit and functional tests prove contracts and deterministic runtime behavior;
this program complements them with model-backed experiments against realistic
work.

The repository's Packaged Service Structure work under
`docs/internal/packaged-service-structure/` is a useful benchmark corpus because
its tasks have substantial architecture context, observable diffs, explicit
standards, and meaningful tests. It is not the required destination of every
experiment, and an experiment output does not need to be merged to be useful.
The corpus helps us judge whether a Factory understood and completed a difficult
request.

The program evaluates all shipped packaged Factories. Every Factory is tested
against work that matches its product goal rather than forcing every Factory to
perform repository refactoring.

## Objective

For every shipped packaged Factory:

1. State the customer outcome the Factory promises.
2. Run the Factory against at least one representative model-backed workload.
3. Preserve enough evidence to reconstruct what ran and what happened.
4. Judge the output against explicit hard gates and a common scoring rubric.
5. Classify failures before changing prompts, topology, runtime, models, or the
   workload.
6. Iterate one variable at a time until the Factory meets its declared goals or
   a product/runtime blocker is explicitly recorded.

The purpose is not to make every experimental patch mergeable. The purpose is
to establish credible evidence that every Factory does what customers are told
it does.

## Scope

The shipped inventory is:

- `@you/classify`
- `@you/deep-research`
- `@you/full-flow`
- `@you/fusion`
- `@you/goal`
- `@you/loop`
- `@you/plan-execute`
- `@you/plan-parallel`
- `@you/quorum`
- `@you/review`
- `@you/spawn`
- `@you/subagent`
- `@you/tournament`
- `@you/tts`

If the generated packaged catalog changes, update this inventory and the goal
coverage matrix before declaring the program complete.

## Experiment Principles

### Match the workload to the Factory

Delivery Factories should receive implementation work. Research and synthesis
Factories should receive questions with judgeable source evidence. Scheduling
and audio Factories should be judged on scheduling and audio behavior. A
Factory is not penalized for declining work outside its advertised purpose.

### Separate evaluation from promotion

Each trial runs in an isolated Git worktree at a pinned base commit. Generated
code, documents, recordings, and patches are experiment evidence first. Promote
an output to a production branch only after an independent review and the
repository's normal verification and delivery process.

### Pin inputs and preserve evidence

Every result must identify the repository base, Factory version, model roles,
prompt, exact command, recording, timestamps, initial working-tree state, final
diff or artifact, and test results. Results without this information are
diagnostic anecdotes, not completed experiments.

### Change one variable at a time

Do not change the Factory prompt, topology, runtime, provider/model assignment,
and workload in one comparison. Classify the failure, change the smallest
responsible variable, rerun the failed case, and then run a holdout case before
accepting the improvement.

### Keep evaluation workloads distinct from tuning workloads

It is acceptable to tune a Factory against a failed workload. At least one
holdout workload must remain unused during prompt or topology iteration so that
the final result does not merely prove overfitting.

## Factory Goal Coverage Matrix

Each row is a separate product goal. A Factory is `MEETS_EXPECTATIONS` only when
all of its goal rows have acceptable evidence.

| Factory | Product goals to prove | Representative workload | Minimum evidence |
| --- | --- | --- | --- |
| `@you/classify` | Classify complexity; route to the correct small, medium, or large lane; return that lane's useful result | Three requests selected in advance: one small, one medium, and one large repository task | Expected lane for all three; observed worker/model lane; useful terminal result; no cross-lane dispatch |
| `@you/deep-research` | Decompose a research question into bounded specialist investigations; synthesize evidence; distinguish facts, disagreement, and uncertainty | Investigate one unresolved service-boundary decision using repository architecture, plans, code, and tests | Multiple relevant specialist contributions within configured bounds; source-grounded synthesis; unsupported claims identified; one coherent final answer |
| `@you/full-flow` | Plan implementation waves; isolate tasks in worktrees; review and verify them; merge safely; replan until complete or bounded failure | A bounded multi-story Packaged Service Structure slice with meaningful cross-file verification | Unique worktrees/branches; task review and CI evidence; serialized safe merges; at least one completion re-evaluation; coherent final tree or honest bounded failure |
| `@you/fusion` | Produce a useful first draft; improve it with an independent second pass; return the refined result | Draft and refine a migration decision or implementation plan against current repository evidence | Both stages observed; final output materially improves correctness, completeness, or clarity; unsupported first-pass claims are repaired rather than repeated |
| `@you/goal` | Continue working across bounded passes; preserve progress; stop only when the goal is complete; fail safely at the visit bound | A bounded cleanup with at least two independently verifiable completion conditions | Progress across passes; completion token only after conditions pass; no further executor dispatch after completion; honest failure if the bound is reached |
| `@you/loop` | Parse the requested interval; trigger one execution per scheduled occurrence; preserve the request; enforce consecutive-failure policy | A safe read-only repository audit run at a short supported interval in a disposable session | Optional start trigger behaves as configured; at least two scheduled executions observed; intervals are within documented tolerance; cancellation stops future work; failure bound is exercised separately |
| `@you/plan-execute` | Write consistent Markdown and JSON PRDs; hand them to a fresh executor; execute the complete plan in order | A bounded service-root cleanup with explicit acceptance criteria and tests | Both PRD files exist and agree; stories satisfy planning standards; executor reads the durable handoff; requested behavior and verification are completed or honestly reported |
| `@you/plan-parallel` | Generate a valid Work DAG; run independent Work concurrently; block dependent Work; merge all completed results | `test-improvement.md` or another task with at least two genuinely independent changes and one integration check | Valid batch and relationships; observable parallel readiness; dependency ordering; all-child fan-in; final result and repository verification agree |
| `@you/quorum` | Produce genuinely independent assessments in parallel; reconcile agreement and disagreement; return one evidence-based answer | Ask for an architecture recommendation with at least two defensible alternatives | Two non-duplicative branches; parallel dispatch; merger retains material disagreement and explains its decision; final answer is grounded in repository evidence |
| `@you/review` | Produce candidate work; review it independently; route rejection back for correction; stop on approval or bounded failure | A bounded code or plan change with a seeded, judgeable defect or missing failure case | Writer and reviewer are distinct; the seeded defect is detected; rejection causes a corrected pass; approval is evidence-based; visit bounds prevent endless review |
| `@you/spawn` | Generate exactly the requested number of independent tasks; execute them concurrently; merge all results | Divide a repository audit across a fixed number of service families | Generated and executed task count exactly matches `count`; tasks are meaningfully distinct; concurrency is observed; merger covers every child result without fabrication |
| `@you/subagent` | Run one bounded read-only subagent; prevent nested or mutating behavior; return its result | Inspect and summarize one service's contract surface without editing files | Exactly one child; no nested child execution; no working-tree mutation; useful result returned; policy violations fail closed |
| `@you/tournament` | Create the bounded candidate field; run 1v1 matches; use judges to advance winners; return the champion | Generate competing migration strategies for a clearly stated architecture problem | Candidate and match counts agree with configured rounds; every advancement has judge evidence; no eliminated candidate reappears; champion result is returned with decision rationale |
| `@you/tts` | Resolve the packaged local speech model; convert input text to audio; return usable audio content | A fixed short release-summary fixture containing punctuation and numbers | Non-empty audio result with the expected content type; deterministic invocation metadata; intelligibility checked; missing model/assets produce an actionable failure |

## Workload Corpus

Maintain three workload classes:

1. **Contract canaries** are small, controlled requests that prove invocation,
   routing, cardinality, bounds, failure behavior, and return selection.
2. **Repository benchmarks** use current architecture, test, documentation, or
   refactoring work whose output can be judged against the repository. The
   Packaged Service Structure plans are preferred inputs here, but the current
   source and standards remain authoritative.
3. **Holdouts** are representative workloads not used while tuning the Factory.

Do not use actively moving Providers/Workers restructuring as the first
benchmark unless its base commit and known in-flight state are frozen. Prefer a
small, stable service-root task for early delivery canaries and larger
cross-service slices only after the Factory passes its basic contract canary.

## Recommended Campaign Sequence

Run lower-risk observation and synthesis Factories first so the experiment
harness, recordings, scoring, and evidence storage are proven before allowing
multi-agent implementation and merge behavior.

1. **Harness canaries:** `subagent`, `classify`, and `fusion`.
2. **Parallel knowledge work:** `quorum`, `deep-research`, `spawn`, and
   `tournament`.
3. **Specialized runtime behavior:** `tts` and `loop`.
4. **Bounded delivery:** `plan-execute`, `plan-parallel`, and `goal`.
5. **Correction and end-to-end delivery:** `review` and `full-flow`.

Suggested initial IDs are `BOOT-<SLUG>-001` for the contract canary and
`BOOT-<SLUG>-002` for the representative workload. Use `R01`, `R02`, and so on
for repeats of the same frozen case. This keeps Factory coverage readable
without encoding model or commit choices into the identifier.

## Standard Trial Protocol

### 1. Preflight

- Record `git rev-parse HEAD` for the repository and Factory source.
- Confirm the Factory appears in the generated packaged manifest.
- Run `you run --named <factory> --help` and save the effective signature.
- Confirm each configured provider and model is available.
- Record known baseline failures before the trial.
- Create a uniquely named trial branch and isolated worktree from the pinned
  base commit.
- Ensure the trial worktree is clean before invocation.

### 2. Invoke

- Use `you run --named`, the Factory's published parameter names, and an
  explicit `--record` path.
- Supply the complete workload through `--to` unless the goal specifically
  tests positional input or stdin.
- Do not intervene in the worktree while the Factory is running.
- Preserve stdout, stderr, exit code, response, recording, and elapsed time.

### 3. Observe

Record the behavior relevant to the Factory, including:

- child Work or agent count
- dispatch order and overlap
- dependency and parent/child relationships
- retries, rejections, and loop visits
- provider/model selection by role
- terminal state and primary result
- files, commits, branches, worktrees, and artifacts created
- verification commands and their actual outcomes

### 4. Judge

- Apply all hard gates.
- Score the result using the common rubric.
- Check Factory-specific evidence from the goal coverage matrix.
- Compare the final claims with the recording, diff, artifacts, and tests.
- Classify the result as `PASSED`, `FAILED`, or `INCONCLUSIVE`.

### 5. Iterate or promote

- Classify the failure before changing anything.
- Commit every Factory, runtime, schema, or prompt change separately with the
  experiment IDs that motivated it.
- Rerun the failed workload from the same pinned base where possible.
- Run a holdout after the tuned workload passes.
- Keep experiment-output commits on trial branches unless independently
  accepted for production.
- Production delivery still requires required CI, resolved blocking feedback
  and conflicts, and actual merge; an experiment pass is not a delivery pass.

## Hard Failure Gates

An experiment cannot pass if any applicable condition occurs:

- The invocation reaches a failed or stranded terminal state without the
  expected bounded-failure behavior.
- The Factory reports success while required verification fails.
- The final answer invents child results, test results, citations, files, or
  commits.
- A mutating Factory changes unrelated files or overwrites existing user work.
- Coverage or required behavior is removed merely to make checks pass.
- Generated public artifacts drift from their authored sources.
- Dependency-aware Work dispatches before its declared prerequisites.
- Cardinality differs from a user-requested exact count.
- A read-only Factory mutates the workspace or exceeds its child-agent policy.
- A loop continues after cancellation/completion or exceeds its documented
  bound.
- Worktrees or merges are left conflicted or unsafe while success is claimed.
- The result cannot be reconstructed because required experiment evidence is
  missing.

Factory-runtime bookkeeping is not an agent-authored workspace mutation. A
trial may create the documented `.you-agent-factory` session state and an
explicitly requested recording path. Record those paths as expected runtime
artifacts and exclude them when comparing the pre/post repository diff. The
gate still fails if a read-only worker changes tracked source, creates any
other unrequested workspace artifact, writes runtime state outside its
documented root, or overwrites pre-existing user state.

## Common Scoring Rubric

Score only after applicable hard gates pass.

| Category | Points | Question |
| --- | ---: | --- |
| Intended outcome | 0-5 | Did the Factory achieve the customer outcome it promises? |
| Factory-specific behavior | 0-4 | Did it demonstrate the orchestration shape, policy, and bounds unique to this Factory? |
| Correctness and evidence | 0-4 | Are claims supported by source, recordings, artifacts, tests, or observable output? |
| Safety and scope | 0-3 | Did it preserve unrelated work and stay inside its authority? |
| Final result quality | 0-2 | Is the returned result complete, coherent, and honest about limitations? |
| Efficiency | 0-2 | Was time, model usage, concurrency, and repetition proportionate to the outcome? |

Interpretation:

- `16-20`: meets expectations
- `13-15`: useful but needs iteration
- `0-12`: does not yet meet expectations

Scores support judgment; they do not override a hard failure or missing
Factory-specific goal.

## Reliability Evidence

For each Factory:

- Run at least one live contract canary.
- Run at least one representative live workload.
- Obtain a second successful observation through either an independent repeat
  or existing deterministic functional coverage of the same critical behavior.
- After prompt or topology tuning, rerun the failed workload and one holdout.

Use three live repeats when model variance is itself the failure mode. Do not
require three expensive repository implementations when a deterministic runtime
test plus two judgeable live outcomes provide the necessary confidence.

## Stop Criterion

The bootstrap program is complete when all of the following are true:

1. Every Factory in the shipped inventory is present in the goal coverage
   ledger.
2. Every product-goal row has representative evidence and is marked
   `MEETS_EXPECTATIONS`.
3. Each Factory has passed its applicable hard gates, Factory-specific minimum
   evidence, and reliability requirement.
4. No severity-one correctness, safety, permission, orchestration, or false
   success defect remains open for any packaged Factory.
5. Known lower-severity limitations are documented with an owner and a concrete
   follow-up rather than hidden by the aggregate score.
6. The generated packaged catalog is current and the required repository tests
   for accepted Factory/runtime changes pass.

This is intentionally an outcome-based stop criterion. It does not require
every experimental repository patch to merge, every model run to be perfect,
or every Factory to operate on Packaged Service Structure work. It requires us
to have credible evidence that every advertised Factory goal is achieved to the
quality level we expect.

## Goal Coverage Ledger

Update this table after each accepted experiment. One Factory may require
multiple experiment IDs to cover all goals.

| Factory | Canary experiment | Representative experiment | Holdout/repeat evidence | Goal status | Open limitations |
| --- | --- | --- | --- | --- | --- |
| `@you/classify` | `BOOT-CLASSIFY-001-R01` | `BOOT-CLASSIFY-001-R02` | deterministic three-lane and invalid-label coverage | MEETS_EXPECTATIONS | Live small and medium requests selected the appropriate lanes and returned source-grounded results. |
| `@you/deep-research` | `BOOT-DEEP-RESEARCH-002-R02` | `BOOT-DEEP-RESEARCH-002-R01` | bounded delegation and synthesis coverage; bounded retry regression | MEETS_EXPECTATIONS | The holdout completed technical research, recovered a rejected trade-off specialist through one bounded retry, then produced a source-grounded lead synthesis with explicit specialist statuses. |
| `@you/full-flow` | TBD | TBD | deterministic full-flow invocation tests | NEEDS_ITERATION | Caller-selected bounds are not independently enforced below fixed topology ceilings. |
| `@you/fusion` | `BOOT-FUSION-001-R01` | `BOOT-FUSION-001-R03` | deterministic draft-then-refine coverage | MEETS_EXPECTATIONS | Both stages produced a useful migration decision. One provider flake and one disputed peripheral defect label remain documented. |
| `@you/goal` | TBD | TBD | TBD | UNVALIDATED | |
| `@you/loop` | TBD | TBD | TBD | UNVALIDATED | |
| `@you/plan-execute` | TBD | TBD | TBD | UNVALIDATED | |
| `@you/plan-parallel` | `BOOT-PLAN-PARALLEL-001` (passed twice) | simplified-graph holdout required | deterministic DAG invocation coverage plus frozen repeat | NEEDS_ITERATION | The first holdout completed four parallel evidence tasks but redundantly generated a synthesis child that failed before the dedicated merger. Planner prompts now reserve terminal synthesis for the merger. |
| `@you/quorum` | `BOOT-QUORUM-002-R02` | `BOOT-QUORUM-002-R01` | deterministic parallel-branch, merge-gating, and insufficient-member failure coverage | MEETS_EXPECTATIONS | Both live cases ran independent branches then gated merge. One transient provider failure and one disputed peripheral defect label remain documented. |
| `@you/review` | `BOOT-REVIEW-001-R01` | `BOOT-REVIEW-001-R02` | rejection feedback and bounded correction-loop coverage | MEETS_EXPECTATIONS | Independent review correctly disproved the seeded defect with traced code and a passing focused test; live approval occurred on the first review. |
| `@you/spawn` | TBD | TBD | TBD | UNVALIDATED | |
| `@you/subagent` | `BOOT-SUBAGENT-003-R05` | `BOOT-SUBAGENT-003-R04` | deterministic invocation coverage | MEETS_EXPECTATIONS | Current published artifact passes repository-read and cross-file analysis cases. Named trials must report resolution because an older editable global install is intentionally not replaced. |
| `@you/tournament` | TBD | TBD | TBD | UNVALIDATED | |
| `@you/tts` | TBD | TBD | TBD | UNVALIDATED | |

Allowed goal statuses are `UNVALIDATED`, `RUNNING`, `NEEDS_ITERATION`,
`BLOCKED`, and `MEETS_EXPECTATIONS`.

## Recommended Model Profiles

Confirm exact provider and model identifiers locally before freezing an
experiment. Record the resolved values rather than relying on these display
names.

### Strong reasoning

- Provider `CODEX`, model `gpt-5.6-sol`
- Provider `cursor-acp`, model `cursor-grok-4.5-high`

Use for planning, architecture synthesis, merge decisions, judging, and large
implementation work.

### Small and medium work

- Provider `CODEX`, model `gpt-5.6-terra`
- Provider `cursor-acp`, model `composer-2.5`

Use for bounded execution, independent branches, classification lanes, and
lower-risk review passes.

Where a Factory has separate author and judge roles, prefer different providers
for the primary evaluation profile so that the judge does not merely reproduce
the author's assumptions. Keep a same-provider profile as a separate comparison
rather than changing providers mid-trial.

## Canonical Command Example

Use the published help as the final authority. For `@you/plan-parallel`, a
recorded mixed-provider trial is shaped as:

```powershell
you run --named @you/plan-parallel `
  --planner-provider CODEX `
  --planner-model gpt-5.6-sol `
  --executor-provider cursor-acp `
  --executor-model composer-2.5 `
  --merge-provider CODEX `
  --merge-model gpt-5.6-sol `
  --record .artifacts/bootstrap/BOOT-PLAN-PARALLEL-001/run-01.replay.json `
  --to "Read test-improvement.md, implement the justified improvements, preserve behavioral coverage, and verify the complete result."
```

Do not abbreviate recorded commands with placeholders. Use the exact command
that was executed. Declarative packaged agent workers default permission bypass;
record any invocation override or JavaScript policy difference when applicable.

## Experiment Record Template

Create one record per trial.

````markdown
## BOOT-<FACTORY>-<CASE>-R<REPEAT>

### Identity

- Status: PLANNED | RUNNING | PASSED | FAILED | INCONCLUSIVE
- Factory:
- Factory goal rows covered:
- Factory source commit:
- Generated Factory artifact SHA-256:
- Repository base commit:
- Trial branch:
- Worktree:
- Recording:
- Started: <ISO-8601 timestamp with UTC offset>
- Finished: <ISO-8601 timestamp with UTC offset>
- Initial working tree clean: yes | no, with exact pre-existing paths

### Hypothesis

Given <workload>, the Factory will demonstrate <specific advertised behavior>
and produce <observable outcome> without <relevant failure>.

### Workload

- Workload class: contract canary | repository benchmark | holdout
- Source request or plan:
- Why this workload matches the Factory:
- Intended observable outcome:
- Explicit non-goals:
- Expected files or artifacts:
- Areas that must not change:
- Required focused checks:
- Required broad checks:
- Known baseline failures:

### Models

| Role | Provider | Model | Reasoning/policy configuration |
| --- | --- | --- | --- |
| | | | |

### Exact command

```powershell
<complete command>
```

### Expected Factory behavior

- <Factory-specific dispatch, relationship, count, loop, review, or artifact expectation>

### Observed result

- Exit code and terminal status:
- Primary result:
- Child/dispatch count:
- Ordering, overlap, loops, retries, or matches:
- Duration:
- Files/artifacts/branches/worktrees created:
- Focused verification:
- Broad verification:
- Final diff or artifact reference:
- Trial commit, if any:
- Promoted PR, if any:

### Hard gates

| Gate | Pass/Fail/N/A | Evidence |
| --- | --- | --- |
| Terminal behavior is correct | | |
| Claims match observable evidence | | |
| Scope and permissions are respected | | |
| Factory-specific bounds and ordering hold | | |
| Required verification passes | | |

### Score

| Category | Score | Evidence |
| --- | ---: | --- |
| Intended outcome | /5 | |
| Factory-specific behavior | /4 | |
| Correctness and evidence | /4 | |
| Safety and scope | /3 | |
| Final result quality | /2 | |
| Efficiency | /2 | |
| Total | /20 | |

### Failure classification

- Factory prompt
- Factory topology
- Runtime behavior
- Provider/model behavior
- Workload ambiguity
- Repository baseline
- Environment or external dependency
- No failure

### Decision

- Goal status after this trial:
- Keep Factory unchanged | revise prompt | revise topology | revise runtime |
  revise model profile | revise workload | mark inconclusive
- Exact next change:
- Variables held fixed:
- Required rerun:
- Required holdout:
````

## Change and Commit Policy

- Commit every prompt, Factory definition, schema, generated catalog, runtime,
  and test change separately with the motivating experiment IDs.
- Regenerate packaged artifacts from authored sources; never edit generated
  Factory files directly.
- Record the before-and-after Factory commits on every rerun.
- Experimental implementation outputs may be committed on their disposable
  trial branch for reproducibility without being promoted.
- Never combine unrelated Factory improvements merely because one trial exposed
  them at the same time.
- Required CI, blocking review feedback, conflict resolution, and actual merge
  remain the delivery boundary for any change promoted to production.
