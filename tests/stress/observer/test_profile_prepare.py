"""Preparation failure behavior; no repository topology assertions."""
import argparse
import tempfile
from pathlib import Path
import unittest
from unittest.mock import patch
import prepare


class ProfilePreparationTests(unittest.TestCase):
    def arguments(self, root):
        return argparse.Namespace(source_workspace=str(root / 'source'), output=str(root / 'output'), mode='pin', build_profile=True)

    def test_wrong_pin_invalidates_previous_handoff(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'output').mkdir()
            handoff = root / 'output/profile-manifest.json'
            handoff.write_text('{}')
            with patch.object(prepare, 'git', return_value='0' * 40):
                with self.assertRaisesRegex(ValueError, 'original pin'):
                    prepare.prepare(self.arguments(root))
            self.assertFalse(handoff.exists())

    def test_dirty_tool_cannot_build_qualified_artifact(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            with patch.object(prepare, 'git', side_effect=[prepare.PIN, '', ' M tooling']), patch.object(prepare, 'build_profile') as build:
                with self.assertRaisesRegex(ValueError, 'committed clean tooling'):
                    prepare.prepare(self.arguments(root))
                build.assert_not_called()

    def test_unsupported_transform_is_actionable(self):
        with self.assertRaisesRegex(ValueError, 'unsupported constructor'):
            prepare.instrument('func New() *Host {\nreturn nil\n}', 'Host', 'observe', '')


if __name__ == '__main__':
    unittest.main()
