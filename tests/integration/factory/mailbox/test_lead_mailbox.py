"""I1: prebuilt runtime admission, matched wake and mailbox resumption.

The build owner supplies YOU_TEST_BINARY; this test never builds. The lead's
inference boundary is replaced with a SCRIPT_RUN adapter, retaining authored
wake inputs, guard, outputs and resources. This is not live authority judgment.
"""

import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import unittest
import urllib.error
import urllib.request
import uuid


ROOT = Path(__file__).resolve().parents[4]
ANSWER_WORKER = '''import json, os, sys
from pathlib import Path
payload = json.loads(sys.argv[1])
assert payload['kind'] == 'mailbox-question'
assert payload['sessionId'] == sys.argv[4]
assert sys.argv[3] == 'owning-project'
request = Path(payload['requestPath'])
assert str(request.stat().st_mtime_ns) == payload['requestVersion']
root = Path(os.environ['YOU_OPERATOR_MAILBOX_DIR'])
observation = {'projectWorkId': sys.argv[2], 'projectTag': sys.argv[3], 'payload': payload}
(root / (payload['laneName'] + '.wake.json')).write_text(json.dumps(observation))
response = root / 'responses' / (payload['laneName'] + '.md')
temporary = response.with_suffix('.tmp')
temporary.write_text('Controlled lead response for ' + payload['requestVersion'])
temporary.replace(response)
'''


def http_json(url, body=None, method=None):
    request = urllib.request.Request(url, data=None if body is None else json.dumps(body).encode(),
                                     headers={"Content-Type": "application/json"}, method=method)
    with urllib.request.urlopen(request, timeout=3) as response:
        return json.load(response)


def submit(base, session, request_id, works):
    return http_json(f"{base}/factory-sessions/{session}/work-requests/{request_id}",
                     {"type": "FACTORY_REQUEST_BATCH", "requestId": request_id, "works": works}, "PUT")


def await_result(process, get, predicate, log):
    deadline = time.monotonic() + 35
    observed = None
    while time.monotonic() < deadline and process.poll() is None:
        try:
            observed = get()
            if predicate(observed):
                return observed
        except (OSError, urllib.error.HTTPError):
            pass
        # The compiled host exposes no in-process event hook. Bound polling
        # observes real listener/Work readiness, rather than causing readiness.
        time.sleep(0.1)
    log.flush()
    log.seek(0)
    raise AssertionError(f"runtime observation timed out: {observed!r}\n{log.read()[-6000:]}")


def events(url):
    with urllib.request.urlopen(url, timeout=3) as response:
        count = int(response.headers["X-Factory-Session-Retained-Event-Count"])
        result = []
        while len(result) < count:
            line = response.readline().decode()
            if line.startswith("data:"):
                result.append(json.loads(line[5:]))
        return result


class LeadMailboxIntegrationTest(unittest.TestCase):
    def test_I1_a_tagged_task_wakes_only_its_project_and_resumes(self):
        self.journey("task", include_untagged=False)

    def test_I1_b_idea_and_untagged_operator_route_resume(self):
        self.journey("idea", include_untagged=True)

    def journey(self, kind, include_untagged):
        binary = os.environ.get("YOU_TEST_BINARY", "")
        self.assertTrue(binary and Path(binary).is_file(), "required prebuilt YOU_TEST_BINARY is absent")
        with tempfile.TemporaryDirectory(prefix="lead-mailbox-") as directory:
            root = Path(directory)
            factory = root / "factory"
            factory.mkdir()
            mailbox = root / "mailbox"
            (mailbox / "requests").mkdir(parents=True)
            (mailbox / "responses").mkdir()
            profile = root / "profile"
            profile.mkdir()
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                port = listener.getsockname()[1]
            base = f"http://127.0.0.1:{port}"
            authored = json.loads((ROOT / "factory/factory.json").read_text(encoding="utf-8"))
            selected = {"mailbox-wait", "mailbox-wait-idea", "project-lead-wake"}
            stations = [s for s in authored["workstations"] if s["name"] in selected]
            wake = next(s for s in stations if s["name"] == "project-lead-wake")
            wake["type"] = "SCRIPT_RUN"  # controlled external inference adapter
            wake.pop("outcomeFormat", None)
            waiter = next(w for w in authored["workers"] if w["name"] == "mailbox-waiter")
            script = factory / "scripts/mailbox-wait.py"
            script.parent.mkdir()
            shutil.copy2(ROOT / "factory/scripts/mailbox-wait.py", script)
            waiter["command"] = sys.executable
            waiter["args"][-1] = base
            (factory / "answer.py").write_text(ANSWER_WORKER, encoding="utf-8")
            lead = {"id": "project-lead", "name": "project-lead", "type": "SCRIPT_WORKER",
                    "command": sys.executable, "args": [str(factory / "answer.py"),
                    "{{ (index .Inputs 1).Payload }}", "{{ (index .Inputs 0).WorkID }}",
                    '{{ index (index .Inputs 0).Tags "project" }}', "{{.Context.SessionID}}"]}
            config = {"name": "lead-mailbox-proof", "workTypes": authored["workTypes"],
                      "workstations": stations, "workers": [waiter, lead],
                      "resources": [r for r in authored["resources"] if r["name"] == "executor-slot"]}
            (factory / "factory.json").write_text(json.dumps(config), encoding="utf-8")
            env = dict(os.environ, HOME=str(profile), USERPROFILE=str(profile),
                       YOU_OPERATOR_MAILBOX_DIR=str(mailbox), MAILBOX_WAIT_WINDOW_SECONDS="20",
                       MAILBOX_WAIT_POLL_SECONDS="0.1")
            with (root / "host.log").open("w+", encoding="utf-8") as log:
                process = subprocess.Popen([binary, "run", "--dir", str(factory), "--continuously",
                                            "--with-server", "--listen", f"127.0.0.1:{port}"],
                                           cwd=root, env=env, stdin=subprocess.DEVNULL, stdout=log, stderr=log)
                try:
                    data = await_result(process, lambda: http_json(base + "/factory-sessions"),
                                        lambda d: bool(d.get("sessions")), log)
                    session = data["sessions"][0]["id"]
                    project_id, other_id, lane_id = [str(uuid.uuid4()) for _ in range(3)]
                    lane = f"proof-{kind}"
                    request = mailbox / "requests" / f"{lane}.md"
                    request.write_text("# Controlled narrowing decision\n", encoding="utf-8")
                    works = [
                        {"name": "owning-project", "workId": project_id, "workTypeName": "project",
                         "state": "waiting", "tags": {"project": "owning-project"}},
                        {"name": "other-project", "workId": other_id, "workTypeName": "project",
                         "state": "waiting", "tags": {"project": "other-project"}},
                        {"name": lane, "workId": lane_id, "workTypeName": kind, "state": "awaiting-answer",
                         "tags": {"project": "owning-project", "continue_feedback": "AWAITING_OPERATOR_ANSWER proof"}}]
                    if include_untagged:
                        (mailbox / "requests/operator-lane.md").write_text("# Operator decision", encoding="utf-8")
                        works.append({"name": "operator-lane", "workTypeName": "task", "state": "awaiting-answer",
                                      "tags": {"continue_feedback": "AWAITING_OPERATOR_ANSWER proof"}})
                    submit(base, session, "initial-" + str(uuid.uuid4()), works)
                    url = base + f"/factory-sessions/{session}/work?includeSuperseded=true"
                    def resumed(data):
                        return any(w["workId"] == lane_id and w["state"]["name"] == "init"
                                   for w in data.get("results", []))
                    data = await_result(process, lambda: http_json(url), resumed, log)
                    notice = json.loads((mailbox / f"{lane}.wake.json").read_text())
                    self.assertEqual(notice["projectWorkId"], project_id)
                    self.assertEqual(notice["payload"]["laneWorkId"], lane_id)
                    self.assertEqual(notice["payload"]["requestVersion"], str(request.stat().st_mtime_ns))
                    self.assertTrue((mailbox / "responses" / f"{lane}.md").exists())
                    reports = [w for w in data["results"] if w["workTypeName"] == "project-report"]
                    self.assertEqual(len(reports), 1)
                    self.assertEqual(reports[0]["state"]["name"], "delivered")
                    # Replaying the actual admitted request uses public idempotency;
                    # no second report or wake is created.
                    report = reports[0]
                    submit(base, session, report["requestId"], [{"name": report["name"],
                        "workTypeName": "project-report", "state": "pending", "tags": report["tags"],
                        "payload": report["payload"]}])
                    if include_untagged:
                        untagged = next(w for w in data["results"] if w["name"] == "operator-lane")
                        self.assertEqual(untagged["state"]["name"], "awaiting-answer")
                        self.assertFalse((mailbox / "operator-lane.wake.json").exists())
                        (mailbox / "responses/operator-lane.md").write_text("Binding operator response", encoding="utf-8")
                        await_result(process, lambda: http_json(url), lambda d: any(
                            w["name"] == "operator-lane" and w["state"]["name"] == "init"
                            for w in d.get("results", [])), log)
                    history = events(base + f"/factory-sessions/{session}/events")
                    dispatches = [e for e in history if e["type"] == "DISPATCH_REQUEST"
                                  and e["payload"].get("transitionId") == "project-lead-wake"]
                    self.assertEqual(len(dispatches), 1)
                    serialized = json.dumps(dispatches[0]["payload"])
                    self.assertIn(project_id, serialized)
                    self.assertNotIn(other_id, serialized)
                    current = http_json(url)["results"]
                    self.assertEqual(sum(w["workTypeName"] == "project-report" for w in current), 1)
                    self.assertEqual(next(w for w in current if w["workId"] == other_id)["state"]["name"], "waiting")
                    print(f"I1 {kind}: session={session} report={report['workId']} matched-project={project_id}; resumed")
                finally:
                    try:
                        http_json(base + "/shutdown", {}, "POST")
                        process.wait(timeout=10)
                    except (OSError, subprocess.TimeoutExpired):
                        process.terminate()
                        try:
                            process.wait(timeout=10)
                        except subprocess.TimeoutExpired:
                            process.kill()
                            process.wait(timeout=10)


if __name__ == "__main__":
    unittest.main()
