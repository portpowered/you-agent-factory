Read `prd.json`, `prd.md`, and `progress.txt`. Implement only the highest
priority story whose `passes` value is false. Run proportional tests, append
evidence and reusable learnings to `progress.txt`, and mark the story passed
only after its behavioral criteria hold. Commit reviewable product changes and
continue until every story passes, CI is terminal and green, blocking feedback
and conflicts are resolved, and the pull request is actually merged. Return
`<CONTINUE>` while work remains and `<COMPLETE>` only at that terminal state.
