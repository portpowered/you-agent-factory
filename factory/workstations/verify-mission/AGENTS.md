# Bound mission verification
{{range .Inputs}}{{if eq .DataType "work"}}
Payload: {{.Payload}}
Work ID: {{.WorkID}}
Tags: {{.Tags}}
Previous output: {{.PreviousOutput}}
Correction feedback (verbatim checker reason): {{.RejectionFeedback}}
{{end}}{{end}}
Factory Session: {{.Context.SessionID}}
On correction, fix the field named in the checker reason above.
Keep the full bound mission and available evidence.

Execute only the bound mission, including cron-origin missions. Run every named command/read and report every named value, unit and source; never invent evidence.
Use canonical server http://127.0.0.1:7437 and the explicit bound Factory Session for all API-backed commands. Preserve the payload, Work identity and project tag.
Read factory/docs/batch-inputs.md for corrective batch shape and relations.
Before deciding, resolve read-only preconditions yourself: git fetch origin main, file reads, and GET requests.
Fetch missing merge objects before ancestry checks; unavailable objects are not observed product defects.
Use the read helper for these operations, retrying once; do not retry fetch, reads or checks beyond its budget.
Use python factory/scripts/mission-read.py [--required] -- <read-command> [args...] for authorized reads, including GitHub reads. Mark --required only when the mission requires that read. Each failed read retries once; retain both attempts and available output. Optional exhaustion is a recorded gap, not alone FAILED.
Never retry mutations through the helper or add an agent retry after exhaustion.
Observed defects and command failures unrelated to prerequisites return FAILED with the exact reason and available values.
If the only problem is an unmet prerequisite, return PRECONDITION even with partial measurements; retain available evidence in output.precondition and output.measurements.
This includes exhausted required reads and a daemon not restarted onto a fix; file no corrective Work or proposal for it.
Do not restart the daemon or mutate product state to satisfy a precondition.
On correction feedback, correct the output shape while retaining the mission, measurements and original failure evidence; do not reset the rejection marker.

Only an observed product or factory defect qualifies as a corrective gap below.
File corrective work as a lane that changes behavior with tests shipped in the same PR.
Never file a lane whose outcome is evidence, verification, characterization, measurement or re-running a check.
Never file amendment or retrospective lanes, or corrective Work merely to make merge evidence available.
For gaps, prepare a narrow corrective batch plus its own dependent loopback.
For untagged Work, use a stable request ID, dry-run, idempotent submission and verified receipt. Inspect by request ID before repeating an uncertain submission.
Verify returned request ID, Session ID, Work count and Work IDs and read the Work.
Use you --server http://127.0.0.1:7437 --json submit batch --dry-run --session {{.Context.SessionID}} <file>,
then the same command without --dry-run. Keep the receipt and named values.
For tagged Work, resolve the main checkout as the parent of the absolute git common directory. Save the raw proposal at docs/temp/projects/<project>/proposals/<loopback-name>.json in the main checkout.
Keep stable request ID, origin Work ID and original validation findings.
Dry-run only in the bound Factory Session; submit no Project children.
The existing project-report route informs the owning lead, who owns admission
and follow-up validation. Keep measurements and proposal path; do not emit
another report or thoughts join. Admission ownership alone never causes FAILED.
Write/dry-run/admission failures return FAILED with exact reason and any saved path.
Do not use Work controls, equivalent APIs, canonical edits or operatorOverride.
Return only decision, feedback and output. decision is ACCEPTED, FAILED or PRECONDITION,
feedback is a string, and output is a native JSON object containing non-empty measurements
or a non-empty precondition naming the unmet requirement with available values.
Use the exact key output.precondition with a non-blank string for an unmet precondition.
Alternatively, output.precondition may be a non-empty object with at least one immediate non-blank string value.
Object example: {"precondition":{"requirement":"daemon running the fix","observed":"old revision","needed":"restart"}}.
Every supplied measurements list must be non-empty; name/source must be non-blank.
Each measurement requires value. Zero, false and null are valid values.
Keep optional read records, units, corrective receipt and proposal path in output.
A path or receipt alone cannot complete a mission.
Measurements example:
{"decision":"ACCEPTED","feedback":"Measured pending Work.","output":{"measurements":[{"name":"pending","value":0,"source":"authorized Work list"}]}}
Unmet precondition example:
{"decision":"PRECONDITION","feedback":"Required read unavailable.","output":{"precondition":"Required recording read unavailable; pending Work observed: 0"}}
{"decision":"PRECONDITION","feedback":"Daemon not restarted onto the fix.","output":{"precondition":{"requirement":"daemon running the fix","observed":"old revision","needed":"restart"}}}
