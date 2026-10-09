# Recording-read policy

`recordingreads` implements the backend standard's recording boundary through
compiler-resolved function identities. Its catalog comes from Recordings' public
contracts and their re-exported internal contracts. It also includes structural
forwarding interfaces whose parameters or results retain Recordings-owned types.
Method values and expressions count as read access even before invocation.
An unrelated operation with the same name does not count.

| Contract family | Full-history boundary | Treatment |
| --- | --- | --- |
| `contracts.go`, `lifecycle_capability.go`, `replay_artifact_capability.go` | Historical queries; replay/resume loaders; canonical event access; world/scope reductions; replay planning and reconnect; dashboard and workstation-request reductions | Explicit replay, restore, inspection, export and canonical event delivery declarations may read. Ordinary projections retain exact debt until materialized. |
| Artifact contracts | Build, read, decode and export portable recording envelopes | Explicit recording surfaces and their codecs may read. Ordinary request helpers receive no service-wide exemption. |
| `worker_capture.go` and re-exported Worker contracts | Load, reduce, replay, build, export, encode and decode full Worker histories | Recording codec and IO mechanics may read; ordinary usage, identity and terminal-cause queries may not. |
| `worker_work_attribution.go` | `ResolveWorkerWorkAttribution`, `ReadWorkerFactoryHistory` | These conceal historical reconstruction and are retained debt, including callers inside Recordings. |
| Worker activity/artifact delivery | `ReadWorkerCapturedActivity`, `ReadWorkerCapturedArtifact` | Explicit Worker log, transcript and artifact requests may read. Summary and control callers are not exempt. |
| Capture metadata and control | Capture list/lookup, continuation source, restart recipe, recording projection | These can hydrate a journal or rebuild a catalog. Treat them as read boundaries until readiness preparation and bounded metadata behavior remove that dependency. |

Incremental `AdvanceWorkerRecording`/`FailWorkerRecording`, validation, redaction,
pure presentation and separate input-blob reads are not full-history reads.
New public capabilities require classification against this catalog; a method
named “query” or “summary” does not establish bounded behavior.

`recordingReadOwners` lists exact package/declaration pairs and explains each
owner family. There is no blanket Recordings, runtime, Worker Sessions or Wire
allowance. Nested closures inherit their enclosing declaration's ownership.
Generated sources are excluded; a production filename, package or declaration
containing “test” does not gain recording access. `_test.go` files may exercise
the readers as their test subject.

`baseline.txt` retains exact file/declaration/operation/count keys for unresolved
callers. The first migration may seed this new rule; after it lands, additions,
replacement sites and increased occurrence counts fail `baselinegrowth`.
The migration marker remains after the last entry is removed, preventing a
second seed when the allowance count reaches zero. Removing calls or decreasing
the count at the same site is permitted. A removed
site must lose its entry or the analyzer reports stale debt. Inactive platform
and build-tag files are judged by the compilation that selects them.

Remediation belongs to the API's service owner: record facts when known, prepare
retained metadata before readiness, or cache immutable ended projections by
recording generation. An exact debt entry does not prove runtime compliance.
Keep boundary tests in the analyzer fixtures; prove repaired customer behavior
through the public application boundary with recording reads denied after
readiness and measure latency separately in the load lane.
