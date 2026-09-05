#!/usr/bin/env python3
"""Stage verified artifacts and a private environment for a fresh probe."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
import re
import shutil
import sys


def artifact(value: object) -> dict:
    if not isinstance(value, dict) or set(value) - {"identity", "path", "sha256"}:
        raise ValueError("artifacts require identity, absolute path and sha256 only")
    source = Path(value.get("path", ""))
    if not source.is_absolute() or not source.is_file():
        raise ValueError("artifact path must be an existing absolute file")
    if not re.fullmatch(r"[0-9a-fA-F]{64}", str(value.get("sha256", ""))):
        raise ValueError("artifact requires an exact SHA-256")
    return value


def stage(value: dict, destination: Path) -> dict:
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(value["path"], destination)
    with destination.open("rb") as stream:
        digest = hashlib.file_digest(stream, "sha256").hexdigest()
    if digest.lower() != value["sha256"].lower():
        raise ValueError("staged artifact SHA-256 does not match the mission")
    return {"path": str(destination), "sha256": digest,
            "identity": value.get("identity", digest)}


def prepare(root: Path, name: str, payload: str) -> Path:
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,150}", name):
        raise ValueError("validation name must be a safe, unique directory name")
    request = json.loads(payload)
    allowed = {"role", "project", "mission", "criteria", "reportPath", "budget",
               "build", "fixtures", "publicDocs"}
    if not isinstance(request, dict) or set(request) - allowed:
        raise ValueError("validation payload must contain only mission fields")
    if request.get("role") not in {"customer", "engineering", "retrospective"}:
        raise ValueError("validation role must be customer, engineering, or retrospective")
    for field in ("project", "mission", "criteria", "reportPath", "budget"):
        if not request.get(field):
            raise ValueError(f"validation payload requires {field}")
    if not isinstance(request["criteria"], list) or any(
        not isinstance(c, dict) or set(c) != {"id", "rubric"} or not c["id"] or not c["rubric"]
        for c in request["criteria"]
    ):
        raise ValueError("criteria require IDs and observable rubrics")
    if not isinstance(request["budget"], dict) or any(
        request["budget"].get(key) in (None, "") for key in ("time", "download", "disk", "process", "paid")
    ):
        raise ValueError("budget requires time, download, disk, process and paid bounds")
    root = root.resolve()
    projects = (root / "docs/temp/projects").resolve()
    report = Path(request["reportPath"])
    if not report.is_absolute():
        raise ValueError("reportPath must be absolute")
    report = report.resolve()
    if not projects.is_relative_to(root) or not report.is_relative_to(projects) or report.suffix != ".md":
        raise ValueError("reportPath must be a Markdown report under docs/temp/projects")
    if report.exists():
        raise ValueError("reportPath already exists; use a new report name")
    probes = root / "docs/temp/probes"
    if not probes.resolve().is_relative_to(root):
        raise ValueError("probe root escapes workspace")
    target = probes / name
    if target.exists() or target.is_symlink() or not target.resolve().is_relative_to(probes.resolve()):
        raise ValueError("probe workspace already exists or escapes probes; use a new name")
    build = request.get("build")
    if request["role"] != "retrospective" and build is None:
        raise ValueError("customer and engineering validation require a prebuilt artifact")
    if build is not None:
        artifact(build)
        if not build.get("identity"):
            raise ValueError("build requires an immutable identity")
    for field in ("fixtures", "publicDocs"):
        if not isinstance(request.get(field, []), list):
            raise ValueError(f"{field} must be an artifact list")
        for value in request.get(field, []):
            artifact(value)
    # Atomic creation rejects repeated/concurrent admission. A failed staging
    # attempt leaves its evidence and requires a fresh validation Work name.
    target.mkdir(parents=True, exist_ok=False)
    if build is not None:
        request["build"] = stage(build, target / "bin" / Path(build["path"]).name)
    for field in ("fixtures", "publicDocs"):
        request[field] = [stage(value, target / field / str(i) / Path(value["path"]).name)
                          for i, value in enumerate(request.get(field, []))]
    environment = {}
    for key, folder in (("HOME", "profile"), ("USERPROFILE", "profile"),
                        ("XDG_CACHE_HOME", "cache"), ("XDG_CONFIG_HOME", "config"),
                        ("APPDATA", "appdata"), ("LOCALAPPDATA", "localappdata")):
        location = target / folder
        location.mkdir(exist_ok=True)
        environment[key] = str(location)
    request["environment"] = environment
    request["reportPath"] = str(report)
    (target / "mission.json").write_text(json.dumps(request, indent=2) + "\n", encoding="utf-8")
    report.parent.mkdir(parents=True, exist_ok=True)
    return target


def main() -> int:
    if len(sys.argv) != 3:
        print("usage: prepare-validation.py <name> <payload-json>", file=sys.stderr)
        return 2
    try:
        prepare(Path.cwd(), sys.argv[1], sys.argv[2])
    except (ValueError, OSError, TypeError) as error:
        print(f"validation admission failed: {error}", file=sys.stderr)
        return 2
    print("validation workspace ready")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
