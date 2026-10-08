#!/usr/bin/env python3
"""Read-only lane prompt policy lint; not runtime admission or agent proof."""

import argparse
from pathlib import Path
import re
import sys


# Required clauses are deliberately explicit: diagnostics identify the policy
# to restore. Normalize whitespace only; semantic slice quality needs review.
PLAN_RULES = (
    ('slice', 'Plan at most ONE independently mergeable slice per lane.'),
    ('stories', 'at most 2 stories'),
    ('criteria', 'about 8 unique process-owned criteria total'),
    ('lines', 'about 2,000 changed lines (added plus deleted)'),
    ('bytes', 'below 20 KB (20,000 UTF-8 bytes)'),
    ('first-slice', 'retain the first correct slice'),
    ('successors', 'remaining names/outcomes/requirements and merge gates'),
    ('successor-section', 'Named successor slices — not admitted'),
    ('empty-successors', 'State "None" if empty'),
    ('successor-stories', 'exclude successors from userStories'),
    ('admission', 'only lead/operator admits them through existing routes'),
    ('merge-gate', "Each named successor must depend on this lane's merge before lead/operator admission"),
    ('immutable', 'Preserve immutable criteria/IDs, source-plan alignment, required sections/proof and later owning gates'),
    ('indivisible', 'escalate indivisible scope'),
    ('no-evasion', 'Never evade caps with compound scope or weakened acceptance'),
    ('no-routing', 'No runtime/routing change or invented approval'),
)
PROCESS_RULES = (
    ('bytes', 'Keep new prd.json below 20 KB (20,000 UTF-8 bytes)'),
    ('minimal-status', 'update minimal status/passes/blockers only'),
    ('immutable', 'Preserve requirements/amendments'),
    ('false-pass', 'never falsely pass unproved/delegated criteria'),
    ('overflow', 'escalate if minimal status cannot fit'),
    ('entry', 'at most one short four-line progress.txt entry per visit'),
    ('interrupted', 'even blocked/interrupted'),
    ('handoffs', 'put deferred handoffs in the PR body too'),
    ('evidence', 'Evidence/transcripts/audits/CI references go only in PR comments'),
    ('pre-pr', 'retain in session before a PR exists'),
    ('no-commit', 'Never commit scaffolding/verification records'),
    ('legacy', 'Grandfather oversized files: no retroactive rewrite/truncation/compaction/archive'),
    ('prospective', 'only new entries follow these rules'),
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

RECOVERY_RULES = {
    'project-lead': (
        ('diagnosis', 'visit_cap_with_progress'),
        ('breaker', 'breaker_one_blocker'),
        ('deterministic', 'deterministic_failure'),
        ('budget', 'at most two accepted successors per original lineage'),
        ('idempotency', 'with the same request ID'),
        ('adoption', 'tags.recovery-worktree'),
        ('relative-path', 'normalized repo-relative managed path'),
        ('binding', 'Bind replacements by targetWorkId'),
        ('closure', 'evidenced failed DEPENDS_ON closure'),
        ('no-controls', 'operatorOverride repair remain forbidden even after operator answers'),
        ('size', 'about 2,000 changed lines (added plus deleted)'),
    ),
    'project-lead-wake': (
        ('recovery', "lead's Corrected successor recovery procedure"),
        ('budget', 'at most two accepted successors'),
        ('binding', 'targetWorkId'),
        ('idempotency', 'with the same request ID'),
    ),
    'project-lead-checkin': (
        ('recovery', "lead's Corrected successor recovery procedure"),
        ('budget', 'At most two accepted successors per original lineage'),
        ('binding', 'targetWorkId'),
        ('idempotency', 'with the same request ID'),
    ),
    'plan': (
        ('forward', 'forward recovery exactly to context.recovery'),
        ('tag', 'recovery-worktree tags'),
        ('relative-path', 'same normalized repo-relative'),
        ('lineage', 'originalLaneWorkId'),
        ('attempt', 'attempt 1/2 (not bool)'),
    ),
    'process': (
        ('packet', 'Select tasks/todo/'),
        ('adoption', 'context.recovery.workspace'),
        ('same-pr', 'never create a second PR'),
        ('ordinary', 'without the tag continues to use root prd.json'),
    ),
    'review': (
        ('packet', 'select tasks/todo/'),
        ('adoption', 'context.recovery.workspace'),
        ('same-pr', 'never create a second PR'),
        ('ordinary', 'without this tag keeps root prd.json'),
    ),
}


def check_recovery_policy(prompts):
    """Diagnose controlled recovery text; runtime/lead judgment is separate."""
    diagnostics = []
    for owner, rules in RECOVERY_RULES.items():
        normalized = ' '.join(prompts.get(owner, '').split())
        for name, clause in rules:
            if clause not in normalized:
                diagnostics.append(f'{owner}:recovery-{name}: missing policy clause: {clause}')
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
        ('cap', 'Count only process-owned criteria toward the criterion cap, once by criterion ID'),
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
    'plan': (
        ('all-criteria-cap', r'about 8 criteria total'),
    ),
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
    ('commands', 'Run every named command/read and report every named value, unit and source'),
    ('retry', 'Each failed read retries once; retain both attempts and available output'),
    ('optional', 'Optional exhaustion is a recorded gap, not alone FAILED'),
    ('required', 'Required read/command failures return FAILED with the exact reason and available values'),
    ('correction', 'For gaps, prepare a narrow corrective batch plus its own dependent loopback'),
    ('receipt', 'For untagged Work, use a stable request ID, dry-run, idempotent submission and verified receipt'),
    ('tagged', 'Dry-run only in the bound Factory Session; submit no Project children'),
    ('ownership', 'The existing project-report route informs the owning lead, who owns admission and follow-up validation'),
    ('shape', 'output is a native JSON object containing non-empty measurements'),
    ('precondition', 'or a non-empty precondition naming the unmet requirement with available values'),
    ('marker', 'do not reset the rejection marker'),
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
        texts = [
            (args.root / 'factory' / 'workstations' / owner / 'AGENTS.md').read_text(encoding='utf-8')
            for owner in ('plan', 'process', 'project-lead')
        ]
        recovery = {
            owner: (args.root / 'factory' / 'workstations' / owner / 'AGENTS.md').read_text(encoding='utf-8')
            for owner in RECOVERY_RULES
        }
        verifier = (args.root / 'factory/workstations/verify-mission/AGENTS.md').read_text(encoding='utf-8')
        loopback = {
            owner: (args.root / 'factory' / 'workstations' / owner / 'AGENTS.md').read_text(encoding='utf-8')
            for owner in LOOPBACK_RULES
        }
    except (OSError, UnicodeError) as error:
        print(f'lane prompt policy: cannot read authored prompts: {error}', file=sys.stderr)
        return 1
    diagnostics = (check_policy(*texts[:2]) + check_mailbox_policy(*texts)
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
