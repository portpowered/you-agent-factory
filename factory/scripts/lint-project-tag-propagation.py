#!/usr/bin/env python3
"""Check declared Work outputs for an effective input-tag inheritance path."""

import json
import sys
from pathlib import Path


def objects(value, location):
    if not isinstance(value, list) or any(not isinstance(item, dict) for item in value):
        raise ValueError(f"{location}: expected an array of objects")
    return value


def name(value, location):
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{location}: expected a nonempty name")
    return value


def check(document):
    """Return findings and audited output count; malformed metadata fails closed."""
    if not isinstance(document, dict):
        raise ValueError("Factory: expected an object")
    resources = {name(r.get("name"), "resource.name") for r in objects(document.get("resources", []), "resources")}
    work_types = {name(w.get("name"), "workType.name") for w in objects(document.get("workTypes"), "workTypes")}
    findings, count = [], 0
    for station in objects(document.get("workstations"), "workstations"):
        station_id = name(station.get("id"), "workstation.id")
        inputs = io_types(station.get("inputs", []), station_id + ".inputs", work_types, resources)
        work_inputs = inputs - resources
        policy = station.get("workPropagation", {})
        if not isinstance(policy, dict):
            raise ValueError(f"{station_id}: workPropagation must be an object")
        mode = policy.get("mode", "OUTPUT_AS_PAYLOAD")
        if mode not in ("PRESERVE_INPUT", "OUTPUT_AS_PAYLOAD"):
            raise ValueError(f"{station_id}: unsupported workPropagation mode {mode!r}")
        outcomes = [(key, station.get(key, [])) for key in ("outputs", "onFailure", "onRejection", "onContinue")]
        for route in objects(station.get("classificationRoutes", []), station_id + ".classificationRoutes"):
            label = name(route.get("label"), station_id + ".classificationRoutes.label")
            outcomes.append((f"classificationRoutes[{label}].outputs", route.get("outputs")))
        for outcome, outputs in outcomes:
            for index, output in enumerate(objects(outputs, f"{station_id}.{outcome}")):
                location = f"{station_id}.{outcome}[{index}]"
                target = next(iter(io_types([output], location, work_types, resources)))
                if target in resources:
                    continue
                count += 1
                if not inputs:  # A source has no inherited tags to preserve.
                    continue
                if target in work_inputs:
                    continue  # Same-type runtime outputs clone the matching input.
                if mode == "PRESERVE_INPUT" and work_inputs:
                    continue  # Cross-type outputs inherit the primary Work input.
                findings.append(f"{location} ({target}): input tags are lost; require PRESERVE_INPUT with a consumed Work input")
    return findings, count


def io_types(values, location, work_types, resources):
    result = set()
    for value in objects(values, location):
        target = name(value.get("workType"), location + ".workType")
        if target not in work_types | resources:
            raise ValueError(f"{location}: unknown workType {target!r}")
        name(value.get("state"), location + ".state")
        result.add(target)
    return result


def main(args):
    try:
        if len(args) != 1:
            raise ValueError("usage: lint-project-tag-propagation.py <factory.json>")
        findings, count = check(json.loads(Path(args[0]).read_text(encoding="utf-8")))
        for finding in findings:
            print(finding, file=sys.stderr)
        print(f"Project tag propagation: {count} Work outputs audited, {len(findings)} tag-loss findings")
        print(f"LINT_VIOLATION_COUNT: {len(findings)}")
        return int(bool(findings))
    except (OSError, ValueError) as error:
        print(f"Project tag propagation: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
