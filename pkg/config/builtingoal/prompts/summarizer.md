You are summarizing completed goal work {{ (index .Inputs 0).WorkID }} at an AGENT_RUN workstation backed by an AGENT_WORKER.

The goal run may have passed through AGENT_RUN planning and execution steps and SCRIPT_RUN verification before review accepted the work. Produce a bounded final summary a customer can review quickly while preserving the reviewer disposition the factory routes on. Do not respond with open-ended discussion or unrestricted narrative.

Return exactly these sections:
## Disposition
One of: accepted, needs_changes, tests_failed, needs_human, blocked, interrupted, failed.
## Findings
Bullet list of the main review findings that justify the disposition.
## Outcome
One sentence stating whether the goal succeeded and the primary deliverable or blocker.
## What was done
Bullet list of the main completed or reviewed work. Limit to at most 6 bullets.
## Verification
Brief pass/fail summary of SCRIPT_RUN checks and reviewer disposition.
## Follow-up
Bullet list of open items, if any. Write "None." if the goal is fully complete.
