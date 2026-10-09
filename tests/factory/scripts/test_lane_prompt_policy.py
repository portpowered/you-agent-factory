"""Checker units, controlled lint regressions, and authored mission-policy contracts."""

from contextlib import redirect_stderr, redirect_stdout
import importlib.util
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


SCRIPT = Path(__file__).resolve().parents[3] / 'factory/scripts/lint-lane-prompts.py'
spec = importlib.util.spec_from_file_location('lane_prompt_policy', SCRIPT)
policy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(policy)

PLAN = '''
Plan each lane as ONE independently mergeable PR that delivers the whole ask.
There is no story, criterion, line or file cap; a large ask is one large plan.
Split only when parts ship separately or one part must merge before another can be built; then list remaining names/outcomes/requirements and merge gates in Markdown's "Named successor slices — not admitted".
State "None" if empty; exclude successors from userStories; only lead/operator admits them through existing routes.
Preserve immutable criteria/IDs, source-plan alignment, required sections/proof and later owning gates.
Never weaken acceptance. No runtime/routing change or invented approval.
Each named successor must depend on this lane's merge before lead/operator admission.
'''
PROCESS = '''
Update minimal status/passes/blockers only.
Preserve requirements/amendments; never falsely pass unproved/delegated criteria.
Keep concise visit/story, changed, blocker and next records, even blocked/interrupted.
Include concise patterns/browser status/deferred handoffs there; put deferred handoffs in the PR body too.
Retain observations in session before a PR exists.
Never commit scaffolding/verification records. Preserve existing artifacts without retroactive rewrite/truncation/compaction/archive.
Push at most once per visit, at its end, after focused tests/lint, when the visit changed code.
If previous-head CI is still running, push anyway; the superseding push cancels it.
Never spend a visit only waiting for CI or return CONTINUE solely because CI is running.
No ACCEPTED with final push pending.
A lane that must stop on a contract conflict commits and pushes verified unblocked changes at visit end
 after focused tests/lint, within the one-push limit, and opens a non-draft PR naming the blocker;
 never claim an unresolved current-slice blocker is ready for acceptance.
Commit verified unblocked changes and push at visit end after focused tests/lint,
 within the one-push limit, then end the visit with CONTINUE.
Do not spend process visits watching CI.
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


MISSION = """
{{range .Inputs}}{{if eq .DataType "work"}}
Payload: {{.Payload}}
Work ID: {{.WorkID}}
Tags: {{.Tags}}
Previous output: {{.PreviousOutput}}
Correction feedback (verbatim checker reason): {{.RejectionFeedback}}
{{end}}{{end}}
On correction, fix the field named in the checker reason above.
Run every named command/read and report every named value, unit and source.
Each failed read retries once; retain both attempts and available output.
Optional exhaustion is a recorded gap, not alone FAILED.
Observed defects and command failures unrelated to prerequisites return FAILED with the exact reason and available values.
If the only problem is an unmet precondition, return ACCEPTED with output.precondition and available values, including partial measurements.
For gaps, prepare a narrow corrective batch plus its own dependent loopback.
For untagged Work, use a stable request ID, dry-run, idempotent submission and verified receipt.
Dry-run only in the bound Factory Session; submit no Project children.
The existing project-report route informs the owning lead, who owns admission and follow-up validation.
output is a native JSON object containing non-empty measurements
or a non-empty precondition naming the unmet requirement with available values.
do not reset the rejection marker.
Use the exact key output.precondition with a non-blank string for an unmet precondition.
Each measurement requires value. Zero, false and null are valid values.
{"decision":"ACCEPTED","feedback":"Measured pending Work.","output":{"measurements":[{"name":"pending","value":0,"source":"authorized Work list"}]}}
{"decision":"ACCEPTED","feedback":"Required read unavailable.","output":{"precondition":"Required recording read unavailable; pending Work observed: 0"}}
"""


RECOVERY = {
    'project-lead': '''visit_cap_with_progress breaker_one_blocker deterministic_failure
positive integer with no ceiling; reconcile with the same request ID.
tags.recovery-worktree uses a normalized repo-relative managed path.
Bind replacements by targetWorkId within the evidenced failed DEPENDS_ON closure.
operatorOverride repair remain forbidden even after operator answers.''',
    'project-lead-wake': '''Apply the lead's Corrected successor recovery procedure.
positive integer with no ceiling; targetWorkId; reconcile with the same request ID.''',
    'project-lead-checkin': '''Apply the lead's Corrected successor recovery procedure.
positive integer with no ceiling; targetWorkId; reconcile with the same request ID.''',
    'plan': '''forward recovery exactly to context.recovery.
Preserve recovery-worktree tags and the same normalized repo-relative path.
originalLaneWorkId; positive integer (not bool), with no ceiling.''',
    'process': '''Select tasks/todo/ from context.recovery.workspace; never create a second PR.
Ordinary input without the tag continues to use root prd.json.
Preserve positive integer attempt, with no ceiling.''',
    'review': '''select tasks/todo/ from context.recovery.workspace; never create a second PR.
Ordinary input without this tag keeps root prd.json.
Preserve positive integer attempt, with no ceiling.''',
}


OUTPUT_POLICY = """
Do not prescribe arbitrary output-size requirements unless the customer explicitly asks for them.
Measurements, timings, calibration runs and evidence belong in the PR body or a PR comment; CI evidence belongs only in PR comments.
Committed tests protect customer behavior and ship with the change.
Do not commit large one-off fixtures, calibration harnesses, evidence documents or proof files.
Each observable process outcome names one measurement or test and can close in one visit once the behavior and witness exist.
Do not invent gates that demand repeated or escalating proof; preserve independent review, CI and merge obligations.
"""


class LanePromptPolicyTests(unittest.TestCase):
    def test_changed_line_checker_accepts_retained_limits_and_explanations(self):
        source = '''For recovery, retain the independently mergeable PR slice with JSON below
20 KB (20,000 UTF-8 bytes) with status headroom.
Do not impose a changed-line budget. Changed-line filtering explains the diff.
The physical prompt must be under 60 lines. Bound paid calls to 5 and time to 60 seconds.
'''
        self.assertEqual(policy.check_changed_line_budgets({'lead/AGENTS.md': source}), [])

    def test_changed_line_checker_rejects_original_and_wrapped_mixed_case(self):
        source = 'about 2,000 changed lines (added plus deleted), and JSON below 20 KB.'
        path = 'factory/workstations/project-lead/AGENTS.md'
        for text in (source, source.upper().replace(' ', '\n\t')):
            with self.subTest(text=text):
                self.assertEqual(policy.check_changed_line_budgets({path: text}), [
                    f'{path}:changed-line-budget: remove changed-line budgets from lane-facing prompts'])

    def test_scope_checker_rejects_story_and_criterion_caps(self):
        path = 'factory/workstations/plan/AGENTS.md'
        for source in ('Use at most 2 stories, about 8 unique process-owned criteria total.',
                       'retain the slice with at most 2 stories', 'about 8 criteria total'):
            for text in (source, source.upper().replace(' ', '\n')):
                with self.subTest(text=text):
                    self.assertEqual(policy.check_changed_line_budgets({path: text}), [
                        f'{path}:scope-cap: remove story and criterion caps from lane-facing prompts'])

    def test_changed_line_checker_rejects_representative_diff_budgets(self):
        for source in ('PR under about 2,000 changed lines', 'changed-line budget of 1500',
                       'diff limit: 2000 lines', 'at most 1,500 lines (added plus deleted)'):
            for text in (source, source.upper().replace(' ', '\n')):
                with self.subTest(text=text):
                    self.assertEqual(policy.check_changed_line_budgets({'other/AGENTS.md': text}), [
                        'other/AGENTS.md:changed-line-budget: remove changed-line budgets from lane-facing prompts'])

    def test_output_policy_accepts_compliant_and_wrapped_roles(self):
        for text in (OUTPUT_POLICY, OUTPUT_POLICY.replace(' ', '\n')):
            prompts = dict.fromkeys(('plan', 'process', 'review', 'planning-standard'), text)
            self.assertEqual(policy.check_output_policy(prompts), [])

    def test_missing_output_policy_names_role_and_rule(self):
        prompts = dict.fromkeys(('plan', 'process', 'review', 'planning-standard'), OUTPUT_POLICY)
        for owner in prompts:
            for name, clause in policy.OUTPUT_RULES:
                with self.subTest(owner=owner, rule=name):
                    changed = dict(prompts)
                    changed[owner] = changed[owner].replace(clause, '')
                    self.assertEqual(policy.check_output_policy(changed), [
                        f'{owner}:{name}: missing policy clause: {clause}'])

    def test_output_budgets_are_rejected_beside_correct_policy(self):
        prompts = dict.fromkeys(('plan', 'process', 'review', 'planning-standard'), OUTPUT_POLICY)
        cases = (
            'Use a PR under about 2,000 changed lines (added plus deleted).',
            'Keep JSON below 20 KB (20,000 UTF-8 bytes).',
            'Use at most 12 files.',
            'Add a maximum of 40 tests.',
            'The diff target is 1500 lines.',
            'Set a file-count budget of 20.',
            'Set a test-count cap: 30.',
            'Keep the packet budget 20000 bytes.',
            'Keep output below 300 lines.',
            'Escalate indivisible scope.',
            'Add at most one short four-line progress.txt entry per visit.',
        )
        for owner in prompts:
            for instruction in cases:
                for text in (instruction, instruction.replace(' ', '\n')):
                    with self.subTest(owner=owner, instruction=text):
                        changed = dict(prompts)
                        changed[owner] += text
                        self.assertEqual(policy.check_output_policy(changed), [
                            f'{owner}:output-size: conflicting output policy; remove arbitrary output budgets or committed proof instructions'])

    def test_committed_proof_requests_are_rejected(self):
        prompts = dict.fromkeys(('plan', 'process', 'review', 'planning-standard'), OUTPUT_POLICY)
        for owner in prompts:
            for instruction in ('You must commit proof.',
                                'Always commit a calibration harness.',
                                'Add an evidence document to the PR.'):
                with self.subTest(owner=owner, instruction=instruction):
                    changed = dict(prompts)
                    changed[owner] += instruction
                    self.assertEqual(policy.check_output_policy(changed), [
                        f'{owner}:committed-proof: conflicting output policy; remove arbitrary output budgets or committed proof instructions'])

    def test_runtime_resource_limits_are_preserved(self):
        prompts = dict.fromkeys(('plan', 'process', 'review', 'planning-standard'),
                               OUTPUT_POLICY +                                'Bound paid validation to 5 calls, 60 seconds and 10 dollars. '
                               'The customer explicitly requests output-size requirements. '
                               'Preserve the customer requirement verbatim in its criterion.')
        self.assertEqual(policy.check_output_policy(prompts), [])

    def test_mission_compliant_and_wrapped_policy(self):
        self.assertEqual(policy.check_mission_policy(MISSION), [])
        self.assertEqual(policy.check_mission_policy(MISSION.replace(' ', '\n')), [
            'verify-mission:mission-lines: prompt must be under 60 lines'])

    def test_each_missing_mission_clause_is_diagnosed(self):
        for name, clause in policy.MISSION_RULES:
            with self.subTest(name=name):
                self.assertEqual(policy.check_mission_policy(MISSION.replace(clause, '')),
                                 [f'verify-mission:mission-{name}: missing policy clause: {clause}'])

    def test_dedicated_prompt_isolation_and_budgets(self):
        self.assertEqual(policy.check_mission_isolation('Portfolio supervision', MISSION), [])
        for extra in ('Run the portfolio routine.', 'Acquire supervisor-slot.'):
            self.assertIn('verify-mission:mission-isolation: remove portfolio/supervisor instructions',
                          policy.check_mission_policy(MISSION + extra))
        self.assertIn('ideafy:mission-isolation: remove mission execution instructions',
                      policy.check_mission_isolation('Execute the mission', MISSION))
        self.assertIn('ideafy:mission-budget: prompt must shrink below 13665 UTF-8 bytes',
                      policy.check_mission_isolation('x' * 13665, MISSION))
        self.assertIn('verify-mission:mission-lines: prompt must be under 60 lines',
                      policy.check_mission_policy(MISSION + '\n' * 60))

    def test_empty_mission_policy_reports_required_rules(self):
        self.assertEqual(len(policy.check_mission_policy('')), len(policy.MISSION_RULES))

    def test_optional_inventory_fatal_instructions_conflict_with_mission(self):
        prompts = {owner: '\n'.join(clause for _, clause in rules)
                   for owner, rules in policy.LOOPBACK_RULES.items()}
        for instruction in ('Fail with the final command evidence when the helper exhausts its budget.',
                            'On a failed CLI operation or unverified admission, return FAILED.',
                            'If inventory fails return FAILED.'):
            changed = dict(prompts)
            changed['ideafy'] += '\n' + instruction
            self.assertEqual(policy.check_loopback_policy(changed),
                             ['ideafy:loopback-inventory-fatal: conflicting optional inventory failure policy'])

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

    def test_push_p1_complete_and_wrapped_policy(self):
        self.assertEqual(policy.check_policy(PLAN, PROCESS), [])
        self.assertEqual(policy.check_policy(PLAN, PROCESS.replace(' ', '\n')), [])

    def test_push_p2_each_working_visit_clause_is_required(self):
        cases = (
            ('at most once per visit', 'push-count'),
            ('at its end', 'push-count'),
            ('after focused tests/lint', 'push-count'),
            ('when the visit changed code', 'push-count'),
            ('If previous-head CI is still running, push anyway', 'running-ci'),
            ('the superseding push cancels it', 'running-ci'),
            ('Never spend a visit only waiting for CI', 'no-ci-wait'),
            ('or return CONTINUE solely because CI is running', 'no-ci-wait'),
            ('No ACCEPTED with final push pending', 'pending-push'),
            ('Do not spend process visits watching CI', 'no-watcher'),
        )
        for clause, diagnostic in cases:
            with self.subTest(clause=clause):
                results = policy.check_policy(PLAN, PROCESS.replace(clause, '', 1))
                self.assertEqual(results, [
                    f'process:{diagnostic}: missing policy clause: '
                    + dict(policy.PROCESS_RULES)[diagnostic]])

    def test_push_p3_old_gate_is_rejected_beside_new_policy(self):
        for instruction in (
            'never during running previous-head CI, even final/blocker visits.',
            'Never push during running previous-head CI.',
            'Do not push while previous-head CI is running.',
        ):
            for text in (instruction, instruction.replace(' ', '\n')):
                with self.subTest(instruction=text):
                    self.assertEqual(policy.check_policy(PLAN, PROCESS + text), [
                        'process:previous-ci-gate: conflicting legacy instruction; remove or reconcile it'])

    def test_push_p4_ci_only_continue_is_rejected(self):
        for instruction in (
            'If previous-head CI is running, retain local commits and return CONTINUE with the push pending.',
            'If previous-head CI is still running, retain local commits and return CONTINUE.',
            'Return CONTINUE solely because CI is running.',
        ):
            with self.subTest(instruction=instruction):
                self.assertEqual(policy.check_policy(PLAN, PROCESS + instruction.replace(' ', '\n')), [
                    'process:ci-only-continue: conflicting legacy instruction; remove or reconcile it'])

    def test_push_p5_blocked_visits_keep_verification_end_and_count(self):
        normalized = ' '.join(PROCESS.split())
        for diagnostic in ('blocker-push', 'mailbox-push'):
            clause = dict(policy.PROCESS_RULES)[diagnostic]
            for obligation in ('verified unblocked changes', 'at visit end',
                               'after focused tests/lint', 'within the one-push limit'):
                with self.subTest(diagnostic=diagnostic, obligation=obligation):
                    source = normalized.replace(clause, clause.replace(obligation, ''))
                    self.assertEqual(policy.check_policy(PLAN, source), [
                        f'process:{diagnostic}: missing policy clause: {clause}'])

    def test_push_p6_unrelated_guards_and_watcher_diagnostic_remain(self):
        for diagnostic in ('non-draft', 'auto-merge', 'handoff', 'review',
                           'false-pass', 'no-commit', 'comment-failure'):
            clause = dict(policy.PROCESS_RULES)[diagnostic]
            with self.subTest(diagnostic=diagnostic):
                self.assertEqual(policy.check_policy(PLAN, PROCESS.replace(clause, '')), [
                    f'process:{diagnostic}: missing policy clause: {clause}'])
        self.assertEqual(policy.check_policy(
            PLAN, PROCESS + 'CI watching: at most ONE bounded watcher per head.'), [
                'process:ci-watcher: conflicting legacy instruction; remove or reconcile it'])

    def test_compliant_text_and_wrapped_lines(self):
        self.assertEqual(policy.check_policy(PLAN, PROCESS), [])
        self.assertEqual(policy.check_policy(PLAN.replace(' ', '\n'), PROCESS.replace(' ', '\n')), [])

    def test_missing_slice_and_authority_rules_have_specific_diagnostics(self):
        cases = (
            ('plan', 'There is no story, criterion, line or file cap', 'no-scope-cap'),
            ('plan', 'Split only when parts ship separately', 'split'),
            ('plan', 'only lead/operator admits them through existing routes', 'admission'),
            ('plan', 'Preserve immutable criteria/IDs', 'immutable'),
            ('plan', "Each named successor must depend on this lane's merge", 'merge-gate'),
            ('process', 'Never claim evidence was published when it was not', 'comment-failure'),
            ('process', 'Keep concise visit/story, changed, blocker and next records', 'entry'),
            ('process', 'without retroactive rewrite/truncation/compaction/archive', 'legacy'),
            ('process', 'never falsely pass unproved/delegated criteria', 'false-pass'),
            ('process', 'review owns terminal CI/conflicts/merge', 'review'),
            ('process', 'the superseding push cancels it', 'running-ci'),
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
        prompts = dict(RECOVERY)
        self.assertEqual(policy.check_recovery_policy(prompts), [])
        for owner, rules in policy.RECOVERY_RULES.items():
            for name, clause in rules:
                with self.subTest(owner=owner, rule=name):
                    changed = dict(prompts)
                    changed[owner] = changed[owner].replace(clause, '')
                    self.assertTrue(any(result.startswith(f'{owner}:recovery-{name}:')
                                        for result in policy.check_recovery_policy(changed)))

    def test_wrapped_recovery_policy_keeps_positive_attempt_diagnosis_and_binding(self):
        prompts = {owner: text.replace(' ', '\n') for owner, text in RECOVERY.items()}
        self.assertEqual(policy.check_recovery_policy(prompts), [])

    def test_recovery_ceiling_conflicts_even_beside_valid_policy(self):
        instructions = (
            'at most two accepted successors per original lineage',
            'Only 2 successors are allowed.',
            'Apply the two-successor lineage budget.',
            'Validate attempt 1/2 (not bool).',
            'Set attempt to integer 1 or 2.',
            'attempt must be integer 1 or 2.',
            'This is the last-successor rule.',
            'Admit the final successor.',
            'Exhausted lineage takes a hold.',
            "Do not reset the original lineage's attempt budget.",
            'Never assume unused budget.',
        )
        for owner in RECOVERY:
            for instruction in instructions:
                for text in (instruction, instruction.replace(' ', '\n')):
                    with self.subTest(owner=owner, instruction=text):
                        prompts = dict(RECOVERY)
                        prompts[owner] += '\n' + text
                        self.assertEqual(policy.check_recovery_policy(prompts),
                                         [f'{owner}:recovery-ceiling: conflicting recovery instruction; remove or reconcile it'])


class LanePromptLintTests(unittest.TestCase):
    """Observe lint status/streams and read-only behavior on isolated authored files."""

    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        prompts = dict(RECOVERY)
        prompts['plan'] += PLAN + MAILBOX + OWNER_PLAN + OUTPUT_POLICY
        prompts['process'] += PROCESS + MAILBOX + OWNER_PROCESS + OUTPUT_POLICY
        prompts['review'] += OWNER_REVIEW + OUTPUT_POLICY
        prompts['project-lead'] += LEAD
        for owner, rules in policy.LOOPBACK_RULES.items():
            prompts[owner] = prompts.get(owner, '') + '\n' + '\n'.join(clause for _, clause in rules)
        prompts['verify-mission'] = MISSION
        for owner, text in prompts.items():
            self.write(f'factory/workstations/{owner}/AGENTS.md', text)
        self.write('factory/docs/standards/planning-standards.md', OUTPUT_POLICY)

    def write(self, path, text):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text, encoding='utf-8')
        return target

    def run_lint(self):
        before = {path: path.read_bytes() for path in self.root.rglob('*') if path.is_file()}
        stdout, stderr = io.StringIO(), io.StringIO()
        with redirect_stdout(stdout), redirect_stderr(stderr):
            status = policy.main([str(self.root)])
        self.assertEqual({path: path.read_bytes() for path in self.root.rglob('*') if path.is_file()}, before)
        return status, stdout.getvalue(), stderr.getvalue()

    def test_clean_root_preserves_success_message(self):
        self.assertEqual(self.run_lint(), (0, 'Lane prompt policy validation passed\n', ''))

    def test_new_nested_role_and_lead_produce_ordered_path_diagnostics(self):
        nested = 'factory/workstations/new-role/nested/AGENTS.md'
        self.write(nested, 'diff limit: 2000 lines')
        self.assertEqual(self.run_lint(), (1, '',
            f'{nested}:changed-line-budget: remove changed-line budgets from lane-facing prompts\n'))
        lead = 'factory/workstations/project-lead/AGENTS.md'
        target = self.root / lead
        target.write_text(target.read_text(encoding='utf-8') + '\nabout 2,000 changed lines (added plus deleted)', encoding='utf-8')
        self.assertEqual(self.run_lint(), (1, '', ''.join(
            f'{path}:changed-line-budget: remove changed-line budgets from lane-facing prompts\n'
            for path in (nested, lead))))

    def test_missing_required_prompt_fails_with_read_error(self):
        (self.root / 'factory/workstations/process/AGENTS.md').unlink()
        status, stdout, stderr = self.run_lint()
        self.assertEqual((status, stdout), (1, ''))
        self.assertTrue(stderr.startswith('lane prompt policy: cannot read authored prompts:'), stderr)
        self.assertIn('AGENTS.md', stderr)

    def test_invalid_utf8_discovered_prompt_fails_with_read_error(self):
        target = self.write('factory/workstations/new-role/nested/AGENTS.md', '')
        target.write_bytes(b'\xff')
        status, stdout, stderr = self.run_lint()
        self.assertEqual((status, stdout), (1, ''))
        self.assertTrue(stderr.startswith('lane prompt policy: cannot read authored prompts:'), stderr)
        self.assertIn('decode', stderr)

    def test_discovered_prompt_oserror_is_not_silently_skipped(self):
        target = self.write('factory/workstations/new-role/nested/AGENTS.md', 'valid prompt')
        read_text = Path.read_text

        def controlled_read(path, *args, **kwargs):
            if path == target:
                raise OSError('controlled discovered prompt read failure')
            return read_text(path, *args, **kwargs)

        with patch.object(Path, 'read_text', controlled_read):
            self.assertEqual(self.run_lint(), (1, '',
                'lane prompt policy: cannot read authored prompts: controlled discovered prompt read failure\n'))


class AuthoredMissionPolicyTests(unittest.TestCase):
    """The mission instructions themselves are the published policy contract."""

    @classmethod
    def setUpClass(cls):
        prompt = SCRIPT.parent.parent / 'workstations/verify-mission/AGENTS.md'
        cls.mission = ' '.join(prompt.read_text(encoding='utf-8').split())

    def test_read_only_preconditions_are_resolved_before_deciding(self):
        clauses = (
            'Before deciding, resolve read-only preconditions yourself: git fetch origin main, file reads, and GET requests.',
            'Fetch missing merge objects before ancestry checks; unavailable objects are not observed product defects.',
            'Use the read helper for these operations, retrying once; do not retry fetch, reads or checks beyond its budget.',
            'Use python factory/scripts/mission-read.py [--required] -- <read-command> [args...]',
            'Each failed read retries once; retain both attempts and available output.',
            'Never retry mutations through the helper or add an agent retry after exhaustion.',
            'Execute only the bound mission, including cron-origin missions. Run every named command/read and report every named value, unit and source;',
        )
        for clause in clauses:
            with self.subTest(clause=clause):
                self.assertIn(clause, self.mission)

    def test_unsatisfied_preconditions_name_missing_values_without_filing(self):
        clauses = (
            'Observed defects and command failures unrelated to prerequisites return FAILED with the exact reason and available values.',
            'If the only problem is an unmet precondition, return ACCEPTED with output.precondition and available values, including partial measurements.',
            'This includes exhausted required reads and a daemon not yet restarted onto the fix; file no corrective Work or proposal for it.',
            'Do not restart the daemon or mutate product state to satisfy a precondition.',
            'Use the exact key output.precondition with a non-blank string for an unmet precondition.',
        )
        for clause in clauses:
            with self.subTest(clause=clause):
                self.assertIn(clause, self.mission)

    def test_corrective_work_requires_an_observed_defect_and_same_pr_tests(self):
        clauses = (
            'Only an observed product or factory defect qualifies as a corrective gap below.',
            'File corrective work as a lane that changes behavior with tests shipped in the same PR.',
            'Never file a lane whose outcome is evidence, verification, characterization, measurement or re-running a check.',
            'Never file amendment or retrospective lanes, or corrective Work merely to make merge evidence available.',
            'For gaps, prepare a narrow corrective batch plus its own dependent loopback.',
        )
        for clause in clauses:
            with self.subTest(clause=clause):
                self.assertIn(clause, self.mission)
        self.assertLess(self.mission.index(clauses[0]), self.mission.index(clauses[-1]))


class AuthoredProgressStartupPolicyTests(unittest.TestCase):
    """The delivered first-visit instructions are the requested contract."""

    def test_first_visit_initialization_precedes_dependent_reading(self):
        prompt = SCRIPT.parent.parent / 'workstations/process/AGENTS.md'
        text = ' '.join(prompt.read_text(encoding='utf-8').split())
        startup = text[text.index('2. Read the progress log'):text.index('2.1.')]
        for clause in (
            'An absent or empty `progress.txt` on a first visit is normal.',
            'Start it with a `# Codebase Patterns` header, then continue the task.',
            'Never block or escalate solely because the first-visit progress log is absent or empty.',
            'Preserve existing non-empty progress and retained history.',
        ):
            with self.subTest(clause=clause):
                self.assertIn(clause, startup)
        reminder = 'Read the Codebase Patterns section in progress.txt before starting.'
        self.assertLess(text.index(startup), text.index(reminder))
        self.assertIn(reminder + ' For an absent or empty first-visit log, initialize it as described in step 2 and continue.', text)


if __name__ == '__main__':
    unittest.main()
