#!/usr/bin/env python3
"""Emit the planner's authored full-flow cycle route without model ambiguity."""

from __future__ import annotations

import sys


VALID_DECISIONS = {"continue", "complete"}


def decide_cycle(payload: str) -> str:
    decision = payload.strip().lower()
    if decision not in VALID_DECISIONS:
        raise ValueError("cycle-control payload must be exactly 'continue' or 'complete'")
    return decision


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print("usage: decide-cycle.py <continue|complete>", file=sys.stderr)
        return 2
    try:
        decision = decide_cycle(argv[1])
    except ValueError as error:
        print(error, file=sys.stderr)
        return 2
    print(decision)
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
