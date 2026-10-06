# Functional CI latency audit

Measured October 5, 2026. Initial local source revision:
`73d83286167b941e3f2f668bcc7425926481fd99`.

## Current result

### Reuse the decoded public input across boundary checks

The generated-model boundary decoded the same input independently for retired
aliases, normalization, three unsupported-field checks, portable layout and
unknown-field diagnostics. Reuse the normalized JSON object for the read-only
checks before decoding the generated model. The standalone authored/layout
boundaries retain their own parsing, and strict/tolerant field handling and
generated-model validation remain. No global cache, new effect port or shared
mutable configuration is introduced.

Two complete customer-package executions per side in B/C/C/B order pass.
Initial baseline totals 72.895403 CPU-seconds / 37.996198394s elapsed;
candidate 68.465248 / 38.340250861: **6.1% less CPU / 0.9% more elapsed**.
A separate confirmation passes with baseline **61.163351 CPU-seconds /
35.029389710s elapsed** versus candidate **55.638802 / 32.720213763**,
saving **9.0% CPU / 6.6% elapsed**. Builds are excluded and the initial pair's
builtin lint overlap is retained as a measurement limitation; confirmation
has no concurrent local lint/build work. These are package comparisons, not
complete hosted checkpoint evidence.

All mapping test packages and Factory Definitions Wire pass, as do both
scoped lint suites and the catalog-generator command tests. The broader
artifact-generation inventory test fails the same existing @you/subagent
skipPermissions expectation on the restored baseline and candidate. Do not
claim that test green or weaken its assertion to justify this change.
Evidence: `factory-boundary-decode-paired/`,
`factory-boundary-decode-confirmation/` in `.artifacts/latency-audit/`.

The subsequent full lane on the rebased 45e5504c66 base passes at **133.50s
supervisor / 122.331s coverage / 423.68 CPU-seconds** (349.25 user, 74.43
system), 67 packages / 763 results, 761 pass/two skip. There is no flake ledger
and no retry reported in the supervisor log. It rebuilds **649 compiler
commands**, consuming 150.529880 CPU-seconds / 48.878179s active compile wall,
plus **19 links**, 23.291724 CPU-seconds / 22.162300s active link wall. Preserve
that rebuild cost; this full run is not a controlled speedup comparison.
It precedes the final missing-field guard, whose focused public-input tests
pass. Case-insensitive container checks retain the generated model's sorted
last-field selection, and absent containers do not treat an unrelated empty
object key as Workstations. All mapping/Wire and focused canonical publication
tests pass. Evidence: `factory-boundary-live-full/`.

### Hosted rebased canonical loading result, October 6

Head 93d1fd8767 passes hosted Functional Coverage in run 37484727726.
The complete job runs 15:08:53-15:14:42 UTC, **349s**; its coverage invocation
takes **259.580s**, with 67 packages / 763 results, 761 pass and two skip.
The concurrent supervisor takes 290.384s and quarantine verification 125.840s.
A prefix compiler-archive restore from d2b1f09cc0 still requires **361 compiler
commands / 16 links**. Retain this substantial rebuild work and distinguish
coverage invocation, concurrent supervisor and complete job timing. This hosted
sample does not isolate the loading cleanup's execution effect or prove the
two-minute checkpoint. Backend Lint later passes, and the complete workflow is
confirmed terminal with success after its Architecture Preview job finishes.
Evidence: `canonical-load-cleanup-hosted/` in `.artifacts/latency-audit/`.

A separate cached-decoder binding experiment routes canonical loading through
the already injected serialized decoder instead of its mapper. Both complete
customer-package executions per side pass, but baseline totals 63.174784
CPU-seconds / 35.086235051s elapsed versus candidate 73.983320 /
38.276307960: **17.1% more CPU / 9.1% more elapsed**. The private source is
restored; this additional binding does not ship. Evidence:
`canonical-decoder-binding-paired/` in `.artifacts/latency-audit/`.

### Remove discarded canonical loading serialization

Canonical loading used Expand to validate/map its public input, then cloned the
result through JSON, removed authored-only inline content, flattened the clone
to canonical JSON, discarded both clone and bytes, and returned the original
configuration. The loader subsequently performs its blocking-definition and
canonical-file checks. Remove the unused authored-output round trip; retain
Expand and all subsequent selected validation and loading capabilities.
Persistence still normalizes and serializes authored output when it writes it.

Two complete `product/customer_journeys` executions per side in B/C/C/B order
all pass. Baseline totals **70.199821 CPU-seconds / 37.198240684s elapsed**;
candidate **61.468967 / 34.483024415**, saving **12.4% CPU / 7.3% elapsed**.
Baseline individual wall is 18.512-18.686s; candidate 17.125-17.358s.
Builds are excluded. The smaller three-journey comparison also passes all six
repetitions per side, saving 5.3% CPU / 4.0% elapsed. These package measurements
are not complete hosted checkpoint evidence.

The discarded clone also converted repeated example arguments from []string to
[]interface{}, causing the output mapper to reject an otherwise valid public
input. Focused component tests cover repeated argument preservation, independent
loads, forward-compatible field diagnostics and invalid name/description/
arguments/trailing-input rejection. The complete Wire, compilation loader and
two public mapping test packages pass, as do both scoped lint suites.
Evidence: `canonical-load-cleanup-paired/` and
`canonical-load-cleanup-full-package/` in `.artifacts/latency-audit/`.

The pre-rebase complete local candidate finishes at **113.93s supervisor /
113.924s coverage / 343.11 CPU-seconds** (268.94 user, 74.17 system).
It performs 15 compiler commands (9.132855 CPU-seconds) and 20 links
(25.144966 CPU-seconds / 24.304098s active link wall). The unchanged retry
supervisor recovers one ACP child-visibility cancellation failure on the same
source; retain that failure and do not describe this run as retry-free. This
single warm measurement is compatibility and diagnostic evidence, not the
hosted two-minute checkpoint. Evidence: `canonical-load-cleanup-full/`.
The change is subsequently rebased without conflicts onto live main 45e5504c66
and published as 93d1fd8767; hosted run 37484727726 is pending verification.

### Cached serialization versus pre-rendered packaged layouts

The current implementation already ships generated native Factory configurations
and canonical-output bytes for the 20 published packaged Factories. Exact input
hashes and the serialization version govern hits; customer edits and unsupported
inputs take the ordinary conversion path. Returned configurations remain owned by
each operation. These caches avoid conversion work, but do not eliminate layout
preparation, validation, rendering or destination filesystem writes.

The fresh customer-package profile below supports investigating an immutable
pre-rendered file manifest next: installation consumes 74.01% of sampled CPU,
fresh creation 58.46%, and preparation 38.52%. These are overlapping cumulative
figures for one package, not independent savings or whole-CI attribution.
Existing managed installations are reconciled rather than blindly reinstalled:
the service caches expected content identities, rechecks installed files and
management evidence, and preserves customer modifications. Fresh test homes
still materialize the publications repeatedly.

A generated manifest should contain relative file paths, bytes and modes, keyed
by exact publication input plus layout-generator version and output format.
Reuse immutable rendering, retain independent destination homes and Factory
Sessions, and keep selected validator/pruning/persistence capabilities, portable
file handling, atomic replacement and customer-edit detection. A global cache of
mutable PreparedFactoryLayoutPayload or live session state cannot provide those
ownership guarantees. The current prepared-payload contract contains Config,
Canonical and RootFileName; it does not yet expose a rendered-file manifest.
Measure that larger change against the same customer journeys and full lane
before attributing a speedup or merging a checkpoint.

Additional small-cache experiments do not justify production changes:

- Memoizing the native envelope initially saves 3.9% CPU / 1.5% elapsed:
  baseline 12.125798 CPU-seconds / 17.268215101s elapsed, candidate
  11.651659 / 17.001907400. The subsequent typed-reader approach, which
  parses envelopes once at catalog loading, regresses in confirmation:
  baseline 13.973658 / 18.914790433, candidate 14.242807 / 19.381321658,
  or 1.9% more CPU / 2.5% more elapsed. All six repetitions per side in the
  confirmation pass. An earlier baseline-only portability failure and a
  subsequent passing diagnostic are retained; no candidate ran in that failed
  comparison, and its cause is not established.
- Reusing canonical-output caching in the loading normalizer initially saves
  6.1% CPU / 4.2% elapsed, but confirmation is nearly flat: baseline
  12.601696 CPU-seconds / 17.918361247s elapsed, candidate
  12.715114 / 17.803706988, or 0.90% more CPU / 0.64% less elapsed.
  Both six-repetition-per-side comparisons pass.

Builds are excluded from these execution comparisons. All prototype source is
restored in both checkouts. Evidence is retained in `native-envelope-cache-paired/`,
`parsed-native-reader-paired/`, `parsed-native-startup-diagnostic/`,
`parsed-native-reader-confirmation/`, `normalizer-encoder-cache-paired/` and
`normalizer-encoder-cache-confirmation/` under `.artifacts/latency-audit/`.

### Hosted canonical-cache result and follow-up experiments

Published source `d2b1f09cc0` passes every executed hosted check in run
37474546433. Functional Coverage takes **270s complete / 204.848s coverage**,
67 packages / 763 results, 761 pass and two skip. Its prefix compiler archive
comes from 47760cc66d and it executes **409 compiler commands / 16 links**.
This is substantially more rebuild work than the preceding 138-compiler run;
neither sample isolates the canonical cache's execution-time effect. The
complete two-minute checkpoint remains unmet and PR #2923 remains draft.

Three additional approaches were tested and discarded:

- One initialized CLI process/home for packaged CLI/REST parity, with local
  invocations serialized, saves 33.7% CPU but increases group elapsed 27.0%.
  Baseline totals 13.541772 CPU-seconds / 14.249476 elapsed; candidate
  8.975320 / 18.101596. Six full journey repetitions per side all pass.
- Sharing just the initialized CLI home, retaining independent parallel CLI
  processes, initially saves 11.8% CPU / 3.8% elapsed. Its simplified final
  confirmation regresses: baseline 12.862909 CPU-seconds / 13.565003 elapsed,
  candidate 13.965284 / 14.094856, or **8.6% more CPU / 3.9% more elapsed**.
  The initial favorable pair and final unfavorable pair are both retained.
- A bounded successful-output-schema compilation cache initially saves 4.6%
  CPU / 3.2% elapsed, but its production confirmation is nearly flat:
  baseline 12.132046 CPU-seconds / 17.425941 elapsed, candidate
  12.247028 / 17.324233, or **0.95% more CPU / 0.58% less elapsed**.
  Both six-per-side customer comparisons pass. The candidate's complete
  validation package, concurrent-use, error, external-reference and memory-bound
  tests and both scoped lint suites pass. Correctness alone does not justify
  keeping an optimization whose performance benefit does not repeat.

All three source experiments are restored in both the shared Windows worktree
and private Linux checkout. Their data lives under `cross-cli-fixture-paired/`,
`cross-cli-home-paired/`, `cross-cli-home-confirmation/`,
`output-schema-cache-paired/` and `output-schema-cache-production-paired/`
inside `.artifacts/latency-audit/`.

A fresh native profile of the complete `product/customer_journeys` suite on
d2b1f09cc0 passes at **17.036389s elapsed / 29.826053 CPU-seconds** (22.418198
user / 7.407855 system). Profiling is enabled; this is diagnostic evidence, not
a clean speedup measurement. Of 29.44 sampled CPU-seconds, initialization
accounts for **21.87s / 74.29%**, packaged installation **21.79s / 74.01%**,
fresh managed creation **17.21s / 58.46%**, preparation **11.34s / 38.52%**,
JSON unmarshalling **11.49s / 39.03%**, and the uncached mapper's Expand stack
**7.33s / 24.90%**. These cumulative stacks overlap and cannot be added.
Syscalls account for 6.57s / 22.32% of flat samples. This profile covers one
customer package; do not present its percentages as whole-lane attribution.
Data and cumulative/flat reports are in `canonical-cache-customer-profile/`.

The next substantial target is redundant packaged-definition parsing and fresh
installation preparation, preserving selected validation and filesystem ports.
In particular, the serialized decoder still parses its native envelope and then
parses the contained config again, while preparation and staged validation still
reach uncached mapping paths. The fresh profile supports inspecting those paths
before adding more fixture complexity or relying on small favorable samples.

## Packaged Factory cache follow-up, October 6

Rebased source `47760cc66d` includes live-main subagent MCP and daemon-restart
scenarios. The hosted Functional Coverage job passes in **177s complete /
122.174s coverage**, with 67 packages / 763 results, 761 pass and two skip.
The compiler archive is a prefix hit from f367362e37; this run performs **138
compiler commands and 16 links**. The two-minute complete checkpoint remains
unmet. All executed hosted jobs are now confirmed successful.

The corresponding local unchanged lane passes at **121.66s supervisor /
111.549s coverage / 372.63 CPU-seconds** (307.84 user, 64.79 system). Rebuilding
live-main changes performs 218 compiler commands (80.657503 CPU-seconds) and
19 links (21.473357 CPU-seconds). Their active wall intervals are 31.423484s
and 21.285776s and overlap; do not add them to estimate a critical path.

Caching immutable packaged input is worthwhile; sharing mutable Factory Session
state is not required. The shipped native serialization cache previously saved
25.9% CPU / 19.0% elapsed in six customer runs per side. Installing once in an
owned host and opening distinct explicit Factory Sessions avoids more work:
the Work CLI confirmation comparison saves 47.9% CPU with nearly flat elapsed.

A new private cache of decoded definitions with detached recursive copies saves
only **0.84% CPU / 1.47% elapsed** across six repetitions per side of three
customer journeys: baseline 13.307300 CPU-seconds / 18.358842 elapsed versus
candidate 13.195456 / 18.089478. Discarded; the private source was restored.

A different private cache stores the canonical serialized output after authored
normalization. Generation checks whole-value round-trip equality for all 20
published definitions. Lookup keys hash the exact normalized native input;
nonserialized prompt metadata, unknown fields, edits and misses use the original
encoder. Returned bytes are detached. This wraps only the concrete canonical
encoder: validators, pruning, normalization, portable-file effects, writers,
staged validation and publication still run.

Three unchanged customer journeys, in baseline/candidate/candidate/baseline
order with three repetitions per block, all pass. Builds are excluded from
execution timing. Baseline totals **13.136105 CPU-seconds / 18.087352 elapsed**;
candidate **11.849373 / 17.168674**, a **9.8% CPU / 5.1% elapsed** reduction.
This supports further canonical-encoder work, but the prototype is not shipped
and this focused result is not a complete CI speedup claim. Data lives in
`.artifacts/latency-audit/canonical-encoder-paired/`.

The prototype also passes the full unchanged lane at **113.41s supervisor /
108.470s coverage / 347.54 CPU-seconds**, with the same 67 packages / 763
results, 761 pass and two skip. It performs 118 compiler commands (40.151530
CPU-seconds) and 19 links (21.906707 CPU-seconds), versus the baseline's 218
compiler commands. Different rebuild work prevents attributing the full-lane
elapsed or CPU difference to the encoder cache. This is a compatibility result;
the repeated native-binary pairs are the performance evidence. Private Wire
source is restored byte-for-byte and the prototype file is removed. Full data
lives in `.artifacts/latency-audit/canonical-encoder-full/`.

### Canonical-output production candidate

The candidate now generates optional canonical output alongside each native
conversion and indexes exact normalized-input hashes in the validated catalog.
Catalog loading splits native and canonical bytes once, so the decoder does not
repeatedly scan the larger combined asset. Canonical reads and encoder results
are detached. Missing, stale, malformed and non-object optional output misses
retain the original encoder. Unknown fields, file-backed prompts and runtime-only
worker session, concurrency and model metadata also retain original mapping.
Wire binds the encoder once and injects a distinct typed canonical reader;
validation, pruning, normalization and persistence effects still run.

The first unsplit production pair is nearly flat: baseline 12.969901
CPU-seconds / 17.989911 elapsed, candidate 12.759414 / 17.719674. Retained;
adding encoder output to the native envelope increased repeated decoder work.
After splitting, a six-per-side confirmation passes at baseline **12.958215
CPU-seconds / 18.016064 elapsed**, candidate **12.097475 / 17.207918**:
**6.6% CPU / 4.5% elapsed reduction**. The final metadata-fallback refinement
also passes six runs per side at baseline **13.427476 / 18.325541**, candidate
**12.039741 / 17.308812**, saving **10.3% CPU / 5.5% elapsed**. Same three
customer journeys, alternating blocks and compilation excluded throughout.
Preserve both valid samples, rather than treating the fastest as a ceiling.

An intervening baseline-only sample fails all three portability repetitions
with the existing API-server-startup/shutdown timeouts (80.717671 elapsed).
A fresh WSL instance is observed during investigation; causation is unproven.
The driver stops before running a candidate. That failed sample is retained in
`canonical-encoder-production-split-paired/` and excluded from speedup totals.
The subsequent existing-timeout comparison passes on both sides.

The full production candidate passes unchanged coverage before the additional
runtime-metadata fallback checks, at **135.19s supervisor / 129.679s coverage /
388.06 CPU-seconds**, 67 packages / 763 results, 761 pass and two skip. It
rebuilds 414 compiler commands (72.966217 CPU-seconds) and 19 links
(22.773331 CPU-seconds). This full run overlaps the Windows custom-linter build
and checks; use it as compatibility evidence, not a clean latency comparison.
Final metadata/fallback and all-20-definition equivalence tests pass, together
with both scoped lint suites and all 11 catalog / 16 packaged-source smoke
scenarios using a freshly built custom linter. Hosted validation of the new
candidate is required. The complete two-minute merge checkpoint remains unmet.

The preferred order is: reuse compatible initialized owned hosts with separate
Factory Sessions; cache verified immutable serialization/renderings for tests
that need independent installation; retain independent install, persistence and
restart journeys to exercise those customer paths. A rendered-file cache should
still write to each owned destination and preserve all selected validation and
filesystem effects. Never reuse event ledgers, session state or mutated factory
directories through an immutable packaged cache.


Earlier published source `f367362e37` passes all hosted checks.
Functional Coverage takes **147s complete / 92.644s coverage**, with 67 packages /
762 results, 760 pass, two skip and no failures or retries. Its restored compiler
archive is a prefix hit from d9e4f57e8e; coverage runs zero compiler commands and
16 links. Complete job steps include 98s for the coverage supervisor, 6s checkout,
8s Go setup, 4s module restore, 4s archive restore, 3s Node setup and 7s compiler
archive capture/save. Those rounded step durations are not physical CPU data.
This static-fixture-only revision's faster runtime demonstrates substantial
cache/machine timing variation; do not attribute its 42s difference to the fixture
asset. Earlier `d9e4f57e8e` passes hosted Functional
Coverage in **189s complete / 139.494s coverage**, 67 packages / 762 results,
760 pass and two skip, with no failures or retries. Its compiler archive is a
prefix hit from c8b026366d; coverage runs two compiler commands and 16 linker
commands. Every other executed job passes except Backend Lint: its migration
smoke fixture publication predates native conversion assets and reports one
missing conversion in the default clean-source cell. f367362e37 regenerates that
tiny fixture through the canonical catalog generator with assertions retained.
All 16 packaged-source and 11 catalog smoke scenarios pass locally, and the
fresh hosted Backend Lint check passes. Parent c8b026366d passes Functional
Coverage in 209s / 143.728s but fails the earlier catalog drift expectations,
corrected in d9e4f57e8e. Earlier source `643e6f3b7b` passes every hosted workflow check in
201s complete / 142.303s functional coverage with one recovered interrupt
admission failure. Parent `69eaa32820` takes 210s / 150.914s coverage with
recovered interrupt and Script failures; its remaining workflow is canceled by
the next push. The preceding
`f7e138ae4e` functional job takes 222s complete / 154.403s coverage, with one
recovered Agent cancellation failure, and its integration job fails the
released probe-port race. The preceding `e243441e58` workflow passes
all checks, with Functional Coverage taking **247s complete / 169.073s coverage**,
no retries, 110 compiler commands and 17 links after a prefix f0 archive restore.
The complete hosted two-minute checkpoint
remains **unmet** and the PR stays draft. The earlier warm three-minute
checkpoint is merged. Keep prior complete 146s, 198s and 207s samples and their
cache/retry differences; 146 seconds is not a reliable ceiling.

The generated native conversion cache passes complete local validation. A warm
candidate takes **140.07s complete / 133.380s coverage / 449.12 CPU-seconds**,
versus a clean repeated disabled-reader baseline at **193.65s / 184.204s /
636.78 CPU-seconds**, with zero compiler commands and 19 links on each side.
This one complete comparison saves 27.7% elapsed time / 29.5% CPU. Six-per-side
native customer journeys save 19.0% elapsed / 25.9% CPU. Cold rebuild cost,
baseline timing variation and the earlier baseline's recovered admission failure
are preserved below. Fresh hosted validation is required; 140 seconds remains
above the complete two-minute checkpoint.

A private rendered-file cache prototype passes the complete unchanged functional
lane. In one matched four-CPU comparison, baseline takes **181.21s / 564.70
CPU-seconds**, versus **145.74s / 452.47 CPU-seconds** for the prototype: about
20% less elapsed time and aggregate CPU. This supports caching immutable,
format-specific packaged Factory renderings. It does not justify sharing live
Factory Session state. The prototype is not shipped; it needs the production
writer and validation dependency boundaries preserved. The failed JSON-only
prototype and the slower prepared-struct cache experiment are retained below.

The published worker CLI home and native-join change passes full local coverage
with **38 monolith groups / 569 registrations**, all **68 packages / 738 results**,
no retries, unchanged coverage floors and quarantine. Finite worker CLI pairs
use **32.1% less CPU / 11.7% less wall**; removing the invoke/continue framework
subprocess probe uses **18.0% less CPU / 19.2% less wall** in fresh-process pairs.
Work watch now joins through lazy setup and owned cleanup. A warm-helper local
sample takes 140.95s / 451.29 CPU-seconds; final shared-helper correction takes
204.98s / 688.92 CPU-seconds including 117 compiler commands. Both samples are
retained, and neither establishes a complete hosted checkpoint.
The published MCP change shares one initialized, owned client home and model
cache across independent stdio connections. Six native repetitions per version
use **42.5% less CPU / 34.1% less elapsed time**. Full local validation preserves
all 740 results and coverage gates without retries at **143.62s supervisor /
136.618s coverage**, using **453.12 CPU-seconds**. This full run is slower than
the previous local 129.30s / 398.02s sample; the controlled package improvement
must not be presented as a full-lane improvement.
The published candidate shares one host across lifecycle, dispatch and eligibility
journeys, retaining all 31 journey selectors with isolated Factory Sessions,
command routes and owned home/model storage. Six native repetitions per version
use **36.6% less CPU / 5.8% less wall**. Removing a test-framework subprocess
probe permits concurrency to join the monolith without weakening its classifier;
that package uses **41.7% less CPU / 14.1% less wall** across six repetitions.
These targeted comparisons exclude build time and do not predict hosted totals.
A full local four-CPU validation passes 68 packages / 744 results without retries
in **165.72s supervisor / 159.276s coverage**, using 556.32s aggregate CPU.

The published e02 candidate removes a service-only inventory probe, reuses an owned
home for sequential JavaScript policy invocations, and releases the final-history
read's unused live subscription before runtime teardown. The policy home change
uses 42.3% less native execution CPU across six repetitions; separately, fixing
history subscription ownership cuts six repetitions from 20.713s to 2.631s.
Focused component regressions and lint pass. The full four-CPU lane passes in
171.33s supervisor / 165.362s coverage, 68 packages / 743 results, no retries.

The refreshed published-source monolith profile samples **272.45 CPU-seconds**
over 86.05s wall. System initialization consumes **161.64s cumulative (59.33%)**,
mostly packaged Factory installation/layout preparation. These nested values
must not be summed. Package labels include fixture background goroutines:

| Functional package suffix | Sampled CPU | Initialization subset |
| --- | ---: | ---: |
| product/customer_lifecycles | 48.34s | 16.98s |
| product/customer_journeys | 48.33s | 38.86s |
| factory/execution | 20.97s | 11.75s |
| product/cli_rest_journeys | 18.26s | 12.55s |
| transport/mcp/worker_sessions | 14.46s | 12.69s |
| models/inference | 11.76s | 7.45s |

Prioritize already-initialized owned homes and isolated sessions in customer
journeys and MCP worker-session setups. Customer lifecycles also has a distinct
25.30s ReviewFailureRecovery story: its focused stack shows JSON-heavy public
session opening (9.28s) and Work admission (7.78s), rather than initialization.
Avoid merging more constructors on the assumption that alone removes bootstrap
work. The native packages require separate profiling; this table only covers
the 36-group monolith. Source and measurement caveats are retained below.

### Prior checkpoint measurements

The five-minute checkpoint merged in
[PR #2867](https://github.com/portpowered/you-agent-factory/pull/2867), commit
`0dcdbf1fde04e573730e19c4b39fe097e7389d8c`. Its merge-queue functional supervisor
passed in **272.327s**, including **215.102s coverage invocation**. An identical
source tree also took 340.850s; both samples and recovered flakes are retained
below. The three-minute warm-cache checkpoint merged in PR #2905; the two-minute checkpoint remains outstanding. Cold-cache hosted jobs remain above three minutes.

Latest measured head `f5d203cad5` passes all 68 packages / 758 results with
no retries. The [hosted functional job](https://github.com/portpowered/you-agent-factory/actions/runs/37416481644/job/112116898954)
takes **226s complete / 161.028s coverage invocation**, with 64 compiler actions
and 22 links. It combines 33 compatible package groups / 491 tests. Its identical-head warm-cache rerun passes in **155s complete / 111.277s
coverage**, zero compiler actions and 22 links, with all 758 results and no
retries. This demonstrates the three-minute checkpoint on a warm cache; the
changed-source 226s sample remains material. The earlier identical
`ec85aae468` archive-cache rerun took 223s complete / 167.054s coverage, zero
compiler actions and 25 links. Changing candidates and runner variability do
not establish a consistent total-job speedup from the last three links removed.
These current-runtime results supersede pre-rebase latency for merging.

The last pre-rebase hosted candidate, `592184914d`, passes the complete functional job in **192s (3m12s)**, with **147.527s coverage invocation**, all 68 packages/751 results, and no retries. The complete job remains above the three-minute checkpoint. Successful restored-cache jobs observed during this work range **192–237s**; this is a sequence of changing candidates, not identical-source repetitions. The eight-way candidate reduces concurrency and combines 26 compatible packages/373 tests, while preserving customer guarantees and coverage floors. Cold archive population still takes roughly five minutes.

The first hosted next-phase run, at `0878fd26d0`, passes in **316.816s** full
supervisor / **287.224s** coverage invocation. Its trace reports 1,355 compiler
commands and 73 linker commands, and one MCP ACP permission case passes on a
same-head retry. The existing 67 MB dependency cache leaves repository packages
cold; the warm local result below does not predict this cold hosted run. The
next change tests a bounded compiler-archive cache instead of the previously
removed whole build-cache restore, which cost roughly four minutes for 7 GB.

The next candidate's full four-CPU Linux supervisor passes in **111.55s**, with
**103.120s coverage invocation**, all **758 selected tests** accounted for,
756 passes, two skips and no retries. Coverage floors, the selected inventory,
quarantine verification and compile-probe diagnostics remain enabled. Compatible
packages use a build-only overlay: 19 packages share an executable and 36
fixture-sensitive packages keep native binaries. Original source coordinates,
test identities and evidence declarations are preserved. The coverage trace
records **37 linker commands**. This is a warm-cache local result, not a hosted
checkpoint or a cold-build guarantee.

Consolidation has a measured benefit. A controlled build-only comparison reduced
**136 linker invocations to one**, **160.292s linker CPU to 1.807s**, and warm
build wall time from **44.29s to 3.95s**. CPU time is summed across processes;
it is not elapsed time that can be subtracted directly from a CI duration.
The latest private single-binary execution repeat **passes** after removal of
a process-wide working-directory mutation. It combines 63 package groups and
takes **133.03s command wall / 116.653s package execution**, with **one linker
at 1.609s wall / 1.585s CPU**. The repository's coverage evaluator accepts the
recorded profile against unchanged total and package gates: **61.4% total**.
This is an experimental package execution, not the full CI supervisor. Peak RSS
is **6.28 GiB**; fixture memory and hosted behavior still need validation before
shipping the all-in-one layout.

The current source layout converges another **21 packages** into three existing
customer-use-case packages: Factory execution, packaged Factory invocation,
and Work admission. This preserves ordinary and `functionallong` cases and
their session-owned fixtures. Independent parent tests overlap through Go's
test scheduler. Sixteen composition-oriented test names now describe customer
CLI, REST, Work, session control, recording and replay behavior. Coverage floors are unchanged. The rebased manifest preserves all 160 reviewed
scenario decisions, including the new captured-activity surface from main. Six further CLI packages
share one customer-command package. Four workflow fixture suites share one
package, and 28 compatible suites now share Product customer journeys, removing
27 more binaries while retaining their explicit sessions and public observers.
Fifteen further fixture-owning suites now share Product customer lifecycles,
removing another 14 binaries. Their former package setup and shutdown belong to
parallel parent tests; cleanup runs after their parallel children. Repetition
resets fixture state, and resume fixtures resolve repository paths explicitly.

Detailed measurements below include failed runs and their causes. Remaining
work is to consolidate fixture-heavy packages with their own lifecycles, reduce
scenario startup and execution costs, and verify the complete hosted lane at
each requested checkpoint.

## Initial finding

### Latest fixture consolidation measurement

The additional customer-lifecycle package combines 15 fixture-owning suites:
audio artifacts, packaged catalog/classification/CLI–REST parity, review failure
recovery, operator settings, Factory dispatch/eligibility/session lifecycle,
CLI discovery/resume/worker lifecycle/modes, and Work routing. MCP protocol
remains separate after the 16-suite candidate exposed startup contention.
Only their test packaging changes. Each fixture remains owned by one parallel
parent and its customer scenarios; independent parents have independent routes,
sessions, homes and cleanup. Package state resets between repeated runs.

The first full four-CPU Linux coverage run of the 16-suite candidate took **244.70s overall
/ 194.208s tests**, and failed. It observed **68 links**, consuming **95.742
CPU-seconds** with **142.827s of active wall time**, and **1,543 compilations**
consuming **234.302 CPU-seconds**. The previous synchronized-layout sample had
80 links / 110.065 linker CPU-seconds, but only 121 compilations. The new sample
used a fresh source directory and rebuilt substantially more dependencies;
these are different cache states and are not a controlled elapsed-time comparison.
Active compiler and linker intervals overlap each other and running tests.

The run reported 822 top-level passes, two skips and three failures across
two packages. Failures were an incorrect fixed-UUID expectation added during
identity-test cleanup, MCP protocol startup exceeding its existing five-second
guard, and the unchanged injected invocation cancellation case returning an
empty request/session identity. The UUID generator deliberately returns fresh
UUIDs: the corrected customer assertion checks the persisted local UUID and
retains the separate existing-identity reuse proof. MCP now uses the shared
isolated-home helper to exclude operator model caches. The cancellation failure
remains recorded; this failed sample establishes no latency checkpoint.

Pure fixture-ledger checks for packaged cross-surface characterization and the
guard fake's own marker-selection self-test are removed. Operator-settings
construction counters are also removed; CLI output, settings-file persistence,
UUID validity/reuse, session/Work/Event correlation and customer failure
contracts remain. Large migrated scenarios now have named assertion stages
and case helpers, split into focused files. The migrated package's built-in
and repository lint, including the integration/long/conformance tag union,
report zero issues on the migrated package. Three complete native repetitions
of the final customer-lifecycle scenarios pass in 123.771s. Manifest projection
still validates all 159 reviewed decisions. Catalog fixtures now author their
known customer paths directly instead of calling the production path-policy
function to arrange the assertion.

The warm-cache repeat took **174.52s overall / 164.897s tests**, but also
failed: MCP's initial protocol handshake exceeded five seconds, an unchanged
Worker Session missing-transcript case returned recording-unavailable, and an
unchanged mock-worker case failed temporary-directory cleanup. The initial
UUID and cancellation-attribution failures did not recur. This is another
failed sample, not achievement of the three-minute checkpoint.

| Warm-cache phase | Invocations | Aggregate CPU seconds | Active wall seconds |
| --- | ---: | ---: | ---: |
| Compile | 2 | 4.200 | 11.093 |
| Link | 65 | 93.965 | 133.330 |
| Vet | 27 | 0.260 | 1.054 |
| Entire supervisor | — | 627.560 | 174.520 |

Linking remains costly after dependencies are cached, but its active interval
overlaps execution. It accounts for approximately 15% of this sample's total
CPU; eliminating repeated linking alone does not eliminate the remaining
customer-scenario and harness compute. The two-minute target also requires
reducing repeated application construction and execution costs.

| Largest package execution in this warm sample | Seconds | Outcome |
| --- | ---: | --- |
| Product customer journeys | 126.966 | Pass |
| Product customer lifecycles | 94.786 | Fail |
| Packaged Factory invocation | 54.533 | Pass |
| Factory execution | 52.327 | Pass |
| Models inference | 47.026 | Pass |
| Work admission | 39.627 | Pass |
| MCP stdio | 37.629 | Pass |

An MCP experiment completed public first-run home initialization before
starting the protocol response guard. Three focused native repetitions passed
in 14.477s, but full coverage still failed two startup handshakes and the
unchanged mock-worker cleanup. That run took **167.46s overall / 158.046s tests**
with 65 links / 85.919 linker CPU-seconds and two compilations / 4.096 CPU-seconds.
It reported 823 passes, two skips and two top-level failures; it is not a passing
checkpoint. The extra initialization is not shipped because it did not solve
the failure. MCP protocol remains in its separate transport package with
explicit model-cache isolation and its five-second guard unchanged. The other
15 suites remain consolidated; the final layout needs hosted verification.

The initial measurements below are followed by an [implementation and validation
update](#implementation-and-validation-update) for the authorized cleanup.

The three-minute goal is not met. Two successful Linux functional jobs took
**476 seconds and 622 seconds**. Their authoritative coverage invocations took
**344.601 seconds and 421.149 seconds**. The work is split between repeated
compilation/linking, expensive customer scenarios, and work outside the measured
coverage invocation. Reducing timeout constants or enabling more parallelism
alone will not solve it.

The highest priorities are:

1. Isolate model caches and customer profiles, then remove repeated bootstrap
   and application construction where immutable process reuse is safe.
2. Profile the slowest customer journeys on the actual four-CPU Linux runner.
3. Reduce the approximately 146 full-application test binaries, while retaining
   scenario concurrency and customer guarantees.
4. Fix synchronization failures and move fixture, source-shape, and executable
   tests into their correct lanes.
5. Cache the exact coverage build graph and account for the entire harness,
   not just `go test` package durations.

One optimization was demonstrated without modifying committed tests: a private
empty model cache reduced an isolated Models test's median package execution
from **15.929 seconds to 1.220 seconds**, with all assertions passing. Package
consolidation reduced link work but did **not** demonstrate an overall latency
improvement in the tested group.

## Evidence and measurement boundaries

### Hosted CI

These are two observed samples, not a statistical baseline or a controlled
before/after comparison. They ran different revisions:

| Measurement | Earlier run | Later run |
| --- | ---: | ---: |
| Run | [37262603762](https://github.com/portpowered/infinite-you/actions/runs/37262603762) | [37270620737](https://github.com/portpowered/infinite-you/actions/runs/37270620737) |
| Revision | `11e0ad42e37699ebe28f56cc496a3fa6f76bd0d0` | `8079404795e9799312144eb52a0805aad456e205` |
| Whole Functional Coverage job | 622s | 476s |
| Supervisor/coverage child | 572.221s | 425.438s |
| Measured coverage invocation | 421.149s | 344.601s |
| Difference: coverage child minus measured invocation | 151.072s | 80.837s |
| Concurrent quarantine validator | 125.515s | 56.502s |
| Selected packages | 149 | 149 |
| Top-level tests in timing summary | 1,064 | 1,064 |
| Summed package execution time | 2,782.136s | 2,248.576s |
| Compiler commands in build diagnostics | 892 | 864 |
| Linker commands in build diagnostics | 146 | 146 |

The later job log reports **four logical CPUs and 12 package jobs**. Its
dependency build cache restored successfully in about two seconds. The earlier
job did not have that restore step. This is evidence of cache behavior, not
proof that the cache caused the entire difference between runs.

The earlier run retained an initial failure of
`TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected` in
`workers/mock` and completed successfully with retry machinery. Keep the
slower successful sample in the range; a green job is not evidence that its
first attempt was clean.

The quarantine validator overlaps coverage. Its duration must **not** be
added to the coverage child's duration. Likewise, the 80.837–151.072-second
difference is unattributed harness time, not a measured compiler duration.
It includes work before/after the timed invocation and can include retry work
and reporting. The invocation timing excludes the enclosing `go run`
commands, discovery, and other checker/report phases.

### Local experiments

Host: Windows/amd64, Go 1.25.0, 24 logical processors, approximately 64 GiB RAM.
The build cache was inherited; it was not globally cleared. Other unrelated
Go workloads were observed on this machine. Local numbers explain mechanisms
and are not substitutes for measurements on the Linux CI runner.

All executions used `-count=1`, so test-result caching cannot explain their
speed. Broad diagnostic commands explicitly used `-p=12` to reproduce CI's
package budget. This is a diagnostic configuration, not a proposed change to
the canonical Windows lane.

Measurement methods:

- Outer subprocess stopwatch: complete command wall time and exit status.
- `go test -json`: package and individual test execution durations/outcomes.
- Go 1.25's internal `-debug-trace`: package-loading and build-action spans.
- A temporary `-toolexec` wrapper: actual tool subprocess wall time, user CPU,
  system CPU, package attribution, and exit status.
- CPU and blocking profiles for the slow Models named-route scenario.

Summed tool wall time and summed package time represent concurrent work, not
elapsed job time. CPU seconds can exceed wall seconds because tools use
multiple threads. Trace unions show time with at least one action of a class
active; different classes overlap and their unions are not additive.

The first two tool-timer attempts had filename collisions. Their overall
command timings and Go traces remain valid, but their tool counts/CPU sums
are incomplete and are excluded from the authoritative tool totals below.
The corrected forced-rebuild capture has no malformed records and accounts
for all 145 linked packages.

## What CI actually runs

The required lane is:

```text
ci.yml: Backend Functional Coverage
  run-functional-coverage-with-quarantine.sh
    concurrent: verify-functional-quarantine-packages.sh
    concurrent: make functional-test-viz
      go run ./cmd/functionaltestviz
        go run ./cmd/gocoveragecheck
          discover current packages/tests and subtract quarantine
          go test -coverpkg=<backend packages> -p=12
                  -covermode=count -timeout=10m -json -x ...
          coverage checks, failure capture/retry, reports
        render diagnostics and verdict
```

This is full, non-short coverage; it is not equivalent to the default short
`make test-functional` command. The captured invocation instruments **556
backend packages**. There are 157 discovered functional packages, eight
package exclusions, and one quarantined live-test selector. The selected
inventory is 149 packages and 1,064 top-level tests.

The current functional dependency-cache workflow builds ordinary standard and
third-party dependencies, but does not preserve the complete project coverage
graph or the functional test binaries. Cache hits therefore do not mean that
the required coverage compilation/linking work disappeared. Both observed
jobs still compiled hundreds of packages and linked 146 executables.

The quarantine script also invokes the coverage checker for validation and
runs exact-package `go test -list` commands for default and `functionallong`
tags. This is additional compilation/discovery pressure even though it is
off the critical path once coverage takes longer. Preserve its validation
guarantees while avoiding duplicate discovery or unnecessary test linking.

## Highest package execution latencies

These durations come from hosted `go test` package outcomes and exclude
package compilation/linking. Paths are relative to `tests/functional/`.

| Package | Later run | Earlier run | Audit focus |
| --- | ---: | ---: | --- |
| `models/root_composition` | 73.678s | 89.668s | Model cache/readiness IO; repeated application/server fixtures |
| `factory/review_failure_routing` | 58.564s | 73.670s | Shared host bootstrap/profile ownership; scenario IO and polling |
| `sessions/chat_sessions/root_composition` | 49.552s | 60.637s | ACP journeys, profile bootstrap, public completion/cleanup |
| `factory/visualization/runtime_metrics` | 49.413s | 56.701s | Multiple immutable fixtures, selected-clock/readiness phases |
| `providers/acp` | 46.277s | 64.855s | Provider protocol lifecycle, cancellation, daemon/process boundaries |
| `sessions/execution` | 45.516s | 56.247s | Repeated process/server setup; result and response observation |
| `transport/mcp/stdio` | 35.309s | 42.964s | Protocol startup/shutdown and stream completion |
| `sessions/root_composition` | 35.133s | 43.630s | Recording/lifecycle matrices and repeated setup |
| `events/factory_events` | 34.960s | 43.396s | Webhook/fault journeys, repeated server setup, polling |
| `factory/replay_contracts` | 33.121s | 42.277s | Persisted/replay guarantees and fixture cost |
| `provider_sessions/details` | 32.468s | 43.680s | Transcript/session inspection fixtures |
| `factory/process_time` | 32.330s | 39.139s | Existing controlled-clock journeys and fixture overhead |
| `factory/packaged/named_invocation` | 30.198s | 40.488s | Multi-stage packaged customer journeys |

Notable individual hosted tests:

| Test | Later run | Earlier run |
| --- | ---: | ---: |
| `TestModelsNamedBuiltinRouteUsesEffectiveDefinitionWithoutWorker` | 56.33s | 44.17s |
| `TestAPIResultAndResultsExposeTerminalInvocationData` | 25.27s | 25.93s |
| `TestCLISettingsUnknownFieldsPrecedenceAndSafeFailure` | 24.91s | 27.97s |
| `TestFactoryWebhooksRecordedFaultsAndPeerContinuation` | 23.11s | 18.14s |
| `TestPackagedDubVideoRunsFourStagesAndRejectsFailures` | 17.84s | 23.45s |

Do not sum individual test times to infer package wall time: parallel tests,
subtests, fixture setup, and cleanup make those different metrics.

At the observed package durations, ideal scheduling into 12 slots has a lower
bound of **187.381–231.845 seconds**, before build and harness work. This bound
holds for those measured durations; reducing contention or changing fixtures
can change the durations themselves. The current average occupied package
slots over the coverage invocation is only about 6.5–6.6, illustrating that
the lane also pays build barriers and uneven scheduling.

## Build, linking, instrumentation, and vetting

Three local covered build-only commands used `-run=^$`. They select no test
bodies, but still include package loading, compilation, coverage generation,
linking, vetting, test-process startup, and any `TestMain` work.

| Experiment | Wall | Outcome |
| --- | ---: | --- |
| Initial covered build, inherited cache | 77.820s | pass |
| Repeat, warm compiler cache | 50.731s | pass |
| Corrected timer, warm compiler cache | 49.197s | pass |
| Forced rebuild, `-a`, including standard/third-party dependencies | 111.587s | pass |

The local pattern `-coverpkg=github.com/portpowered/infinite-you/pkg/...`
instruments a broader set than CI's explicit 556-package list. It also
includes Windows-selected functional packages and support packages: 158
package outcomes and 145 link actions. Consequently, these are diagnostic
build experiments, not exact CI lane reproductions.

The forced rebuild's actual child-tool costs were:

| Tool | Calls | Summed tool wall | User CPU | System CPU |
| --- | ---: | ---: | ---: | ---: |
| Compiler | 1,251 | 243.901s | 144.000s | 85.531s |
| Linker | 145 | 266.389s | 192.141s | 106.672s |
| Vet | 1,111 | 143.242s | 37.063s | 35.703s |
| Coverage instrumentation | 644 | 61.902s | 9.406s | 13.500s |
| Assembler | 93 | 9.657s | 0.859s | 0.938s |

Linking consumed **298.813 child CPU seconds**, more than compilation's
**229.531**, and approximately **48% of the captured tool CPU total**. These
totals exclude `cmd/go`, the timer wrappers, test processes, and OS activity
outside the recorded children.

In the warm build, no compiler or coverage-tool calls were recorded, yet
145 link actions remained. At least one link action was active for **44.995
of 49.197 seconds**. A better compiler cache helps, but cannot by itself remove
this warm-build linking floor. The corresponding build-action union was
2.687 seconds. Build and link unions overlap with vet and test startup.

### Highest compiler latencies

Measured actual compiler subprocesses in the forced-rebuild sample:

| Package | Wall | User + system CPU |
| --- | ---: | ---: |
| `modernc.org/sqlite/lib` | 5.840s | 12.406s |
| `github.com/dop251/goja` | 3.766s | 9.016s |
| `runtime` | 1.238s | 3.172s |
| `pkg/transports/http/generated` | 1.218s | 2.000s |
| `packages/packaged-factories` | 1.111s | 0.063s |
| `tests/functional/models/root_composition_test` | 1.104s | 2.422s |
| `pkg/services/factory_sessions/internal/execution` | 1.091s | 2.234s |
| `net/http` | 1.045s | 2.703s |

These identify where expensive cold compilation is concentrated. Do not
rewrite SQLite, Goja, or generated contracts just to shave this sample. First
verify that CI's dependency cache hits for the actual build flags and retain
project coverage archives where useful.

### Highest warm linker latencies

| Test binary | Wall | User + system CPU |
| --- | ---: | ---: |
| `factory/packaged/named_invocation.test` | 4.190s | 2.906s |
| `factory/packaged/quorum.test` | 4.059s | 2.719s |
| `factory/packaged/plan_execute.test` | 3.990s | 2.656s |
| `factory/packaged/plan_parallel.test` | 3.940s | 2.672s |
| `factory/packaged/javascript_families.test` | 3.761s | 2.500s |

Individual links are only a few seconds, but almost every functional package
links the broad application dependency graph. The aggregate is the problem.

## Experiments on the proposed optimization approaches

### 1. Session/process reuse and cache isolation

The Models named-route test already calls `t.Parallel`, but creates a separate
API host using `StartFunctionalAPIServer`. Its configured edge only replaces
provider execution. It then lists/inspects effective models and checks two
named-route failures. Its missing-model/readiness contract does not need a
populated developer model cache.

The targeted CPU profile attributed **13.22 cumulative sample seconds** to
`InspectRuntimeCache -> inspectGenericRuntimeFiles -> verifyCachedFile ->
fileSHA256`, including **11.32 seconds** through model catalog listing. Windows
fsnotify/syscall frames also appear prominently. These profile samples are
diagnostic attribution, not additive phase durations or proof of the same
cause in Linux CI.

Three ordinary isolated invocations and three identical invocations with
`INFINITE_YOU_OMNIVOICE_CACHE_DIR` pointed at fresh audit-owned directories:

| Metric | Ambient cache | Private empty cache |
| --- | --- | --- |
| Command wall samples | 24.202, 19.420, 16.518s | 4.640, 3.811, 5.753s |
| Command wall median | 19.420s | 4.640s |
| Package execution samples | 19.855, 15.929, 13.658s | 1.220, 1.173, 1.438s |
| Package execution median | 15.929s | 1.220s |
| Verdicts | 3/3 pass | 3/3 pass |

That is approximately **76% less command latency and 92% less package
execution latency** in this local experiment. The scope of the evidence is
this one test and this host. Verify the gain on Linux before extrapolating it
to the 73–90-second package.

Static source review also found:

- `models/root_composition` has approximately 50 application-construction
  call sites across helpers and scenarios, plus repeated API-host setup.
  Different immutable injected edges may legitimately require separate
  processes; reuse only within compatible cohorts.
- `sessions/execution` has approximately 23 construction call sites and
  repeated server fixtures. Its 25-second result/results journey is a good
  next profile target.
- Review-failure routing already owns one shared process and opens explicit
  Factory Sessions. Recommending a fresh session conversion for the whole
  package would duplicate completed work. Its shared host uses `FakeInputs`,
  whose default environment is `os.Environ()`, without an invocation-local
  profile override in that construction path. Fix this remaining bootstrap
  ownership issue and measure it.
- Chat Session composition already has a controlled cohort with distinct
  Chat/Factory Sessions and working roots. Diagnose remaining setup and
  completion phases rather than replacing that fixture wholesale.
- The invalid generic CLI Models fixture holds `executeMu` across each
  invocation and observes aggregate effect counters. Its subtests can be
  marked parallel while the customer commands remain serialized. Attribute
  effects to the scenario before removing that lock.

Across all source files under `tests/functional`, a static scan found 280
explicit `root`/`support` construction sites, 56 `time.Sleep` sites, and 120
files mentioning `~default` or `DefaultSessionID`. These counts include support
code and tagged files and are triage signals, not runtime invocation counts
or evidence that every default-session assertion is invalid.

### 2. Timeouts, deterministic observation, and synctest

Go 1.25 already supports `testing/synctest`. It advances fake time when the
bubble's goroutines are durably blocked and provides `synctest.Wait` for
quiescence. Real network and filesystem IO are not automatically converted
into controlled fake effects. Use it for suitable isolated scheduling,
deadline, retry, and cancellation components; use injected clock/runner edges
and public events for whole-application journeys. See
[Testing Time](https://go.dev/blog/testing-time) and the
[synctest contract](https://pkg.go.dev/testing/synctest).

No functional source file currently imports `testing/synctest`. However,
`factory/process_time`, scheduling selected-time tests, and runtime-metrics
selected-time tests already implement controlled clock/timer edges. Their
32–39-second package times are not proof of real-time sleeping: profile their
application/bootstrap/IO work before replacing the clock implementation.

Good observation-conversion targets include the repeated pollers in
`sessions/root_composition/p3_p7_behavior_matrix_test.go`,
`events/factory_events/order_and_cursor_test.go`,
`recordings/root_composition/record_replay_lifecycle_activation_test.go`, and
`work/recovery/helpers_test.go`. Review-failure routing's Work-state polling
documents a real ordering distinction: a dispatch response can precede the
public Work projection. Replace it only with a signal that proves the
projection is visible, not merely with the earlier response event.

Keep safety timeouts as ceilings that return as soon as completion arrives.
Shortening a five-minute package timeout to three minutes would make the
observed blocked test fail earlier; it would not make its customer behavior
correct or its synchronization deterministic.

### 3. Joining test packages

Temporary source copies combined `plan_parallel`, `fusion`, and
`deep_research` into one package. Helpers were namespaced, assertions and
fixtures were retained, and the temporary package was removed afterward.
The initial copy had a generic-helper name collision and failed compilation;
those failed samples are retained but excluded from performance conclusions.

The corrected experiment showed:

| Arrangement | Command wall samples | Median | Verdicts |
| --- | --- | ---: | --- |
| Three original packages | 10.152, 10.471, 11.890s | 10.471s | 3/3 pass |
| One merged package, original serial top-level tests | 18.309, 21.425, 21.907s | 21.425s | 3/3 pass |
| Three original packages, second paired series | 11.093, 11.458, 17.056s | 11.458s | 3/3 pass |
| One merged package, parallel top-level tests | 12.292, 15.613, 11.267s | 12.292s | 3/3 pass |

The build-only comparison reduced **three linker calls to one**. In the
first corrected comparison, actual linker CPU dropped from **4.844 seconds
to 1.266 seconds**, and build-only wall time from 4.449 to 2.714 seconds.
The second comparison included compilation of the changed merged test source
and had build-only wall times of 3.192 versus 3.305 seconds.

The original top-level tests do not call `t.Parallel`; separate packages
provided their overlap. Naive joining removed that overlap and doubled
latency. Adding top-level parallelism helped, but the complete paired ranges
overlap and the merged median remained slightly slower. **This experiment
proves lower link compute, not a consistent end-to-end speedup.**

Consolidation should therefore pair compatible behavior cohorts with preserved
parallelism and, where appropriate, one reusable immutable process. Retain
scenario-owned profiles/routes/sessions and independent cleanup. Use a
bounded number of binaries that can still keep the runner busy; one giant
serial test package is not the goal.

### 4. Removing non-customer test work

Concrete reclassification candidates:

| Current test/work | Appropriate destination |
| --- | --- |
| `TestFunctionalTestVizLaneScriptSmoke_UsesCanonicalOwnedCommandAndCapturesLog`: reads workflow/supervisor source and searches strings | Static CI/lint check |
| `TestFunctionalTestVizLaneScriptSmoke_PreservesFailureExitAndLog`: reads Makefile and Go source to check implementation strings | Runner-owned unit/behavior tests; static checks for routing |
| `TestClaudeHaikuGoldenRouterRejectsInvalidRoutesWithoutLeaks`: duplicate fake routes, fake route counts, fake close behavior | Unit tests for the support router |
| `TestJourneySchedulerSeparatesReadinessAndSourceWaits`: custom fixture clock behavior | Support-boundary unit test |
| Helper-executable/Go-test-binary cases in provider ACP/mock-worker coverage | Small compiled-artifact integration proofs, where OS behavior is the actual contract |

`observability/verification` costs **16.945–20.528 seconds** as a package, but
only part of that package is static checking. Do not claim that moving the
two identified source checks saves the entire package duration.

The customer promises for persisted identity, cache reuse, recovery,
redaction, CLI/API parity, and replay remain valuable. Retain one focused
public-boundary proof for each distinct promise. Filesystem assertions that
verify a customer-visible saved/exported artifact are not automatically
source-shape tests. Reconcile coverage floors after justified reclassification
rather than keeping internal inventory tests solely for incidental coverage.

## Failures in the local full diagnostic

The full non-short, non-covered local command completed in **634.675 seconds**
with 158 package outcomes and exit status 1. Failed packages were:

- `automations/scheduling`
- `orchestration/javascript/durability`
- `sessions/root_composition`
- `transport/mcp/protocol`
- `workers/inference`
- `work/watch`
- `transport/submit`

The submit package consumed **300.389 seconds** and hit the five-minute package
timeout. `SUB-015 deadline reaches handler and joins` was blocked in
`waitForSignal` at `matrix_test.go:592`, called from line 285: the test was
waiting for its handler-observation signal before joining the deadline-bound
command. That receive has no local ceiling. The intended 250ms cancellation
contract can therefore turn into a whole-package timeout if the handler is
never reached. Investigate the fixture/request path and await either the
observed handler or the command's terminal error. Do not silently accept
deadline expiry before the scenario's required observation.

The same submit package passed in the later Linux sample in 7.530 seconds.
This local failure is not proof that it caused the sampled Linux slowdown.
The failed local full run, concurrent ambient workload, broader inventory,
and tracing prevent it from serving as a clean Linux baseline. Focused cache,
build, and corrected consolidation experiments all passed. No full passing
suite under three minutes was demonstrated.

## Plan to reach a three-minute whole-job budget

Define the target as **start of Functional Coverage job to completed verdict
and diagnostic uploads**, excluding GitHub queue time and unrelated jobs.
Do not relabel only package execution or a warm cached replay as the total.

Proposed budgets are acceptance targets, not measured forecasts:

| Phase | Target |
| --- | ---: |
| Checkout, tool setup, restore, dependency availability | 35s |
| Harness/discovery, build/instrument/link critical path | 45s |
| Customer-scenario execution critical path | 80s |
| Coverage gate, verdict, diagnostic upload | 15s |
| Margin | 5s |
| Whole job | **180s** |

Build and execution overlap in today's runner. Phase budgets must be checked
against the actual dependency graph, not enforced by simply summing overlapping
timers. The table allocates a practical total for the optimized lane.

### Recommended implementation order

1. **Fix isolation and blocking first.** Give every invocation explicit
   profile/cache roots, allocate session identity before runtime opening,
   retain profile bootstrap as a prerequisite outside readiness clocks unless
   first-run behavior is the assertion, and fix unbounded observations.
   Start with the demonstrated Models cache issue and the local submit failure.
2. **Measure the missing hosted phases.** Publish package load/discovery,
   checker/tool build, compile, instrumentation, vet, link, execution,
   retries, coverage processing, and rendering/upload spans. Capture actual
   per-tool package timings and CPU, not just `-x` command counts. Explain the
   81–151 seconds outside the timed coverage invocation before claiming a
   three-minute total.
3. **Reduce the slowest runtime cohorts.** Profile Models, result/results,
   review-failure routing, ACP, Chat Session composition, and runtime metrics
   on the four-CPU runner. Group compatible edges into reusable processes;
   remove unnecessary servers/streams and repeated installation, and observe
   the specific public session/projection each assertion owns.
4. **Reduce link count deliberately.** Trial compatible packaged-factory and
   transport behavior groups. Verify retained scenario counts/results and
   runtime overlap. Set an evidence-based binary-count target after a real
   covered Linux comparison; the local merge trial does not justify a
   guaranteed wall-time savings estimate.
5. **Tune cache and concurrency together.** Trial `-p=2,4,6,12` on the same
   four-CPU runner with fixed source and cache conditions. Consider per-binary
   parallelism as well as package count. Restore coverage-compatible project
   archives with scoped keys/size limits, and prebuild the runner tools once
   where useful. Do not return to the previous approximately 7GB cache without
   measuring restore/save cost.
6. **Reclassify static/support/OS work.** Keep customer behavior in functional
   tests, source-policy checks in lint, support-fake tests near their boundary,
   and small executable proofs in integration. Reconcile the inventory and
   coverage policies to those retained guarantees.

For an 80-second execution budget at 12 package slots, summed package time
must be below about 960 seconds even with ideal scheduling. Today's samples
need approximately **57–65% less aggregate package duration** to reach that
bound, plus better scheduling and a substantially cheaper build/harness path.

Accept completion only after at least five unchanged-head Linux samples of
the complete required non-short covered inventory, including project-cache
miss and representative restored-cache conditions. Record every sample and
failure/retry; require every complete job sample to stay below
180 seconds, no new quarantine, retained customer guarantees, and separate
race validation. Hardware changes, sharding, or moving coverage to another
required job must be stated explicitly rather than counted as reduced work.

## Reproduction and retained artifacts

All raw local evidence is under `.artifacts/latency-audit/` (ignored diagnostic
output), including CI downloads/job metadata, JSON events, Go traces,
per-tool records, profiles, and `measurement-summary.json`. Temporary merge
source was removed; production code and committed tests were not changed.

Representative commands, from the repository root:

```text
go test -p=12 -json -count=1 -timeout=5m ./tests/functional/...

go test -p=12 -json -count=1 -timeout=5m -run=^$ \
  -covermode=count \
  -coverpkg=github.com/portpowered/infinite-you/pkg/... \
  ./tests/functional/...

# Add -a to deliberately rebuild standard and third-party packages too.
# Add -debug-trace=<absolute-path> for Go 1.25 build-action diagnostics.
# The audit's tooltimer.go/measurement scripts retain exact tool arguments.

go test -json -count=1 -timeout=5m \
  -run=^TestModelsNamedBuiltinRouteUsesEffectiveDefinitionWithoutWorker$ \
  ./tests/functional/models/root_composition
```

For the cache comparison, set `INFINITE_YOU_OMNIVOICE_CACHE_DIR` only in each
child process's environment to a fresh owned directory; preserve normal
customer assertions. Python subprocess argument arrays were used to avoid
PowerShell's handling of certain `-flag=value` arguments. For an authoritative
Linux comparison, use the required CI supervisor and quarantine selection,
not the broader raw Windows wildcard above.

## Implementation and validation update

### Changes applied

Managed worker execution now always uses the joined Models operation contract.
The separate OmniVoice command runner, supervised `/invoke` adapter, local handle
cache, `InvokeLocal` Models operation and its request/result contracts, and their
wiring have been removed. Non-LocalAI supervised servers require an explicit
command; there is no implicit `omnivoice-llamacpp` fallback. Current LocalAI
operation, host, asset, output, cancellation, and scope coverage remains.
The old opt-in real OmniVoice functional sweep and legacy invocation and unary
retirement tests have been removed. Current model-reference resolution and
scope resource isolation unit tests were retained in `model_reference_test.go`.

Default functional API hosts now override an inherited model cache with a cache
inside their own temporary profile. Explicit fixture environments remain
authoritative. The model suite's profile environment selects its own
`.agent-factory/models` directory, matching the small assets seeded by its
fixtures. This prevents tests from traversing or hashing an operator's installed
models while retaining cache-selection and offline customer scenarios.

Three packaged invocation suites (`plan_parallel`, `fusion`, and `deep_research`)
now share `factory/packaged/invocation`. Their top-level journeys run in parallel,
preserving overlap that previously came from separate package binaries.
Model list/inspect/pull and invocation CLI tests now share `models/cli`.

The functional package names now describe behavior:

| Previous package | Current package |
| --- | --- |
| `models/root_composition` | `models/local_inference` |
| `factory_runtime/root_composition` | `factory_runtime/execution` |
| `operator_settings/root_composition` | `operator_settings/configuration` |
| `recordings/root_composition` | `recordings/lifecycle` |
| `work/root_composition` | `work/admission` |
| `sessions/root_composition` | `sessions/isolation_and_recovery` |
| `sessions/chat_sessions/root_composition` | `sessions/chat_sessions/acp` |

Active imports, CI selectors, Make targets, and reviewed scenario evidence paths
were updated. Generated Wire composition was regenerated. Constructor effect
count checks, Models contract-value checks, fixture self-tests, and direct
recording-service scope checks were removed from functional coverage.
Make/CI verification checks moved to `tests/tooling/verification`, outside the
customer functional lane. The functional README now states the customer-surface
rule explicitly. This cleanup is not a claim that every historical test in the
repository has been reviewed or that every use of a service observer has been
eliminated.

Two submit timeout cases no longer wait for an HTTP request that may never start
when initialization exhausts the caller's deadline. They still assert deadline
failure, join handlers that started, and prove subsequent submission works.
Shared submission boundary waits are bounded. The configured-server
withheld-header scenario now cancels upon an observed backend-start signal,
replacing a fixed 500ms delay. Its real HTTP interaction makes an explicit event
signal appropriate; virtual time is better reserved for isolated in-memory timer
behavior.

### Measurements after the changes

These are Windows diagnostics, not a new authoritative Linux CI baseline.

| Measurement | Result |
| --- | --- |
| Earlier warm covered build-only command | 49.197s; 145 link actions |
| First covered build after cleanup | 111.473s; pass; 636 compiler subprocesses |
| Warm covered build after cleanup | 39.014s; pass; 142 link actions |
| Final model local-inference suite | 27.038s; pass |
| Final submit suite | 23.229s; pass |
| Final merged model CLI suite | 20.181s; pass |
| Final merged packaged invocation suite | 27.241s; pass |

The warm build command used `-run=^$`, `-count=1`, `-p=12`, `-covermode=count`,
and the same whole-backend `-coverpkg` setting as the earlier local build audit.
The package cleanup removes **three link actions**, about 2.1% of the previous
145-action Windows graph. The warm command was about 20.7% shorter in this sample;
that difference is not a controlled CI improvement estimate.

The warm run recorded 140 actual linker subprocesses. Their summed child wall
time was 283.972s, user CPU 185.500s, and system CPU 122.453s. Two compiler
subprocesses consumed 1.560s summed wall time, and vet consumed 20.534s summed wall
time across 164 subprocesses. These sums overlap because work runs concurrently;
they must not be added to estimate the command's 39.014s wall time. Tool records
were filtered to this command's start timestamp because the timer directory also
contains records from the preceding build.

A broader non-covered functional run on an intermediate cleanup snapshot took
**635.099s and failed 24 of 143 package outcomes**. Seven packages reached the
two-minute package timeout. Other Go builds, lint jobs, and package discovery
were active on the same host. The run also included legacy model-invocation
tests and deadline assumptions removed or fixed in the final cleanup. It is
neither a passing final-suite result nor evidence of a speedup. The previous raw
Windows baseline also failed, so Linux CI remains necessary to distinguish host
contention, existing failures, and regressions.

Highest durations in that diagnostic run were `factory/process_time` (122.322s,
failed), `work/transports/cli/submit/batch_contract` (120.462s, failed),
`provider_sessions/details` (120.358s, failed), `factory/replay_contracts`
(120.182s, failed), `factory/packaged/named_invocation` (120.163s, failed),
`sessions/execution` (120.161s, failed), and `transport/mcp/stdio` (120.111s,
failed). Passing packages worth profiling next include `transport/cli/process`
(114.706s), `events/factory_events` (114.487s),
`factory/visualization/runtime_metrics` (109.432s), and `sessions/lifecycle`
(108.872s). The successful hosted CI rankings earlier in this audit remain the
better basis for prioritizing actual CI latency.

### Verification and remaining target

Models unit suites, current worker runner/interface tests, and canonical wiring
tests pass. Model-reference and resource-scope tests pass after separating them
from the retired executor suite. The affected CLI, local inference, submit,
packaged invocation, factory execution, work admission, settings, and recording
functional packages pass in focused fresh runs. The full covered functional
tree builds successfully. The union of `functionallong`, `backendconformance`,
`factoryartifact`, and `managed_process_integration` tags compiles for wiring,
model functional tests, and renamed session recovery tests. All 156 reviewed
functional scenario records validate. `git diff --check` passes.

The **under-three-minute total CI target remains unverified and unmet by the
available full-run evidence**. No passing hosted CI run of these changes has
been produced. Removing three links is a useful cleanup, not enough on its own
to remove the 165–241 seconds still needed from the earlier authoritative
344.601–421.149s coverage invocations. The next measurements should use the
four-CPU Linux runner, preserve the canonical coverage and quarantine population,
profile the highest customer journeys, and separately time supervisor and
quarantine overhead. Remaining default-session bootstrap, repeated graph
construction, and broad whole-backend instrumentation are the largest targets
identified by the original audit.

Raw update evidence is retained locally under `.artifacts/latency-audit/`,
including `after-summary.json`, build traces and tool records, full-run JSON
events, focused validation logs, and tagged compile results. The raw full run
is explicitly an intermediate snapshot; focused final validation logs supersede
its model and submit results.

## Hosted checkpoint and Linux follow-up

PR [#2867](https://github.com/portpowered/you-agent-factory/pull/2867), based on
main `9ca420195a1d2f3135334b293193fa92df29cf31`, first measured cleanup commit
`8a09e574de32b5ebd3792869e227d4e28cbbddc9` in hosted
[CI run 37283549677](https://github.com/portpowered/you-agent-factory/actions/runs/37283549677).
All **1,022 selected tests passed or intentionally skipped**: 1,020 passed,
two skipped, zero failed. The coverage test invocation took **385.076s**.
The supervised coverage child took **468.477s**, with concurrent quarantine
verification taking **59.263s**. The 83.401s difference between the coverage
child and test invocation remains material overhead. These measurements sit
inside the earlier observed 344.601–421.149s test-invocation range; they do not
establish a controlled improvement or a five-minute checkpoint.

Hosted coverage failed its package floor for `models/transports/http`:
440/798 statements (55.1378%) against 58.27%. Backend Lint also found stale
compiler-owner baseline entries after moves and seven newly unreachable
production functions left behind by the executor removal. Models wiring and
race verification passed. The five-, three-, and two-minute merge checkpoints
remain unachieved; this PR has not been merged.

For additional diagnostics, the same tracked source was archived into an
isolated Linux filesystem snapshot under WSL, running Go 1.25 with four-CPU
affinity, `GOMAXPROCS=4`, and the canonical 12-package budget. The populated
Linux module/build caches were retained; this is not a clean hosted runner.
The first instrumented run took **436.49s overall**, including **368.666s**
inside its coverage test invocation. Two top-level tests failed: a LocalAI
fixture asserted internal CPU-only platform facts on this CUDA-capable host,
and a packaged loop scenario failed but passed its focused rerun. Partial
coverage from a failed run is diagnostic only.

| Linux tool measurement | Invocations | User CPU | System CPU | Sum of child wall time | Union of active wall intervals |
| --- | ---: | ---: | ---: | ---: | ---: |
| Compiler | 1,637 | 240.364s | 37.980s | 918.352s | 200.731s |
| Linker | 147 | 266.927s | 40.231s | 1,084.364s | 314.304s |
| Vet | 1,247 | 49.254s | 13.853s | 249.964s | 88.565s |
| Coverage instrumentation | 658 | 2.604s | 1.808s | 22.850s | 16.201s |

These tool totals include discovery/helper compilation and overlap both one
another and execution. They cannot be summed to estimate elapsed time. The
whole command consumed 1,275.33s user CPU and 332.40s system CPU. The residual
includes customer commands, Go orchestration, profile processing, and reporting;
it must not all be attributed to tests without process-level profiling.

The highest package durations were `runtime_api` 73.083s,
`models/local_inference` 58.977s, `factory/review_failure_routing` 52.148s,
`transport/mcp/stdio` 48.697s, `factory/visualization/runtime_metrics` 43.190s,
`providers/acp` 41.383s, and `sessions/chat_sessions/acp` 40.036s. Linker child
wall times reached 15.376s for `transport/run_scoped_server`, 14.997s for
`transport/submit`, and 14.458s for `workers/transports/http`, while consuming
about 3.3–3.7 CPU seconds each. Much of that wall time is competition for the
four CPUs rather than unique package code size.

A follow-up snapshot, with warmer compilation caches, fresh test execution
(`-count=1`), one further CLI-package merge, cache isolation, and functional
vet disabled, took **253.92s overall / 231.050s test invocation**. It still
failed three top-level tests, so it is not a passing latency result. Its 145
linker invocations consumed 215.223s CPU; 304 compiler invocations consumed
55.729s CPU. Nine vet invocations consumed 0.138s CPU, showing that the
instrumented functional graph no longer repeated repository vet. The warmer
cache and several simultaneous changes prevent attributing the full elapsed
difference to any single optimization.

### Further changes being validated

- Removed the unused legacy runtime HTTP edge, six legacy catalog-host
  functions and their compatibility unit tests, and the retired worker
  working-directory helper. Renamed dead-code baseline paths without adding
  findings; removed vanished compiler-owner allowances.
- Combined CLI command and parameter journeys under `transport/cli/invocation`.
  Independent top-level tests overlap, and parameter runs use unique explicit
  sessions. Negative parameter cases assert their own public errors instead
  of counting another concurrent invocation's provider calls. The merged
  package passed Windows (5.952s), Linux (3.868s), and three fresh Windows
  repetitions (25.297s total).
- Moved Factory execution under `factory/execution` and visualization journeys
  under `factory/visualization/presentation`, matching the durable customer
  ownership required by the functional lane.
- Extended owned home/model-cache environments to shared review routing and
  common packaged-Factory/daemon helpers. Seeded assets retain the normal
  fixture-owned `.agent-factory/models` layout.
- Removed the functional PID-parser self-test and two packaged-loop fixture
  self-tests. Removed legacy cache-migration and provider-backed OmniVoice
  smoke cases; current LocalAI CLI, REST, recorded-event, and audio/file
  journeys remain. Added one shared-server table for actionable errors on
  malformed current model REST requests; its nine customer cases pass in
  1.592s locally.
- Removed internal model configuration assertions from the Factory inference
  scenario while retaining public results, errors, and canonical recordings.
  It passes on the CUDA-capable Linux host.
- Made recording cleanup use the selected process fact clock when an owned
  session supplies no clock. Linux session deletion had exposed a nil-clock
  panic; the existing fact-clock lifecycle test now checks this missing-clock
  case. The unit test passes.
- Corrected the unified event-log smoke's assumption that a continuous server
  must publish a completed live Factory state before closing. It retains the
  completed terminal run result, completed Work, canonical ordering, and
  identical live/recorded event payloads.
- Removed a fake-timer channel-release assertion after session close and
  post-close TCP probes against LocalAI fixture ports. A stopped timer need
  not fire, and another test may own a reused port. The public no-late-delivery,
  peer-isolation, model-result, and joined-close checks remain. Fresh Windows
  runs of process-time and local-inference packages pass in 35.151s and
  20.392s respectively; these are focused checks, not a full-lane result.

Required Backend Lint continues to own repository-wide vet. Bounded event and
cleanup waits have inline reasons; they are not replaced with shorter sleeps.
Full hosted verification of this follow-up, restored Models HTTP coverage, and
a passing supervised latency measurement are still required before merging.
Raw local evidence is under `.artifacts/latency-audit/linux-8a09/`,
`linux-next/`, `ci-functional/`, and the focused validation logs.

### Fresh execution after the follow-up fixes

The next four-CPU Linux run passed all **1,018 selected top-level tests**:
1,016 passed, two skipped, zero failed, across 145 packages. Its covered test
invocation took **202.455s (3m22.455s)**; the complete `gocoveragecheck` command
took **220.85s (3m40.85s)**. This diagnostic command excludes the outer
`functionaltestviz` supervisor and its separate quarantine validation. The
populated compilation caches were reused, while `-count=1` forced execution.
It is neither a cold CI measurement nor a verified hosted checkpoint.

The command consumed 655.76s user CPU and 139.92s system CPU. Instrumentation
recorded 144 linker invocations consuming **204.122s CPU**, 40 compiler
invocations consuming **15.308s CPU**, and nine vet invocations consuming
0.227s CPU. Linker active wall intervals covered 185.106s; summed child wall
time was 704.149s. Those overlapping intervals must not be added to elapsed
test time. The largest linker wall times were HTTP status (9.180s), CLI
session resume (8.898s), and docs (8.711s), each using about 1.5–1.6s CPU.
Reducing repeated application links remains material even with warm caches.

This run still failed the Models HTTP coverage floor: 405/798 statements,
50.7519%, against 58.27%. Current cloud-model REST discovery, invocation and
unsupported local-pull behavior now share one owned server. The malformed
request table also covers invalid text, media location, metadata and generic
JSON envelopes through actual REST calls. Together these focused tests pass
in 1.170s locally. A diagnostic union with the Linux profile reaches
465/798 statements (58.2707%); only a fresh complete covered run can confirm
the package floor. No coverage threshold was lowered.

The customer inference package is now `tests/functional/models/inference`,
because it covers both local and configured cloud models. Earlier measurements
retain the historical `models/local_inference` path. Workflow and scenario
manifest references follow the new path; all 156 reviewed scenario records
validate. Evidence for this run is under `.artifacts/latency-audit/linux-current/`.

The Linux snapshot contained the initial measured runtime plus subsequent
patches. It did not incorporate the intervening main-branch dead-code tooling
change; its snapshot commit identity is not the PR head identity. Hosted
verification of the exact rebased PR head remains the merge authority.

### Complete supervisor measurement and subsequent cleanup

An exact tracked-source archive of `ac9dc75bef` was measured with the same
four-CPU affinity, warm build/module caches, fresh execution and package budget.
The complete CI supervisor ran coverage/reporting concurrently with the real
quarantine validator. It took **255.84s (4m15.84s)** overall. Quarantine passed
in **22.294s**; the coverage child took **255.828s**, including **234.580s**
for its test invocation. It observed 146 packages and 1,021 top-level tests:
1,018 passed, two skipped and one failed. The Models HTTP floor was not evaluated
because execution failed. This is not a passing checkpoint. Setup-only attempts
that lacked required validator metadata or `jq` were terminated and excluded
from these results.

The failure was the specialized webhook-time journey's submission returning
HTTP 500. Three focused Linux repetitions of the complete two-cohort journey
passed in 15.283s total. The fixture observed secret resolution before submitting
Work; that effect precedes completed runtime startup. It now additionally awaits
the exact session's public `RUNNING` status and includes command/session identity
in failures. Verification of the updated fixture under full covered load is
still required. Evidence is under `.artifacts/latency-audit/linux-ac9dc/`.

Five further packaged-Factory suites—Quorum, Subagent, Ralph, Plan Execute and
Factory Builder—now share `factory/packaged/invocation`, saving five additional
test binaries. Each independently owned top-level cohort runs in parallel;
stateful sequences within a cohort retain their ordering. Their owned-home
environments also discard inherited model-cache overrides. The combined suite
passes locally in 14.491s and compiles with `functionallong`; a full-lane latency
result is needed to establish the saving.

Hosted lint on `b80e2127e6` identified remaining internal adapter construction,
operator-path policy calls, stale timing/construction baseline entries and the
missing maintenance-lane assignment for moved tooling tests. The recordings
journey now retains actual CLI recording files and REST artifacts without its
internal adapter/decoder checks. Operator settings retains CLI output, dispatched
model arguments and the persisted customer config file without constructor or
filesystem call-count assertions. Tooling tests have an explicit maintenance
lane. The dead-code baseline records the new test-support-only home-environment
helper, consistent with its existing helpers excluded from production reachability;
no new unreachable production code is accepted.

Model test names now describe customer outcomes: built-in CLI validation,
REST audio output, readiness errors, CLI output modes and unsupported pinned
backends. The TTS named/generic REST journey runs as its own test rather than
inside the embedding suite. The complete model package passes in 12.548s
locally; its scenario evidence references were updated and all 156 reviewed
records validate. Fixture source identities use `customer-inference` rather
than the retired `root-composition` label.

After the five-package merge, the complete Linux supervisor at `d087be567f`
took **237.00s (3m57s)**, including a **222.064s** test invocation across
141 packages. Quarantine passed in 21.766s. Of 1,021 top-level tests, 1,018
passed, two skipped and one failed. The webhook journey passed. The remaining
failure was admitted-Work cancellation/recovery: the fixture received the exact
dispatch-response event, then immediately read the Work projection while it
still held its initial state. The event is not a public read barrier. It now
awaits the exact Work's terminal state and output through REST before checking
correlation/isolation. Three fresh local repetitions pass in 11.214s total.
The package declaration is also corrected from `root_composition_test` to
`concurrency_test`. Full coverage gates were not evaluated on this failed run;
evidence is under `.artifacts/latency-audit/linux-d087/`.

A sequential four-CPU linker experiment compiled the fully covered HTTP status
test binary with ordinary debug data and with `-ldflags=-w`. The first ordinary
build warmed additional compiler entries and is excluded from comparison.
The subsequent ordinary build took 1.815s wall / 2.425s child CPU and produced
84,485,923 bytes. Two builds without DWARF took 1.435s/1.388s wall and
1.623s/1.553s child CPU, producing 68,593,982 bytes. These are complete warm
compile/link command measurements for one package, not a suite-wide forecast.
Functional coverage now omits debugger-only DWARF while retaining Go runtime
stack symbols/source coordinates and coverage counters. Other coverage lanes
retain their existing flags. A fresh full-lane measurement is required to
establish its actual elapsed saving.

The Provider Sessions root constructor/effect-count tests were subsequently
removed from the functional lane. They never called CLI/MCP/REST or checked
customer files; current transcript discovery, inspection, association and replay
journeys remain in their behavior subsections. Status HTTP journeys now share
the existing HTTP server package, with independent scenarios running in parallel.
The combined server suite passes in 4.274s locally. These changes remove two
more test binaries; their saving is not included in the earlier measurements.

### Passing full supervised run with reduced debug data

The complete four-CPU Linux supervisor for `630fb269c5` **passed**, including
tests, quarantine and existing coverage gates. It took **191.29s (3m11.29s)**
overall; the test invocation took **176.714s (2m56.714s)**. It observed all
141 expected packages and 1,022 top-level tests: 1,020 passed, two skipped,
zero failed. Aggregate coverage was 61.2% against the unchanged 33.1% minimum;
existing staged coverage holds remained in force. The previously deficient
Models HTTP floor passed. Quarantine passed in 19.845s.

The full command consumed 578.62s user CPU and 119.10s system CPU. Recorded
tools included 142 linker processes consuming **171.525s CPU**, 143 compiler
processes consuming **8.931s CPU**, and 27 vet processes consuming 0.242s CPU.
Linker active wall intervals covered 160.770s; summed child wall time was
607.187s. Different warm cache states and concurrent scenario fixes prevent
attributing the entire 45.71s elapsed reduction from the prior failed run to
DWARF removal alone. The passing measurement includes reporting and quarantine,
unlike the earlier 220.85s direct coverage command.

This establishes a passing local result below five minutes; the complete lane
is still **11.29s above three minutes**. Hosted CI of the pushed head remains
required before the first merge checkpoint. The two subsequent binary removals
are not included here. Evidence is under `.artifacts/latency-audit/linux-link-opt/`.

### Follow-up isolation and unsuccessful repeat

The next full four-CPU run, including the two binary removals, took **203.55s**
overall and 189.668s for the test invocation. It failed: 1,014 passed, two skipped
and three failed across 140 packages and 1,019 top-level tests. Coverage gates
were not evaluated. CPU consumption was 618.79s user and 131.39s system.
This result is not a passing latency checkpoint. Evidence is retained under
`.artifacts/latency-audit/linux-next-checkpoint/`.

The failures remain customer-visible promises: restarting an automation session
returned HTTP 504; a deleted packaged-loop session remained readable; and
CC-12 observed session event sequence 6 before 5. These assertions must remain.
Increasing polling timeouts or deleting these journeys would hide unresolved
lifecycle or ordering behavior and would not establish a reliable fast lane.

The preceding passing run identified two avoidable model stalls: the built-in
readiness/unknown-model case took 30.580s and custom CLI model discovery took
18.150s. Their asset HTTP clients were uncontrolled; the reusable CLI process
also used default model-cache resolution. Both now reject unavailable fixture
asset downloads immediately. The CLI process resolves its cache into its own
temporary home and the custom-model invocation uses the shared isolated-home
environment helper. Three Linux repetitions of the focused cases pass in
**1.154s total** for readiness and **0.226s total** for CLI discovery. These
focused, uncovered measurements identify unnecessary network/cache exposure;
they are not a full coverage-lane forecast.

Hosted lint at `630fb269c5` found two new model-test complexity violations.
The built-in route test now retains public audio and readiness responses while
removing internal launch/protocol/network call-count assertions. Cloud REST
discovery uses the existing model lookup helper, and its invocation response
assertions are separated from fixture setup. The retired
`ValidateLocalInvocationRequest` service-root lint allowance is also deleted;
its stale entry failed packaged-factory verification. Hosted validation remains
required after these corrections.

### Hosted result and single-binary consolidation experiment

Hosted run `37292866856` at `630fb269c5` passed functional coverage: 1,020
tests passed, two skipped and none failed. The test invocation took **388.339s
(6m28s)**, and its complete supervisor took **474.070s (7m54s)**. Diagnostics
observed 846 compiler commands and 138 linker commands. The dependency-cache
restore succeeded, but instrumented compilation still occurred. This hosted
result does not meet the five-minute checkpoint. The same workflow failed lint
and packaged-factory verification for the specific issues corrected above.

Following the request to evaluate monolithic test packages, a private build-only
experiment combines 709 Go source files from 133 customer-test package groups
into one package. Typed identifier references, including embedded struct fields,
are namespaced to prevent collisions, and embedded test assets are preserved.
The binary lists all **1,008 customer test functions**. Its original fixture
`TestMain` routines are dormant in the build-only variant; no successful customer
execution or coverage-gate result is claimed from that variant.

The controlled build comparison uses four CPUs, `GOMAXPROCS=4`, jobs 12,
`-vet=off`, `-ldflags=-w`, `-covermode=count`, and the exact canonical backend
coverage package list. The existing-layout command uses `-exec=/bin/true` so
that test bodies and fixture setup do not contaminate build/link measurements.

| Build-only configuration | Command wall | Compiler CPU | Linker CPU | Links |
| --- | ---: | ---: | ---: | ---: |
| Existing layout, warm compiler cache | 44.29s | 0s | 160.292s | 136 |
| Single customer binary, first compilation | 16.67s | 31.675s | 1.745s | 1 |
| Single customer binary, warm compiler cache | 3.95s | 0s | 1.807s | 1 |

The existing-layout measurement includes three fixture-support test binaries
and 12 fixture self-tests. These are already classified as maintenance by
`ForImportPath`, but functional discovery excluded only the support root and
accidentally included its descendants and the REST-client fixture package.
Discovery now consistently excludes `tests/functional/internal/`. The fixture
tests remain available in their maintenance packages. Customer coverage floors
still require a full-lane check after this classification correction.

These measurements strongly favor reducing the number of functional binaries.
They also show why summed linker CPU cannot simply be subtracted from total
elapsed time: the existing layout's 160.292s linker CPU occupies only 42.334s
of active elapsed intervals when test execution is absent. The first consolidated
compile costs about 31.7s CPU for the large test source package. The next step is
an execution experiment retaining every fixture's setup/cleanup and the original
scenario concurrency; a package move alone can accidentally serialize tests.
Raw evidence is under `.artifacts/latency-audit/linux-baseline-build/`,
`linux-monolith-count/`, `linux-monolith-count-warm/`, and `hosted-630/`.

The intervening local full run after model isolation also passed all coverage
gates, with 1,018 tests passed and two skipped, taking **258.10s overall** and
237.974s for tests. It recorded 139 links consuming 234.122s CPU and only 3.828s
compiler CPU. The spread from the preceding passing 191.29s run prevents treating
focused model-case savings as a guaranteed full-lane reduction.

The first private execution variant kept every original fixture's setup and
cleanup, but registered all tests directly in the single package. It failed:
931 passed, 74 failed and three skipped across all 1,008 customer functions.
Its command took 364.32s and its test execution took 348.313s. It is explicitly
excluded from passing latency evidence. Previously separate serial tests ran
in one long serial phase; relative workflow-fixture paths, evidence registry
references and self-executed test helper selectors also needed migration.

A second private variant organizes the original suites as parallel cohorts,
retains their declaration order, and preserves subprocess entrypoint selectors.
It gives the shared binary 48 parallel test slots, corresponding to the previous
12 packages times four slots per package on the four-CPU runner. Repository
fixture lookups and evidence declarations are adapted to the experimental
source paths, without suppressing their checks. After correcting fixture paths,
the complete command took **187.51s**, including **173.374s test execution**.
It consumed 538.82s user CPU and 103.62s system CPU. One linker consumed
**1.394s CPU** and 1.413s wall; compilation consumed 27.999s CPU. It failed
five suites, so this is not a passing latency or coverage checkpoint. Failures
include fixture-only listener probes, response-event visibility, session startup
and controlled ACP peer/version startup. Evidence is retained under
`.artifacts/latency-audit/linux-cohort-path-execution/`.

The platform ListenerStopObserver's synthetic socket/probe test and the HTTP
advanced-save-version matrix's fixture-probe classification subtests are now
removed from functional coverage. They test fixture/platform mechanics and add
OS port-reuse and scheduling sensitivity without another customer journey.
Platform component tests retain listener observation/cancellation/deadline
coverage. HTTP server tests pass in 3.644s, platform HTTP tests in 1.749s, and
the advanced-save-version REST matrix in 1.340s locally.

The aggregate experiment also exposed a failure-propagation bug in the selected
clock fixture: an activation callback using `t.Fatal` closed its completion
channel while returning no value, then the caller dereferenced a nil session.
The driver now distinguishes a returned result from an aborted callback and
fails the owning test immediately. Three focused repetitions pass in 14.361s.
No startup timeout is increased and the public HTTP failure remains visible.

A follow-up private execution variant starts only the owning fixture when a
subprocess helper selector is supplied, avoiding initialization of all 54
unrelated fixture lifecycles in each fake protocol peer. Execution validation
is pending. These experiments are private; the committed suite has not been
replaced with generated `TestSuite` names.

Hosted run `37296571427` at `f5b5dfcfc9` passed every required check, including
lint, unit coverage, Models wire/race, integration, conformance and functional
coverage. Functional tests took **376.528s** and the full supervisor took
**457.177s (7m37s)**; all 1,020 tests were observed, with 1,018 passed and two
skipped. The job including setup/reporting took 8m19s. No merge checkpoint is
claimed from this successful but above-target hosted run.

## Further consolidation and contention findings

The private child-fixture monolith variant took **222.21s** command wall time,
with **527.94s user CPU**, **125.56s system CPU**, and approximately **5.98 GiB**
peak resident memory. It failed five customer cohorts. The failures included
deadlines expiring before HTTP/CLI attachment, recovery startup, runtime
configuration execution and ACP startup. This result demonstrates that removing
linkers alone does not establish the customer execution target.

The committed layout migration moves 76 source files without combining them
into oversized files or introducing generated numeric suite names. Related
packages share a binary while each scenario retains owned inputs and effects.
Typed identifier rewriting changes only colliding helper names. Build-tagged
cases remain available, reviewed scenario evidence points at the new files,
and exact lint debt entries move with their original code rather than expanding
the allowed debt.

Two deadline fixtures now expire their caller context after observing an
outstanding request or attached Work watch stream. This proves handling of
DeadlineExceeded without racing startup against 100ms/250ms wall clocks.
Standard-library expired-deadline cases remain available separately. Three
focused repetitions pass for both the generated HTTP client and Work watch.

Internal effect-count assertions are removed from session-control and Work
submission journeys. The tests retain public pause/resume results, submitted
Work identities, REST listings and completed dispatch events. The dead TTS
`/invoke` protocol fixture and the named-Factory no-signature compatibility
matrix are removed; current signatures, positional/file/stdin inputs and
LocalAI audio/recording/replay scenarios retain behavioral coverage.

| Four-CPU consolidation measurement | First source-layout run | Follow-up run |
| --- | ---: | ---: |
| Supervisor wall | 277.48s | 259.21s |
| Link commands | 115 | 114 |
| Link CPU | 173.957s | 212.998s |
| Link active wall interval union | 192.849s | 222.519s |
| Compile commands | 1,586 | 6 |
| Compile CPU | 208.498s | 7.512s |
| User CPU, whole supervisor | 846.58s | 749.85s |
| System CPU, whole supervisor | 171.30s | 180.28s |
| Outcome | Coverage passed; quarantine invocation failed | One test failed; quarantine passed |

The first run supplied incorrectly named quarantine environment variables,
so its supervisor failure is an audit invocation error and its elapsed time
is coverage-only evidence. The corrected follow-up observed 115 packages and
1,006 top-level tests: 1,003 passed, two skipped and one failed. Its test command
took **233.532s**. The failure was an OS port-rebind probe in
`TestRunScopedServerUsesExactListenAddress`: another concurrent owner can bind
a released port before the probe. The test now retains the public CLI address
and completion checks through a controlled server edge and an owned completion
signal. Real socket policy remains platform/integration behavior. The legacy
ascending fallback compatibility scenario is removed. The complete run-scoped
server package passes three local repetitions in 19.573s.

Focused built-in lint, repository test-shape/boundary/sleep/debt analyzers, and
the 156-scenario manifest check pass. These checks do not replace required
hosted verification or establish any elapsed-time checkpoint.

### Latest complete consolidation run and subsequent cleanup

The next full four-CPU supervisor took **280.40s**, with **251.160s** for the
coverage test invocation. Quarantine validation passed. All 115 packages were
observed: 1,005 top-level tests, 1,001 passed, two skipped and two failed.
The run consumed 791.81s user CPU and 196.46s system CPU. Its 114 linker
commands consumed **214.241s CPU** with **224.601s** active wall interval union;
four compile commands consumed **3.503s CPU**. Failed tests prevent this run
from establishing a coverage or elapsed-time checkpoint.

The two failures were session Response Event history returning zero events
before a 25ms quiet timer, and the Loop fixture-cleanup matrix timing out while
checking its own lifecycle bookkeeping. The first now observes retained public
MESSAGE completion through the existing SSE fixture before asserting disjoint
session, event and dispatch identities. The customer isolation guarantee is
retained. The cleanup matrix, its action counters, release counters, timer-stop
counter wrapper and private stream probes are removed: its assertions concerned
fixture mechanics and duplicated the retained public Loop duration validation,
scheduled Work admission and overlap suppression cases. Ordinary scenario
cleanup still closes sessions, routes and processes.

The retained FSCP-03 live isolation and packaged Loop cases passed three focused
repetitions (8.722s and 9.685s respectively). Focused built-in lint, repository
boundary/shape/sleep/debt analyzers and all 156 reviewed scenarios also pass.
Hosted CI is required on the current pushed source before a merge checkpoint
can be claimed. The audit keeps all complete failed measurements alongside the
passing coverage-only and earlier complete passing measurements.


### CLI fixture packages consolidated into customer command journeys

The six packages for documentation, shell completion, submit, CLI output,
process behavior and invocation help now live under
`tests/functional/transport/cli/customer_commands`. Six named customer journey
parents run independently in parallel. Each former TestMain setup belongs to
its journey parent; parent cleanup runs after all parallel child scenarios
finish. The layout uses ordinary Go test scheduling, without a global lifecycle
shim. Existing scenario names remain visible as named subtests.

| Covered build-only measurement, four CPUs | Six original packages | One customer-command package |
| --- | ---: | ---: |
| Link commands | 6 | 1 |
| Link user + system CPU | 7.803s | 2.403s |
| Summed linker wall | 12.988s | 3.551s |
| Active linker wall interval union | 2.507s | 3.551s |
| Whole build command wall | 4.82s | 7.64s |

These warm dependency-cache runs use `-exec=/bin/true`, full backend coverage
instrumentation and the same four-CPU affinity. They measure binary construction,
not scenario execution. Link CPU falls 69%, but this small batch's elapsed build
sample increases; filesystem and scheduling costs remain material. Summed CPU
savings must not be presented as CI elapsed savings. Raw tool records are in
`linux-cli-fixtures-baseline` and `linux-cli-fixtures-warm` under the audit artifacts.

The initial covered Linux execution of the merged package passed in 5.764s;
its whole command took 20.37s including compilation and linking. The full merged
Windows package passes three consecutive repetitions in 76.185s. Earlier
repetition attempts exposed package-lazy Sync.Once and process references
surviving cleanup: the owned output, help and process fixtures now reset those
references before each journey setup. Those failed attempts are retained as
validation findings rather than counted as passing measurements. The ordinary
and functionallong source variants compile, focused lint and repository custom
analyzers pass, and all 156 reviewed scenario records validate. The complete four-CPU coverage supervisor subsequently passed in **316.66s**,
including **297.001s** for the test invocation: 111 packages, 966 top-level tests,
964 passed, two skipped and no failures. Moving former top-level CLI scenarios
under six journey parents lowers the top-level count without deleting those
scenarios. All coverage gates and quarantine validation passed. Compilation
consumed 183.423s CPU across 781 commands; linking consumed 225.519s CPU across
109 commands, with 248.486s active linker wall. Whole supervisor CPU was 937.12s
user and 230.50s system. This dependency-cache-populating run exceeds five minutes;
a warm rerun is recorded separately. Raw evidence is in linux-cli-cohorts-verified.

The preceding hosted commit 032f9aa889 completed functional coverage successfully
in **448.588s supervisor wall** and **366.218s test invocation wall**, with 115
packages, 1,002 top-level passes, two skips and no failures. Backend Lint failed:
nine existing timer/sleep allowances moved with consolidated source and were
rejected by the deletion-only baseline growth rule; a removed listener-probe
allowance was stale; three new test-only controlled-deadline helper entries were
absent from the deadcode inventory. The stale allowance and helper inventory are
corrected. The relocation gate still needs resolution before required checks
can pass. No elapsed-time or merge checkpoint is established.

A fresh-checkout measurement before the passing local run stopped before coverage
because the audit repository had no Git HEAD, which the diagnostic collector
requires. Its 102.34s invocation is retained under linux-cli-cohorts-full and is
excluded from test-runtime comparisons.


### Warm CLI consolidation follow-up and relocation lint correction

The warm full supervisor took **227.51s**, including **212.589s** for tests,
with 111 packages, 963 top-level passes, two skips and one failure. Its 109 links
consumed **183.089s CPU** and **199.934s active wall**; there were no compiler
commands. Whole supervisor CPU was 683.76s user and 158.51s system. The failure
was `TestAutomationsSelectedTimeControlsWorkAndJoinedShutdown`: public watcher
Factory Session activation returned HTTP 504 while the controlled scheduler
advanced readiness polls. Coverage floors were consequently not evaluated.
The earlier dependency-cache-populating complete run passed; this failed warm
run cannot establish a checkpoint. Raw evidence is in linux-cli-cohorts-warm.

The lint relocation correction now uses Git-detected test-file renames to map
existing testsleep allowances to the destination file and its declared package.
The function identity, finding kind and occurrence number stay exact. Other
rules remain deletion-only. Unit tests verify that added timer occurrences,
changed function identities, copies without rename evidence and unreadable
renamed sources cannot pass through this mapping. One recovery helper retains
its original name; no current symbol collision requires renaming it. Focused
baseline-growth tests, custom analyzers, built-in lint and the affected Work
recovery scenarios pass. Hosted required checks remain necessary.


### Broader customer journey consolidation

Four workflow fixture suites first converged into customer-workflows journeys,
with parent-owned setup and cleanup. Three partial-start fixture-analysis tests
and their failure-injection helpers were removed. The retained workflow suite
passed three repetitions in 29.099s. Automation readiness now acknowledges each
registered readiness timer without advancing the entire scheduler; three focused
repetitions passed in 16.905s. This prevents filesystem initialization under CPU
contention from expiring the session-open deadline through artificial time advance.

A larger consolidation moves 28 suites into
`tests/functional/product/customer_journeys`, removing 27 more binaries. It keeps
original customer test names except 13 names clarified to describe REST, CLI,
ACP and workflow behavior. Typed helper renames avoid package symbol collisions;
scenario-owned sessions, profiles and cleanup stay independent. Two suites remain
separate because they share existing timer-debt identities requiring further
cleanup. The package remains under the approved Product functional domain and
contains public calls, events, Work results and file outputs.

The first measured larger layout ran all 939 top-level passes with two skips:
87 packages, 941 top-level tests, **234.695s test wall**, **286.00s supervisor**.
It consumed 843.37s user CPU and 174.57s system CPU. Its 82 links consumed
**135.550s CPU** with **161.728s active linker wall**; 1,631 compile commands
consumed **233.542s CPU**. The supervisor failed the Live Change package floor
(198/349 statements, 56.7335%, against 57.77%) after the direct internal
coordinator test was removed. Other unmeasured-package notes concern contract-only
packages and are diagnostics, not the blocking regression. Raw evidence is in
linux-meta-full. This failed supervisor is not a checkpoint.

The next warm measurement uses the functional lane budget for in-package
parallel tests as well as package builds. It took **216.264s test wall** and
**229.10s supervisor wall**: 87 packages, 938 top-level passes, two skips and one
failure. Its 80 links consumed **142.683s CPU** with **175.095s active wall**;
four compile commands consumed **10.831s CPU**. Whole supervisor CPU was 661.79s
user and 158.72s system. The remaining failure was the legacy config-alignment
sweep; coverage floors were not evaluated. Raw evidence is in linux-meta-current.

The removed internal coordinator test is replaced by a public REST resource
capacity journey that verifies owned session/resource identities, accepted
changes, stable replay decisions, request-ID conflicts, stale revisions, missing
resources and subsequent valid changes. Three repetitions pass in 2.533s. The
previously missing setFactorySessionResourceCapacity surface now has reviewed
functional evidence; the total inventory still contains 156 reviewed surfaces.

Durability resume now waits for canonical SESSION_COMPLETED through the public
Factory Event history. The response endpoint retains the interrupted attempt;
it cannot provide resumed completion. Private snapshot filesystem notifications
were racing actual successful REST state on Windows. The snapshot-file-only
case and its entire unused fixture are removed, along with cross-project private
snapshot manipulation. Public restart recovery, checkpoint identity, final
results and cross-project REST isolation remain covered, with three focused
repetitions passing in 18.823s.

The legacy config-alignment sweep and its three files are removed. They combine
private flatten/readback analysis, five retired-alias rejection cells, cron
startup, script retry timing, resource accounting and internal topology analysis.
The current customer execution guarantees remain in focused workflow, Work,
worker, resource-concurrency, automation and API suites. The new full measurement
rechecks package floors after this cleanup; floors are not lowered to accommodate
it. The coverage runner unit suite, focused built-in lint, custom analyzers and
ordinary/functionallong compile checks pass.

Recent pushes have no hosted checks because PR #2867 now conflicts with live
main. The next source checkpoint must rebase onto the current main and receive
fresh hosted verification. Local passing or failed measurements before that
rebase do not establish hosted checkpoint latency.


The cleanup follow-up, linux-meta-verified, took **213.292s test wall** and
**225.97s supervisor wall**: 87 packages, 940 top-level tests, 936 passed, two
skipped and two failed. Its 79 links consumed **130.353s CPU** with **163.365s
active linker wall**; four compile commands consumed **8.850s CPU**. Whole
supervisor CPU was 648.73s user and 152.52s system. Failures were a shared review
fixture's session-open request returning HTTP 504 under contention, and a mock
ACP test's temporary directory receiving writes during cleanup. Neither is an
elapsed-time checkpoint, and coverage floors were not evaluated. Live main has
since changed process/session lifecycle code; rebase and current-source
verification precede further fixes rather than patching the superseded lifecycle.

## Rebase, full measurement, and public replay synchronization

The working branch was rebased onto main `c038071b04` after an actual merge
conflict prevented PR CI from starting. Main's new cancellation and gateway
customer tests were retained at their relocated package paths. Wire output was
regenerated from the merged construction source. The complete functional source
inventory compiles, and the scenario projection validates 159 reviewed surfaces.

Hosted run [37317442555](https://github.com/portpowered/you-agent-factory/actions/runs/37317442555)
at `849480ac49` has 88 selected packages, 946 top-level tests, 944 passes,
two skips and zero test failures. Its test invocation took **344.020s**;
the full supervisor took **419.113s** (13:35:30.551–13:42:29.664 UTC).
The subsequent coverage gate failed: Live Change covered 198/349 statements,
56.7335%, below its unchanged 57.77% floor. This is **not a passing checkpoint**.
A historical cleanup error appears in the separate flake evidence even though
the current timing inventory records zero test failures; it remains a cleanup
ownership concern rather than evidence that the complete verification passed.

The four-CPU local measurements on this base were:

| Sample | Full supervisor | Test invocation | Links | Link CPU | Active link intervals | Compile CPU | Result |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Rebased source, changed coverage graph | 276.26s | 209.354s | 82 | 116.168s | 156.748s | 288.360s | Two Response Event replay tests failed |
| First retained-count helper, warm graph with changed tests | 206.93s | 194.221s | 80 | 117.860s | 148.960s | 48.577s | Three customer tests failed |

The first helper implementation incorrectly excluded STREAM_GAP from the
advertised count. The advertised catch-up prefix includes that marker; it now
consumes every advertised frame. The complete Response Event package passes
three repetitions in 34.327s. The failed full sample above remains recorded;
its other failures were the selected-process-time journey and the selected
Work watch reconnect/cancellation journey. They are retained customer behaviors.

The underlying original helper ended history collection after 25ms of silence.
CPU contention can insert a longer gap between retained SSE frames, yielding
an empty or partial history. The replacement observes the public
`X-Factory-Session-Retained-Response-Event-Count` header and reads exactly that
prefix. It removes the idle timer, separate scanner goroutine, and their timer
baseline allowance. It does not lengthen the quiet period or weaken replay
assertions.

Hosted CI also found the docs-smoke Make target still pointing to the retired
Docs package. It now selects the documentation subtest in the consolidated CLI
parent and passes locally. Main intentionally changed baseline policy to permit
count-neutral replacements; the relocation unit now tests actual added debt
while retaining the exact file/function relocation check. Focused baseline units
pass. A TTY output assertion was extracted to reduce the migrated CLI helper's
cyclomatic complexity; focused built-in lint reports zero issues.

A source/reference audit found Live Change's old `New`, `Apply`, and `Recover`
adapters have no production callers. They existed only to construct the
component in unit tests and in the removed internal functional characterization.
Those compatibility adapters are removed; the canonical wire coordinator and
its `ApplyLiveChange`/`RecoverLiveChange` operations remain. Component tests now
construct their own state and exercise the private policy implementation;
they pass. The REST resource-capacity journey no longer requests MockWorkers.
Coverage floors have not been lowered. A new complete run is required to assess
this source and all gates together.

The corrected retained-count follow-up took **196.82s for the full local
supervisor** and **179.743s for the test invocation**. It executed 946 top-level
cases in 88 packages: 943 passed, two skipped, and one failed. The remaining
failure is `TestProcessTimeJourneys/TestSpecializedSourceTimeJourney`: public
session close can race the background CLI's final activation binding, producing
“activated session ... is unavailable” during daemon cleanup. Response Event
replay and selected Work watch passed in this sample. All 80 link commands
consumed **110.065 CPU-seconds** across **137.614s of active linker intervals**;
121 compile commands consumed **62.865 CPU-seconds**. Whole-run user/system
CPU was 579.51s/117.47s. This source-changed warm-build sample remains failed;
coverage floors were not evaluated after the test failure. It proves neither
a three-minute complete lane nor a hosted checkpoint.

The next consolidation candidates are the fixture-owning packages whose
`TestMain` can become a named parallel parent with cleanup after its children.
The existing fixture variables must reset between repetitions, and assets and
subprocess cases require separate handling. The private one-binary experiment's
99% link-CPU reduction remains strong evidence for this direction, but passing
customer execution and complete hosted gates remain the release criteria.


## Immutable startup work and duplicate scenario execution

Two successive hosted samples used the customer-lifecycle consolidation:

| Revision | Full functional supervisor | Coverage invocation | Compile commands | Link commands | Final outcome |
| --- | ---: | ---: | ---: | ---: | --- |
| `e120ac9028` | 398.946s | 326.594s | 694 | 62 | 825 passes, two skips |
| `0958ddb01f` | 307.456s | 248.406s | 696 | 63 | 827 passes, two skips |

These are different hosted samples, not a controlled before/after benchmark.
The second revision restores MCP protocol's separate binary after combined
fixtures exposed initialization failures. Both diagnostics report compilation
work. The first run's Verification Policy failed because Backend Lint was
canceled by the subsequent push. The second run's required jobs all passed,
although its raw diagnostics retain recovered MCP stdio and mock-worker failures.
Neither full supervisor reaches the five-minute merge checkpoint.

At `0958ddb01f`, the slowest packages were:

| Functional package | Elapsed time |
| --- | ---: |
| Product customer journeys | 157.472s |
| Product customer lifecycles | 127.960s |
| Packaged Factory invocation | 85.694s |
| Factory execution | 77.547s |
| Models inference | 71.569s |
| Chat Sessions ACP | 51.193s |
| CLI customer commands | 45.360s |
| Providers ACP | 44.816s |

Package durations overlap and cannot be added to obtain CI elapsed time. The
supervisor also includes coverage tooling and concurrent quarantine validation.

Product customer journeys was profiled with complete production `-coverpkg`,
count coverage, `-ldflags=-w`, four pinned CPUs, `GOMAXPROCS=4` and parallelism
12. Each sample executed fresh tests once and passed. Profile CPU measures the
test process; command wall also includes the package build. These are individual
observations, not repeated statistical estimates.

| Sample | Command wall | Profile duration | Sampled test CPU |
| --- | ---: | ---: | ---: |
| Original startup behavior | 42.967s | 39.930s | 67.880s |
| Cache validated embedded publication | 38.192s | 30.690s | 48.790s |
| Also cache verified managed publication identity | 33.219s | 26.570s | 38.520s |
| Also deduplicate helpers and parallelize parents | 28.588s | 21.630s | 38.430s |

The initial profile attributed 36.46 CPU-seconds to system initialization,
22.44 to process construction and 30.12 to generated Factory JSON decoding.
These are overlapping call-tree totals. Repeated validation and installation
compete with test execution, in addition to the previously measured linker cost.

The embedded publication cannot change during a process. Its validated catalog
now initializes once; accessors still return detached payloads and formats.
Injected filesystems still validate every call. The existing test also verifies
that caller mutations cannot poison a second load and compares publication names
with the exact generated manifest, replacing a stale nineteen-factory list.
Three repetitions of focused catalog checks passed in 0.475s.

Each installer retains only a successfully verified publication fingerprint,
keyed by installation root, Factory name, root filename and payload SHA-256.
Customer files and management stamps remain freshly read on every reconciliation.
Changed input misses the cache; changed customer files retain the repair/backup
path. Prepared layouts stay operation-owned because persistence may mutate them.
A new installer computes its own fingerprints. Three repetitions of existing
reconciliation tests passed in 0.545s, including adoption, malformed evidence,
source refresh, customer modifications, contention and failures.

Eleven helpers had `Test...CaseN` names and ran directly as well as through their
parents' `t.Run`. Renaming them to private `run...CaseN` helpers preserves every
scenario assertion under the six customer-behavior parents. The parents have
separate process/home fixtures and now run in parallel without shared environment
mutation. Three repeated native workflow runs passed in 20.849s. The final
profile has a shorter critical path, with nearly unchanged sampled CPU compared
with the preceding sample; no further CPU reduction is claimed from that sample.

Hosted evidence: [e120 run](https://github.com/portpowered/you-agent-factory/actions/runs/37325476800),
[0958 run](https://github.com/portpowered/you-agent-factory/actions/runs/37327031568).
The cache and duplicate-run changes require complete-lane and hosted validation
before establishing a new checkpoint.


### Complete four-core validation of the cache and duplicate-run changes

The full supervisor passed in **192.74s overall / 143.595s for the coverage
invocation**, with 816 passes, two skips and no failures. All existing coverage
floors and quarantine inventory checks passed. The source snapshot uses the
final fifteen-suite layout with MCP protocol separate. Its 818 top-level cases
are exactly eleven fewer than the hosted 829: only the duplicate standalone
`CaseN` executions disappear; their original parent scenarios remain.

The complete command consumed 581.06 user CPU-seconds and 129.61 system
CPU-seconds. Compiler instrumentation recorded 1,559 compilations consuming
224.163 CPU-seconds and 113.921s of active wall time; 69 links consumed
90.324 CPU-seconds and 109.670s of active wall time. These tool intervals overlap
one another and test execution. There were also 563 vet invocations in the
supervisor/tooling paths (17.720 CPU-seconds), even though the functional
coverage invocation itself uses `-vet=off`. Product customer journeys passed
in 80.669s, customer lifecycles in 71.758s, packaged invocation in 33.986s and
Factory execution in 28.609s under this lane's contention.

This measurement rebuilt dependencies in a fresh source directory. It is not
a controlled warm comparison with previous samples, and it does not establish
a hosted checkpoint. It does provide complete passing verification with the
unchanged gates, rather than extrapolating the isolated profile to CI.


### Further customer-proof cleanup

Two `readWorkAtState` fake-HTTP self-tests are removed because they exercise
test helpers rather than the application. Removal of a legacy metadata
conversion/clone roundtrip and programmatic metadata-snapshot rejection was
also measured, but is not shipped yet: that removal uncovered two existing
functional package floors that require equivalent customer-boundary proof. The real filesystem watcher scenarios and their public Work observers
remain. These tests have no reviewed scenario declarations. They are not removed
because of a runtime failure.

The six retry scenarios previously in `tests/functional/workflow` move into
Product customer journeys, removing one more application-linked binary. Each
has its own process, home, Factory fixture and controlled provider. They run in
parallel. Success observes correlated terminal Work directly rather than an
unconditional quiet window; rejection/exhaustion still inspect terminal Work
and public dispatch events. The long cases retain their `functionallong` tag.
Three repeated focused runs, including the long retry cases, passed in 8.114s.
Three obsolete missing-subsection exemptions for the vacated workflow sources
are removed without adding new debt allowances. Complete coverage validation
of these changes is running separately from the preceding source snapshot.


The first full cleanup follow-up took **153.91s overall / 143.303s tests** and
failed: 811 passes, two skips and one failure. The recording-path reuse case
in Workers inference inherited the operator home, so packaged installation
contended on `/home/andre/.you-agent-factory/factories` with other processes.
This is an isolation failure, not a reason to remove the recording behavior.
Both invocations now use one test-owned home and the existing isolated model
cache environment. The test still reuses the same process and recording path
and proves new Worker/recording identities and unmerged durable history.
Three focused repetitions pass; a full isolated follow-up is required.

The failed run observed 65 links / 104.767 linker CPU-seconds and two compiles /
5.760 compiler CPU-seconds. Its warmer cache differs from the preceding fresh
source rebuild; a shorter failed run establishes no latency checkpoint.


### Hosted validation of immutable caches and duplicate-run removal

Revision `02a491c07b` passes the hosted functional job and unchanged coverage
gates: **333.016s full supervisor / 257.763s coverage invocation**, 816 final
passes, two skips and no final failures. It records 696 compiler commands and
63 linker commands. Providers' timeout-recovery case and JavaScript mock-worker
case failed initially and passed retry; raw diagnostics retain both failures.
Product customer journeys takes 151.687s, customer lifecycles 143.794s, packaged
invocation 81.356s, Factory execution 59.471s and CLI customer commands 54.272s.

This is slower overall than the preceding hosted 307.420s supervisor despite
the isolated CPU improvement. The profiles establish avoided computation, not
a hosted latency checkpoint. Cache state, contention and retries prevent treating
single samples as a controlled comparison. The build-only one-link result remains
a strong reason to continue consolidation, while complete execution and unchanged
gates are required before shipping a monolith.

Exact supervisor timestamps are retained in each run's
`c09-critical-path/critical-path-timing.txt`. The earlier 0958 value of 307.456s
in this document spans the whole workflow step including its shell preamble;
the supervisor itself is 307.420s. The e120 supervisor is 398.914s (398.946s for
the workflow step). Future checkpoint comparisons use supervisor start/end.
[Hosted run 37331029622](https://github.com/portpowered/you-agent-factory/actions/runs/37331029622).

The isolated-home cleanup repeat takes **139.92s overall / 126.732s tests**
and still fails one existing Worker Session observation case (811 passes, two
skips). Its recording fake represents missing history as a generic error, while
the real reader returns typed `os.ErrNotExist`, which the application recognizes
as absent live history. The fake now preserves that error contract. Five focused
repetitions of selected-session observations, recording reuse and opening-gate
scenarios pass in 12.776s. The inference environment helper also adopts the
shared isolated-home helper so an operator's explicit model cache cannot leak in.
The failed run records 65 links / 95.130 linker CPU-seconds and two compiles /
1.373 compiler CPU-seconds. Its shorter failed duration establishes no checkpoint.


The corrected follow-up has **812 passes, two skips and no test failures**, with
**133.49s overall / 122.177s tests**, but fails two unchanged coverage floors:
NameValue metadata validation drops from the required 72.22% to 29.63%, and
Factory snapshot mapping from 75.00% to 68.75%. These gates are not weakened.
The metadata-only tests remain temporarily pending replacement with appropriate
public behavior; the two polling-helper self-tests can be removed independently.
This failed run's 65 links consume 87.740 CPU-seconds, with 103.300s of active
wall time; two compiles consume 0.999 CPU-seconds. A shorter invocation with a
failed gate establishes no checkpoint. Recording reuse and selected-session
observation now pass in this complete run with owned homes and typed fake
missing-history behavior.


### Rebase onto current main

The branch is rebased onto `c54a1263c6` after main added selected Worker captured
activity reads and required the selected packaged-installation logger. The
canonical Wire graph is regenerated from current providers to preserve those
new roles alongside legacy model removal. Eight new Worker HTTP fixture files
move with their existing suite into Product customer journeys, with unchanged
customer assertions and helper ownership. Their package declaration and scenario
references follow the new location. Main's captured-log evidence supersedes the
older read-surface declaration where appropriate. No new binary is introduced
for these additions. Verification and full-lane measurements from before this
rebase remain labeled by their source revisions; they do not verify this new
combined source snapshot.


The first complete four-core rebased snapshot takes **198.81s overall /
151.518s tests** and fails: 819 passes, two skips, one existing Work-watch
startup failure (`SERVER_START_FAILED` before host readiness). The consolidated
customer-journey package, including main's newly moved captured-log cases,
passes, as does Workers inference with the recording corrections. The isolated
captured-log selection also passes natively in 20.896s. The rebased manifest
checks all 160 reviewed scenarios. This full run rebuilds 1,571 compiler actions
(225.939 CPU-seconds, 117.583s active wall) and 68 links (93.624 CPU-seconds,
110.612s active wall). The unrelated readiness failure is retained and prevents
using this sample as a checkpoint. A fresh execution with warm build caches
follows to verify the complete combined source and coverage gates.


### Rebased warm lane and readiness correction

The warm repeat on the current-main rebase took 137.03 seconds for the full
supervisor and 126.450 seconds for the coverage invocation. It recorded 819
passing tests, two skipped tests and one failure: the same Work-watch customer
journey failed when public `session create` returned HTTP 504. There were no
compiler actions; 65 linker actions consumed 88.443 summed CPU seconds over
98.412 seconds of overlapping active intervals. This failed sample is not a
latency checkpoint.

The selected-clock fixture advanced its entire process clock by 10 milliseconds
for each runtime-readiness poll. Under contention, repeated polls could consume
the independent one-second startup deadline before the runtime goroutine made
progress. Readiness now receives a fixture-owned timer wakeup without advancing
that clock. Reconnect timers and the public 99/100 millisecond boundary assertions
remain controlled by explicit clock advancement. Host initialization also uses
the common isolated customer-home/model-cache environment. Five focused native
runs passed in 31.634 seconds. A complete lane measures the correction separately.


The corrected complete sample took 155.12 seconds overall and 144.144 seconds
for the coverage invocation. Work-watch passed in 13.655 seconds under full-lane
load. The sample still failed: 818 tests passed, two skipped and two failed.
Runtime-metrics cancellation reported that closed-session history changed after
clock advancement and peer completion. Mock-worker cleanup reported a nonempty
temporary directory during removal. Both raw failures are retained. The 65 linker
actions consumed 102.978 summed CPU seconds and 109.305 overlapping active
seconds; two compiler actions consumed 0.549 CPU seconds. This sample does not
establish a successful sub-three-minute lane.

The refreshed private monolith experiment includes 63 package groups, 741 Go
files and 823 Test-prefixed declarations. It retains package fixtures, embedded
assets, source mappings and evidence declarations. Unlike the earlier build-only
transform, executable variants retain package initialization functions.


### Current-main one-binary execution experiment

The refreshed four-core experiment built and executed one binary in 145.28
seconds. Its single linker action took 2.186 wall seconds and 2.142 CPU seconds;
two compiler actions consumed 33.707 CPU seconds. Package execution took 128.713
seconds. This establishes the linker reduction, not a passing CI checkpoint:
the mock JavaScript ACP test failed, as did an invocation-scoped cancellation
case. Parent cohort failures are additional JSON events for those same failures,
not independent failed customer scenarios. Peak RSS was 6,325,684 KiB, compared
with 1,362,000 KiB in the preceding native-package lane. A single binary therefore
also concentrates memory and fixture concurrency; its end-to-end benefit is much
smaller than its isolated linker benefit in this sample.

The mock JavaScript ACP fixture unnecessarily changed the process-wide working
directory while running in parallel. Its command now supplies an absolute Factory
path and an explicit command working directory. Removing the global mutation
alone initially exposed the relative Factory-path dependency; making the path
absolute preserved the customer invocation. Five focused native repetitions
passed in 3.729 seconds, and changed watch/mock scopes passed built-in and tagged
repository lint. The private monolith is repeated with this correction before
considering any shipping layout.


The repeat with an absolute mock-worker Factory path passed: 133.03 seconds
command wall and 116.653 seconds package execution, with no failed JSON events.
The one linker consumed 1.609 wall seconds and 1.585 CPU seconds; two compilers
consumed 32.799 CPU seconds. Whole-command CPU was 288.91 user plus 83.21 system
seconds. Peak RSS was 6,584,844 KiB (6.28 GiB). The recorded profile was checked
using the repository's own canonicalization, coverage evaluation and blocking
manifest functions through a private audit-only harness: 61.4% total exceeds the
unchanged 33.1% total threshold, and all active package gates pass with the
existing 0.25-point epsilon and existing main holds. No baseline was edited.
The private harness and monolith are not committed production tests. This
measurement excludes the full functional supervisor and hosted CI environment;
it proves feasibility, not completion of a latency checkpoint.


### Shipping CLI and REST fixture consolidation

Nine fixture-owning packages join `product/cli_rest_journeys`: Provider Session
CLI diagnosis, Models catalog/invocation CLI, Factory YAML parity, Factory
validation/persistence, named Factory lifecycle, Session CLI controls, Work CLI
collections, Factory Events and REST server behavior. Forty-three existing source
files retain their customer assertions beneath named parallel scenario parents.
Each parent constructs and cleans up its own former package fixture; cleanup
runs after parallel children. Repetition resets fixture-owned state. This removes
eight native package links without shipping the private monolith fixture harness.
Two complete native repetitions passed in 58.536 seconds before the additional
cleanup and 62.728 seconds after it. Built-in and tagged repository lint pass.
All 160 reviewed scenarios retain their decisions; evidence declarations move
to executable scenario parents, and existing lint-debt identities follow their
source moves without adding allowances.

The concurrent REST identity proof now performs one overlapping observation
per owned session rather than ten repeated rounds. Distinct identity and
cancellation observations remain. Direct filesystem implementation checks and
durable-appender component failure probes are removed from the functional
webhook case. Their behavior is a component concern; customer webhook delivery,
recorded history, signing, filtering, retries and dead-letter artifacts remain.
Public Work Request observation uses a bounded ticker instead of a fixed 50 ms
sleep. Existing cleanup and failure observation ceilings are unchanged.

Two attempted full measurements in the reused private Linux checkout were
invalid because functional discovery also found the completed experimental
monolith sources. Removing those sources from Git's index did not exclude them
from the repository's filesystem discovery. The experimental directory was moved
outside the functional source tree, and a clean discovery now selects 69
packages. Neither incomplete attempt counts as latency or coverage evidence.


The first platform-complete native consolidation sample passed all 755 selected
tests with two skips: 133.92 seconds full supervisor / 124.764 seconds coverage
invocation. It recorded 57 linker actions, 79.944 CPU seconds and 102.292 seconds
of overlapping link-active intervals; four compiler actions consumed 3.200 CPU
seconds. Whole-command CPU was 380.20 user plus 104.81 system seconds, and peak
RSS 1,359,684 KiB. It failed the unchanged filesystem floor: 31/47 statements
(65.9574%) versus 75%. This is a failed gate sample, not a latency checkpoint.
The largest executing packages were Product customer journeys (91.346s), Product
customer lifecycles (81.527s), packaged invocation (47.874s), the new CLI/REST
journeys (43.430s), Factory execution (39.372s), and Models inference (29.054s).
These package elapsed intervals overlap and cannot be added to supervisor wall.

A Linux compile check also caught a non-Windows memory observation helper that
the Windows typed source load had omitted. Both platform variants now move with
the Provider Session CLI fixture. The early compile-failed sample did not check
coverage; no latency conclusion is drawn from its short termination.

To retain the filesystem floor through customer behavior, the existing webhook
append-failure scenario now uses an actual blocked destination directory through
the exact appender edge. Two Linux cases add an exhausted file and unavailable
durable flush. Each case submits Work through the CLI, observes signed delivery
and retained canonical history through public interfaces, verifies one terminal
append attempt at the external-effect boundary, and proves an independent peer
continues delivering. The dead-letter record is still checked for canonical body,
identity, retry classification and redaction. Device fixtures are confined to
scenario-owned symlinks, and non-Linux execution skips those two device cases.
Three Windows repetitions passed in 33.541s; three four-core Linux repetitions
passed in 7.860s. Built-in and tagged repository lint report zero issues.


The customer file-failure replacement passes the complete four-core canonical
lane: **130.64s full supervisor / 120.533s coverage invocation**, 755 top-level
passes, two skips and no failures. The original total and package coverage gates
and all quarantine checks pass. Total coverage is 61.4%; filesystem coverage is
36/47 statements (76.6%), above its unchanged 75% floor. The recorded tool trace
has 57 linker actions consuming 76.129 summed CPU seconds over 97.962 seconds of
overlapping active intervals. Two compiler actions consume 2.880 CPU seconds.
Whole-command CPU is 366.93 user plus 103.21 system seconds; peak RSS is
1,422,344 KiB (1.36 GiB). This avoids the experimental all-in-one candidate's
6.28 GiB peak while retaining measured link reduction. The failed predecessor
was 133.92s; these are individual samples, not a statistical speedup estimate.

The largest package intervals in the passing sample are Product customer journeys
83.254s, Product customer lifecycles 74.332s, CLI/REST journeys 50.061s, packaged
invocation 45.905s, Factory execution 36.648s and Models inference 28.192s. They
identify the next fixture/compute targets; they overlap and are not additive.
Thirteen resolved sleep/deadline debt records are removed: the fixed sleep no
longer exists, and existing signal-driven failure/cleanup ceilings now explain
their purpose in code. No coverage floor or debt allowance is increased.


### Hosted current-main rebase measurement

The hosted functional job for `79cb69b1e7` succeeds: 332.773 seconds full
supervisor (16:40:14.102Z through 16:45:46.875Z), 258.435 seconds coverage
invocation, 820 final passes, two skips and no failures. It still exceeds five
minutes. Required Backend Lint fails on the now-empty `tests/functional/workflow`
package stub after its customer cases moved to Product customer journeys. The
obsolete `doc.go` stub is removed rather than adding a shape exception. The
new nine-suite consolidation and customer file-failure cases are the next hosted
candidate. No checkpoint merge is claimed from this revision.

### Consolidated API smoke selector correction

Hosted run `37344044590` finds three Make targets still selecting the retired
HTTP server package. The API contract job fails before running the customer
case (`no Go files`), so this is a package-routing failure rather than a failed
REST assertion. `api-smoke`, the focused HTTP baseline, and the related contract
target now select `product/cli_rest_journeys` and the exact nested
`TestRESTServerJourneys/TestGeneratedClientAndServerSchemaStayAligned` case.
The dollar anchor is escaped for Make. Native Go JSON confirms that the moved
case and its fixture-owning parent run and pass (1.550s package elapsed).
Scenario projection defaults now reference the executable CLI/REST parents;
the reviewed 160-decision manifest remains unchanged. The scenario package's
unit tests pass in 0.164s.

### Memory-limited monolith experiments

The retired private single-binary sources are temporarily restored under their
original functional path for measurement, then moved back outside discovery.
An initial attempt to run them under `.artifacts/` fails immediately because Go
correctly forbids importing the functional internal REST client from there.
It supplies no latency evidence. The first correctly located `GOMEMLIMIT=3GiB`
sample takes 130.68s command wall and peaks at 3,463,308 KiB (3.30 GiB), but
fails evidence-declaration checks because the native lane had restored its
current registry while these older generated sources require their original
exact nested aliases. This failed sample cannot establish behavior equivalence
or a checkpoint. A repeat installs the corresponding source revision's registry
and exact mapped aliases only for the private run, restoring the current native
registry in cleanup. The memory limit is a Go GC target, not a hard RSS ceiling.

The corrected registry repeat passes: **139.04s command wall / 134.011s package
execution**, one link at **2.390s wall / 1.973 CPU seconds**, and peak RSS
3,529,376 KiB (3.37 GiB). Whole-command CPU is 343.42 user plus 90.46 system
seconds. No compiler action is recorded, so this is a warm-source-build sample;
it must not be compared as a cold build with the earlier 133.03s experiment.
The repository's actual profile evaluator passes unchanged total and active
package gates at 61.4% total coverage. There are no failed Go JSON events;
2,254 passing events include nested cases and wrappers and are not a count of
distinct original top-level tests. The registry and retired source location
are restored after execution. This still excludes the canonical supervisor and
hosted runner.

Increasing the same memory-limited binary to `-parallel=48` fails and is slower:
**148.62s command wall / 143.450s package execution**, peak RSS 3,318,448 KiB
(3.16 GiB), one link at 2.415s wall / 2.028 CPU seconds, whole-command CPU
369.79 user plus 96.82 system seconds. The selected Worker Session MCP read
exceeds its deadline, and two JavaScript ACP prompt cases receive request
cancellation. Eight failed JSON events include affected parent wrappers.
This concurrency setting is not a passing optimization and is not adopted.

### Hosted nine-suite consolidation result

Hosted run `37344044590` (`d9c1aa259f`) passes the complete functional supervisor
in **320.437s** (17:05:45.416Z through 17:11:05.853Z), with **250.280s** coverage
invocation, 755 final passes, two skips and no failures. Quarantine succeeds in
56.975s while overlapping the coverage child. This sample is below the preceding
332.773s result but still exceeds the five-minute checkpoint by 20.437s. The
separate API package job fails on the retired Make selector described above;
revision `38d312b762` carries its correction. The three-minute and two-minute
checkpoints remain unmet. No merge is claimed.

### Fleet pagination fixture cleanup

The CLI/REST fleet continuation proof now uses six Workers across three owned
Factory Sessions, with a four-row first page and two-row continuation. Each
session still contributes both a COMPLETED and FAILED Worker Session. Ordered
identity, attribution, filtered selection, malformed-token behavior, empty
selection, public session visibility and CLI/REST parity assertions remain.
The case documents its serial execution because the root fleet projection
includes parallel siblings' rows. A duplicate identical terminal fleet read
is removed; the active-to-terminal ordering proof remains.

Process working-set/commit/hostname observations and response-stopwatch logs
were performance diagnostics rather than customer assertions. They are removed,
including the Windows and unsupported-platform memory helper files. Performance
measurements stay in the audit harness. The focused pagination/concurrent-fleet
pair passes three repetitions in 22.420s, and union-tag repository lint reports
zero issues. These changes do not establish a full-lane latency result.

### First hosted five-minute timing observation and merge gate

Run `37346055096` (`38d312b762`) passes the full functional supervisor in
**260.106s** (17:26:37.668Z through 17:30:57.774Z), with a **202.865s** coverage
invocation, 755 final passes and two skips. Quarantine succeeds in 47.869s while
overlapping coverage. The API package check also succeeds with the corrected
Make selector. This is the first passing hosted supervisor below five minutes.
Other required jobs on this revision are cancelled by a subsequent push, so
this observation does not establish a merged checkpoint. Earlier passing
320.437s and 332.773s observations remain part of the measured range.

Run `37347513698` (`91b288cfd4`) fails required Backend Lint on three generated
fixture resets that copy structs containing a Mutex or Once. It also fails the
required functional verdict: the reduced pagination fixture accidentally made
an active-fleet sibling's default-page-size assertion expect four instead of
the public default of twenty. Its recorded supervisor interval is 212.793s and
invocation is 166.920s, but the final summary retains two failed tests, so this
is not a passing latency checkpoint even though the supervisor status files
record zero. The required verdict, not a short elapsed interval, controls merge.

Local corrective commit `676f7a1e05` initializes fresh lock-bearing fixture state
directly from composite literals rather than copying another value, and makes
the empty-page assertion accept the selected expected limit. The continuation
case still checks its explicit four-row limit; the active case retains the
default twenty-row contract. Three repetitions of all three fleet cases pass
in 33.442s, and tagged `go vet` passes. The user subsequently authorized the
corrective push needed to unblock merging, and commit `676f7a1e05` was published.
The merge request is enabled, but GitHub
reports `BLOCKED` on the current published head's required checks. Further
optimization remains separate from the minimal merge correction.

### Package-preserving consolidated build prototype

A private overlay experiment reuses the unit lane's package/bridge construction
to combine 35 compatible functional packages (426 original top-level tests)
into one binary, while explicitly leaving 19 custom-TestMain packages native.
Production exports and original source packages are unchanged. The initial
warm-dependency build succeeds in 9.37s, with 24.28 user plus 3.28 system CPU
seconds and 835,476 KiB peak RSS (0.80 GiB). Subsequent coordinator corrections
relink from cached package archives. This prototype is not shipped and has not
executed its 19 native exceptions or the canonical supervisor.

The first execution panics because parallel coordinator ancestors prohibit
existing process-wide `t.Setenv` calls; 412 expected tests remain unexecuted.
It supplies no complete latency evidence. A repeat preserves serial barriers
for the four active source packages with global environment/directory mutation:
bootstrap portability, coverage observability, JavaScript worker scenarios and
script workers. All 426 expected tests then run, but execution fails in 96.73s
(96.045s package interval), with 43 failed JSON events including wrappers and
peak RSS 3,180,728 KiB (3.03 GiB). Existing subprocess fixtures require their
original test selectors, and the fleet default-limit mistake is also exposed.

A repeat routes unique legacy helper selectors through their exact coordinator
group and includes the fleet assertion correction. All 426 expected tests run,
but four original ACP scenario families still fail (ten failed JSON events
including wrappers): terminal output, permission selection, and new/config RPC
failures. It takes 98.99s command wall / 98.283s package interval, with 228.93
user plus 75.18 system CPU seconds and 3,180,512 KiB peak RSS. These failed,
partial-lane observations demonstrate build feasibility and remaining fixture
coupling; they do not establish equivalent coverage or a CI checkpoint. Source
revision registry declarations receive exact temporary coordinator aliases only
during each private run and are restored in cleanup. No tests are silently
dropped, no guard is increased, and no native exception is reported as executed.

### Corrected-head hosted retry and runner acquisition

Run `37366971390`, attempt 1, ended with numerous cancelled required checks.
The Docs Reference check annotation states: "The job was not acquired by Runner
of type hosted even after multiple attempts". Its job had no runner or steps;
this cancellation does not measure test execution. The failed-job rerun was
requested without another source push, and attempt 2 acquired runners.

On the same corrected head `676f7a1e05`, attempt 2's functional check passed.
The supervisor ran from 22:12:13.417Z to 22:15:53.834Z on 2026-10-05:
**220.417s total wall time**, including concurrent quarantine and coverage work.
Quarantine took 42.440s and overlapped the coverage lane. The functional timing
summary records **169.582s invocation wall time**, 68 expected/observed package
entries, 757 top-level results, 755 passes, two skips, and zero final failures.
The workflow flake ledger records one `TestPackagedLoop` failure that passed on
same-head retry; the failure was an eight-second scheduled-execution readiness
guard. Retain that retry when interpreting this observation and prioritizing
future synchronization cleanup.

This successful full functional interval meets five minutes but exceeds the
three-minute target. At evidence collection, backend lint was still running,
and the PR was open with auto-merge enabled. A timing observation alone does
not establish a merged checkpoint.

Attempt 2 ultimately passed every applicable check, including Backend Lint
(16m8s job duration) and Verification Policy. PR #2867 entered the merge queue
at 22:30:23Z behind PR #2868. A read-only merge-tree comparison against that
queued commit exposed a conflict confined to generated `pkg/wire/wire_gen.go`.
The authored source graph merged cleanly. A separate checkout merged the queued
commit, regenerated Wire, and passed `go test ./pkg/root ./pkg/wire -short
-count=1` (root 0.034s, Wire 25.878s). PR #2868 then merged as
`8284dec71ffaf510c389d3fbb38e16018255c5ae` at 22:39:17Z, and GitHub classified
our PR as conflicting and removed it from the queue.

Merge correction `6759f55ae3a1f6b82e297b59268e3422abc02b22` merges that live main
commit and regenerates Wire. Its generated file matches the tested preview
byte-for-byte; there are no unresolved paths and `git diff --cached --check`
passes. The correction was pushed under the user's authorization to unblock
merging. New-head run `37383931843` started, and auto-merge was enabled again.
The additional optimization prototypes and this unpublished audit were excluded
from the correction.

The corrected-head functional check in run `37383931843` passed, but was slower:
supervisor 22:41:36.403Z to 22:47:17.253Z, **340.850s total wall**, with quarantine
57.969s overlapping coverage. The invocation summary reports **268.630s wall**,
69 expected/observed package entries, 758 top-level results, 756 passes, two
skips, and zero final failures. It exceeds both five- and three-minute targets;
do not carry the previous head's faster interval forward as this head's result.
The slowest package execution intervals are customer journeys 160.694s,
CLI/REST journeys 92.031s, packaged invocation 88.159s, factory execution 69.831s,
Models inference 54.934s, and CLI customer commands 53.550s. These intervals
overlap and are not independent amounts to add to total wall time. At collection,
lint and the packaged-factory candidate build were still active, with no failed
checks. The current priority remains completing the authorized merge before
publishing more optimization changes.

Merge-queue run `37385807288` tests candidate
`0dcdbf1fde04e573730e19c4b39fe097e7389d8c`. Git tree IDs confirm its source tree
is identical to corrected PR head `6759f55ae3`. Its functional check passes in
**272.327s supervisor wall** (23:00:05.530Z to 23:04:37.857Z), with **215.102s
invocation wall**, 69/69 expected package entries, 758 results, 756 passes, two
skips, and zero final failures. Quarantine takes 46.590s and overlaps coverage.
The same source tree thus has successful 340.850s and 272.327s observations;
the 68.523s difference is not evidence of an intervening source optimization.
Both observations and their retry work must remain visible.

The corrected-head run's flake ledger records one review-failure-recovery case
that passes on same-head retry. The queue run records three recovered cases:
resume recovery retaining a scenario TCP port, MCP initialization exceeding its
five-second response guard, and an MCP ACP permission case failing to resolve
`@you/subagent` from a project/global factory catalog. Each passes on same-head
retry. These are concrete remaining synchronization, teardown, and isolation
targets; a green retried verdict does not imply the first attempt was clean.
At collection, all other queue checks had passed and Backend Lint remained
active. The queue observation meets five minutes, but not three minutes, and
the PR had not yet merged.

The queue run ultimately completed successfully with no failed checks. GitHub
merged PR #2867 at **2026-10-05 23:11:00Z**, as commit
`0dcdbf1fde04e573730e19c4b39fe097e7389d8c`. This establishes the first merged
five-minute checkpoint using the queue's 272.327s full supervisor observation,
with the recovered flakes and slower identical-tree observation disclosed above.
The three- and two-minute checkpoints remain outstanding. The checkout now
starts the next optimization branch from that live main commit, retaining this
audit locally; no additional optimization changes were pushed to unblock merge.

### Next checkpoint: compatible build consolidation

The canonical full Linux supervisor on four pinned CPUs passes in **111.55s**
(297.68s user + 83.06s system, 2.01 GiB peak RSS). Coverage invocation is
103.120s: 69/69 original package entries, 758 selected tests, 756 passes, two
skips, no failures and no retries. The unchanged blocking policy retains its
existing staged coverage holds. No floor, hold or quarantine selector is edited.
The build trace records zero compiler commands and 37 linker commands: this
repeat uses a warm compiler cache. Phase times are list 1.706s, plan 0.893s,
test including diagnostics 107.048s, canonicalization 0.168s, evaluation 0.014s
and manifest processing 0.022s. These phases are sequential, while linking and
test execution overlap inside the test phase.

The overlay merges 19 compatible packages containing 171 original tests.
Thirty-six packages retain native binaries for custom TestMain, process-global
state, helper executables or relative/embedded fixtures. Production boundaries
and source files remain unchanged. Native exceptions execute within the same
bounded lane; test-level quarantine selection remains native. Normalized JSON
preserves original test identities, nested names, source locations, failure
capture and evidence declarations. Missing terminal events and registration
mismatches fail the lane. A same-head retry uses the original native package.

Failed development samples are not checkpoints: 130.00s had a provider timeout
recovery observation race and coordinator inventory bookkeeping; 110.14s passed
all customer cases but failed raw-capture completeness. Both failures are fixed
and covered by runner checks. Batch admission now waits for the exact accepted
Work projection; timeout recovery observes successful Work and verifies public
timeout/recovery events rather than assuming retry admission means completion.
Obsolete captured-log migration checks and direct internal publisher privacy
checks are removed; public CLI/REST/replay and live privacy cases remain.

The asynchronous quarantine ratchet previously occupied 30.824s of planning in
the merged-main local baseline. With overlap enabled it now runs beside coverage
and is joined before a passing verdict; current planning takes 0.893s. The
baseline's 208.424s full supervisor failed a batch Work visibility race, so it
is disclosed as a failed reference rather than a passing comparison. The earlier
private 427-test prototype was partial and cannot establish a full checkpoint.
Hosted confirmation and merge remain required for the three-minute milestone.

### Hosted cold-build gap and bounded compiler archives

Run `37390711081` at `0878fd26d0` passes in 316.816s supervisor wall, with
287.224s invocation wall, 1,355 compiler/73 linker trace commands and one
recovered MCP ACP permission case. Run `37391871366` after merging main
(`daf8e9afc6`) passes in 316.715s supervisor / 291.821s invocation, with
679 compiler/37 linker trace commands. Neither meets three minutes. The existing
67 MB dependency-cache hit does not retain compiled repository packages.

The new compiler-archive tier retains recently used Go archives and action
metadata only, with a hard 1 GiB bound. It excludes executable/test-result
payloads and cached executable directories, avoiding the former whole build
cache's 7 GB/four-minute restore. Keys include platform, Go version, module
inputs and source head, with a same-module fallback; Go checks source and tool
identities before accepting each artifact. Functional runs explicitly use
`-count=1`. The transfer reports retained bytes/files and omitted entries, and
the existing build diagnostic reports exact/fallback action-cache identity.

A real coverage-instrumented Go build proof restores only these artifacts into
an empty cache: the repeat performs zero compiler invocations and produces the
original output; changing source recompiles the affected packages and produces
the changed output. Filtering and byte-budget checks also pass, and this proof
now runs in Workflow Lint. Hosted cold-fill, restored-cache timing, transfer
cost and retained size still need measurement; a first cold fill is not claimed
as a three-minute checkpoint.

The private dispatch-concurrency experiment passes at 113.65s versus 111.55s
for the retained candidate, so its increase from two to four is not adopted.
It records 40 links across tests/tools, 61.181s linker CPU, 57.764s overlapping
active linker wall and 186.654s summed linker wall. Largest links are Product
customer journeys (8.829s), Product CLI/REST journeys (8.210s), Providers ACP
(7.311s), Runtime API Factory transformation (7.094s), and Providers (7.030s).
Their intervals overlap and cannot be added to supervisor time. Removing two
Git-file diagnostic helpers privately allows 21 merged groups/320 tests, but
that full sample takes 115.06s and raises peak RSS to 3.86 GiB; it is not adopted
as a latency improvement.

### First hosted archive fill

Run `37393044364`, head `6efdbac547`, has no archive hit. The supervisor takes
296.357s, including 265.724s of test invocation; all 69 package terminals and
759 test results finish (757 pass, two skip, one recovered permission-request
flake). The job fails afterward because the newly wired exact-hit diagnostic
receives an empty GitHub output on a cache miss. The correction explicitly
defaults that output to `false`; test and coverage gates remain unchanged.

Capture retains 12,129 files / 964,300,415 bytes with zero budget omissions.
Capture takes three seconds and cache save takes five seconds. Restored hosted
execution is still required before claiming the three-minute checkpoint.

A separate private archive-restore calibration takes 203.06s with 2,038
compiler tool records and 40 link records. It uses an older private source tree,
the unadopted 21-group experiment, and a capture-time recency window rather
than the workflow's job-start window. It cannot establish current-head latency
or cache effectiveness. Its 96.818s active compiler wall / 318.784s compiler
CPU demonstrates substantial misses, which require investigation if repeated
by the hosted restored run.

The coverage-specific cache proof reproduces one avoidable compile after an
archive-only restore: Go requires its separately cached static coverage
metadata (`\x00cvm`) even when the compiled archive is present. The filter now
retains that metadata too. Repeating fresh `go test -count=1 -coverpkg=./...`
against an empty restored cache performs zero compiles and produces an identical
coverage profile. All four cache proofs pass. Executable, result and counter
payloads remain excluded, and the same total byte limit applies.

Quarantine outcome and package-terminal invocations now use `-vet=off`, as
inventory discovery already does. The required canonical Backend Lint lane
retains vet ownership. This removes duplicate static analysis and regeneration
of vet-only cached source side files without changing runtime selection,
terminal-event evidence, retries, or quarantine expectations. Focused quarantine
and overlap tests pass.

### Corrected metadata fill and local restored execution

Hosted run `37393987887`, source `0e5bd429e7`, passes without retries:
202.617s full supervisor, 193.405s invocation, 467 compiler and 37 linker
trace commands. The earlier archive-only snapshot restores in four seconds
and applies in fifteen seconds (220 MB compressed / 964,300,415 bytes retained).
The corrected capture contains 13,707 files / 969,143,201 bytes, with no budget
omissions; capture and save each take four seconds. This is still above three
minutes and includes rebuilding missing static metadata.

The exact-source four-CPU local restored supervisor passes in 108.938s,
including 102.291s invocation, all 69 package terminals and 759 test results
(757 pass, two skip), without retries. Restoring into an empty cache performs
zero Go compiler invocations. There are 42 test/tool link records: 48.467s
linker CPU, 53.744s active linker wall, and 159.370s summed overlapping link
wall. Peak RSS is 2.22 GiB. The capture reaches the 1 GiB cap and omits 115
older entries; all selected customer cases and unchanged coverage gates pass.
This local result supports the corrected cache mechanism, but hosted reuse
including transfer cost still needs measurement.

The first exact-source local fill sample takes 150.247s and fails the existing
`TestPackagedFullFlowDispatchTerminalFinalization/RoutesNoResultFailureToLead`
case because its immediate public Work list is empty. It remains a failed
sample, not checkpoint evidence. An earlier local harness attempt has no Git
repository and fails before tests while reading the raw-failure artifact head;
that harness setup is corrected before the two full exact-source samples.

### Hosted restored result and complete-job budget

Run `37394846636`, head `e671ed2e38`, passes the full functional supervisor in
140.882s, including 132.624s invocation, zero compiler commands and 37 linker
commands. There are no retries. Archive download takes four seconds and the
second archive copy takes eight seconds. The entire functional job takes
213s (00:45:08–00:48:41 UTC), so this is not yet a three-minute total-job
checkpoint. Checkout takes 15s, Go setup 13s, other preparation and reporting
account for the remainder. These costs are included rather than hiding them
behind a passing supervisor-only measurement.

The next workflow uses a shallow checkout and the restored archive directory
directly as GOCACHE. A cold directory is seeded from the existing dependency
tier. Capture still filters archives, static coverage metadata and action
metadata into a fresh directory capped at 1 GiB before saving; executable and
test-result payloads remain excluded. A real coverage-test proof also executes
against the snapshot in place, with zero compiles and the same coverage profile.
Its external marker confirms all three cold/restored/in-place tests actually
run. Twelve workflow contract checks and actionlint pass.

Backend Lint on the hosted head rejects two monolith complexity violations.
Inventory validation is separated from construction and event identity/boundary
tests are split; the focused tests and scoped built-in lint now pass with no
allowance changes. Diagnostics also distinguish link-only work from compilation:
the earlier JSON correctly reports zero compilers but labels those 37 links
`compile-work-observed`; new diagnostics use `link-work-observed` for that case.

A private cleanup candidate relocates eight internal Make/coverage-runner checks
to their tool package, uses a scenario-owned project Factory for MCP ACP
permissions, observes terminal Work through REST, and copies current captured
journals for recovery fixtures instead of constructing legacy snapshots. Its
first warm control passes in 111.92s with 68 packages/751 results. Raising test
parallelism to 24 passes in 164.19s but needs four retries; a subsequent unchanged
12-way control passes in 162.52s without retries. This sequence does not establish
a parallelism improvement and the increase is not adopted. Linker CPU also
drifts between samples, so none is used to predict a hosted checkpoint.

### In-place archive restore: hosted job still exceeds three minutes

Head `4492b4921d451cf335eff925b5aaeff63deb516a`, Actions run
`37397370508`, functional job `112056602110`, passed with all coverage gates
unchanged. The complete job took **221 seconds** (01:06:30–01:10:11 UTC).
The supervisor took **171.948 seconds** and the coverage invocation reported
**157.896 seconds**. This is not the three-minute checkpoint.

Shallow checkout took six seconds, Go setup nine seconds, archive restore four
seconds, and applying the in-place archive cache took less than one second.
The coverage trace reported zero compiler commands and 73 linker command
observations from the coverage invocation, including its failed attempt. Retry stderr is not included in these diagnostics; they do not measure retry link work. These trace
counts are not linker CPU seconds. Capture and save each took four seconds.

One recovered failure was `TestMCPSubagentCustomACPHandlesPermissionRequest`:
its named `@you/subagent` Factory was absent from both the project and the
runner's global catalog. The next fixture creates its own project Factory
through the public application process before starting MCP. Three focused
repetitions passed; the previous failure is retained in this measurement.

### Customer-facing test cleanup entering the checkpoint candidate

The next candidate moves eight Make/coverage-rendering checks from functional
`observability/coverage` into `cmd/functionaltestviz`, retaining their assertions
in tooling unit tests. Five Automations files change their misleading
`root_composition` names to `customer_journeys`, without changing registered
customer tests. Worker-session recovery fixtures now copy the current writer's
journal output rather than constructing legacy snapshots. Public CLI/REST
parity, restart, incomplete follow, unreadable recovery, damaged-recording
isolation, and oversized-payload checks remain. The packaged full-flow failure
journey observes terminal public Work records before asserting cardinality and
lineage, replacing the race with an immediately read projection.

Private Linux full-lane verification of these edits passed with 68 packages,
751 test results (749 pass, two skip), and unchanged coverage policy. Twelve-way
execution samples ranged from **111.92 to 162.52 seconds**, with no retries;
a 24-way experiment took 164.19 seconds and recovered four failures, so it is
not adopted. These are local supervisor samples, not hosted whole-job claims.
Focused repetitions passed for MCP permission (three), current-journal recovery
(three), full-flow terminal observation (five), and tooling checks (three).


The archive restore now precedes the smaller dependency tier: a restored archive
cache is used in place, so restoring another directory that the test job never
reads is redundant. Cold misses retain dependency seeding; main pushes retain
the independent dependency-cache maintenance step. This saves the three-second
redundant restore observed in run 37397370508 without skipping fresh tests.

A private cold-cache quarantine compilation experiment is rejected. Native
verification passed in 162.14 seconds; adding the full coverage flags to its
registration/outcome commands also passed but took 247.79 seconds. Compiler
counts fell from 1530 to 1324, while compiler CPU rose from 194.254 to 241.088
seconds, coverage-tool calls rose from 553 to 1157, and linker CPU rose from
48.476 to 87.494 seconds. Aggregate job user/system time rose from 474.77/92.29
to 705.96/175.34 seconds. Host variance and a brief overlapping scoped lint run
limit wall-time attribution, but the experiment does not justify adopting
coverage-instrumented quarantine checks. Both retained the same coverage gates.


### Owned fixtures: changed-source hosted measurement

Head `0e227f2485c866234b2441cbac369e1bb4535c26`, run `37398418294`,
functional job `112060950738`, passed all 68 packages and 751 results
(749 pass, two skip) with no recovered failures. The complete job took
**231 seconds** (01:22:24–01:26:15 UTC); the supervisor took **186.484 seconds**,
and coverage invocation **175.850 seconds**. The restored archive came from
`4492b4921d`; changed test sources required 23 compiler commands and 36 linker
command observations. This is another valid above-target sample, not the
three-minute checkpoint. The isolated MCP Factory fixture resolved the prior
failure in this run.

On exact archive hits, the next workflow skips filtering a snapshot that cannot
be saved again under GitHub's immutable cache key. Fresh tests still execute;
cold or fallback hits still capture and save the bounded archive. This removes
otherwise redundant post-test copying on same-head verification.


### Parallel execution of isolated customer scenarios

Nineteen previously serial top-level scenarios now call `t.Parallel`: ten
initialization scenarios, three JSON/YAML failure journeys, three CLI startup
journeys, and three JavaScript restart/resume journeys. Each owns its temporary
home/project, process, buffers, and external-effect mocks. Steps inside each
restart/recovery scenario remain sequential; no process-wide environment or
working directory is mutated. Registered test names and assertions are retained.

The first sixteen-scenario full-lane sample passed in **117.96 seconds**, with
751 results and a **55.980-second** customer-journeys package. Adding the three
JavaScript scenarios passed in **121.92 seconds**, with **115.960 seconds**
coverage invocation and a **54.948-second** customer-journeys package. Its fresh
tool trace recorded 39 links, **54.524 seconds linker CPU**, **57.593 seconds
active linker wall intervals**, and **159.173 seconds summed linker wall**.
Summed overlapping wall intervals are not elapsed job time. Aggregate job
user/system times were 329.91/77.72 seconds. Three focused JavaScript repetitions
also passed. Existing private twelve-way cleanup samples ranged from 111.92 to
162.52 seconds, so this experiment establishes a lower package serial tail,
not a consistent whole-lane speedup. Hosted whole-job measurement is still needed.

A preliminary private edit accidentally added `t.Parallel` to a test that already
called it; that full run failed and is excluded from successful latency evidence.
An interrupted repeat and a stale overlay lock also produced no valid latency
sample. The duplicate call was removed before the successful full-lane checks.
The final timing directory uses fresh tool records; earlier repeated harness
output directories contained nested stale report copies and are not used for
compiler/link totals.

For timer-only asynchronous components, `testing/synctest` remains appropriate.
Real file, pipe, and network I/O—including loopback HTTP—is not durably blocking
inside a synctest bubble. Accordingly, these public REST/MCP recovery scenarios
use observable completion and controlled clocks/edges rather than shortening
wall-clock sleeps or wrapping real sockets in virtual time. See the Go team's
[Testing Time](https://go.dev/blog/testing-time) explanation of these limits.


### Hosted parallel candidate and loop-duration race

Head `bd5bf2728139c9d4e438d1df3de8a0c1ecb251c2`, run `37399514886`,
functional job `112064602388`, passed in **223 seconds whole job**
(01:37:28–01:41:11 UTC), **171.610 seconds supervisor**, and **158.206 seconds
coverage invocation**. It accounted for all 68 packages and 751 results. The
trace reported four compiler commands and 71 linker command observations.
The customer-journeys package fell from 102.975 to 86.032 seconds, while the
whole job still exceeded three minutes. The trace counts are observations,
not measured linker CPU or a count of uniquely executed link actions.

One loop boundary case recovered on retry: `maximum_168h` timed out observing
its first scheduled Work after the invocation used a 20 ms return deadline.
The next fixture issues the accepted-duration request asynchronously, observes
the first scheduled Work, cancels its owned HTTP call, and joins that call
before teardown. Duration rejection cases retain their HTTP 400 assertions;
the separate overlap journey retains the bounded HTTP terminal-response check.
Ten focused repetitions passed in 7.009 seconds, and scoped repository lint
passed. This avoids an arbitrary short admission deadline without padding
its timeout or removing the customer duration boundaries.

The private removal of two direct internal metadata tests ran all 749 remaining
results but failed existing floors: localized metadata validation fell to
29.63% against 72.22%, and snapshot mapping fell to 68.75% against 75.00%. That
deletion is not shipped. It needs public canonical-metadata proof and a
documented classification of the purely programmatic defensive snapshot branch;
no floor has been lowered to claim a performance checkpoint.

A private explicit package-cleanup-hook experiment merged two more packages
(21 groups, 175 original tests). Its full lane passed in **112.81 seconds**,
with all 751 results and unchanged floors. It preserves native TestMain for
packages with startup before m.Run or unsafe global/relative fixtures. The
current prototype requires further lifecycle ordering and eligibility checks
before adoption. Its tool trace includes tool-program links as well as tests:
39 links, 44.354 seconds linker CPU, 44.088 seconds active linker wall intervals,
and 137.839 seconds summed link wall. It also performed 30 compiler calls
(3.097 seconds CPU) for changed tooling/bridge sources. This is local evidence
for a prototype, not the next hosted checkpoint.


### Latest hosted customer-observation fix: 62e5d98d79

[Hosted run 37401008431](https://github.com/portpowered/you-agent-factory/actions/runs/37401008431) passes the complete functional lane without retries: 68 packages, 751 results, 749 pass and two skips. The functional job takes **237 seconds**, including setup and reporting. Its supervisor takes **188.583 seconds** and coverage invocation **177.583 seconds**. This does **not** satisfy the three-minute complete-job checkpoint.

The restored compiler archive cache observes only two compiler commands for changed test sources and 36 linker commands. A cache miss does not explain this slower run. The largest package is customer_journeys (116.003s), followed by cli_rest_journeys (77.730s), packaged/invocation (73.274s), provider_sessions/details (47.802s), MCP worker_sessions (45.970s), recordings/lifecycle (45.310s), and models/inference (43.309s). Package elapsed times overlap and include parallel scheduling; they must not be added as the lane critical path or treated as CPU measurements.

The separate Verification Policy gate fails on an outdated dry-run assertion that assumed `-stream` immediately followed `-suite functional`. The command now includes `-functional-monolith=true`; the job-count handoff remains four in that policy test. Backend Lint passes. The policy assertion is corrected in the next candidate.

### Teardown-aware consolidation experiments

The first private cleanup-hook prototype joins two additional teardown-only packages (rollout and MCP protocol), preserving all 751 results and blocking floors. It passes in **112.81s**, with 39 observed tool links including supervisor tools: measured link CPU **44.354s**, link active wall **44.088s**, and sum of overlapping link wall **137.839s**. It is a private source experiment, not hosted evidence or a three-minute checkpoint.

A subsequent same-feature experiment limiting each process to GOMAXPROCS=2 passes in **134.20s** (coverage invocation 128.005s), without retries. Its 39 links consume **44.501s CPU**, occupy **42.401s active wall**, and sum to **114.963s overlapping wall**. Four compiler commands are observed after source changes. This does not demonstrate a total-lane improvement over the four-thread prototype; the reduced thread limit is not adopted. Earlier valid four-thread private samples range from 111.92s to 162.52s and remain relevant to variance.


The expanded teardown-aware candidate consolidates **24 packages and 223 top-level tests**, compared with the shipped 19 packages/171 tests. Only five reviewed teardown-only packages opt in: rollout, MCP protocol, runtime_api, workers/inference, and workers/inference/agy. Other fixture, global-state, executable, relative-path, startup-TestMain and quarantine boundaries remain native. Native retries continue to use original packages. The hook runs through Go's parent `t.Cleanup` after every parallel child; cleanup errors still fail the package. Functional execution is fresh `-count=1`; this lifecycle opt-in does not claim repeat-count support for once-only shared fixtures.

The expanded private run passes in **155.79 seconds**, with coverage invocation **145.599s**, all 68 packages/751 results, 749 pass/two skips, and unchanged blocking coverage gates. It observes 13 compiler actions after source changes and **34 links**, versus 39 in the smaller prototype. Those links consume **62.228s CPU**, occupy **68.692s active wall**, and sum to **175.355s overlapping wall**. The total run is slower than the smaller prototype despite fewer binaries; the evidence establishes reduced link count, not a controlled wall-time improvement. Focused Go eligibility tests, scoped built-in and repository lint, and six lane-budget policy tests pass. Hosted timing remains necessary before the three-minute merge checkpoint.


### Hosted teardown candidate and bounded-concurrency comparison

[Run 37402715640](https://github.com/portpowered/you-agent-factory/actions/runs/37402715640), head b18c676cdd, has a green **234s complete functional job** and **175.238s coverage invocation**, with 751 results/68 packages. One Providers adverse-recovery timeout subcase fails its first-dispatch error assertion and recovers on same-head retry. The next candidate removes this duplicate subcase, its runner and call counter. `TestScriptExecutor_RuntimeWorkstationTimeoutRequeuesAndRetriesOnLaterTick` retains the same workstation limit through public Factory Events and Work, observing first timeout, later retry and successful completion. The distinct worker-level timeout case also remains. No coverage policy change is made.

Its trace reports 66 compiler/61 linker command observations. Failed monolith command handling appends raw output to diagnostics after normalized output; trace lines can therefore appear twice. These observations must not be reported as unique builds or CPU. Local external tool timing records actual subprocess invocations.

Removing two diagnostic-only Git walks allows Product customer_journeys and cli_rest_journeys to join the monolith: **26 packages/373 tests**, 751 total results. Revision diagnostics still use supplied CI/build revision metadata. No customer assertion or customer-owned Git history is removed. Twelve-way execution passes in **170.16s**, with **161.762s coverage invocation**, **541.55s user+system CPU**, peak process RSS **3.95 GiB**, and 32 tool links (**56.192s CPU**, **49.912s active wall**, **152.645s overlapping wall sum**). This layout does not demonstrate a wall improvement at twelve-way execution; the smaller 24-group sample took 155.79s.

Eight-way execution of the 26-group source passes in **138.97s**, **131.283s coverage invocation**, **398.99s user+system CPU**, peak process RSS **3.90 GiB**. Its 33 links, including supervisor tools, consume **45.651s CPU**, occupy **43.962s active wall**, and sum to **94.404s overlapping wall**. The rerun script redundantly attempted to remove the already-removed Git walk and printed a preparation error before continuing; the measured source and full suite are unchanged and complete. The preparation step is omitted from subsequent reruns.

Six-way execution with the duplicate timeout subcase removed passes in **141.73s**, **134.129s coverage invocation**, **396.59s user+system CPU**, peak process RSS **3.68 GiB**. Its 32 links consume **48.858s CPU**, occupy **41.083s active wall**, and sum to **71.549s overlapping wall**. Both lower concurrency runs preserve all top-level results and blocking gates without retries. Six does not demonstrate an improvement over eight; the next hosted candidate uses eight. These are consecutive source experiments, not controlled identical-source repetitions; the source change and observed range must remain disclosed.

The `you init` fixture also registers cleanup for both ordinary and interactive root-built processes. Five focused repetitions of Init/normal-command cases and Providers adverse-recovery/retained causal workstation timeout cases pass. Built-in lint and six workflow cache/concurrency contract tests pass; workflow lint passes all eight files. Hosted timing and required checks remain necessary for the merge checkpoint.


### Latest hosted eight-way candidate and actual CPU investigation

[Run 37404058648](https://github.com/portpowered/you-agent-factory/actions/runs/37404058648), head `592184914dc53c10bc2a1a63343ac07ada8673e9`, passes the complete functional job in **192 seconds** (02:26:55–02:30:07 UTC). Coverage invocation takes **147.527s**, with 68 packages/751 results, 749 pass/two skips, and no retries. The successful trace records eight compiler actions and 29 linker commands. Its restored archive key is the preceding b18 candidate, so this is a changed-source cache measurement. Setup takes approximately 24s; supervisor approximately 156s; remaining capture, save, reporting and post steps approximately 12s. Twelve more seconds must be removed from the complete job before claiming the three-minute checkpoint.

The 26-group private twelve-way run consumes **541.55 user+system CPU seconds**; the eight-way run consumes **398.99 CPU seconds**, a 26.3% reduction. In the latter, links consume **45.651 CPU seconds**, about **11.4% of total measured CPU**, and occupy 43.962s of overlapping active wall intervals. The 94.404s sum of overlapping link durations is neither physical CPU nor total lane wall time. Test execution, supporting tools and supervision account for the remaining CPU; CPU profiles are required to attribute that remainder accurately. Hosted package elapsed times include scheduling and waits and cannot substitute for per-package CPU accounting.

A separate private control keeps the two large customer journey packages native (24 groups, eight jobs, current provider timeout cleanup and init process cleanup). It passes in **147.04s**, using **479.81 CPU seconds**, with 34 links consuming **56.487 CPU seconds**, 66.872s active linker intervals and 123.605s overlapping link sum. Peak single-process RSS is 2.22 GiB. The corresponding broader eight-way sample took 138.97s/398.99 CPU seconds with 3.90 GiB peak single-process RSS. These are consecutive source experiments, not identical-source repetitions; consolidation trades larger process memory for less aggregate work. Maximum RSS reported by time is the largest individual process, not the sum of concurrently resident processes.

Increasing GOGC to 150 in the 26-group/eight-way private layout does not improve this sample: **144.97s elapsed**, **419.29 CPU seconds** (329.31 user/89.98 system), and **4.74 GiB** peak single-process RSS, versus 138.97s/398.99 CPU seconds/3.90 GiB with the default collector setting. This tuning is rejected. No collector setting change ships, and this experiment does not establish that garbage collection dominates CPU; execution profiles will identify the actual hot paths.


### CPU attribution: packaged installation dominates execution

A private diagnostic run adds `runtime/pprof` labels around each original monolith test and uses an external `/usr/bin/time` wrapper around every native test binary. The monolith samples **230.42 CPU seconds over 86.30s elapsed**. Labels cover **211.76s (91.90%)** of sampled CPU; the remaining **18.66s** are unlabelled runtime/background work. Labels propagate to spawned goroutines; long-lived shared fixture goroutines retain their creating test's label, so package attribution is more reliable than individual-test attribution. Native totals below are measured process user+system CPU, including child-process accounting; monolith totals are statistical sampled CPU. This diagnostic run fails a selected-time event-stream observation deadline and recovers that test natively; it is not a successful checkpoint or clean performance baseline. The temporary labels/profile wrapper are private and do not ship.

| Functional package | Execution CPU seconds | Measurement | Primary next action |
| --- | ---: | --- | --- |
| product/customer_journeys | 47.39 | labelled samples | Reuse initialized hosts and explicit sessions; avoid repeated fresh-home catalog installation in unrelated journeys |
| product/customer_lifecycles | 40.04 | labelled samples | Retain shared session fixtures; profile remaining catalog setup and repeated factory decode during review/payload scenarios |
| factory/packaged/invocation | 26.87 | native user+system | Keep focused packaged CLI behavior, share setup across independent invocations, avoid reinstalling every catalog for each scenario |
| product/cli_rest_journeys | 18.01 | labelled samples | Share public preparation while retaining CLI/REST parity and independent state |
| models/inference | 16.19 | native user+system | Retain isolated model caches; replace legacy fixture semantics with canonical LocalAI behavior |
| factory/execution | 15.17 | labelled samples | Reuse prepared home/host for customer execution scenarios; isolate Work through sessions |
| work/admission | 14.84 | labelled samples | Reuse prepared home/host; retain distinct rejection and accepted Work proofs |
| providers/acp | 14.57 | native user+system | Reduce repeated bootstrap for protocol cases; preserve ACP protocol observation |
| transport/mcp/worker_sessions | 14.23 | labelled samples | Keep one owned host with isolated sessions and MCP observations |
| transport/cli/customer_commands | 11.74 | labelled samples | Share public installed-home setup across command matrices |
| factory/visualization/runtime_metrics | 12.03 | native user+system | Separate actual customer metric behavior from any system-analysis fixtures |
| sessions/chat_sessions/acp | 10.72 | native user+system | Reuse host and isolate conversations/sessions |

The dominant production stack is **system_initialization/internal/workflow.Initialize: 149.93 sampled CPU seconds (65.07% of monolith samples)**, almost entirely **factory_definitions/internal/services/distribution/packagedinstallation.InstallPackagedFactory: 149.56s**. Within that nested path, packaged layout preparation consumes **90.58s**, canonical factory mapping/expansion **67.69s**, and definition validation **42.10s**. These are overlapping cumulative stacks and must not be added. JSON unmarshalling across the binary consumes **86.91s cumulative**; string parsing/validation is a substantial flat hotspot. Syscall6 accounts for **41.50s flat**; mkdir-related stacks account for **25.11s cumulative**. This identifies repeated factory decoding, validation and materialization as the main execution target rather than merely long timer waits.

The optimal next order is: (1) remove repeated full-catalog bootstrap from unrelated runtime journeys using prepared, isolated homes and then owned hosts with explicit sessions; (2) simplify the largest customer journey matrices while retaining distinct customer guarantees; (3) consolidate additional safe native packages where their lifecycle boundaries permit it; (4) replace timer waits with causal observations or synctest only where no real-I/O boundary prevents virtual time. Linking remains worthwhile but the current warm sample spends about 11.4% of total CPU there; it cannot explain most remaining CPU. Collector tuning is rejected by the measured GOGC150 regression.


### Rejected prepared-home prototype and retained synchronization fix

The private prototype prepares the packaged catalog once through public `Process.Execute(you run --factory <missing>)`, snapshots its file outputs, and copies those immutable bytes into distinct homes for ordinary generic runtime journeys. Model cache paths remain isolated; configured-home and bootstrap-specific cases retain empty homes. The first run rebuilds 118 compiler actions (**52.309 compiler CPU seconds**), takes **142.94s** and **437.06 total CPU seconds**, and fails the pre-existing active-script captured-log assertion. This is not a valid performance benchmark.

A warm repeat passes all **68 packages/751 results**, 749 pass/two skips, and blocking coverage gates without retries, but takes **153.77s** and **473.23 total CPU seconds** (369.11 user/104.12 system), with no compiler actions. Its 32 links consume **53.086 CPU seconds** and occupy **49.933s active wall**. Peak individual process RSS is approximately 3.88 GiB. This is slower and consumes more CPU than the earlier unmodified eight-way sample (**138.97s/398.99 CPU seconds**), so the fixture snapshot cache is **rejected and removed**. This is a sequential source experiment rather than controlled identical-source repetitions. Existing installed targets still require validation, expected-content preparation and reconciliation; copying installed files does not remove those production paths. The next direction is explicit session reuse on an already-initialized process, not broader home-copy caching.

The first prototype exposed an independent synchronization race in `TestWorkerSessionCapturedLogsActiveScriptWriteFailure`: its controlled command signals that output has been emitted before the asynchronous recording writer commits the chunk. A single immediate logs read can therefore observe only the opening event. The retained change waits through the public logs endpoint for committed position two and two events before the existing CLI/HTTP parity and fault-preservation assertions. It adds no sleep, no timeout extension, no relaxed assertion and no internal observer. Ten focused repetitions pass in **3.201s**. This prevents a false failure and avoidable native retry; it is not claimed as a measured whole-lane CPU saving.


The private eight-build-job/four-parallel-test experiment takes **173.08s** and **386.29 CPU seconds** (312.90 user/73.39 system), with a selected-time event-stream observation failure. Its combined binary takes approximately 101s. Seven compiler calls consume 13.698 CPU seconds and 35 links consume 44.879 CPU seconds; peak individual process RSS is 3.54 GiB. This is a failed diagnostic sample, not checkpoint evidence. It does not improve elapsed time over the earlier eight/eight run and is not adopted. Build and execution budgets remain eight.

Package wall reporting also needs correction: Go's parent test `Elapsed` excludes parallel children. Once customer journeys are combined and their children run in parallel, some successful original packages appear to take zero seconds or only the parent's serial work. The normalizer now records each original group's start timestamp and calculates terminal package elapsed from the full event window, including child execution and parallel scheduling waits. Child test timings and native package events retain their original values. A regression test proves a ten-second package window despite the parent terminal reporting zero. Earlier consolidated-package elapsed rankings must therefore be read with this limitation; CPU attribution above remains independently measured. This reporting correction changes no execution, inventory, coverage floor or verdict.


## Isolated-session host consolidation, October 5 continuation

The next implementation targets the measured initialization hotspot rather than
only cutting linker count. Twenty-two existing customer scenarios now use four
explicit, parent-owned process hosts: ten routing journeys, five guard journeys,
three provider retry/recovery journeys, and four prompt/template journeys. Each
leaf still owns its copied Factory, command-response queue, Factory Session,
Work identities and output assertions. Closing one session does not stop peer
sessions; cleanup terminates and deletes the session through REST before removing
its external command route. Each host retains its own isolated operator home and
model-cache configuration. No process-global default session or cache registry
was added.

Provider-internal mocks and token assertions were replaced with the command edge,
public dispatch events, terminal Work, rendered command stdin and the actual
working directory. The branch-tag prompt case now proves the customer-visible
worker directory selected by that tag. Matching and mismatching guards still
prove public dispatch, Work correlation and absence of unwanted provider calls.
The blocked sessions legitimately report ACTIVE; an extra IDLE assumption was
rejected after five focused repetitions exposed it. The final guard cases retain
the original public Work-category guarantee. Their 50ms sleep loops now use the
bounded public-observation helper, which evaluates immediately.

Workflow source files now identify routing, guards and provider retries instead
of internal Petri composition. Re-grouping preserves all 22 leaf scenarios while
reducing reported top-level results from 751 to 742. Coverage manifests, minimums,
quarantine and retry policy are unchanged.

Packaged Review exposes its existing teardown as an explicit monolith cleanup
hook. Runtime Metrics, Factory Transformation and CLI Invocation initialize their
existing package fixtures lazily through sync.Once and expose the same cleanup
hook. Native TestMain still owns native teardown; the combined package owns
teardown after all of its parallel children finish. This makes **30 packages / 451
top-level tests** eligible, versus 26 / 373 before this batch. All executable,
relative-fixture and process-wide-state exclusions remain intact. Attempted
teardown opt-ins for Packaged Fix, Chat Sessions ACP, Models and docs smoke were
removed because other independent exclusions still require their native binaries;
there is no benefit in shipping inactive hooks or relaxing those exclusions.

An observed provider-log retry exposed a separate completion assumption: stopping
one session does not stop peer log writers. A trailing incomplete JSON record is
therefore transient, not proof of malformed persisted output. The file-output
assertion now waits for the complete matching customer record, stops scanning once
it finds that record, and preserves missing-record/malformed-record failure
reporting. Ten focused repetitions pass. No additional fixed sleep was introduced.

Validation so far: five routing/prompt repetitions, five guard/retry repetitions,
three native lazy-fixture packages, twenty selected-time automation repetitions,
and scoped repository/built-in lint pass. The complete four-CPU Linux experiments
retain coverage floors, selected inventory, quarantine and fresh execution:

| Private experiment | Elapsed | User + system CPU | Compiler actions | Links | Outcome |
| --- | ---: | ---: | ---: | ---: | --- |
| Routing/prompt hosts, changed source | 184.07s | 580.25s | 119 | 34 | Pass |
| Same routing/prompt source, warm archives | 140.58s | 403.37s | 0 | 32 | Pass |
| Add guard/retry hosts and Review cleanup | 166.18s | 503.93s | 119 | 32 | Pass after one provider-log native retry |
| Add three lazy fixture packages | 124.37s | 355.74s | 21 | 29 | Failed selected-time automation observation; native leaf retry passed |

The first warm repeat partially overlapped a focused Windows check, so it is
reported as diagnostic evidence rather than a controlled speedup. Reused report
directories also retained an old flake ledger in that repeat; its fresh timing
summary has 746 results, 744 passes, two skips and no failing test. Subsequent runs
clear only the private generated report directory and use distinct tool-timer
folders. The broad failed run's coordinator failure keeps the lane failed even
though the individual automation retry passes; its short elapsed time is **not** a
successful baseline. New diagnostics report the pending Work identities and public
Work projection if that automation completion observation expires.

These measurements use the previously described private Linux audit checkout with
current changed source files copied in; they are not exact hosted-head results.
The latest hosted complete-job result remains 192s. No three-minute checkpoint has
been claimed from these local samples.

The current expanded batch passes without retries: **129.01s supervisor elapsed**,
**122.034s coverage invocation**, **378.63s aggregate CPU** (303.63 user + 75.00
system), 68 packages, 742 results, 740 passes and two skips. Peak single-process
RSS is 4,034,972 KiB (about 3.85 GiB). This is a successful changed-source local
sample, not the hosted three-minute checkpoint; an identical-source warm repeat
and a conflict-only rebase against current main follow.

The identical-source archive-warm repeat also passes without retries, but takes
**165.17s supervisor / 159.151s coverage invocation** and **486.04s aggregate CPU**
(379.22 user + 106.82 system). It has no compiler actions and 28 links. Linking
uses 48.734 CPU seconds with 47.818s active wall time; the previous successful
sample uses 36.578 linker CPU seconds with 32.122s active wall time. Successful
expanded-batch samples therefore range **129.01–165.17s**. This variance prevents
claiming a consistent suite-wide speedup from one favorable local sample, even
though four additional package binaries are eliminated and eighteen repeated
customer host initializations are removed.

GitHub subsequently reports a content conflict. The candidate is rebased against
current main, preserving its additional explicit home/model-cache isolation in
MCP Protocol and Worker Inference alongside the teardown hooks. The obsolete
legacy-log helper remains removed; current-writer incomplete/damaged recovery
helpers retain those customer outcomes. Current main also changes runtime
construction/opening, so the older private measurements cannot establish the
rebased candidate's latency. Scoped repository lint on the rebased tree reports
zero issues; exact-source Linux validation and hosted checks are required next.


### Rebased hosted result and durable-store isolation

Head `6617d42231`, based on current main, passes functional coverage with no
retries: **310s complete job**, **268.955s supervisor**, **247.004s coverage
invocation**, 68 packages / **760 results** (758 passes, two skips). Current main
adds 18 selected top-level tests; 30 packages / **462 tests** now share the
combined executable. The build trace records **508 compiler actions and 25 links**,
versus eight compiler actions and 29 links in the older 192s hosted result. This
is a changed-runtime/archive-refill sample, not a regression measured on identical
source. Compiler archives are captured and saved in eight seconds; setup consumes
26 seconds and the remaining completion steps consume about 15 seconds. The
three-minute complete-job checkpoint is still outstanding.

The complete-event-window package ranking is Customer Lifecycles 112.417s,
CLI/REST Journeys 109.251s, Recording Lifecycle 102.429s, Customer Journeys
101.904s, Worker Inference 101.828s, CLI Customer Commands 97.492s, CLI Invocation
97.233s and Factory Transformation 97.117s. Those windows overlap and include
scheduling waits; they are **not package CPU measurements** and must not be added
as elapsed CI time. The earlier labelled profile remains the package CPU evidence
for the old runtime, with the source/failure limitations already recorded.

An exact-source private copy is verified against tree
`80ee56fd93969dbce13c603130184bdf9cc9e53d`. Its diagnostic run begins with 1,310
compiler actions and accumulated runtime output from previous private runs. WSL
then stops responding; this run is terminated and excluded from latency/CPU
baselines. Its generated evidence and original private Git history are not
shipped. Clean source contents alone are insufficient for a controlled repeat:
invocation-owned durable state must also be fresh or deliberately restored.

The API helper exposes a related real test-isolation issue. It passes the
scenario's working directory to the CLI, but the storage construction edge still
defaults to the binary's process working directory. Consolidation makes that
one shared directory for unrelated hosts. The next fix binds the storage edge to
the same explicit invocation directory, preserving explicit recovery/storage
overrides. Independent hosts therefore do not load and append to the same
package-wide durable store. This is a fixture ownership change; its performance
benefit remains to be measured on hosted CI.

The rebased hosted lint failures identify three stale sleep-baseline entries and
the moved Runtime Metrics teardown deadline. Delete the stale entries and mark
the deadline as bounded teardown, without increasing any debt allowance. The
previous local scoped custom-binary invocation used the default lint config and
did not activate the repository plugin. Subsequent checks explicitly use
`.golangci-repository.yml` with the repository's CI tag set; prior built-in lint
results remain valid, but are not evidence for that plugin.

The current repository plugin is rebuilt from the rebased source and reports
**zero issues** with explicit repository config and CI's complete tag set across
the affected packages. With the directory binding, three routing/guard/retry
repetitions and three prompt repetitions pass. A broader native Windows attempt
passes Factory Execution in 43.661s, but Customer Journeys times out at 600.437s
in hosted-runtime lifecycle scenarios that call BuildProcess directly and bypass
the changed API helper. This run is not a successful suite baseline. One such
scenario is checked separately; Linux hosted coverage remains required for the
helper's broader consumers.

The isolated Windows `TestHostedContinuousRunsStayLiveWhileIdle/server` also fails
in 35.222s waiting for listener startup/shutdown. It does not call the changed
API helper, so this is separately recorded as a native hosted-runtime limitation,
not attributed to the storage binding. No timeout ceiling is increased.


### Test-only hosts and further setup reduction

A fresh-state four-CPU Linux sample of `ec85aae468` passes in **128.04s
supervisor / 122.132s coverage invocation**, 68 packages / 760 results, no retries
and **373.87s aggregate CPU** (307.20 user + 66.67 system). Compilation consumes
40.688 CPU seconds across 117 actions; 31 links consume 37.840 CPU seconds.
Compiler active wall is 22.167s; linker active wall is 34.464s. These overlap
other work. Maximum individual-process RSS is 5,208,324 KiB (about 4.97 GiB),
not aggregate peak memory. Unrelated Windows workloads limit comparability.

The fresh-state warm attempt fails the selected-time watcher despite a passing
native leaf retry: 137.98s elapsed, 389.80 CPU seconds, two compiler actions and
29 links. This is failed diagnostic evidence. No peer Work had been admitted.
The next fixture drives watcher debounce registrations until the owned file
read is acknowledged, then retains public dispatch and Work assertions. A real
filesystem notification may replace the timer observed first; its first timer
registration alone is insufficient. Exact pre-due, retry, cancellation and
late-result checks remain. The initial observer implementation rejected unrelated
5ms metrics waits; it now ignores those as the existing scheduler waiter does.
Three native repetitions pass. A prior 20-repeat native attempt stalls in hosted
activation before the final explicit storage edge and is interrupted; it is not
successful evidence.

Backend Lint finds five new support helpers in its production dead-code scan,
which excludes tests as roots. Move the hosts into their two owning test
packages' `_test.go` files, and withdraw the broad automatic API-helper storage
binding. Each new host and the selected-time host instead supplies its owned
storage directory explicitly. Production support dead-code findings then match
the existing baseline exactly. Routing, guards and provider retries now share
one parent-owned host; prompts retain a second host. All 22 customer scenarios
remain session-isolated. Grouping changes selected results from 760 to 758.
Five shared-session repetitions and three prompt repetitions pass.

Remove the functional provider-Go-panic compatibility case; public provider
failure routing remains, and the passing worker unit
`TestExecuteRunnerPanicBecomesSafeFailedResult` retains safe panic recovery proof.
Providers, Codex inference and invoke/continue expose existing lazy teardown
through consolidation hooks, without changing native TestMain behavior.
Complete Linux validation and hosted timing follow before accepting the batch.

The first test-local host batch passes the full Linux lane with 758 selected
results and no retries, but takes **203.09s supervisor / 195.446s coverage
invocation**, **637.09 aggregate CPU seconds** (504.62 user + 132.47 system),
119 compiler actions and 28 links. Peak individual-process RSS is 7,265,056 KiB.
It does not show an improvement over the earlier 128.04s sample. That batch
withdrew the general API-helper directory binding; unrelated hosts then share
storage again, including within one execution. This is an isolation concern,
not an established explanation for the whole measured variance.

The final batch retains the general binding using the existing policy-free
filesystem `Local` adapter's optional working-directory value. Its default
remains `os.Getwd`; an explicit value changes only that effect, never the host
process directory. A focused unit verifies both the owned value and unchanged
host directory. No new production function/dead-code allowance is introduced.
Explicit scenario storage overrides remain authoritative.

Redundant missing-Factory bootstrap invocations are also removed from eight
customer fixture files. Normal public host startup still initializes the
customer home; construction-inertness assertions remain, and no model catalog
or initialization implementation is mocked or bypassed. Three selected-time /
shared-session repetitions pass after this change, and the affected repository
lint reports zero issues. The three additional opt-ins compile natively with
empty selectors and retain nil-safe teardown. Complete final-source Linux and
hosted checks remain required.

Linux registration rejects the three additional Providers/Codex/invoke-continue
hooks because those packages still have executable fixtures; those inactive
hooks are withdrawn. Factory Definitions has no such exclusions. Its test-only
bridge moves to a test file and delegates directly to lazy fixture setup; its
existing teardown is shared by native TestMain and the monolith cleanup hook.
The retired production-helper dead-code baseline entry is deleted. An obsolete
selected-time directory-helper reference left by the last cleanup causes a
monolith build failure before any valid complete-suite timing; it is removed.
That 109.69s incomplete run is excluded from latency baselines.

The corrected 31-package candidate passes all 68 packages / 758 results without
retries in **147.18s supervisor / 140.269s coverage invocation**. Aggregate CPU
is **447.95s** (362.02 user + 85.93 system), 54 compiler actions consume 24.185
CPU seconds, and 28 links consume 40.751 CPU seconds with 37.053s active wall.
Peak individual-process RSS is 5,532,956 KiB. This is successful private evidence,
not a hosted checkpoint or a consistently faster identical-source comparison.

Claude golden transcript reads now use the existing absolute repository-path
helper instead of embedding package-relative fixtures. Transcript bytes, hashes
and public replay assertions remain unchanged. Session restart's workflow input
also resolves through that helper. Both packages pass three native repetitions
(37.731s and 10.947s), and repository lint is clean. Their cwd/embed exclusions
can therefore be removed by ordinary source discovery without loosening any
consolidation classifier. The next combined lane verifies those cases together.

## Current-runtime CPU profile and admission consolidation

A successful full private Linux diagnostic run at `f5d203cad5` takes 173.17s
elapsed and 527.62 aggregate CPU seconds (413.37 user / 114.25 system), including
profiling overhead. The combined binary takes 112.91s elapsed / 325.76 CPU;
its profile contains 318.15 sampled CPU seconds. All 68 packages / 758 selected
results pass with no retries. This is diagnostic evidence, not an unprofiled
hosted checkpoint. Labels cover 289.32s (90.94%) of sampled CPU; long-lived
fixture goroutines inherit the creator's label, so individual test attribution
can include shared background work.

| Combined package | Sampled CPU seconds |
| --- | ---: |
| Product customer journeys | 56.37 |
| Product customer lifecycles | 55.80 |
| Work admission | 24.82 |
| Product CLI/REST journeys | 23.66 |
| Factory execution | 16.93 |
| CLI customer commands | 14.61 |
| Runtime metrics | 11.46 |
| CLI invocation | 9.69 |
| MCP Worker Sessions | 8.71 |
| Session isolation and recovery | 8.10 |

System initialization has 193.30 cumulative sampled CPU seconds (60.76% of the
combined binary); packaged installation has 192.50, managed Factory creation
169.55, layout preparation 116.76, JSON unmarshalling 122.05, and configuration
expansion 86.94. These nested cumulative values overlap and must not be summed.
Repeated fresh-home installation and definition expansion are the primary next
CPU targets. Sharing already initialized hosts with explicit sessions preserves
customer behavior without substituting internal initialization implementations.

The unprofiled 33-group private sample takes 207.52s complete / 196.623s
coverage, 662.78 CPU seconds, six compiler actions and 25 timed linker processes
(56.834 CPU seconds / 49.684s active intervals). The preceding 31-group sample
takes 147.18s complete. Fewer binaries did not yield a consistent measured wall
improvement on this loaded local host. Claude and Restart contribute only 5.50
sampled CPU seconds together; their inclusion alone does not explain the
observed total-run variance.

The next change groups four batch admission journeys behind one initialized
host: duplicate-name rejection, explicit Work-ID conflicts and exact replay,
payload-limit diagnostics across file/stdin/inline/dry-run inputs, and relation
endpoint validation. Every child opens and closes its own explicit Factory
Session and factory directory. CLI submissions, Work lists and retained events
all name that session. No public assertion is removed. Three native repetitions
pass in 7.755s; full-suite and hosted measurements follow separately. Four
former top-level tests become four named children of one parent, reducing the
selected top-level inventory by three without removing scenarios.

The same-head hosted warm-cache rerun of `f5d203cad5` completes successfully in
155s (05:18:26–05:21:01 UTC), with 111.277s coverage, zero compiler actions,
22 links and an exact archive-cache hit. All 68 packages / 758 results pass with
no retries. PR #2905 is queued for merge at this validated head. This meets the
180s complete-job checkpoint for a warm cache; it does not establish a cold
build guarantee or the 120s checkpoint.

The admission consolidation passes the full private Linux supervisor in
179.56s. Its selected inventory is 755 results (four former parents become one
parent), with every customer scenario retained. The source still combines 33
compatible packages; their top-level registration count becomes 488. Host-load
variance remains substantial, so this one run is validation rather than a
claimed throughput improvement. Focused native repetitions and repository lint
also pass.

### Native-package CPU costs from the successful diagnostic run

These are measured process CPU seconds, not monolith label samples. Wall times
include concurrency with other packages and profiling overhead. Do not combine
these wall values into a total or compare them directly with sampled CPU.

| Native package | User + system CPU seconds | Wall seconds |
| --- | ---: | ---: |
| Packaged Factory invocation | 26.45 | 35.23 |
| Models inference | 15.76 | 22.65 |
| Providers ACP | 14.09 | 19.21 |
| Chat Sessions ACP | 11.51 | 18.49 |
| MCP stdio | 10.57 | 16.27 |
| Worker invoke/continue | 6.94 | 13.22 |
| Providers | 6.74 | 13.85 |
| Mock Workers | 5.98 | 6.37 |

These executable/protocol fixtures remain native. Prioritize repeated setup
within their compatible cases rather than merging away executable isolation.

### Subsequent cold-cache hosted evidence

PR #2905's merge-queue candidate passes functional coverage in **316s complete /
243.519s coverage**. It has no matched archive cache, 672 compiler actions and
22 links. This does not contradict its 155s same-head warm-cache rerun, but it
shows that cache availability is material to the three-minute goal. PR-scoped
cache entries are not sufficient evidence of default-branch/queue reuse; the
main-push cache-seeding path must be verified after merging.

The admission candidate `6ce70798aa` passes its initial hosted functional job
in **310s complete / 235.462s coverage**, all 68 packages / 755 results, with
no retries, no matched archive cache, 671 compiler actions and 22 links. The
120s complete-job checkpoint remains unmet. The local full-run CPU is 570.51s
(449.41 user / 121.10 system); three compiler processes use 1.940 CPU seconds
and 25 timed linker processes use 48.838 CPU seconds, with 48.516s active linker
intervals. Peak process RSS is 5,757,144 KiB. CI diagnostic command counts and
local timed-process counts use different instrumentation and are reported
separately.

PR #2905 merged on October 6 at 05:39:06 UTC as
`e9ab358719619483a270f9e32bec547b5af92e5b`, with all merge-queue checks passing.
The admission-only follow-up is draft PR #2923, rebased onto that merged main
commit. Its complete hosted 120s checkpoint remains pending; no two-minute
result is claimed.

The first admission PR full Backend Lint fails gocyclo at complexity 17 after a
former top-level Test function becomes a private shared-host runner. The
initial scoped repository-only lint did not run that built-in checker. Extract
input acquisition and diagnostic assertions into separate helpers without
changing assertions. Three post-rebase repetitions pass in 5.632s; both the
built-in `.golangci.yml` and repository `.golangci-repository.yml` scoped runs
now report zero issues. The hosted correction still needs its fresh full gate;
no lint allowance is added.


## Continued consolidation and customer-boundary cleanup (October 6)

The two-minute complete hosted-job checkpoint remains unmet. The three-minute
checkpoint previously merged in #2905 was a warm-cache result (155 seconds);
its merge-queue run took 316 seconds after an archive-cache miss.

### Current-source measurements

| Run | Complete duration | Coverage command | Compile/link tool invocations | Outcome |
| --- | ---: | ---: | --- | --- |
| #2923 rebased head `496dca52d5`, hosted initial | 251 s | 191.605 s | 510 / 22 | Passed; complete CI passed |
| Same head hosted rerun, exact archive-cache hit | 327 s | 251.213 s | 380 / 43 | Failed; excluded from successful timing evidence |
| Main archive seed `e9ab358719` | 278 s | 204.126 s | 1341 / 43 | Passed; archive-cache miss |
| Pending cleanup, current-source Linux, Models consolidated | 174.09 s | 167.293 s | 4 / 24 | Passed; 68 original packages, 755 results, 753 pass, 2 skip, no retry |

The last row is a local controlled warm-cache measurement, not hosted checkpoint
proof. It used four pinned CPUs, GOMAXPROCS=4, GOGC=100, package jobs=8, fresh
execution (`-count=1`), full functional coverage and unchanged coverage policies.
Aggregate process CPU was **569.28 seconds** (442.82 user + 126.46 system).
Compiler CPU was 3.466 seconds; linker CPU was **43.752 seconds**. Linker tools
were active for 41.412 wall seconds, with an overlapping invocation sum of
88.784 seconds. Tool CPU is included in aggregate CPU; these numbers must not
be added together. Largest-process RSS was 6,475,544 KiB, not aggregate memory.

A preceding current-source run passed in 241.90 seconds (221.798 coverage,
1364 compilers, 25 links). A 12.546-second native Models validation overlapped
that sample, so it is validation-only and excluded from latency comparisons.
Neither local sample is a paired estimate of savings against the earlier
runtime revision. Private-repository Git-history contamination remains the
previously documented limitation for Git fixture results.

### Changes and retained customer proof

- Removed redundant customer-home initialization before real CLI/API startup
  in Metrics, response Events, provider-session reads, Work admission/watch,
  CLI/REST journeys, process-time and worker-concurrency fixtures. Tests retain
  owned profiles, explicit sessions, model-cache paths, external-effect fakes,
  and customer assertions. Initializing or repairing the profile is exercised
  through the actual customer operation.
- Replaced the 462-line internal invocation probe with REST success, provider
  failure and caller-timeout journeys. REST status, request/trace/Work identity,
  Factory Event correlation, private-error redaction, continued Work after
  timeout, missing-session diagnostics and peer-session reuse are asserted.
  Existing REST cancellation/session-isolation tests remain. Assertions about
  private invocation struct layouts and historical implementation behavior
  were removed from the functional layer.
- Converted Models inference's eager TestMain fixture into an owned lazy fixture
  with bounded native and monolith cleanup. Its unsafe sibling-reference case
  now uses an actual owned outside-cache sentinel and a relative escape path;
  it still proves fail-closed removal and preservation of both managed cache
  and outside content. This makes the package independent of process CWD and
  eligible for the 34th compatible group without weakening the compatibility
  classifier or sharing model assets between unrelated tests.
- Corrected coordinator failure attribution: after an original combined-package
  test failure, redundant Go wrapper failures do not become another customer
  test/package. The actual original failure and native command error remain;
  raw panic diagnostics, unattributed process failures and completion checks
  for every original package remain. Focused retry selection therefore uses
  the identified customer package, rather than rebuilding the entire combined
  inventory solely because the synthetic wrapper also failed.

The failed hosted rerun identified the removed internal caller-cancellation
probe: it required request identity even when the caller returned before
admission correlation. Its synthetic wrapper failure also exposed the broad
native rebuild path. This failed run is diagnostic evidence, not a performance
baseline or justification for silently accepting a failed test.

Focused validation before publication: Metrics cost journeys passed three
repetitions (54.553 s); replacement REST journeys passed three repetitions
(14.548 s); native Models package passed (12.546 s), and unsafe sibling removal
passed five repetitions (1.287 s). Scoped built-in and repository lint passed
for the nine affected functional package families. Full Linux inventory and
coverage validation passed as reported above. Coordinator regression tests
cover focused original-package selection, panic rejection and unattributed
process-failure visibility; existing partial-inventory checks remain.

The earlier CPU profile still prioritizes packaged installation / managed
Factory layout preparation and JSON expansion. Current package event windows
are scheduling-inclusive and overlapping; they are not package CPU estimates.
The next optimization should remove more repeated initialization inside
isolated shared-session fixtures, then measure fresh CPU attribution on the
current runtime. Archive restoration across PR/main/merge-queue scopes also
needs hosted verification before promising consistent three-minute cold runs.


### Current-runtime CPU profile and follow-up cleanup

A fresh instrumented current-source run passed all 68 original packages / 755
results without retries. Complete local supervisor: 212.92 s; coverage command:
200.209 s; aggregate process CPU: 686.95 s. The consolidated test process used
458.80 CPU seconds (349.78 user + 109.02 system) over 141.93 wall seconds.
The profiler sampled 444.24 CPU seconds; 406.40 seconds (91.48%) carried package
labels. Instrumentation and scheduling affect absolute performance; this is
attribution evidence, not a paired savings estimate against an unprofiled run.

| Combined original package | Label-attributed sampled CPU |
| --- | ---: |
| product/customer_lifecycles | 100.28 s |
| product/customer_journeys | 85.31 s |
| product/cli_rest_journeys | 35.36 s |
| work/admission | 25.30 s |
| factory/execution | 19.63 s |
| transport/mcp/worker_sessions | 18.39 s |
| models/inference | 17.29 s |
| transport/cli/customer_commands | 15.29 s |
| recordings/lifecycle | 11.67 s |
| factory/definitions | 11.42 s |
| factory/visualization/runtime_metrics | 8.03 s |

Package labels can include background fixture goroutines inheriting the
creator's label. They exclude native binaries and unlabeled process work.
They are sampled CPU, not elapsed package windows. Cumulative stack costs
nest: InitializeSystem 262.56 s (59.10%), packaged installation 261.88 s,
managed Factory creation 232.94 s, layout preparation 157.93 s and JSON
unmarshal 169.41 s must not be added. The immediate CPU target remains repeated
initialization and Factory expansion, rather than further blanket link merging.

Hosted head `968c90fe99` passed its functional job in **308 seconds**, with a
230.527-second coverage command, 211 compiler calls and 21 linker calls, all
68 original packages / 755 results and no failed tests. It restored the previous
head's compiler archive through a prefix key, rather than an exact key. Its
bounded capture reached 1,073,741,791 bytes and omitted 924 eligible cache
entries. This establishes a cache-budget concern, but does not establish that
all 211 compilations resulted from omission: source changes also invalidate
archives. An exact-head successful rerun and a controlled cache-budget comparison
are needed before changing the budget or promising a cold-run target.

Follow-up removes three visualization service-root composition probes and their
unused tracker/config fixture. Those tests directly invoked Activate/Join,
Observe and Open/Present/Finalize/Close operations and asserted private method
counts. Existing component/root-contract tests cover these outcomes under
`pkg/services/factory_visualization`; customer CLI presentation, REST state and
Factory Event journeys remain in the functional suite. Metrics costs,
process-time and concurrency direct-process hosts now inject owned session
working directories, preserving custom process-time overrides. This matches
API-helper isolation and prevents implicit repository-directory storage.
Coordinator output preserves panic/build-death headers even after an ordinary
customer failure, so such crashes cannot be mistaken for retryable flakes.


Follow-up full validation passed: 68 original packages, 752 results (750 pass,
2 skip), no retry, unchanged coverage policy. The inventory decrease is exactly
the three removed service-root visualization probes. Complete controlled Linux
run: **198.36 s**, coverage command: 184.957 s, aggregate CPU: 612.96 s
(468.42 user + 144.54 system), compiler calls: 12, linker calls: 26. Linker CPU:
36.350 s; active linker wall: 33.104 s. This run is slower than the earlier
174.09-second warm sample, so the cleanup is validated but no net latency win
is established by these unpaired samples. Both scoped linters passed and the
coordinator regression selection passed after the final panic-header change.
The two-minute hosted checkpoint remains outstanding.


## Exact-head build evidence and shared Work submission host

Head `65efc94442` passed every required hosted check. Its changed-source
functional job took 241 seconds (06:28:35–06:32:36 UTC), coverage command
179.466 s, 370 compiler commands and 21 links. Prefix cache restoration used
`968c90fe99`; capture retained 1,038,345,460 bytes with **zero omissions**.
The completed-workflow exact-head rerun passed the functional job in **212
seconds** (06:37:01–06:40:33), coverage 160.286 s, **zero compiler commands**
and 21 links. Both ran all 68 original packages / 752 results without failures.
The two-minute complete hosted-job checkpoint remains unmet.

### Compiler archive budget comparison

A build-only controlled comparison restored the same unchanged local compiler
cache into separate 1 GiB and 2 GiB snapshots. Both compiled the exact selected
functional inventory and overlay from the preceding full run with `-run=^$`:
no customer test bodies ran, and the result is not a functional latency sample.
Snapshots exclude test-result payloads and executables, as before. Both Go
invocations succeeded in each trial; both required only **one tiny compiler
invocation (~0.010 CPU seconds)** and 21 links. The 1 GiB build took 19.005 s
and 50.623 aggregate CPU seconds; the 2 GiB build took 24.407 s and 73.971 CPU
seconds. This is a sequential budget experiment, not a statistical linker-speed
comparison. It demonstrates that increasing this local snapshot's budget does
not recover meaningful missing compilation. The 1 GiB policy remains unchanged.
The actual hosted exact-head zero-compiler result supports the same next action:
prioritize execution/setup CPU, rather than a larger compiler cache.

The already-committed workflow skips archive capture and save on exact primary
cache hits. No additional snapshot-copy optimization is added.

### Three submission hosts become one, with explicit sessions

`TestWorkSubmissionJourneys` owns one initialized API host. Its parallel groups
for HTTP batch/files, structured content and canonical batch content each open
and close their own Factory Session and own their Factory directory. The host's
default Factory is idle. Submission, staging, Work list/get, completion polling,
upsert identity and the CLI `submit` call all carry the selected session ID.
The original 13 customer cases and their assertion sets remain: ordered text,
header-only and rejected empty content, staged file/media metadata, forged
reference rejection, typed unknown-Work errors, canonical request/Work identity,
and CLI/REST behavior. Two fewer reported top-level results reflect grouping
three original tests under one parent; no customer case is removed.
The reviewed REST endpoint evidence is updated to the new parent and published
only after its parallel groups join successfully. The 160-scenario manifest
check passes. Other fixtures explicitly retain their existing default selector
while the shared helpers now also accept selected-session IDs.

A full controlled Linux inventory/coverage run passes **68 packages / 750
results**, 748 pass and 2 skip, without retry. Its 196.39-second elapsed and
186.374-second coverage command overlap independent Windows validation work;
these timings are **validation-only**, excluded from performance comparisons.
Aggregate CPU was 639.83 s, with 118 compiler and 24 linker invocations. The
final response helper was split after lint identified existing complexity at
the newly modified assertion boundary; HTTP 201, identity and content guarantees
are retained. Both scoped linters pass; native focused customer journeys pass
three repetitions and the existing support upsert check passes.

### Paired native execution measurement

Baseline and candidate test binaries were built separately against the same
runtime, with build time excluded from these execution numbers. Four CPUs were
pinned, GOMAXPROCS=4, GOGC=100. Runs used five repetitions each in the order
baseline, candidate, candidate, baseline; source/evidence files were restored
for the matching binary and the candidate was restored afterward. Every run
passed the same customer cases.

| Version / run order | Five-repetition wall | User + system CPU |
| --- | ---: | ---: |
| Original three hosts, first | 3.525 s | 5.201 s |
| Shared host, first | 4.507 s | 3.850 s |
| Shared host, second | 3.255 s | 2.565 s |
| Original three hosts, second | 3.134 s | 5.096 s |

Across ten repetitions per version, aggregate CPU falls from **10.298 s to
6.414 s**, approximately **37.7% less compute**. Aggregate wall rises from
6.660 s to 7.762 s; elapsed samples are variable and no latency win is claimed.
This is a native cell-level comparison, not a coverage-instrumented whole-CI
checkpoint. Fresh candidate hosted CI is required. The remaining priority is
consolidating compatible initialized-session fixtures in the largest measured
customer packages, without collapsing distinct persistence or recovery proofs.

## Remove remaining legacy and internal-only functional probes

Remove the legacy metadata mapper/serializer/clone/help-renderer probe and the
invalid programmatic snapshot argument probe from Product customer journeys.
They call internal operations rather than public customer boundaries. Existing
Factory snapshot, mapping, and CLI renderer component tests retain their owned
checks; current customer CLI invocation-help journeys remain in the functional
suite.

Remove the smoke probe that writes retired exhaustion_rules and calls the loader
directly. Keep the long customer guarded-loop routing journey. Remove the five
retired initialization command/flag cells; retain supported initialization,
invalid current input, atomic failure, and normal-command bootstrap journeys.
No customer behavior assertion is weakened and no production code changes.
This cleanup is not claimed as a measured latency improvement.

## Shared submission host hosted measurement and causal payload observation

At ce93d0ef0d, hosted job 112147854840 passes 68 packages / 750 results
(748 pass, two skip) without retries. It runs 06:57:17–07:01:06 UTC on
2026-10-06: **229s complete**, **175.035s coverage invocation**. Its archive
prefix restore is the prior 65ef head; 110 compiler commands and 21 linker
commands execute. This differs from the prior source and cache state and does
not establish a paired whole-job improvement. The two-minute checkpoint remains
unmet. Evidence: .artifacts/latency-audit/pr2923-submission-host-hosted.

Replace review-failure payload prompt polling with notification from the
controlled ProviderCommandRunner boundary. Each observed prompt closes a
mutex-protected notification channel and installs the next channel; observers
check existing prompts and capture the channel under the same lock. This avoids
lost wakeups and the fixed 50ms sampling delay. A real timeout remains as a
failure ceiling. Customer admission and payload assertions are unchanged.

### Incidental coverage audit for removed internal probes

The first removal run executes every selected case successfully (68 packages,
746 results, 744 pass / two skip) in 178.59s supervisor / 172.391s coverage,
470.57s user plus 114.79s kernel CPU. The overall gate fails on three package
floors; this is a failed validation, not a checkpoint.

The direct localized metadata calls had covered NameValue validation/resolution
(72.22% prior floor, 16/54 = 29.6296% after removal). Retained component proof is
TestNameValueValidationAndResolution, including exact locale fallback and
invalid locale handling, plus canonical JSON/YAML structured example roundtrip
checks. Customer CLI invocation help remains. The programmatically invalid
snapshot mapping call had covered the mapper error return (75.00% prior floor,
11/16 = 68.75% after removal); retained component proof is
TestObjectFromFactoryConfigRejectsUnrepresentableExampleArguments. These focused
component checks pass. That argument-shape error is rejected before customer
snapshot creation, so reintroducing an internal mapper call into functional
coverage would recreate the invalid test layer.

Reconcile only these two incidental functional floors to 29.62% and 68.75%,
respectively, under factory/docs/standards/testing-standards.md's explicit
requirement to audit retained guarantees and reconcile incidental coverage when
removing an invalid functional test. Other floors and remediation holds stay
unchanged. Do not report these floor changes as performance gains.

The third regression affects the public CLI config output handler. Its floor
stays 62.22%; extend the existing invalid-Factory CLI journey with current
flatten rejection and a closed customer output stream. Reuse its process and
use explicit input paths/working directories instead of process-wide Chdir.

Hosted lint on ce93 also finds three new generic support helpers in its
non-test deadcode inventory. Keep the session-specific observation helpers
local to Work admission test files instead of growing the support API or its
deadcode allowance. The existing support API returns to its previous shape.

### Final cleanup validation

The final full Linux supervisor passes **68 packages / 746 results**, 744 pass
and two skip, without retries. Coverage gates pass with only the two audited
incidental floors reconciled above. Supervisor wall is 178.66s and coverage
invocation is 170.727s. Concurrent native linter build/check work overlapped;
this is **validation-only**, not a comparable performance result or hosted
checkpoint. Evidence: .artifacts/latency-audit/legacy-customer-final.

Both scoped built-in and freshly rebuilt repository linters report zero issues.
The causal payload observer passes three focused repetitions. Current CLI
Factory validation/flatten errors pass, as do retained localized metadata,
canonical structured JSON/YAML examples, snapshot rejection and invocation-help
component proofs. Hosted CI still must validate the new source and deadcode
inventory. The last measured hosted source remains ce93 at 229s.

## Share lifecycle, dispatch and eligibility hosts

Three compatible customer journey groups now share one root-built host under
TestFactorySessionRuntimeJourneys. Each group retains its own command router,
scenario Factory directory, explicit Factory Session and causal gates. The
parent owns host shutdown after all parallel groups and their children finish;
group cleanup releases only its routes and sessions. Home, workflow storage,
Factory Session storage and model cache belong to this host. Remove redundant
bootstrap definitions and two separate process/start/stop implementations.
All 31 existing journey selectors and their nested cases remain.

Eligibility previously captured internal FactoryDispatchRecord values and
compared joined Work IDs, ticks and counts. Its existing public Factory Event
assertions already prove one joined dispatch with exactly the two requested
Work IDs and causal producer/relationship/join order. Keep those assertions and
remove the duplicate internal recorder. Validation rejects malformed definitions
through HTTP 400 with diagnostics and no live session; internal recorder counts
are unnecessary for those failures.

Four-CPU native paired execution (baseline, candidate, candidate, baseline),
three repetitions per command, all pass:

| Scope | Baseline CPU | Candidate CPU | Baseline wall | Candidate wall |
| --- | ---: | ---: | ---: | ---: |
| Lifecycle + dispatch, six repetitions each | 15.200s | 10.676s | 28.381s | 26.011s |
| Lifecycle + dispatch + eligibility, six repetitions each | 29.506s | 18.700s | 32.162s | 30.288s |

The final three-group candidate uses **36.6% less aggregate CPU** and **5.8%
less aggregate elapsed time** in this targeted comparison. Compilation is
excluded; no other local CPU-heavy validation overlapped these measurements.
This is not coverage-instrumented whole-job evidence. The two-group intermediate
also saved 29.8% CPU / 8.4% wall; it does not establish a stronger final gain.
Evidence: .artifacts/latency-audit/runtime-session-host-three-groups.

## Remove test-framework forced-cleanup subprocess

Remove the concurrency CC-14 cell which deliberately fails a subprocess of the
test executable and asserts harness process/port/root cleanup. It does not
invoke a customer CLI, REST or MCP operation. Retain customer capacity,
concurrency, exact-target cancellation, peer isolation, recovery, timeout,
idempotency and ordering scenarios and their test-owned cleanup. The causal
channel helper used by admitted-Work cancellation stays with the controlled
runner. The assertion ledger records the classification/removal.

Paired native execution, six repetitions per version, all pass. CPU falls from
**7.207s to 4.200s (41.7%)**, and wall from **9.629s to 8.275s (14.1%)**. No
local CPU-heavy validation overlaps the timed commands. Removing executable
self-inspection also permits this package to join the existing monolith through
its unchanged classifier; do not relax native-binary exclusions.
Evidence: .artifacts/latency-audit/concurrency-customer-only-paired.

## Hosted cleanup measurement at d3b757b530

Hosted job 112154415136 passes **746 results / 68 packages** in
07:18:51–07:22:39 UTC on 2026-10-06, **228s complete / 168.592s coverage**.
It restores the preceding ce93 compiler archive, executes 186 compiler commands
and 41 links, and records one same-head retry for
TestConcurrencySharedProcess/Cancel/CC-05. The first run returns HTTP 503 for
one concurrent exact-target cancel; retry passes. The customer assertion remains
in place. This is a real defect to investigate, not a no-retry checkpoint or
proof that adding a wait fixes cancellation. Both paired concurrency versions
above pass without retries, so they do not reproduce or resolve this defect.
The two-minute target remains unmet. The entire workflow, including hosted lint, subsequently passes.
Evidence: .artifacts/latency-audit/pr2923-customer-cleanup-hosted.

## Exact-head hosted cleanup rerun and full shared-host validation

The same d3 source rerun, job 112160465716, finishes successfully at
07:35:52–07:40:22 UTC on October 6: **270s complete / 176.798s coverage**.
It accounts for 68 packages / 746 results, 744 pass and two skip, with no raw
failures or retries. Despite an exact primary compiler archive hit, the coverage
invocation executes **363 compiler commands and 21 links**. Setup lasts about
72s before coverage starts; distinguish that overhead from test execution.
This result does not validate the unpublished shared runtime host. Evidence:
.artifacts/latency-audit/pr2923-customer-cleanup-warm-hosted.

The combined shared-host and concurrency-cleanup candidate passes the full
four-CPU Linux supervisor in **165.72s / 159.276s coverage**, 68 packages / 744
results, 742 pass and two skip, without retries. All existing coverage gates,
scenario decisions and quarantine checks remain enabled. Aggregate CPU is
**556.32s** (458.95s user / 97.37s kernel). Tooltimer records six compiler
commands and 23 supervisor-wide links, including tool builds; linker CPU totals
34.38s, with 29.65s union elapsed activity. Do not sum overlapping link elapsed
times into critical-path wall. No other local CPU-heavy work overlaps this run.
Evidence: .artifacts/latency-audit/shared-runtime-host-customer-only.

Factory definition flatten/expand now reuses one test-owned environment across
its three CLI calls instead of constructing three isolated homes for one
customer journey. Model storage remains owned by the test. Three focused Linux
repetitions pass; the scoped repository linter reports zero issues. The earlier
full 165.72s run precedes this final environment simplification.

The final source, including the flatten/expand environment simplification,
passes a second full four-CPU run: **185.36s supervisor / 178.845s coverage**,
68 packages / 744 results, 742 pass and two skip, no retries. CPU totals
**620.79s** (513.95s user / 106.84s kernel). It executes two compiler commands
and 23 supervisor-wide links, 34.94s linker CPU / 35.28s union linker activity.
The unchanged classifier now combines **35 packages / 549 original tests**,
with 19 native exclusions. The 160 reviewed scenario decisions remain current.
This full-run variability (165.72s versus 185.36s) prevents a claim of consistent
whole-job latency reduction from these targeted changes. Keep the paired
package measurements separate from complete-lane observations. Evidence:
.artifacts/latency-audit/shared-runtime-host-final.

## Customer-only inventory and owned JavaScript policy setup

Remove TestGatewayServiceInventoryExclusionAndFiltersPreservePeers and its
three service-only helpers from the functional lane. It calls StartSync and
ListSessions directly through the process service and tests status filters and
history exclusion unavailable through the customer REST inventory. Retain
TestGatewaySessionInventoriesPreserveIdentityAndHistory: CLI recording plus
REST live/history/persisted/all scopes, exact identities, stable ordering,
partial history failure and recovery. Existing component checks for listing
filters/normalization, recorded history exclusion, assembly source selection
and scoped merging pass in three service packages.

Paired native inventory execution (baseline/candidate/candidate/baseline,
three repetitions per command) passes. Aggregate CPU is **3.985s → 2.639s,
33.8% lower**; wall is **2.584s → 2.733s, 5.8% higher**. Removal saves compute,
not measured elapsed time in this small comparison. Evidence:
.artifacts/latency-audit/gateway-customer-only-paired.

The CLI JavaScript policy fixture now owns one home/model cache for its three
sequential invocations, preserving fresh Factory directories, request IDs and
Factory Session IDs. These calls do not run concurrently, so mutable first-run
installation is never shared across concurrent invocations. Inject owned
Factory Session storage/home and workflow storage. Remove unused host workflow,
API server startup/stream counters, hosted process machinery and synthesized
resource reports; neither retained policy scenario starts that server. Keep
the no-provider-dispatch assertion and stable public failure diagnostics.

Six native repetitions per version pass: **5.062s → 2.921s CPU (42.3% lower)**
and **22.771s → 20.792s wall (8.7% lower)**. This comparison precedes the
subscription ownership fix below and excludes compilation. Evidence:
.artifacts/latency-audit/policy-owned-home-paired.

## Release final-history subscription before runtime teardown

A block profile of the policy CLI journey identifies about one second per
invocation in FactoryEventHistory.CloseLiveSubscriptions. The final-history
presentation read opens a live subscription using its caller context, reads
History, then leaves that subscription active. Subsequent runtime termination
waits for the one-second bounded drain deadline because the read has no live
consumer. This is avoidable subscription lifetime, not customer execution time.

readFactoryEventHistory now derives and cancels its own context on return for
both default live history and durable session history. Clone/present all history
before releasing it. Preserve the real subscriber drain deadline and queued
terminal-event delivery. The focused bridge regression fails in both live and
durable cases before the fix, then passes after it, checking history release,
caller context preservation, exact presented event IDs and no duplicates. All
Factory Sessions wire and Recordings event component tests pass, including
queued terminal delivery and unread-subscriber bounding. Scoped built-in and
repository linters report zero issues.

A separate paired comparison keeps the simplified policy fixture identical
and changes only the history-read fix: six repetitions per version pass with
**20.713s → 2.631s wall (87.3% lower)** and **2.857s → 2.437s CPU (14.7% lower)**.
No other local CPU-heavy validation overlaps the pairs. Do not add this gain
to other percentages or extrapolate it to whole CI. Evidence:
.artifacts/latency-audit/policy-event-history-release-paired.

## Hosted shared-runtime source at d5d14fe082

Hosted functional job 112164923707 passes at 07:51:16–07:55:15 UTC on October 6:
**239s complete / 173.206s coverage**, 68 packages / 744 results, 742 pass and
two skip, no retries. It restores the preceding d3 compiler archive, executes
369 compiler commands and **20 links**. Backend lint also passes. Archive
capture reaches 1,073,730,377 bytes and omits 172 eligible files; a prefix restore
is not a warm build. These measurements precede the new inventory/policy/history
changes. The complete two-minute checkpoint remains unmet. Evidence:
.artifacts/latency-audit/pr2923-runtime-host-hosted.

### Combined history-release candidate validation

The full four-CPU Linux supervisor passes **68 packages / 743 results**, 741
pass and two skip, no failures or retries, in **171.33s complete / 165.362s
coverage**. All existing coverage floors and holds pass unchanged; quarantine
and 160 reviewed scenario decisions remain enabled/current. Aggregate CPU is
**581.48s** (490.38s user / 91.10s kernel). Seven compiler commands and 23
supervisor-wide links consume 7.375s compiler CPU and 31.429s linker CPU;
linker union elapsed activity is 31.365s. No other local CPU-heavy validation
runs concurrently. This is changed-source local evidence; do not infer a
hosted two-minute checkpoint or claim a matched whole-lane speedup. Evidence:
.artifacts/latency-audit/policy-history-release-full.

## Named Factory and session CLI fixture simplification

Named Factory lifecycle and remote session commands have identical immutable
process wiring. One parent TestFactoryAndSessionCLIJourneys now owns the process
until both parallel groups join. Each scenario keeps its own home, working
directory, HTTP target and command buffers; every current create/list/update/
delete, packaged-install, profile-isolation, missing-home/help and selected-
session command assertion remains. Rename the misleading composition-routing
cell to SessionCommandsPreserveSelectedTargets. Remove only the standalone
legacy submit --port rejection and the deprecated session-show --port tail;
retain supported session-create port selection and default-session failure.

Six native repetitions per version pass. CPU **4.132s → 4.228s** and wall
**1.663s → 1.673s** show no measured performance gain. This is structural setup
simplification and retired-flag cleanup, not a claimed latency optimization.
Both scoped linters pass. Full Linux coverage passes 68 packages / 742 results,
740 pass and two skip, no retries, in **222.48s supervisor / 214.107s coverage**,
680.83s CPU (569.15 user / 111.68 kernel). Other WSL integration Go/Git work is
observed concurrently; this whole-lane result is validation-only. The coordinator
combines 35 packages / 547 top-level tests and preserves all native exclusions.
Evidence: .artifacts/latency-audit/factory-session-cli-host-full and
.artifacts/latency-audit/factory-session-cli-host-paired.

Concurrent-cancellation investigation repeats the exact public CC-05 selector
50 times using JSON results and verifies **50 leaf passes / zero failures**.
This does not reproduce or fix the earlier hosted HTTP503; its customer assertion
remains unchanged. An initial shell wrapper printed the passing Go result but
failed to propagate its status; the corrected script records and verifies each
selected leaf result explicitly. Evidence in private Linux:
.artifacts/cc05-cancellation-stress.jsonl.

## Hosted final-history release at e02e408d02

Hosted job 112169220807 passes at 08:01:38–08:05:39 UTC on October 6:
**241s complete / 169.801s coverage**, 68 packages / 743 results, 741 pass and
two skip, no retries. It restores d5 compiler archives, executes **seven
compiler commands and 20 links**. Entire workflow subsequently succeeds.
This prefix archive recovers most compilation; execution and supervisor overhead
now dominate this sample. Steps before the coverage supervisor total about
33s. The coverage supervisor step lasts 189s, including tool startup and
quarantine work beyond the measured 169.801s coverage invocation. Post-coverage
capture/save/report/cleanup occupies the remaining roughly 19s. The complete
two-minute target remains unmet. Evidence:
.artifacts/latency-audit/pr2923-history-release-hosted.

A private scheduling experiment now removes only coordinator-level package
parallelism while preserving each original scenario's t.Parallel and the eight
native build/run jobs. The current all-overlapping coordinator is observed at
about 15.5 GiB resident memory late in execution; test-owned hosts remain alive
while nested scenario children wait for shared Go test slots. This is a
measurement hypothesis, not proof that sequential package groups improve CPU
or elapsed time. Coverage, inventory, quarantine and native exclusions remain
enabled for the experiment. Canonical generator source is unchanged pending
evidence; the private source is restored after the experiment finishes.

### Serialized-package experiment: rejected

The private serialized-group variant passes all 68 packages / 742 results,
740 pass and two skip, no retries, but takes **258.99s supervisor / 252.957s
coverage** and **705.73s CPU** (603.47 user / 102.26 kernel). Peak process RSS is
17,540,296 KiB. Twenty-three supervisor links use 32.652s CPU and 34.100s union
elapsed activity. This does not justify changing default package scheduling;
the canonical generator remains unchanged, and the private generator is verified
byte-identical to it after restoration. Evidence:
.artifacts/latency-audit/serial-functional-groups-full.

A separate diagnostic-only repeat adds process CPU counters around each serial
package epoch and writes a post-GC heap profile after all groups join. Original
scenario parallelism, inventory and coverage gates remain enabled. Its purpose
is attribution of retained memory and package CPU, not a candidate CI schedule.

### Recording recovery attribution and default functional process isolation

The diagnostic serialized-group run passed customer and coverage gates, but its
post-GC heap retained **9,157.61 MB**. Pprof attributes **7,321.77 MB (79.95%)
cumulative** to Worker capture `FileWriter.hydrate`, **7,422.64 MB (81.05%)** to
`Local.ScanDirectory`, and **7,177.47 MB (78.38%)** to `recoverRecordingOwner`.
These nested values overlap; do not sum them. Flat allocations include
3,151.76 MB in `events.Record.Detached`, 1,916.09 MB in
`recordingSession.acceptRecord`, and 1,370.77 MB in JSON literal decoding.
This is retained memory, **not a CPU profile or hosted memory measurement**.

Construction resolves Worker recordings through
`FactorySessionsWorkingDirectory.Getwd()`, before CLI inputs provide Cwd/HOME.
The functional helper previously retained the OS working directory. The private
workspace contains **68 MB** of old Worker journals under
`pkg/monolithpilot/.you-agent-factory/worker-recordings`, plus package-local
journals. Many independent hosts repeatedly hydrate that store. Repeated private
measurements amplify history; a fresh hosted job does not start with it, but
can generate and reread peer journals within one suite. Do not extrapolate the
entire private heap reduction to hosted CI.

The pending helper supplies a unique real temporary working directory when a
scenario has not supplied `FactorySessionsWorkingDirectory`. Real production
composition, storage, recovery and public readers are retained. Explicit paths
and writers are preserved for persistence/restart scenarios. The process removes
its owned directory after successful Close and on construction failure. A Close
failure retains it while resources may still be live. Invocation Cwd/HOME remain
explicit. Existing shared journals are left untouched.

The earlier diagnostic intended to collect package CPU deltas, but the generated
counter only supported Windows; Linux returned unsupported, so **no package CPU
measurements were produced**. The pending generator adds Linux Getrusage.
Parallel group deltas include peers and must not be summed or presented as
package CPU attribution. No sequential scheduling change is proposed.
Rejected serialized repeats passed at 258.99s / 705.73s CPU and, with heap
diagnostics, 278.75s / 781.13s CPU. Diagnostic overhead prevents using the
latter as a candidate timing checkpoint.

The owned-store candidate passes the complete four-CPU Linux supervisor at
**143.70s overall / 137.174s coverage**, **470.78s aggregate CPU** (384.16 user /
86.62 kernel), peak process RSS **6,447,928 KiB**. It retains 68 selected packages
and 742 results (740 pass / two skip), no retries, all existing floors and holds.
This changed source compiles 119 commands (51.887s CPU) and links 23 commands
(37.138s CPU / 36.200s union active wall). Compared with published e02's local
171.33s / 581.48s CPU, it is a promising sample; these are not controlled paired
runs and compile state differs. The earlier pending CLI-only run was also
contended, so its 222.48s sample is validation rather than a baseline speedup.
Artifacts: `.artifacts/latency-audit/owned-recording-store-full`.

Unitlane and coverage supervisor component suites pass. The generated Linux CPU
counter is compiled by full verification; an isolated test of that exact generated
source verifies Getrusage advances (0.008560 CPU seconds). The canonical manifest
check confirms all 160 reviewed scenarios remain current. The helper's owned
store does not replace real customer recording behavior with a mock.

### Published owned-store hosted result and further compatible consolidation

Published `b1d1087428` passes hosted Functional Coverage job 112182439929 in
**222s complete / 170.789s coverage**, 68 packages / 742 results, 740 pass and
two skip, no test failures/retries, 111 compiler commands and 20 links after a prefix e02 archive restore. The preceding e02 coverage interval was
169.801s: **execution latency is essentially unchanged in these hosted samples**.
Do not attribute the 19s complete-job difference entirely to store isolation.
Before-supervisor overhead is 27s and post-supervisor overhead 15s; the
supervisor step takes 180s. The complete workflow, including Backend lint, subsequently passes.
Artifacts: `.artifacts/latency-audit/pr2923-owned-recording-hosted`.

A private cleanup-hook experiment on five native suites exposed additional
native constraints: executable fixtures (Providers, ACP chat, Codex), test-level
quarantine (AGY), and relative-path strings (Docs smoke). No classifier or
quarantine policy was weakened. The first diagnostic source accidentally
changed UTF-8 punctuation while editing on Windows; its full run failed a
Providers docs marker and is invalid for performance conclusions. All five
files were restored byte-for-byte from the published head before the corrected
changes. The unsupported hooks were discarded, not shipped.

Docs smoke now retains current CLI headings, content markers, topic index,
alias behavior, embedding prerequisite guidance and unknown-topic diagnostics.
It removes 19 legacy absence tables, retired topic rejection checks, obsolete
executable-name regex checks, removed duplicate-tree path checks and old goal
topology checks. These retirement assertions were a relative-path false
positive in the existing classifier. The same owned cleanup finalizer runs in
native TestMain and the existing monolith package cleanup hook, after children
join. Native Docs smoke passes. Full verification passes **36 merged groups /
551 top-level registrations**, 68 packages / 742 results, no retries, unchanged
gates, at **127.01s supervisor / 121.423s coverage**, **409.71s CPU**. Four
compiler actions use 0.287s CPU; 22 supervisor links use 38.608s CPU / 34.135s
union active wall. Warmer compilation explains part of the difference from the
prior 143.70s sample; one removed binary does not explain the entire gain.

A further pending parent consolidates Named Factories, Session commands,
authored JSON/YAML parity and validation/persistence onto one process, retaining
all scenario leaves. Its provider router preserves the existing command-scoped
validation observation and session-specific authored-source routes. Validation
still rejects API startup; authored runtime calls retain their owned server.
Duplicated initialization/reset/teardown helpers are removed. Three focused
repetitions and six paired repetitions per source pass. CPU **17.662s baseline
versus 18.944s candidate** and wall **7.987s versus 8.187s** are neutral to
slightly worse, not a speedup claim. The change is setup simplification; the
full lane is being validated before publication. Baseline has three processes
for four journey families; the artifact folder's historical four-host name
counts families, not process instances.

The final compatible-CLI source passes full verification at **129.30s supervisor /
123.035s coverage**, **398.02s CPU** (319.90 user / 78.12 kernel), peak process
RSS 6,501,336 KiB. It retains **68 packages / 740 results**, 738 pass and two
skip, no failures/retries, 36 monolith groups / 549 top-level registrations.
The two fewer top-level results are removed journey wrappers, not customer
scenario leaves. Five compiler commands use 2.883s CPU and 23 supervisor links
use 33.304s CPU / 32.127s union active wall. The previous Docs-only full sample
was 127.01s / 409.71s CPU; neither sample proves a wall-time gain from sharing
two more process constructors. Source simplicity and reduced total fixture
graphs justify this cleanup, while the next performance work must address
repeated initialization rather than treating constructor merging as sufficient.
All 160 canonical decisions remain current. Artifact directory:
`.artifacts/latency-audit/factory-cli-four-host-full`.

### Refreshed published-source CPU attribution and CI blocker

Profiling `21bde1452a` passes all 68 packages / 740 results with no retries,
unchanged gates, at 125.94s supervisor / 119.874s coverage and 404.83s total CPU.
This diagnostic adds CPU profiles and goroutine labels, so it is not a hosted
checkpoint. Monolith duration is 86.05s with 272.45s sampled CPU; 246.89s (90.62%)
is package-labelled. InitializeSystem accounts for 161.64s cumulative, including
161.21s packaged installation, 99.19s managed-layout preparation and 74.59s
FactoryConfigMapper expansion. JSON unmarshalling is 107.83s cumulative and
Syscall6 is 43.40s flat. These costs overlap. Filtered initialization labels
identify customer journeys (38.86s), customer lifecycles (16.98s), MCP worker
sessions (12.69s), CLI/REST journeys (12.55s) and Factory execution (11.75s).
The diagnostic generator is restored byte-for-byte afterward. Artifact:
`.artifacts/latency-audit/owned-source-current-cpu`.

Hosted source 21 passes Functional Coverage job 112188112819 in **146s complete /
102.318s coverage**, 68 packages / 740 results, no retries, six compilers and 19
links following a prefix b1 archive restore. Source b1's 111 compile commands
versus six here materially affects the comparison. The two-minute complete job
checkpoint is still unmet. Artifact:
`.artifacts/latency-audit/pr2923-compatible-cli-hosted`.

The same workflow's API Contract And Package job 112188113716 fails in
`TestDefaultRegistryValidFixtures`: canonical runtime-api.json is momentarily
invalid JSON. Contract staging generation rewrites that authored projection;
its repository-mutating tests already acquire LockRepositoryStagingForTest.
The validator's repository-root helper did not acquire the lock. The pending
correction acquires the existing cross-process lock for canonical repository
reads and releases it at test cleanup. No validation assertion is weakened or
retry added. A contract-suite repetition is validating this merge unblocker.

All contract tool packages pass three repetitions with the reader lock, including
contractstaging and contractvalidator running as separate concurrent binaries.
The API blocker correction changes test ownership only; no functional-runtime
behavior, source contract or coverage floor changes.

Both scoped linters pass for the contract reader correction. This corrective
push is authorized to unblock CI; the previous head's Backend Lint job was
still running when the completed API failure was corrected. No latency checkpoint
is inferred from the correction and the PR remains draft.


### Owned MCP client home: measured package improvement

The shared remote-client process previously allocated a new HOME for each MCP
connection, repeatedly installing packaged Factories. It now completes public
`you init --provider codex` once before parallel children, then binds all client
commands to the parent's owned HOME/USERPROFILE and model cache. Each connection
retains its own context, pipes and working directory. Real server hosts retain
their independent stores, sessions, persistence and restart paths. Selected-host
routing, cancellation, failure, lineage and public response assertions remain.

Controlled native execution compares baseline/candidate in B3/C3/C3/B3 order,
six repetitions per version on four CPUs, excluding build/link time:

| Version | Total execution CPU | Total elapsed |
| --- | ---: | ---: |
| Baseline | 104.943s | 36.716s |
| Owned client home | 60.330s | 24.204s |
| Reduction | 42.5% | 34.1% |

All repetitions pass. The final canonical full lane passes 68 packages / 740
results (738 passes, two skips), no retries, unchanged coverage floors and
quarantine gates. Supervisor 143.62s; coverage 136.618s; CPU 453.12s (360.31 user,
92.81 system); maximum RSS 6,606,656 KiB. Four compiler commands consume 0.993s
CPU. Twenty-two supervisor links consume 32.198s CPU and 29.095s union active
wall; their overlapping elapsed durations must not be summed into total latency.
The earlier local source sample was 129.30s / 398.02s CPU. Both are retained;
full-lane variability prevents inferring a complete-job gain from this sample.
Both scoped linters pass and all 160 reviewed scenario decisions remain current.
Artifacts: `.artifacts/latency-audit/mcp-client-home-paired` and
`.artifacts/latency-audit/mcp-client-home-full`.

The corrective contract-reader head `deec6a6f0f` passes its entire hosted workflow
37440563162, including API job 112193456824. Functional job 112193454540 takes
198s complete (09:06:46–09:10:04 UTC), 142.217s coverage, zero compiler commands
and 19 links after a prefix 21 archive restore. Source 21's 146s complete sample
used six compilers; neither warming nor the component-test-only correction
explains away the slower valid sample. The observed runtime-identical hosted
range is 146–198s. Two-minute merge remains gated on the complete hosted job,
not just coverage execution. Artifact:
`.artifacts/latency-audit/pr2923-contract-reader-hosted`.


### Hosted MCP source: recovered interrupt race retained

Functional job 112198276041 (workflow 37442087379, source `78153f7b20`) succeeds
in **207s complete**, 09:19:40–09:23:07 UTC, with **153.481s coverage**, all
68 packages / 740 final results. Eight compiler commands and 37 links are
reported after a prefix deec archive restore. One initial `TestInterruptRace`
failure recovers on the enabled exact-test retry. The CLI caller returned
`WORKER_SESSION_INTERRUPT_ADMISSION_FAILED` during the simultaneous CLI/HTTP
interrupt journey, where both identical requests must return the same accepted
successor. This is a customer guarantee, so retain the test and raw failure;
do not remove it or loosen its assertion for speed. This run cannot establish
a no-retry checkpoint or a complete-job gain from the focused MCP reduction.
The complete workflow finishes successfully, including Backend Lint.
Artifact: `.artifacts/latency-audit/pr2923-mcp-client-home-hosted`.

### Factory Event session-host experiments

Six compatible retained-history, cursor, trace and stream cases previously
started independent hosts, several using the default session. The first
candidate keeps mock-worker and controlled-provider cases on two shared hosts,
with one explicit session per case. All six repetitions per version pass, but
baseline execution uses **49.204s CPU / 26.773s wall** versus candidate
**51.162s CPU / 42.181s wall**. Reject that two-host performance candidate.
Artifact: `.artifacts/latency-audit/events-session-host-paired`.

The next candidate uses one controlled-provider host for these public event
assertions; they do not require a particular worker implementation. Each case
owns its explicit Factory Session, authored directory, submitted Work and event
cursor. Mock-dependent topology edits, initial CLI/recorded sessions and webhook
fault cases retain their existing dedicated fixtures. All current ordering,
no-gap/no-duplicate, typed cursor error/recovery, trace propagation and stream
termination assertions remain. Parent host cleanup follows child session cleanup.

Six native repetitions per version in B3/C3/C3/B3 order pass. Baseline execution
uses **48.870s CPU / 25.896s wall**; candidate **38.389s CPU / 24.070s wall**,
**21.4% less CPU / 7.1% less wall**. This excludes compilation/linking and does
not predict hosted total. Both scoped linters pass and all 160 reviewed scenario
decisions remain current. Full canonical coverage validation passes.
Artifact: `.artifacts/latency-audit/events-single-host-paired`.


The single-host event candidate's complete local lane passes all 68 packages /
740 results (738 passes, two skips), no retries, unchanged coverage gates,
quarantine selectors, 36 monolith groups and 549 registrations. Supervisor
**150.57s**, coverage **143.117s**, aggregate CPU **493.18s** (397.63 user,
95.55 system), maximum RSS 6,815,132 KiB. The shared support source change
requires **115 compiler commands / 51.132s CPU / 28.410s union active wall**;
22 supervisor links consume **36.041s CPU / 34.968s union active wall**.
The preceding MCP-only local run had four compiler commands / 0.993s CPU.
Record these complete samples without claiming a whole-lane speedup from the
controlled event comparison. Artifact:
`.artifacts/latency-audit/events-single-host-full`.

Remaining native-binary exclusions are retained: executable/helper-process,
process-wide environment/Cwd, relative fixtures and unsupported TestMain
semantics still require native registration. The safe Docs join is already
included. Do not weaken the classifier merely to reduce link count. Existing
selected-time automation journeys already advance injected clocks only after
registration/readiness acknowledgements; their wall deadlines bound failure
rather than successful completion. Network/filesystem observation is not made
virtual by blindly wrapping it in synctest.


### Finite worker CLI home and additional native-package joins

Finite worker CLI lifecycle journeys now initialize one parent-owned home
through public `you init --provider codex` before parallel children. Every run
retains its explicit UUID Factory Session, authored Factory, Cwd, streams and
provider-command route. Hosted adverse scenarios keep their independent homes.
The environment helper replaces inherited model-cache overrides with a cache
under each owned home. Parent teardown precedes removal of the shared home.

Six native repetitions per version in B3/C3/C3/B3 order pass. Baseline uses
**33.537s CPU / 14.627s wall**, candidate **22.778s CPU / 12.921s wall**:
**32.1% less CPU / 11.7% less wall**. Artifact:
`.artifacts/latency-audit/worker-cli-home-paired`.

Remove `TestInvokeContinueForcedAssertionCleansOwnedResources` and its report
helpers: it launches an intentionally failing test executable and examines
framework cleanup counters rather than a customer operation. Retain actual
CLI/HTTP invoke, continue, live stream, persistence, peer-session and interrupt
assertions, including TestInterruptRace. Reuse native TestMain's existing close
routine as FunctionalMonolithCleanup after children, allowing invoke/continue
to join without weakening the classifier.

The initial `-test.count=3` comparison is invalid: the baseline's package-wide
fixture reuses request identities across repeated m.Run iterations and fails
with request-ID conflicts. It is not a candidate regression or a performance
sample. Correct measurement launches a fresh native process for each repetition,
with the canonical parallelism of eight. Six fresh processes per version pass:
baseline **15.930s CPU / 15.432s wall**, candidate **13.069s CPU / 12.470s wall**,
**18.0% less CPU / 19.2% less wall**. This excludes build/link time. Artifacts:
`.artifacts/invoke-continue-probe-paired` (private invalid attempt) and
`.artifacts/latency-audit/invoke-continue-fresh-process-paired` (valid comparison).

Work watch's process/profile setup becomes lazy and shared. Both native TestMain
and FunctionalMonolithCleanup close the same selected-clock/legacy-clock
processes after all children and then remove their owned profile root. Source-
relative recorded-ledger fixture lookup remains unchanged and passes in the
monolith. Remove a context-helper-only top-level probe; all customer CLI watch
stream, child-deadline, cancellation, reconnect and retained-history cases remain.
The existing real teardown timeout has a justified failure-ceiling comment;
remove its single obsolete TestMain timeout baseline entry. No replacement debt
is added. The rebuilt repository linter and focused analyzer tests pass.

The first complete candidate passes in **140.95s supervisor / 134.554s coverage**,
**451.29s CPU** (359.65 user / 91.64 system), RSS 6,639,120 KiB. Eight compilers
consume 7.043s CPU; **20 supervisor links** consume 31.827s CPU / 31.046s union
active wall. All 738 results, 736 passes/two skips, no retries, unchanged gates.
Two removed framework-only results explain the count change. Compared with
22 supervisor links before the joins, two separate binaries are removed.
Artifact: `.artifacts/latency-audit/worker-home-and-native-joins-full`.

### Hosted event source and corrective cursor-helper reuse

Source f0's functional job 112202027678 succeeds in **220s complete**,
09:29:41–09:33:21 UTC, **167.250s coverage**, all 740 results, no retries,
110 compiler commands and 19 links after a prefix 781 archive restore. Artifact:
`.artifacts/latency-audit/pr2923-events-single-host-hosted`.

The final workflow fails Backend Lint and Frontend Browser. Deadcode's 2530
reported findings differ from its 2528 inherited baseline by exactly the two
new exported scoped-cursor support helpers. Correction removes the duplicate
file and makes the existing unused cursor-error/recovery helpers require the
explicit session ID. All callers use the same public session-scoped endpoint;
no baseline allowance is widened. The UI failure waits ten seconds for Add
workstation; retain the customer UI assertion and diagnose recurrence on the
next head. Failure artifacts:
`.artifacts/latency-audit/pr2923-events-single-host-lint` and
`.artifacts/latency-audit/pr2923-events-single-host-ci-failure.log`.

The final corrected source passes full canonical coverage in **204.98s supervisor
/ 197.160s coverage**, **688.92 CPU-seconds** (538.75 user / 150.17 system),
RSS 6,648,936 KiB, all 68 packages / 738 results, no retries. The shared support
source change incurs **117 compilers / 71.866s CPU / 35.851s union active wall**;
20 supervisor links consume **45.410s CPU / 48.517s union active wall**. Preserve
this slower valid sample. Compilation alone does not account for all additional
CPU, so no complete-lane improvement is claimed. Both scoped linters and all
160 reviewed decisions pass. Artifact:
`.artifacts/latency-audit/worker-home-and-native-joins-final`.
## Refreshed CPU attribution after worker home and native joins

The current corrected candidate passes the complete profiled lane with no
retry. This diagnostic takes 162.52 seconds and 516.17 CPU-seconds; it is not
a hosted checkpoint or a comparable uninstrumented timing sample. The
monolith profile samples 351.78 CPU-seconds over 113.08 seconds. Initialization
accounts for 204.73 cumulative CPU-seconds (58.20%), and packaged Factory
installation accounts for 204.10 (58.02%). These overlapping stacks must not
be summed. Two compiler commands consume 0.225 CPU-seconds; 20 supervisor
links consume 35.120 CPU-seconds across 34.554 seconds of active wall time.

| Labelled functional package | Sampled CPU seconds | Initialization CPU seconds |
| --- | ---: | ---: |
| product/customer_journeys | 79.29 | 63.37 |
| product/customer_lifecycles | 58.73 | 19.88 |
| factory/execution | 25.45 | 13.80 |
| product/cli_rest_journeys | 19.90 | 13.48 |
| transport/cli/customer_commands | 13.80 | 9.99 |
| models/inference | 13.76 | 8.80 |
| work/admission | 11.71 | 8.84 |
| transport/mcp/worker_sessions | 10.67 | 8.72 |

Package labels propagate into fixture background goroutines and cover 89.76%
of sampled monolith CPU. They identify the next inspection targets, rather
than establish isolated per-package regressions against older profiles.
Customer journeys' repeated initialization is the largest remaining measured
target. Preserve independently owned persistence/restart paths and explicitly
isolated session routes when sharing setup. The review failure/recovery journey
alone accounts for 30.79 sampled CPU-seconds, including actual session opening
and Work admission that cannot be removed as fixture overhead.

Raw diagnostic evidence: `.artifacts/latency-audit/worker-joined-source-cpu/`.

## Packaged Factory preparation cache experiment and causal cancellation

The existing installer memoizes publication fingerprints by install root,
Factory name, authored root filename and payload digest. New homes still
perform preparation and actual materialization. The current monolith profile
attributes 121.45 cumulative sampled CPU-seconds (34.52%) to
PreparePackagedFactoryLayout; generated-boundary mapping accounts for 91.97
(26.14%). These costs overlap initialization and JSON decoding; do not sum
them or promise equivalent elapsed-time savings.

A bounded, persistence-service-owned prototype cached validated prepared
layouts by Factory name and payload SHA-256, cloning configuration and
canonical bytes before use. It retained fresh filesystem reconciliation,
atomic writes and customer-edit handling. Six repetitions per version of the
native CLI process journey, in baseline/candidate/candidate/baseline blocks,
all pass. Baseline consumes 29.892559 CPU-seconds and 15.823867484 seconds wall;
candidate consumes 30.900592 CPU-seconds and 16.974246406 seconds wall. User CPU
falls from 22.939407 to 19.661966, but kernel CPU rises from 6.953152 to
11.238626. Total CPU is 3.4% higher and wall is 7.3% higher. Reject and remove
this prototype; the measurement does not establish that pre-rendered caching
is ineffective. It measures only this service-scoped cloned-layout design in
this customer group, without coverage instrumentation.

A separate initialized-home experiment for the two CLI worker-outcome cases
also passes six repetitions per version but raises CPU from 32.131414 to
34.530866 and wall from 16.481528131 to 17.739607701. Remove it. Help/version
and validation cases retain their clean homes and filesystem-effect assertions.

The next cache design should reuse immutable generated, pre-rendered packaged
files across roots, keyed by packaged payload digest and rendering policy.
Materialize them into each owned home through the existing filesystem boundary,
and keep current stamp/customer-edit detection, refresh backups, cancellation
and atomic publication. Validate identical JSON/YAML, Worker/Workstation
prompts, portable files and inbox sentinels, payload invalidation, detached
session state and cross-process ownership before claiming a gain. This design
is a follow-up hypothesis; no pre-rendered production cache is shipped here.

Replace the partial-result provider's 5ms cancellation polling loop with the
provider's own cancellation acknowledgement channel. Keep the existing 5s
failure ceiling and the customer HTTP interrupt/durable-status assertions.
Three focused native journey repetitions pass in 1.178 seconds. Remove the
exact now-stale sleep baseline entry rather than add any allowance.

Hosted current consolidation head e243441e58 passes functional CI in 247s
complete (09:58:31–10:02:38 UTC), with 169.073s coverage, 68 packages and 738
final results. Diagnostics record 110 compiler commands and 17 links, compared
with the preceding head's 19 hosted links. The complete hosted two-minute
checkpoint remains unmet. Its previously failing Add workstation browser
journey passes on this run without a UI change. Backend lint remains pending
at the initial observation. Its workflow subsequently completes successfully,
including Backend Lint, confirming the duplicate helper correction.

Raw evidence: `.artifacts/latency-audit/packaged-preparation-cache-paired/`,
`.artifacts/latency-audit/cli-outcome-home-paired/` and
`.artifacts/latency-audit/pr2923-worker-home-joins-hosted/`.

## Rendered-file prototype: full functional proof and matched timing

A private Linux prototype captures the current installer's materialized output
for 20 embedded packaged Factories. It reuses file bytes across process
instances while creating every case's fresh home and retaining staged layout
validation, ownership leases, stamps, customer-edit reconciliation, refresh
backups and atomic publication. The production tree does not contain this
prototype. Its snapshot input is a private audit artifact, and custom writer
and validation-dependency selection is not yet suitable for production.

Initial JSON-only snapshots pass six native repetitions per version of the
invocation/result and dispatch-usage customer groups: baseline consumes
14.776348 CPU-seconds / 14.493914341 seconds wall, candidate 7.398516 /
9.223422374. The full lane then correctly fails three YAML/YML customer leaves
because the requested roots are missing. Preserve this failure; do not claim
the narrow passing comparison establishes a complete optimization.

Corrected snapshots come from real public installs in JSON, YAML and YML: 60
format variants, 588 files, 1,822,949 bytes in the serialized snapshot artifact.
Capture permissions from Linux, not Windows UNC stat results. Preserve actual
directory and file modes and select the requested root filename. The unchanged
CLI format, validation, customer-edit preservation and explicit replacement
assertions pass. Six native repetitions per version, now including packaged
YAML portability, all pass: baseline consumes 27.578133 CPU-seconds /
29.166335468 seconds wall; candidate 11.599016 / 16.946829844. This is 57.9%
less CPU and 41.9% less wall for these selected customer groups.

The corrected prototype passes two complete, unweakened functional coverage
runs: all 68 packages / 738 results, 736 pass and two skip, without retries.
The first takes 139.07s supervisor / 129.857s coverage / 429.04 CPU-seconds,
with zero compiler commands and 20 supervisor links. This is not a hosted
checkpoint and is not directly comparable with earlier changed-source runs.

For a stronger comparison, run baseline then corrected prototype in one
continuous Linux session with the same four CPUs, jobs=8, GOGC=100, coverage
manifest and quarantine. Both complete without retries. Baseline takes
181.21s supervisor / 167.779s coverage / 564.70 CPU-seconds; prototype takes
145.74s / 134.493s / 452.47 CPU-seconds. Total CPU falls 19.9% and supervisor
wall 19.6% in this one matched pair. Baseline has three compiler commands
(7.513292 CPU-seconds); prototype has zero. Both have 20 links, but prototype
link CPU rises from 39.759837 to 64.912419 and active link wall from 37.695s to
46.225s. Preserve that variation and do not equate this pair with a stable
hosted speedup. The private source is restored to canonical after comparison.

The production implementation should generate immutable format-specific
renderings through the canonical compiler/writer, bind them to exact packaged
payload and rendering policy, and load them through the owning Factory
Definitions composition. Replay through the authored writer's filesystem
boundary rather than the prototype's persistence-filesystem shortcut. Preserve
portable-file effects and custom dependency behavior, cancellation, file modes,
fresh runtime/session state and final staged validation. Keep arbitrary or
changed payloads on the existing preparation path. The prototype proves useful
potential; it does not prove that bypassing arbitrary supplied validation or
writer ports is correct.

Published cancellation-cleanup head f7e138ae4e passes hosted functional CI in
222s complete / 154.403s coverage with six compiler commands and 33 links. One
TestAgentSharedProcess/Cancel failure recovers on the existing same-head retry;
retain its flake ledger and do not claim it is fixed. Workflow fails Backend
Integration when a released probe port is claimed before the browser-test
server binds. The successful browser cases now use server-owned automatic port
binding and derive the observed endpoint from the public dashboard readiness
line. The deliberately occupied explicit-listen failure remains. All launcher
suppression, concurrent endpoint distinction and post-stop port-reuse assertions
remain. Three focused Linux repetitions pass in 11.454s; both scoped linters
pass. No timeout or retry allowance is increased.

Evidence: `.artifacts/latency-audit/rendered-layout-spike-paired/`,
`.artifacts/latency-audit/rendered-layout-spike-full/` (failed first prototype),
`.artifacts/latency-audit/rendered-layout-formats-spike-paired/`,
`.artifacts/latency-audit/rendered-layout-formats-spike-full/`,
`.artifacts/latency-audit/rendered-layout-formats-matched-baseline/`,
`.artifacts/latency-audit/rendered-layout-formats-matched-candidate/`,
`.artifacts/latency-audit/pr2923-causal-hosted/` and
`.artifacts/latency-audit/pr2923-causal-flakes/`.


## Agent customer cleanup and one additional shared-binary package

Remove the Agent package's constructor-only provider inventory child and its
deliberately failing test-executable cleanup census. The latter constructs a
second host, installs packaged Factories again and reports internal fixture
counts and paths. It does not prove a customer operation. Keep actual process,
listener, stream and Factory Session cleanup, public session deletion, provider
selection, Work/output/event identity, failure, timeout, cancellation and later
recovery assertions. Let testing's owned temporary directories manage their own
removal rather than manually deleting and rechecking them.

The Agent journey runs in parallel with other packages. Its cases remain
sequential because Recovery must observe the same host after adverse cases.
Every operation filters inherited home and model-cache overrides and uses an
owned home/model cache. Before cancellation, wait for the public Work query to
show the exact admitted Work in init/PROCESSING. Provider command entry does not
acknowledge publication of that projection. Preserve the same stopped-runtime,
terminal response stream, canceled edge and post-cancellation Work assertions;
no timeout, retry or accepted-state allowance is increased.

Six fresh native processes per version, in baseline/candidate/candidate/baseline
blocks of three, all pass. Baseline uses 9.508912 CPU-seconds / 16.692884793s
elapsed; candidate uses 6.344212 / 13.631833126s: 33.3% less aggregate execution
CPU and 18.3% less elapsed time for this journey. Build time is excluded. Both
scoped linters pass, and all 160 reviewed scenario decisions remain current.
Twenty additional focused cancellation repetitions pass with the processing
observation in place. This is local evidence, not proof that every hosted
cancellation race has been eliminated.

The unchanged classifier now joins Agent automatically: 39 package groups /
570 top-level registrations. Full four-CPU validation passes all 68 packages /
738 selected results, 736 pass and two skip, without retries and with unchanged
coverage gates and quarantine. It takes 209.95s supervisor / 200.927s coverage /
679.97 CPU-seconds (519.06 user, 160.91 kernel). Four compiler commands consume
0.718873 CPU-seconds. Nineteen links consume 39.862169 CPU-seconds over 39.309s
active link wall, one fewer supervisor link than the preceding 20-link local
layout-cache comparison. This slower full sample does not establish a complete
lane speedup or the two-minute checkpoint.

Parent 69eaa32820 passes hosted functional coverage in 210s complete / 150.914s
coverage. Integration passes the port-binding correction. Preserve its recovered
TestInterruptRace admission failure and Script Cancellation init/INITIAL failure,
including the extra routed-call observation; neither is claimed fixed by the
Agent change. These are recorded in the same-head flake ledger.

Evidence: `.artifacts/latency-audit/agent-customer-cleanup-paired/`,
`.artifacts/latency-audit/agent-customer-cleanup-full/`,
`.artifacts/latency-audit/pr2923-server-port-hosted/` and
`.artifacts/latency-audit/pr2923-server-port-flakes/`.


## Generated native conversion cache

The supported implementation caches immutable serialized Factory definitions,
not initialized sessions or rendered filesystem effects. Generation runs the
canonical strict public-to-native mapper for each of the 20 packaged Factories,
serializes the result and verifies whole-value round-trip equality. The generated
assets add 440,152 bytes. Each entry records its conversion format version and
exact source payload SHA-256. Existing generated-catalog drift checks regenerate
these assets with the current mapper; the ordinary JSON/YAML publications and
manifest remain unchanged.

The validated packaged catalog supplies detached optional bytes through the
canonical Wire graph. The compilation implementation checks version and source
identity, then decodes a fresh independent native definition. Missing, stale or
malformed entries retain canonical conversion and its diagnostics. Arbitrary
customer input continues on that path. Installation still executes definition
validation, layout pruning, authored normalization, canonical formatting,
portable-file effects, the authored writer, staged validation and atomic
publication. There is no rendered-file shortcut or shared mutable runtime state.

The source profile attributes 91.97 cumulative CPU-seconds (26.14% of the
351.78-second monolith sample) to generated conversion; this overlaps JSON and
installation stacks and must not be added to them. Six fresh customer-journey
repetitions per side compare the same graph and generated assets with only the
optional reader disabled for baseline. Both binaries are built before timing.
The three existing public journeys verify API terminal results, canonical
stream dispatch usage and installed-factory invocation outside the repository
with bootstrap parity. Baseline/candidate/candidate/baseline blocks of three
all pass. Baseline totals **27.481948 CPU-seconds / 29.172077655s elapsed**;
candidate totals **20.350384 / 23.626741598s**: **25.9% less aggregate execution
CPU and 19.0% less elapsed time** for these selected journeys. This is not a
claim about complete hosted CI timing or linker improvement.

Focused isolated decoder checks cover independently mutable returned definitions
and canonical fallback diagnostics; generation checks cover deterministic output
and replacement. Existing focused authored-layout/required-tool/persistence
checks pass. Both scoped lint suites pass after placing the decoder under the
compilation service's private implementation and constructing it in its owning
Wire provider. No architecture exception or lint allowance is added.

Evidence: `.artifacts/latency-audit/serialized-reader-paired/comparison.json`
and `.artifacts/latency-audit/pr2923-agent-cleanup-hosted/`, plus the recovered
interrupt evidence under `pr2923-agent-cleanup-flakes/`. The first complete candidate run passes all 68 packages / 738 results (736 pass,
two skip), with no failures or retries and unchanged gates. It takes **211.35s
supervisor / 187.850s coverage / 722.97 CPU-seconds** (585.05 user, 137.92
kernel). Changed-source coverage compiles 1,068 units, consuming 232.189588
CPU-seconds over 75.558s active compiler wall. Nineteen links consume 31.204730
CPU-seconds over 33.322s active link wall. Keep this cold build cost; it does not
establish a full-lane speedup. Evidence is under `serialized-reader-full/`.
The continuous four-CPU full comparison passes both versions. Baseline takes
**169.74s supervisor / 161.948s coverage / 544.19 CPU-seconds**; candidate takes
**140.07s / 133.380s / 449.12 CPU-seconds**. Baseline recovers one
TestInterruptRace CLI admission failure on the unchanged same-head retry; retain
that ledger and do not claim a clean matched full speedup from this pair.
Baseline has eight compiler commands (13.142591 CPU-seconds) and 20 links
(37.620202 CPU-seconds / 37.444s active wall); candidate has zero compiler
commands and 19 links (34.985537 CPU-seconds / 34.340s active wall). Candidate
has no failures or retries. Both preserve 68 packages / 738 results and all
gates. This local candidate is still above the complete hosted two-minute
checkpoint. Evidence: `serialized-reader-matched-baseline/` and
`serialized-reader-matched-candidate/`. Repeat baseline after candidate without changing CPU/coverage settings. It passes
without failures or retries at **193.65s supervisor / 184.204s coverage /
636.78 CPU-seconds** (474.41 user, 162.37 kernel), with zero compiler commands
and 19 links (35.578623 CPU-seconds / 37.074s active wall). Candidate's
140.07s / 449.12 CPU-seconds is **27.7% less supervisor wall and 29.5% less
aggregate CPU** than this clean baseline. Baseline variation from 169.74s to
193.65s remains visible; the first includes one recovered admission failure.
This is one complete candidate sample between two baselines, not a stable hosted
ceiling. The independent six-per-side native customer comparison supports the
conversion improvement. Confirmation evidence is under
`serialized-reader-confirmation-baseline/`.

The full internal packaged-catalog unit suite also runs against the parent source
through an overlay and against candidate. Both fail the same five top-level
tests: the validation-ledger inventory lacks the new dub-video Factory; two TTS
characterizations expect the former builtin-command model instead of the current
owned model resource; the subagent prompt inventory expects the former file
layout; and a generated-artifact check expects the former skipPermissions shape.
These existing assertions are not weakened to pass this cache change. New
focused decoder/generation/authoring checks and the complete functional lane
pass; the entire catalog unit suite is not claimed green. Exact parent/candidate
failure lists and JSON event logs are under `catalog-baseline-overlay/`.


## Live-main rebase and Script customer cleanup

After publication of cache head 110e1b3bd9, GitHub reports the PR conflicting
with advanced main; no workflow run exists for that head. Rebase the optimization
branch onto main 8cc84d34c6. The only conflict is generated Wire code; regenerate
it from the combined handwritten graph instead of choosing either generated
side. Cache generation leaves published assets unchanged, and focused decoder,
generator and authoring checks pass on the combined source. Both rebuilt
repository lint and builtin scoped lint pass.

Script cancellation has the same causal gap observed previously in Agent:
controlled command entry precedes publication of the exact processing Work.
Before cancellation, observe that Work through its explicit Factory Session's
public query, retaining the existing bounded timeout, state requirements,
stopped-runtime, event, command, deletion and isolation assertions. Twenty
focused cancellation repetitions pass. Remove the test-router constructor-only
probe, fixture build/server-start counters and manual temporary-directory census.
Keep per-scenario command requests and public session/Work/event identity,
listener shutdown, failure and recovery checks. Testing owns directory removal.

Six fresh native processes per side all pass, including the public shared Script
journey. Baseline totals 3.253245 CPU-seconds / 3.429897654s elapsed; candidate
3.464901 / 3.582569223s. The additional public processing observation increases
this short sample by 6.5% CPU / 4.5% elapsed (about 0.035 CPU-seconds per process).
This is a causal readiness/retry-risk correction and customer-test cleanup, not
an execution speedup; preserve the cost rather than treating removed fixture
assertions as measurable CPU savings. No retry or timeout allowance increases.
The package retains its genuine host-environment privacy proof and corresponding
native-binary safety exception.

Live-main discovery selects 67 packages / 762 results from 75 discovered
packages / 763 results; unchanged quarantine excludes eight packages and one
selector. The unchanged shared-binary classifier joins 39 package groups / 595
top-level registrations. These counts include new customer behavior merged on
main; historical 68/738 captures are not current-source validation. Complete
combined-source coverage is recorded separately under `cache-live-script-full/`.
Evidence: `script-customer-paired/comparison.json`, preserved native logs and
`cache-live-delta.tar` / deletion manifest. The latter records the exact source
delta applied to the owned Linux mirror; it is not a benchmark result.


Complete live-main validation passes all **67 packages / 762 results**, 760 pass
and two skip, in **172.00s supervisor / 163.712s coverage / 555.61 CPU-seconds**
(449.78 user, 105.83 kernel). Changed-source compilation runs 519 commands,
consuming 127.433284 CPU-seconds over 46.843s active compiler wall. Twenty links
consume 34.182308 CPU-seconds over 32.732s active link wall. All 160 reviewed
scenario decisions remain current and coverage/quarantine gates are unchanged.
One TestInterruptExplicitModes/recorded failure recovers on the unchanged
same-head retry: the expected Worker Session is absent from the public list.
Retain this distinct lookup failure in its flake ledger; it is not the earlier
TestInterruptRace admission diagnostic and is not resolved by the Script
correction. This combined source is not the earlier clean 140.07s sample,
and does not establish the complete hosted two-minute checkpoint.


## Exact Worker Session reads, cache drift coverage and refreshed CPU profile

The recovered TestInterruptExplicitModes/recorded lookup reads only the first
page of a fleet-wide list to find known source/successor identities. The public
list defaults to 50 results; a shared host retains unrelated sessions. Use
existing CLI worker-sessions show calls for the exact two IDs and retain source
CANCELED, successor RUNNING and both interrupt-lineage assertions. Other customer
list tests remain. No page-size or accepted-state allowance is increased.

Twenty repeated explicit-mode runs pass their customer assertions, then initially
fail fixture cleanup with an active provider count of -1. Reset clears active
accounting while an older call still owes its deferred decrement. Remove that
counter reset from both controlled providers; keep balanced lifetime accounting
and the existing zero-active cleanup assertion. The next 20 repetitions pass,
including cleanup. Six fresh processes per side also pass. Baseline totals
4.687995 CPU-seconds / 7.163311929s elapsed; candidate 4.831780 / 7.121415181s:
3.1% more CPU and 0.6% less elapsed in this small sparse-host sample. This is an
exact-read/retained-host reliability correction, not a measured package speedup.

Hosted c8b026366d reports the native cache's extra asset correctly, but old
analyzer unit expectations omit it in missing, added and renamed Factory cases.
Update those expected findings using the content-address contract, retaining
all existing read-only and unexpected-output assertions. Add explicit stale,
missing and non-regular native conversion checks. All PackagedFactoryCatalog
analyzer tests pass. The exact hosted race-check selection and both complete analyzer/plugin unit
suites pass; both scoped lint suites pass. Backend Lint's later
missing-Bun/deadcode evidence errors follow its failed race step, which skips
the normal setup/build steps; do not treat those cascades as an independent
runtime dependency fix. Packaging and lint failures are preserved in terminal
job logs under the latency audit artifacts.

The complete instrumented lane passes 67 packages / 762 results without failures
or retries at 186.77s supervisor / 175.736s coverage / 571.79 CPU-seconds (441.22
user, 130.57 kernel). It has two compiler commands (2.140022 CPU-seconds) and 19
links (36.843303 CPU-seconds / 33.589s active wall). Profiling affects timing;
this slower sample is retained and is not a full-lane speedup claim. The monolith
profile spans 134.89s and samples 411.61 CPU-seconds. Initialization accounts for
209.43 cumulative seconds (50.88%), packaged installation 208.78 (50.72%), layout
preparation 110.04 (26.73%), definition validation 67.52 (16.40%), and generated
config expansion 69.07 (16.78%). JSON unmarshal accounts for 143.29 cumulative
seconds; syscall.Syscall6 has 69.19 flat seconds. These nested stacks overlap.
This capture lacks package labels; do not infer per-package CPU by adding
concurrent process deltas. Earlier labelled package priorities remain historical.

The next major target is repeated canonical loading/validation during immutable
packaged preparation and publication. Inspect exact byte forms and cache hits,
then measure reuse of canonical conversions or generated renderings through the
owning compiler/writer ports. Preserve customer edits, validator calls, portable
files, file modes, independent values and atomic publication. Do not replace
those ports with the rejected filesystem shortcut or share session state.

Evidence: `interrupt-identity-paired/`, `interrupt-identity-profile/`,
`pr2923-cache-live-hosted/`, `job-112262243278-terminal.log`,
`job-112262243397-terminal.log` and `cache-live-backend-lint-failed.log`.


## Packaged serialization, canonical encodings and installation reuse

The existing immutable native conversion cache is effective on its selected
decoder port: a private stderr probe of three existing customer journeys passes
with 242 reads, 241 hits and one miss (20 distinct published inputs). This is
read accounting, not a timing experiment. It does not imply installation is
cached: each initialized home still validates, prepares, writes and publishes
its own definitions and portable files. Session/runtime state stays independent.

Canonical pre-persist validation marshals the submitted generated API value,
then the loader expands that different JSON encoding. A private experiment
routes the normalizer through the existing decoder, preserving authored
normalization, Flatten validation and every loader/validator operation. The
same three journeys pass, but reads become 483 / 241 hits / 242 misses, with 21
distinct misses. Simply binding the existing reader cannot remove this cost.

A second experiment generates the exact pre-persist encoding's SHA alongside
the published source SHA, verifies whole native configuration equality through
both strict mappings, and accepts only those two exact inputs. It retains all
validation, writer ports, independent decoded values and customer-edit fallback.
Generation, focused serializer/analyzer tests, scoped builtin lint and the
customer journeys pass. Probe coverage rises to 482 hits / 483 reads across 40
exact published/canonical forms. However, measured execution gets worse.

In B3/C3/C3/B3 order, six repetitions per side of the same three customer journeys
all pass. Baseline consumes 23.729038 CPU-seconds / 26.347894477s elapsed;
candidate 25.862887 / 28.864202045: **9.0% more CPU / 9.6% more elapsed**. Valid
three-repetition samples span 12.921-13.427s baseline and 13.741-15.123s candidate.
Binary builds are excluded from these execution totals. This is a small sample,
but it provides no performance justification for shipping the alias expansion.
Restore all experiment-only source, generated assets and tests; retain its patch
and results under private latency artifacts. No additional production cache or
changed cache-hit allowance is installed.

Prioritize avoiding repeated installation through package-owned initialized
test homes and isolated Factory Sessions, then investigate a prepared-layout
cache through the existing authoring writer and persistence ports. Cache only
immutable definition/file preparation, keep destination writes, validators,
permissions, portable artifacts and atomic replacement, and invalidate against
exact source plus serializer/generator version. Retain dedicated fresh-install
customer coverage. The earlier rendered-file prototype's full-lane 20% saving
remains historical evidence for that direction, not a validated production
shortcut or an estimate for current main. Sharing live Factory Session state is
not part of this optimization.

Hosted d9e4f57e8e's functional ledger explicitly reports no test failures.
The catalog drift unit/packaging corrections pass in that workflow; the separate
lint migration fixture omission must also be repaired. Complete 189s exceeds
the two-minute checkpoint, so PR #2923 stays draft.

The regenerated synthetic publication supplies the missing native asset without
changing authored inputs, JSON/YAML pairs, manifest or smoke assertions. Both
delivered-plugin smoke cohorts pass: 16 packaged-source and 11 catalog cases.

Evidence: `serialized-cache-read-probe/`, `canonical-cache-read-probe/`,
`canonical-cache-alias-read-probe/`, `canonical-cache-paired/`,
`canonical-cache-candidate.patch`, `pr2923-identity-hosted/`,
`identity-hosted-functional.log` and `identity-hosted-lint-failed.log`.

## Work CLI scenarios share installation through explicit Factory Sessions

The Work CLI group originally owns one command process plus four server
processes and four server homes. Each parallel scenario also uses a separate
CLI home and targets its server's default Factory Session. The scenarios test
Work filters/counts, three-page REST traversal versus CLI aggregation, exact
detail lookup beyond the first page, and same-name supersession history.

The parent now starts one root-built host in one initialized owned home and
uses that host's Process.Execute for CLI requests. Each child still scaffolds
its own Factory directory, opens a distinct public Factory Session, supplies
its exact --session to every CLI submission/move/read and scopes its independent
REST page walk to that ID. Child cleanup terminates/deletes only its own session;
the parent's existing server/process cleanup runs after all parallel children.
Overlapping Work IDs remain deliberately present. All original customer output,
count, state, pagination and history assertions remain. No global environment
mutation, binary invocation, timeout change or retry allowance is added.

Six fresh Linux processes per side pass in B3/C6/B3 order. The first pair totals
10.645610 CPU-seconds / 3.765948623s baseline versus 4.193485 / 2.921788809s
candidate, but baseline wall varies 0.387-1.193s. Retain that variance rather
than using its large percentage as the headline. A fresh confirmation pair
totals **6.619411 CPU-seconds / 2.541354250s baseline versus 3.448197 /
2.495368699s candidate**, saving **47.9% CPU / 1.8% elapsed**. Per-process elapsed
is 0.393-0.460s baseline and 0.387-0.466s candidate. Builds are excluded from
execution totals. Windows customer scenarios and both scoped lint suites pass.

The exact Work CLI candidate passes the complete lane at 103.05s supervisor /
98.240s coverage / 310.37 CPU-seconds (247.17 user, 63.20 kernel), with 67/762,
760 pass/two skip, no failures or retries. Two compilers consume 2.468672 CPU
seconds; 19 links consume 22.714726 CPU seconds / 22.053s active link wall.
A subsequent same-budget previous-source full run also passes without retries
at 123.34s / 118.506s / 374.89 CPU-seconds. This single full pair improves, but
the focused Work group cannot explain the whole difference. Do not extrapolate
its small package saving into a reliable full-suite ceiling or a hosted claim.

Additional cleanup-hook experiments preserve all finalizers and pass native
tests, scoped lint and full coverage, but do not change the 39-group monolith
inventory. After custom TestMain is handled, metadata exposes additional
constraints: packaged Fix and Codex have executable Git fixtures, AGY has a
live quarantined selector, and base Providers uses the Go test executable as a
mock script child. Preserve those constraints; restore all ineffective hook
changes. The broader hook probe takes 121.99s / 113.638s coverage / 368.19 CPU
seconds; the base Providers hook probe takes 110.43s / 305.86 CPU-seconds. They are retained
experiments, not linker reductions. Future consolidation requires resolving
the actual executable/quarantine construction, not bypassing native selection.

Evidence: `work-cli-sessions-paired/`, `work-cli-sessions-confirmation/`,
`work-cli-sessions-full/`, `work-cli-sessions-baseline-full/`,
`compatible-cleanup-hooks-full/`, `provider-cleanup-hook-full/`,
`pr2923-smoke-fixture-hosted/` and `smoke-fixture-hosted-functional.log`.
Complete hosted timing remains above 120s, so PR #2923 stays draft.

### October 6: remaining initialization cost and publication synchronization

Hosted run 37487752183 at ae23bdff07 passes Functional Coverage in 229s
complete / 154.149s coverage invocation: 67 packages, 763 results, 761 passes
and two skips. The supervisor takes 165.768s, including 15.259s quarantine.
The prefix-restored build executes 136 compiler commands and 16 links. This
sample remains above the two-minute checkpoint and is not a controlled
comparison with earlier runners. The workflow fails Backend Lint: its exact
deadcode set adds one helper, validatePortableLayoutBoundaryJSON, to 2,528
existing findings. The corrective change inlines that decoding into the
existing public wrapper; it does not raise the baseline or change validation.
All factoryconfig package tests and both scoped linters pass after correction.

A fresh native customer_journeys profile exposes an intermittent transcript
publication race: completed live state precedes durable capture. One of ten
focused baseline repetitions fails with WORKER_SESSION_TRANSCRIPT_UNAVAILABLE.
The customer test now observes its existing public terminal stream before its
existing transcript read. That stream joins terminal publication. The change
adds no requests, retries, sleeps or timeout allowance; all session correlation,
continuation and lineage assertions remain. Fifty Linux and twenty Windows
focused repetitions pass; the complete profiled customer package also passes.

The passing profile measures 16.930s elapsed and 27.516843 CPU-seconds
(19.587286 user, 7.929557 system), with profiling enabled. Initialization owns
69.62% of sampled CPU; packaged installation owns 69.29%; fresh creation
54.18%; layout preparation 32.22%; JSON unmarshalling 34.33%; filesystem syscall
execution is 24.17% flat. These cumulative shares overlap and cannot be added.
The sample is one native customer package, not whole-lane attribution or a
controlled before/after speedup. Existing serialized native configurations and
canonical-output caches remove conversions, but retain selected validation,
layout preparation and operation-owned files. Sharing compatible initialized
hosts with explicit independent Sessions remains the first priority; immutable
rendered layout content is the next cache candidate, preserving selected ports
and file validation. Mutable live Factory state must remain session-owned.

A shared-host YAML portability experiment fails correctness before timing:
public folder-based Session creation resolves factory.json, while this scenario
invokes a materialized YAML root through the CLI's explicit factory-file route.
The experiment is restored rather than dropping YAML invocation coverage or
changing the public resolver to obtain a favorable benchmark.

Evidence: boundary-decode-hosted/, canonical-boundary-customer-profile/,
remote-transcript-reproduction/, remote-transcript-barrier/,
canonical-boundary-barrier-profile/ and
packaged-portability-shared-session-attempt.patch under the ignored local
latency-audit artifact directory.
