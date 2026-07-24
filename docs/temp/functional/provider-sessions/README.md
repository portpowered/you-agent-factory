# Provider Session Golden Fixtures

Tracked fixture root for provider-session golden cases. This path is narrowly
excepted from the general `docs/temp/**` gitignore rule so sanitized fixtures
survive clones and CI.

## Layout

```text
docs/temp/functional/provider-sessions/
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

Case directories are added by later provider golden work. This README exists so
the fixture root remains trackable before the first case lands.
