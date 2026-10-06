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
    def test_I2_successor_keeps_tag_packet_and_pr_through_repeated_delivery(self):
        """Real compiled routing; controlled lead/planner/provider decisions.

        I1 owns real Git adoption. This cell owns propagation, directory
        expansion, repeated delivery and exact-ID dependency release, not AI
        judgment. Every process/profile/port/file belongs to this cell.
        """
        with tempfile.TemporaryDirectory(prefix="successor-artifact-") as directory:
            root = Path(directory)
            factory = root / "factory"
            factory.mkdir()
            retained = root / ".claude/worktrees/old-lane"
            retained.mkdir(parents=True)
            saved = {"prd.json": b'{"project":"old"}', "progress.txt": b"retained progress\n",
                     "dirty.txt": b"useful unpublished bytes"}
            for name, contents in saved.items():
                (retained / name).write_bytes(contents)
            recovery = {"originalSessionId": "11111111-1111-4111-8111-111111111111",
                        "originalLaneWorkId": "old-idea", "predecessorWorkId": "old-idea", "attempt": 1,
                        "diagnosis": {"classification": "visit_cap_with_progress",
                                      "evidence": ["fixture:failed-child"], "blocker": "finish slice",
                                      "correction": "retain PR and finish"},
                        "workspace": {"branch": "old-lane", "worktree": ".claude/worktrees/old-lane",
                                      "prUrl": "https://github.com/example/repository/pull/99", "headSha": "a" * 40}}
            (root / "recovery.json").write_text(json.dumps(recovery))
            fixture = root / "worker.py"
            fixture.write_text(RECOVERY_WORKER, encoding="utf-8")
            authored = json.loads((ROOT / "factory/factory.json").read_text())
            selected = {"plan", "setup-workspace", "process", "ci-wait", "review", "consume",
                        "report-idea-complete", "report-idea-failure", "project-lead-wake"}
            stations = [w for w in authored["workstations"] if w["name"] in selected]
            workers = []
            for station in stations:
                if not station.get("worker"):
                    continue
                stage = station["name"]
                # Controlled decisions replace inference/external CI; retain
                # authored inputs, outputs, guards and directory templates.
                station["resources"] = []
                if stage in ("ci-wait", "setup-workspace"):
                    workers.append({"id": station["worker"], "name": station["worker"],
                        "type": "SCRIPT_WORKER", "command": sys.executable,
                        "args": [str(fixture), str(root), stage, "{{.Context.SessionID}}",
                                 "{{ (index .Inputs 0).Name }}", "{{ index (index .Inputs 0).Tags \"recovery-worktree\" }}"]})
                else:
                    workers.append({"id": station["worker"], "name": station["worker"],
                                    "type": "AGENT_WORKER", "model": "gpt-5-codex",
                                    "modelProvider": "CODEX", "executorProvider": "SCRIPT_WRAP"})
                    prompt = factory / "workstations" / stage / "AGENTS.md"
                    prompt.parent.mkdir(parents=True, exist_ok=True)
                    prompt.write_text('RECOVERY_FIXTURE ' + stage + ' {{.Context.SessionID}} '
                                      '{{ (index .Inputs 0).Name }}\nRECOVERY_TAG '
                                      '{{ index (index .Inputs 0).Tags "recovery-worktree" }}\n')
            for name, kind in (("release-dependent", "validation"), ("release-loopback", "project-cycle")):
                stations.append({"id": name, "name": name, "type": "LOGICAL_MOVE",
                                 "inputs": [{"workType": kind, "state": "init"}],
                                 "outputs": [{"workType": kind, "state": "complete"}]})
            config = {"name": "successor-artifact", "workTypes": authored["workTypes"],
                      "workers": workers, "resources": [], "workstations": stations}
            (factory / "factory.json").write_text(json.dumps(config))
            profile = root / "profile"
            profile.mkdir()
            environment = dict(os.environ, HOME=str(profile), USERPROFILE=str(profile),
                               HOMEDRIVE=profile.drive, HOMEPATH=str(profile)[len(profile.drive):])
            provider = root / "provider"
            provider.mkdir()
            provider_script = root / "provider.py"
            provider_script.write_text(RECOVERY_PROVIDER)
            if os.name == "nt":
                (provider / "codex.cmd").write_text(
                    '@echo off\n"' + sys.executable + '" "' + str(provider_script) + '" %*\n')
            else:
                executable = provider / "codex"
                executable.write_text('#!' + sys.executable + '\n' + RECOVERY_PROVIDER)
                executable.chmod(0o755)
            environment['RECOVERY_FIXTURE_ROOT'] = str(root)
            environment['PATH'] = str(provider) + os.pathsep + environment.get('PATH', '')
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                port = listener.getsockname()[1]
            base = f"http://127.0.0.1:{port}"
            (root / "base.txt").write_text(base)
            with (root / "host.log").open("w+") as log:
                process = subprocess.Popen([os.environ["YOU_TEST_BINARY"], "run", "--dir", str(factory),
                    "--continuously", "--with-server", "--listen", f"127.0.0.1:{port}"],
                    cwd=root, env=environment, stdin=subprocess.DEVNULL, stdout=log, stderr=log)
                try:
                    session = await_merged_probe(self, process, base + "/factory-sessions",
                        lambda data: data.get("sessions", []), log)["sessions"][0]["id"]
                    tags = {"project": "fixture-project"}
                    merged_probe_submit(base, session, "old-failure", [
                        {"name": "fixture-project", "workId": "lead", "workTypeName": "project", "state": "waiting", "tags": tags},
                        {"name": "old-lane", "workId": "old-idea", "workTypeName": "idea", "state": "reporting-failed", "tags": tags},
                        {"name": "old-dependent", "workId": "old-dependent", "workTypeName": "validation", "state": "failed"},
                        {"name": "old-loopback", "workId": "old-loopback", "workTypeName": "project-cycle", "state": "failed"},
                        {"name": "healthy", "workId": "healthy", "workTypeName": "idea", "state": "to-complete"},
                        {"name": "parked", "workId": "parked", "workTypeName": "task", "state": "awaiting-answer"}])
                    url = base + f"/factory-sessions/{session}/work?includeSuperseded=true"
                    def at_review(data):
                        return (root / "review-entered").exists()
                    before = await_merged_probe(self, process, url, at_review, log)
                    states = {w["workId"]: w["state"]["name"] for w in before["results"]}
                    self.assertEqual(states["new-dependent"], "init")
                    self.assertEqual(states["new-loopback"], "init")
                    (root / "release-review").touch()
                    def complete(data):
                        states = {w["workId"]: w["state"]["name"] for w in data.get("results", [])}
                        return all(states.get(key) == "complete" for key in ("new-idea", "new-dependent", "new-loopback"))
                    final = await_merged_probe(self, process, url, complete, log)
                    states = {w["workId"]: w["state"]["name"] for w in final["results"]}
                    for identity in ("old-idea", "old-dependent", "old-loopback"):
                        self.assertEqual(states[identity], "failed")
                    self.assertEqual(states["healthy"], "to-complete")
                    self.assertEqual(states["parked"], "awaiting-answer")
                    self.assertEqual(sum(w["workId"] == "new-idea" for w in final["results"]), 1)
                    self.assertEqual((root / "process-count").read_text(), "2")
                    self.assertEqual((root / "review-count").read_text(), "2")
                    for name, contents in saved.items():
                        self.assertEqual((retained / name).read_bytes(), contents)
                    self.assertEqual(json.loads((retained / "tasks/todo/lane-r2.json").read_text())["context"]["recovery"], recovery)
                    events = merged_probe_events(base + f"/factory-sessions/{session}/events")
                    for transition in ("release-dependent", "release-loopback"):
                        self.assertEqual(sum(e["type"] == "DISPATCH_REQUEST" and
                            e["payload"].get("transitionId") == transition for e in events), 1)
                    self.assertFalse(any("CONTROL" in e["type"] or "OVERRIDE" in e["type"] for e in events))
                finally:
                    process.terminate()
                    try:
                        process.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=10)

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
            selected = {"escalate-plan-failure", "escalate-task-failure", "consume", "report-idea-failure",
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


RECOVERY_WORKER = r'''
import json, sys, time, urllib.request
from pathlib import Path
root, stage, session, name, tag = sys.argv[1:]
root = Path(root)
base = (root / 'base.txt').read_text()
recovery = json.loads((root / 'recovery.json').read_text())
def submit():
    tags = {'project': 'fixture-project', 'recovery-worktree': recovery['workspace']['worktree']}
    works = [{'name': 'lane-r2', 'workId': 'new-idea', 'workTypeName': 'idea', 'state': 'init',
              'tags': tags, 'payload': {'recovery': recovery}},
             {'name': 'dependent-r2', 'workId': 'new-dependent', 'workTypeName': 'validation', 'state': 'init'},
             {'name': 'loopback-r2', 'workId': 'new-loopback', 'workTypeName': 'project-cycle', 'state': 'init'}]
    body = {'type': 'FACTORY_REQUEST_BATCH', 'requestId': 'corrected-successor', 'works': works,
            'relations': [{'type': 'DEPENDS_ON', 'sourceWorkName': 'dependent-r2', 'targetWorkId': 'new-idea', 'requiredState': 'complete'},
                          {'type': 'DEPENDS_ON', 'sourceWorkName': 'loopback-r2', 'targetWorkId': 'new-dependent', 'requiredState': 'complete'}]}
    req = urllib.request.Request(base + '/factory-sessions/' + session + '/work-requests/corrected-successor',
        data=json.dumps(body).encode(), headers={'Content-Type':'application/json'}, method='PUT')
    with urllib.request.urlopen(req, timeout=15) as response:
        response.read()
if stage == 'project-lead-wake':
    if not (root / 'admitted').exists():
        submit()
        # The same unchanged request ID must coalesce, not create a new owner.
        submit()
        (root / 'admitted').touch()
elif stage in ('plan', 'setup-workspace'):
    assert tag == recovery['workspace']['worktree'], (stage, tag)
    packet = root / tag / 'tasks/todo/lane-r2.json'
    packet.parent.mkdir(parents=True, exist_ok=True)
    packet.write_text(json.dumps({'project':'lane-r2', 'context':{'recovery':recovery}}))
elif stage in ('process', 'review'):
    assert Path.cwd() == root / tag, (stage, str(Path.cwd()), tag)
    packet = json.loads((root / tag / 'tasks/todo/lane-r2.json').read_text())
    assert packet['context']['recovery'] == recovery
    assert packet['context']['recovery']['workspace']['prUrl'].endswith('/99')
    counter = root / (stage + '-count')
    count = int(counter.read_text()) + 1 if counter.exists() else 1
    counter.write_text(str(count))
    if stage == 'review' and count == 1:
        (root / 'review-entered').touch()
        deadline = time.monotonic() + 35
        while not (root / 'release-review').exists():
            assert time.monotonic() < deadline, 'review gate not released'
            # Real executable/file edge; no in-process event hook exists.
            time.sleep(.05)
        print(json.dumps({'decision':'REJECTED','feedback':'controlled review correction','output':''}))
        sys.exit(0)
elif stage == 'ci-wait':
    print('review')
    sys.exit(0)
print(json.dumps({'decision':'ACCEPTED','feedback':'controlled recovery decision','output':''}))
'''

RECOVERY_PROVIDER = r'''
import contextlib, io, json, os, re, runpy, sys
from pathlib import Path
if '--version' in sys.argv:
    print('codex-cli 0.0.1')
    sys.exit(0)
prompt = sys.stdin.read()
root = Path(os.environ['RECOVERY_FIXTURE_ROOT'])
(root / 'provider-prompt.txt').write_text(prompt + '\nARGV\n' + repr(sys.argv))
if 'RECOVERY_FIXTURE' not in prompt:
    prompt += '\n'.join(sys.argv)
stage, session, name = re.search(r'RECOVERY_FIXTURE (\S+) (\S+) (\S+)', prompt).groups()
tag = re.search(r'RECOVERY_TAG[ \t]*([^\r\n]*)', prompt).group(1)
sys.argv = ['worker.py', str(root), stage, session, name, tag]
output = io.StringIO()
with contextlib.redirect_stdout(output):
    try:
        runpy.run_path(str(root / 'worker.py'), run_name='__main__')
    except SystemExit as result:
        if result.code:
            raise
text = output.getvalue().strip()
print(json.dumps({'type':'thread.started','thread_id':'fixture-recovery'}))
print(json.dumps({'type':'item.completed','item':{'id':'result','type':'agent_message','text':text}}))
print(json.dumps({'type':'turn.completed','usage':{'input_tokens':1,'output_tokens':1}}))
'''


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
    detail = ''
    for runtime_log in log_path_logs(log):
        for line in runtime_log.read_text(errors='replace').splitlines():
            try:
                entry = json.loads(line)
            except ValueError:
                continue
            if entry.get('level') == 'error':
                detail += json.dumps({key: value for key, value in entry.items()
                                      if key != 'stacktrace'}) + '\n'
    for prompt in Path(log.name).parent.glob('provider-prompt.txt'):
        detail += prompt.read_text()[-4000:]
    test.fail(f"artifact probe did not complete: observed={observed!r}; host={log.read()}; runtime={detail}")


def log_path_logs(log):
    return Path(log.name).parent.rglob('*runtime-log*.log')


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
