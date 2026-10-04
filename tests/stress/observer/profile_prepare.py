"""Compile once and attest the dedicated observer handoff (never measure)."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def build_profile(source, output, tool, manifest, files, mode):
    artifact = output / ("observer.test.exe" if os.name == "nt" else "observer.test")
    command = ["go", "test", "-c", "-overlay", str(output / "overlay.json"),
               "-p", "1", "-o", str(artifact), "./tests/stress/observer"]
    # Preparation alone uses the already cached auto toolchain. Runtime never
    # invokes Go, and receives fresh owned cache/config paths recorded below.
    env = dict(os.environ, GOPROXY="off")
    with (output / "build.stdout.txt").open("w") as stdout, (output / "build.stderr.txt").open("w") as stderr:
        result = subprocess.run(command, cwd=source, env=env, stdout=stdout, stderr=stderr)
    (output / "build.exit.txt").write_text(str(result.returncode) + "\n")
    if result.returncode:
        raise ValueError("profile build failed; see " + str(output / "build.stderr.txt"))
    build_info = subprocess.check_output(["go", "version", "-m", str(artifact)], text=True)
    home = output / "runtime" / "home"
    paths = {key: str(output / "runtime" / key.lower()) for key in [
        "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "GOCACHE",
        "GOPATH", "GOMODCACHE", "GOTMPDIR", "TEMP", "TMP"]}
    paths.update(HOME=str(home), USERPROFILE=str(home),
                 HOMEDRIVE=home.drive or "/", HOMEPATH=str(home)[len(home.drive):])
    for key, value in paths.items():
        if key not in ("HOMEDRIVE", "HOMEPATH"):
            Path(value).mkdir(parents=True, exist_ok=True)
    tool_files = [{"path": str(path), "sha256": sha(path)} for path in sorted(tool.rglob("*"))
                  if path.is_file() and "__pycache__" not in path.parts]
    identity = dict(sourceCommit=manifest["sourceCommit"],
                    sourceTree=subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD^{tree}"], text=True).strip(),
                    sourceStatus=manifest["sourceStatus"], toolCommit=manifest["toolSourceCommit"],
                    toolStatus="", toolFiles=tool_files, overlayPath=str(output / "overlay.json"),
                    overlaySHA256=sha(output / "overlay.json"),
                    backingFiles=[dict(virtual=f["virtual"], path=f["backing"], sha256=f["sha256"]) for f in files],
                    preparationManifestPath=str(output / "manifest.json"),
                    preparationManifestSHA256=sha(output / "manifest.json"), artifactPath=str(artifact),
                    artifactSHA256=sha(artifact), goVersion=manifest["goVersion"], buildInfo=build_info,
                    buildCommand=subprocess.list2cmdline(command), mode=mode, race=False)
    instrumentation = dict(registration="one callback per real owner constructor",
                           retainedCallbacks="all owners retained through report write; no state mutation",
                           snapshotLocking="collector unlocks before each owner lock; detached copy; not atomic across owners",
                           sortingAndAllocation="copy under owner lock; sorting and capture outside spans",
                           serialization="outside timed spans",
                           timer="Go time.Now/time.Since monotonic duration in nanoseconds",
                           median="sorted; even N midpoint arithmetic mean",
                           p95="nearest-rank ceil(0.95*N), one-based")
    for schema in tool.glob("testdata/*.schema.json"):
        shutil.copyfile(schema, output / schema.name)
    report = dict(schemaVersion=1, identity=identity, childPaths=paths,
                  fixtureSHA256=sha(tool / "testdata/lifecycle-tests.go.txt"),
                  methodSHA256=sha(tool / "testdata/profile-tests.go.txt"), instrumentation=instrumentation,
                  profileCommand="unsupported until joined cycles and composed capacity are implemented",
                  capabilityCommand=subprocess.list2cmdline([str(artifact), "-test.run=^TestLifecycleProfileSamplesCapability$", "-test.short=false", "-test.count=1", "-test.timeout=5m", "-test.v"]))
    (output / "profile-manifest.json").write_text(json.dumps(report, indent=2) + "\n")
