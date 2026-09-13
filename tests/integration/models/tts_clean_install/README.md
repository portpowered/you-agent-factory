# Windows TTS clean-install probe

This directory is a test-only, source-blind preparation bundle. The immutable
manifest and report schemas are the machine-readable handoff contract for a
later real Windows TTS validation gate.

The current entry point performs only fail-closed preflight:

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
stories are added.
