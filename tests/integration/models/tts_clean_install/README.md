# Windows TTS clean-install probe

This directory is a test-only, source-blind preparation bundle. The immutable
manifest and report schemas are the machine-readable handoff contract for a
later real Windows TTS validation gate.

The entry point performs fail-closed preflight before any effect. The Go
component in this package also projects a controlled, source-blind journey
with the ordered steps `install`, installed identity, help, docs, models,
cold invoke/readiness, warm `--offline` invoke/readiness, remove, post-remove,
and cleanup. Two named WAV records retain byte counts, SHA-256, RIFF, duration,
and non-silent-sample evidence; raw text, voice markers, URLs, and ambient
paths are redacted.

```powershell
pwsh -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass `
  -File .\invoke.ps1 `
  -ArtifactPath <absolute-existing-file> `
  -ArtifactIdentity <immutable-build-identity> `
  -ArtifactSha256 <64-lowercase-hex> `
  -ManifestPath <absolute-existing-manifest.json> `
  -ReportPath <absolute-path-under-an-empty-task-owned-output-root>
```

It performs no child, listener, or network action. Exit `2` is an input or
identity rejection; a matching manifest is recorded as an `INCONCLUSIVE`
preparation report and exits `3` until the public journey and OS-boundary
stories are added. Controlled fixtures cannot mark real LA-02, LA-06, LA-09,
LA-14, or LA-15 acceptance as PASS.
