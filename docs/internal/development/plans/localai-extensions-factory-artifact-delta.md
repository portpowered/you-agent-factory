# LocalAI OMNI-to-Factory artifact contract delta

Status: proposal-only private contract delta, story
`localai-omni-artifact-contract-delta-authority-retry-002`. This document
retains the verified current path and now records a reviewable Proposed private
handoff. It does not authorize production change, executable test change,
generated-artifact change, or Project acceptance.

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
public/OpenAPI shapes in §5. The next bounded story is story 003: define the
semantic Factory acceptance matrix against this preserved private-only
boundary. No implementation, public field, generated artifact, protobuf
field, CLI grammar, Factory graph, artifact store, or runtime test is admitted
without the required operator/private-contract gate.

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
