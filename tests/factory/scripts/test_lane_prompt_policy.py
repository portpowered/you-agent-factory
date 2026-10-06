"""Isolated diagnostics tests: controlled text, no application or prompt scan."""

import importlib.util
from pathlib import Path
import unittest


SCRIPT = Path(__file__).resolve().parents[3] / 'factory/scripts/lint-lane-prompts.py'
spec = importlib.util.spec_from_file_location('lane_prompt_policy', SCRIPT)
policy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(policy)

PLAN = '''
Plan at most ONE independently mergeable slice per lane.
Use at most 2 stories, about 8 criteria total, and a PR under about 2,000 changed lines (added plus deleted).
Keep JSON below 20 KB (20,000 UTF-8 bytes), including status updates.
For larger asks, retain the first correct slice; list remaining names/outcomes/requirements and merge gates in Markdown's "Named successor slices — not admitted".
State "None" if empty; exclude successors from userStories; only lead/operator admits them through existing routes.
Preserve immutable criteria/IDs, source-plan alignment, required sections/proof and later owning gates.
Never evade caps with compound scope or weakened acceptance; escalate indivisible scope. No runtime/routing change or invented approval.
Each named successor must depend on this lane's merge before lead/operator admission.
'''
PROCESS = '''
Keep new prd.json below 20 KB (20,000 UTF-8 bytes); update minimal status/passes/blockers only.
Preserve requirements/amendments; never falsely pass unproved/delegated criteria; escalate if minimal status cannot fit.
Add at most one short four-line progress.txt entry per visit (visit/story, changed, blocker, next), even blocked/interrupted.
Include concise patterns/browser status/deferred handoffs there; put deferred handoffs in the PR body too.
Evidence/transcripts/audits/CI references go only in PR comments; retain in session before a PR exists.
Never commit scaffolding/verification records. Grandfather oversized files: no retroactive rewrite/truncation/compaction/archive; only new entries follow these rules.
Push at most once per visit, at its end, after focused tests/lint; never during running previous-head CI, even final/blocker visits.
Retain local commits until eligible; no ACCEPTED with final push pending.
Create/ready a non-draft PR; arm gh pr merge <n> --auto --squash.
Stop with final head pushed, non-draft PR, CI started and blockers addressed; review owns terminal CI/conflicts/merge.
Never claim evidence was published when it was not.
'''


class LanePromptPolicyTest(unittest.TestCase):
    def test_compliant_text_and_wrapped_lines(self):
        self.assertEqual(policy.check_policy(PLAN, PROCESS), [])
        self.assertEqual(policy.check_policy(PLAN.replace(' ', '\n'), PROCESS.replace(' ', '\n')), [])

    def test_missing_budgets_and_authority_have_specific_diagnostics(self):
        cases = (
            ('plan', 'at most 2 stories', 'stories'),
            ('plan', 'about 8 criteria total', 'criteria'),
            ('plan', 'about 2,000 changed lines (added plus deleted)', 'lines'),
            ('plan', '20,000 UTF-8 bytes', 'bytes'),
            ('plan', 'only lead/operator admits them through existing routes', 'admission'),
            ('plan', 'Preserve immutable criteria/IDs', 'immutable'),
            ('plan', "Each named successor must depend on this lane's merge", 'merge-gate'),
            ('process', 'escalate if minimal status cannot fit', 'overflow'),
            ('process', 'Never claim evidence was published when it was not', 'comment-failure'),
            ('process', 'at most one short four-line progress.txt entry per visit', 'entry'),
            ('process', 'no retroactive rewrite/truncation/compaction/archive', 'legacy'),
            ('process', 'go only in PR comments', 'evidence'),
            ('process', 'never falsely pass unproved/delegated criteria', 'false-pass'),
            ('process', 'review owns terminal CI/conflicts/merge', 'review'),
            ('process', 'never during running previous-head CI, even final/blocker visits', 'running-ci'),
        )
        for owner, clause, diagnostic in cases:
            with self.subTest(owner=owner, diagnostic=diagnostic):
                plan = PLAN.replace(clause, '') if owner == 'plan' else PLAN
                process = PROCESS.replace(clause, '') if owner == 'process' else PROCESS
                results = policy.check_policy(plan, process)
                self.assertTrue(any(r.startswith(f'{owner}:{diagnostic}: missing policy clause:') for r in results), results)

    def test_conflicting_instructions_are_rejected_even_with_valid_clauses(self):
        cases = (
            ('APPEND to progress.txt (never replace, always append)', 'expanded-entry'),
            ('If progress.txt exceeds 500 lines, compact it first', 'compaction'),
            ('The learnings section is critical', 'extra-learning'),
            ('Do NOT push (every push cancels that run), unless (a) final', 'ci-exception'),
            ('still pushes its verified commits', 'unconditional-blocker-push'),
            ('Commit and push the unblocked work first', 'unconditional-blocker-push'),
            ('gh pr merge <n> --squash', 'manual-merge'),
            ('record that exact result in progress.txt', 'scaffold-evidence'),
        )
        for instruction, diagnostic in cases:
            with self.subTest(diagnostic=diagnostic, instruction=instruction):
                results = policy.check_policy(PLAN, PROCESS + instruction)
                self.assertIn(f'process:{diagnostic}: conflicting legacy instruction; remove or reconcile it', results)

    def test_empty_text_reports_both_owners(self):
        results = policy.check_policy('', '')
        self.assertTrue(any(r.startswith('plan:slice:') for r in results))
        self.assertTrue(any(r.startswith('process:entry:') for r in results))

    def test_recovery_diagnostics_require_each_owned_policy(self):
        prompts = {owner: '\n'.join(clause for _, clause in rules)
                   for owner, rules in policy.RECOVERY_RULES.items()}
        self.assertEqual(policy.check_recovery_policy(prompts), [])
        for owner, rules in policy.RECOVERY_RULES.items():
            for name, clause in rules:
                with self.subTest(owner=owner, rule=name):
                    changed = dict(prompts)
                    changed[owner] = changed[owner].replace(clause, '')
                    self.assertTrue(any(result.startswith(f'{owner}:recovery-{name}:')
                                        for result in policy.check_recovery_policy(changed)))

    def test_wrapped_recovery_policy_keeps_diagnosis_budget_and_binding(self):
        prompts = {owner: '\n'.join(clause for _, clause in rules).replace(' ', '\n')
                   for owner, rules in policy.RECOVERY_RULES.items()}
        self.assertEqual(policy.check_recovery_policy(prompts), [])


if __name__ == '__main__':
    unittest.main()
