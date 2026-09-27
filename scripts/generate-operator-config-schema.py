#!/usr/bin/env python3
"""Generate the MCP operator config schema from the bundled OpenAPI contract."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
OPENAPI = ROOT / "api" / "openapi.yaml"
OUTPUT = ROOT / "pkg" / "transports" / "mcp" / "content" / "operator-config.schema.json"
ROOT_SCHEMA = "GlobalConfig"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true", help="fail if the generated file is stale")
    args = parser.parse_args()

    document = yaml.safe_load(OPENAPI.read_text(encoding="utf-8"))
    schemas = document["components"]["schemas"]
    definitions: dict[str, object] = {}
    pending = [ROOT_SCHEMA]
    while pending:
        name = pending.pop()
        if name in definitions:
            continue
        schema = rewrite_refs(schemas[name])
        definitions[name] = schema
        refs = list(references(schema))
        for ref in refs:
            prefix = "#/$defs/"
            if not ref.startswith(prefix):
                raise ValueError(f"unsupported schema reference: {ref}")
            pending.append(ref[len(prefix) :])

    root = definitions.pop(ROOT_SCHEMA)
    result = {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "$id": "you://operator/config/schema",
        "title": "Operator configuration",
        **root,
        "$defs": definitions,
    }
    rendered = json.dumps(result, indent=2, ensure_ascii=False) + "\n"
    if args.check:
        if not OUTPUT.exists() or OUTPUT.read_text(encoding="utf-8") != rendered:
            print(f"{OUTPUT.relative_to(ROOT)} is stale; run {Path(__file__).name}")
            return 1
        return 0

    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    OUTPUT.write_text(rendered, encoding="utf-8")
    return 0


def references(value: object):
    if isinstance(value, dict):
        ref = value.get("$ref")
        if isinstance(ref, str):
            yield ref
        for child in value.values():
            yield from references(child)
    elif isinstance(value, list):
        for child in value:
            yield from references(child)


def rewrite_refs(value: object):
    if isinstance(value, dict):
        result = {}
        for key, child in value.items():
            if key == "$ref" and isinstance(child, str) and child.startswith("#/components/schemas/"):
                result[key] = "#/$defs/" + child.removeprefix("#/components/schemas/")
            else:
                result[key] = rewrite_refs(child)
        return result
    if isinstance(value, list):
        return [rewrite_refs(child) for child in value]
    return value


if __name__ == "__main__":
    raise SystemExit(main())
