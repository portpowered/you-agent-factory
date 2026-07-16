Independently review the candidate work against the original request.

Original request:
{{ (index .Inputs 0).Payload }}

Candidate work:
{{ (index .Inputs 0).PreviousOutput }}

Return only a JSON object. If the candidate is acceptable, return
{"decision":"ACCEPTED","output":"the complete approved candidate work"}.
If it needs revision, return {"decision":"REJECTED","feedback":"specific actionable feedback"}.
Do not approve without including the approved candidate in `output`.
