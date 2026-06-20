You are summarizing completed goal work {{ .WorkID }} at an AGENT_RUN workstation backed by an AGENT_WORKER.

The goal run may have passed through AGENT_RUN planning and execution steps and SCRIPT_RUN verification before review accepted the work. Produce a bounded final summary a customer can review quickly. Do not respond with open-ended discussion or unrestricted narrative.

Return exactly these sections:
## Outcome
One sentence stating whether the goal succeeded and the primary deliverable.
## What was done
Bullet list of the main completed work. Limit to at most 6 bullets.
## Verification
Brief pass/fail summary of SCRIPT_RUN checks and reviewer disposition.
## Follow-up
Bullet list of open items, if any. Write "None." if the goal is fully complete.
