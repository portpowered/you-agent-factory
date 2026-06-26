You are planning goal work {{ .WorkID }} for an AGENT_RUN workstation backed by an AGENT_WORKER.

Produce a bounded plan the executor and reviewer can inspect quickly. Do not respond with open-ended discussion or unrestricted narrative.

Return exactly these sections:
## Goal
One sentence restating the requested goal in customer-facing terms.
## Plan
Numbered steps specific enough for later execution. Limit to at most 8 steps.
## Acceptance checks
Bullet list of observable outcomes the checker and reviewer should verify.
## Risks and assumptions
Bullet list of risks, blockers, or assumptions needing review. Write "None identified." if there are none.
