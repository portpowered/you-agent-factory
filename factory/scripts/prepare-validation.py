#!/usr/bin/env python3
"""Stage verified artifacts and a private environment for a fresh probe."""

from __future__ import annotations

import hashlib
import json
import re
import shutil
import sys
from pathlib import Path

MISSION_FIELDS = frozenset(
    {
        "role",
        "project",
        "mission",
        "criteria",
        "reportPath",
        "budget",
        "build",
        "fixtures",
        "publicDocs",
    }
)


def artifact(value: object) -> dict:
    required = {"identity", "path", "sha256"}
    if not isinstance(value, dict) or set(value) != required:
        raise ValueError("artifacts require identity, absolute path and sha256 only")
    if not isinstance(value["identity"], str) or not value["identity"]:
        raise ValueError("artifact requires an immutable identity")
    source = Path(value.get("path", ""))
    if not source.is_absolute() or not source.is_file():
        raise ValueError("artifact path must be an existing absolute file")
    if not re.fullmatch(r"[0-9a-fA-F]{64}", str(value.get("sha256", ""))):
        raise ValueError("artifact requires an exact SHA-256")
    return value


def _staging_error(field, value, destination, code, *, observed):
    """Return one bounded staging diagnostic without exposing source bytes."""
    return ValueError(
        f"artifact-staging code={code} field={field} "
        f"path={destination} expected={value.get('sha256')} observed={observed}"
    )


def stream_file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def stage(value: dict, destination: Path, field="artifact") -> dict:
    """Copy one already-admitted artifact and verify its private copy."""
    try:
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(value["path"], destination)
    except FileNotFoundError as error:
        raise _staging_error(
            field, value, destination, "missing-input", observed="missing",
        ) from error
    except (OSError, ValueError, TypeError) as error:
        raise _staging_error(
            field, value, destination, "input-read", observed="unavailable",
        ) from error

    try:
        digest = stream_file_sha256(destination)
    except OSError as error:
        raise _staging_error(
            field,
            value,
            destination,
            "input-read",
            observed="unreadable",
        ) from error
    if digest.lower() != value["sha256"].lower():
        raise _staging_error(
            field,
            value,
            destination,
            "digest-mismatch",
            observed=digest,
        )
    return {
        "path": str(destination),
        "sha256": digest,
        "identity": value["identity"],
    }


def _write_mission(target, request):
    """Publish mission.json atomically after all staging has succeeded."""
    partial = target / "mission.json.partial"
    mission = target / "mission.json"
    partial.write_text(json.dumps(request, indent=2) + "\n", encoding="utf-8")
    partial.replace(mission)


def prepare(root: Path, name: str, payload: str) -> Path:
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,150}", name):
        raise ValueError("validation name must be a safe, unique directory name")
    request = json.loads(payload)
    if not isinstance(request, dict) or set(request) - MISSION_FIELDS:
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
    if target.exists() or target.is_symlink():
        raise ValueError("validation workspace already exists; use a new name")
    if not target.resolve().is_relative_to(probes.resolve()):
        raise ValueError("probe workspace escapes probes; use a new name")
    build = request.get("build")
    if request["role"] != "retrospective" and build is None:
        raise ValueError("customer and engineering validation require a prebuilt artifact")
    if build is not None:
        request["build"] = artifact(build)
    for field in ("fixtures", "publicDocs"):
        values = request.get(field, [])
        if not isinstance(values, list):
            raise TypeError(f"{field} must be an array of artifacts")
        request[field] = [artifact(value) for value in values]
    # Atomic creation rejects repeated/concurrent admission. A failed staging
    # attempt leaves its evidence and requires a fresh validation Work name.
    try:
        target.mkdir(parents=True, exist_ok=False)
    except FileExistsError as error:
        raise ValueError(
            "validation workspace already exists; use a new name"
        ) from error
    if build is not None:
        request["build"] = stage(
            build, target / "bin" / Path(build["path"]).name, "build",
        )
    for field in ("fixtures", "publicDocs"):
        request[field] = [
            stage(
                value,
                target / field / str(i) / Path(value["path"]).name,
                f"{field}[{i}]",
            )
            for i, value in enumerate(request.get(field, []))
        ]
    environment = {}
    for key, folder in (("HOME", "profile"), ("USERPROFILE", "profile"),
                        ("XDG_CACHE_HOME", "cache"), ("XDG_CONFIG_HOME", "config"),
                        ("APPDATA", "appdata"), ("LOCALAPPDATA", "localappdata")):
        location = target / folder
        location.mkdir(exist_ok=True)
        environment[key] = str(location)
    request["environment"] = environment
    request["reportPath"] = str(report)
    report.parent.mkdir(parents=True, exist_ok=True)
    _write_mission(target, request)
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
