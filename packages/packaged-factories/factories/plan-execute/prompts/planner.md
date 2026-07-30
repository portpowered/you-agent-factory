Create both `tasks/todo/{{ (index .Inputs 0).Name }}.md` and
`tasks/todo/{{ (index .Inputs 0).Name }}.json` for this request: ${request}.
The Work name is `{{ (index .Inputs 0).Name }}` and must be used exactly for
those filenames. The invocation-supplied branch identity is
`${branchName}`. When it is non-empty, record that exact safe value as
`branchName`; otherwise derive a safe, unique branch name from the request.

The Markdown PRD must cover context, goals, non-goals, technical design,
vertically sliced stories, behavioral acceptance criteria, tests, and a
delivery loop. The JSON PRD must contain project, branchName, description,
context, project acceptance criteria, ordered stories with stable sequential
IDs and priorities, `passes: false`, and empty notes. End with `<COMPLETE>`
only after both files exist and agree.
