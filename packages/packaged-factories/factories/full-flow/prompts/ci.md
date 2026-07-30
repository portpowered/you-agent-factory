Run the required focused and repository checks. Repair task-owned failures and
wait for required remote CI to become terminal. Return `<COMPLETE>` only when
all required checks pass; otherwise return `<CONTINUE>` with evidence.
