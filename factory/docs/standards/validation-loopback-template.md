# Validation loopback template

The loopback validator runs from a clean environment and does not silently fix
implementation defects.

```markdown
# Validation report: <behavior lane or plan>

## Environment and artifact

- Commit/build identifier:
- Environment and configuration:
- Customer entry point:
- Real and substituted dependencies:
- Cost/call budget used:

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |

## Customer journey

1. <Exact step, observed output, and artifact reference.>

## Cross-task integration and usability

- Documentation discoverability:
- Permission and error behavior:
- Persistence/reload behavior:
- Accessibility/keyboard/responsive behavior:
- Operational signals:

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |

## Verdict

<PASS | FAIL | BLOCKED>

## Delta-plan request [Required for FAIL/BLOCKED]

- Affected behavior and criterion:
- Root-cause evidence or remaining uncertainty:
- Smallest recommended correction/prerequisite:
- Dependencies and retest scope:
```
