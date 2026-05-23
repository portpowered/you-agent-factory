# PRD: Transformer Workstation

## Introduction

Add a new `TRANSFORMER` workstation type plus file-backed content parts so the agent factory can ingest authored documents such as `.epub`, `.pdf`, `text.md`, plain text files, and HTTP or HTTPS URIs, extract readable text from them, and pass that normalized text into downstream workstations such as text-to-speech. This solves the current gap where submitted work content can carry text or image references, but cannot express document-like source material that should be transformed into text before model or TTS execution.

In version 1, the transformer focuses on text extraction only. It accepts document-like content parts, reads local files or fetches public HTTP or HTTPS URIs, extracts plain text, and emits normalized text content for the next workstation. OCR, scanned-document handling, DRM, authenticated URIs, and advanced document understanding remain out of scope.

## Goals

- Support document-like work content parts for submitted source material.
- Allow factory authors to declare a `TRANSFORMER` workstation that converts supported file or URI inputs into normalized text.
- Make transformed output easy to pass into a downstream TTS workstation without custom glue code.
- Preserve clear diagnostics and replay evidence showing which file or URI was transformed and whether extraction succeeded.
- Keep version 1 narrow by supporting text extraction only, not summarization, chunking, OCR, or narration-specific rewriting.

## User Stories

### US-001: Submit file-backed content parts
**Description:** As a factory author or API client, I want submitted work to carry file-backed source material so that the factory can ingest documents instead of requiring all source text inline up front.

**Acceptance Criteria:**
- [ ] The public work content contract supports a new file-backed content part type for document-like inputs.
- [ ] A file-backed content part can reference a local file path.
- [ ] A file-backed content part can reference a public HTTP or HTTPS URI.
- [ ] Version 1 documentation defines supported source formats at minimum as `.epub`, `.pdf`, `.md`, and `.txt`.
- [ ] Validation rejects unsupported URI schemes in v1.
- [ ] Validation rejects empty file references.
- [ ] Typecheck, lint, generated-artifact checks, and relevant backend contract tests pass.

### US-002: Author a transformer workstation in factory config
**Description:** As a factory author, I want to declare a transformer workstation so that document extraction is an explicit workflow step instead of an implicit behavior hidden inside TTS or a model worker.

**Acceptance Criteria:**
- [ ] A workstation can declare `type: "TRANSFORMER"` in the public config contract.
- [ ] A transformer workstation can bind to a compatible worker.
- [ ] Transformer workstation validation documents which workstation fields remain valid and which are transformer-specific.
- [ ] Existing `MODEL_WORKSTATION` and `LOGICAL_MOVE` behavior remains unchanged.
- [ ] Typecheck, lint, generated-artifact checks, and relevant backend contract tests pass.

### US-003: Extract plain text from supported local files
**Description:** As a factory author, I want a transformer workstation to read supported local files and extract plain text so that downstream narration flows do not need to understand source document formats.

**Acceptance Criteria:**
- [ ] A transformer workstation can read local `.epub` files and extract text.
- [ ] A transformer workstation can read local `.pdf` files and extract text when the PDF already contains extractable text.
- [ ] A transformer workstation can read local `.md` and `.txt` files and extract text.
- [ ] Extracted text preserves readable ordering closely enough for narration-oriented use.
- [ ] Extraction failures include enough diagnostics to identify the failing file and failure reason.
- [ ] Typecheck, lint, and relevant backend integration tests pass.

### US-004: Extract plain text from public URIs
**Description:** As a factory author, I want a transformer workstation to fetch supported source material from a public URI so that I can start a narration flow from a remote document without downloading it manually first.

**Acceptance Criteria:**
- [ ] A transformer workstation can fetch source material from public `http` and `https` URIs.
- [ ] Version 1 rejects authenticated, signed, or otherwise runtime-secret-backed URI fetch flows.
- [ ] URI fetches use explicit timeout and failure handling.
- [ ] Unsupported content types or unreadable responses fail with actionable diagnostics.
- [ ] Typecheck, lint, and relevant backend integration tests pass.

### US-005: Emit normalized text content for downstream TTS
**Description:** As a factory author, I want transformer output to become canonical text content so that downstream TTS workstations can consume it without source-format-specific logic.

**Acceptance Criteria:**
- [ ] Successful transformation emits canonical text content parts in the ordinary work-submission or dispatch-output path.
- [ ] Transformer success does not require downstream TTS nodes to inspect raw file references directly.
- [ ] Transformer output can preserve basic source metadata such as original filename or URI in diagnostics or metadata without changing the primary output from extracted text.
- [ ] The transformed work remains compatible with existing text-oriented downstream workstations.
- [ ] Typecheck, lint, and relevant backend integration tests pass.

### US-006: Fail cleanly on unsupported or unreadable source material
**Description:** As a factory author, I want unsupported file types and unreadable documents to fail predictably so that bad source material does not silently produce broken TTS output.

**Acceptance Criteria:**
- [ ] Unsupported file extensions fail through the ordinary failure path.
- [ ] Missing local files fail through the ordinary failure path.
- [ ] Network fetch failures fail through the ordinary failure path.
- [ ] Image-only or scanned PDFs that cannot yield text without OCR fail with clear diagnostics in v1.
- [ ] If `onFailure` is configured, transformer failures use that route exactly as other workstation failures do.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-007: Expose transformer behavior in events and projections
**Description:** As a user inspecting runtime history, I want to see that a document was transformed and whether extraction succeeded so that I can debug content-ingestion flows.

**Acceptance Criteria:**
- [ ] Dispatch completion evidence identifies transformer execution distinctly from ordinary model or logical execution.
- [ ] Replay preserves enough transformation evidence to show source reference and success or failure outcome.
- [ ] Runtime or dashboard-facing projections can expose transformer workstation kind distinctly from existing workstation types.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-008: Document transformer authoring and content parts
**Description:** As a factory author, I want clear documentation for file-backed content parts and transformer workstations so that I can build TTS ingestion flows without reverse-engineering the runtime.

**Acceptance Criteria:**
- [ ] Documentation explains when to use a transformer workstation instead of putting file parsing inside TTS or a custom model prompt.
- [ ] Documentation defines the new file-backed content part with local-file and URI examples.
- [ ] Documentation explains that version 1 extracts plain text only.
- [ ] Documentation calls out version 1 exclusions such as OCR, DRM, scanned PDFs, and authenticated URIs.
- [ ] Documentation includes at least one example factory snippet showing source submission, transformer workstation, and downstream TTS.

## Functional Requirements

1. FR-1: The system must support a new canonical work content part type for file-backed source material.
2. FR-2: The file-backed content part must support local file references and public HTTP or HTTPS URI references.
3. FR-3: The system must support a new workstation type value named `TRANSFORMER`.
4. FR-4: A transformer workstation must accept file-backed source content as input and produce canonical text content as output.
5. FR-5: Version 1 transformer output must focus on plain extracted text rather than summarization, chunking, or structured narration planning.
6. FR-6: The system must support text extraction from `.epub`, `.pdf`, `.md`, and `.txt` sources in version 1.
7. FR-7: PDF support in version 1 must be limited to PDFs that already contain extractable text.
8. FR-8: The system must not require downstream TTS workstations to parse source files or URIs directly when transformer output is available.
9. FR-9: Transformer execution must use explicit timeout, cancellation, and failure handling for local reads and URI fetches.
10. FR-10: The system must reject unsupported URI schemes in version 1.
11. FR-11: The system must reject authenticated or secret-dependent URI fetch flows in version 1.
12. FR-12: Transformer failures must route through the ordinary workstation failure path.
13. FR-13: Validation must reject malformed file-backed content parts such as empty references or unsupported source declarations.
14. FR-14: Successful transformer output must be represented as canonical text content parts compatible with existing downstream text consumers.
15. FR-15: Event and replay surfaces must preserve enough transformation evidence to identify the source reference and extraction outcome.
16. FR-16: Documentation must define the new content part contract, transformer workstation authoring, supported formats, and version 1 exclusions.

## Non-Goals

- OCR for scanned or image-only PDFs.
- DRM-protected EPUB or PDF support.
- Authenticated, signed, or otherwise secret-backed URI fetches.
- Chunking long text into TTS-sized segments automatically in version 1.
- Summarizing, cleaning, paraphrasing, or rewriting extracted text.
- Replacing the generic work payload with a new document-only submission envelope.
- Support for every possible document format beyond `.epub`, `.pdf`, `.md`, and `.txt` in version 1.

## Design Considerations

- The content-part shape should make it obvious when submitted work carries source files versus already-extracted text.
- Transformer workstations should feel like an explicit ingestion or normalization step in the graph, not like hidden preprocessing.
- The docs should show a straightforward “source file -> transformer -> TTS” example because that is the motivating flow.
- Any UI treatment for transformer nodes should make it clear that they change content format rather than generate original text.

## Technical Considerations

- The existing `WorkContentPart` contract currently supports `text` and `image`, so this feature requires public contract and generated-surface changes for a new file-backed part.
- Runtime config and validation will need a new workstation type for `TRANSFORMER`.
- Transformer execution should remain separate from TTS execution so model-operation workers can continue to assume text-like input.
- URI fetching must define timeout, size, and content-type boundaries explicitly so document ingestion does not become an unbounded network path.
- Document extraction libraries or adapters will need to preserve a narrow, testable contract that returns plain text and failure diagnostics.
- Tests should cover content-part validation, transformer config validation, local-file extraction, URI fetch failure handling, unsupported formats, replay evidence, and integration with downstream text-oriented workstations.

## Success Metrics

- A factory author can submit an `.epub`, `.pdf`, `.md`, `.txt`, or public document URI and route it through one explicit transformer step before TTS.
- Downstream TTS flows can consume transformer output as ordinary text content without custom source-format parsing.
- Unsupported or unreadable documents fail predictably instead of producing silent empty narration.
- The feature can be documented with a small, understandable example that does not require custom scripting for the common ingestion path.

## Open Questions

- Should the new file-backed content part use a single `file` field for both local paths and URIs, or separate fields such as `file` and `uri`?
- Should transformer workstations support both `SCRIPT_WORKER` and `HOSTED_WORKER` from day one, or should version 1 start with one execution backend?
- Should extracted text preserve Markdown formatting where possible for `.md` inputs, or should all outputs normalize to plain prose text?
- Where should basic source metadata such as filename, URI, title, or author live if it should survive transformation without changing the main text output contract?
- What explicit size limits should version 1 impose on fetched remote documents and extracted text output?
