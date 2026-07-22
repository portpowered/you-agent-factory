# First exemption burn-down baseline

> Repository revision: `9f5944f40dd57d2a1ae8fe05972f075e9e6cff21`  
> Ownership snapshot: `2026-07-13T08:09:44Z` (UTC)  
> Comparison base: `origin/main` at `9f5944f40dd57d2a1ae8fe05972f075e9e6cff21`

## Selected cohort

The highest-ranked eligible inventory subset is the first two rows of the
[`pkg/factory/requests` ranked cohort](backend-quality-exemption-inventory.md#ranked-first-burn-down-cohort):

| Rank | File | Registered target | Owner | Removal reason |
| ---: | --- | --- | --- | --- |
| 1 | `pkg/factory/requests/work_request_submit_test.go` | `pkgmaintcheck:ignore-cyclomatic-complexity` on `TestWorkRequestFromSubmitRequests_PreservesCanonicalBatchContract` | `backend-maintainers` | Preserve the canonical batch conversion's cross-item payload, relation, trace, and clone behavior while splitting its assertions. |
| 2 | `pkg/factory/requests/work_request_test.go` | `pkgmaintcheck:ignore-cyclomatic-complexity` on `TestNormalizeWorkRequest_IndependentWorkItemsShareRequestAndTrace` | `backend-maintainers` | Preserve normalized request identity, trace identity, generated work IDs, tags, and payload while splitting its assertions. |

This is one package, two handwritten Go test files, and two active registered
directives. It excludes generated and vendored code, checker fixtures,
`cmd/factory`, composition/root/wire code, Factory Session code, response-stream
code, and website-session code. The two matching entries are present in
`docs/internal/baselines/backend-exemption-budget.json` at this snapshot.

## Collision refresh

The exact changed-file set for each Git branch below is the sorted output of
`git diff --name-only origin/main...<head>`. The exact PR set is the sorted
`filename` output from the GitHub pull-files API at the recorded head. Counts
and SHA-256 digests make those newline-delimited sets auditable without copying
large unrelated file inventories into this document. Every set was compared
with both selected paths; every comparison returned zero matches.

Batch 005 uses the two active implementation heads. Batch 006 retains the
authoritative reservations from the inventory: every detailed directive file
under `pkg/api/**`, `pkg/apisurface/**`, `pkg/cli/**`, `pkg/mcp/**`,
`pkg/workers/**`, `pkg/factory/sessions/execution/**`, `pkg/interfaces/**`,
`pkg/services/provider_sessions/cursor/**`, `pkg/platform/replay/**`, and `pkg/factory/replay/**`; its named Work-family
roots contain no registered directive file. Batch 007 reserves every detailed
directive file under `pkg/factory/sessions/execution/**` and `pkg/service/**`.
Batch 008 reserves every detailed directive file under `pkg/runtimehost/**`
and `pkg/service/**`. Neither selected `pkg/factory/requests` path is in those
reservation sets.

| Ownership reference | Head | Files | Sorted-path SHA-256 |
| --- | --- | ---: | --- |
| Batch 005 `create-root-process-owner` | `b70e5c5e09b76d760e5c589abdb77b81c8a70472` | 22 | `285c8c0f85699ffeb0072f32a8e8665f3b1c710c24265f6d8aa7a2456f1f09fa` |
| Batch 005 `create-wire-application-graph` | `0cdbbab94b10e98a9ebebccdd3e86263ddba13d3` | 16 | `0bd3da0e5f8ad231a184dc177528aa97074cf5c1b3b8b8e1520f52267fb54daa` |
| Batch 008 `runtime-cleanup-model-direct-di` | `6330a97f02086cf39d544b199bf367436b12942a` | 8 | `5c99ef2d2f393145b8b3c1ebe0b8f7e83a279743a591388b7ce27b8b2cb86b00` |
| Batch 008 `runtime-cleanup-search-artifact-hygiene` | `be66893143d67e745c799ba9ca0b567b5ff41fc0` | 3 | `a217c9b53459d0cbf3230f1f08182bb54518e9c5c34d827c6c8eb1ffc2d53881` |
| Batch 008 `runtime-cleanup-shim-markers` | `9c663c6258655439038238a839dfc588aaacc246` | 9 | `8a0e234bcf144e1f64c39fc071856368df1906678896be5898f3de23f4fcb8ea` |
| Batch 007/008 `service-break-06` | `3ed4c025a74670a7221a3405ad9934a1393109cb` | 35 | `db5c410d096380626a09ba5d938bb2a9f9690e16bd22182ba895a8938f913389` |
| Batch 007/008 `service-break-p03-extract-models-service` | `56b33976cfc34702c016c0a9b4b2effd868dbf2b` | 23 | `4232b5b2a22035ccc074afe73e9cc088de11394903b4578dc40f222b72fced96` |

Active website repair and response-stream work was refreshed independently of
the open-PR union because several local heads had not yet opened a PR:

| Ownership reference | Head | Files | Sorted-path SHA-256 |
| --- | --- | ---: | --- |
| Website `dynamic-workflows-cell-real-backend-api-website-inspection` | `6af3008095ba1c5e050fd2172c6efb5668d32395` | 19 | `9fbffe5107b43357470b6ade16d47dfd2163687c3ef694cd3a00a8f76879e387` |
| Website `dynamic-workflows-cell-website-artifact-drilldown` | `01ab43eeb85bbcc7911ff9736b2c16e8f82c6dd1` | 19 | `90141c634cda5a07f837cd10f9bb237f5f17758865f4faf85a9b9e1be4ab3f0e` |
| Website `dynamic-workflows-cell-website-event-replay` | `58b72bdc233c6312c08f77de8045d0f4c71467c2` | 6 | `aadefa1977be1b75060d4459554e4d24c81f67d83791d0043f979607c7b46745` |
| Website `dynamic-workflows-cell-website-inspection` | `591f1f4dbd17f0f59f47172185a7528dfa55d5b4` | 10 | `9aa94b86c42141bc81deaa0632dc42f6a6996b90d7f725b0c1177106e5316305` |
| Website `session-repair-explicit-hydration-state` | `ea91682ce342bd55a84434f53ea396f4a47a8ebc` | 13 | `4eda84cc1c75ff9226eb60b3b3d0b5912d69df7526649ff1c859f93c11c42db3` |
| Website `session-repair-multi-session-contamination-regression` | `9d30715b002d0c92772ad332764c83293d0959fc` | 7 | `35fa5a8846f0ef3c92ccb5b25b5e02b4a738a51864fd5908e2b19444d9ff623b` |
| Website `session-repair-persistence-failure-race-regression` | `a803d4d6d7d4ad3318f0a37a9d53d01fcfd1ad02` | 11 | `646000ff7009c17ecb0cbf09579e3089b0e395540150e97b0446c7fa14afb5b9` |
| Website `session-repair-pure-preflight-resolution` | `95550e0c39c368897e044f234dec777fdd159b04` | 12 | `1156963875096a9f11672a20168ce2f32a62e3214575b97c89f459fd2113e8ed` |
| Response stream `you-goal-p17-add-cli-response-stream-renderer` | `ecf2cc653583c46755e0338c6f695b53e1a2f79e` | 21 | `774adbbc4b6d5bea595aa86b530a92543028da6834fbf49ab5c3afd08456b7b5` |

All open PRs at the snapshot time were also compared, including work already
listed above. The PR number plus immutable head is the ownership reference.

| PR | Head | Files | Sorted-path SHA-256 |
| ---: | --- | ---: | --- |
| #1073 | `0cdbbab94b10e98a9ebebccdd3e86263ddba13d3` | 15 | `286164535dfe859f39ce2c0c2d85bff697f1ed43d370e2f2a79ab8606059a23b` |
| #1072 | `39d71ef5456cac13f26e7f58575f21b3119ae2b2` | 21 | `126b3ad6734f90692ceaf3859d0b91ecf18322a6369f3b0e2d5327fb7dcc2a9a` |
| #1071 | `9b36b57c196e2203896f91f5ce50d782d5d93250` | 8 | `94f49d8deab34d82d798ebb150ccb5f810a2ec855e9a3197ab228b3f88e81109` |
| #1070 | `ad4b106cdb8ce5eb29e122262bd64d4579dbaec3` | 33 | `d1d7c37f591e6ec137e86eeced514e851a161187efd05ec0e2e7a8542c9e3205` |
| #1064 | `e4430d6e028a863e1408048546f5c4410f00cbae` | 26 | `b13255e7cbf9eff05a9cbe1a63ae7fe465ee770ce1732253a2a9db1bf13be89f` |
| #1062 | `76db760b53e0cb0a15b5da8485d4a74f851eb93d` | 3 | `bc799367ba3a2c05f68dde9cc353873def050bcb2be60b5c12d03b0c553906b5` |
| #1040 | `41ad64d6d527d7bb6c09131f94e71c879318f9f3` | 13 | `9320fdbf233ab703b6c4535d8c25fe0f9642de202e779a59cdcb75ccf3a0f750` |
| #1001 | `ad2e805a360c1f6aaef11b0a9087c38cd36174ca` | 26 | `1097d6155500d95198ded315660520b1d343fc92b64e3e57919c368f004b55b6` |

No PR existed for `system-consolidation-first-exemption-burn-down`, so there
were no PR conversation comments to address before this story began.

## Behavioral baseline

Before changing either selected source file, the focused command was run at the
repository revision above:

```sh
go test ./pkg/factory/requests -run 'Test(WorkRequestFromSubmitRequests_PreservesCanonicalBatchContract|NormalizeWorkRequest_IndependentWorkItemsShareRequestAndTrace)$' -count=1
```

It passed both tests. Existing adjacent observable tests also cover the relevant
boundaries and failures: empty and empty-mutable submit-request conversion,
legacy trace/request inheritance, empty work rejection, duplicate names,
invalid relation endpoints and semantics, invalid work types and states,
dependency cycles, and content/payload conflicts. The evidence asserts returned
requests and errors only; it does not scan source text, directives, file layout,
command or route inventories, or documentation topology.
