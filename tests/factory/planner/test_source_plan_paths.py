#!/usr/bin/env python3
"""Deterministic behavior matrix for the planner source-plan contract."""

from __future__ import annotations

import importlib.util
import json
import ntpath
import os
import subprocess
import sys
import tempfile
import threading
import unittest
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from unittest.mock import Mock


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "tests" / "factory" / "planner" / "source_plan_paths_functional.py"
SPEC = importlib.util.spec_from_file_location("source_plan_paths_functional", SCRIPT_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


FIXTURE_BYTES = b"unique source-plan fixture\nbytes must remain exact\n"


class SourcePlanPathsTest(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name).resolve()
        self.source = self.root / "docs" / "temp" / "fixture.md"
        self.source.parent.mkdir(parents=True)
        self.source.write_bytes(FIXTURE_BYTES)

    def tearDown(self):
        self.temp_dir.cleanup()

    def test_case_sp_001_and_002_preserve_both_windows_absolute_spellings(self):
        logical_root = r"C:\workspace\planner"
        logical_source = logical_root + r"\docs\temp\fixture.md"
        files = {
            ntpath.normcase(ntpath.normpath(logical_source)): FIXTURE_BYTES,
        }

        def regular_file(path):
            return ntpath.normcase(ntpath.normpath(path)) in files

        def read_file(path):
            return files[ntpath.normcase(ntpath.normpath(path))]

        for raw_source_plan in (
            logical_source,
            logical_source.replace("\\", "/"),
        ):
            with self.subTest(raw_source_plan=raw_source_plan):
                self.assertTrue(MODULE.is_windows_drive_absolute(raw_source_plan))
                resolution = MODULE.resolve_source_plan(
                    raw_source_plan,
                    logical_root,
                    authorized_roots=[logical_root],
                    is_regular_file=regular_file,
                    read_file=read_file,
                )
                self.assertIsNotNone(resolution)
                assert resolution is not None
                self.assertEqual(resolution.raw_source_plan, raw_source_plan)
                self.assertEqual(resolution.persisted_source_plan, raw_source_plan)
                self.assertEqual(resolution.content, FIXTURE_BYTES)

                packet = MODULE.write_packet(self.root / raw_source_plan.replace("\\", "_"), resolution)
                MODULE.validate_packet(
                    packet,
                    expected_raw_source_plan=raw_source_plan,
                    expected_persisted_source_plan=raw_source_plan,
                    expected_content=FIXTURE_BYTES,
                    consumer_cwd=self.root,
                    read_file=read_file,
                )

    def test_case_sp_003_resolves_relative_input_from_repository_root(self):
        relative = "docs/temp/fixture.md"
        resolution, packet = MODULE.plan_and_write(self.root / "relative", relative, self.root)
        self.assertIsNotNone(resolution)
        assert resolution is not None
        expected = str(self.source)
        self.assertEqual(resolution.persisted_source_plan, expected)
        MODULE.validate_packet(
            packet,
            expected_raw_source_plan=relative,
            expected_persisted_source_plan=expected,
            expected_content=FIXTURE_BYTES,
            consumer_cwd=self.root,
        )

    def test_case_sp_004_rejects_empty_source_plan_without_packet(self):
        output = self.root / "empty"
        with self.assertRaisesRegex(MODULE.SourcePlanError, "must not be empty"):
            MODULE.plan_and_write(output, "", self.root)
        self.assertFalse(output.exists())

    def test_case_sp_005_rejects_missing_source_plan(self):
        with self.assertRaisesRegex(MODULE.SourcePlanError, "regular file"):
            MODULE.resolve_source_plan("docs/temp/missing.md", self.root)

    def test_case_sp_006_rejects_directory_source_plan(self):
        directory = self.root / "docs" / "temp" / "directory"
        directory.mkdir()
        with self.assertRaisesRegex(MODULE.SourcePlanError, "regular file"):
            MODULE.resolve_source_plan("docs/temp/directory", self.root)

    def test_case_sp_007_stops_on_permission_error_without_fallback_read(self):
        calls = []

        def regular_file(path):
            calls.append(("stat", path))
            return True

        def denied_read(path):
            calls.append(("read", path))
            raise PermissionError("permission denied")

        with self.assertRaisesRegex(MODULE.SourcePlanError, "permission denied"):
            MODULE.resolve_source_plan(
                "docs/temp/fixture.md",
                self.root,
                is_regular_file=regular_file,
                read_file=denied_read,
            )
        self.assertEqual([kind for kind, _ in calls], ["stat", "read"])

    def test_case_sp_008_rejects_relative_escape_before_read(self):
        read_file = Mock(side_effect=AssertionError("escape was read"))
        with self.assertRaisesRegex(MODULE.SourcePlanError, "outside an authorized workspace"):
            MODULE.resolve_source_plan(
                "../outside.md",
                self.root,
                is_regular_file=lambda _path: True,
                read_file=read_file,
            )
        read_file.assert_not_called()

    def test_case_sp_009_classifies_provider_failure_without_success(self):
        result = subprocess.CompletedProcess(
            ["you", "run"],
            17,
            stdout="",
            stderr="provider timeout",
        )
        with self.assertRaisesRegex(MODULE.SourcePlanError, "exit 17.*provider timeout"):
            MODULE.require_planner_success(result)

    def test_case_sp_010_rejects_partial_and_mismatched_artifacts(self):
        partial_dir = self.root / "partial"
        partial_dir.mkdir()
        partial_json = partial_dir / "source-plan-path-fixture.json"
        partial_json.write_text("{}\n", encoding="utf-8")
        partial_packet = MODULE.PacketPaths(
            partial_json,
            partial_dir / "source-plan-path-fixture.md",
        )
        with self.assertRaisesRegex(MODULE.PacketValidationError, "json=True markdown=False"):
            MODULE.validate_packet(
                partial_packet,
                expected_raw_source_plan="docs/temp/fixture.md",
                expected_persisted_source_plan=str(self.source),
                expected_content=FIXTURE_BYTES,
            )

        resolution = MODULE.resolve_source_plan("docs/temp/fixture.md", self.root)
        assert resolution is not None
        packet = MODULE.write_packet(self.root / "mismatch", resolution)
        document = json.loads(packet.json_path.read_text(encoding="utf-8"))
        document["context"]["sourcePlan"] = str(self.root / "other.md")
        packet.json_path.write_text(json.dumps(document), encoding="utf-8")
        with self.assertRaisesRegex(MODULE.PacketValidationError, "values differ"):
            MODULE.validate_packet(
                packet,
                expected_raw_source_plan="docs/temp/fixture.md",
                expected_persisted_source_plan=str(self.source),
                expected_content=FIXTURE_BYTES,
            )

    def test_case_sp_011_concurrent_fixtures_keep_independent_paths_and_bytes(self):
        cases = []
        for label in ("first", "second"):
            root = self.root / label
            source = root / "docs" / "temp" / f"{label}.md"
            source.parent.mkdir(parents=True)
            content = f"{label} fixture bytes\n".encode()
            source.write_bytes(content)
            cases.append((root, source, content))

        def run_case(case):
            root, source, content = case
            resolution, packet = MODULE.plan_and_write(
                root / "packet",
                str(source.relative_to(root)).replace(os.sep, "/"),
                root,
            )
            assert resolution is not None
            MODULE.validate_packet(
                packet,
                expected_raw_source_plan=str(source.relative_to(root)).replace(os.sep, "/"),
                expected_persisted_source_plan=str(source),
                expected_content=content,
                consumer_cwd=root,
            )
            return resolution.persisted_source_plan, resolution.content

        with ThreadPoolExecutor(max_workers=2) as executor:
            results = list(executor.map(run_case, cases))
        self.assertEqual(results, [(str(cases[0][1]), cases[0][2]), (str(cases[1][1]), cases[1][2])])

    def test_case_sp_012_cancellation_leaves_no_partial_packet(self):
        output = self.root / "cancelled"
        cancel_event = threading.Event()
        cancel_event.set()
        with self.assertRaises(MODULE.PlannerCancelled):
            MODULE.plan_and_write(
                output,
                "docs/temp/fixture.md",
                self.root,
                cancel_event=cancel_event,
            )
        self.assertFalse(output.exists())

        late_output = self.root / "cancelled-after-read"
        late_cancel = threading.Event()

        def read_and_cancel(path):
            late_cancel.set()
            return Path(path).read_bytes()

        with self.assertRaises(MODULE.PlannerCancelled):
            MODULE.plan_and_write(
                late_output,
                "docs/temp/fixture.md",
                self.root,
                cancel_event=late_cancel,
                read_file=read_and_cancel,
            )
        self.assertFalse(late_output.exists())

    def test_case_sp_013_repeat_is_stable_without_prefix_or_slash_drift(self):
        output = self.root / "repeat"
        first_resolution, first_packet = MODULE.plan_and_write(
            output,
            "docs/temp/fixture.md",
            self.root,
        )
        first_json = first_packet.json_path.read_text(encoding="utf-8")
        first_markdown = first_packet.markdown_path.read_text(encoding="utf-8")
        second_resolution, second_packet = MODULE.plan_and_write(
            output,
            "docs/temp/fixture.md",
            self.root,
        )
        self.assertEqual(first_resolution, second_resolution)
        self.assertEqual(first_json, second_packet.json_path.read_text(encoding="utf-8"))
        self.assertEqual(first_markdown, second_packet.markdown_path.read_text(encoding="utf-8"))
        self.assertNotIn(str(self.root) + os.sep + str(self.root), first_resolution.persisted_source_plan)

    def test_case_sp_014_no_plan_preserves_null_compatibility(self):
        resolution, packet = MODULE.plan_and_write(self.root / "no-plan", None, self.root)
        self.assertIsNone(resolution)
        document = MODULE.validate_packet(
            packet,
            expected_raw_source_plan=None,
            expected_persisted_source_plan=None,
            expected_content=None,
        )
        self.assertIsNone(document["context"]["sourcePlan"])

    def test_case_sp_015_absolute_reference_reads_exact_bytes_from_consumer_cwd(self):
        resolution, packet = MODULE.plan_and_write(
            self.root / "consumer",
            "docs/temp/fixture.md",
            self.root,
        )
        assert resolution is not None
        before = Path.cwd()
        with MODULE.temporary_git_worktree(REPO_ROOT) as consumer_cwd:
            MODULE.validate_packet(
                packet,
                expected_raw_source_plan="docs/temp/fixture.md",
                expected_persisted_source_plan=str(self.source),
                expected_content=FIXTURE_BYTES,
                consumer_cwd=consumer_cwd,
            )
        self.assertEqual(Path.cwd(), before)


if __name__ == "__main__":
    unittest.main()
