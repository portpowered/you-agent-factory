"""Preparation failure behavior; no repository topology assertions."""
import argparse
import os
import tempfile
from pathlib import Path
import unittest
from unittest.mock import patch
import prepare
from preparation_environment import owned_paths, child_environment, seed_inputs


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

    def cached_fixture(self, root):
        source, cache, toolchain, output = [root / name for name in ('source', 'cache', 'go', 'output')]
        source.mkdir()
        (source / 'go.mod').write_text('module fixture\ngo 1.25.0\nrequire Example.org/Lib v1.0.0\n')
        module = cache / '!example.org/!lib@v1.0.0'
        module.mkdir(parents=True)
        (module / 'lib.go').write_text('package lib')
        metadata = cache / 'cache/download/!example.org/!lib/@v'
        metadata.mkdir(parents=True)
        for extension in ('mod', 'info', 'ziphash'):
            (metadata / ('v1.0.0.' + extension)).write_text('cached')
        (toolchain / 'bin').mkdir(parents=True)
        (toolchain / 'bin' / ('go.exe' if os.name == 'nt' else 'go')).write_text('cached executable')
        return source, cache, toolchain, output

    def test_children_override_shared_environment_and_network(self):
        with tempfile.TemporaryDirectory() as tmp:
            paths = owned_paths(Path(tmp) / 'owned')
            with patch.dict('os.environ', {'HOME': 'shared', 'GOENV': 'shared-config',
                                        'GOPROXY': 'https://example.com', 'GOFLAGS': '-overlay=shared',
                                        'GOPRIVATE': '*', 'GONOPROXY': '*', 'GOCACHEPROG': 'shared-cache'}):
                env = child_environment(paths)
            self.assertEqual(env['HOME'], paths['HOME'])
            for key in ('GOPROXY', 'GOSUMDB', 'GOWORK', 'GOENV', 'GOTELEMETRY'):
                self.assertEqual(env[key], 'off')
            self.assertEqual(env['GOFLAGS'], '')
            self.assertEqual(env['GOCACHEPROG'], '')
            self.assertEqual(env['GOPRIVATE'], '')
            self.assertEqual(env['GONOPROXY'], 'none')
            self.assertEqual(env['GOTOOLCHAIN'], 'auto')
            self.assertEqual(env['HOMEDRIVE'] + env['HOMEPATH'], env['HOME'])

    def test_cached_inputs_are_detached_hashed_and_owned(self):
        with tempfile.TemporaryDirectory() as tmp:
            source, cache, toolchain, output = self.cached_fixture(Path(tmp))
            paths = owned_paths(output / 'preparation/environment')
            env, command, records = seed_inputs(source, output, paths, cache, toolchain)
            self.assertTrue(Path(command).is_relative_to(output))
            self.assertEqual(env['GOROOT'], str(output / 'preparation/toolchain'))
            self.assertEqual(len(records), 5)
            for record in records:
                self.assertTrue(Path(record['path']).is_relative_to(output))
                self.assertEqual(len(record['sha256']), 64)
            copied = Path(paths['GOMODCACHE']) / '!example.org/!lib@v1.0.0/lib.go'
            copied.write_text('changed owned bytes')
            self.assertEqual((cache / '!example.org/!lib@v1.0.0/lib.go').read_text(), 'package lib')
            # Repeated preparation replaces only owned files, including read-only cache bytes.
            seed_inputs(source, output, paths, cache, toolchain)
            self.assertEqual(copied.read_text(), 'package lib')

    def test_missing_input_and_budget_fail_before_copy(self):
        with tempfile.TemporaryDirectory() as tmp:
            source, cache, toolchain, output = self.cached_fixture(Path(tmp))
            paths = owned_paths(output / 'preparation/environment')
            with self.assertRaisesRegex(ValueError, 'disk budget'):
                seed_inputs(source, output, paths, cache, toolchain, budget_bytes=1)
            self.assertFalse((output / 'preparation/toolchain').exists())
            (cache / 'cache/download/!example.org/!lib/@v/v1.0.0.mod').unlink()
            with self.assertRaisesRegex(ValueError, 'offline cached input missing'):
                seed_inputs(source, output, paths, cache, toolchain)
            self.assertFalse((output / 'preparation/toolchain').exists())


if __name__ == '__main__':
    unittest.main()
