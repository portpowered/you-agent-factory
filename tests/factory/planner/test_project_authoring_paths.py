#!/usr/bin/env python3
"""CASE-SP-016 checks for the Project authoring path contract."""

from __future__ import annotations

import json
import os
import re
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[3]
PROJECT_LEAD_PROMPT = (
    REPO_ROOT / "factory" / "workstations" / "project-lead" / "AGENTS.md"
)
PROJECTS_GUIDE = REPO_ROOT / "factory" / "docs" / "projects.md"
WINDOWS_DRIVE_ABSOLUTE = re.compile(r"^[A-Za-z]:[\\/]")


def _json_code_block_after(text: str, heading: str) -> dict:
    start = text.index(heading)
    block_start = text.index("```json", start) + len("```json")
    block_end = text.index("```", block_start)
    return json.loads(text[block_start:block_end].strip())


def _is_absolute_cross_stage_path(value: str) -> bool:
    return bool(WINDOWS_DRIVE_ABSOLUTE.match(value)) or os.path.isabs(value)


class ProjectAuthoringPathsTest(unittest.TestCase):
    def test_case_sp_016_minimal_admission_teaches_absolute_references(self):
        guide = PROJECTS_GUIDE.read_text(encoding="utf-8")
        admission = _json_code_block_after(guide, "Minimal admission")
        payload = admission["works"][0]["payload"]

        self.assertTrue(_is_absolute_cross_stage_path(payload["sourcePlan"]))
        self.assertTrue(_is_absolute_cross_stage_path(payload["projectRoot"]))
        self.assertFalse(payload["sourcePlan"].startswith("docs/temp/"))
        self.assertFalse(payload["projectRoot"].startswith("docs/temp/"))

    def test_case_sp_016_project_lead_idea_example_emits_absolute_source_plan(self):
        prompt = PROJECT_LEAD_PROMPT.read_text(encoding="utf-8")
        batch = _json_code_block_after(prompt, "When work remains")
        idea = next(
            work
            for work in batch["request"]["works"]
            if work["workTypeName"] == "idea"
        )

        source_plan = idea["payload"]["sourcePlan"]
        self.assertTrue(_is_absolute_cross_stage_path(source_plan))
        self.assertTrue(WINDOWS_DRIVE_ABSOLUTE.match(source_plan))
        self.assertNotIn("path/from/project/request", source_plan)

    def test_case_sp_016_prompt_agrees_on_resolution_preservation_defaults_and_errors(self):
        prompt = PROJECT_LEAD_PROMPT.read_text(encoding="utf-8")
        for required in (
            "^[A-Za-z]:[\\\\/]",
            "git rev-parse --show-toplevel",
            "Preserve an absolute `sourcePlan`",
            "Every emitted idea must carry the resolved absolute `sourcePlan` value",
            "emit a `blocked` Project cycle",
            "existing authorized-workspace policy",
        ):
            self.assertIn(required, prompt)

    def test_case_sp_016_absolute_reference_is_read_after_consumer_cwd_changes(self):
        content = b"project authoring fixture bytes\n"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            source = root / "docs" / "temp" / "project-plan.md"
            consumer = root / "lane-worktree"
            source.parent.mkdir(parents=True)
            consumer.mkdir()
            source.write_bytes(content)

            previous_cwd = Path.cwd()
            try:
                os.chdir(consumer)
                self.assertEqual(Path(str(source)).read_bytes(), content)
            finally:
                os.chdir(previous_cwd)


if __name__ == "__main__":
    unittest.main()
