# Invocation Relevant Files

Use this map when changing factory invocation input, return-policy, or
primary-result behavior.

- `pkg/invocations/` contains shared pure invocation contract logic used by CLI
  and API adapters.
- `pkg/interfaces/factory_runtime.go` owns the backend canonical
  `WorkContentPart` shape returned by invocation resolvers.
- `pkg/workcontent/` translates between generated OpenAPI `WorkContent` and the
  backend-owned `interfaces.WorkContentPart` shape.
- `pkg/api/handlers_work_write.go` includes the session invocation HTTP
  boundary alongside other session work-write handlers.
- `pkg/cli/run/` is the `you run --factory` CLI boundary.
- `docs/architecture/invocation-contract.md` documents CLI/API equivalence and
  invocation-return policy ownership.
