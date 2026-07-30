`test-functional` still takes about 100 seconds because it is now dominated by real runtime/process integration work, not `go list` or CLI compilation.

The important detail is that `-jobs 8` maps directly to `go test -p=8` in [cmd/functionallane/main.go](C:/Users/andre/work/portos/infinite-you/cmd/functionallane/main.go:55). That parallelizes packages, but tests inside each package remain serial. The largest functional packages contain many tests and almost no `t.Parallel()` usage.

From the last passing run:

| Package | Duration | Tests |
|---|---:|---:|
| `providers/acp` | 32.7s | 35 |
| `runtime_api/factory_transformation` | 29.5s | 39 |
| `work/relationships` | 22.9s | 20 |
| `factory/packaged/catalog` | 19.6s | 12 |
| `work/routing` | 18.3s | 10 |
| `providers` | 18.0s | 90 |
| `providers/cursor` | 16.2s | 8 |
| `workers/inference` | 15.8s | 35 |
| `work/submission` | 15.4s | 22 |
| `factory/definitions` | 13.3s | 17 |

These overlap, but there are enough large packages that they execute in several scheduling waves. Windows subprocess, filesystem, Git, HTTP server, and antivirus contention also means eight concurrent packages do not achieve an eightfold speedup.

The two largest packages explain the pattern:

- ACP starts a complete process/runtime harness in 28 different places. Its failure-classification table alone runs five separate factory processes and took roughly 9 seconds. ACP shutdown also includes a 500ms graceful wait and a 2-second termination fallback in [service.go](C:/Users/andre/work/portos/infinite-you/pkg/services/providers/internal/services/acp/internal/service/service.go:404).
- `factory_transformation` repeatedly starts a complete API server. `TestCurrentFactoryPUT_RequiresAdvancedSaveVersion` runs eight serial subtests, each constructing a new server in [api_current_factory_put_test.go](C:/Users/andre/work/portos/infinite-you/tests/functional/runtime_api/factory_transformation/api_current_factory_put_test.go:489).

There is also a fixed tail: after the functional packages finish, the Make target runs three CLI baseline packages as a separate command. That added about 4.5 seconds in the passing run; see [Makefile](C:/Users/andre/work/portos/infinite-you/Makefile:389). `-count=1` also intentionally disables Go test-result caching.

The best next steps, in order, are:

1. Split the largest serial packages into independent test packages. ACP could be divided into daemon lifecycle, protocol failures, wire goldens, and packaged-factory scenarios. Factory transformation could separate current-save, named-factory, import/export, and document tests. This lets the existing `-p=8` scheduling actually parallelize them safely.

2. Stop rebuilding a complete server for table rows. The eight advanced-version cases can use one server with isolated factory names or controlled state reset. ACP’s five failure modes can share one process/server where protocol semantics allow it.

3. Move classification matrices down to service-level tests. Keep one or two process-boundary tests proving JSON-RPC and lifecycle wiring, then test every failure-code mapping against the ACP service directly.

4. Profile ACP shutdown paths. Several 1.5–2.5 second tests appear to approach termination fallback timing. Make helper peers exit promptly on stdin closure before considering shorter production grace periods.

5. Avoid simply increasing `-jobs`. Running ACP and factory transformation together during profiling inflated them to roughly 63 and 72 seconds because they competed for Windows process and filesystem resources. Eight jobs is already near or beyond the useful concurrency level on this machine.

6. Consider moving the CLI baseline command out of `test-functional` if it belongs in the contract/unit tier. That is an immediate ~4–5 second saving.

So the next highest-value change is package decomposition plus server reuse. The earlier changes removed accidental latency; what remains is mostly intentional end-to-end coverage being serialized inside oversized Go test packages.