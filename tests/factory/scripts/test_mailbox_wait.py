#!/usr/bin/env python3
"""Tests for the TEMPORARY mailbox-wait park (AM-T0; removed by AM-T11).

Covers the script's routing decisions and the factory.json shape that keeps the
park free of executor slots and process/review visits.
"""

import importlib.util
import io
import json
import os
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from pathlib import Path
from unittest.mock import patch


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory" / "scripts" / "mailbox-wait.py"
FACTORY_PATH = REPO_ROOT / "factory" / "factory.json"
LANE = "am-probe-lane"
MARKED = "AWAITING_OPERATOR_ANSWER request filed; no-answer path: option A"


def load_script():
    spec = importlib.util.spec_from_file_location("mailbox_wait", SCRIPT_PATH)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class FakeClock:
    def __init__(self, start, on_sleep=None):
        self.value = start
        self.sleeps = []
        self.on_sleep = on_sleep

    def now(self):
        return self.value

    def sleep(self, seconds):
        self.sleeps.append(seconds)
        self.value += seconds
        if self.on_sleep:
            self.on_sleep(self)


class MailboxWaitScriptTest(unittest.TestCase):
    def setUp(self):
        self.script = load_script()
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.mailbox = Path(self.tmp.name)
        (self.mailbox / "requests").mkdir()
        (self.mailbox / "responses").mkdir()
        self.request = self.mailbox / "requests" / f"{LANE}.md"
        self.response = self.mailbox / "responses" / f"{LANE}.md"
        env = patch.dict(
            os.environ,
            {
                "YOU_OPERATOR_MAILBOX_DIR": str(self.mailbox),
                "MAILBOX_WAIT_WINDOW_SECONDS": "",
                "MAILBOX_WAIT_POLL_SECONDS": "",
            },
        )
        env.start()
        self.addCleanup(env.stop)

    def write(self, path, mtime):
        path.write_text("x", encoding="utf-8")
        os.utime(path, (mtime, mtime))

    def run_script(self, work_type, feedback, clock=None):
        clock = clock or FakeClock(10_000)
        out, err = io.StringIO(), io.StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            code = self.script.run(
                ["mailbox-wait.py", LANE, work_type, feedback],
                now=clock.now,
                sleep=clock.sleep,
            )
        return code, out.getvalue(), err.getvalue(), clock

    def test_unmarked_task_continue_returns_to_init_at_once(self):
        self.write(self.request, 10_000)
        code, out, _, clock = self.run_script("task", "ordinary progress; more stories")
        self.assertEqual(code, 0)
        self.assertIn("ordinary CONTINUE", out)
        self.assertEqual(clock.sleeps, [])

    def test_unmarked_idea_continue_keeps_legacy_failure_route(self):
        code, _, err, clock = self.run_script("idea", "incomplete planning")
        self.assertEqual(code, 1)
        self.assertIn("reporting-failed", err)
        self.assertEqual(clock.sleeps, [])

    def test_marked_without_request_does_not_wait(self):
        self.assertEqual(self.run_script("task", MARKED)[0], 0)
        self.assertEqual(self.run_script("idea", MARKED)[0], 1)

    def test_parks_until_response_appears(self):
        self.write(self.request, 10_000)

        def answer_after_two_polls(clock):
            if len(clock.sleeps) == 2:
                self.write(self.response, clock.value)

        clock = FakeClock(10_000, on_sleep=answer_after_two_polls)
        for work_type in ("task", "idea"):
            with self.subTest(work_type=work_type):
                self.response.unlink(missing_ok=True)
                (self.mailbox / "waits" / f"{LANE}.delivered").unlink(missing_ok=True)
                clock.sleeps.clear()
                code, out, _, _ = self.run_script(work_type, MARKED, clock)
                self.assertEqual(code, 0)
                self.assertIn("response present", out)
                self.assertEqual(clock.sleeps, [30, 30])

    def test_poll_interval_meets_two_minute_resume_bound(self):
        self.assertLessEqual(self.script.POLL_INTERVAL_SECONDS, 60)

    def test_no_answer_returns_to_init_after_sixty_minutes(self):
        self.write(self.request, 10_000)
        for work_type in ("task", "idea"):
            with self.subTest(work_type=work_type):
                clock = FakeClock(10_000)
                code, out, _, _ = self.run_script(work_type, MARKED, clock)
                self.assertEqual(code, 0)
                self.assertIn("no answer within the wait window", out)
                self.assertEqual(clock.value, 10_000 + 60 * 60)
                self.assertEqual(sum(clock.sleeps), 60 * 60)

    def test_second_park_on_unchanged_unanswered_request_does_not_wait(self):
        self.write(self.request, 10_000)
        late = FakeClock(10_000 + 60 * 60)
        code, out, _, _ = self.run_script("task", MARKED, late)
        self.assertEqual((code, late.sleeps), (0, []))
        self.assertIn("already waited its full window", out)
        late = FakeClock(10_000 + 60 * 60)
        code, _, err, _ = self.run_script("idea", MARKED, late)
        self.assertEqual((code, late.sleeps), (1, []))
        self.assertIn("reporting-failed", err)

    def test_rewritten_request_opens_a_fresh_window(self):
        self.write(self.request, 10_000 + 2 * 60 * 60)
        clock = FakeClock(10_000 + 2 * 60 * 60 + 5)
        code, _, _, _ = self.run_script("task", MARKED, clock)
        self.assertEqual(code, 0)
        self.assertGreater(sum(clock.sleeps), 60 * 59)

    def test_delivered_response_cannot_loop_a_planner(self):
        self.write(self.request, 10_000)
        self.write(self.response, 10_100)
        self.assertEqual(self.run_script("idea", MARKED)[0], 0)
        code, _, err, clock = self.run_script("idea", MARKED)
        self.assertEqual((code, clock.sleeps), (1, []))
        self.assertIn("already delivered", err)
        self.assertEqual(self.run_script("task", MARKED)[0], 0)

    def test_new_request_after_delivered_response_waits_for_a_new_answer(self):
        self.write(self.request, 10_000)
        self.write(self.response, 10_100)
        self.assertEqual(self.run_script("task", MARKED)[0], 0)
        self.write(self.request, 20_000)

        def answer(clock):
            if len(clock.sleeps) == 1:
                self.write(self.response, clock.value)

        clock = FakeClock(20_000, on_sleep=answer)
        code, out, _, _ = self.run_script("task", MARKED, clock)
        self.assertEqual((code, clock.sleeps), (0, [30]))
        self.assertIn("response present", out)

    def test_rejects_unknown_work_type_and_unsafe_lane(self):
        self.assertEqual(self.run_script("review", MARKED)[0], 1)
        out = io.StringIO()
        with redirect_stdout(out), redirect_stderr(io.StringIO()):
            code = self.script.run(["mailbox-wait.py", "../x", "task", MARKED])
        self.assertEqual(code, 0)
        self.assertIn("unsafe lane name", out.getvalue())


class MailboxWaitFactoryShapeTest(unittest.TestCase):
    """The park must hold no executor slot and sit outside the loop breakers."""

    @classmethod
    def setUpClass(cls):
        cls.factory = json.loads(FACTORY_PATH.read_text(encoding="utf-8"))
        cls.workstations = {w["name"]: w for w in cls.factory["workstations"]}
        cls.workers = {w["name"]: w for w in cls.factory["workers"]}

    def states(self, work_type):
        for entry in self.factory["workTypes"]:
            if entry["name"] == work_type:
                return {s["id"]: s["type"] for s in entry["states"]}
        self.fail(f"missing work type {work_type}")

    def test_awaiting_answer_states_exist(self):
        self.assertEqual(self.states("task").get("awaiting-answer"), "PROCESSING")
        self.assertEqual(self.states("idea").get("awaiting-answer"), "PROCESSING")

    def test_asking_workstations_continue_into_the_park(self):
        self.assertEqual(
            self.workstations["process"]["onContinue"],
            [{"state": "awaiting-answer", "workType": "task"}],
        )
        self.assertEqual(
            self.workstations["plan"]["onContinue"],
            [{"workType": "idea", "state": "awaiting-answer"}],
        )

    def test_park_workstations_hold_no_resources(self):
        expected = {
            "mailbox-wait": ("task", [{"state": "init", "workType": "task"}],
                             [{"state": "init", "workType": "task"}]),
            "mailbox-wait-idea": ("idea", [{"state": "init", "workType": "idea"}],
                                  [{"state": "reporting-failed", "workType": "idea"}]),
        }
        for name, (work_type, outputs, on_failure) in expected.items():
            with self.subTest(name=name):
                station = self.workstations[name]
                self.assertEqual(station["type"], "SCRIPT_RUN")
                self.assertEqual(station["worker"], "mailbox-waiter")
                self.assertNotIn("resources", station)
                self.assertEqual(
                    station["inputs"],
                    [{"state": "awaiting-answer", "workType": work_type}],
                )
                self.assertEqual(station["outputs"], outputs)
                self.assertEqual(station["onFailure"], on_failure)
                self.assertIn("AM-T11", station["description"]["value"])

    def test_park_is_outside_the_loop_breakers(self):
        for name in ("executor-loop-breaker", "review-loop-breaker"):
            for guard in self.workstations[name].get("guards", []):
                counted = {guard.get("workstation")}
                counted |= set(guard.get("logicalRoundTrip", {}).get("workstations", []))
                self.assertFalse(counted & {"mailbox-wait", "mailbox-wait-idea"})

    def test_waiter_passes_lane_type_and_bounded_continue_feedback(self):
        worker = self.workers["mailbox-waiter"]
        self.assertEqual(worker["type"], "SCRIPT_WORKER")
        self.assertNotIn("resources", worker)
        self.assertEqual(worker["args"][0], "factory/scripts/mailbox-wait.py")
        self.assertIn(".Name", worker["args"][1])
        self.assertIn(".WorkTypeID", worker["args"][2])
        self.assertIn("continue_feedback", worker["args"][3])
        self.assertIn("%.512s", worker["args"][3])


if __name__ == "__main__":
    unittest.main()
