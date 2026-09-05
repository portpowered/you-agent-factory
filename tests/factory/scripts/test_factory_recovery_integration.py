"""Compiled-artifact integration proof; set YOU_TEST_BINARY to a prebuilt you.

Exercises the authored failure joins through a private HTTP host. No provider
calls, binary builds, or operator-profile mutations occur in this test.
"""

import json
import os
from pathlib import Path
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


if __name__ == "__main__":
    unittest.main()
