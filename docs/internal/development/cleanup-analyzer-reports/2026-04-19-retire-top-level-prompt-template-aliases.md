# Cleanup Analyzer Report: Retire Top-Level Prompt Template Aliases

Date: 2026-04-19

## Scope

Agent Factory cleanup evidence for retiring legacy top-level token aliases from
prompt templates, workstation template fields, script arguments, examples,
functional fixtures, and `workers.PromptData`.

The retired top-level fields are:

- `.WorkID`
- `.WorkTypeID`
- `.Name`
- `.Payload`
- `.Project`
- `.Tags`
- `.History`
- `.PreviousOutput`
- `.RejectionFeedback`

Active templates must read token data through `.Inputs` and workflow/runtime
data through `.Context`. Historical cleanup reports under
`libraries/agent-factory/docs/development/cleanup-analyzer-reports/` are
excluded from active-match conclusions because cleanup reports intentionally
preserve retired text as audit evidence. Non-rendered recording fixtures are
also excluded from active-template conclusions.

## Analyzer Commands

The branch-base snapshot for reproducible before comparisons is:

```text
5e20588a33f5ac2f9c7a376ea69c8a091c8ac4b1
```

Initial active-template inventory command shape, from the cleanup request:

```powershell
rg -n "\{\{\s*\.(WorkID|WorkTypeID|Name|Payload|Project|Tags|History|PreviousOutput|RejectionFeedback)" libraries/agent-factory/docs libraries/agent-factory/examples libraries/agent-factory/pkg/workers libraries/agent-factory/tests/functional_test -g "*.go" -g "*.md" -g "*.json" -g "!docs/development/cleanup-analyzer-reports/**" -g "!tests/functional_test/testdata/*recording*.json"
```

The initial customer-supplied analyzer result before migration found 229
matches across 55 files. The matches were active checked-in prompt, worker-test,
script-argument, workstation-field, docs, example, and fixture template strings
that used top-level token aliases.

The branch-base equivalent for the PRD active paths was:

```powershell
git grep -n -E "\{\{\s*\.(WorkID|WorkTypeID|Name|Payload|Project|Tags|History|PreviousOutput|RejectionFeedback)" 5e20588a33f5ac2f9c7a376ea69c8a091c8ac4b1 -- libraries/agent-factory/docs libraries/agent-factory/examples libraries/agent-factory/pkg/workers libraries/agent-factory/tests/functional_test ':!libraries/agent-factory/docs/development/cleanup-analyzer-reports/**' ':!libraries/agent-factory/tests/functional_test/testdata/*recording*.json'
```

That reproducible branch-base command returned 192 matches across 47 files.
Additional active template producers outside the PRD path list, including root
factory scaffolding, CLI init fixtures, and package test utilities, were
migrated during the cleanup because normal verification still renders or emits
those templates.

Final active-template inventory command:

```powershell
rg -n "\{\{\s*\.(WorkID|WorkTypeID|Name|Payload|Project|Tags|History|PreviousOutput|RejectionFeedback)" libraries/agent-factory/docs libraries/agent-factory/examples libraries/agent-factory/pkg/workers libraries/agent-factory/tests/functional_test -g "*.go" -g "*.md" -g "*.json" -g "!docs/development/cleanup-analyzer-reports/**" -g "!tests/functional_test/testdata/*recording*.json"
```

Worker code inventory command:

```powershell
rg -n "buildPromptData|TokenData|PromptData|ResolveTemplateFields|promptProject|ResourceToken|AllResourceTokens" libraries/agent-factory/pkg/workers -g "*.go"
```

Branch-base worker code inventory command:

```powershell
git grep -n -E "buildPromptData|TokenData|PromptData|ResolveTemplateFields|promptProject|ResourceToken|AllResourceTokens" 5e20588a33f5ac2f9c7a376ea69c8a091c8ac4b1 -- 'libraries/agent-factory/pkg/workers' ':(glob)libraries/agent-factory/pkg/workers/**/*.go'
```

## Before Inventory

The initial active-template inventory found 229 matches across 55 files before
the migration. The branch-base active-path rerun found 192 matches across 47
files in:

- `libraries/agent-factory/docs`
- `libraries/agent-factory/examples`
- `libraries/agent-factory/pkg/workers`
- `libraries/agent-factory/tests/functional_test`

The branch-base worker code inventory found 54 matches across 6 files. The
important retired contract was in `libraries/agent-factory/pkg/workers/prompt.go`:

- `PromptData` embedded `TokenData`, which exposed top-level token fields.
- `buildPromptData(...)` appended inputs to `.Inputs`, then selected a first
  non-resource token as synthetic top-level `TokenData`.
- `.Context.Project` was derived from the synthetic top-level token tags when
  no workflow context was present.
- Resource-token tests covered fallback behavior tied to top-level alias
  compatibility.

## After Inventory

The final active-template inventory returned 0 matches across 0 files. Active
docs, examples, worker tests, and functional fixtures no longer teach or depend
on top-level token aliases.

The final worker code inventory returned 52 matches across 6 files:

- `PromptData` now exposes only `Inputs []TokenData` and
  `Context PromptContext`.
- `buildPromptData(...)` appends every dispatch input token to `.Inputs`,
  including resource tokens.
- `buildPromptData(...)` has no branch that selects a primary non-resource
  token to populate top-level prompt fields.
- `ResolveTemplateFields(...)`, script workers, prompt rendering, and
  workstation execution all render through the same canonical `PromptData`
  model.
- `promptProject(...)` remains token-scoped for `(index .Inputs N).Project`.
  `.Context.Project` is resolved separately by `promptContextProject(...)`:
  explicit context wins, the first non-resource work input may provide the
  documented fallback, and resource-only dispatches use `default-project`.
- `ResourceToken` and `AllResourceTokens` matches remain in tests that prove
  resource tokens stay visible through `.Inputs`.

## Historical Matches

The active-template inventory intentionally excludes historical reports and
non-rendered recording fixtures. A broader scan that includes ad hoc recordings
and root factory logs returned retained historical matches in 3 files:

- `libraries/agent-factory/tests/adhoc/factory-recording-04-11-02.json`
- `libraries/agent-factory/tests/adhoc/factory-recording-batch.json`
- `factory/logs/adhoc-recording-batch.json`

These files preserve captured historical prompt text or generated logs. They
are not rendered by the normal active verification path for this cleanup, so
they are classified as historical evidence rather than active template drift.

This report names the retired fields but avoids literal retired template
snippets so the final active inventory remains reproducible from the repository
root even when the report path is included in the searched tree.

## Validation Commands

Go commands were run from `libraries/agent-factory`. Inventory commands were
run from the repository root.

```powershell
go test ./pkg/workers ./tests/functional_test -count=1
make lint
rg -n "\{\{\s*\.(WorkID|WorkTypeID|Name|Payload|Project|Tags|History|PreviousOutput|RejectionFeedback)" libraries/agent-factory/docs libraries/agent-factory/examples libraries/agent-factory/pkg/workers libraries/agent-factory/tests/functional_test -g "*.go" -g "*.md" -g "*.json" -g "!docs/development/cleanup-analyzer-reports/**" -g "!tests/functional_test/testdata/*recording*.json"
rg -n "buildPromptData|TokenData|PromptData|ResolveTemplateFields|promptProject|ResourceToken|AllResourceTokens" libraries/agent-factory/pkg/workers -g "*.go"
```

Results on 2026-04-19:

- `go test ./pkg/workers ./tests/functional_test -count=1`: passed.
- `make lint`: passed; `go vet ./...` completed and the deadcode baseline matched.
- Final active-template inventory: 0 matches across 0 files.
- Final worker code inventory: 52 matches across 6 files; no primary-token
  alias branch remains.
