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

## OS-boundary self-test

The focused integration matrix consumes a helper test executable compiled once
by the integration/build lane. It must be supplied by absolute path and its
lowercase SHA-256; the runner rejects a missing, non-regular, reparse-point, or
hash-mismatched helper before starting any child:

`tts-clean-install-helper/v2`

```powershell
$helper = Join-Path $env:TEMP 'tts-clean-install-helper.exe'
go test -c -o $helper ./tests/integration/models/tts_clean_install
$hash = (Get-FileHash -LiteralPath $helper -Algorithm SHA256).Hash.ToLowerInvariant()
$env:INFINITE_YOU_TTS_PREBUILT_HELPER_PATH = $helper
$env:INFINITE_YOU_TTS_PREBUILT_HELPER_SHA256 = $hash
```

The helper creates no LocalAI, model, download, or external-network activity.
It only emits deterministic events/WAV bytes and held processes for the real
Windows pipe, process-tree, filesystem, and loopback boundaries. The Windows
runner drains stdout and stderr concurrently, assigns owned helpers to a
kill-on-close Job Object, and records readiness before timeout/cancellation.
The listener uses IPv4 loopback port `0`; port `7437` is forbidden, and an
unrelated sentinel is checked after owned cleanup.

Run the bounded matrix with:

```text
go test ./tests/integration/models/tts_clean_install -count=10
go test -race ./tests/integration/models/tts_clean_install -count=3 -timeout 20m
go vet ./tests/integration/models/tts_clean_install
make pkg-file-count
git diff --check
```

OS-boundary cases skip on non-Windows hosts; the pure component tests remain
portable. The integration lane builds the helper once and passes the same
absolute path/hash through `INFINITE_YOU_TTS_PREBUILT_HELPER_PATH` and
`INFINITE_YOU_TTS_PREBUILT_HELPER_SHA256`. These tests are preparation evidence
only and do not claim real installer, backend, model, or speech semantics.
