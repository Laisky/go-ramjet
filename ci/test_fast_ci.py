"""Contracts for fast PR CI scope and browser smoke."""

import contextlib
import io
import json
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
        self.assertEqual(steps['Run current browser smoke']['env']['CV_PRINT_PROBE_BASE_REF'], '${{ github.base_ref }}')
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

    def test_smoke_retains_safe_failure_observations(self):
        """test_smoke_retains_safe_failure_observations exposes finite phases without raw output."""
        smoke = load('probe_cv_smoke')
        fixtures = load('test_probe_cv_print')
        row = smoke.probe.evaluate_cell(smoke.probe.CELLS[2], fixtures.result(
            fixtures.synthetic_output(instrumented=True, mode='auto'), exit=1))
        row['raw_stderr'] = 'private-browser-error-canary'
        output = io.StringIO()
        with mock.patch.dict(os.environ, CV_PRINT_PROBE_EXPECTED_HEAD='a' * 40), \
                mock.patch.object(smoke.probe, 'git', return_value='same-tree'), \
                mock.patch.object(smoke, 'run_smoke', return_value=row), \
                contextlib.redirect_stdout(output):
            self.assertEqual(smoke.main(), 1)
        rendered = output.getvalue()
        payload = json.loads(rendered.split(' ', 1)[1])
        self.assertEqual(payload['observations'], row['observations'])
        self.assertNotEqual(payload['status'], 'passed')
        self.assertNotIn('private-browser-error-canary', rendered)

    def test_smoke_binds_pull_request_merge_checkout_to_expected_head(self):
        """test_smoke_binds_pull_request_merge_checkout_to_expected_head accepts GitHub's merge checkout of a stale head."""
        smoke = load('probe_cv_smoke')
        passed = {'status': 'passed', 'originalPassCount': 33, 'cleanupComplete': True, 'missingOriginal': [],
                  'failedActions': 0, 'skippedActions': 0, 'observations': []}
        with tempfile.TemporaryDirectory(prefix='fast-ci-binding-') as tmp:
            root = pathlib.Path(tmp)

            def git(*args):
                """git runs one command in the throwaway repository and returns its stripped output."""
                return subprocess.run(['git', '-C', str(root), '-c', 'user.email=ci@example.invalid',
                                       '-c', 'user.name=CI Test', '-c', 'commit.gpgsign=false', *args],
                                      check=True, capture_output=True, text=True).stdout.strip()

            def commit(path, text, message):
                """commit writes one file, commits it, and returns the new commit."""
                (root / path).write_text(text)
                git('add', path)
                git('commit', '-qm', message)
                return git('rev-parse', 'HEAD')

            git('init', '-q', '-b', 'master')
            commit('shared.go', 'package fixture\n', 'base')
            git('checkout', '-qb', 'pr')
            head = commit('pr.go', 'package fixture\n// pr\n', 'pr head')
            git('checkout', '-q', 'master')
            commit('master.go', 'package fixture\n// newer master\n', 'master moves ahead')
            git('checkout', '-q', '--detach', 'master')
            git('merge', '-q', '--no-ff', '-m', 'Merge pr into master', head)
            github_merge = git('rev-parse', 'HEAD')
            git('checkout', '-q', '--detach', 'master')
            git('merge', '-q', '--no-ff', '-s', 'ours', '-m', 'Merge without pr changes', head)
            merge_without_head = git('rev-parse', 'HEAD')
            git('checkout', '-q', '--detach', head)
            descendant = commit('extra.go', 'package fixture\n// not in pr\n', 'unreviewed descendant')
            # A merge whose first parent carries an unreviewed commit on top of the head.
            merge_of_descendant = git('commit-tree', git('merge-tree', '--write-tree', descendant, head),
                                      '-p', descendant, '-p', head, '-m', 'Merge pr into descendant')
            git('checkout', '-q', '--detach', head)
            git('merge', '-q', '--no-ff', '-m', 'Merge master into pr', 'master')
            reversed_parents = git('rev-parse', 'HEAD')
            git('checkout', '-q', '--detach', 'master~1')
            commit('side.go', 'package fixture\n// not on master\n', 'side branch')
            git('merge', '-q', '--no-ff', '-m', 'Merge pr into side', head)
            merge_into_side = git('rev-parse', 'HEAD')
            git('checkout', '-q', 'master')
            unrelated = commit('later.go', 'package fixture\n// master moves again\n', 'master moves again')
            # A PR head that was updated from master, merged by GitHub after master moved on again.
            updated_head = reversed_parents
            git('checkout', '-q', '--detach', 'master')
            git('merge', '-q', '--no-ff', '-m', 'Merge updated pr into master', updated_head)
            merge_of_updated_head = git('rev-parse', 'HEAD')
            git('update-ref', 'refs/remotes/origin/master', 'master')

            def run_main(checkout, expected=head, **environment):
                """run_main runs the smoke entry point on one checkout and returns its code, output and run flag."""
                git('checkout', '-q', '--detach', checkout)
                output = io.StringIO()
                with mock.patch.dict(os.environ, CV_PRINT_PROBE_EXPECTED_HEAD=expected, **environment), \
                        mock.patch.object(smoke, 'ROOT', root), \
                        mock.patch.object(smoke, 'run_smoke', return_value=passed) as run, \
                        contextlib.redirect_stdout(output):
                    if 'CV_PRINT_PROBE_BASE_REF' not in environment:
                        os.environ.pop('CV_PRINT_PROBE_BASE_REF', None)
                    code = smoke.main()
                return code, output.getvalue(), run.called

            # The merge stays accepted after master moves again, because its base is an ancestor of the tip.
            for checkout, accepted in ((head, True), (github_merge, True), (merge_without_head, False),
                                       (descendant, False), (unrelated, False), (merge_of_descendant, False),
                                       (merge_into_side, False), (reversed_parents, False)):
                with self.subTest(checkout=checkout, accepted=accepted):
                    code, output, ran = run_main(checkout, CV_PRINT_PROBE_BASE_REF='master')
                    self.assertEqual(code == 0, accepted, output)
                    self.assertEqual(ran, accepted)
                    if not accepted:
                        self.assertEqual(output, 'CV_PRINT_SMOKE setup_failed\n')
            # A head that already contains master is bound the same way; the superseded head is not.
            for expected, accepted in ((updated_head, True), (head, False)):
                with self.subTest(updated_head_merge=expected, accepted=accepted):
                    code, output, ran = run_main(merge_of_updated_head, expected, CV_PRINT_PROBE_BASE_REF='master')
                    self.assertEqual((code == 0, ran), (accepted, accepted), output)
            for environment in ({}, {'CV_PRINT_PROBE_BASE_REF': 'missing'},
                                {'CV_PRINT_PROBE_BASE_REF': '../master'}):
                with self.subTest(base=environment):
                    self.assertEqual(run_main(github_merge, **environment)[:2], (1, 'CV_PRINT_SMOKE setup_failed\n'))


if __name__ == '__main__':
    unittest.main()
