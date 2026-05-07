# Templates Reference

Use this page when you need the supported Go-template surfaces, the core
variable families, and the quoting rules that differ between Markdown and
JSON.

## Current Contract

- Agent Factory renders templates with Go `text/template`.
- Workstation prompt bodies and files referenced by `promptFile` are template
  surfaces.
- Script-worker `args`, workstation `workingDirectory`, workstation `worktree`,
  and workstation `env` values are also template surfaces.
- `.Inputs` is the token-data root. `.Context` is the workflow and execution
  context root.
- Use canonical field names from the current prompt-variable contract. Invalid
  template syntax or missing field names fail rendering.

## Supported Surfaces

| Surface | Where authors put it | What it is for |
|---------|----------------------|----------------|
| Prompt body | `workstations/<name>/AGENTS.md` markdown body | Main rendered user message |
| `promptFile` content | File referenced by `promptFile` | External prompt template instead of inline markdown body |
| Script `args` | `workers/<name>/AGENTS.md` frontmatter | Per-dispatch command arguments |
| `workingDirectory` | `factory.json` or workstation frontmatter | Execution working directory |
| `worktree` | `factory.json` or workstation frontmatter | CLI provider worktree path |
| `env` values | `factory.json` or workstation frontmatter | Per-dispatch environment values |

## Core Variable Families

| Variable family | Use it for | Example |
|-----------------|------------|---------|
| `.Inputs` | Current work item data such as payload, tags, IDs, relations, and retry history | `{{ (index .Inputs 0).Payload }}` |
| `.Inputs[N].Tags` | Per-token metadata lookups | `{{ index (index .Inputs 0).Tags "branch" }}` |
| `.Inputs[N].History` | Attempt-aware prompts and retries | `{{ (index .Inputs 0).History.AttemptNumber }}` |
| `.Context` | Execution context such as working dir, artifact dir, project, and env | `{{ .Context.WorkDir }}` |

Use `index` for map lookups such as tags and environment values. See
[`prompt-variables.md`](prompt-variables.md) for the full variable
inventory.

## Quoting Rules

Use normal quotes inside Markdown prompt templates:

```text
Branch: {{ index (index .Inputs 0).Tags "branch" }}
```

Escape inner quotes when the template appears inside a JSON string:

```json
{
  "workingDirectory": "{{ index (index .Inputs 0).Tags \"worktree\" }}"
}
```

That escaping rule matters because JSON strings use double quotes. In Markdown
prompt bodies and prompt files, the template expression can use normal quotes.

## Minimal Authoring Checklist

- Use `.Inputs` for submitted work data and `.Context` for execution context.
- Use `index` for tag or env map lookups.
- Keep JSON template expressions escaped inside string literals.
- Keep Markdown prompt expressions unescaped and readable.
- Link out to the deeper package docs instead of inventing new template roots
  or retired aliases.

## Related

- [CLI reference landing page](README.md)
- [Package docs index](../README.md)
- [Prompt variables](prompt-variables.md)
- [Workstations and workers](workstations-and-workers.md)
- [Author AGENTS.md](authoring-agents-md.md)
