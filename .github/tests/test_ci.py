import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


class WorkflowTests(unittest.TestCase):
    def setUp(self):
        path = Path('.github/workflows/check.yml')
        self.assertTrue(path.is_file(), 'The pull request workflow must exist.')
        self.workflow = json.loads(path.read_text())
        self.steps = self.workflow['jobs']['check']['steps']

    def test_checks_only_pull_requests_into_release_branches(self):
        self.assertEqual(self.workflow['on'], {
            'pull_request': {'branches': ['dev', 'main']},
        })

    def test_limits_repository_permissions(self):
        self.assertEqual(self.workflow['permissions'], {'contents': 'read'})
        checkout = self.steps[0]
        self.assertFalse(checkout['with']['persist-credentials'])
        self.assertFalse(any('continue-on-error' in step for step in self.steps))
        self.assertFalse(any('if' in step for step in self.steps))
        self.assertNotIn('if', self.workflow['jobs']['check'])
        self.assertEqual(self.workflow['defaults']['run']['shell'], 'bash')

    def test_pins_tools_and_actions(self):
        actions = [step['uses'] for step in self.steps if 'uses' in step]
        self.assertEqual(actions, [
            'actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1',
            'actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e',
        ])
        self.assertEqual(self.steps[1]['with']['go-version'], '1.27.3')
        install = next(step['run'] for step in self.steps if step['name'] == 'Install linter')
        self.assertIn('/v2.15.0/golangci-lint-2.15.0-linux-amd64.tar.gz', install)
        self.assertIn('9b7fe6a6bf6334b98decf8d86b11415ea7889afdabdd1959fdce23af2a9ebcea', install)
        self.assertIn('sha256sum --check', install)

    def test_propagates_check_failures(self):
        command = self.steps[-1]['run']
        with tempfile.TemporaryDirectory(dir='.test-artifacts') as directory:
            root = Path(directory).resolve()
            runner = root / 'make'
            runner.write_text('#!/bin/sh\ntest "$1" = check || exit 99\nexit 42\n')
            runner.chmod(0o755)
            result = subprocess.run(
                ['/bin/bash', '-e', '-o', 'pipefail', '-c', command],
                env=dict(os.environ, PATH=f'{root}:{os.environ["PATH"]}', RUNNER_TEMP=str(root)),
                capture_output=True, text=True, timeout=10,
            )
            self.assertEqual(result.returncode, 42, result.stderr)

    def test_rejects_a_missing_linter(self):
        with tempfile.TemporaryDirectory(dir='.test-artifacts') as directory:
            root = Path(directory).resolve()
            go = root / 'go'
            go.write_text('#!/bin/sh\nprintf "go version go1.27.3 linux/amd64\\n"\n')
            go.chmod(0o755)
            for tool in ('awk', 'grep', 'mkdir'):
                (root / tool).symlink_to(shutil.which(tool))
            result = subprocess.run(
                [shutil.which('make'), 'check-tools'],
                env=dict(os.environ, PATH=str(root)),
                capture_output=True, text=True, timeout=10,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('Install golangci-lint 2.15.0.', result.stderr)


if __name__ == '__main__':
    Path('.test-artifacts').mkdir(exist_ok=True)
    unittest.main()
