#!/usr/bin/env python3
"""Read-only lane prompt policy lint; not runtime admission or agent proof."""

import argparse
from pathlib import Path
import re
import sys


# Required clauses are deliberately explicit: diagnostics identify the policy
# to restore. Normalize whitespace only; semantic slice quality needs review.
PLAN_RULES = (
    ('slice', 'Plan each lane as ONE independently mergeable PR that delivers the whole ask.'),
    ('no-scope-cap', 'There is no story, criterion, line or file cap'),
    ('split', 'Split only when parts ship separately or one part must merge before another can be built'),
    ('successors', 'remaining names/outcomes/requirements and merge gates'),
    ('successor-section', 'Named successor slices — not admitted'),
    ('empty-successors', 'State "None" if empty'),
    ('successor-stories', 'exclude successors from userStories'),
    ('admission', 'only lead/operator admits them through existing routes'),
    ('merge-gate', "Each named successor must depend on this lane's merge before lead/operator admission"),
    ('immutable', 'Preserve immutable criteria/IDs, source-plan alignment, required sections/proof and later owning gates'),
    ('no-weakening', 'Never weaken acceptance'),
    ('no-routing', 'No runtime/routing change or invented approval'),
)
PROCESS_RULES = (
    ('minimal-status', 'Update minimal status/passes/blockers only'),
    ('immutable', 'Preserve requirements/amendments'),
    ('false-pass', 'never falsely pass unproved/delegated criteria'),
    ('entry', 'Keep concise visit/story, changed, blocker and next records'),
    ('interrupted', 'even blocked/interrupted'),
    ('handoffs', 'put deferred handoffs in the PR body too'),
    ('pre-pr', 'Retain observations in session before a PR exists'),
    ('no-commit', 'Never commit scaffolding/verification records'),
    ('legacy', 'without retroactive rewrite/truncation/compaction/archive'),
    ('push-count', 'Push at most once per visit, at its end, after focused tests/lint, when the visit changed code'),
    ('running-ci', 'If previous-head CI is still running, push anyway; the superseding push cancels it'),
    ('no-ci-wait', 'Never spend a visit only waiting for CI or return CONTINUE solely because CI is running'),
    ('pending-push', 'No ACCEPTED with final push pending'),
    ('blocker-push', 'stop on a contract conflict commits and pushes verified unblocked changes at visit end after focused tests/lint, within the one-push limit'),
    ('mailbox-push', 'Commit verified unblocked changes and push at visit end after focused tests/lint, within the one-push limit'),
    ('no-watcher', 'Do not spend process visits watching CI'),
    ('non-draft', 'Create/ready a non-draft PR'),
    ('auto-merge', 'gh pr merge <n> --auto --squash'),
    ('handoff', 'Stop with final head pushed, non-draft PR, CI started and blockers addressed'),
    ('review', 'review owns terminal CI/conflicts/merge'),
    ('comment-failure', 'Never claim evidence was published when it was not'),
)
CONFLICTS = (
    ('expanded-entry', r'APPEND to progress\.txt|## Progress Report Format|## Consolidate Patterns'),
    ('compaction', r'compact it first|delete the rest|archive (?:old|legacy) (?:prd|progress)'),
    ('extra-learning', r'The learnings section is critical|add it to the.*section at the TOP'),
    ('ci-exception', r'\(every push cancels that run\), unless|unless \(a\)'),
    ('previous-ci-gate', r'never (?:push )?during running previous-head CI|(?:do not|never) push (?:even )?while[^.]*CI[^.]*running'),
    ('ci-only-continue', r'If previous-head CI is (?:still )?running, retain local commits and return CONTINUE(?: with the push pending)?|(?:^|[.!?]\s*)return CONTINUE solely because CI is running'),
    ('ci-watcher', r'CI watching: at most ONE bounded watcher per head'),
    ('unconditional-blocker-push', r'Commit and push the unblocked work first|still pushes its verified commits'),
    ('manual-merge', r'gh pr merge <n> --squash'),
    ('scaffold-evidence', r'record (?:that )?exact result in progress\.txt'),
)

# These checks cover authored output policy, not customer runtime constraints.
OUTPUT_RULES = (
    ('customer-size', 'Do not prescribe arbitrary output-size requirements unless the customer explicitly asks for them.'),
    ('evidence-location', 'Measurements, timings, calibration runs and evidence belong in the PR body or a PR comment; CI evidence belongs only in PR comments.'),
    ('behavior-tests', 'Committed tests protect customer behavior and ship with the change.'),
    ('no-proof-files', 'Do not commit large one-off fixtures, calibration harnesses, evidence documents or proof files.'),
    ('closable', 'Each observable process outcome names one measurement or test and can close in one visit once the behavior and witness exist.'),
    ('no-escalating-proof', 'Do not invent gates that demand repeated or escalating proof; preserve independent review, CI and merge obligations.'),
)
# Detect familiar positive prescriptions, even beside the correct policy.
# Deliberately bounded diagnostics: semantic policy and customer exceptions
# still need independent review; numeric resource/runtime limits are unrelated.
OUTPUT_CONFLICTS = (
    ('output-size', r'(?:under|below|at most|maximum(?: of)?|target(?: of)?|budget(?: of)?|cap(?: of)?) (?:about )?[\d,]+(?:[-–][\d,]+)? (?:changed lines|lines(?: of (?:diff|output))?|files|tests|test cases|(?:UTF-8 )?bytes|KB|MB)'
     r'|(?:changed[- ]line|file[- ]count|test[- ]count|diff|packet|output)[^.]*? (?:target|budget|cap|limit)(?: is| of|:)? [\d,]+'
     r'|(?:JSON-byte|PR-line) caps|escalate indivisible scope'
     r'|at most one short four-line progress\.txt entry'),
    ('committed-proof', r'(?:must|always|require(?:d)?) commit (?:the |an? )?(?:proof|evidence document|calibration harness|one-off fixture)'
     r'|(?:commit|add) (?:the |an? )?(?:proof file|evidence document|calibration harness|one-off fixture) (?:to|in) (?:the )?(?:PR|repository)'),
)


def check_changed_line_budgets(prompts):
    """Reject numeric diff and scope caps in supplied path/text pairs, without I/O."""
    number = r'\d[\d,]*(?:[-–]\d[\d,]*)?'
    pattern = (
        rf'\b{number} changed[- ]lines?\b'
        rf'|\bchanged[- ]line budget(?: is| of|:)? {number}\b'
        rf'|\bdiff (?:limit|budget|cap)(?: is| of|:)? {number} lines?\b'
        rf'|\b{number} lines? \(added plus deleted\)'
    )
    scope = rf'\bat most {number} stories\b|\babout {number} (?:unique )?(?:process-owned )?criteria total\b'
    diagnostics = []
    for path, source in sorted(prompts.items()):
        normalized = ' '.join(source.split())
        if re.search(pattern, normalized, re.IGNORECASE):
            diagnostics.append(f'{path}:changed-line-budget: remove changed-line budgets from lane-facing prompts')
        if re.search(scope, normalized, re.IGNORECASE):
            diagnostics.append(f'{path}:scope-cap: remove story and criterion caps from lane-facing prompts')
    return diagnostics


def check_output_policy(prompts):
    """Diagnose size/proof policy in isolated role strings, without I/O."""
    diagnostics = []
    for owner in ('plan', 'process', 'review', 'planning-standard'):
        normalized = ' '.join(prompts.get(owner, '').split())
        for name, clause in OUTPUT_RULES:
            if clause not in normalized:
                diagnostics.append(f'{owner}:{name}: missing policy clause: {clause}')
        for name, pattern in OUTPUT_CONFLICTS:
            if re.search(pattern, normalized, re.IGNORECASE):
                diagnostics.append(f'{owner}:{name}: conflicting output policy; remove arbitrary output budgets or committed proof instructions')
    return diagnostics


RECOVERY_RULES = {
    'project-lead': (
        ('diagnosis', 'visit_cap_with_progress'),
        ('breaker', 'breaker_one_blocker'),
        ('deterministic', 'deterministic_failure'),
        ('attempt', 'positive integer'),
        ('uncapped', 'with no ceiling'),
        ('idempotency', 'with the same request ID'),
        ('adoption', 'tags.recovery-worktree'),
        ('relative-path', 'normalized repo-relative managed path'),
        ('binding', 'Bind replacements by targetWorkId'),
        ('closure', 'evidenced failed DEPENDS_ON closure'),
        ('no-controls', 'operatorOverride repair remain forbidden even after operator answers'),
    ),
    'project-lead-wake': (
        ('recovery', "lead's Corrected successor recovery procedure"),
        ('attempt', 'positive integer'),
        ('uncapped', 'with no ceiling'),
        ('binding', 'targetWorkId'),
        ('idempotency', 'with the same request ID'),
    ),
    'project-lead-checkin': (
        ('recovery', "lead's Corrected successor recovery procedure"),
        ('attempt', 'positive integer'),
        ('uncapped', 'with no ceiling'),
        ('binding', 'targetWorkId'),
        ('idempotency', 'with the same request ID'),
    ),
    'plan': (
        ('forward', 'forward recovery exactly to context.recovery'),
        ('tag', 'recovery-worktree tags'),
        ('relative-path', 'same normalized repo-relative'),
        ('lineage', 'originalLaneWorkId'),
        ('attempt', 'positive integer (not bool)'),
        ('uncapped', 'with no ceiling'),
    ),
    'process': (
        ('attempt', 'positive integer'),
        ('uncapped', 'with no ceiling'),
        ('packet', 'Select tasks/todo/'),
        ('adoption', 'context.recovery.workspace'),
        ('same-pr', 'never create a second PR'),
        ('ordinary', 'without the tag continues to use root prd.json'),
    ),
    'review': (
        ('attempt', 'positive integer'),
        ('uncapped', 'with no ceiling'),
        ('packet', 'select tasks/todo/'),
        ('adoption', 'context.recovery.workspace'),
        ('same-pr', 'never create a second PR'),
        ('ordinary', 'without this tag keeps root prd.json'),
    ),
}

RECOVERY_CONFLICTS = (
    ('ceiling', r'two[- ]successor|(?:at most|only|maximum of) (?:two|2) (?:accepted )?successors'
     r'|attempt(?:s)? (?:1/2|(?:(?:must be|to) )?(?:integer )?1 or 2)'
     r'|last[- ]successor|(?:final|last) (?:accepted )?successor'
     r'|attempt budget'
     r'|(?:exhausted|unused)[^.]* (?:lineage|budget)'
     r'|(?:lineage|attempt) (?:has|have)[^.]* (?:cap|ceiling)'),
)


def check_recovery_policy(prompts):
    """Diagnose controlled recovery text; runtime/lead judgment is separate."""
    diagnostics = []
    for owner, rules in RECOVERY_RULES.items():
        normalized = ' '.join(prompts.get(owner, '').split())
        for name, clause in rules:
            if clause not in normalized:
                diagnostics.append(f'{owner}:recovery-{name}: missing policy clause: {clause}')
        for name, pattern in RECOVERY_CONFLICTS:
            if re.search(pattern, normalized, re.IGNORECASE):
                diagnostics.append(f'{owner}:recovery-{name}: conflicting recovery instruction; remove or reconcile it')
    return diagnostics
MAILBOX_RULES = (
    ('lead-first', 'address the request to its project lead first'),
    ('untagged', 'A lane without a project tag keeps the current operator route'),
    ('decision', 'one explicit decision'),
    ('evidence', 'source/acceptance/rules citations and evidence'),
    ('recommendation', 'A recommended with evidence and tradeoffs'),
    ('identity', 'Project tag (or none), Factory Session, Work ID and addressed decision owner'),
    ('no-answer', 'no unauthorized widening or contract change'),
)
LEAD_RULES = (
    ('authority', 'Answer inside the immutable source plan, acceptance and rules.md pre-authorizations, including narrowing changes'),
    ('response', 'responses/<lane>.md'),
    ('progress', "this Project's progress.md"),
    ('exposure', 'widening public exposure'),
    ('owner', 'adding an owner or a second path'),
    ('baseline', 'growing a lint or boundary baseline'),
    ('contract', 'contradicting the immutable plan or acceptance'),
    ('tooling', 'factory/tooling defects'),
    ('forward', 'Write a separate operator request'),
    ('version', 'original lane request and its version'),
    ('no-speculation', 'do not place a speculative answer'),
    ('bridge', "preserve its authority and deliver the lane's binding response"),
    ('deduplicate', 'Use recorded request identities to avoid duplicate forwarding'),
    ('reconcile', 'reconcile outstanding forwarded requests and operator responses'),
    ('recheck', 'Recheck the live Session, lane Work ID, project tag and request mtime_ns'),
    ('atomic', 'rename atomically'),
)

OWNERSHIP_RULES = {
    'plan': (
        ('explicit', 'Every project and story criterion MUST carry an explicit `owner`: `process` or `review` in new planner output'),
        ('categories', 'Hosted/terminal CI results, merge, independent or post-merge validation, and reviewer judgment are review-owned'),
        ('direct-proof', 'Direct implementation proof and the implementation-stage delivery criterion are process-owned'),
        ('composite', 'A composite immutable criterion requiring review evidence is review-owned as a whole; retain its implementation work and later gates'),
        ('default', 'Missing owner defaults to process, including legacy string criteria'),
        ('invalid', 'Invalid explicit owners (unknown, null or non-string) are malformed metadata and never bypass a blocker'),
    ),
    'process': (
        ('gate', 'Set `decision` to `ACCEPTED` only when every retained process-owned criterion is passes:true'),
        ('default', 'Missing owner defaults to process, including legacy string criteria'),
        ('invalid', 'Invalid explicit owners never bypass a blocker'),
        ('false-process', 'A false process-owned criterion prevents ACCEPTED'),
        ('no-review-pass', 'Process never marks review-owned criteria true'),
        ('false-review', 'Unproved review-owned criteria remain false and do not block process ACCEPTED'),
        ('handoff', 'List their IDs and later gate IDs in envelope feedback and the PR handoff as "owned by review"'),
        ('story', 'Story passes reports retained process completion only, never review proof'),
        ('empty', 'Empty or all-review criterion sets still require complete retained implementation, a pushed final head, an open non-draft PR, CI started, auto-merge armed and blocking feedback addressed'),
        ('blockers', 'An unresolved blocker or pending final push prevents ACCEPTED'),
    ),
    'review': (
        ('default', 'Missing owner defaults to process, including legacy string criteria'),
        ('invalid', 'Invalid explicit owners never bypass a blocker'),
        ('independent', 'Independently evaluate review-owned criteria using current-PR and current-head evidence'),
        ('story', 'a process story passes flag is not review proof'),
        ('checks', "ownership changes the handoff gate, never review's checks"),
        ('reject-id', 'For actionable failures, return REJECTED naming the specific failing criterion ID, evidence and smallest correction'),
        ('holds', 'Pending external proof uses existing holds and later gates'),
        ('later', 'Post-merge or integrated Project validation stays with its named later gate'),
    ),
}

# Reject known contradictory gates even when the correct clause also appears.
# This is a bounded text diagnostic, not a semantic classifier or PRD evaluator.
OWNERSHIP_CONFLICTS = {
    'process': (
        ('all-criteria-gate', r'(?:all|every)(?: other)? retained (?:current-slice items|(?:story and )?acceptance criteri(?:a|on)|criteri(?:a|on))(?: in the PRD)? (?:have been marked as passes:true|(?:is |are )?pass(?:ing|es|es:true)|must (?:pass|be (?:true|satisfied)))'),
        ('all-criteria-gate', r'Every retained criterion and blocker still requires completion'),
        ('review-pass', r'(?:must|always) mark review-owned criteria (?:true|passes:true)'),
    ),
}


def check_ownership_policy(plan, process, review):
    """Diagnose owner clauses in controlled text; never compute PRD status."""
    diagnostics = []
    for role, source in (('plan', plan), ('process', process), ('review', review)):
        normalized = ' '.join(source.split())
        for name, clause in OWNERSHIP_RULES[role]:
            if clause not in normalized:
                diagnostics.append(f'{role}:owner-{name}: missing policy clause: {clause}')
        for name, pattern in OWNERSHIP_CONFLICTS.get(role, ()):
            if re.search(pattern, normalized, re.IGNORECASE):
                diagnostics.append(f'{role}:owner-{name}: conflicting ownership instruction; remove or reconcile it')
    return diagnostics


def check_policy(plan, process):
    """Return actionable diagnostics for supplied prompt strings, without I/O."""
    diagnostics = []
    for owner, source, rules in (
        ('plan', plan, PLAN_RULES), ('process', process, PROCESS_RULES)
    ):
        normalized = ' '.join(source.split())
        for name, clause in rules:
            if clause not in normalized:
                diagnostics.append(f'{owner}:{name}: missing policy clause: {clause}')
        for name, pattern in CONFLICTS:
            if re.search(pattern, normalized, re.IGNORECASE):
                diagnostics.append(f'{owner}:{name}: conflicting legacy instruction; remove or reconcile it')
    return diagnostics


def check_mailbox_policy(plan, process, lead):
    """Diagnose supplied answer/forward policy; no repository scan or runtime."""
    diagnostics = []
    for owner, text, rules in (('plan', plan, MAILBOX_RULES),
                               ('process', process, MAILBOX_RULES),
                               ('project-lead', lead, LEAD_RULES)):
        normalized = ' '.join(text.split())
        for name, clause in rules:
            if clause not in normalized:
                diagnostics.append(f'{owner}:{name}: missing policy clause: {clause}')
    return diagnostics


LOOPBACK_RULES = {
    'ideafy': (
        ('inventory-command', 'python factory/scripts/ideafy-read.py --server http://127.0.0.1:7437 session inventory'),
        ('inventory-retry', 'The optional session inventory uses an initial 10-second HTTP timeout and exactly one retry at 60 seconds.'),
        ('inventory-class', 'Every failed admitted inventory read is transient; invalid arguments and cancellation are not retried.'),
        ('inventory-gap', 'On exhaustion, record the inventory gap in feedback and supervisor working memory; never treat it as an empty inventory.'),
    ),
    'project-lead': (
        ('authority', 'Review tagged loopback proposals against immutable authority and live ownership before admission.'),
        ('dedup', 'Deduplicate proposals by stable request ID and origin Work ID across wakes and check-ins.'),
        ('resolution', 'Admit ready fixes with explicit-session dry-run, submission and verified receipt; otherwise record reason and release event in progress.md.'),
        ('controls', 'Do not use Work controls, equivalent APIs, canonical edits or operatorOverride.'),
    ),
    'project-lead-wake': (
        ('origin', 'For a thoughts loopback proposal, inspect the exact origin Work ID, retained payload, _last_output and failure Events.'),
        ('resolution', "Apply the lead's Loopback proposals admit-or-record policy in this same pass,"),
    ),
}


def check_loopback_policy(prompts):
    """Diagnose supplied handoff/resolution text; no routing or agent proof."""
    diagnostics = []
    for owner, rules in LOOPBACK_RULES.items():
        normalized = ' '.join(prompts.get(owner, '').split())
        for name, clause in rules:
            if clause not in normalized:
                diagnostics.append(f'{owner}:loopback-{name}: missing policy clause: {clause}')
    ideafy = ' '.join(prompts.get('ideafy', '').split())
    if re.search(r'Fail with the final command evidence when the helper exhausts|On a failed CLI operation or unverified admission|inventory[^.\n]*\b(?:return|returns)\s+`?FAILED', ideafy):
        diagnostics.append('ideafy:loopback-inventory-fatal: conflicting optional inventory failure policy')
    return diagnostics


MISSION_RULES = (
    ('binding', '{{range .Inputs}}{{if eq .DataType "work"}}'),
    ('payload', 'Payload: {{.Payload}}'),
    ('identity', 'Work ID: {{.WorkID}}'),
    ('tags', 'Tags: {{.Tags}}'),
    ('previous', 'Previous output: {{.PreviousOutput}}'),
    ('feedback', 'Correction feedback (verbatim checker reason): {{.RejectionFeedback}}'),
    ('field', 'On correction, fix the field named in the checker reason above.'),
    ('commands', 'Run every named command/read and report every named value, unit and source'),
    ('retry', 'Each failed read retries once; retain both attempts and available output'),
    ('optional', 'Optional exhaustion is a recorded gap, not alone FAILED'),
    ('required', 'Observed defects and command failures unrelated to prerequisites return FAILED with the exact reason and available values'),
    ('precondition-accepted', 'If the only problem is an unmet precondition, return ACCEPTED with output.precondition and available values, including partial measurements.'),
    ('correction', 'For gaps, prepare a narrow corrective batch plus its own dependent loopback'),
    ('receipt', 'For untagged Work, use a stable request ID, dry-run, idempotent submission and verified receipt'),
    ('tagged', 'Dry-run only in the bound Factory Session; submit no Project children'),
    ('ownership', 'The existing project-report route informs the owning lead, who owns admission and follow-up validation'),
    ('shape', 'output is a native JSON object containing non-empty measurements'),
    ('precondition', 'or a non-empty precondition naming the unmet requirement with available values'),
    ('marker', 'do not reset the rejection marker'),
    ('precondition-key', 'Use the exact key output.precondition with a non-blank string for an unmet precondition.'),
    ('values', 'Each measurement requires value. Zero, false and null are valid values.'),
    ('measurements-example', '{"decision":"ACCEPTED","feedback":"Measured pending Work.","output":{"measurements":[{"name":"pending","value":0,"source":"authorized Work list"}]}}'),
    ('precondition-example', '{"decision":"ACCEPTED","feedback":"Required read unavailable.","output":{"precondition":"Required recording read unavailable; pending Work observed: 0"}}'),
)


def check_mission_policy(source):
    """Check supplied verifier text; runtime obedience is a later gate."""
    normalized = ' '.join(source.split())
    diagnostics = []
    for name, clause in MISSION_RULES:
        if clause not in normalized:
            diagnostics.append(f'verify-mission:mission-{name}: missing policy clause: {clause}')
    if len(source.splitlines()) >= 60:
        diagnostics.append('verify-mission:mission-lines: prompt must be under 60 lines')
    if re.search(r'portfolio|supervisor-slot', source, re.IGNORECASE):
        diagnostics.append('verify-mission:mission-isolation: remove portfolio/supervisor instructions')
    return diagnostics


def check_mission_isolation(ideafy, verifier):
    diagnostics = check_mission_policy(verifier)
    if re.search(r'\bmission(?:s|-bearing|-less)?\b', ideafy, re.IGNORECASE):
        diagnostics.append('ideafy:mission-isolation: remove mission execution instructions')
    if len(ideafy.encode('utf-8')) >= 13_665:
        diagnostics.append('ideafy:mission-budget: prompt must shrink below 13665 UTF-8 bytes')
    return diagnostics


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root', nargs='?', type=Path, default=Path('.'))
    args = parser.parse_args(argv)
    try:
        workstation_prompts = {
            path.relative_to(args.root).as_posix(): path.read_text(encoding='utf-8')
            for path in sorted((args.root / 'factory/workstations').rglob('AGENTS.md'))
        }
        texts = [
            (args.root / 'factory' / 'workstations' / owner / 'AGENTS.md').read_text(encoding='utf-8')
            for owner in ('plan', 'process', 'project-lead')
        ]
        recovery = {
            owner: (args.root / 'factory' / 'workstations' / owner / 'AGENTS.md').read_text(encoding='utf-8')
            for owner in RECOVERY_RULES
        }
        output_prompts = {owner: recovery[owner] for owner in ('plan', 'process', 'review')}
        output_prompts['planning-standard'] = (args.root / 'factory/docs/standards/planning-standards.md').read_text(encoding='utf-8')
        verifier = (args.root / 'factory/workstations/verify-mission/AGENTS.md').read_text(encoding='utf-8')
        loopback = {
            owner: (args.root / 'factory' / 'workstations' / owner / 'AGENTS.md').read_text(encoding='utf-8')
            for owner in LOOPBACK_RULES
        }
    except (OSError, UnicodeError) as error:
        print(f'lane prompt policy: cannot read authored prompts: {error}', file=sys.stderr)
        return 1
    diagnostics = (check_changed_line_budgets(workstation_prompts)
                   + check_output_policy(output_prompts) + check_policy(*texts[:2]) + check_mailbox_policy(*texts)
                   + check_recovery_policy(recovery)
                   + check_ownership_policy(texts[0], texts[1], recovery['review'])
                   + check_loopback_policy(loopback)
                   + check_mission_isolation(loopback['ideafy'], verifier))
    if diagnostics:
        print('\n'.join(diagnostics), file=sys.stderr)
        return 1
    print('Lane prompt policy validation passed')
    return 0


if __name__ == '__main__':
    sys.exit(main())
