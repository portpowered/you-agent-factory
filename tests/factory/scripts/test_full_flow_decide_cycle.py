#!/usr/bin/env python3
"""Regression tests for the packaged full-flow cycle routing boundary."""

import importlib.util
import sys
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = (
    REPO_ROOT
    / "packages"
    / "packaged-factories"
    / "factories"
    / "full-flow"
    / "scripts"
    / "decide-cycle.py"
)
SPEC = importlib.util.spec_from_file_location("decide_cycle", SCRIPT_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.dont_write_bytecode = True
SPEC.loader.exec_module(MODULE)


class FullFlowDecideCycleTest(unittest.TestCase):
    def test_accepts_only_authored_routes(self):
        self.assertEqual(MODULE.decide_cycle(" continue \n"), "continue")
        self.assertEqual(MODULE.decide_cycle("COMPLETE"), "complete")

    def test_rejects_prose_even_when_it_mentions_completion(self):
        with self.assertRaisesRegex(ValueError, "exactly 'continue' or 'complete'"):
            MODULE.decide_cycle("Completed the remaining full-flow work.")


if __name__ == "__main__":
    unittest.main()
