#!/usr/bin/env python3
"""Focused tests for the public-CLI Project reconciliation script."""

import sys
sys.dont_write_bytecode = True

import importlib.util
import json
import subprocess
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory" / "scripts" / "reconcile-projects.py"
SPEC = importlib.util.spec_from_file_location("reconcile_projects", SCRIPT_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


def project(name="demo", state="waiting", work_id="project-1"):
    return {
        "name": name,
        "workId": work_id,
        "workTypeName": "project",
        "state": {"name": state, "type": "PROCESSING" if state == "waiting" else "FAILED"},
        "payload": {"projectRoot": "docs/temp/projects/demo"},
    }


def child(name="demo-c01-lane", state="complete", work_type="idea"):
    state_type = "FAILED" if state in {"failed", "blocked"} else (
        "INITIAL" if state == "init" else "TERMINAL"
    )
    return {
        "name": name,
        "workId": name + "-work",
        "workTypeName": work_type,
        "state": {"name": state, "type": state_type},
        "payload": {"project": "demo"},
    }


def cycle(name="demo", state="failed"):
    return {
        "name": name,
        "workId": "cycle-1",
        "workTypeName": "project-cycle",
        "state": {"name": state, "type": "FAILED" if state != "continue" else "PROCESSING"},
        "payload": "continue",
    }


class ReconcileProjectsTest(unittest.TestCase):
    def setUp(self):
        self.commands = []

    def runner(self, responses):
        def run(command):
            self.commands.append(command)
            key = tuple(command[4:])
            response = responses.get(key)
            if response is None and command[4:6] == ["worker-sessions", "list"]:
                response = {"sessions": []}
            if response is None and command[4:6] == ["work", "move"]:
                response = {"workId": command[6]}
            if response is None:
                self.fail(f"unexpected command: {command}")
            return subprocess.CompletedProcess(command, 0, stdout=json.dumps(response), stderr="")

        return run

    def base_responses(self, works):
        return {
            ("session", "show", "session-1"): {
                "runtime": {
                    "progress": {"factoryState": "RUNNING"},
                    "lifecycle": {"updatedAt": "2026-09-05T00:00:00Z"},
                }
            },
            ("work", "list", "--session", "session-1"): {"results": works},
        }

    def test_failed_child_moves_waiting_project_without_overlapping_cycle(self):
        works = [project(), child(state="failed")]
        result = MODULE.reconcile(
            server="http://127.0.0.1:7437",
            session_id="session-1",
            runner=self.runner(self.base_responses(works)),
        )

        self.assertEqual(result["moved"][0]["reason"], "missing-cycle")
        move = self.commands[-1]
        self.assertEqual(move[0:4], ["you", "--server", "http://127.0.0.1:7437", "--json"])
        self.assertIn("--request-id", move)
        request_id = move[-1]
        self.assertRegex(request_id, r"^project-reconcile-project-1-[0-9a-f]{20}$")
        self.assertIn(result["moved"][0]["observationRevision"], request_id)

    def test_current_cycle_is_left_to_authored_transitions(self):
        works = [project(), child(state="failed"), cycle()]
        result = MODULE.reconcile(
            server="http://127.0.0.1:7437",
            session_id="session-1",
            runner=self.runner(self.base_responses(works)),
        )

        self.assertEqual(result["moved"], [])
        self.assertEqual(result["skipped"], [{"name": "demo", "reason": "cycle-transition-pending"}])
        self.assertEqual(len(self.commands), 2)

    def test_unfinished_child_does_not_bar_a_stranded_waiting_lead(self):
        works = [project(), child(state="init")]
        result = MODULE.reconcile(
            server="http://127.0.0.1:7437",
            session_id="session-1",
            runner=self.runner(self.base_responses(works)),
        )

        self.assertEqual(result["moved"][0]["reason"], "missing-cycle")
        self.assertEqual(result["moved"][0]["unfinishedChildren"], ["demo-c01-lane-work"])

    def test_healthy_waiting_project_with_cycle_is_not_moved(self):
        works = [project(), child(state="complete"), cycle(state="complete")]
        result = MODULE.reconcile(
            server="http://127.0.0.1:7437",
            session_id="session-1",
            runner=self.runner(self.base_responses(works)),
        )

        self.assertEqual(result["moved"], [])
        self.assertEqual(result["skipped"], [{"name": "demo", "reason": "cycle-transition-pending"}])

    def test_active_project_lead_is_not_moved(self):
        works = [project(), child(state="failed")]
        active_lead = {
            "workerSessionId": "worker-session-1",
            "factorySessionId": "session-1",
            "state": "RUNNING",
            "workId": "project-1",
            "workIds": ["project-1"],
        }
        responses = self.base_responses(works)
        responses[("worker-sessions", "list", "--work-id", "project-1", "--session", "session-1")] = {
            "sessions": [active_lead]
        }

        result = MODULE.reconcile(
            server="http://127.0.0.1:7437",
            session_id="session-1",
            runner=self.runner(responses),
        )

        self.assertEqual(result["moved"], [])
        self.assertEqual(result["skipped"], [{"name": "demo", "reason": "project-lead-active"}])

    def test_blocked_project_is_inspect_only_even_with_failed_child(self):
        works = [project(state="blocked"), child(state="failed", work_type="validation")]
        result = MODULE.reconcile(
            server="http://127.0.0.1:7437",
            session_id="session-1",
            runner=self.runner(self.base_responses(works)),
        )

        self.assertEqual(result["moved"], [])
        self.assertEqual(result["skipped"], [{"name": "demo", "reason": "blocked-inspect-only"}])

    def test_active_scoped_observation_without_work_identity_fails_closed(self):
        works = [project(), child(state="failed")]
        responses = self.base_responses(works)
        responses[("worker-sessions", "list", "--work-id", "project-1", "--session", "session-1")] = {
            "sessions": [{"state": "RUNNING"}]
        }

        result = MODULE.reconcile(
            server="http://127.0.0.1:7437",
            session_id="session-1",
            runner=self.runner(responses),
        )

        self.assertEqual(result["moved"], [])
        self.assertEqual(result["skipped"], [{"name": "demo", "reason": "project-lead-active"}])

    def test_dry_run_reports_move_without_calling_move_endpoint(self):
        works = [project(), child(state="failed")]
        result = MODULE.reconcile(
            server="http://127.0.0.1:7437",
            session_id="session-1",
            dry_run=True,
            runner=self.runner(self.base_responses(works)),
        )

        self.assertEqual(result["status"], "dry-run")
        self.assertEqual(result["moved"][0]["workId"], "project-1")
        self.assertEqual(self.commands[-1][4:6], ["worker-sessions", "list"])


if __name__ == "__main__":
    unittest.main()
