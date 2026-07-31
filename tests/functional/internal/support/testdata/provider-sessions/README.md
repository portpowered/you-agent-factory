# Provider Session Golden Fixtures

Tracked fixture root for provider-session golden cases. This path is narrowly
excepted from the general `docs/temp/**` gitignore rule so sanitized fixtures
survive clones and CI.

## Layout

```text
tests/functional/internal/support/testdata/provider-sessions/
  <provider>/
    <case>/
      manifest.json
      request.json
      process.json
      stdout.jsonl|stdout.json|stdout.txt
      stderr.txt
      expected-provider-session.json
      expected-response-events.ndjson
      expected-invocation-result.json
```

Harness-owned sample cases live under `harness/` (for example
`harness/load-smoke`) so loader and comparison helpers can prove end-to-end
fixture loading without claiming a real provider fidelity matrix. Provider
cases land under `<provider>/<case>/` as provider golden work lands.
