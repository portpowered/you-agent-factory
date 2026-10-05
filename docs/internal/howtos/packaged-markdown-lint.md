# Packaged Markdown validation

Run `make docs-reference-check` to validate `docs/README.md` and Markdown files
under `docs/reference`. `make docs-reference-smoke` retains that check followed
by the existing packaged CLI documentation scenarios. Run
`make lint-migration-smoke LINT_MIGRATION_COHORT=markdown` for protecting static
fixtures. Backend Lint runs this cohort before Go formatting and plugin
preparation; its `all` cohort also runs them before compiler fixtures.

The supported environment provides Bash, GNU find, iconv, Go 1.25 or later,
Python 3.10 or later and pip. On Windows, put Git for Windows' `bin` and
`usr/bin` directories before the Windows Bash launcher and Windows find.
Preparation installs gomarklint v3.3.1 and the exact Python wheel dependencies
in `scripts/docs-markdown-lint-requirements.txt` into the checkout-local
`.cache/docs-markdown-lint`. It uses the existing Go toolchain with
`GOTOOLCHAIN=local`; it never installs globally or downloads a Go toolchain.
Remove that cache to prepare it again. Installation failures never write the
readiness stamp. Missing prepared tools still fail on invocation.

GNU find completes discovery before any lint execution. Discovery includes
nested and hidden directories, mixed-case `.MD` extensions, filenames with
spaces and linked Markdown files; other extensions are ignored. Missing roots,
unreadable directories and unreadable files fail visibly. iconv validates UTF-8
and copies unchanged bytes to invocation-owned scratch. Each source path is
printed before validation, so subsequent scratch-path diagnostics refer to that
original input. The pipeline does not edit sources and stops at the first fault.

[gomarklint](https://github.com/shinagawa-web/gomarklint) checks unclosed code
blocks and the final LF. [PyMarkdown MD047](https://pymarkdown.readthedocs.io/en/latest/plugins/rule_md047/)
also checks the final newline, including frontmatter-only content that
gomarklint exempts. Empty files and CRLF ending in LF remain valid; a nonempty
CR-only ending fails gomarklint even when Python normalizes newlines.
Maintained parser behavior accepts a longer outer fence containing shorter
fence literals and lines longer than the former Go scanner's 64 KiB limit.
Unclosed backtick/tilde fences, mismatched markers and short closing fences fail.
No unrelated Markdown style rules or external link requests are enabled.

Pinned check-jsonschema validates `.gomarklint-docs.json` against the authored
schema before checking content, even empty files. Unknown, missing, disabled or
downgraded required rules and malformed or missing configuration fail closed.
Missing commands, invalid pins and failed installs also return nonzero with
their failed stage. The static fixture logs and tool identities are written to
`.artifacts/lint-migration-smoke/markdown-*`; nonprivileged POSIX CI supplies the
permission-denied evidence unavailable on Windows.

Revert the Make recipe, configuration, schema, pins, fixtures and checker
retirement together if rollback is required. Historical command evidence and
immutable project plans retain their original references. Review owns terminal
PR CI and merge; the independent retirement loopback owns fresh-main command
absence and latest-main-push Backend Lint proof.
