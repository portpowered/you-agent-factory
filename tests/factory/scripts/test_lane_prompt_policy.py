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
Use at most 2 stories, about 8 unique process-owned criteria total, and a PR under about 2,000 changed lines (added plus deleted).
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

MAILBOX = '''
For a lane carrying a project tag, address the request to its project lead first.
A lane without a project tag keeps the current operator route.
Include one explicit decision, source/acceptance/rules citations and evidence,
A recommended with evidence and tradeoffs, Project tag (or none), Factory Session,
Work ID and addressed decision owner, and no unauthorized widening or contract change.
'''
LEAD = '''
Answer inside the immutable source plan, acceptance and rules.md pre-authorizations,
including narrowing changes. Write responses/<lane>.md and this Project's progress.md.
Forward only widening public exposure; adding an owner or a second path;
growing a lint or boundary baseline; contradicting the immutable plan or acceptance;
or factory/tooling defects. Write a separate operator request naming the
original lane request and its version; do not place a speculative answer.
When binding operator authority arrives, preserve its authority and deliver the lane's binding response.
Use recorded request identities to avoid duplicate forwarding;
reconcile outstanding forwarded requests and operator responses on every visit.
Recheck the live Session, lane Work ID, project tag and request mtime_ns; rename atomically.
'''

# Explicit controlled fixtures: tests do not read authored prompt files or
# synthesize a process decision engine from criterion dictionaries.
OWNER_PLAN = '''
Every project and story criterion MUST carry an explicit `owner`: `process` or
`review` in new planner output. Hosted/terminal CI results, merge, independent
or post-merge validation, and reviewer judgment are review-owned.
Direct implementation proof and the implementation-stage delivery criterion are process-owned.
A composite immutable criterion requiring review evidence is review-owned as a whole;
retain its implementation work and later gates.
Missing owner defaults to process, including legacy string criteria.
Invalid explicit owners (unknown, null or non-string) are malformed metadata and never bypass a blocker.
Count only process-owned criteria toward the criterion cap, once by criterion ID.
'''
OWNER_PROCESS = '''
Set `decision` to `ACCEPTED` only when every retained process-owned criterion is passes:true.
Missing owner defaults to process, including legacy string criteria.
Invalid explicit owners never bypass a blocker.
A false process-owned criterion prevents ACCEPTED.
Process never marks review-owned criteria true.
Unproved review-owned criteria remain false and do not block process ACCEPTED.
List their IDs and later gate IDs in envelope feedback and the PR handoff as "owned by review".
Story passes reports retained process completion only, never review proof.
Empty or all-review criterion sets still require complete retained implementation,
a pushed final head, an open non-draft PR, CI started, auto-merge armed and blocking feedback addressed.
An unresolved blocker or pending final push prevents ACCEPTED.
'''
OWNER_REVIEW = '''
Missing owner defaults to process, including legacy string criteria.
Invalid explicit owners never bypass a blocker.
Independently evaluate review-owned criteria using current-PR and current-head evidence;
a process story passes flag is not review proof.
Recheck process claims: ownership changes the handoff gate, never review's checks.
For actionable failures, return REJECTED naming the specific failing criterion ID,
evidence and smallest correction.
Pending external proof uses existing holds and later gates.
Post-merge or integrated Project validation stays with its named later gate.
'''


class LanePromptPolicyTest(unittest.TestCase):
    def test_loopback_handoff_resolution_and_wrapping(self):
        prompts = {owner: '\n'.join(clause for _, clause in rules)
                   for owner, rules in policy.LOOPBACK_RULES.items()}
        self.assertEqual(policy.check_loopback_policy(prompts), [])
        wrapped = {owner: text.replace(' ', '\n') for owner, text in prompts.items()}
        self.assertEqual(policy.check_loopback_policy(wrapped), [])
        for owner, rules in policy.LOOPBACK_RULES.items():
            for name, clause in rules:
                with self.subTest(owner=owner, rule=name):
                    changed = dict(prompts)
                    changed[owner] = changed[owner].replace(clause, '')
                    self.assertEqual(policy.check_loopback_policy(changed),
                                     [f'{owner}:loopback-{name}: missing policy clause: {clause}'])

    def test_p1_p3_complete_ownership_and_false_review_handoff_clauses(self):
        self.assertEqual(policy.check_ownership_policy(OWNER_PLAN, OWNER_PROCESS, OWNER_REVIEW), [])

    def test_p2_each_planner_ownership_clause_has_specific_diagnostic(self):
        source = ' '.join(OWNER_PLAN.split())
        for name, clause in policy.OWNERSHIP_RULES['plan']:
            with self.subTest(name=name):
                self.assertIn(clause, source)
                results = policy.check_ownership_policy(source.replace(clause, ''), OWNER_PROCESS, OWNER_REVIEW)
                self.assertEqual(len(results), 1, results)
                self.assertTrue(results[0].startswith(f'plan:owner-{name}: missing policy clause:'), results)

    def test_p4_each_process_gate_clause_has_specific_diagnostic(self):
        source = ' '.join(OWNER_PROCESS.split())
        for name, clause in policy.OWNERSHIP_RULES['process']:
            with self.subTest(name=name):
                self.assertIn(clause, source)
                results = policy.check_ownership_policy(OWNER_PLAN, source.replace(clause, ''), OWNER_REVIEW)
                self.assertEqual(len(results), 1, results)
                self.assertTrue(results[0].startswith(f'process:owner-{name}: missing policy clause:'), results)

    def test_p4_universal_gates_conflict_even_beside_correct_gate(self):
        cases = (
            ('all retained current-slice items in the PRD have been marked as passes:true', 'all-criteria-gate'),
            ('Every retained criterion and blocker still requires completion.', 'all-criteria-gate'),
            ('every retained story and acceptance criterion is passing', 'all-criteria-gate'),
            ('all retained criteria must pass', 'all-criteria-gate'),
            ('every other retained story and acceptance criterion is passing', 'all-criteria-gate'),
            ('You must mark review-owned criteria true.', 'review-pass'),
        )
        for clause, name in cases:
            with self.subTest(clause=clause):
                results = policy.check_ownership_policy(OWNER_PLAN, OWNER_PROCESS + clause, OWNER_REVIEW)
                self.assertEqual(results, [f'process:owner-{name}: conflicting ownership instruction; remove or reconcile it'])

    def test_p5_each_independent_review_clause_has_specific_diagnostic(self):
        source = ' '.join(OWNER_REVIEW.split())
        for name, clause in policy.OWNERSHIP_RULES['review']:
            with self.subTest(name=name):
                self.assertIn(clause, source)
                results = policy.check_ownership_policy(OWNER_PLAN, OWNER_PROCESS, source.replace(clause, ''))
                self.assertEqual(len(results), 1, results)
                self.assertTrue(results[0].startswith(f'review:owner-{name}: missing policy clause:'), results)

    def test_p6_legacy_and_empty_sets_do_not_exempt_delivery(self):
        for clause, diagnostic in (
            ('Missing owner defaults to process, including legacy string criteria', 'default'),
            ('a pushed final head', 'empty'),
            ('auto-merge armed', 'empty'),
            ('blocking feedback addressed', 'empty'),
        ):
            with self.subTest(clause=clause):
                source = ' '.join(OWNER_PROCESS.split()).replace(clause, '')
                results = policy.check_ownership_policy(OWNER_PLAN, source, OWNER_REVIEW)
                self.assertEqual(len(results), 1, results)
                self.assertTrue(results[0].startswith(f'process:owner-{diagnostic}:'), results)

    def test_p7_wrapped_ownership_clauses_are_accepted(self):
        self.assertEqual(policy.check_ownership_policy(
            OWNER_PLAN.replace(' ', '\n'), OWNER_PROCESS.replace(' ', '\n'),
            OWNER_REVIEW.replace(' ', '\n')), [])

    def test_p8_process_cap_rejects_legacy_universal_cap(self):
        results = policy.check_ownership_policy(OWNER_PLAN + 'about 8 criteria total', OWNER_PROCESS, OWNER_REVIEW)
        self.assertEqual(results, ['plan:owner-all-criteria-cap: conflicting ownership instruction; remove or reconcile it'])

    def test_mailbox_answer_forward_and_wrapped_policy(self):
        self.assertEqual(policy.check_mailbox_policy(MAILBOX, MAILBOX, LEAD), [])
        self.assertEqual(policy.check_mailbox_policy(MAILBOX.replace(' ', '\n'), MAILBOX, LEAD), [])

    def test_each_forwarding_category_and_answer_obligation_is_diagnosed(self):
        for diagnostic, clause in policy.LEAD_RULES:
            with self.subTest(diagnostic=diagnostic):
                results = policy.check_mailbox_policy(MAILBOX, MAILBOX, ' '.join(LEAD.split()).replace(clause, ''))
                self.assertTrue(any(r.startswith(f'project-lead:{diagnostic}:') for r in results), results)

    def test_request_decision_evidence_recommendation_and_identity_are_diagnosed(self):
        for owner in ('plan', 'process'):
            for diagnostic, clause in policy.MAILBOX_RULES:
                with self.subTest(owner=owner, diagnostic=diagnostic):
                    source = ' '.join(MAILBOX.split()).replace(clause, '')
                    results = policy.check_mailbox_policy(source if owner == 'plan' else MAILBOX,
                                                          source if owner == 'process' else MAILBOX, LEAD)
                    self.assertTrue(any(r.startswith(f'{owner}:{diagnostic}:') for r in results), results)

    def test_compliant_text_and_wrapped_lines(self):
        self.assertEqual(policy.check_policy(PLAN, PROCESS), [])
        self.assertEqual(policy.check_policy(PLAN.replace(' ', '\n'), PROCESS.replace(' ', '\n')), [])

    def test_missing_budgets_and_authority_have_specific_diagnostics(self):
        cases = (
            ('plan', 'at most 2 stories', 'stories'),
            ('plan', 'about 8 unique process-owned criteria total', 'criteria'),
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
