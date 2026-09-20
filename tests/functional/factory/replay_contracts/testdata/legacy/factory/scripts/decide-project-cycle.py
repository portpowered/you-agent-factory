#!/usr/bin/env python3
"""Route the Project Lead's explicit cycle decision."""

from __future__ import annotations

import sys


VALID_DECISIONS = {"continue", "complete", "blocked"}


def decide_project_cycle(payload: str) -> str:
    decision = payload.strip().lower()
    if decision not in VALID_DECISIONS:
        raise ValueError(
            "project-cycle payload must be exactly 'continue', 'complete', or 'blocked'"
        )
    return decision


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print(
            "usage: decide-project-cycle.py <continue|complete|blocked>",
            file=sys.stderr,
        )
        return 2
    try:
        decision = decide_project_cycle(argv[1])
    except ValueError as error:
        print(error, file=sys.stderr)
        return 2
    print(decision)
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
