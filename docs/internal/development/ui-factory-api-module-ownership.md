# UI factory API module ownership

This guide records which dashboard API modules own session factory HTTP,
canonical factory normalization, and editor adapters after the U7 cleanup that
removed `ui/src/api/named-factory/`. Use it when adding import, export, editor,
or prompt-template behavior so ownership stays explicit and product code does
not reintroduce a parallel API folder.

## Module map

| Module | Owns | Does not own |
| --- | --- | --- |
| `ui/src/api/factory-definition/` | `normalizeFactoryDefinition`, `isCanonicalFactoryDefinition`, `FactoryDefinitionAPIError`; pure shaping of generated `Factory` payloads | `fetch`, `transport.ts`, session routing, save modes, or operator dialog copy |
| `ui/src/api/session-factory/` | Session factory `GET`/`PUT` (`getSessionFactory`, `saveSessionFactory`), `SessionFactoryAPIError`, PNG import activation (`activateImportedFactoryForSession`, `discoverSessionNamedFactoryNames`, `getCurrentFactory`), import save-mode helpers, workstation prompt-template contract/validate under `prompt-template/` | Editor-specific error vocabulary, React Query keys, or feature UI state |
| `ui/src/api/current-factory-definition/` | Thin editor adapter: `getCurrentFactoryDocument`, `saveCurrentFactoryDocument`, `saveFactoryForSessionDocument`; maps `SessionFactoryAPIError` → `CurrentFactoryDefinitionError` (for example `INVALID_FACTORY` → `INVALID_FACTORY_DEFINITION`) | Duplicate HTTP, normalization, or import activation logic |

Dependency direction is always **features → adapter (optional) → session-factory → factory-definition**. Normalization runs before HTTP in `session-factory/api.ts`; features should not call `fetch` on `/factory-sessions/.../factory` except through these modules.

## `session-factory/` responsibilities

- **HTTP:** `api.ts` implements `getSessionFactory` and `saveSessionFactory` with `{ mode?, factory }` bodies on `/factory-sessions/{session_id}/factory`, using `factoryAPIURL`, `currentFactorySessionPath`, and `transport.ts`.
- **Errors:** `errors.ts` and `operator-errors.ts` expose machine-readable codes used by import preview and activation flows: `STALE_FACTORY_VERSION`, `FACTORY_NOT_IDLE`, `INVALID_FACTORY`, `INVALID_FACTORY_NAME`, and `NOT_FOUND`.
- **Import activation:** `import-activation.ts` and `import-save-mode.ts` own PNG confirm paths. Replace-current uses session PUT with `REPLACE_CURRENT` (or default mode). Create-new-named uses `UPSERT_NAMED_AND_ACTIVATE` with correct version inclusion for new vs existing names. Activation never issues `POST /factories`.
- **Prompt template:** `prompt-template/api.ts` owns session-scoped workstation prompt-template GET/POST on the current-factory workstation path segment. Feature code may import via `session-factory` or the `current-factory-prompt-template/` re-export barrel when a stable path is required.
- **Public surface:** `index.ts` re-exports the family; contract tests live beside the module (`openapi.session-factory.test.ts`, `import-activation*.test.ts`, `get-current-factory*.test.ts`).

## `factory-definition/` responsibilities

- **Normalization only:** `api.ts` validates and normalizes customer `Factory` JSON against the generated OpenAPI contract. No network I/O.
- **Callers:** `session-factory` normalizes before PUT; import activation and export type helpers consume `CanonicalFactoryDefinition` / version-stripped shapes after normalization or readback.

## `current-factory-definition/` responsibilities

- **Editor adapter only:** Delegates load/save to `session-factory` and translates errors for dashboard hooks (`useCurrentFactoryDefinition`, `useFactoryDocumentSave` in `ui/src/features/current-factory-definition/`).
- **Save mode:** Editor and replace-current import paths use `REPLACE_CURRENT` via `CURRENT_FACTORY_EDITOR_SAVE_MODE`. Named create during import uses `UPSERT_NAMED_AND_ACTIVATE` through `saveFactoryForSessionDocument` in session-factory, not through a separate named-factory module.

## Feature import guidance

| Concern | Import from |
| --- | --- |
| Session factory GET/PUT, import activation, `ImportFactoryValue`, `SessionFactoryAPIError` | `ui/src/api/session-factory` |
| Normalize a raw factory payload before save | `ui/src/api/factory-definition` (usually via session-factory) |
| Editor document load/save and `CurrentFactoryDefinitionError` | `ui/src/api/current-factory-definition` or `ui/src/features/current-factory-definition/public` hooks |
| App-shell and API unit tests mocking session factory traffic | `ui/src/testing/session-factory-mocks.ts` |

Production code under `ui/src/features/` and app entry tests must **not** import `ui/src/api/named-factory` — that folder was deleted. If a change needs named-factory semantics, extend `session-factory` or the editor adapter instead of adding a new legacy barrel.

## Related references

- Process map rows: `docs/internal/processes/development-guide-relevant-files.md` (search for `ui/src/api/session-factory`).
- Sharing contract and PNG metadata: `docs/internal/development/development.md` (**Factory Sharing Contract**) and `docs/internal/development/named-factory-api-contract-data-model.md` (backend route vocabulary; UI HTTP is session-scoped only).
- OpenAPI save modes and error codes: `ui/src/api/generated/openapi.session-factory.test.ts`.

## Verification

After touching these modules:

```bash
make typecheck
cd ui && bun run test:unit -- --run src/api/session-factory src/api/factory-definition src/api/current-factory-definition
```

For import/export feature seams, also run targeted app-shell tests (`App.import.test.tsx`, `App.export-dialog.test.tsx`, `App.export-submit.test.tsx`) when behavior crosses module boundaries.
