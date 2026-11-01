"""Contracts for fast PR CI scope and browser smoke."""

import importlib.util
import pathlib
import os
import subprocess
import tempfile
import unittest

import yaml
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]


def load(name):
    """load imports the named CI helper."""
    spec = importlib.util.spec_from_file_location(name, ROOT / 'ci' / (name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class FastCIContracts(unittest.TestCase):
    def test_browser_change_scope_includes_code_and_dependency_changes(self):
        """test_browser_change_scope_includes_code_and_dependency_changes catches security-relevant updates."""
        scope = load('fast_ci_scope')
        for path in ('go.mod', 'go.sum', 'internal/tasks/cv/document.go',
                     'cmd/crawler-browser-launcher/main.go', 'internal/tasks/gptchat/tasks/testdata/crawler_failure_diagnostics.json', 'library/transport.go',
                     'ci/probe_cv_print.py', '.github/workflows/pr-validation.yml', 'Dockerfile'):
            with self.subTest(path=path):
                self.assertTrue(scope.browser_related(path))
        for path in ('README.md', 'docs/design.md', 'web/src/index.ts', 'ci/README.md'):
            with self.subTest(path=path):
                self.assertFalse(scope.browser_related(path))

    def test_real_git_diff_detects_deleted_or_renamed_browser_files(self):
        """test_real_git_diff_detects_deleted_or_renamed_browser_files prevents false skips."""
        with tempfile.TemporaryDirectory(prefix='fast-ci-diff-') as tmp:
            root = pathlib.Path(tmp)
            subprocess.run(['git', 'init', '-q', str(root)], check=True)
            subprocess.run(['git', '-C', str(root), 'config', 'user.email', 'ci@example.invalid'], check=True)
            subprocess.run(['git', '-C', str(root), 'config', 'user.name', 'CI Test'], check=True)
            (root / 'browser.go').write_text('package fixture\n')
            subprocess.run(['git', '-C', str(root), 'add', '.'], check=True)
            subprocess.run(['git', '-C', str(root), 'commit', '-qm', 'base'], check=True)
            base = subprocess.check_output(['git', '-C', str(root), 'rev-parse', 'HEAD'], text=True).strip()
            (root / 'browser.go').rename(root / 'renamed.txt')
            subprocess.run(['git', '-C', str(root), 'add', '-A'], check=True)
            subprocess.run(['git', '-C', str(root), 'commit', '-qm', 'rename'], check=True)
            out = root / 'decision'
            env = dict(os.environ, GITHUB_OUTPUT=str(out))
            subprocess.run(['python3', '-B', str(ROOT / 'ci/fast_ci_scope.py'), base, 'HEAD'],
                           cwd=root, env=env, check=True)
            self.assertEqual(out.read_text(), 'browser=true\n')
            bad = subprocess.run(['python3', '-B', str(ROOT / 'ci/fast_ci_scope.py'), 'missing-ref', 'HEAD'],
                                 cwd=root, env=env, capture_output=True)
            self.assertNotEqual(bad.returncode, 0)

    def test_workflows_keep_full_qualification_available_manually(self):
        """test_workflows_keep_full_qualification_available_manually checks dispatch and PR gates."""
        def workflow(name):
            value = yaml.safe_load((ROOT / '.github/workflows' / name).read_text())
            value['on'] = value.pop(True) if True in value else value['on']
            return value
        pr = workflow('pr-validation.yml')
        bolt = workflow('bolt-verify.yml')
        self.assertIn('pull_request', pr['on'])
        self.assertIn('workflow_dispatch', pr['on'])
        self.assertEqual(set(bolt['on']), {'workflow_dispatch'})
        self.assertIn('base_sha', bolt['on']['workflow_dispatch']['inputs'])
        self.assertIn('workflow_dispatch', pr['jobs']['benchmark']['if'])
        self.assertIn('workflow_dispatch', pr['jobs']['image']['if'])
        steps = {step.get('name'): step for step in pr['jobs']['backend']['steps']}
        self.assertEqual(steps['Run Go tests']['run'], 'go test ./... -count=1')
        self.assertIn('browser_scope.outputs.browser', steps['Run current browser smoke']['if'])
        for name in ('Prepare immutable browser dependency cache profiles', 'Run browser dependency integration tests'):
            self.assertIn('workflow_dispatch', steps[name]['if'])
        for name in ('Run go vet', 'Run golangci-lint', 'Run govulncheck'):
            self.assertNotIn('if', steps[name])
        self.assertIn('inputs.run_paired', steps['Run issue 94 paired service behavior (race)']['if'])

    def test_smoke_rejects_missing_tests_and_failed_runner(self):
        """test_smoke_rejects_missing_tests_and_failed_runner keeps smoke fail-closed."""
        smoke = load('probe_cv_smoke')
        fixtures = load('test_probe_cv_print')
        passed = fixtures.synthetic_output(instrumented=True, mode='auto')
        with mock.patch.object(smoke.probe, 'run_browser', return_value=fixtures.result(passed)):
            self.assertEqual(smoke.run_smoke(ROOT)['status'], 'passed')
        for output, code in ((fixtures.synthetic_output(instrumented=True, mode='auto',
                                                        omit=next(iter(smoke.probe.ORIGINAL_TESTS))), 0),
                             (passed, 1)):
            with self.subTest(code=code), mock.patch.object(smoke.probe, 'run_browser',
                                                           return_value=fixtures.result(output, exit=code)):
                self.assertNotEqual(smoke.run_smoke(ROOT)['status'], 'passed')


if __name__ == '__main__':
    unittest.main()
