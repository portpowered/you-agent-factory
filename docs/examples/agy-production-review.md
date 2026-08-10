# AGY production review composition

This example shows where the two first-party media-review Factories belong in a
render-and-assemble pipeline. The roles consume paths; they do not become a
media-processing wrapper.

## Workflow

~~~text
render clip
    │
    ├─ execution/provider/media failure ───────────────► failed
    │
    ▼
@you/agy-clip-qa(clipPath, shotSpecification)
    ├─ execution, schema, or inaccessible-file failure ─► failed
    ├─ verdict = reroll ────────────────────────────────► reroll clip
    │
    ▼ verdict = pass
mechanical checks on the accepted clip
    ├─ check rejects the artifact ──────────────────────► reroll clip
    ├─ checker cannot execute/read the media ───────────► failed
    │
    ▼ checks pass
assemble completed cut
    ├─ assembly cannot execute/read its inputs ─────────► failed
    │
    ▼
@you/agy-cold-watch(cutPath)
    ├─ execution, refusal, or invalid report ───────────► failed
    ├─ recommendation = reroll ─────────────────────────► reroll cut
    └─ recommendation = pass ──────────────────────────► pass
~~~

pass, reroll, and failed are intentionally different routes:

- pass means a successful inspection accepted the artifact.
- reroll means a successful inspection found a defect or specification
  deviation and the upstream media should be rendered again.
- failed means the system could not obtain a trustworthy inspection or could
  not execute an upstream operation. It must not be converted into reroll.

## PowerShell-safe invocation sketch

Run these commands from the directory that AGY will receive as its existing
--add-dir workspace. The renderer and assembler are upstream pipeline
operations; their outputs are passed directly to the review roles.

~~~powershell
$jobRoot = (Resolve-Path '.\production\job-42').Path
$clipPath = Join-Path $jobRoot 'renders\SH080.mp4'
$shotSpecification = 'A silver-haired woman points at a bright star; no speech is audible.'

# The renderer creates $clipPath. A missing output is an execution failure.
if (-not (Test-Path -LiteralPath $clipPath -PathType Leaf)) {
  throw "renderer failed to create readable clip: $clipPath"
}

$qaJson = you run --named @you/agy-clip-qa --clip-path $clipPath --shot-specification $shotSpecification --output primary --no-record
if ($LASTEXITCODE -ne 0) {
  Write-Output 'failed: clip-QA did not produce a verdict'
  return
}

try { $qa = $qaJson | ConvertFrom-Json -ErrorAction Stop }
catch {
  Write-Output 'failed: clip-QA returned malformed JSON'
  return
}

switch ($qa.verdict) {
  'pass'   { break }
  'reroll' { Write-Output 'reroll: render the clip again'; return }
  default  { Write-Output 'failed: clip-QA returned an invalid verdict'; return }
}

# Run the pipeline's mechanical duration/codec/mux checks here. A checker
# error or unreadable input is failed; a detected media defect is reroll.
# Assemble the completed cut only after those checks pass.
$completedCut = Join-Path $jobRoot 'assembled\completed-cut.mp4'
if (-not (Test-Path -LiteralPath $completedCut -PathType Leaf)) {
  Write-Output 'failed: assembly did not produce a readable completed cut'
  return
}

$report = you run --named @you/agy-cold-watch --cut-path $completedCut --output primary --no-record
if ($LASTEXITCODE -ne 0) {
  Write-Output 'failed: cold-watch did not produce a report'
  return
}

$recommendationMatch = [regex]::Match($report, '(?im)^[ \t]*##[ \t]+Overall recommendation[ \t]*\r?\n(?:[ \t]*\r?\n)*[ \t]*Recommendation:[ \t]*(?:\*\*)?(pass|reroll)(?:\*\*)?[ \t]*\r?$')
if (-not $recommendationMatch.Success) {
  Write-Output 'failed: cold-watch report has no valid recommendation'
  return
}

switch ($recommendationMatch.Groups[1].Value.ToLowerInvariant()) {
  'pass'   { Write-Output 'pass: completed cut accepted'; break }
  'reroll' { Write-Output 'reroll: revise the completed cut'; break }
}
~~~

The cutPath passed to cold watch is the only creative context. Even if the
clip-rendering stage had a brief or the QA stage had a shot specification, do
not pass either one to @you/agy-cold-watch; its purpose is to report what the
completed artifact actually communicates.

## Path and failure boundary

Use a workspace-relative path such as .\renders\SH080.mp4 or an absolute path
inside the same workspace, such as
C:\production\job-42\renders\SH080.mp4. The existing ANTIGRAVITY adapter passes
that workspace through --add-dir and forwards the path text unchanged. No role
decodes, uploads, copies, probes, extracts frames, or extracts audio.

If a path is missing, unreadable, unsupported, or outside the directory AGY can
access, the invocation must fail with an actionable diagnostic. AGY can still
exit zero and report provider status SUCCESS for a refusal, so provider exit
status alone is not a production verdict.

## Offline verification

The fully offline end-to-end check replays the real AGY video, audio, clip-QA,
and missing-file recordings through the public process boundary:

~~~powershell
go test ./tests/functional/providers/agy -run '^TestAgyProductionReviewRolesThroughRootBuildProcess$' -count=1
~~~

That test also covers malformed/schema-invalid clip-QA output and provider
failure. It uses root.BuildProcess, Process.Execute, and only the
ProviderCommandRunner edge replacement; it does not start a live AGY process.
The existing operator-gated B1 smoke is separate and remains disabled in
ordinary CI.
