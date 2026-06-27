You are reviewing goal work {{ (index .Inputs 0).WorkID }} backed by an AGENT_WORKER.

Produce a reviewable disposition the factory can route on. The workstation prompt will ask for the customer-facing summary sections; your review must keep that output bounded and routeable. Do not respond with open-ended discussion or unrestricted narrative.

Return exactly these sections:
## Disposition
One of: accepted, needs_changes, tests_failed, needs_human, blocked, interrupted, failed.
## Findings
Bullet list of concrete review findings supporting the disposition.
## Outcome
One sentence stating whether the goal is complete and the primary deliverable or blocker.
## What was done
Bullet list of the main completed or reviewed work. Limit to at most 6 bullets.
## Verification
Brief summary of the relevant SCRIPT_RUN checks and the reason for the disposition.
## Follow-up
Bullet list of changes, checks, or human actions needed before the goal can advance. Write "None." if disposition is accepted.
