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
    ('criteria', 'about 8 criteria total'),
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
    ('push-count', 'Push at most once per visit, at its end, after focused tests/lint'),
    ('running-ci', 'never during running previous-head CI, even final/blocker visits'),
    ('pending-push', 'Retain local commits until eligible; no ACCEPTED with final push pending'),
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
    ('ci-exception', r'\(every push cancels that run\), unless|unless \(a\)|push (?:even )?while.*CI.*running'),
    ('unconditional-blocker-push', r'Commit and push the unblocked work first|still pushes its verified commits'),
    ('manual-merge', r'gh pr merge <n> --squash'),
    ('scaffold-evidence', r'record (?:that )?exact result in progress\.txt'),
)


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


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root', nargs='?', type=Path, default=Path('.'))
    args = parser.parse_args(argv)
    try:
        texts = [
            (args.root / 'factory' / 'workstations' / owner / 'AGENTS.md').read_text(encoding='utf-8')
            for owner in ('plan', 'process')
        ]
    except (OSError, UnicodeError) as error:
        print(f'lane prompt policy: cannot read authored prompts: {error}', file=sys.stderr)
        return 1
    diagnostics = check_policy(*texts)
    if diagnostics:
        print('\n'.join(diagnostics), file=sys.stderr)
        return 1
    print('Lane prompt policy validation passed')
    return 0


if __name__ == '__main__':
    sys.exit(main())
