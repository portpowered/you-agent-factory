# LocalAI OMNI-to-Factory artifact contract delta

Status: proposal-only characterization, story
`localai-omni-artifact-contract-delta-001`. This document records the verified
current path. It does not authorize a Proposed contract, production change,
executable test change, generated-artifact change, or Project acceptance.

## 0. Authority and scope boundary

- Current evidence is from `HEAD`
  `e10e38843aff30c7871b732b284976ee13ab42f1`.
- The execution packet is `prd.json` for
  `localai-omni-artifact-contract-delta`; no `operatorAmendment` is present.
- The PRD identifies `docs/temp/projects/localai/source-plan.md`,
  `docs/temp/projects/localai/request.md`, and
  `docs/temp/projects/localai/acceptance.md` as immutable authority. None of
  those paths exists in this worktree and none is tracked by this checkout.
  Their contents therefore cannot be independently reconciled here. This is a
  recorded authority gap, not permission to recreate, rewrite, or weaken them.
- No production Go, test, OpenAPI, CLI, Factory graph, protobuf, generated, or
  immutable authority file is changed by this characterization.

The supplied failure witness and the PRD's embedded current-chain references
are retained as inputs. A later operator-approved contract revision is still
required before implementation; `LA-05`, `LA-06`, and every runtime quality
gate remain unproven.

## 1. Reconciled current OMNI-to-Factory propagation

The current path has a single content-only private hop. The public Models
contract is already capable of carrying an artifact, but the OMNI codec does
not produce one and the OMNI wire adapter does not populate the existing
runtime artifact-source field. Consequently, the existing downstream mapping
has no artifact fact to preserve.

### 1.1 Models public output capability

Authored source: `pkg/services/models/local_execution_contract.go`.

At lines 194-201, `InferenceContent` is detached, named content. At lines
243-271, `InferenceOutput` can carry inline content and an optional
`InferenceArtifact`; `InferenceArtifact` contains an opaque reference, name,
media type, byte size, and safe properties:

```go
type InferenceContent struct {
	Name        string
	Modality    Modality
	ContentType string
	MediaType   string
	Content     string
}

type InferenceOutput struct {
	Name        string
	Modality    Modality
	ContentType string
	MediaType   string
	Content     string
	Artifact    *InferenceArtifact
}

type InferenceArtifact struct {
	Artifact   InferenceArtifactRef
	Name       string
	MediaType  string
	SizeBytes  int64
	Properties map[string]string
}
```

At lines 779-797, `InvokeModelResult` already carries both legacy `Content`
and `Artifacts`, plus generic `Outputs` and lifecycle fields. In particular,
the result shape already records status and lease disposition; no public result
field is missing for this characterization.

```go
type InvokeModelResult struct {
	Invocation ModelInvocationRef
	Scope      RuntimeScopeRef
	Lease      ModelLeaseRef
	ModelName  string
	Operation  string
	Status     ModelInvocationStatus
	Content    []InferenceContent
	Artifacts  []InferenceArtifact
	Outputs    []InferenceOutput
	LeaseDisposition InvocationLeaseDisposition
	CancellationOutcome InvocationCancellationOutcome
}
```

The internal comments at the source retain the exact compatibility meaning:
`Content` and `Artifacts` remain populated for the prepared primitive while
`Outputs` is the additive generic projection.

### 1.2 Models protocol boundary and private LocalAI codec

The provider-neutral protocol boundary is
`pkg/services/models/managed_runtime_contract.go:159-166`:

```go
type InvocationProtocolResponse struct {
	Text  string
	Usage string
}
```

The private LocalAI adapter is
`pkg/services/models/internal/backends/localai/omni_codec.go`:

- `PredictResponse` at lines 31-36 contains only `Text` and `Usage`.
- `OmniCodec.Invoke` at lines 118-166 returns
  `([]models.InferenceContent, error)`.
- It rejects a blank response text as a typed
  `InvocationFailureClassMalformedResponse` for slot `text`.
- It creates a named `text` `InferenceContent` with modality `TEXT`,
  `ContentType: "text/plain"`, `MediaType: "text/plain"`, and the response
  text. It may append named JSON `usage` content when the declared operation
  includes that output slot.
- It never constructs `InferenceArtifact`, so no identity, media metadata, or
  size fact can leave this codec.

The protocol client is intentionally private and its interface accepts and
returns codec-owned detached types. This is the first and currently decisive
loss point; it is not a LocalAI protobuf or public Models-schema gap.

### 1.3 Private wire handoff

Authored source:
`pkg/services/models/wire/wire.go:540-584`, with the runtime result contract
at `pkg/services/models/internal/services/inference/runtime_contract.go:15-40`.

The existing private runtime result can carry artifact sources, including
Models-owned metadata and internal-only source details:

```go
type InvocationArtifactSource struct {
	RefValue   string
	SourcePath string
	Name       string
	MediaType  string
	SizeBytes  int64
	Properties map[string]string
}

type InvocationRuntimeResult struct {
	Content   []models.InferenceContent
	Artifacts []InvocationArtifactSource
}
```

The OMNI adapter currently calls the codec and returns only its content:

```go
content, err := runtime.codec.Invoke(ctx, request.Request, request.Operation)
if err != nil {
	return inference.InvocationRuntimeResult{}, err
}
return inference.InvocationRuntimeResult{Content: content}, nil
```

`backendInvocationRuntime` in the same file already maps backend-returned
artifacts through `invocationArtifactSources`; the asymmetry is specific to
the OMNI codec return path. No second runtime, provider, or storage owner is
needed to explain the observed loss.

### 1.4 Models inference lifecycle and registrar

Authored source:
`pkg/services/models/internal/services/inference/internal/service/invoke_lifecycle.go`.

`finishCompletedInvocation` at lines 101-137 registers
`runtimeResult.Artifacts`, normalizes generic outputs with the registered
artifact metadata, stores a detached `InvokeModelResult`, marks the result
completed, and releases the invocation lease. `registerInvocationArtifacts` at
lines 140-156 is the only path that turns private artifact sources into public
`InferenceArtifact` values.

The registrar is
`pkg/services/models/internal/services/inference/internal/artifacts/registrar.go:17-73`.
It assigns `models-inference:artifact:<n>` when `RefValue` is empty, rejects a
non-empty-but-whitespace reference, copies name/media/size/properties, and
keeps a source path only for the separate export operation. The reference is
opaque and is not a filesystem path.

If registration or output normalization fails, the lifecycle calls
`finishFailedInvocation`, stores a failed/cancelled result with no successful
outputs, and releases the lease. This existing failure/release behavior is
characterized but is not an OMNI artifact success proof.

### 1.5 Workers output mapping

Authored source:
`pkg/services/workers/internal/services/runners/internal/inference/generic_invocation.go`.

`proposedOutputFromModelResult` at lines 198-233 prefers the generic
`result.Outputs`; only when that slice is empty does it synthesize outputs from
`result.Content`. It appends a Workers `ArtifactRef` only when an output has a
non-zero `output.Artifact`.

`workContentPartFromModelOutput` and
`baseWorkContentPartFromModelOutput` at lines 236-274 set the canonical Work
content part's `ArtifactID`, label, and metadata only from that same artifact
pointer. Text content itself is mapped at lines 277-284, with `text/plain` as
the default content type. Therefore text arriving only as `InferenceContent`
is observable, but it cannot produce a Work artifact identity.

The Workers-owned shapes involved are unchanged:

```go
type ProposedOutput struct {
	Primary        []work.WorkContentPart
	Feedback       string
	Classification string
	ProposedWork   []ProposedWork
	ArtifactRefs   []ArtifactRef
}

type ArtifactRef struct {
	ArtifactID string
	Label      string
	URI        string
}
```

### 1.6 Work identity, materialization, and lineage

Work owns canonical content and materialization. The relevant public/internal
shape at `pkg/services/work/contracts.go:339-352` already includes
`ArtifactID` and metadata:

```go
type WorkContentPart struct {
	Type        WorkContentPartType
	Text        string
	URL         string
	File        string
	JSON        json.RawMessage
	Slot        string
	Label       string
	Role        string
	ContentType string
	ArtifactID  string
	Metadata    map[string]any
}
```

`applyMaterializedWorkerOutput` at
`pkg/services/factory_runtime/internal/services/orchestration/runtime/work_output_materialization.go:391-478`
passes `Primary`, feedback, classifications, proposed Work, valid types, and
`lineageContextFromDispatch` into the Work service. The lineage context at
lines 494-507 carries dispatch ID, request ID, source Work IDs, current and
previous chaining trace IDs, parent Work ID, and trace ID. Materialization
failure clears `RecordedOutputWork` and output content and changes the result
to failed; cancellation clears output and does not materialize a completed
proposal.

The Work path is therefore already able to preserve an artifact-bearing text
part once Workers receives one. It is not the source of the current OMNI loss.

### 1.7 Recordings canonical Factory Event and replay projection

Recordings owns the canonical Factory Event ledger and replay; the process-local
Events service is not this history. In
`pkg/services/recordings/internal/events/event_history.go:602-646`,
`RecordWorkstationResponse` appends a `DISPATCH_RESPONSE` event with dispatch,
Work, and trace context and an `OutputWork` payload derived from completed
token mutations. `outputWorkItems` at lines 916-927 copies each non-resource
token into a detached `FactoryWorkItem`, including prior chaining trace IDs.

`pkg/services/recordings/internal/events/event_history_generated.go:185-215`
maps each Work item to the event representation, retaining Work ID, Work type,
state, chaining/trace fields, cloned content, structured result, and tags.
Because the Work content is copied rather than re-derived, a future
artifact-bearing Work part can be replayed without a new event field. The
current path has no such part to copy.

### 1.8 HTTP and CLI projections

The transport owners are adapters, not artifact authorities:

- HTTP mapping in
  `pkg/services/models/transports/http/invoke_operations.go:203-325`
  (`GenericInvocationResponseToGenerated`,
  `genericInferenceOutputToGenerated`, and
  `genericInferenceArtifactToGenerated`) projects output name, modality,
  content/media type, opaque `artifactRef`, name, media type, `sizeBytes`, and
  properties when `InferenceOutput.Artifact` is present.
- CLI mapping in
  `pkg/services/models/transports/cli/root_invoke.go:966-998`
  (`genericInvocationResponseFromInferenceResult`) performs the equivalent
  projection and omits an empty artifact reference.
- The generated HTTP shape at
  `pkg/transports/http/generated/server.gen.go:6264-6278` is
  `ModelInvocationArtifact` with required opaque `artifactRef` and optional
  `name`, `mediaType`, `sizeBytes`, and `properties`; the enclosing
  `GenericModelInvocationResponse` has ordered `outputs` at lines 5377-5384.
  The authored OpenAPI source remains the owner of those generated shapes; no
  authored or generated transport file is changed in this story.

These mappers can expose a populated artifact without a shape change. They
cannot manufacture one from text-only OMNI content, which explains why a
direct Models transport test is not a Factory-lineage witness.

## 2. Coverage characterization

The focused inventory was obtained with `rg -n '^func Test'` over the cited
Models, LocalAI, Workers, Work/Runtime, Recordings, and Models transport
directories. The tests below prove existing local behavior, but none joins a
successful OMNI response to a Factory Session's Work and replayed canonical
event.

| Boundary | Existing focused evidence | What remains unproven |
| --- | --- | --- |
| LocalAI protocol/codec | `pkg/services/models/internal/backends/localai/omni_test.go`: `TestOmniCodecForwardsOrderedMediaAndDetectedTypes`, `TestOmniCodecRejectsUnsupportedModalityBeforeProtocolCall`, `TestOmniCodecPreservesArtifactReferenceAndRejectsMalformedResponse`; `grpc_protocol_test.go`: `TestPinnedGRPCProtocolClientMapsOrderedOmniValuesToPinnedFields` | Successful OMNI artifact descriptor creation and size semantics |
| OMNI wire/runtime | `pkg/services/models/wire/omni_runtime_test.go`: `TestNewInvocationRuntimeFailsClosedWhenOmniProtocolIsUnbound`, `TestNewInvocationRuntimeForwardsOmniInputsAndDeclaredUsage`, `TestInvocationProtocolAdapterPreservesFailureAndOperationFallback`; `construction_effect_doubles_test.go`: `TestBackendInvocationRuntimeMapsContentAndArtifacts` | OMNI `InvocationRuntimeResult.Artifacts` population |
| Models registration/normalization | `pkg/services/models/internal/services/inference/internal/service/invocation_outputs_test.go`: `TestInvokeModelWithLeaseReturnsDetachedArtifactMetadata`; `generic_invocation_test.go`: `TestNormalizeGenericInvocationOutputsPreservesNamesAndMetadata`, `TestNormalizeGenericInvocationOutputsRejectsMalformedAndOversizedResponsesAtomically`; `internal/artifacts/registrar_test.go`: `TestRegistrarExportsInvocationArtifactThroughInjectedFilesystem`, `TestRegistrarRejectsInvalidArtifactReference`; `service_test.go`: timeout, cancellation, failed-status, and detached-result cases | Registration of an artifact sourced by the OMNI path and end-to-end success/release together |
| Workers | `pkg/services/workers/internal/services/runners/internal/inference/generic_invocation_test.go`: `TestProposedOutputMapsAllModalitiesAndArtifacts`, `TestProposedOutputUsesContentFallbackAndNamesByModality`, `TestWorkContentPartFromModelOutputRejectsMalformedOutputs`, `TestRunnerRejectsFailedAndCancelledModelStatuses`, `TestWorkContentPartEmptyRecognizesArtifactBackedParts`; `runner_test.go`: `TestRunnerPreservesModelsOwnedOutputURLsAndArtifacts`, `TestRunnerNormalizesModelsFailureWithoutSuccessfulOutput` | A real OMNI result arriving with an artifact through the runner |
| Work/Factory Runtime | `pkg/services/factory_runtime/internal/services/orchestration/runtime/work_output_materialization_test.go`: `TestAcceptWorkersResultMaterializesDetachedProposalOnce`, `TestCanceledAttemptDoesNotMaterializeCompletedOutput`; runtime event-history tests cover ordered Work submissions and replay | OMNI-selected text, artifact identity, and lineage in one customer Factory Session |
| Recordings/replay | `pkg/services/recordings/internal/events/event_history_dispatch_test.go`: `TestFactoryEventHistory_RecordDispatchCompletion_PreservesSelectedClassificationLabel`, `TestFactoryEventHistory_RecordDispatchCompletion_PreservesStructuredSchemaViolationClassification`, `TestFactoryEventHistory_RecordDispatchCompletion_PreservesOutputWorkStateFromTokenPlace`; `event_history_lineage_test.go` and replay/artifact tests cover detached history behavior | An artifact-bearing OMNI OutputWork item and its replay equality |
| HTTP/CLI | HTTP `invoke_operations_test.go` covers named output, ASR output metadata, invalid artifact input, and typed failure mapping; CLI `http_protocol_test.go` includes `TestCLIOutputContentProjectionBranches` and artifact projection assertions, while `output_required_test.go` covers output publication/rollback | A transport observation sourced from a successful Factory Session OMNI artifact, rather than a direct fabricated result |

The inventory proves component capability and failure isolation. It does not
prove the requested customer behavior. In particular, a direct CLI/HTTP
Models invocation, a text-only response, a TTS path, a source inventory, or a
test that fabricates `InferenceOutput.Artifact` cannot satisfy the missing
Factory Session witness.

## 3. Supplied failure witness

The PRD supplies the following failed worker witness; it is retained without a
new runtime call or modification:

| Fact | Value |
| --- | --- |
| Category | `contract_conflict` |
| Classification | `scope_or_plan_failure; deterministic under localai-project-v1` |
| Current protocol shape | `type InvocationProtocolResponse struct { Text string; Usage string }` |
| Observed gap | OMNI returns text/usage only; the private content-only hop drops the fact before Workers, which assigns Work artifact identity only when `InferenceOutput.Artifact` exists. |
| Provider session | `codex/session_id/01a07036-a996-7a12-aa4f-7602eb55ff4e` |
| Work ID | `batch-localai-project-cycle-001-e10e3884-localai-factory-llm-lineage` |
| Worker session ID | `96b67964-3721-4d05-afd0-611f4a4f6f8b` |
| Decision | `FAILED` |

The witness is consistent with the source inspection: no public/durable shape
is required merely to move an artifact fact through the existing private
runtime result. Whether the source plan authorizes that private seam remains
unverifiable until the missing immutable files are supplied or the operator
issues a successor authority decision.

## 4. Story-001 evidence and handoff

### Procedure executed

From the current worktree:

1. `git rev-parse HEAD` ->
   `e10e38843aff30c7871b732b284976ee13ab42f1`.
2. `git ls-files docs/temp/projects/localai/source-plan.md docs/temp/projects/localai/request.md docs/temp/projects/localai/acceptance.md` -> no output; `Test-Path` for the directory is false.
3. Inspected the cited current symbols in Models, LocalAI, Models wire and
   lifecycle, Workers, Work/Factory Runtime, Recordings, and HTTP/CLI mappers.
4. Ran `rg -n '^func Test'` over the focused directories listed in §2.
5. Did not run runtime, network, real LocalAI, paid, or remote validation.

### Evidence boundary

- Proves: current ownership, current content-only OMNI hop, existing public
  artifact capability, downstream conditional mapping, Work lineage/event
  copying, focused coverage inventory, and the supplied deterministic failure
  interpretation.
- Does not prove: any Proposed implementation, compilation after a private
  return-shape change, artifact registration from OMNI, Factory Session
  admission/invocation/selection, Work identity or replay for OMNI, real
  LocalAI behavior, exportability, platform support, or Project acceptance.
- Remaining gates: private implementation -> `GATE-OMNI-PRIVATE`; integrated
  Work/Event/replay -> `GATE-FACTORY-LINEAGE`; operator authority ->
  `GATE-OPERATOR-APPROVAL`; later semantic criteria -> `LA-05`/`LA-06`.

### Smallest next step

Supply the immutable source-plan/request/acceptance authority, or record an
operator-authorized successor decision that explicitly permits the proposal to
continue. After that authority check, story 002 may render the exact private
Current/Proposed handoff and unchanged public/OpenAPI shapes. It must not add a
public field, generated artifact, protobuf field, CLI grammar, Factory graph,
artifact store, or implementation without a new approved contract revision.

