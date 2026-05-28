#!/usr/bin/env python3
"""setup-workspace.py — Create or reuse a git worktree for a PRD.

Usage: python scripts/agents/setup-workspace.py <prd-name>
"""

import json

import shutil
import subprocess
import sys
from pathlib import Path


def main():
    print("Setting up workspace...", file=sys.stderr)


if __name__ == "__main__":
    main()
