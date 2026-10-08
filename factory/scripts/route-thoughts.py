#!/usr/bin/env python3
"""Select mission execution without changing ordinary thoughts."""

import json
import sys


def route_thoughts(payload):
    try:
        value = json.loads(payload)
    except (ValueError, TypeError):
        return "supervision"
    if not isinstance(value, dict) or "mission" not in value:
        return "supervision"
    if not isinstance(value["mission"], str):
        raise ValueError("thoughts.mission must be a string")
    return "mission" if value["mission"].strip() else "supervision"


def main(argv=None):
    args = sys.argv[1:] if argv is None else argv
    if len(args) != 1:
        print("usage: route-thoughts.py <payload>", file=sys.stderr)
        return 2
    try:
        print(route_thoughts(args[0]))
    except ValueError as error:
        print(error, file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
