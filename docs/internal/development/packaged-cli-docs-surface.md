# Packaged CLI Docs Surface

This note describes how the installed `you docs` command is maintained so
future topic additions stay aligned with the authored reference docs and the
observable CLI contract.

## Scope

The packaged CLI docs surface exists so customers can read the most important
reference topics directly from the installed binary without cloning the
repository. The installed surface is intentionally narrow:

- `you docs` with no topic prints an index of canonical packaged topics.
- `you docs <topic>` prints the raw packaged markdown for one topic.
- `you docs --help` remains standard Cobra help.

This surface must not add wrapper prose around topic output. The topic command
returns the authored markdown body exactly as packaged so terminal output,
copy/paste, and smoke coverage stay stable.

## Source Of Truth

Customer-facing authored markdown under `docs/reference/` remains the source of
truth for topic content and terminology. The installed CLI serves synchronized
packaged copies from `pkg/cli/docs/reference/` because the binary must keep
working when the repository docs tree is absent.

When you add or revise a packaged topic:

1. Update the canonical authored page in `docs/reference/`.
2. Synchronize the packaged copy in `pkg/cli/docs/reference/`.
3. Update the topic registration entry in `pkg/cli/docs/docs.go`.
4. Update focused tests for index output, topic output, aliases, and unsupported
   topic behavior.

Do not treat the packaged copy as an independent authoring surface. If the two
markdown files diverge, the repository docs should win and the packaged copy
should be resynchronized.

## Topic Registration

`pkg/cli/docs/docs.go` owns the single registration table for the packaged
surface. Keep each topic in one `topicDocuments` entry with:

- canonical topic name
- packaged markdown path
- customer-facing index description
- explicit display order
- compatibility aliases, if any

`SupportedTopics()` is the canonical list used for the printed index and
unsupported-topic errors. Keep it canonical-only and deterministic. Aliases are
accepted commands, not first-class topics.

`SupportedTopicCommands()` is the command-wiring list. It can include canonical
topics plus compatibility aliases so Cobra accepts older names without adding
alias noise to the index or error messages.

## Alias Policy

Aliases are acceptable only for compatibility when an older topic name still
has user value, such as singular-versus-plural normalization or a renamed
customer-facing term.

When you keep an alias:

- register it in the same `topicDocuments` entry as the canonical topic
- make it resolve to the same raw markdown as the canonical topic
- keep the printed docs index focused on the canonical topic name
- keep unsupported-topic errors focused on canonical topics unless the CLI
  contract intentionally changes

Do not add redirect wrappers such as "use `you docs <canonical>` instead" to
alias output. The alias should behave like the canonical topic for customers.

## Test Surfaces

When the packaged docs surface changes, update the focused Go tests that prove
observable behavior:

- `pkg/cli/docs/docs_test.go` for topic registration metadata, deterministic
  ordering, canonical supported-topic output, alias behavior, unsupported-topic
  errors, and exact raw-markdown reads from the packaged files.
- `pkg/cli/root_run_test.go` for Cobra command behavior, including `you docs`
  index output and help text expectations at the root-command layer.
- `tests/functional/smoke/cli_docs_smoke_test.go` for installed-binary-style
  proof that packaged topics remain available when the repository docs tree is
  not present.

Prefer assertions on rendered CLI behavior and stable markdown content. Do not
replace these tests with source-inventory checks or path-only assertions.

## Maintainer Checklist

Before a packaged docs change is review-ready, confirm:

- the authored `docs/reference/` page and packaged `pkg/cli/docs/reference/`
  copy match for the affected topic
- the `topicDocuments` entry has the right canonical name, order, description,
  packaged path, and aliases
- `you docs` still lists only canonical topics in the expected order
- `you docs <topic>` returns raw markdown with no wrapper formatting
- any compatibility alias returns the same markdown as the canonical topic
- unsupported-topic errors still list canonical topics in deterministic order
- the docs package, root command, and functional smoke tests were updated as
  needed
