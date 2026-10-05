"""Compiled-artifact integration proof; set YOU_TEST_BINARY to a prebuilt you.

Exercises the authored failure joins through a private HTTP host. No provider
calls, binary builds, or operator-profile mutations occur in this test.
"""

import json
import os
from pathlib import Path
import shutil
import sys
import socket
import subprocess
import tempfile
import time
import unittest
import urllib.error
import urllib.request


ROOT = Path(__file__).resolve().parents[3]


def read_json(url):
    with urllib.request.urlopen(url, timeout=2) as response:
        return json.load(response)


@unittest.skipUnless(os.environ.get("YOU_TEST_BINARY"), "requires a prebuilt YOU_TEST_BINARY")
class FactoryRecoveryIntegrationTest(unittest.TestCase):
    def test_merged_pr_script_completes_idea_and_releases_dependent(self):
        with tempfile.TemporaryDirectory(prefix="merged-pr-artifact-") as directory:
            root = Path(directory)
            factory = root / "factory"
            factory.mkdir()
            authored = json.loads((ROOT / "factory/factory.json").read_text())
            selected = {"ci-wait", "consume", "report-idea-complete"}
            waiter = next(w for w in authored["workers"] if w.get("id") == "ci-waiter")
            script = factory / "scripts/ci-wait.py"
            script.parent.mkdir(parents=True)
            shutil.copy2(ROOT / "factory/scripts/ci-wait.py", script)
            waiter["command"] = sys.executable
            config = {"name": "merged-pr-artifact", "workTypes": authored["workTypes"],
                      "workers": [waiter], "resources": [],
                      "workstations": [w for w in authored["workstations"] if w["name"] in selected]}
            config["workstations"].append({"name": "release-dependent", "type": "LOGICAL_MOVE",
                "inputs": [{"workType": "validation", "state": "init"}],
                "outputs": [{"workType": "validation", "state": "complete"}]})
            (factory / "factory.json").write_text(json.dumps(config))
            environment = merged_probe_environment(root, factory)
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                port = listener.getsockname()[1]
            base = f"http://127.0.0.1:{port}"
            with (root / "host.log").open("w+") as log:
                process = subprocess.Popen([os.environ["YOU_TEST_BINARY"], "run", "--dir", str(factory),
                    "--continuously", "--with-server", "--listen", f"127.0.0.1:{port}"],
                    cwd=root, env=environment, stdin=subprocess.DEVNULL, stdout=log, stderr=log)
                try:
                    session = await_merged_probe(self, process, base + "/factory-sessions",
                        lambda data: data.get("sessions", []), log)["sessions"][0]["id"]
                    works = [{"name": "merged-lane", "workId": "merged-idea", "workTypeName": "idea", "state": "to-complete"},
                             {"name": "dependent", "workId": "merged-dependent", "workTypeName": "validation", "state": "init"}]
                    merged_probe_submit(base, session, "merged-parents", works, [{"type": "DEPENDS_ON",
                        "sourceWorkName": "dependent", "targetWorkName": "merged-lane", "requiredState": "complete"}])
                    merged_probe_submit(base, session, "merged-task", [{"name": "merged-lane",
                        "workId": "merged-task", "workTypeName": "task", "state": "awaiting-ci"}])
                    url = base + f"/factory-sessions/{session}/work?includeSuperseded=true"
                    def completed(data):
                        states = {w["workId"]: w["state"]["name"] for w in data.get("results", [])}
                        return all(states.get(key) == "complete" for key in (
                            "merged-idea", "merged-task", "merged-dependent"))
                    await_merged_probe(self, process, url, completed, log)
                    events = merged_probe_events(base + f"/factory-sessions/{session}/events")
                    releases = [e for e in events if e["type"] == "DISPATCH_REQUEST"
                                and e["payload"].get("transitionId") == "release-dependent"]
                    self.assertEqual(len(releases), 1)
                    self.assertTrue((root / "gh-observed.txt").exists(), "real script never reached gh stub")
                finally:
                    process.terminate()
                    try:
                        process.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=10)

    def test_failed_delivery_wakes_only_its_dependent_cycle(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            factory = root / "factory"
            factory.mkdir()
            authored = json.loads((ROOT / "factory/factory.json").read_text())
            selected = {"escalate-plan-failure", "escalate-task-failure", "consume",
                        "retry-project-after-cycle-failure", "escalate-project"}
            config = {"name": "failure-feedback-probe", "workTypes": authored["workTypes"],
                      "workers": [], "resources": [],
                      "workstations": [s for s in authored["workstations"] if s["name"] in selected]}
            (factory / "factory.json").write_text(json.dumps(config))
            works, relations = [], []
            for child in ("plan", "task"):
                lane = f"failed-{child}"
                works.extend([
                    {"name": lane, "workTypeName": "idea", "state": "to-complete"},
                    {"name": f"cycle-{child}", "workTypeName": "project-cycle", "state": "init"}])
                # At admission each lane name refers only to its idea carrier.
                relations.append({"type": "DEPENDS_ON", "sourceWorkName": f"cycle-{child}",
                                  "targetWorkName": lane, "requiredState": "complete"})
            works.append({"name": "blocked-example", "workTypeName": "project", "state": "needs-supervision"})
            works.append({"name": "healthy", "workTypeName": "idea", "state": "to-complete"})
            works.append({"name": "healthy-project", "workTypeName": "project", "state": "waiting"})
            batch = root / "batch.json"
            batch.write_text(json.dumps({"type": "FACTORY_REQUEST_BATCH", "requestId": "failure-probe",
                                         "works": works, "relations": relations}))
            failures = root / "failures.json"
            failures.write_text(json.dumps({"type": "FACTORY_REQUEST_BATCH", "requestId": "failed-children",
                                           "works": [{"name": f"failed-{child}", "workTypeName": child,
                                                      "state": "failed"} for child in ("plan", "task")]
                                           + [{"name": f"cycle-{child}", "workTypeName": "project",
                                               "state": "waiting"} for child in ("plan", "task")]}))
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                port = listener.getsockname()[1]
            base = f"http://127.0.0.1:{port}"
            profile = root / "profile"
            profile.mkdir()
            environment = dict(os.environ, HOME=str(profile), USERPROFILE=str(profile))
            with (root / "host.log").open("w+") as log:
                process = subprocess.Popen([os.environ["YOU_TEST_BINARY"], "run", "--dir", str(factory),
                                            "--continuously", "--with-server",
                                            "--listen", f"127.0.0.1:{port}"], cwd=root, env=environment,
                                           stdin=subprocess.DEVNULL, stdout=log, stderr=log)
                try:
                    deadline = time.monotonic() + 40
                    observed = {}
                    submitted = False
                    while time.monotonic() < deadline and process.poll() is None:
                        try:
                            sessions = read_json(base + "/factory-sessions")["sessions"]
                            if sessions:
                                if not submitted:
                                    initial = subprocess.run([os.environ["YOU_TEST_BINARY"], "--server", base,
                                                              "submit", "batch", str(batch), "--session",
                                                              sessions[0]["id"]], cwd=root, env=environment,
                                                             capture_output=True, text=True, timeout=15)
                                    self.assertEqual(initial.returncode, 0, initial.stdout + initial.stderr)
                                    admission = subprocess.run([os.environ["YOU_TEST_BINARY"], "--server", base,
                                                                "submit", "batch", str(failures), "--session",
                                                                sessions[0]["id"]], cwd=root, env=environment,
                                                               capture_output=True, text=True, timeout=15)
                                    self.assertEqual(admission.returncode, 0, admission.stdout + admission.stderr)
                                    submitted = True
                                data = read_json(base + f"/factory-sessions/{sessions[0]['id']}/work?includeSuperseded=true")
                                items = data if isinstance(data, list) else data.get("results", [])
                                observed = {(w["name"], w["workTypeName"]): w["state"]["name"] for w in items}
                                if all(observed.get((f"cycle-{child}", "project")) == "init"
                                       for child in ("plan", "task")) and observed.get(("blocked-example", "thoughts")) == "init":
                                    break
                        except (OSError, KeyError, TypeError, urllib.error.HTTPError):
                            pass
                        # Poll a real OS/listener boundary; no in-process event hook exists here.
                        time.sleep(0.1)
                    log.flush()
                    log.seek(0)
                    diagnostic = repr(observed) + "\n" + log.read()[-6000:]
                    for runtime_log in root.rglob("*.log"):
                        if "runtime-log" in runtime_log.name:
                            diagnostic += runtime_log.read_text(errors="replace")[-5000:]
                    for child in ("plan", "task"):
                        self.assertEqual(observed.get((f"failed-{child}", "idea")), "failed", diagnostic)
                        self.assertEqual(observed.get((f"cycle-{child}", "project")), "init", diagnostic)
                    self.assertEqual(observed.get(("blocked-example", "project")), "blocked", diagnostic)
                    self.assertEqual(observed.get(("blocked-example", "thoughts")), "init", diagnostic)
                    self.assertEqual(sum(w["name"] == "blocked-example" and w["workTypeName"] == "thoughts" for w in items), 1)
                    self.assertEqual(observed.get(("healthy", "idea")), "to-complete", diagnostic)
                    self.assertEqual(observed.get(("healthy-project", "project")), "waiting", diagnostic)
                finally:
                    try:
                        urllib.request.urlopen(urllib.request.Request(base + "/shutdown", data=b"{}",
                                                                      headers={"Content-Type": "application/json"}), timeout=5).close()
                        process.wait(timeout=10)
                    except (OSError, subprocess.TimeoutExpired):
                        process.terminate()
                        process.wait(timeout=10)


def merged_probe_environment(root, factory):
    profile = root / "profile"
    profile.mkdir()
    bindir = root / "bin"
    bindir.mkdir()
    source = ("import json\nfrom pathlib import Path\n"
              f"Path({str(root / 'gh-observed.txt')!r}).write_text('MERGED')\n"
              "print(json.dumps([{'number': 99, 'state': 'MERGED'}]))\n")
    if os.name == "nt":
        shutil.copy2(sys.executable, bindir / "gh.exe")
        for name in (f"python{sys.version_info.major}{sys.version_info.minor}.dll",
                     "vcruntime140.dll", "vcruntime140_1.dll"):
            runtime = Path(sys.executable).with_name(name)
            if runtime.exists():
                shutil.copy2(runtime, bindir / name)
        for directory in (root, factory):
            (directory / "pr").write_text(source)
    else:
        gh = bindir / "gh"
        gh.write_text(f"#!{sys.executable}\n" + source)
        gh.chmod(0o755)
    environment = dict(os.environ, HOME=str(profile), USERPROFILE=str(profile),
                       HOMEDRIVE=profile.drive, HOMEPATH=str(profile)[len(profile.drive):])
    environment["PATH"] = str(bindir) + os.pathsep + environment.get("PATH", "")
    return environment


def merged_probe_submit(base, session, identity, works, relations=()):
    payload = json.dumps({"requestId": identity, "type": "FACTORY_REQUEST_BATCH",
                          "works": works, "relations": list(relations)}).encode()
    request = urllib.request.Request(base + f"/factory-sessions/{session}/work-requests/{identity}",
        data=payload, headers={"Content-Type": "application/json"}, method="PUT")
    with urllib.request.urlopen(request, timeout=15) as response:
        return json.load(response)


def await_merged_probe(test, process, url, predicate, log):
    deadline = time.monotonic() + 40
    observed = None
    while time.monotonic() < deadline and process.poll() is None:
        try:
            observed = read_json(url)
            if predicate(observed):
                return observed
        except (OSError, urllib.error.HTTPError):
            pass
        # Listener and executable are real OS edges with no in-process gate.
        time.sleep(0.1)
    log.flush()
    log.seek(0)
    test.fail(f"artifact probe did not complete: observed={observed!r}; host={log.read()}")


def merged_probe_events(url):
    with urllib.request.urlopen(url, timeout=15) as response:
        count = int(response.headers["X-Factory-Session-Retained-Event-Count"])
        events = []
        while len(events) < count:
            line = response.readline().decode()
            if line.startswith("data:"):
                events.append(json.loads(line[5:]))
        return events


if __name__ == "__main__":
    unittest.main()
