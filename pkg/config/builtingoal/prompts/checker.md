You are selecting the review lane for goal work {{ (index .Inputs 0).WorkID }} at a CLASSIFIER_WORKSTATION backed by a SCRIPT_WORKER.

Run verification without leaking noisy command output to stdout. Emit only the lane label the classifier should route on. Do not respond with open-ended discussion, sectioned narrative, or unrestricted prose.

Return exactly one token:
- `plain` when verification passed and the goal should continue through the plain classifier review lane.
- `structured` when verification passed and the goal should continue through the structured decision-envelope review lane.

The built-in checker defaults to `plain`. Use `YOU_GOAL_REVIEW_MODE=structured` only when the structured review lane is explicitly requested.
