# LocalAI OMNI-to-Factory artifact contract delta

Status: proposal-only private contract delta, story
`localai-omni-artifact-contract-delta-authority-retry-003`. This document
retains the verified current path, the reviewable Proposed private handoff, and
the complete selected semantic Factory acceptance matrix. It does not authorize
production change, executable test change, generated-artifact change, or Project
acceptance.

## 0. Authority and scope boundary

- The pre-edit checkout/current application head was
  `e10e38843aff30c7871b732b284976ee13ab42f1`; the proposal commits are
  documentation-only descendants of that head.
- The execution packet is `prd.json` for
  `localai-omni-artifact-contract-delta`; no `operatorAmendment` is present.
- The immutable authority was read from the supplied absolute paths and was
  verified before any proposal edit:

  | Authority | Absolute path | SHA-256 |
  | --- | --- | --- |
  | Source plan | `C:\Users\andre\work\portos\infinite-you\docs\temp\projects\localai\source-plan.md` | `0c81aac27358bff4014ee623f13f5255d07ab67798ad6d306fc7e1a7f2af972e` |
  | Request | `C:\Users\andre\work\portos\infinite-you\docs\temp\projects\localai\request.md` | `1562b040348625dc1a608011e13458d97cc5b600f4d95dd8bf98cf1dbe2da52c` |
  | Acceptance | `C:\Users\andre\work\portos\infinite-you\docs\temp\projects\localai\acceptance.md` | `83c1368c05d1f84e7c61ca5632ad9422b33d646c7c4166d7fd5812764fd18172` |

  `Get-FileHash -Algorithm SHA256` produced the recorded value for each
  path. The same three paths and values are retained in `prd.json` under
  `immutableAuthority`; neither artifact is authority for rewriting the other.
- The supplied authority files remain outside this checkout and were not
  copied, edited, rewritten, or committed. Their absence from the worktree is
  therefore no longer an authority blocker because the operator supplied and
  the preflight verified the absolute read-only paths.
- The reconciliation found a clean task worktree, no open PR for this branch,
  and no tracked application changes before the proposal commits. Relevant
  existing refs were retained and inspected: `main`,
  `localai-omni-artifact-contract-delta`, and the active
  `localai-recover-real-inference` lane. The untracked main-workspace draft
  named by the PRD was not copied or overwritten.
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
runtime result. The supplied authority preserves the private-only boundary;
operator approval or the existing-private-contract implementation gate is
still required before any implementation work.

## 4. Story-001 evidence and handoff

### Procedure executed

From the current worktree, before the proposal edit:

1. `git rev-parse HEAD` ->
   `e10e38843aff30c7871b732b284976ee13ab42f1`.
2. For each supplied absolute authority path, `Test-Path` succeeded and
   `Get-FileHash -Algorithm SHA256` matched the exact value in §0.
3. `git status --short --branch` was clean at the application head; `gh pr
   list --state all --head
   localai-omni-artifact-contract-delta-authority-retry` returned `[]`.
4. Inspected the cited current symbols in Models, LocalAI, Models wire and
   lifecycle, Workers, Work/Factory Runtime, Recordings, and HTTP/CLI mappers.
5. Ran `rg -n '^func Test'` over the focused directories listed in §2. The
   focused counts are retained in `prd.json.coverageSignal`; this is an
   inventory signal, not behavior proof.
6. Did not run runtime, network, real LocalAI, paid, or remote validation.

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

Story 002 renders the exact private Current/Proposed handoff and unchanged
public/OpenAPI shapes in §5. Story 003 defines the semantic Factory acceptance
matrix against this preserved private-only boundary in §6. The next bounded
story is story 004: independent clean-room proposal validation. No
implementation, public field, generated artifact, protobuf field, CLI grammar,
Factory graph, artifact store, or runtime test is admitted without the required
operator/private-contract gate.

## 5. Story 002 — render the bounded private OMNI artifact handoff

**Parent behavior:** `BEH-PRIVATE` — OMNI semantic text and detached artifact
metadata cross the private Models seam and reach the existing registrar.

**Problem:** The current OMNI codec returns only named content and the wire
adapter drops the existing artifact-capable runtime field, so Workers cannot
preserve artifact identity even though Models, Work, and Recordings already
have compatible downstream shapes.

**Outcome:** A self-contained, exact, proposal-only private delta defines one
detached `text/plain` artifact descriptor with UTF-8 byte size, zero identity
at the codec boundary, forwarding through the existing runtime result, and
identity assignment only by the Models Inference registrar. All public Go,
OpenAPI, generated, protobuf, CLI, Work, event, and Factory shapes remain
unchanged.

**Plan reference:** The immutable source-plan snapshot at
`C:\Users\andre\work\portos\infinite-you\docs\temp\projects\localai\source-plan.md`,
“Package interface changes”, “Service interactions”, “Failure modes”, and
“Bounded operator amendments — Contract and scope boundary”. The exact
authority tuple is recorded in §0 and `prd.json`.

**Actor and trigger:** A future Models implementation worker, after
`GATE-OPERATOR-APPROVAL` or the existing-private-contract implementation gate,
receives a validated OMNI request and a successful protocol response with
non-empty Unicode text. This iteration only records the contract the worker
would implement; it does not invoke a model, backend, server, or Factory
Session.

**Dependencies:**

- `localai-omni-artifact-contract-delta-authority-retry-001` — completed
  authority and characterization reconciliation.
- `GATE-OPERATOR-APPROVAL` — required before any production or executable-test
  change.

**Parallel and shared-surface ownership:** Models owns the future private
codec, private wire adapter, inference lifecycle, registrar, cache/backend
lifecycle, and release behavior. `pkg/services/models/wire` owns the private
adapter population. Workers consumes the existing public Models result;
Work, Factory Runtime, Sessions, and Recordings retain their existing
ownership. `pkg/wire`, OpenAPI authors, generated-client owners, CLI owners,
protobuf owners, and Factory graph owners receive no changes in this story.
No concurrent proposal task may edit this plan's private/public shape
inventory; story 003 owns the later matrix section.

**Scope:**

- In: exact private codec and wire Current/Proposed Go shapes; exact unchanged
  Models, runtime, Work, event, protobuf, OpenAPI, generated-consumer, and CLI
  shapes; registrar identity ownership; UTF-8 size semantics; atomic failure;
  release, redaction, compatibility, migration, rollout, rollback, and stop
  conditions.
- Out: public fields or methods; authored/generated OpenAPI; protobuf or CLI
  changes; persisted Work/Event/Factory graph changes; Models cache/backend or
  platform changes; `pkg/wire` production changes; implementation; executable
  tests; runtime, network, paid, server, blind, or platform validation.

**Implementation constraints:**

- Keep the private boundary detached. LocalAI protocol types remain inside
  `pkg/services/models/internal/backends/localai`; no LocalAI type, endpoint,
  process, cache path, source path, credential, signed URL, or raw protocol
  payload crosses the Models public boundary.
- The codec returns exact text content and exactly one text artifact
  descriptor. The descriptor name is `text`, media type is `text/plain`, and
  `SizeBytes` is `int64(len([]byte(response.Text)))`, not rune count and not a
  digest claim. Its `Artifact` reference is the zero value.
- The wire adapter forwards detached descriptor metadata through the existing
  `InvocationRuntimeResult.Artifacts` field by the existing
  `invocationArtifactSources` mapper. It does not assign identity, add a
  `SourcePath`, or release a lease.
- The Models Inference registrar is the only identity owner. An empty incoming
  reference receives its opaque `models-inference:artifact:<n>` reference;
  source paths remain an internal export concern and are absent from this OMNI
  descriptor. The lifecycle registers and normalizes before publishing a
  completed result.
- Blank, malformed, invalid, or oversized output returns a zero private
  result or a typed failure and publishes no successful `Outputs` or
  `Artifacts`. `finishFailedInvocation` remains the failure path and releases
  the lease; the codec and wire adapter must not add a second release.
- Non-OMNI requests keep the existing fallback. Existing request/result,
  Work/Event, public HTTP/CLI, and generated representations remain unchanged.
  If implementation needs a public, persisted, CLI, OpenAPI, protobuf, or
  Factory graph field, stop and request a new operator-approved contract
  delta with its own Current/Proposed native shapes.

### 5.1 Private OMNI codec result handoff

Authored source: `pkg/services/models/internal/backends/localai/omni_codec.go`.
Classification: proposed private implementation delta; authority gate required.
The current and Proposed blocks below retain the surrounding response and
method shape needed to determine the private result, output name, media type,
UTF-8 size, zero identity, usage handling, and zero-on-error behavior.

Current:

```go
package localai

type PredictResponse struct {
	Text  string
	Usage string
}

func (codec *OmniCodec) Invoke(
	ctx context.Context,
	request models.InvokeModelRequest,
	operations ...models.Operation,
) ([]models.InferenceContent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	predict, err := codec.Encode(request)
	if err != nil {
		return nil, err
	}
	if codec == nil || codec.client == nil {
		return nil, &models.InvocationFailure{
			Class:     models.InvocationFailureClassBackendProtocol,
			Model:     request.Model,
			Operation: models.OperationOMNI,
			Message:   "OMNI protocol client is unavailable",
			Cause:     models.ErrUnavailable,
		}
	}
	response, err := codec.client.Predict(ctx, predict)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.Text) == "" {
		return nil, &models.InvocationFailure{
			Class:     models.InvocationFailureClassMalformedResponse,
			Model:     request.Model,
			Operation: models.OperationOMNI,
			Slot:      "text",
			Message:   "OMNI response did not contain text output",
		}
	}
	content := []models.InferenceContent{{
		Name:        "text",
		Modality:    models.ModalityText,
		ContentType: "text/plain",
		MediaType:   "text/plain",
		Content:     response.Text,
	}}
	if strings.TrimSpace(response.Usage) != "" && declaresOutputSlot(operations, "usage") {
		content = append(content, models.InferenceContent{
			Name: "usage", Modality: models.ModalityJSON,
			ContentType: "application/json", MediaType: "application/json",
			Content: response.Usage,
		})
	}
	return content, nil
}
```

Proposed:

```go
package localai

type PredictResponse struct {
	Text  string
	Usage string
}

// OmniInvocationResult is private LocalAI/Models output. Artifact is zero
// here; the Models Inference registrar owns opaque identity.
type OmniInvocationResult struct {
	Content   []models.InferenceContent
	Artifacts []models.InferenceArtifact
}

func (codec *OmniCodec) Invoke(
	ctx context.Context,
	request models.InvokeModelRequest,
	operations ...models.Operation,
) (OmniInvocationResult, error) {
	if err := ctx.Err(); err != nil {
		return OmniInvocationResult{}, err
	}
	predict, err := codec.Encode(request)
	if err != nil {
		return OmniInvocationResult{}, err
	}
	if codec == nil || codec.client == nil {
		return OmniInvocationResult{}, &models.InvocationFailure{
			Class:     models.InvocationFailureClassBackendProtocol,
			Model:     request.Model,
			Operation: models.OperationOMNI,
			Message:   "OMNI protocol client is unavailable",
			Cause:     models.ErrUnavailable,
		}
	}
	response, err := codec.client.Predict(ctx, predict)
	if err != nil {
		return OmniInvocationResult{}, err
	}
	if strings.TrimSpace(response.Text) == "" {
		return OmniInvocationResult{}, &models.InvocationFailure{
			Class:     models.InvocationFailureClassMalformedResponse,
			Model:     request.Model,
			Operation: models.OperationOMNI,
			Slot:      "text",
			Message:   "OMNI response did not contain text output",
		}
	}
	content := []models.InferenceContent{{
		Name:        "text",
		Modality:    models.ModalityText,
		ContentType: "text/plain",
		MediaType:   "text/plain",
		Content:     response.Text,
	}}
	if strings.TrimSpace(response.Usage) != "" && declaresOutputSlot(operations, "usage") {
		content = append(content, models.InferenceContent{
			Name: "usage", Modality: models.ModalityJSON,
			ContentType: "application/json", MediaType: "application/json",
			Content: response.Usage,
		})
	}
	return OmniInvocationResult{
		Content: content,
		Artifacts: []models.InferenceArtifact{{
			Name:       "text",
			MediaType:  "text/plain",
			SizeBytes:  int64(len([]byte(response.Text))),
			Properties: nil,
		}},
	}, nil
}
```

The descriptor's omitted `Artifact` field is `models.InferenceArtifactRef{}`;
it is deliberately not parsed, generated, or guessed in the codec. The
`Content` and descriptor are returned together only after the non-empty text
check. `Usage` remains a separate JSON content slot when the declared output
contract contains `usage`; it does not receive the text artifact. `PredictResponse`,
`ProtocolClient`, `OmniCodec`, request encoding, capability validation, and
typed failure classes otherwise remain as characterized. No generated output
is produced by this private Go change. Consumers are the LocalAI codec package,
the Models wire adapter, and Models Inference lifecycle.

### 5.2 Private OMNI wire handoff

Authored source: `pkg/services/models/wire/wire.go`; existing runtime carrier:
`pkg/services/models/internal/services/inference/runtime_contract.go`.
Classification: proposed private caller-population delta; authority gate
required.

Current:

```go
func (runtime omniInvocationRuntime) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if !isOMNIOperation(request) {
		return runtime.fallback.Invoke(ctx, request)
	}
	if runtime.codec == nil {
		return inference.InvocationRuntimeResult{}, models.ErrUnavailable
	}
	ctx = localai.WithInvocationEndpoint(ctx, request.HostSlot.Endpoint)
	content, err := runtime.codec.Invoke(ctx, request.Request, request.Operation)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	return inference.InvocationRuntimeResult{Content: content}, nil
}
```

Proposed:

```go
func (runtime omniInvocationRuntime) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if !isOMNIOperation(request) {
		return runtime.fallback.Invoke(ctx, request)
	}
	if runtime.codec == nil {
		return inference.InvocationRuntimeResult{}, models.ErrUnavailable
	}
	ctx = localai.WithInvocationEndpoint(ctx, request.HostSlot.Endpoint)
	omniResult, err := runtime.codec.Invoke(ctx, request.Request, request.Operation)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	return inference.InvocationRuntimeResult{
		Content:   omniResult.Content,
		Artifacts: invocationArtifactSources(omniResult.Artifacts),
	}, nil
}
```

This uses the existing `InvocationRuntimeResult.Artifacts` and
`invocationArtifactSources`; that mapper copies the descriptor's zero
reference into `RefValue`, copies only detached name/media/size/properties,
and leaves `SourcePath` empty. It introduces no endpoint/path/secret field and
does not alter fallback routing, context cancellation, or release ownership.
Consumers are `pkg/services/models/wire` and the Models Inference lifecycle.
No generated output is produced.

### 5.3 Unchanged private registrar and release lifecycle

Authored sources:
`pkg/services/models/internal/services/inference/internal/artifacts/registrar.go`
and `pkg/services/models/internal/services/inference/internal/service/`.
These are unchanged context blocks, included to make identity ownership,
failure atomicity, and release behavior explicit.

Current:

```go
func (r *Registrar) Register(source inference.InvocationArtifactSource) (models.InferenceArtifact, error) {
	if r == nil {
		return models.InferenceArtifact{}, models.ErrUnavailable
	}
	rawRef := source.RefValue
	if rawRef != "" && strings.TrimSpace(rawRef) == "" {
		return models.InferenceArtifact{}, models.ErrInferenceArtifactInvalid
	}
	refValue := strings.TrimSpace(rawRef)
	if refValue == "" {
		r.mu.Lock()
		r.nextID++
		refValue = fmt.Sprintf("models-inference:artifact:%d", r.nextID)
		r.mu.Unlock()
	}
	ref, err := (models.InferenceArtifactRef{}).Parse(refValue)
	if err != nil {
		return models.InferenceArtifact{}, err
	}
	if strings.TrimSpace(source.SourcePath) != "" {
		r.mu.Lock()
		r.sources[ref.String()] = source.SourcePath
		r.mu.Unlock()
	}
	return models.InferenceArtifact{
		Artifact:   ref,
		Name:       source.Name,
		MediaType:  source.MediaType,
		SizeBytes:  source.SizeBytes,
		Properties: cloneStringMap(source.Properties),
	}.Clone(), nil
}
```

Proposed:

```go
func (r *Registrar) Register(source inference.InvocationArtifactSource) (models.InferenceArtifact, error) {
	if r == nil {
		return models.InferenceArtifact{}, models.ErrUnavailable
	}
	rawRef := source.RefValue
	if rawRef != "" && strings.TrimSpace(rawRef) == "" {
		return models.InferenceArtifact{}, models.ErrInferenceArtifactInvalid
	}
	refValue := strings.TrimSpace(rawRef)
	if refValue == "" {
		r.mu.Lock()
		r.nextID++
		refValue = fmt.Sprintf("models-inference:artifact:%d", r.nextID)
		r.mu.Unlock()
	}
	ref, err := (models.InferenceArtifactRef{}).Parse(refValue)
	if err != nil {
		return models.InferenceArtifact{}, err
	}
	if strings.TrimSpace(source.SourcePath) != "" {
		r.mu.Lock()
		r.sources[ref.String()] = source.SourcePath
		r.mu.Unlock()
	}
	return models.InferenceArtifact{
		Artifact:   ref,
		Name:       source.Name,
		MediaType:  source.MediaType,
		SizeBytes:  source.SizeBytes,
		Properties: cloneStringMap(source.Properties),
	}.Clone(), nil
}
```

The registrar is the sole owner of opaque output identity. For this OMNI
descriptor, `SourcePath` is empty, so registration does not expose or retain a
backend/cache path for export. A later implementation must preserve the
existing `finishCompletedInvocation` ordering: register all sources, normalize
generic outputs, then store a completed detached result and release the lease.
Any registration or normalization error goes through `finishFailedInvocation`,
stores a failed/cancelled result with no successful output, and calls
`releaseInvocationLease` exactly once. `context.WithoutCancel` remains the
release context. The host/lease service owns capacity; codec and wire do not
release it. These blocks are lifecycle evidence targets for `GATE-OMNI-PRIVATE`
and `GATE-RELEASE-RACE`, not evidence that this proposal has executed them.

### 5.4 Exact unchanged public, runtime, Work, event, and protocol shapes

The following authored Go/protocol shapes are unchanged. Their paired blocks
are intentionally concrete so the reviewer can verify that the private delta
does not require a public or persisted contract change.

#### Public Models request

Authored source: `pkg/services/models/local_execution_contract.go`.

Current:

```go
type ModelReference struct {
	NameOrURI string
}

type InferenceInput struct {
	Name        string
	Modality    Modality
	ContentType string
	MediaType   string
	Content     string
	Artifact    *InferenceArtifactRef
}

type InvokeModelRequest struct {
	Scope        RuntimeScopeRef
	Lease        ModelLeaseRef
	Holder       string
	ModelName    string
	Model        ModelReference
	Operation    string
	ResponseMode ResponseMode
	Input        InferenceInput
	Inputs       []InferenceInput
	Parameters   []OperationParameter
	OutputMode   OutputMode
	Offline      bool
}
```

Proposed:

```go
type ModelReference struct {
	NameOrURI string
}

type InferenceInput struct {
	Name        string
	Modality    Modality
	ContentType string
	MediaType   string
	Content     string
	Artifact    *InferenceArtifactRef
}

type InvokeModelRequest struct {
	Scope        RuntimeScopeRef
	Lease        ModelLeaseRef
	Holder       string
	ModelName    string
	Model        ModelReference
	Operation    string
	ResponseMode ResponseMode
	Input        InferenceInput
	Inputs       []InferenceInput
	Parameters   []OperationParameter
	OutputMode   OutputMode
	Offline      bool
}
```

Compatibility: `ModelReference`, `InferenceInput`, and `InvokeModelRequest`
remain unchanged; this private delta begins after request preparation.
Consumers: Models generic invocation, LocalAI codec, Workers, and Models
transports. No generated output.

#### Public Models output and lifecycle

Authored source: `pkg/services/models/local_execution_contract.go`.

Current:

```go
type InferenceArtifactRef struct {
	value string
}

func (InferenceArtifactRef) Parse(value string) (InferenceArtifactRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return InferenceArtifactRef{}, ErrInferenceArtifactInvalid
	}
	return InferenceArtifactRef{value: value}, nil
}

func (ref InferenceArtifactRef) String() string {
	return ref.value
}

func (ref InferenceArtifactRef) IsZero() bool {
	return strings.TrimSpace(ref.value) == ""
}

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

Proposed:

```go
type InferenceArtifactRef struct {
	value string
}

func (InferenceArtifactRef) Parse(value string) (InferenceArtifactRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return InferenceArtifactRef{}, ErrInferenceArtifactInvalid
	}
	return InferenceArtifactRef{value: value}, nil
}

func (ref InferenceArtifactRef) String() string {
	return ref.value
}

func (ref InferenceArtifactRef) IsZero() bool {
	return strings.TrimSpace(ref.value) == ""
}

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

Compatibility: existing `InferenceOutput`, `InferenceArtifact`, and
`InvokeModelResult` are sufficient; no field, tag, or generated-client change.
The existing generated consumers remain `api/openapi.yaml`,
`pkg/transports/http/generated/server.gen.go`,
`pkg/transports/http/client/client.gen.go`, and
`ui/src/api/generated/openapi.ts`; none is regenerated by this story.
Consumers are Models Inference, Workers, HTTP/CLI mappers, and public Models
callers.

#### Public Models failure shape

Authored source: `pkg/services/models/inference_failure.go`.

Current:

```go
type InvocationFailure struct {
	Class      InvocationFailureClass
	Message    string
	Model      ModelReference
	Operation  string
	Slot       string
	Parameter  string
	Field      string
	ValidNames []string
	Cause      error
}
```

Proposed:

```go
type InvocationFailure struct {
	Class      InvocationFailureClass
	Message    string
	Model      ModelReference
	Operation  string
	Slot       string
	Parameter  string
	Field      string
	ValidNames []string
	Cause      error
}
```

Compatibility: the typed failure taxonomy and customer-safe `Error` behavior
remain unchanged; `Cause` remains internal to Go error unwrapping. Consumers
are Models validation/lifecycle, HTTP/CLI mappers, and public Models callers.
No generated output.

#### Provider-neutral protocol request and response

Authored source: `pkg/services/models/managed_runtime_contract.go`.

Current:

```go
type InvocationProtocolRequest struct {
	Operation  string
	Prompt     string
	Inputs     []InvocationProtocolInput
	Parameters []OperationParameter
}

type InvocationProtocolInput struct {
	Slot      string
	Modality  Modality
	MediaType string
	Content   string
	Reference string
}

type InvocationProtocolResponse struct {
	Text  string
	Usage string
}
```

Proposed:

```go
type InvocationProtocolRequest struct {
	Operation  string
	Prompt     string
	Inputs     []InvocationProtocolInput
	Parameters []OperationParameter
}

type InvocationProtocolInput struct {
	Slot      string
	Modality  Modality
	MediaType string
	Content   string
	Reference string
}

type InvocationProtocolResponse struct {
	Text  string
	Usage string
}
```

Compatibility: response remains `Text`/`Usage`; a needed public artifact
field is a stop/new-revision condition. Consumers are Models wire, controlled
protocol fixtures, and provider-neutral callers. No generated output.

#### Private inference runtime result

Authored source:
`pkg/services/models/internal/services/inference/runtime_contract.go`.

Current:

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

Proposed:

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

Compatibility: the existing `Artifacts` field is populated for OMNI;
`SourcePath` remains private and empty for this descriptor. Consumers are the
Models wire adapter and Models Inference lifecycle. No generated output.

#### Work content, identity, event-work, and lineage

Authored sources: `pkg/services/work/contracts.go` and
`pkg/services/work/proposal_materialization_contract.go`.

Current:

```go
type Work struct {
	Name                     string               `json:"name"`
	WorkID                   string               `json:"workId,omitempty"`
	RequestID                string               `json:"requestId,omitempty"`
	WorkTypeID               string               `json:"workTypeName,omitempty"`
	State                    string               `json:"state,omitempty"`
	ChainingTraceDepth       int                  `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string               `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string             `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string               `json:"traceId,omitempty"`
	Content                  []WorkContentPart    `json:"content,omitempty"`
	Payload                  any                  `json:"payload,omitempty"`
	Tags                     map[string]string    `json:"tags,omitempty"`
	ExecutionID              string               `json:"-"`
	RuntimeRelations         []Relation           `json:"-"`
	InvocationArguments      *InvocationArguments `json:"-"`
}

type WorkContentPart struct {
	Type        WorkContentPartType `json:"type"`
	Text        string              `json:"text,omitempty"`
	URL         string              `json:"url,omitempty"`
	File        string              `json:"file,omitempty"`
	JSON        json.RawMessage     `json:"json,omitempty"`
	Slot        string              `json:"slot,omitempty"`
	Label       string              `json:"label,omitempty"`
	Role        string              `json:"role,omitempty"`
	ContentType string              `json:"contentType,omitempty"`
	ArtifactID  string              `json:"artifactId,omitempty"`
	Metadata    map[string]any      `json:"metadata,omitempty"`
}

type WorkRequestEventWork struct {
	Name                     string            `json:"name"`
	WorkID                   string            `json:"workId,omitempty"`
	RequestID                string            `json:"requestId,omitempty"`
	WorkTypeID               string            `json:"workTypeName,omitempty"`
	State                    *WorkEventState   `json:"state,omitempty"`
	ChainingTraceDepth       int               `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId,omitempty"`
	Content                  []WorkContentPart `json:"content,omitempty"`
	Payload                  json.RawMessage   `json:"payload,omitempty"`
	StructuredResult         any               `json:"structuredResult,omitempty"`
	Tags                     map[string]string `json:"tags,omitempty"`
	StructuredResultPresent  bool              `json:"-"`
}

type MaterializationLineageContext struct {
	DispatchID               string
	RequestID                string
	SourceWorkIDs            []string
	CurrentChainingTraceID   string
	PreviousChainingTraceIDs []string
	ChainingTraceDepth       int
	ParentWorkID             string
	TraceID                  string
}
```

Proposed:

```go
type Work struct {
	Name                     string               `json:"name"`
	WorkID                   string               `json:"workId,omitempty"`
	RequestID                string               `json:"requestId,omitempty"`
	WorkTypeID               string               `json:"workTypeName,omitempty"`
	State                    string               `json:"state,omitempty"`
	ChainingTraceDepth       int                  `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string               `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string             `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string               `json:"traceId,omitempty"`
	Content                  []WorkContentPart    `json:"content,omitempty"`
	Payload                  any                  `json:"payload,omitempty"`
	Tags                     map[string]string    `json:"tags,omitempty"`
	ExecutionID              string               `json:"-"`
	RuntimeRelations         []Relation           `json:"-"`
	InvocationArguments      *InvocationArguments `json:"-"`
}

type WorkContentPart struct {
	Type        WorkContentPartType `json:"type"`
	Text        string              `json:"text,omitempty"`
	URL         string              `json:"url,omitempty"`
	File        string              `json:"file,omitempty"`
	JSON        json.RawMessage     `json:"json,omitempty"`
	Slot        string              `json:"slot,omitempty"`
	Label       string              `json:"label,omitempty"`
	Role        string              `json:"role,omitempty"`
	ContentType string              `json:"contentType,omitempty"`
	ArtifactID  string              `json:"artifactId,omitempty"`
	Metadata    map[string]any      `json:"metadata,omitempty"`
}

type WorkRequestEventWork struct {
	Name                     string            `json:"name"`
	WorkID                   string            `json:"workId,omitempty"`
	RequestID                string            `json:"requestId,omitempty"`
	WorkTypeID               string            `json:"workTypeName,omitempty"`
	State                    *WorkEventState   `json:"state,omitempty"`
	ChainingTraceDepth       int               `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId,omitempty"`
	Content                  []WorkContentPart `json:"content,omitempty"`
	Payload                  json.RawMessage   `json:"payload,omitempty"`
	StructuredResult         any               `json:"structuredResult,omitempty"`
	Tags                     map[string]string `json:"tags,omitempty"`
	StructuredResultPresent  bool              `json:"-"`
}

type MaterializationLineageContext struct {
	DispatchID               string
	RequestID                string
	SourceWorkIDs            []string
	CurrentChainingTraceID   string
	PreviousChainingTraceIDs []string
	ChainingTraceDepth       int
	ParentWorkID             string
	TraceID                  string
}
```

Compatibility: `ArtifactID`, content, Work identity, event-work fields, and
lineage fields are preserved. Work and Recordings retain ownership; no Work,
event, or Factory graph schema is added. Consumers are Workers, Factory
Runtime, Factory Sessions, Work, and Recordings. No generated output.

#### Canonical dispatch response event

Authored source: `pkg/services/workers/execution_contracts.go`.

Current:

```go
type DispatchResponseEventPayload struct {
	CompletionID                *string                       `json:"completionId,omitempty"`
	CurrentChainingTraceID      *string                       `json:"currentChainingTraceId,omitempty"`
	Cancellation                *DispatchCancellation         `json:"cancellation,omitempty"`
	DurationMillis              *int64                        `json:"durationMillis,omitempty"`
	Error                       *string                       `json:"error,omitempty"`
	ArtifactVerification        *ExpectedArtifactVerification `json:"artifactVerification,omitempty"`
	FailureDetail               *FailureDetail                `json:"failureDetail,omitempty"`
	Feedback                    *string                       `json:"feedback,omitempty"`
	Metadata                    map[string]string             `json:"metadata,omitempty"`
	Metrics                     *WorkMetricsEventPayload      `json:"metrics,omitempty"`
	Outcome                     WorkOutcome                   `json:"outcome"`
	Output                      *string                       `json:"output,omitempty"`
	OutputResources             *[]DispatchResourceEventRef   `json:"outputResources,omitempty"`
	OutputWork                  *[]work.WorkRequestEventWork  `json:"outputWork,omitempty"`
	StructuredResult            any                           `json:"structuredResult,omitempty"`
	PreviousChainingTraceIDs    *[]string                     `json:"previousChainingTraceIds,omitempty"`
	ProviderFailure             *WorkFailureMetadata          `json:"providerFailure,omitempty"`
	SelectedClassificationLabel *string                       `json:"selectedClassificationLabel,omitempty"`
	TransitionID                string                        `json:"transitionId"`
	Usage                       *DispatchUsageEventPayload    `json:"usage,omitempty"`
	StructuredResultPresent     bool                          `json:"-"`
}
```

Proposed:

```go
type DispatchResponseEventPayload struct {
	CompletionID                *string                       `json:"completionId,omitempty"`
	CurrentChainingTraceID      *string                       `json:"currentChainingTraceId,omitempty"`
	Cancellation                *DispatchCancellation         `json:"cancellation,omitempty"`
	DurationMillis              *int64                        `json:"durationMillis,omitempty"`
	Error                       *string                       `json:"error,omitempty"`
	ArtifactVerification        *ExpectedArtifactVerification `json:"artifactVerification,omitempty"`
	FailureDetail               *FailureDetail                `json:"failureDetail,omitempty"`
	Feedback                    *string                       `json:"feedback,omitempty"`
	Metadata                    map[string]string             `json:"metadata,omitempty"`
	Metrics                     *WorkMetricsEventPayload      `json:"metrics,omitempty"`
	Outcome                     WorkOutcome                   `json:"outcome"`
	Output                      *string                       `json:"output,omitempty"`
	OutputResources             *[]DispatchResourceEventRef   `json:"outputResources,omitempty"`
	OutputWork                  *[]work.WorkRequestEventWork  `json:"outputWork,omitempty"`
	StructuredResult            any                           `json:"structuredResult,omitempty"`
	PreviousChainingTraceIDs    *[]string                     `json:"previousChainingTraceIds,omitempty"`
	ProviderFailure             *WorkFailureMetadata          `json:"providerFailure,omitempty"`
	SelectedClassificationLabel *string                       `json:"selectedClassificationLabel,omitempty"`
	TransitionID                string                        `json:"transitionId"`
	Usage                       *DispatchUsageEventPayload    `json:"usage,omitempty"`
	StructuredResultPresent     bool                          `json:"-"`
}
```

Compatibility: `OutputWork` already carries `WorkRequestEventWork`; canonical
Factory Event ordering, replay, and lineage remain unchanged. Consumers are
Factory Runtime, Recordings, and event replay/projections. No generated
output.

### 5.5 Exact unchanged public OpenAPI, generated, protobuf, and CLI shapes

The following are authored public or protocol sources. Each Current/Proposed
pair is unchanged. The generated files listed after each relevant OpenAPI
pair are consumers to verify, not files to edit. This story intentionally
produces no authored or generated API diff.

#### Pinned LocalAI subset protocol

Authored source:
`pkg/services/models/internal/backends/localai/backend_subset.proto`.

Current:

```proto
syntax = "proto3";

option go_package = "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai;localai";

package backend;

message HealthMessage {}

message PredictOptions {
  string Prompt = 1;
  repeated string Images = 42;
  repeated string Videos = 45;
  repeated string Audios = 46;
  map<string, string> Metadata = 52;
}

message Reply {
  bytes message = 1;
  int32 tokens = 2;
  int32 prompt_tokens = 3;
}
```

Proposed:

```proto
syntax = "proto3";

option go_package = "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai;localai";

package backend;

message HealthMessage {}

message PredictOptions {
  string Prompt = 1;
  repeated string Images = 42;
  repeated string Videos = 45;
  repeated string Audios = 46;
  map<string, string> Metadata = 52;
}

message Reply {
  bytes message = 1;
  int32 tokens = 2;
  int32 prompt_tokens = 3;
}
```

Compatibility: protobuf field numbers and generated private protocol output
remain unchanged. The LocalAI backend package is the only consumer. No
protobuf regeneration.

#### `POST /models/invocations`

Authored source: `api/openapi-main.yaml`.

Current:

```yaml
/models/invocations:
  post:
    tags:
      - Models
    operationId: invokeGenericModel
    summary: Invoke a model through the generic contract
    description: Invokes a configured model or supported source reference through the provider-neutral Models contract. Ordered inputs and named outputs remain detached from backend and cache details.
    requestBody:
      required: true
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/GenericModelInvocationRequest'
    responses:
      '200':
        description: Successful generic model invocation result.
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/GenericModelInvocationResponse'
      '400':
        $ref: '#/components/responses/BadRequest'
      '404':
        $ref: '#/components/responses/NotFound'
      '500':
        $ref: '#/components/responses/InternalError'
```

Proposed:

```yaml
/models/invocations:
  post:
    tags:
      - Models
    operationId: invokeGenericModel
    summary: Invoke a model through the generic contract
    description: Invokes a configured model or supported source reference through the provider-neutral Models contract. Ordered inputs and named outputs remain detached from backend and cache details.
    requestBody:
      required: true
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/GenericModelInvocationRequest'
    responses:
      '200':
        description: Successful generic model invocation result.
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/GenericModelInvocationResponse'
      '400':
        $ref: '#/components/responses/BadRequest'
      '404':
        $ref: '#/components/responses/NotFound'
      '500':
        $ref: '#/components/responses/InternalError'
```

Compatibility: operation, request, success envelope, and typed errors remain
unchanged. Generated consumers to verify are `api/openapi.yaml`,
`pkg/transports/http/generated/server.gen.go`,
`pkg/transports/http/client/client.gen.go`, and
`ui/src/api/generated/openapi.ts`; the Models HTTP transport and generated Go
and TypeScript clients remain unchanged.

#### `GenericModelInvocationResponse`

Authored source:
`api/components/schemas/api/GenericModelInvocationResponse.yaml`.

Current:

```yaml
type: object
additionalProperties: false
required:
  - outputs
description: Provider-neutral generic model invocation result with ordered slot-named outputs.
properties:
  outputs:
    type: array
    description: Ordered outputs. Distinct slots such as ASR transcript and segments remain separate entries.
    items:
      $ref: './ModelInvocationOutput.yaml'
  failure:
    $ref: './ModelInvocationFailure.yaml'
    description: Optional typed failure when a transport chooses an envelope rather than an error response.
```

Proposed:

```yaml
type: object
additionalProperties: false
required:
  - outputs
description: Provider-neutral generic model invocation result with ordered slot-named outputs.
properties:
  outputs:
    type: array
    description: Ordered outputs. Distinct slots such as ASR transcript and segments remain separate entries.
    items:
      $ref: './ModelInvocationOutput.yaml'
  failure:
    $ref: './ModelInvocationFailure.yaml'
    description: Optional typed failure when a transport chooses an envelope rather than an error response.
```

Compatibility: ordered outputs and optional typed failure remain unchanged.
Generated consumers: the bundled OpenAPI plus generated Go server/client and
TypeScript client listed above. HTTP/CLI mappers remain unchanged.

#### `ModelInvocationOutput`

Authored source: `api/components/schemas/api/ModelInvocationOutput.yaml`.

Current:

```yaml
type: object
additionalProperties: false
required:
  - name
  - modality
description: One ordered, slot-named output of a generic model invocation.
properties:
  name:
    type: string
    minLength: 1
    description: Output slot name declared by the selected operation.
  modality:
    $ref: './ModelInvocationContentType.yaml'
    description: Provider-neutral modality of the output.
  contentType:
    type: string
    description: Logical content type retained for compatibility with prepared invocation outputs.
  mediaType:
    type: string
    description: Concrete MIME type for media or file-backed output, when known.
  content:
    type: string
    description: Inline output content. JSON values are carried as their canonical JSON text.
  artifact:
    $ref: './ModelInvocationArtifact.yaml'
    description: Optional opaque artifact metadata for materialized output.
```

Proposed:

```yaml
type: object
additionalProperties: false
required:
  - name
  - modality
description: One ordered, slot-named output of a generic model invocation.
properties:
  name:
    type: string
    minLength: 1
    description: Output slot name declared by the selected operation.
  modality:
    $ref: './ModelInvocationContentType.yaml'
    description: Provider-neutral modality of the output.
  contentType:
    type: string
    description: Logical content type retained for compatibility with prepared invocation outputs.
  mediaType:
    type: string
    description: Concrete MIME type for media or file-backed output, when known.
  content:
    type: string
    description: Inline output content. JSON values are carried as their canonical JSON text.
  artifact:
    $ref: './ModelInvocationArtifact.yaml'
    description: Optional opaque artifact metadata for materialized output.
```

Compatibility: existing slot, modality, content, media, and optional-artifact
representation is sufficient. Generated Go/TypeScript clients and HTTP/CLI
mappers remain unchanged.

#### `ModelInvocationArtifact`

Authored source: `api/components/schemas/api/ModelInvocationArtifact.yaml`.

Current:

```yaml
type: object
additionalProperties: false
required:
  - artifactRef
properties:
  artifactRef:
    type: string
    minLength: 1
    description: Opaque Models-owned artifact reference; cache paths and storage handles are never exposed.
  name:
    type: string
    description: Customer-visible artifact name, when available.
  mediaType:
    type: string
    description: MIME type of the materialized artifact, when known.
  sizeBytes:
    type: integer
    format: int64
    minimum: 0
    description: Artifact size in bytes, when known.
  properties:
    $ref: '../shared/StringMap.yaml'
    description: Safe provider-neutral artifact metadata.
```

Proposed:

```yaml
type: object
additionalProperties: false
required:
  - artifactRef
properties:
  artifactRef:
    type: string
    minLength: 1
    description: Opaque Models-owned artifact reference; cache paths and storage handles are never exposed.
  name:
    type: string
    description: Customer-visible artifact name, when available.
  mediaType:
    type: string
    description: MIME type of the materialized artifact, when known.
  sizeBytes:
    type: integer
    format: int64
    minimum: 0
    description: Artifact size in bytes, when known.
  properties:
    $ref: '../shared/StringMap.yaml'
    description: Safe provider-neutral artifact metadata.
```

Compatibility: existing opaque ref, name, media type, size, and safe
properties are sufficient. Generated Go/TypeScript clients, HTTP mapping, and
CLI projection remain unchanged; no path or cache handle is added.

#### `ModelInvocationFailure`

Authored source: `api/components/schemas/api/ModelInvocationFailure.yaml`.

Current:

```yaml
type: object
additionalProperties: false
required:
  - class
  - message
description: Customer-safe typed failure for a generic model invocation.
properties:
  class:
    $ref: './ModelInvocationFailureClass.yaml'
  message:
    type: string
    minLength: 1
    description: Actionable failure explanation without cache paths, backend addresses, credentials, or protocol payloads.
  model:
    $ref: '../data-models/ModelReference.yaml'
  operation:
    type: string
    description: Operation associated with the failure, when known.
  slot:
    type: string
    description: Input or output slot associated with the failure, when known.
  parameter:
    type: string
    description: Parameter associated with the failure, when known.
  field:
    type: string
    description: Public request field associated with the failure, when known.
```

Proposed:

```yaml
type: object
additionalProperties: false
required:
  - class
  - message
description: Customer-safe typed failure for a generic model invocation.
properties:
  class:
    $ref: './ModelInvocationFailureClass.yaml'
  message:
    type: string
    minLength: 1
    description: Actionable failure explanation without cache paths, backend addresses, credentials, or protocol payloads.
  model:
    $ref: '../data-models/ModelReference.yaml'
  operation:
    type: string
    description: Operation associated with the failure, when known.
  slot:
    type: string
    description: Input or output slot associated with the failure, when known.
  parameter:
    type: string
    description: Parameter associated with the failure, when known.
  field:
    type: string
    description: Public request field associated with the failure, when known.
```

Compatibility: typed failure taxonomy and customer-safe message remain
unchanged. `Cause` remains internal to Go error unwrapping. Generated Go and
TypeScript clients remain unchanged; HTTP/CLI mappers continue to omit unsafe
paths, addresses, secrets, and raw protocol content.

#### CLI invocation grammar

Authored sources: the immutable source-plan CLI section plus
`pkg/services/models/transports/cli/models.go` and
`pkg/services/models/transports/cli/root_invoke.go`.

Current:

```text
you models invoke <model-or-source> [--operation <name>]
  [--input <slot>=<value>]...
  [--param <name>=<value>]...
  [--output <slot>=<path>]...
  [--json]
  [--offline]
```

Proposed:

```text
you models invoke <model-or-source> [--operation <name>]
  [--input <slot>=<value>]...
  [--param <name>=<value>]...
  [--output <slot>=<path>]...
  [--json]
  [--offline]
```

Compatibility: no flag, alias, path, server, or error grammar changes. The
explicit `--server` parity gate remains LA-07. Consumers are direct CLI,
CLI projection, and the generic invocation adapter. No generated output.

### 5.6 Ownership, lifecycle, failure, redaction, and delivery policy

Ownership is intentionally narrow:

| Concern | Owner in the proposed path | Not owned by this delta |
| --- | --- | --- |
| OMNI protocol request/response mapping | `pkg/services/models/internal/backends/localai` | Public Models, Work, Events, OpenAPI |
| Detached artifact descriptor | LocalAI codec creates metadata with zero ref | Identity, cache path, export handle |
| Private runtime forwarding | `pkg/services/models/wire` | `pkg/wire` graph or public transport |
| Opaque identity and optional source registration | Models Inference registrar | LocalAI codec, Workers, Work, Recordings |
| Normalization and publication | Models Inference lifecycle | Direct transport fabrication |
| Work artifact ID and lineage | Work/Factory Runtime existing path | New Work/Event/graph fields |
| Canonical event and replay | Recordings existing ledger/projection | Process-local Events as canonical history |
| Model/backend cache and supervised host | Existing Models runtime owners; later implementation lane | This proposal and the OMNI codec |
| Lease, host capacity, shutdown | Models runtime host/Inference lifecycle | Codec/wire duplicate release |
| Public projection | Existing Models HTTP/CLI mappers and authored OpenAPI | New public/generated shape |

Normal lifecycle after approval is:

1. Existing generic request validation completes before backend work.
2. Existing OMNI capability validation and `Encode` produce the private
   request; protocol response returns detached text/usage.
3. Codec creates one `text`/`text/plain` descriptor with byte length and zero
   identity; wire copies it into the existing runtime artifact-source list.
4. Models Inference registers all sources, assigns the opaque identity, and
   normalizes the named text content with that metadata before publication.
5. The completed `InvokeModelResult` retains content, artifact metadata, named
   output, and `LeaseDisposition: RELEASED`; the existing release call frees
   host capacity exactly once.

Failure lifecycle is:

| Failure | Required result/state | Release and redaction rule |
| --- | --- | --- |
| Context cancellation/deadline | Zero private result; failed/cancelled Models result; no successful output | `finishFailedInvocation`/`finishCancelledInvocation` releases once using cancellation-independent context |
| Missing codec/client or protocol error | Typed backend/protocol failure; no output/artifact publication | Existing lifecycle releases once; error contains no endpoint or raw protocol payload |
| Blank/missing OMNI text | Typed malformed-response failure for slot `text`; zero private result | No registration/public success; release remains lifecycle-owned |
| Invalid descriptor reference/name/media or negative/oversized size | Typed artifact/output failure; no `Outputs`/`Artifacts` in the stored failure result | Registration/normalization is atomic from the caller's perspective; paths and addresses absent |
| Work materialization rejection later | Failed attempt and no completed output Work/event projection | Factory/Work owners retain existing failure route and lineage semantics |
| Non-OMNI request | Existing fallback result/error | No OMNI descriptor or lifecycle change |

Artifact identity is opaque and stable only after registrar assignment. The
descriptor claims media type and UTF-8 byte size; it does not claim a digest,
cache location, host address, or filesystem export path. Logs, errors, events,
HTTP, and CLI projections must retain only safe error class/message and stable
opaque IDs. `HF_TOKEN`, signed URLs, backend addresses, cache paths, source
paths, and protocol payloads remain forbidden in diagnostics and public
representations.

There is no migration or deployment in this proposal. After authority
approval, implementation is a private additive handoff behind the existing
OMNI path; no public consumer migration, schema regeneration, persisted-data
migration, or Factory graph rollout is needed. Rollback is a revert/disable of
the private codec/wire change followed by the existing host/lease shutdown
path; it must release active leases/hosts and must not rewrite recordings or
delete caches to conceal a failure. Stop immediately on authority mismatch,
unreadable authority, any public/persisted/CLI/protobuf/Factory shape need,
unsafe descriptor data, non-atomic publication, missing exactly-once release,
or missing lineage witness. Those conditions require a structured blocker and
the smallest new operator-approved plan delta.

### 5.7 Acceptance criteria for story 002

- [ ] Given non-empty Unicode OMNI text, the Proposed private codec returns
  exact text plus one named `text/plain` descriptor whose
  `SizeBytes == int64(len([]byte(text)))` and whose identity is zero before
  registration.
- [ ] Given the Proposed private result, the wire adapter places detached
  metadata in the existing `InvocationRuntimeResult.Artifacts`, with no
  `SourcePath`, endpoint, cache path, or secret, and leaves non-OMNI fallback
  behavior unchanged.
- [ ] Given blank, missing, malformed, invalid, negative, or oversized output,
  the planned lifecycle returns a typed failed/cancelled result atomically with
  no successful `Outputs`/`Artifacts`, and the existing Models host/lease
  release remains exactly once.
- [ ] Given the existing public/runtime/Work/event/protobuf/OpenAPI/CLI shapes,
  the paired blocks in §§5.4–5.5 are byte-for-byte equivalent in meaning and
  no authored or generated public artifact is in the intended diff.
- [ ] Given an implementation request for a public, persisted, CLI, protobuf,
  OpenAPI, or Factory graph field, the worker stops and emits a structured
  operator-approved contract-delta request rather than broadening this story.
- [ ] `GATE-SHAPES` reports the exact private delta, all unchanged public and
  generated-consumer shapes, ownership, compatibility, and proposal-only
  file boundary.

### 5.8 Verification and evidence boundary

**Behavioral witness:** The paired private blocks and unchanged-shape blocks
determine output naming, `text/plain` media, UTF-8 byte size, zero-at-codec
identity, registrar assignment, atomic publication, fallback, release, and
redaction ownership.

**Executable-spine effect:** `extend` — the proposal extends the retained
OMNI-to-Workers-to-Work-to-Recordings characterization with a bounded private
artifact handoff without changing the executable spine or claiming runtime
success.

**Required evidence:**

- Scope: documentation/contract review.
- Dependency fidelity: none; read-only source and proposal inspection.
- Exact procedure: re-run the three absolute-path SHA-256 checks; parse
  `prd.json`; compare each Current/Proposed block in §§5.1–5.5 with the cited
  source; run `git diff --check`; search both `prd.json` and this plan for
  `Current`, `Proposed`, `InvocationProtocolResponse`, `InvocationRuntimeResult`,
  `OpenAPI`, `generated`, `compatibility`, `rollback`, `artifact`, `lineage`,
  `release`, and `redact`; inspect the final diff file names against the
  proposal-only allowlist.
- Proves: exact private contract delta, unchanged public/generated boundary,
  ownership, failure/release policy, compatibility, and scope stop conditions.
- Does not prove: Go compilation, executable tests, registrar execution,
  backend/cache lifecycle, Factory Session admission/invocation, Work/Event
  artifact lineage, real LocalAI, configured-server parity, blind modalities,
  platforms, LA-05, LA-06, or operator approval.

Highest feasible level is contract/documentation review because the source plan
forbids production and executable-test changes in this corrected retry and the
authority permits no model/backend/network calls. Remaining edges are
`GATE-OPERATOR-APPROVAL` (authority), `GATE-OMNI-PRIVATE` (compilation,
registration, atomicity, and release), `GATE-FACTORY-LINEAGE` (Factory Work,
event, artifact, and lineage), `GATE-BOUNDARY-PROJECTION` (transport
observation), `GATE-RELEASE-RACE` (repeat/race cleanup), and later
`GATE-SERVER-PARITY`, `GATE-BLIND`, `GATE-REAL-C1`, and `GATE-LOOPBACK`.

No tests are changed in this story, so no test-layer implementation is
admitted. The later implementation lane must put component rules beside the
LocalAI codec and registrar; use a functional Factory Session through a
reusable `root.BuildProcess` with controlled `edges.Edges`; keep configured
server and real backend proof in integration/release lanes; and put repeat,
timeout, cancellation, lease, host, and race proof in the dedicated Models/
Factory race lane. Those later tests must assert semantic text, opaque
artifact identity, media type, UTF-8 size, Work/Event/replay lineage, typed
failures, redaction, and exactly-once release rather than source shape.

### 5.9 Operational rollout, rollback, and handoff

There is no deployment, migration, cache mutation, backend artifact download,
or generated regeneration in this iteration. The intended future sequence is:

`GATE-OPERATOR-APPROVAL` → private codec/wire implementation → focused
`GATE-OMNI-PRIVATE` → one controlled Factory Session under
`GATE-FACTORY-LINEAGE` → boundary/release-race review → later server, blind,
real-platform, loopback, and review-CI gates.

The implementation owner must stop before code if the existing private result
or registrar cannot carry the descriptor, if a public shape is required, or if
the future test cannot observe atomic failure and release. A revert restores
the current content-only behavior; existing lease/host shutdown remains the
cleanup authority. No recording rewrite, cache deletion, secret/path/address
exception, or generated-file hand edit is allowed.

Handoff artifacts for this story are this proposal's §5 exact shape record,
the unchanged-shape consumer inventory, the PRD story-002 evidence update,
and the progress entry. Story 003 owns the complete semantic matrix; story
004 owns independent clean-room validation. This story's private Proposed
shape is not an implementation authorization and cannot mark LA-05/LA-06 or
any runtime gate passed.

## 6. Story 003 — define the complete semantic Factory acceptance matrix

**Parent behavior:** `BEH-FACTORY` and `BEH-BOUNDARY` — a selected, currently
supportable non-TTS OMNI operation can be followed from Factory Session
admission through semantic result, Work materialization, canonical event
replay, and safe public projection.

**Problem:** Existing characterization proves the component capabilities
individually, but it does not specify one executable, semantic witness that
joins OMNI text, artifact metadata, Work identity, Factory Event ordering and
replay, lineage, failure atomicity, and release behavior.

**Outcome:** The matrix below names every selected happy, unhappy, and public
boundary behavior, its owning test layer and dependency fidelity, its
session/process isolation, an exact future command, and the observer that
decides PASS or FAIL. It is a test-design artifact only: no row is executed or
marked as runtime evidence in this corrected retry.

**Plan reference:** The immutable source-plan snapshot at
`C:\Users\andre\work\portos\infinite-you\docs\temp\projects\localai\source-plan.md`,
“Functional test plan”, “Service interactions”, “Failure modes”, “Bounded
operator amendments — small factory journey beyond TTS”, and “Admission
evidence rule”. The authority tuple remains in §0 and `prd.json`.

**Actor and trigger:** A later Models/Factory validation worker selects and
records one measured, currently supportable non-TTS operation (default `llm`
text; `embed` or another operation only with an environment-based choice),
then invokes it through an admitted explicit Factory Session. This story
defines the proof; it does not select a model, download an asset, start a
backend, or authorize implementation.

**Dependencies:**

- `localai-omni-artifact-contract-delta-authority-retry-002` — completed exact
  private handoff and unchanged public-shape record.
- `GATE-OPERATOR-APPROVAL` — required before any production or executable-test
  change. A missing private-contract disposition stops the implementation
  lane; it is not replaced by this matrix.

**Parallel and shared-surface ownership:** Models and Factory validation own
future test files and fixtures. Work owns Work admission/materialization;
Recordings owns canonical Factory Event history and replay; Models transport
owners own HTTP/CLI projection tests; the integration/release lane owns
configured-server and real-platform tests. Functional scenarios must allocate
their explicit Factory Session identity before runtime construction, use one
reusable `root.BuildProcess` when its injected edges are immutable, and run
independent sessions in parallel with unique IDs, profiles, directories,
routes, streams, and edge state. No task in this story edits a test or shared
production surface.

**Scope:**

- In: semantic OMNI text and descriptor checks; Factory Session admission,
  invocation, selection, Work identity/materialization, canonical event order
  and replay, artifact identity/media/UTF-8 size semantics, lineage,
  malformed/missing output, materialization failure, timeout/cancellation,
  exactly-once lease/host release, redaction, unchanged public projection,
  configured-server parity, and later real-conformance rows.
- Out: executing future tests in this retry; changing production, test,
  generated, OpenAPI, CLI, protobuf, persisted Work/Event, or Factory graph
  files; direct CLI-only or TTS-only acceptance; unmeasured model feasibility;
  and exceptions for required modalities or target platforms.

**Implementation constraints:**

- `FACTORY-01` cannot be admitted until the reconciliation record names the
  selected non-TTS operation and records measured supportability. If the
  existing Factory/Worker path cannot carry it without a public contract,
  stop with a characterized blocker and request the smallest new
  operator-approved Current/Proposed delta.
- Unit rows remain package-isolated. Functional rows must construct through
  `root.BuildProcess`, execute through `Process.Execute`, use Factory Sessions,
  and replace only exact external effects through `edges.Edges`. They must
  observe public output, session/work state, Factory Events, replay, or a
  test-owned external-effect boundary; they must not inspect engine snapshots,
  registries, constructor counts, or package topology.
- The functional suite must reuse a safe immutable process, not a mutable
  customer profile. Every parallel scenario gets a test-owned home/profile,
  temporary config/cache/state, unique session/request/trace IDs, and
  scenario-scoped cleanup. Channels, event subscriptions, or terminal state
  are the readiness/completion signals; fixed sleeps and timeout exhaustion
  are not success evidence.
- The canonical history observer is Recordings replay, not the process-local
  Events stream. The observer compares the selected result and public Work
  projection before and after replay, including artifact metadata and lineage.
- Artifact identity is opaque and assigned only by the Models Inference
  registrar. The semantic row asserts `text/plain` and
  `SizeBytes == int64(len([]byte(text)))`; it asserts that no digest is claimed
  unless the selected public contract actually supplies one. No row may treat
  a path, endpoint, cache handle, signed URL, secret, or raw protocol value as
  an artifact identity.
- Failure rows must prove typed failure, no successful output/Work/event
  publication, and cleanup. A test that only sees a nonzero process exit or a
  field being present is insufficient. Cancellation must also prove that no
  late completed output is published.
- Configured-server and actual LocalAI rows are integration/release work. They
  must consume a prebuilt deliverable supplied by the invoking lane and may
  not be claimed from the controlled protocol fixture or an in-process
  loopback.

**Contract and configuration excerpts:** No contract or configuration is
changed by this story. The exact private Current/Proposed Go shapes and the
unchanged public/OpenAPI/CLI/protobuf shapes remain in §5; no new authored or
generated source is introduced here.

Generated outputs and consumers: none in this story. Future implementation
must continue to regenerate only from authored sources if a separately
approved contract delta ever changes a public shape.

### 6.1 Run-record contract and evidence status

Before executing any row, the owner must attach one run record containing all
fields required by `LA-10`. The record is part of the evidence, not a reason to
convert an unavailable dependency into PASS.

| Required field | Controlled unit/functional declaration | Later parity/real declaration |
| --- | --- | --- |
| Platform | Current `GOOS/GOARCH`; unit rows use no external process; functional rows use the declared test runner platform. | Explicit macOS, Linux, or Windows plus architecture and accelerator/hardware class. |
| Binary/artifact identity | Source commit under test plus controlled fixture identity; no LocalAI/model download and no compiled binary claim. | Prebuilt `you` binary and pinned LocalAI/backend/model manifest identities, with SHA-256 or the source release identity. |
| State, cache, and ports | `t.TempDir`-owned config/cache/work/recording roots; unique Factory Session, request, trace, route, and in-process fixture state; no real profile. | Separate process-owned profile, cache, work, recording, server, and port locations; all paths recorded without exposing them in customer diagnostics. |
| Timeout and process lifetime | Exact `-timeout` in the command; context ceiling only; no OS process for unit/functional rows. | Exact test timeout, startup/invoke/shutdown ceilings, and child/server lifetime; terminal readiness and cleanup signals are recorded separately. |
| Network policy | `none`; protocol and provider effects are controlled at the declared boundary. | Explicit local or remote endpoint policy; server parity and real conformance must record whether network access was expected and bounded. |
| Model/download budget | `0` model calls and `0` download bytes for controlled rows. | Named model/backend asset, maximum bytes, maximum duration, and provisioning result; an unavailable asset is FAIL/INCONCLUSIVE. |
| Retry/call budget | One protocol fixture call per happy/failure case, except `RELEASE-01`'s declared `-count=20` repeat; no hidden retry. | Maximum provider/server calls and retry count declared before the run; no extra retry after the bounded executor retry without escalation. |

All rows are `UNPROVEN` in this proposal. The PASS/FAIL observers below are
the future execution rules. `PARITY-01` and `REAL-01` additionally remain
owned by `LA-07`/`LA-09`; they are intentionally listed so later work cannot
silently omit their real dependency fidelity.

### 6.2 Selected semantic matrix

| ID | Kind and given/when/then behavior | Layer and dependency fidelity | Isolation and parallel strategy | Exact command/procedure | PASS/FAIL observer | Current status / owner |
| --- | --- | --- | --- | --- | --- | --- |
| `OMNI-01` | Happy. Given a controlled OMNI response containing Unicode semantic text and declared usage, when the codec invokes it, then the `text` output has exactly that text, modality `TEXT`, content/media type `text/plain`, and usage remains a separate JSON output when the operation declares `usage`. | Unit; controlled `ProtocolClient` fixture. | Fresh codec and fixture per test; no Factory Session, network, backend, or shared mutable state; `t.Parallel` is safe. | `go test ./pkg/services/models/internal/backends/localai -run '^TestOmniCodecReturnsSemanticTextAndUsage$' -count=1 -timeout=5m` | Assert exact text, exact usage, slot order, modality/media, and no protocol field loss; missing named test, nonzero exit, or mismatch is FAIL. | `UNPROVEN` → `GATE-OMNI-PRIVATE`; Models codec owner. |
| `OMNI-02` | Happy/boundary. Given the same response, when the proposed private result is inspected before registration, then it has exactly one `text` descriptor with name `text`, media type `text/plain`, `SizeBytes == int64(len([]byte(text)))`, zero private identity, and no path or address. | Unit; controlled protocol fixture. | Fresh codec and detached result; no shared process or registry. | `go test ./pkg/services/models/internal/backends/localai -run '^TestOmniCodecBuildsUTF8TextArtifactDescriptor$' -count=1 -timeout=5m` | Assert descriptor count/name/media/byte size, zero `InferenceArtifactRef`, empty source path, and safe detached fields; rune-count, digest, path, or address substitution is FAIL. | `UNPROVEN` → `GATE-OMNI-PRIVATE`; Models codec owner. |
| `FACTORY-01` | Happy. Given a measured supportable non-TTS operation (default `llm` text) and an admitted explicit Factory Session, when the caller invokes through the existing Worker path, then admission succeeds, the selected operation is invoked once, and the returned selected result is the exact semantic text. | Functional; controlled protocol fixture through `root.BuildProcess` and `Process.Execute`, with external effects at `edges.Edges`. | Allocate the explicit Factory Session before runtime construction; reuse one immutable process per compatible edge shape; unique profile/session/request IDs; independent scenarios use `t.Parallel` and scenario-owned cleanup. | `go test ./tests/functional/factory/omni_artifact -run '^TestFactorySessionOmniArtifactJourney$' -count=1 -timeout=10m -v` | Observe public admission/terminal session state, fixture invocation count, selected output/status, and exact text; any missing phase, duplicate invoke, or unmeasured operation choice is FAIL. | `UNPROVEN` → `GATE-FACTORY-LINEAGE` and `LA-05`; Factory/Models validation owner. |
| `FACTORY-02` | Happy. Given `FACTORY-01` completion, when the selected result is materialized, then exactly one canonical Work has a non-empty identity, its artifact-bearing content points to the selected text, and no second Work is created. | Functional; controlled `edges.Edges` and protocol fixture. | Fresh temp config/cache/state and explicit session per scenario; `t.Cleanup` owns session/process lifetime; unique IDs permit parallel execution. | `go test ./tests/functional/factory/omni_artifact -run '^TestFactorySessionOmniArtifactJourney$' -count=1 -timeout=10m -v` | Read public Work state/history and assert one Work ID, one materialization, selected `ArtifactID`, media/content relation, and no duplicate; field presence without identity/materialization semantics is FAIL. | `UNPROVEN` → `GATE-FACTORY-LINEAGE` and `LA-05`; Work/Factory validation owner. |
| `EVENT-01` | Happy/boundary. Given the artifact-bearing Work, when canonical Factory Event history is read and replayed, then the dispatch response precedes completion projection in canonical order and replay reproduces Work ID, artifact link, content, and lineage exactly. | Functional; controlled `root.BuildProcess` and public Recordings read/replay boundary. | Dedicated recording and Factory Session per scenario; no global history or process-local Events substitution; unique factory/session IDs; independent scenarios parallelize. | `go test ./tests/functional/factory/omni_artifact -run '^TestFactorySessionOmniArtifactReplayPreservesOrderAndLineage$' -count=1 -timeout=10m -v` | Compare ordered public event IDs/ordinals and replayed public projection; order, event identity, Work/artifact content, or replay equality mismatch is FAIL. | `UNPROVEN` → `GATE-FACTORY-LINEAGE` and `LA-06`; Recordings/Factory validation owner. |
| `ARTIFACT-01` | Happy/boundary. Given a successful selected text output, when Models result and public Work/Event projections are inspected, then artifact identity is opaque and non-empty after registration, media type is `text/plain`, size equals UTF-8 bytes, and neither a path nor an unprovided digest is claimed. | Functional; controlled protocol fixture, public Models result, Work, and Recordings projections. The package-level descriptor rule is already isolated in `OMNI-02`. | Detached result copies; unique artifact registrar/session state; no shared cache; `t.Cleanup` releases the scenario. | `go test ./tests/functional/factory/omni_artifact -run '^TestFactorySessionOmniArtifactMetadataIsSemantic$' -count=1 -timeout=10m -v` | Assert identity is opaque/non-empty, media and size/content relation are semantic, digest is absent unless contract-provided, and no path/address/secret appears; field-only or leak result is FAIL. | `UNPROVEN` → `GATE-FACTORY-LINEAGE`, `GATE-BOUNDARY-PROJECTION`, and `LA-06`; Models/Factory validation owner. |
| `LINEAGE-01` | Happy. Given source Work and dispatch lineage, when output is materialized and replayed, then request, dispatch, source, parent, current trace, previous chaining trace IDs, and depth remain consistent across Workers, Work, and Recordings. | Functional; controlled Factory Session and `edges.Edges`. | Unique source Work and trace IDs per scenario; dedicated recording/session; parallel-safe temporary state and scenario cleanup. | `go test ./tests/functional/factory/omni_artifact -run '^TestFactorySessionOmniArtifactLineageIsPreserved$' -count=1 -timeout=10m -v` | Compare each lineage value at the public Worker result, Work, canonical event, and replay boundaries; any dropped, reordered, or re-derived value is FAIL. | `UNPROVEN` → `GATE-FACTORY-LINEAGE` and `LA-06`; Work/Recordings owner. |
| `FAIL-01` | Unhappy/boundary. Given blank text, missing descriptor, invalid reference, mismatched name/media, negative or oversized size, or malformed output, when invocation validation completes, then a typed failure is returned atomically with no successful Outputs/Artifacts and no Work completion. | Unit/package; controlled malformed codec/registrar/lifecycle fixtures. Pure package rules remain below the Factory boundary. | Fresh lifecycle and registrar fixture per subtest; `t.Parallel` only with isolated registries; no network or external process. | `go test ./pkg/services/models/internal/services/inference/... -run '^TestInvocationArtifactFailuresAreAtomic$' -count=1 -timeout=5m` | Assert exact typed failure class/slot, zero successful output/artifact, no partial registration/publication, and no completed Work; any partial result or generic-only error is FAIL. | `UNPROVEN` → `GATE-OMNI-PRIVATE`; Models Inference owner. |
| `FAIL-02` | Unhappy. Given a valid selected result followed by Work materialization rejection, when Factory Runtime applies the worker proposal, then the attempt is failed, recorded output Work is absent, and no successful completion projection is emitted. | Functional; controlled Work validation failure through `edges.Edges` and a Factory Session. | Dedicated session/recording and scenario-owned fault injection; no cross-test state; no package-wide quiescence assertion; parallel with independent IDs. | `go test ./tests/functional/factory/omni_artifact -run '^TestFactorySessionOmniArtifactMaterializationFailureIsAtomic$' -count=1 -timeout=10m -v` | Observe failed attempt, terminal public state, empty recorded output Work, and absence of a false success event; any completed projection or late Work is FAIL. | `UNPROVEN` → `GATE-FACTORY-LINEAGE`; Factory Runtime/Work owner. |
| `RELEASE-01` | Unhappy/boundary. Given success, backend error, timeout, and cancellation, when each invocation terminates, then lease and host capacity are released exactly once and cancellation cannot publish a completed output later. | Package plus functional repeat/race; controlled blocking protocol fixture. The repeat/race portion belongs to the dedicated release/race lane, not a load suite. | Unique lease/host/session per case; deterministic fixture channels for start/cancel/terminal signals; `-count=20` and `-race`; serialize only a documented customer-visible capacity invariant. | `go test -race ./pkg/services/models/internal/services/inference/... ./tests/functional/factory/omni_artifact -run '^(TestInvocationLeaseReleaseIsExactlyOnce|TestFactorySessionOmniArtifactReleaseIsExactlyOnce)$' -count=20 -timeout=15m -v` | Assert success/error/timeout/cancel terminal outcomes, one release disposition, one host-capacity release, no leak, and no post-cancel completion; a deadline-only return or duplicate release is FAIL. | `UNPROVEN` → `GATE-RELEASE-RACE` and `GATE-FACTORY-LINEAGE`; Models/Factory validation owner. |
| `REDACT-01` | Boundary/unhappy. Given unsafe path, address, secret, or raw protocol values in backend and artifact failures, when errors, logs, and HTTP/CLI projections are observed, then safe class/message/IDs remain and forbidden values are absent. | Unit/contract; controlled values and detached mapper/schema inputs. No real server or secret is used. | Fresh logger/mapper/input per test; `t.Parallel` safe; forbidden values are synthetic sentinels and never committed as credentials. | `go test ./pkg/services/models/internal/services/inference/... ./pkg/services/models/transports/http ./pkg/services/models/transports/cli -run '^Test.*Redact' -count=1 -timeout=5m -v` | Inspect observed error/log/transport values for forbidden sentinels and assert typed safe fields, stable opaque IDs, and actionable class/message; any leak or unsafe raw protocol propagation is FAIL. | `UNPROVEN` → `GATE-REDACTION`; Models/transport owner. |
| `BOUNDARY-01` | Happy/unhappy. Given a detached `InferenceOutput.Artifact` and an existing typed failure, when HTTP and CLI mappers project them, then current output metadata/failure categories remain unchanged, empty refs are omitted, and no new public field is required. | Contract/package; detached result and schema-mock inputs at the existing HTTP/CLI mapper boundary. | Fresh mapper input per test; no server; `t.Parallel`; no source-inventory or generated-file scan is used as behavior evidence. | `go test ./pkg/services/models/transports/http ./pkg/services/models/transports/cli -run '^Test(HTTP|CLI)ProjectionPreservesArtifactAndFailureContract$' -count=1 -timeout=5m -v` | Compare normalized public output, artifact metadata, omitted empty refs, and typed failure class/message with the existing contract; shape or redaction mismatch is FAIL. | `UNPROVEN` → `GATE-BOUNDARY-PROJECTION` and `GATE-SHAPES`; transport owner. |
| `PARITY-01` | Happy/unhappy. Given equivalent direct CLI and configured HTTP/server requests, including explicit `--server`, when both invoke the selected operation, then normalized success values, output metadata, error category, and lifecycle/release state match. | Integration later; `local_real` or `remote_real` configured server. The suite consumes an already compiled deliverable supplied by the integration lane. | Isolated server/config/cache/profile/ports and separate process lifetime; no in-process substitution; the invoking lane owns one prebuilt artifact. | `go test ./tests/integration/models/model_invoke -run '^TestCLIAndConfiguredServerInvocationParity$' -count=1 -timeout=15m -v` | Compare request/response evidence for direct and explicit `--server` paths, normalized values/metadata/errors, and release status; unavailable or `INCONCLUSIVE` is blocking for `LA-07`, never PASS. | `UNPROVEN` → `GATE-SERVER-PARITY` and `LA-07`; integration owner. |
| `REAL-01` | Happy/unhappy/boundary. Given each required supported platform and documented prebuilt/small asset, when actual LocalAI/backend conformance runs, then provisioning, load, invoke, semantic output, and release are recorded, or that platform is explicitly FAIL/INCONCLUSIVE. | Integration/release later; `local_real` actual LocalAI/backend and declared artifact. No hermetic fixture substitution. | One declared platform/artifact budget per run; isolated profile/cache/ports/process; no parallel backend startup unless the conformance lane explicitly owns it; cleanup is verified. | `make test-localai-conformance` | The structured platform result must include artifact/model identity, semantic output, and release/cleanup status; an absent asset, unavailable platform, failure, or `INCONCLUSIVE` blocks `LA-09` and is never converted to PASS. | `UNPROVEN` → `GATE-REAL-C1` and `LA-09`; real-conformance owner. |

The selected matrix deliberately does not duplicate the full ASR, TTS,
embedding, LLM-image, or LLM-video blind/real corpus. Those are distinct
project behaviors owned by `LA-02`, `LA-04`, `GATE-BLIND`, and `GATE-REAL-C1`.
Direct CLI-only or TTS-only evidence remains useful characterization but cannot
substitute for `FACTORY-01`–`EVENT-01` or pass `LA-05`/`LA-06`. Cache/backend
resolution and configured-server lifecycle remain later fidelity gates; the
matrix records their ownership rather than implying that a controlled fixture
proved them.

### 6.3 Acceptance criteria for story 003

- [x] Given each selected matrix precondition, when its future exact command
  runs, then the row has a named semantic observer that reports PASS/FAIL from
  meaning, not field presence; zero matched tests is not a pass.
- [x] Given malformed or missing output, timeout/cancellation, materialization
  failure, and release-race cases, then the matrix names atomic failure,
  no-late-success, and cleanup observers with controlled signals and the
  correct package/functional/race layer.
- [x] Given Factory Session success, then the matrix compares selected result,
  one Work identity/materialization, artifact identity/media/UTF-8 size,
  canonical event order, replay equality, and lineage across public boundaries.
- [x] Given the existing public boundary, then `BOUNDARY-01` observes unchanged
  HTTP/CLI/OpenAPI/generated behavior and safe redaction without adding a
  contract or using a source-inventory test.
- [x] Later configured-server and real-platform rows are explicitly
  `UNPROVEN` and owned by `LA-07`/`LA-09`; no row marks `LA-05` or `LA-06`
  passed, and no platform or modality exception is introduced.

These checkmarks mean that the story's proposal/design outcome is complete.
They do not report execution evidence for any row or Project acceptance.

### 6.4 Verification and evidence boundary

**Behavioral witness:** The matrix in §6.2 has one row for every selected
semantic behavior and every row contains given/when/then behavior, layer,
dependency fidelity, isolation, exact command/procedure, PASS/FAIL observer,
and an owning later gate. The run-record contract in §6.1 prevents later
evidence from omitting platform, artifact, state, timeout, network, budget, or
retry identity.

**Executable-spine effect:** `increase_fidelity` — the plan now defines the
semantic Factory witness needed to extend the retained OMNI-to-Workers-to-Work-
to-Recordings path, without changing the executable spine or claiming runtime
success.

**Required evidence:**

- Scope: test design and proposal/documentation review.
- Dependency fidelity: controlled for `OMNI-01` through `REDACT-01` and
  `BOUNDARY-01`; `local_real`/`remote_real` only for the later `PARITY-01` and
  `REAL-01` rows.
- Exact procedure: parse `prd.json`; inspect §6.1–§6.2 and confirm all matrix
  IDs have all six required columns; compare the commands and observers with
  the story-003 matrix in `prd.json`; re-run the three immutable authority
  SHA-256 checks; run `git diff --check`; inspect the final diff against the
  proposal-only allowlist. Do not run the future row commands in this retry.
- Proves: complete selected-case design, correct test-layer classification,
  Factory Session/process isolation, semantic observers, explicit budgets,
  later fidelity ownership, and no silent modality/platform exception.
- Does not prove: any codec/wire compilation, registrar execution, model
  feasibility, Factory Session behavior, Work/Event/artifact lineage,
  configured-server parity, real LocalAI/platform support, `LA-05`, `LA-06`,
  terminal CI, merge, or operator approval.

Highest feasible level is complete test design with a future functional level:
the corrected retry is prohibited from production and executable-test changes,
and the authority permits no model/backend/network calls. Remaining edges are
`GATE-OPERATOR-APPROVAL`, `GATE-OMNI-PRIVATE`, `GATE-FACTORY-LINEAGE`,
`GATE-BOUNDARY-PROJECTION`, `GATE-RELEASE-RACE`, `GATE-SERVER-PARITY`,
`GATE-BLIND`, `GATE-REAL-C1`, and `GATE-LOOPBACK`.

**Test-layer design:** No test source changes in this story. The future unit
rows remain beside the owning Models codec/Inference/transport packages; the
Factory rows belong under `tests/functional/factory/omni_artifact` and use the
public `root.BuildProcess`/`Process.Execute` path with controlled `edges.Edges`;
the configured-server row belongs under `tests/integration/models/model_invoke`
and consumes an artifact built once by the integration lane; the real row
belongs to the dedicated conformance/release lane. `RELEASE-01` uses focused
normal/repeat/race evidence with deterministic blocking channels rather than
sleep-based synchronization. No broad inventory, topology, source scan, or
load test is introduced.

### 6.5 Operational rollout, rollback, and handoff

There is no deployment, migration, cache mutation, backend startup, generated
regeneration, or test execution in this iteration. The future sequence remains
`GATE-OPERATOR-APPROVAL` → private codec/wire implementation →
`GATE-OMNI-PRIVATE` → `FACTORY-01`–`EVENT-01` under
`GATE-FACTORY-LINEAGE` → boundary/release-race review → configured-server,
blind, real-platform, loopback, and review-CI gates.

Stop before implementation if the selected operation is not measured as
supportable, if a public/persisted/CLI/OpenAPI/protobuf/Factory graph shape is
needed, if a row cannot observe semantic output or atomic failure, if release
is not exactly once, or if the authority hashes change. Preserve the blocker
and request the smallest operator-approved plan delta. Rollback of the later
private implementation is a revert/disable followed by existing host/lease
shutdown; it must not rewrite recordings, delete caches to conceal failure, or
leak paths, addresses, protocol payloads, or secrets.

Handoff artifacts for this story are this §6 matrix and run-record contract,
the story-003 `prd.json` evidence update, and the progress entry. Story 004
owns independent clean-room validation. This matrix is not an implementation
authorization and cannot mark `LA-05`, `LA-06`, server parity, real platforms,
or any runtime gate passed.
