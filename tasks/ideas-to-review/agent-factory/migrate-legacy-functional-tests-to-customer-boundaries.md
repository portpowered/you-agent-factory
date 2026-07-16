# Migrate legacy functional tests to customer boundaries

The repository-wide functional boundary guard found a large, repeated architecture deficiency: legacy scenarios under `tests/functional` directly invoke service, handler, projection, replay, and runtime implementation APIs. Rewriting all of them as part of the guard change would combine unrelated behavioral migrations into one review.

Migrate the content-hash-quarantined files in `contracts/functional-boundary-baseline.json` to invoke and observe the product through REST, CLI, MCP, or SSE. Move implementation-level tests to their owning package when their actual contract is not customer-visible. Remove each baseline entry as its file becomes compliant, and delete the baseline and migration task reference when the list reaches zero.

Acceptance should include:

- Each migrated functional scenario uses a customer interface and preserves its observable assertions.
- Implementation-level projection, replay, lifecycle, or construction checks live at the appropriate package/integration layer.
- The required functional command reports fewer quarantined files after every migration and rejects stale baseline entries.
- The final migration removes the baseline with the full-tree boundary check still green.
