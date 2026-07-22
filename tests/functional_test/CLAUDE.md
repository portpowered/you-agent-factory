The functional tests exercise the same application process and public
interfaces as customers.

Construct the system only through:

```go
process, err := root.BuildProcess(t.Context(), edges.Edges{
    ProviderOverride: provider,
})
if err != nil {
    t.Fatalf("BuildProcess() error = %v", err)
}
err = process.Execute(root.Input{
    Args:             []string{"you", "run", "--factory", "./factory"},
    Env:              testEnvironment,
    WorkingDirectory: projectDir,
    Context:          t.Context(),
    Stdout:           &stdout,
    Stderr:           &stderr,
})
```

Put replacements at exact external effects: provider or provider-command
execution, subprocess launch, HTTP, filesystem, clock, listener startup, and
external observability sinks. Do not replace Factory Runtime, Factory Session,
Models, Workers, Work, Automation, projections, stores, schedulers, or complete
service bundles.

Pass configuration through `root.Input.Args`, `Env`, and `WorkingDirectory`.
Functional code must not import or mutate `runtimeinput.Config`, and shared
server helpers must not expose configuration callbacks that bypass CLI parsing.

Assertions use customer-visible CLI output, REST/MCP responses, Factory Events,
Factory Session projections, public token history, and public resource usage.
Do not read internal markings, topology, dispatch-history slices, resource
places, or runtime service locators.

Worker-created Work is emitted as a canonical `FACTORY_REQUEST_BATCH` Work
Request. The retired internal worker-result generated-work field must not be
recreated, including for decode compatibility.

If a malformed internal `WorkResult` or other unreachable invariant needs
coverage, place a focused test in the owning package. If a supported behavior
lacks a public observation, add that observation before removing the old
assertion.
