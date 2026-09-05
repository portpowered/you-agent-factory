#!/usr/bin/env python3
"""Stage verified artifacts and a private environment for a fresh probe."""

from __future__ import annotations

import hashlib
import importlib.util
import json
from pathlib import Path
import re
import shutil
import sys


def _load_packet_preflight():
    """Load the setup-owned validator without executing its CLI entrypoint."""
    script = Path(__file__).with_name("setup-workspace.py")
    spec = importlib.util.spec_from_file_location(
        "factory_setup_workspace_preflight", script,
    )
    if spec is None or spec.loader is None:
        raise RuntimeError("could not load the packet preflight validator")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


_PACKET_PREFLIGHT = _load_packet_preflight()
PacketPreflightError = _PACKET_PREFLIGHT.PacketPreflightError


MISSION_FIELDS = frozenset(
    {
        "role",
        "project",
        "mission",
        "criteria",
        "reportPath",
        "budget",
        "preflight",
        "build",
        "fixtures",
        "publicDocs",
    }
)


def artifact(value: object) -> dict:
    if not isinstance(value, dict) or set(value) - {"identity", "path", "sha256"}:
        raise ValueError("artifacts require identity, absolute path and sha256 only")
    source = Path(value.get("path", ""))
    if not source.is_absolute() or not source.is_file():
        raise ValueError("artifact path must be an existing absolute file")
    if not re.fullmatch(r"[0-9a-fA-F]{64}", str(value.get("sha256", ""))):
        raise ValueError("artifact requires an exact SHA-256")
    return value


def _staging_error(field, value, destination, code, *, observed):
    """Return one bounded staging diagnostic without exposing source bytes."""
    return _PACKET_PREFLIGHT.packet_preflight_error(
        "artifact-staging",
        code,
        field,
        path=str(destination),
        identity=value.get("identity"),
        expected=value.get("sha256"),
        observed=observed,
    )


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
        digest = _PACKET_PREFLIGHT.stream_file_sha256(destination)
    except _PACKET_PREFLIGHT.FileReadFailure as error:
        raise _staging_error(
            field,
            value,
            destination,
            "input-read",
            observed="partial" if error.position else "unreadable",
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


def _sanitized_preflight(packet_digest, envelope, verified_files, mainline):
    """Persist only bounded identities; never persist authority paths."""
    authority = [
        {
            "field": record["field"],
            "identity": record["identity"],
            "sha256": record["observedSha256"],
        }
        for record in verified_files
        if record["field"].startswith("preflight.authority.")
    ]
    return {
        "version": _PACKET_PREFLIGHT.PREFLIGHT_VERSION,
        "status": "verified",
        "packet": {
            "identity": f"sha256:{packet_digest}",
            "sha256": packet_digest,
        },
        "projectIdentity": envelope["projectIdentity"],
        "contractRevision": envelope["contractRevision"],
        "verifiedAuthority": authority,
        "intendedMainline": mainline,
    }


def _validate_preflight(request, packet_digest, root):
    """Validate all v1 inputs before the destination directory exists."""
    envelope = _PACKET_PREFLIGHT.validate_preflight_contract(request)
    verified_files = _PACKET_PREFLIGHT.validate_preflight_files(envelope)
    mainline = _PACKET_PREFLIGHT.validate_intended_mainline(
        root, envelope["intendedMainline"],
    )
    return envelope, _sanitized_preflight(
        packet_digest, envelope, verified_files, mainline,
    )


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
    payload_bytes = payload.encode("utf-8") if isinstance(payload, str) else payload
    packet_digest = hashlib.sha256(payload_bytes).hexdigest()
    envelope, preflight = _validate_preflight(request, packet_digest, root)
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
    if build is not None and not build.get("identity"):
        raise ValueError("build requires an immutable identity")
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
    request["preflight"] = preflight
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
    except (ValueError, OSError, TypeError, PacketPreflightError) as error:
        print(f"validation admission failed: {error}", file=sys.stderr)
        return 2
    print("validation workspace ready")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
