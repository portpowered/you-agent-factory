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

    def write(self, path, mtime):
        path.write_text("x", encoding="utf-8")
        os.utime(path, (mtime, mtime))

    def run_script(self, work_type, feedback, clock=None, identity=(), http=None):
        clock = clock or FakeClock(10_000)
        out, err = io.StringIO(), io.StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            code = self.script.run(
                ["mailbox-wait.py", LANE, work_type, feedback, *identity],
                now=clock.now,
                sleep=clock.sleep,
                mailbox=self.mailbox,
                settings=(3600, 30),
                http=http or self.no_http,
            )
        return code, out.getvalue(), err.getvalue(), clock

    def no_http(self, *args):
        self.fail("unexpected notification")

    def notice_http(self, calls, failure=None):
        def http(method, url, body, timeout):
            calls.append((method, url, body, timeout))
            if failure:
                raise failure
            if method == "PUT":
                return {"requestId": body["requestId"], "works": [
                    {"name": body["works"][0]["name"], "workTypeName": "project-report", "workId": "report-id"}]}
            return dict(calls[0][2]["works"][0], workId="report-id")
        return http

    def test_tagged_notification_then_fresh_lead_answer_for_both_types(self):
        identity = ("project-a", "work-a", "session-a", "http://127.0.0.1:1234")
        for kind in ("task", "idea"):
            with self.subTest(kind=kind):
                self.write(self.request, 10000)
                self.response.unlink(missing_ok=True)
                calls = []
                clock = FakeClock(10000, lambda c: self.write(self.response, c.value))
                code, out, err, _ = self.run_script(kind, MARKED, clock, identity, self.notice_http(calls))
                self.assertEqual(code, 0)
                self.assertIn("lead notified", err)
                self.assertIn("response present", out)
                self.assertEqual([c[0] for c in calls], ["PUT", "GET"])
                body = calls[0][2]
                self.assertEqual(body["works"][0]["tags"], {"project": "project-a"})
                payload = body["works"][0]["payload"]
                self.assertEqual(payload["laneWorkId"], "work-a")
                self.assertEqual(payload["sessionId"], "session-a")
                self.assertEqual(payload["laneWorkType"], kind)
                self.assertEqual(payload["requestPath"], str(self.request.resolve()))
                self.assertNotIn("relations", body)
                self.assertTrue(all(0 < c[3] <= 10 for c in calls))

    def test_outage_and_uncertain_receipt_keep_wait_and_stable_replay_identity(self):
        self.write(self.request, 10000)
        identity = ("project-a", "work-a", "session-a", "http://localhost:1234")
        bodies = []
        for failure in (TimeoutError("outage"), ValueError("lost receipt")):
            calls = []
            code, _, err, clock = self.run_script("task", MARKED, FakeClock(10000), identity,
                                                  self.notice_http(calls, failure))
            self.assertEqual(code, 0)
            self.assertEqual(clock.value, 13600)
            self.assertEqual(len(calls), 1)
            self.assertIn("lead notification failed", err)
            for field in ("session=session-a", "work=work-a", "version=", "request=mailbox-"):
                self.assertIn(field, err)
            self.assertFalse(self.response.exists())
            bodies.append(calls[0][2])
        self.assertEqual(bodies[0], bodies[1])

    def test_identity_changes_only_with_session_work_or_request_version(self):
        self.write(self.request, 10000)
        ids = []
        for session, work, stamp in (("s1", "w1", 10000), ("s1", "w1", 10000),
                                     ("s2", "w1", 10000), ("s1", "w2", 10000), ("s1", "w1", 11000)):
            self.write(self.request, stamp)
            calls = []
            self.run_script("task", MARKED, FakeClock(stamp),
                            ("project-a", work, session, "http://localhost"), self.notice_http(calls))
            ids.append(calls[0][2]["requestId"])
        self.assertEqual(ids[0], ids[1])
        self.assertEqual(len(set(ids)), 4)

    def test_incomplete_identity_or_remote_endpoint_never_submits_or_answers(self):
        self.write(self.request, 10000)
        for identity in (("project-a",), ("project-a", "w", "s", "https://example.com")):
            code, _, err, _ = self.run_script("task", MARKED, identity=identity)
            self.assertEqual(code, 0)
            self.assertIn("lead notification failed", err)
            self.assertFalse(self.response.exists())

    def test_stale_undelivered_answer_cannot_release_rewritten_request(self):
        self.write(self.request, 10000)
        self.write(self.response, 9999)
        code, out, _, clock = self.run_script("task", MARKED)
        self.assertEqual(code, 0)
        self.assertIn("no answer within", out)
        self.assertEqual(clock.value, 13600)

    def test_tagged_skips_notice_on_response_missing_request_unmarked_and_expiry(self):
        identity = ("project-a", "w", "s", "http://localhost")
        self.write(self.request, 10000)
        self.write(self.response, 10001)
        self.assertEqual(self.run_script("idea", MARKED, identity=identity)[0], 0)
        self.assertEqual(self.run_script("idea", MARKED, identity=identity)[0], 1)
        self.response.unlink()
        self.assertEqual(self.run_script("task", "ordinary CONTINUE", identity=identity)[0], 0)
        self.assertEqual(self.run_script("idea", MARKED, FakeClock(13600), identity)[0], 1)
        self.request.unlink()
        self.assertEqual(self.run_script("task", MARKED, identity=identity)[0], 0)

    def test_receipt_and_admitted_identity_mismatches_do_not_claim_notification(self):
        identity = ("project-a", "w", "s", "http://localhost")
        self.write(self.request, 10000)
        for mismatch in ("requestId", "workId", "tags", "payload"):
            with self.subTest(mismatch=mismatch):
                calls = []
                good_http = self.notice_http(calls)
                def http(method, url, body, timeout):
                    result = good_http(method, url, body, timeout)
                    if (method == "PUT") == (mismatch == "requestId"):
                        result[mismatch] = "wrong"
                    return result
                code, _, err, clock = self.run_script("task", MARKED, FakeClock(10000), identity, http)
                self.assertEqual(code, 0)
                self.assertIn("lead notification failed", err)
                self.assertNotIn("lead notified", err)
                self.assertEqual(clock.value, 13600)
                self.assertFalse(self.response.exists())

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
            code = self.script.run(["mailbox-wait.py", "../x", "task", MARKED], mailbox=self.mailbox)
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
