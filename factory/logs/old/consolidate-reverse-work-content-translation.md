# Cleanup Idea: Consolidate Domain-to-Generated Work-Content Encoding

## Why this cleanup exists

PR `#260` already centralized generated-to-domain work-content decoding in
`pkg/workcontent/generated.go`, and the current repository baseline at commit
`ae93b68e` still reuses that decoder from API submit or upsert handling,
projection reconstruction, and replay hydration.

The opposite direction is still duplicated in production code:

- `pkg/api/handlers.go` owns `domainWorkContentToGeneratedPtr(...)`
- `pkg/factory/event_history.go` owns `generatedWorkContentPtr(...)`

Both helpers convert `[]interfaces.WorkContentPart` into
`*factoryapi.WorkContent` with the same behavior:

- the same `text` and `image` switch
- the same generated union constructors
- the same nil or empty input behavior
- the same skip-invalid or unsupported-part behavior

This is still live production duplication on the current baseline:

- `pkg/api/handlers.go` uses the helper on public `/work` readback shaping
- `pkg/factory/event_history.go` uses the helper on emitted work-request and
  output-work event payload shaping

That leaves one public/generated contract rule with two active owners even
though the decode direction already has a canonical home.

This remains the best current cleanup target after re-scanning the repo:

- the strongest frontend overlap is thinner and more semantically coupled
- the maintainer-surface locality contradiction is still a meta observation,
  not the next repo-safe cleanup request
- the work-content encoder seam is still one narrow production rule with two
  active implementations

## Requested change

Move domain-to-generated work-content encoding onto one canonical helper in
`pkg/workcontent` and delete the duplicate boundary-local implementations.

Keep this cleanup narrow:

- preserve current API readback behavior
- preserve current emitted event-history payload behavior
- preserve nil or empty input behavior
- preserve current skip-invalid-part behavior
- keep API-only request validation and error wording local to handlers
- do not redesign the work-content model
- do not broaden this into `/work` request validation or generated `Work`
  hydration cleanup
- prefer deleting duplicate helpers over adding forwarding wrappers

Suggested shape:

- add one backend-owned domain-to-generated encoder in `pkg/workcontent`
- update API response helpers and factory event-history helpers to reuse that
  encoder
- delete `domainWorkContentToGeneratedPtr(...)` and
  `generatedWorkContentPtr(...)` once the shared helper owns the conversion
- if tests currently rebuild the same generated union inline only to express
  this translation rule, prefer tightening them around observable API or event
  outcomes instead of adding new topology assertions

## Relevant files

- `pkg/workcontent/generated.go`
- `pkg/workcontent/generated_test.go`
- `pkg/api/handlers.go`
- `pkg/api/server_test.go`
- `pkg/factory/event_history.go`
- `pkg/factory/event_history_test.go`

## Acceptance criteria

- Domain-to-generated work-content encoding has one canonical implementation in
  `pkg/workcontent`.
- `pkg/api/handlers.go` no longer owns
  `domainWorkContentToGeneratedPtr(...)`.
- `pkg/factory/event_history.go` no longer owns
  `generatedWorkContentPtr(...)`.
- API responses that expose work content still preserve part order and values
  for supported `text` and `image` parts.
- Factory event-history payloads that expose work content still preserve part
  order and values for supported `text` and `image` parts.
- Verification remains behavioral around API and emitted-event outcomes rather
  than helper-topology assertions.

## Review guidance

Review this as a duplication-removal cleanup that should preserve observable
behavior. The highest-signal evidence is:

- API behavior in `pkg/api/server_test.go`, especially `GET /work` and
  `GET /work/{id}` content assertions
- emitted work-request event payload behavior in
  `pkg/factory/event_history_test.go`
- focused shared-helper coverage in `pkg/workcontent/generated_test.go` only if
  it directly locks the translation behavior
